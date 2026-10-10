// Copyright 2022 The CCGO Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package ccgo // import "modernc.org/ccgo/v4/lib"

import (
	"bytes"
	"encoding/json"
	"fmt"
	"go/constant"
	"go/token"
	"io"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"sort"
	"strconv"
	"strings"

	"golang.org/x/mod/semver"
	"golang.org/x/tools/go/packages"
	"modernc.org/gc/v2"
	"modernc.org/strutil"
)

const (
	objectFile = iota
	objectPkg
)

var (
	builtinsObject = &object{}
)

type object struct {
	defs           map[string]gc.Node // extern: node
	externs        nameSet            // CAPI
	id             string             // file name or import path
	meta           jsonMeta
	pkg            *gc.Package
	pkgName        string // for kind == objectPkg
	qualifier      string
	staticInternal nameSet

	kind int // {objectFile, objectPkg}

	imported    bool
	isExtracted bool // from an .a file
	isRequired  bool // from an .a file and needed
}

func newObject(kind int, id string) *object {
	return &object{
		defs: map[string]gc.Node{},
		kind: kind,
		id:   id,
	}
}

func (o *object) requiredFor(nm string) {
	o.isRequired = true
	// if dmesgs {
	// 	if o.isExtracted {
	// 		dmesg("object %q.isRequired for %q", o.id, nm)
	// 	}
	// }
}

func (o *object) load() (file *gc.SourceFile, err error) {
	if o.kind == objectPkg {
		return nil, errorf("object.load: internal error: wrong kind")
	}

	b, err := os.ReadFile(o.id)
	if err != nil {
		return nil, err
	}

	if file, err = gc.ParseSourceFile(&gc.ParseSourceFileConfig{}, o.id, b); err != nil {
		return nil, err
	}

	return file, nil
}

func (o *object) collectExternVars(file *gc.SourceFile) (seps map[*gc.VarSpec]string, vars map[string][]*gc.VarSpec, err error) {
	seps = map[*gc.VarSpec]string{}
	vars = map[string][]*gc.VarSpec{}
	for _, decl := range file.TopLevelDecls {
		switch x := decl.(type) {
		case *gc.VarDecl:
			if len(x.VarSpecs) == 1 {
				seps[x.VarSpecs[0]] = x.Var.Sep()
			}
			for _, spec := range x.VarSpecs {
				if len(spec.IdentifierList) != 1 {
					return nil, nil, errorf("collectExternVars: internal error")
				}

				nm := spec.IdentifierList[0].Ident.Src()
				if symKind(nm) != external {
					continue
				}

				vars[nm] = append(vars[nm], spec)
			}
		}
	}
	return seps, vars, nil
}

// link name -> type ID
func (o *object) collectTypes(file *gc.SourceFile) (types map[string]string, err error) {
	// tq	rc("==== %s (%v:)", o.id, origin(0))
	var a []string
	in := map[string]gc.Node{}
	for _, decl := range file.TopLevelDecls {
		switch x := decl.(type) {
		case *gc.TypeDecl:
			for _, spec := range x.TypeSpecs {
				ts, ok := spec.(*gc.AliasDecl)
				if !ok {
					continue
				}

				nm := ts.Ident.Src()
				if _, ok := in[nm]; ok {
					return nil, errorf("%v: type %s redeclared", o.id, nm)
				}

				in[nm] = ts.TypeNode // eg. in["tn__itimer_which_t"] = TypeNode for ppint32
				a = append(a, nm)
			}
		}
	}
	sort.Strings(a)
	types = map[string]string{}
	for _, linkName := range a {
		if _, ok := types[linkName]; !ok {
			if types[linkName], err = typeID(in, types, in[linkName]); err != nil {
				return nil, err
			}
		}
	}
	return types, nil
}

// link name -> const value
func (o *object) collectConsts(file *gc.SourceFile) (consts map[string]string, err error) {
	var a []string
	in := map[string]string{}
	for _, decl := range file.TopLevelDecls {
		switch x := decl.(type) {
		case *gc.ConstDecl:
			for _, spec := range x.ConstSpecs {
				for i, ident := range spec.IdentifierList {
					nm := ident.Ident.Src()
					if _, ok := in[nm]; ok {
						return nil, errorf("%v: const %s redeclared", o.id, nm)
					}

					var b strings.Builder
					if assert && len(spec.ExprList) == 0 {
						panic(todo("%v: %q", file.EOF.Position().Filename, x.Source(false)))
					}

					b.Write(spec.ExprList[i].Expr.Source(true))
					in[nm] = b.String()
					a = append(a, nm)
				}
			}
		}
	}
	sort.Strings(a)
	consts = map[string]string{}
	for _, linkName := range a {
		// trc("%s: consts[%q] = %q", o.id, linkName, in[linkName]) //TODO-DBG
		consts[linkName] = in[linkName]
	}
	return consts, nil
}

func (t *Task) link() (err error) {
	// 	if dmesgs {
	// 		dmesg("%v: t.linkFiles %v", origin(1), t.linkFiles)
	// 		defer func() {
	// 			if err != nil {
	// 				dmesg("", errorf("", err))
	// 			}
	//		}()
	//	}

	if len(t.inputFiles)+len(t.linkFiles) == 0 {
		return errorf("no input files")
	}

	if !t.keepObjectFiles {
		defer func() {
			for _, v := range t.compiledfFiles {
				os.Remove(v)
			}
		}()
	}

	if len(t.inputFiles) != 0 {
		if err := t.compile(""); err != nil {
			return err
		}
	}

	for i, v := range t.linkFiles {
		if x, ok := t.compiledfFiles[v]; ok {
			t.linkFiles[i] = x
		}
	}

	fset := token.NewFileSet()
	objects := map[string]*object{}
	var libc *object
	var linkFiles []string
	for _, v := range t.linkFiles {
		// if dmesgs {
		// 	dmesg("%v: link file %s", origin(1), v)
		// }
		var object *object
		switch {
		case strings.HasPrefix(v, "-l="):
			for _, prefix := range t.L {
				switch {
				case strings.HasPrefix(prefix, "/"): // -L/foo/bar
					continue
				case strings.HasPrefix(prefix, "."): // -L.
					continue
				case strings.Contains(prefix, "/"):
					a := strings.Split(prefix, "/")
					if !strings.Contains(a[0], ".") { // -Lfoo/bar
						continue
					}

					// -Lexample.com/foo
				}

				lib := "lib" + v[len("-l="):]
				ip := prefix + "/" + lib
				if prefix == defaultLibs && lib == "libc" {
					ip = t.libc
				}
				object, err = t.getPkgSymbols(ip)
				if err == nil {
					if object.pkgName == "libc" && libc == nil {
						libc = object
					}
					break
				}
			}
		default:
			object, err = t.getFileSymbols(fset, v)
		}
		if err != nil {
			if t.isExeced {
				// if dmesgs {
				// 	dmesg("%q: ignoring %v", v, err)
				// }
				continue
			}

			return err
		}

		if object == nil {
			continue
		}

		_, object.isExtracted = t.archiveLinkFiles[v]
		linkFiles = append(linkFiles, v)
		if _, ok := objects[v]; !ok {
			objects[v] = object
		}
	}
	fset = nil

	switch {
	case t.o == "":
		switch len(t.inputFiles) {
		case 1:
			nm := t.inputFiles[0]
			ext := filepath.Ext(nm)
			t.o = nm[:len(nm)-len(ext)] + ".go"
		default:
			t.o = fmt.Sprintf("a_%s_%s.go", t.goos, t.goarch)
		}
		fallthrough
	case strings.HasSuffix(t.o, ".go"):
		l, err := newLinker(t, libc)
		if err != nil {
			return err
		}

		return l.link(t.o, linkFiles, objects)
	default:
		return errorf("TODO t.o %q, t.args %q, t.inputFiles %q,  t.compiledfFiles %q, t.linkFiles %q", t.o, t.args, t.inputFiles, t.compiledfFiles, t.linkFiles)
	}
}

func (t *Task) getPkgSymbols(importPath string) (r *object, err error) {
	if dmesgs {
		dmesg("==== import %q t.goos=%v t.goarch=%v", importPath, t.goos, t.goarch)
	}
	// if dmesgs {
	// 	defer func() {
	// 		switch {
	// 		case r != nil:
	// 			dmesg("lib importPath %q: (%q, %v)", importPath, r.id, err)
	// 		default:
	// 			dmesg("lib importPath %q: (%p, %v)", importPath, r, err)
	// 		}
	// 	}()
	// }
	pkgs, err := packages.Load(
		&packages.Config{
			Mode: packages.NeedFiles,
			Env:  append(os.Environ(), fmt.Sprintf("GOOS=%s", t.goos), fmt.Sprintf("GOARCH=%s", t.goarch)),
		},
		importPath,
	)
	if err != nil {
		return nil, err
	}

	if len(pkgs) != 1 {
		return nil, errorf("%s: expected one package, loaded %d", importPath, len(pkgs))
	}

	pkg := pkgs[0]
	if len(pkg.Errors) != 0 && !t.ignoreLinkErrors {
		var a []string
		for _, v := range pkg.Errors {
			a = append(a, v.Error())
		}
		return nil, errorf("%s", strings.Join(a, "\n"))
	}

	r = newObject(objectPkg, importPath)
	for _, fn := range pkg.GoFiles {
		// if dmesgs {
		// 	dmesg("importing file %q", fn)
		// }
		b, err := os.ReadFile(fn)
		if err != nil {
			return nil, errorf("%s: %v", importPath, err)
		}

		file, err := gc.ParseSourceFile(&gc.ParseSourceFileConfig{}, fn, b)
		if err != nil {
			return nil, errorf("%s: %v", importPath, err)
		}

		if r.pkgName == "" {
			r.pkgName = file.PackageClause.PackageName.Src()
		}
		for _, v := range file.TopLevelDecls {
			switch x := v.(type) {
			case *gc.FunctionDecl:
				if !isCapiFunc(x) {
					break
				}

				// if dmesgs {
				// 	dmesg("imported func %q", x.FunctionName.Src())
				// }
				r.externs.add(x.FunctionName.Src())
			case *gc.VarDecl:
				for _, v := range x.VarSpecs {
					for _, id := range v.IdentifierList {
						nm := id.Ident.Src()
						if !isCapiName(nm) {
							continue
						}

						// 						if dmesgs {
						// 							dmesg("imported var %q", nm)
						// 						}
						r.externs.add(nm)
					}
				}
			}
		}
	}
	return r, nil
}

func isCapiName(nm string) bool {
	return strings.HasPrefix(nm, "X") && len(nm) > 1
}

func isCapiFunc(n *gc.FunctionDecl) bool {
	nm := n.FunctionName.Src()
	if !isCapiName(nm) {
		return false
	}

	if n.TypeParameters != nil {
		return false
	}

	sig := n.Signature
	if sig.Parameters == nil {
		return false
	}

	pl := sig.Parameters.ParameterList
	if len(pl) == 0 {
		return false
	}

	pd0 := pl[0]
	if len(pd0.IdentifierList) != 1 {
		return false
	}

	switch x := pd0.Type.(type) {
	case *gc.PointerTypeNode:
		switch y := x.BaseType.(type) {
		case *gc.TypeNameNode:
			return y.Name.Ident.Src() == "TLS"
		}
	}
	return false
}

func (t *Task) getFileSymbols(fset *token.FileSet, fn string) (r *object, err error) {
	b, err := os.ReadFile(fn)
	if err != nil {
		return nil, err
	}

	var pkgName string
	file, err := gc.ParseSourceFile(&gc.ParseSourceFileConfig{
		Accept: func(file *gc.SourceFile) error {
			pkgName = file.PackageClause.PackageName.Src()
			if !strings.HasPrefix(pkgName, objectFilePackageNamePrefix) {
				return errorf("%s: package %s is not a ccgo object file", fn, pkgName)
			}

			version := pkgName[len(objectFilePackageNamePrefix):]
			if !semver.IsValid(version) {
				return errorf("%s: package %s has invalid semantic version", fn, pkgName)
			}

			if semver.Compare(version, objectFileSemver) != 0 {
				return errorf("%s: package %s has incompatible semantic version compared to %s", fn, pkgName, objectFileSemver)
			}

			return nil
		},
	}, fn, b)
	if err != nil {
		return nil, err
	}

	for _, line := range strings.Split(file.PackageClause.Package.Sep(), "\n") {
		if !strings.HasPrefix(line, "//") {
			continue
		}

		x := strings.Index(line, generatedFilePrefix)
		if x < 0 {
			continue
		}

		s := line[x+len(generatedFilePrefix):]
		if len(s) == 0 {
			continue
		}

		if !strings.HasPrefix(s, fmt.Sprintf("%s/%s", t.goos, t.goarch)) {
			return nil, errorf("%s: object file was compiled for a different target: %s", fn, line)
		}
	}

	r = newObject(objectFile, fn)
	ex := tag(external)
	si := tag(staticInternal)
	for _, v := range file.TopLevelDecls {
		var a []gc.Token
		switch x := v.(type) {
		case *gc.ConstDecl:
			if len(x.ConstSpecs) == 0 {
				continue
			}

			cs := x.ConstSpecs[0]
			idl := cs.IdentifierList
			exprl := cs.ExprList
			if len(idl) == 0 || len(exprl) == 0 {
				continue
			}

			if isJsonMeta(idl[0].Ident.Src()) {
				switch v := exprl[0].Expr.Value(); v.Kind() {
				case constant.String:
					s := constant.StringVal(v)
					a := strutil.SplitFields(s, "|")
					if err := json.Unmarshal([]byte(strings.Join(a, "`")), &r.meta); err != nil {
						return nil, err
					}
				default:
					panic(todo("", v.Kind()))
				}
			}
		case *gc.TypeDecl:
			continue
		case *gc.VarDecl:
			for _, v := range x.VarSpecs {
				for _, id := range v.IdentifierList {
					a = append(a, id.Ident)
				}
			}
		case *gc.FunctionDecl:
			a = append(a, x.FunctionName)
		default:
			panic(todo("%T", x))
		}
		for _, id := range a {
			k := id.Src()
			switch symKind(k) {
			case external:
				if _, ok := r.externs[k]; ok {
					return nil, errorf("invalid object file: multiple defintions of %s", k[len(ex):])
				}

				r.externs.add(k)
			case staticInternal:
				if _, ok := r.staticInternal[k]; ok {
					return nil, errorf("invalid object file: multiple defintions of %s", k[len(si):])
				}

				r.staticInternal.add(k)
			}
		}
	}
	return r, nil
}

type externVar struct {
	rendered []byte
	spec     *gc.VarSpec
}

type linker struct {
	aliases               map[string]string // alias: canonical
	errors                errors
	externVars            map[string]*externVar // key: linkname
	externs               map[string]*object
	fileLinkNames2GoNames dict
	fileLinkNames2IDs     dict
	forceExternalPrefix   nameSet
	fset                  *token.FileSet
	goSynthDeclsProduced  nameSet
	goTags                []string
	imports               []*object
	importsByPath         map[string]*object
	libAliases            map[string]string // linkname: aka linkname, generate var aka = name for aliased functions
	libc                  *object
	maxUintptr            uint64
	out                   io.Writer
	reflectName           string
	runtimeName           string
	stringLiterals        map[string]int64
	synthDecls            map[string][]byte
	task                  *Task
	textSegment           strings.Builder
	textSegmentName       string
	textSegmentNameP      string
	textSegmentOff        int64
	tld                   nameSpace
	tldTypes              map[string]struct{ linkName, goName string } // TLD type ID -> info
	undefsReported        nameSet
	unsafeName            string

	closed      bool
	keepStrings bool
	needFpFunc  bool
}

func newLinker(task *Task, libc *object) (*linker, error) {
	goTags := tags
	for i := range tags {
		switch name(i) {
		case ccgoAutomatic, ccgo:
			goTags[i] = task.prefixCcgoAutomatic
		case define:
			goTags[i] = task.prefixDefine
		case enumConst:
			goTags[i] = task.prefixEnumerator
		case external:
			goTags[i] = task.prefixExternal
		case field:
			goTags[i] = task.prefixField
		case importQualifier:
			goTags[i] = task.prefixImportQualifier
		case macro:
			goTags[i] = task.prefixMacro
		case automatic:
			goTags[i] = task.prefixAutomatic
		case staticInternal:
			goTags[i] = task.prefixStaticInternal
		case staticNone:
			goTags[i] = task.prefixStaticNone
		case preserve:
			goTags[i] = ""
		case taggedEum:
			goTags[i] = task.prefixTaggedEnum
		case taggedStruct:
			goTags[i] = task.prefixTaggedStruct
		case taggedUnion:
			goTags[i] = task.prefixTaggedUnion
		case typename:
			goTags[i] = task.prefixTypename
		case meta:
			// nop
		//TODO case unpinned:
		//TODO 	goTags[i] = task.prefixUnpinned
		//TODO case externalUnpinned:
		//TODO 	goTags[i] = task.prefixExternalUnpinned
		default:
			return nil, errorf("internal error: %v", name(i))
		}
	}
	maxUintptr := uint64(math.MaxUint64)
	switch task.goarch {
	case "386", "arm":
		maxUintptr = math.MaxUint32
	}
	return &linker{
		aliases:        map[string]string{},
		externVars:     map[string]*externVar{},
		externs:        map[string]*object{},
		fset:           token.NewFileSet(),
		goTags:         goTags[:],
		importsByPath:  map[string]*object{},
		keepStrings:    task.keepStrings,
		libAliases:     map[string]string{},
		libc:           libc,
		maxUintptr:     maxUintptr,
		stringLiterals: map[string]int64{},
		synthDecls:     map[string][]byte{},
		task:           task,
		tldTypes:       map[string]struct{ linkName, goName string }{},
	}, nil
}

func (l *linker) err(err error)                      { l.errors.add(err) }
func (l *linker) rawName(linkName string) (r string) { return linkName[len(tag(symKind(linkName))):] }

func (l *linker) goName(linkName string) (r string) {
	return l.goTags[symKind(linkName)] + l.rawName(linkName)
}

func (l *linker) w(s string, args ...interface{}) {
	if l.closed {
		return
	}

	if _, err := fmt.Fprintf(l.out, s, args...); err != nil {
		l.err(err)
		l.closed = true
	}
}

func (l *linker) registerLibAliases(obj *object) error {
	for aka, name := range obj.meta.Aliases {
		// eg. "XFcPatternDestroy": "XIA__FcPatternDestroy"
		visibility := obj.meta.Visibility[aka]
		// eg. "XFcPatternDestroy": "default"
		if visibility != "default" {
			continue
		}

		ex := l.libAliases[name]
		if ex != "" && ex != aka {
			return errorf("conflicting function alias %q: %q and %q", aka, ex, name)
		}

		l.libAliases[name] = aka
		// if dmesgs && {
		// 	dmesg("ALIASED[%q]=%q", aka, name)
		// }
	}
	return nil
}

func (l *linker) link(ofn string, linkFiles []string, objects map[string]*object) (err error) {
	//	if dmesgs {
	//		dmesg("packageName=%v inputFiles=%v inputArchives=%v linkFiles=%v", l.task.packageName, l.task.inputFiles, l.task.inputArchives, linkFiles)
	// 		dmesg("link(%q, %q)", ofn, linkFiles)
	// 		defer func() {
	// 			if err != nil {
	// 				dmesg("", errorf("", err))
	// 			}
	//		}()
	//	}

	// ccgo -o libfoo.go libfoo.a
	handleLibAliases := (l.task.packageName != "" && l.task.packageName != "main") && len(l.task.inputFiles) == 0 && len(l.task.inputArchives) == 1

	//TODO Force a link error for things not really supported or that only panic at runtime.
	var tld nameSet
	// Build the symbol table. First try normal definitions.
	for _, linkFile := range linkFiles {
		object := objects[linkFile]
		if handleLibAliases {
			if err := l.registerLibAliases(object); err != nil {
				return err
			}
		}

		// 		if dmesgs {
		// 			dmesg("checking object.id=%s", object.id)
		// 		}
		for nm := range object.externs { // object defines nm
			// 			if dmesgs {
			// 				dmesg("extern %s declared in %s", nm, object.id)
			// 			}
			if _, ok := l.externs[nm]; !ok { // extern is unresolved
				l.externs[nm] = object
				object.requiredFor(nm)
				// 				if dmesgs {
				// 					dmesg("extern %s resolved in %s", nm, object.id)
				// 				}
			}
			tld.add(nm)
		}
	}
	// Then try aliases
	for _, linkFile := range linkFiles {
		object := objects[linkFile]
		for alias, canonical := range object.meta.Aliases { // object defines an alias for canonical
			// 			if dmesgs {
			// 				dmesg("extern %s alias in %s", nm, object.id)
			// 			}
			if _, ok := l.externs[alias]; !ok { // extern is still unresolved
				l.externs[alias] = object
				object.requiredFor(alias)
				l.aliases[alias] = canonical
				// 				if dmesgs {
				// 					dmesg("extern %s alias resolved in %s", nm, object.id)
				// 				}
			}
			tld.add(alias)
		}
	}
	// Then try weak aliases
	for _, linkFile := range linkFiles {
		object := objects[linkFile]
		for alias, canonical := range object.meta.WeakAliases { // object defines a weak alias for canonical
			// 			if dmesgs {
			// 				dmesg("extern %s weak alias in %s", nm, object.id)
			// 			}
			if _, ok := l.externs[alias]; !ok { // extern is still unresolved
				l.externs[alias] = object
				object.requiredFor(alias)
				l.aliases[alias] = canonical
				// 				if dmesgs {
				// 					dmesg("extern %s weak alias resolved in %s", nm, object.id)
				// 				}
			}
			tld.add(alias)
		}
	}
	l.tld.registerNameSet(l, tld, true)
	l.textSegmentNameP = l.tld.reg.put(ccgoTS)
	l.textSegmentName = l.tld.reg.put(ccgoTS)
	l.reflectName = l.tld.reg.put("reflect")
	l.runtimeName = l.tld.reg.put("runtime")
	l.unsafeName = l.tld.reg.put("unsafe")

	// Check for unresolved references.
	type undef struct {
		pos token.Position
		nm  string
	}
	var undefs []undef
	for _, linkFile := range linkFiles {
		object := objects[linkFile]
		if object.isExtracted && !object.isRequired {
			continue
		}

		switch {
		case object.kind == objectFile:
			file, err := object.load()
			if err != nil {
				return errorf("loading %s: %v", object.id, err)
			}

			for nm, pos := range unresolvedSymbols(file) {
				if !strings.HasPrefix(nm, tag(external)) {
					continue
				}

				lib, ok := l.externs[nm]
				if !ok {
					// trc("%q %v: %q", object.id, pos, nm)
					switch r := l.rawName(nm); {
					case strings.HasPrefix(r, "__builtin_"):
						l.externs[nm] = builtinsObject
					default:
						// 						if dmesgs {
						// 							dmesg("extern %s NOT resolved", nm)
						// 						}
						undefs = append(undefs, undef{pos, nm})
					}
					continue
				}

				// trc("extern %q found in %q", nm, lib.id)
				if lib.kind == objectFile {
					continue
				}

				if l.task.prefixExternal != "X" {
					l.forceExternalPrefix.add(nm)
				}
				if lib.qualifier == "" {
					lib.qualifier = l.tld.registerName(l, tag(importQualifier)+lib.pkgName)
					l.imports = append(l.imports, lib)
					lib.imported = true
					l.importsByPath[lib.id] = lib
				}
			}
		}
	}
	if len(undefs) != 0 && !l.task.header && !l.task.ignoreLinkErrors {
		sort.Slice(undefs, func(i, j int) bool {
			a, b := undefs[i].pos, undefs[j].pos
			if a.Filename < b.Filename {
				return true
			}

			if a.Filename > b.Filename {
				return false
			}

			if a.Line < b.Line {
				return true
			}

			if a.Line > b.Line {
				return false
			}

			return a.Column < b.Column
		})
		var a []string
		for _, v := range undefs {
			r := l.rawName(v.nm)
			if l.undefsReported.add(r) {
				a = append(a, errorf("%v: undefined: %q %v", v.pos, r, symKind(v.nm)).Error())
			}
		}
		err := errorf("%s", strings.Join(a, "\n"))
		if !l.task.isExeced {
			return err
		}

		l.err(err)
	}

	if libc := l.libc; libc != nil && !libc.imported {
		libc.qualifier = l.tld.registerName(l, tag(importQualifier)+libc.pkgName)
		l.imports = append(l.imports, libc)
		libc.imported = true
		l.importsByPath[libc.id] = libc
	}

	out := bytes.NewBuffer(nil)
	l.out = out

	nm := l.task.packageName
	if nm == "" {
		nm = "main"
	}
	l.prologue(nm)
	if !l.task.header {
		l.w("\n\nimport (")
		switch nm := l.reflectName; nm {
		case "reflect":
			l.w("\n\t\"reflect\"")
		default:
			l.w("\n\t%s \"reflect\"", nm)
		}
		rtDummy := ""
		switch nm := l.unsafeName; nm {
		case "unsafe":
			l.w("\n\t\"unsafe\"")
		default:
			l.w("\n\t%s \"unsafe\"", nm)
		}
		if len(l.imports) != 0 {
			l.w("\n")
		}
		for _, v := range l.imports {
			l.w("\n\t")
			if v.pkgName != v.qualifier {
				l.w("%s ", v.qualifier)
			}
			l.w("%q", v.id)
		}
		if len(l.task.imports) != 0 {
			l.w("\n")
			for _, v := range l.task.imports {
				switch x := strings.IndexByte(v, '='); {
				case x > 0:
					l.w("\n\t%s %q", v[:x], v[x+1:])
				default:
					l.w("\n\t%q", v)
				}
			}
		}
		l.w("\n)")
		l.w(`

var (
	_ %s.Type
	_ %s.Pointer
%s)

`, l.reflectName, l.unsafeName, rtDummy)
	}

	for _, linkFile := range linkFiles {
		object := objects[linkFile]
		if object == builtinsObject || object.kind != objectFile {
			continue
		}

		if object.isExtracted && !object.isRequired {
			continue
		}

		file, err := object.load()
		if err != nil {
			return errorf("loading %s: %v", object.id, err)
		}

		// types
		fileLinkNames2IDs, err := object.collectTypes(file)
		if err != nil {
			return errorf("loading %s: %v", object.id, err)
		}

		var linkNames []string
		for linkName := range fileLinkNames2IDs {
			linkNames = append(linkNames, linkName)
		}
		sort.Strings(linkNames)
		l.fileLinkNames2GoNames = dict{}
		for _, linkName := range linkNames {
			typeID := fileLinkNames2IDs[linkName]
			if strings.HasPrefix(typeID, "struct") || strings.HasPrefix(typeID, "[") { // aggregate types
				if nfo, ok := l.tldTypes[typeID]; ok && nfo.linkName == linkName {
					l.fileLinkNames2GoNames[linkName] = nfo.goName
					continue
				}
			}

			associatedTypeID, ok := l.fileLinkNames2IDs[linkName]
			switch {
			case ok && associatedTypeID == typeID:
				l.fileLinkNames2GoNames[linkName] = l.tld.dict[linkName]
			default:
				l.fileLinkNames2IDs.put(linkName, typeID)
				goName := l.tld.registerName(l, linkName)
				l.tldTypes[typeID] = struct{ linkName, goName string }{linkName, goName}
				l.fileLinkNames2GoNames[linkName] = goName
			}
		}

		// consts
		if fileLinkNames2IDs, err = object.collectConsts(file); err != nil {
			return errorf("loading %s: %v", object.id, err)
		}

		linkNames = linkNames[:0]
		for linkName := range fileLinkNames2IDs {
			linkNames = append(linkNames, linkName)
		}
		sort.Strings(linkNames)
		for _, linkName := range linkNames {
			constID := fileLinkNames2IDs[linkName]
			associatedConstID, ok := l.fileLinkNames2IDs[linkName]
			switch {
			case ok && associatedConstID == constID:
				l.fileLinkNames2GoNames[linkName] = l.tld.dict[linkName]
			default:
				l.fileLinkNames2IDs.put(linkName, constID)
				goName := l.tld.registerName(l, linkName)
				l.fileLinkNames2GoNames[linkName] = goName
			}
		}

		// staticInternals
		linkNames = linkNames[:0]
		for linkName := range object.staticInternal {
			linkNames = append(linkNames, linkName)
		}
		sort.Strings(linkNames)
		for _, linkName := range linkNames {
			goName := l.tld.registerName(l, linkName)
			l.fileLinkNames2GoNames[linkName] = goName
		}

		// vars
		seps, vars, err := object.collectExternVars(file)
		if err != nil {
			return errorf("loading %s: %v", object.id, err)
		}

		for linkName, specs := range vars {
			for _, spec := range specs {
				sep := seps[spec]
				switch ex := l.externVars[linkName]; {
				case ex != nil:
					switch {
					case len(spec.ExprList) == 0:
						// nop
					case len(ex.spec.ExprList) == 0:
						fi := l.newFnInfo(spec)
						var b buf
						b.w("\n%svar ", sep)
						l.print0(&b, fi, spec)
						l.externVars[linkName] = &externVar{rendered: b.bytes(), spec: spec}
					default:
						if !l.task.isExeced {
							return errorf("loading %s: multiple definitions of %s", object.id, l.rawName(linkName))
						}
					}
				default:
					fi := l.newFnInfo(spec)
					var b buf
					b.w("\n%svar ", sep)
					l.print0(&b, fi, spec)
					l.externVars[linkName] = &externVar{rendered: b.bytes(), spec: spec}
				}
			}
		}

		for _, n := range file.TopLevelDecls {
			switch x := n.(type) {
			case *gc.ConstDecl:
				if ln := x.ConstSpecs[0].IdentifierList[0].Ident.Src(); isJsonMeta(ln) {
					break
				}

				if len(x.ConstSpecs) != 1 {
					panic(todo(""))
				}

				spec := x.ConstSpecs[0]
				nm := spec.IdentifierList[0].Ident.Src()
				nm = l.fileLinkNames2GoNames[nm]
				if _, ok := l.goSynthDeclsProduced[nm]; ok {
					break
				}

				if strings.HasPrefix(nm, tag(define)) && !l.task.prefixDefineSet {
					break
				}

				l.goSynthDeclsProduced.add(nm)
				fi := l.newFnInfo(nil)
				l.print(fi, n)
				var b buf
				l.print0(&b, fi, n)
				l.synthDecls[nm] = b.bytes()
			case *gc.VarDecl:
				if ln := x.VarSpecs[0].IdentifierList[0].Ident.Src(); l.isMeta(x, ln) || symKind(ln) == external {
					break
				}

				l.print(l.newFnInfo(n), n)
			case *gc.TypeDecl:
				if len(x.TypeSpecs) != 1 {
					panic(todo(""))
				}

				spec := x.TypeSpecs[0]
				nm := spec.(*gc.AliasDecl).Ident.Src()
				nm2 := l.fileLinkNames2GoNames[nm]
				if _, ok := l.goSynthDeclsProduced[nm2]; ok {
					break
				}

				l.goSynthDeclsProduced.add(nm2)
				fi := l.newFnInfo(nil)
				l.print(fi, n)
				var b buf
				l.print0(&b, fi, n)
				l.synthDecls[nm2] = b.bytes()
			case *gc.FunctionDecl:
				ln := x.FunctionName.Src()
				if l.isMeta(x, ln) || l.task.header {
					break
				}

				l.funcDecl(x)
				if len(l.libAliases) == 0 {
					break
				}

				if !strings.HasPrefix(ln, tag(external)) {
					break
				}

				aliasNm := l.libAliases[ln]
				if aliasNm == "" || !strings.HasPrefix(aliasNm, tag(external)) {
					break
				}

				// trc("ALIAS %q -> %q", aliasNm, ln)
				l.w("\n\nvar %s = %s\n\n", aliasNm, ln)
			default:
				l.err(errorf("TODO %T", x))
			}
		}
	}
	l.epilogue()
	if l.task.debugLinkerSave {
		if err := os.WriteFile(ofn, out.Bytes(), 0666); err != nil {
			return errorf("%s", err)
		}
	}

	if err := os.WriteFile(ofn, out.Bytes(), 0666); err != nil {
		return errorf("%s", err)
	}

	switch e := exec.Command("gofmt", "-s", "-w", "-r", "(x) -> x", ofn).Run(); {
	case e != nil:
		l.err(errorf("%s: gofmt: %v", ofn, e))
	default:
		b, err := os.ReadFile(ofn)
		if err != nil {
			break
		}

		b = l.postProcess(ofn, b)
		if err := os.WriteFile(ofn, b, 0666); err != nil {
			return errorf("%s", err)
		}

		if *oTraceG {
			fmt.Fprintf(os.Stderr, "%s\n", b)
		}
	}
	return l.errors.err()
}

// Input must be formatted.
func (l *linker) postProcess(fn string, b []byte) (r []byte) {
	lines := strings.Split(string(b), "\n")
	var inFunc bool
	w := 0
	for _, line := range lines {
		switch s := strings.TrimSpace(line); {
		case strings.HasPrefix(line, "func "):
			inFunc = true
		case strings.HasPrefix(line, "}"):
			inFunc = false
		case s == "":
			if inFunc {
				continue
			}
		case strings.HasPrefix(s, "fallthrough"):
			switch p := strings.TrimSpace(lines[w-1]); {
			case strings.HasPrefix(p, "return"), strings.HasPrefix(p, "goto"), strings.HasPrefix(p, "continue"):
				continue
			case strings.HasPrefix(p, "break"):
				w--
				continue
			}
		}
		lines[w] = line
		w++
	}
	lines = lines[:w]
	return []byte(strings.Join(lines, "\n"))
}

func isJsonMeta(linkName string) bool {
	return strings.HasPrefix(linkName, tag(meta)) && linkName[len(tag(meta)):] == jsonMetaRawName
}

func (l *linker) isMeta(n gc.Node, linkName string) bool {
	if symKind(linkName) != meta {
		return false
	}

	rawName := l.rawName(linkName)
	if obj := l.externs[tag(external)+rawName]; obj != nil && obj.kind == objectPkg {
		if _, ok := obj.defs[rawName]; !ok {
			obj.defs[rawName] = n
		}
	}
	return true
}

func (l *linker) funcDecl(n *gc.FunctionDecl) {
	info := l.newFnInfo(n)
	var static []gc.Node
	w := 0
	for _, stmt := range n.FunctionBody.StatementList {
		if stmt := l.stmtPrune(stmt, info, &static); stmt != nil {
			n.FunctionBody.StatementList[w] = stmt
			w++
		}
	}
	n.FunctionBody.StatementList = n.FunctionBody.StatementList[:w]
	l.print(info, n)
	for _, v := range static {
		l.w("\n\n")
		switch x := v.(type) {
		case *gc.FunctionLit:
			l.w("func init() ")
			l.print(info, x.FunctionBody)
			l.w("\n\n")
		default:
			l.print(info, v)
		}
	}
}

func (l *linker) stmtPrune(n gc.Node, info *fnInfo, static *[]gc.Node) gc.Node {
	switch x := n.(type) {
	case *gc.VarDecl:
		if len(x.VarSpecs) != 1 {
			return n
		}

		vs := x.VarSpecs[0]
		if len(vs.IdentifierList) != 1 {
			return n
		}

		switch nm := vs.IdentifierList[0].Ident.Src(); symKind(nm) {
		case staticInternal, staticNone:
			*static = append(*static, n)
			return nil
		case preserve:
			if nm[len(tag(preserve)):] != "_" {
				break
			}

			if len(vs.ExprList) == 0 {
				break
			}

			if x, ok := vs.ExprList[0].Expr.(*gc.FunctionLit); ok {
				*static = append(*static, x)
				return nil
			}
		}
	case *gc.Block:
		w := 0
		for _, stmt := range x.StatementList {
			if stmt := l.stmtPrune(stmt, info, static); stmt != nil {
				x.StatementList[w] = stmt
				w++
			}
		}
		x.StatementList = x.StatementList[:w]
	case *gc.ForStmt:
		x.Block = l.stmtPrune(x.Block, info, static).(*gc.Block)
	case *gc.IfStmt:
		x.Block = l.stmtPrune(x.Block, info, static).(*gc.Block)
		switch y := x.ElsePart.(type) {
		case *gc.Block:
			x.ElsePart = l.stmtPrune(y, info, static)
		case *gc.IfStmt:
			x.ElsePart = l.stmtPrune(y, info, static)
		case nil:
			// nop
		default:
			l.err(errorf("TODO %T %v:", y, n.Position()))
		}
	case *gc.ExpressionSwitchStmt:
		for _, v := range x.ExprCaseClauses {
			w := 0
			for _, stmt := range v.StatementList {
				if stmt := l.stmtPrune(stmt, info, static); stmt != nil {
					v.StatementList[w] = stmt
					w++
				}
			}
			v.StatementList = v.StatementList[:w]
		}
	case *gc.LabeledStmt:
		x.Statement = l.stmtPrune(x.Statement, info, static)
	case
		*gc.Assignment,
		*gc.BreakStmt,
		*gc.ContinueStmt,
		*gc.DeferStmt,
		*gc.EmptyStmt,
		*gc.ExpressionStmt,
		*gc.FallthroughStmt,
		*gc.GotoStmt,
		*gc.IncDecStmt,
		*gc.ReturnStmt,
		*gc.ShortVarDecl:

		// nop
	default:
		l.err(errorf("TODO %T %v:", x, n.Position()))
	}
	return n
}

func (l *linker) epilogue() {
	if l.task.header {
		return
	}

	if l.needFpFunc {
		l.w(`

func %s(f interface{}) uintptr {
	type iface [2]uintptr
	return (*iface)(unsafe.Pointer(&f))[1]
}
`, ccgoFP)
	}
	var a []string
	for k := range l.externVars {
		a = append(a, k)
	}
	sort.Strings(a)
	for _, k := range a {
		l.w("\n\n%s", l.externVars[k].rendered)
	}

	if l.textSegment.Len() == 0 {
		return
	}

	l.w("\n\nvar %s = (*%s.StringHeader)(%s.Pointer(&(%s))).Data\n", l.textSegmentNameP, l.reflectName, l.unsafeName, l.textSegmentName)
	l.w("\n\nvar %s = %q\n", l.textSegmentName, l.textSegment.String())
}

func (l *linker) prologue(nm string) {
	// 	if dmesgs {
	// 		dmesg("DBG t@%p.buildLines = %q", l.task, l.task.buildLines)
	// 	}
	l.w(`// %s%s/%s by '%s %s'%s

%s

package %s

`,
		generatedFilePrefix,
		l.task.goos, l.task.goarch,
		safeArg(filepath.Base(l.task.args[0])),
		strings.Join(safeArgs(l.task.args[1:]), " "),
		generatedFileSuffix,
		l.task.buildLines,
		nm,
	)
}

type fnInfo struct {
	ns        nameSpace
	linkNames nameSet
	linker    *linker
}

func (l *linker) newFnInfo(n gc.Node) (r *fnInfo) {
	r = &fnInfo{linker: l}
	if n != nil {
		// trc("==== %v:", n.Position())
		walk(n, func(n gc.Node, pre bool, arg interface{}) {
			if !pre {
				return
			}

			tok, ok := n.(gc.Token)
			if !ok {
				return
			}

			switch tok.Ch {
			case gc.IDENTIFIER:
				switch nm := tok.Src(); symKind(nm) {
				case field, preserve, macro:
					// nop
				case staticInternal:
					r.linkNames.add(nm)
				default:
					r.linkNames.add(nm)
				}
			case gc.STRING_LIT:
				if !l.keepStrings {
					r.linker.stringLit(tok.Src(), true)
				}
			}
		}, nil)
	}
	r.ns.registerNameSet(l, r.linkNames, false)
	r.linkNames = nil
	return r
}

func (fi *fnInfo) name(linkName string) string {
	switch symKind(linkName) {
	case external:
		if fi.linker.forceExternalPrefix.has(linkName) {
			return linkName
		}

		fallthrough
	case staticInternal, staticNone:
		if goName := fi.linker.tld.dict[linkName]; goName != "" {
			return goName
		}

		r := fi.linker.rawName(linkName)
		if strings.HasPrefix(r, "__builtin_") {
			return fmt.Sprintf("%s%s%s", fi.linker.task.tlsQualifier, fi.linker.task.prefixExternal, r)
		}

		if !fi.linker.task.ignoreLinkErrors && fi.linker.undefsReported.add(r) {
			fi.linker.err(errorf("undefined: %q %v", r, symKind(linkName)))
		}
		return fi.linker.task.prefixUndefined + r
	case preserve, field:
		return fi.linker.goName(linkName)
	case automatic, ccgoAutomatic, ccgo:
		return fi.ns.dict[linkName]
	case
		typename, taggedEum, taggedStruct, taggedUnion, define, macro, enumConst:

		return fi.linker.fileLinkNames2GoNames[linkName]
	case meta:
		return "X" + linkName[len(tag(meta)):]
	case importQualifier:
		switch nm := linkName[len(tag(importQualifier)):]; nm {
		case "libc":
			if fi.linker.libc == nil {
				fi.linker.err(errorf("TODO %q %v - no libc object", linkName, symKind(linkName)))
				return linkName
			}

			return fi.linker.libc.qualifier
		case "runtime":
			return fi.linker.runtimeName
		case "unsafe":
			return fi.linker.unsafeName
		default:
			fi.linker.err(errorf("TODO %q", nm))
			return linkName
		}
	case -1:
		return linkName
	}

	fi.linker.err(errorf("TODO %q %v", linkName, symKind(linkName)))
	return linkName
}

func (l *linker) stringLit(s0 string, reg bool) string {
	s, err := strconv.Unquote(s0)
	if err != nil {
		l.err(errorf("internal error: %v", err))
	}
	off := l.textSegmentOff
	switch x, ok := l.stringLiterals[s]; {
	case ok:
		off = x
	default:
		if !reg {
			return s0
		}

		l.stringLiterals[s] = off
		l.textSegment.WriteString(s)
		l.textSegmentOff += int64(len(s))
	}
	switch {
	case off == 0:
		return l.textSegmentNameP
	default:
		return fmt.Sprintf("(%s%+d)", l.textSegmentNameP, off)
	}
}

func (l *linker) print(fi *fnInfo, n interface{}) {
	l.print0(l, fi, n)
}

func (l *linker) print0(w writer, fi *fnInfo, n interface{}) {
	if n == nil {
		return
	}

	if x, ok := n.(gc.Token); ok && x.IsValid() {
		w.w("%s", x.Sep())
		switch x.Ch {
		case gc.IDENTIFIER:
			id := x.Src()
			if id == tag(preserve)+ccgoFP {
				l.needFpFunc = true
			}
			nm := fi.name(id)
			if nm == "" {
				w.w("%s", id)
				return
			}

			if symKind(id) != external {
				w.w("%s", nm)
				return
			}

			obj := fi.linker.externs[id]
			if obj == nil {
				r := l.rawName(nm)
				w.w("%s%s", l.task.prefixUndefined, r)
				if !l.task.ignoreLinkErrors && l.undefsReported.add(r) {
					l.err(errorf("%v: undefined: %q %v", x.Position(), r, symKind(nm)))
				}
				return
			}

			if to := l.aliases[id]; to != "" {
				nm = fi.name(to)
			}
			if obj.kind == objectPkg {
				w.w("%s.%s", obj.qualifier, nm)
				return
			}

			w.w("%s", nm)
		case gc.STRING_LIT:
			if l.keepStrings {
				w.w("%s", x.Src())
				break
			}

			w.w("%s", l.stringLit(x.Src(), false))
		default:
			w.w("%s", x.Src())
		}
		return
	}

	if x, ok := n.(*gc.BinaryExpr); ok {
		switch x.Op.Ch {
		case gc.LOR:
			if bytes.Equal(x.A.Source(false), x.B.Source(false)) {
				n = x.A
			}
		}
	}

	t := reflect.TypeOf(n)
	v := reflect.ValueOf(n)
	var zero reflect.Value
	if t.Kind() == reflect.Pointer {
		t = t.Elem()
		v = v.Elem()
		if v == zero {
			return
		}
	}

	switch t.Kind() {
	case reflect.Struct:
		nf := t.NumField()
		for i := 0; i < nf; i++ {
			f := t.Field(i)
			if !f.IsExported() {
				continue
			}

			if v == zero || v.IsZero() {
				continue
			}

			l.print0(w, fi, v.Field(i).Interface())
		}
	case reflect.Slice:
		ne := v.Len()
		for i := 0; i < ne; i++ {
			l.print0(w, fi, v.Index(i).Interface())
		}
	}
}

// PackageLoader implements gc.Checker.
func (l *linker) PackageLoader(pkg *gc.Package, src *gc.SourceFile, importPath string) (r *gc.Package, err error) {
	switch importPath {
	case "reflect":
		return l.reflectPackage()
	case "unsafe":
		return l.unsafePackage()
	}

	switch obj := l.importsByPath[importPath]; {
	case obj != nil:
		var b buf
		b.w("package %s", obj.pkgName)
		var taken nameSet
		var a []string
		if obj == l.libc {
			l.synthLibc(&b, &taken, pkg)
			b.w("\n")
		}
		for k := range l.synthDecls {
			a = append(a, k)
		}
		sort.Strings(a)
		for _, k := range a {
			if !taken.has(l.rawName(k)) {
				b.Write(l.synthDecls[k])
			}
		}
		a = a[:0]
		for k := range obj.defs {
			a = append(a, k)
		}
		sort.Strings(a)
		fi := l.newFnInfo(nil)
		for _, k := range a {
			l.print0(&b, fi, obj.defs[k])
		}
		// trc("\n%s", b.bytes())
		if r, err = l.syntheticPackage(importPath, importPath, b.bytes()); err != nil {
			return nil, err
		}

		return r, nil
	default:
		return nil, errorf("TODO %s", importPath)
	}
}

func (l *linker) synthLibc(b *buf, taken *nameSet, pkg *gc.Package) {
	b.w(`

type TLS struct{
	Alloc func(int) uintptr
	Free func(int)
}

func Start(func(*TLS, int32, uintptr) int32)

func VaList(p uintptr, args ...interface{}) uintptr

`)
	taken.add("TLS")
	taken.add("Start")
	for _, v := range []string{
		"float32",
		"float64",
		"int16",
		"int32",
		"int64",
		"int8",
		"uint16",
		"uint32",
		"uint64",
		"uint8",
		"uintptr",
	} {
		nm := fmt.Sprintf("Va%s", export(v))
		taken.add(nm)
		b.w("\n\nfunc %s(*uintptr) %s", nm, v)
	}
	for _, v := range []string{
		"int16",
		"int32",
		"int64",
		"int8",
		"uint16",
		"uint32",
		"uint64",
		"uint8",
	} {
		nm := fmt.Sprintf("Bool%s", export(v))
		taken.add(nm)
		b.w("\n\nfunc %s(bool) %s", nm, v)
	}
	for _, v := range []string{
		"int16",
		"int32",
		"int64",
		"int8",
		"uint16",
		"uint32",
		"uint64",
		"uint8",
	} {
		for _, bits := range []int{8, 16, 32, 64} {
			nm := fmt.Sprintf("SetBitFieldPtr%d%s", bits, export(v))
			taken.add(nm)
			uv := v
			if !strings.HasPrefix(uv, "u") {
				uv = "u" + uv
			}
			b.w("\n\nfunc %s(uintptr, %s, int, %s)", nm, v, uv)

			nm = fmt.Sprintf("AssignBitFieldPtr%d%s", bits, export(v))
			b.w("\n\nfunc %s(uintptr, %s, int, int, %s) %s", nm, v, uv, v)

			nm = fmt.Sprintf("PostDecBitFieldPtr%d%s", bits, export(v))
			b.w("\n\nfunc %s(uintptr, %s, int, int, %s) %s", nm, v, uv, v)

			nm = fmt.Sprintf("PostIncBitFieldPtr%d%s", bits, export(v))
			b.w("\n\nfunc %s(uintptr, %s, int, int, %s) %s", nm, v, uv, v)
		}
	}
	for _, v := range []string{
		"complex128",
		"complex64",
		"float32",
		"float64",
		"int16",
		"int32",
		"int64",
		"int8",
		"uint16",
		"uint32",
		"uint64",
		"uint8",
		"uintptr",
	} {
		nm := export(v)
		taken.add(nm)
		b.w("\n\nfunc %s(%s) %[2]s", nm, v)
		for _, w := range []string{
			"complex128",
			"complex64",
			"float32",
			"float64",
			"int16",
			"int32",
			"int64",
			"int8",
			"uint16",
			"uint32",
			"uint64",
			"uint8",
			"uintptr",
		} {
			nm := fmt.Sprintf("%sFrom%s", export(v), export(w))
			taken.add(nm)
			b.w("\n\nfunc %s(%s) %s", nm, w, v)
		}
	}
}

// SymbolResolver implements gc.Checker.
func (l *linker) SymbolResolver(currentScope, fileScope *gc.Scope, pkg *gc.Package, ident gc.Token) (r gc.Node, err error) {
	// trc("symbol resolver: %p %p %q %q %q off %v", currentScope, fileScope, pkg.Name, pkg.ImportPath, ident, ident.Offset())
	nm := ident.Src()
	off := ident.Offset()
	for s := currentScope; s != nil; s = s.Parent {
		if s.IsPackage() {
			if r := fileScope.Nodes[nm]; r.Node != nil {
				// trc("defined in file scope")
				return r.Node, nil
			}

			if pkg == l.libc.pkg && nm == "libc" { // rathole
				// trc("defined in libc")
				return pkg, nil
			}
		}

		if r := s.Nodes[nm]; r.Node != nil && r.VisibleFrom <= off {
			// trc("defined in scope %p(%v), parent %p(%v), %T visible from %v", s, s.IsPackage(), s.Parent, s.Parent != nil && s.Parent.IsPackage(), r.Node, r.VisibleFrom)
			return r.Node, nil
		}
	}

	// trc("undefined: %s", nm)
	return nil, errorf("undefined: %s", nm)
}

// CheckFunctions implements gc.Checker.
func (l *linker) CheckFunctions() bool { return true }

// GOARCG implements gc.Checker.
func (l *linker) GOARCH() string { return l.task.goarch }

var (
	reflectSrc = []byte(`package reflect

type StringHeader struct {
    Data uintptr
    Len  int
}

type Type struct{}
`)
	unsafeSrc = []byte(`package unsafe

type ArbitraryType int

type Pointer *ArbitraryType

func Alignof(ArbitraryType) uintptr

func Offsetof(ArbitraryType) uintptr

func Sizeof(ArbitraryType) uintptr

func Add(Pointer, int) Pointer
`)
)

func (l *linker) reflectPackage() (r *gc.Package, err error) {
	return l.syntheticPackage("reflect", "<reflect>", reflectSrc)
}

func (l *linker) unsafePackage() (r *gc.Package, err error) {
	return l.syntheticPackage("unsafe", "<unsafe>", unsafeSrc)
}

func (l *linker) syntheticPackage(importPath, fn string, src []byte) (r *gc.Package, err error) {
	cfg := &gc.ParseSourceFileConfig{}
	sf, err := gc.ParseSourceFile(cfg, fn, src)
	if err != nil {
		return nil, err
	}

	r, err = gc.NewPackage(importPath, []*gc.SourceFile{sf})
	if err != nil {
		return nil, err
	}

	if obj := l.importsByPath[importPath]; obj != nil {
		obj.pkg = r
	}

	if err = r.Check(l); err != nil {
		return nil, err
	}

	return r, nil
}
