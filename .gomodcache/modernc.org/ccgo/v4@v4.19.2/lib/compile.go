// Copyright 2022 The CCGO Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package ccgo // import "modernc.org/ccgo/v4/lib"

import (
	"bufio"
	"encoding/json"
	"fmt"
	"go/token"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"sort"
	"strconv"
	"strings"
	"sync"

	"golang.org/x/mod/semver"
	"golang.org/x/tools/go/packages"
	"modernc.org/cc/v4"
	"modernc.org/strutil"
)

type name int

const (
	defaultLibs        = "modernc.org"
	libcV1             = defaultLibs + "/libc"    // Import paths are always slash separated.
	libcV2             = defaultLibs + "/libc/v2" // Import paths are always slash separated.
	defaultLibcPackage = libcV1

	generatedFilePrefix = "Code generated for "
	generatedFileSuffix = ", DO NOT EDIT."
	jsonMetaRawName     = "__ccgo_meta_json"
	//  package __ccgo_object_file_v1
	objectFilePackageName       = objectFilePackageNamePrefix + objectFileSemver
	objectFilePackageNamePrefix = "__ccgo_object_file_"
	objectFileSemver            = "v1"
)

const (
	// Lower values have higher priority in name allocation.
	external name = iota // storage class static, linkage external

	typename
	taggedStruct
	taggedUnion
	taggedEum
	enumConst
	importQualifier

	macro
	define

	staticInternal // storage class static, linkage internal
	staticNone     // storage class static, linkage none
	automatic      // storage class automatic, linkage none, must be pinned if address taken
	ccgoAutomatic  // storage class automatic, linkage none, must be pinned if address taken
	ccgo           // not visible to transpiled C code, taking address is ok
	field          // field name
	meta           // linker metadata

	preserve
)

var (
	_ writer = (*buf)(nil)

	// Don't change the association once established, otherwise the major
	// objectFileSemver must be incremented.
	//
	// The concatenation of a tag and a valid C identifier must not create a Go
	// keyword, neither it can be a prefix of a Go predefined/protected identifier,
	// see reservedNames.
	tags = [...]string{
		// none of the strings literals/prefixes below should be a prefix of any value
		// listed in reservedNames.
		meta:            "_",
		ccgo:            "cg",
		ccgoAutomatic:   "cc",
		define:          "df", // #define
		enumConst:       "ec", // enumerator constant
		external:        "X",  // external linkage
		field:           "fd", // struct field
		importQualifier: "iq",
		macro:           "mv", // macro value
		automatic:       "aa", // storage class automatic, linkage none
		staticInternal:  "si", // storage class static, linkage internal
		staticNone:      "sn", // storage class static, linkage none
		preserve:        "pp", // eg. TLS in iqlibc.ppTLS -> libc.TLS
		taggedEum:       "te", // tagged enum
		taggedStruct:    "ts", // tagged struct
		taggedUnion:     "tu", // tagged union
		typename:        "tn", // type name
	}

	isystem0    string
	isystem0Err error
	isystemOnce sync.Once
)

func init() {
	if !semver.IsValid(objectFileSemver) {
		panic(todo("internal error: invalid objectFileSemver: %q", objectFileSemver))
	}
}

func isystem(goos, goarch, importPath string) (string, error) {
	isystemOnce.Do(func() {
		pkgs, err := packages.Load(
			&packages.Config{
				Mode: packages.NeedFiles,
				Env:  append(os.Environ(), fmt.Sprintf("GOOS=%s", goos), fmt.Sprintf("GOARCH=%s", goarch)),
			},
			importPath,
		)
		if err != nil {
			isystem0Err = errorf("%s", err)
			return
		}

		if len(pkgs) != 1 {
			isystem0Err = errorf("%s: expected one package, loaded %d", importPath, len(pkgs))
			return
		}

		pkg := pkgs[0]
		if len(pkg.Errors) != 0 {
			var a []string
			for _, v := range pkg.Errors {
				a = append(a, v.Error())
			}
			isystem0Err = errorf("%s", strings.Join(a, "\n"))
			return
		}

		for _, fn := range pkg.GoFiles {
			dir, _ := filepath.Split(fn)
			isystem0 = filepath.Join(dir, "include", goos, goarch)
			_, isystem0Err = os.Stat(isystem0)
			if isystem0Err == nil {
				break
			}
		}
	})
	return isystem0, isystem0Err
}

type writer interface {
	w(s string, args ...interface{})
}

type discard struct{}

func (discard) w(s string, args ...interface{}) {}

type buf struct {
	b []byte
	n cc.Node

	volatileOrAtomicHandled bool
}

func (b *buf) Write(p []byte) (int, error) { b.b = append(b.b, p...); return len(p), nil }
func (b *buf) len() int                    { return len(b.b) }
func (b *buf) reset()                      { *b = buf{} }

func (b *buf) w(s string, args ...interface{}) {
	//trc("%v: %q %s", origin(2), s, args)
	fmt.Fprintf(b, s, args...)
}

func (b *buf) bytes() []byte {
	if b == nil {
		return nil
	}

	return b.b
}

func (b *buf) Format(f fmt.State, verb rune) {
	switch verb {
	case 's':
		f.Write(b.bytes())
	case 'q':
		fmt.Fprintf(f, "%q", b.bytes())
	default:
		panic(todo("%q", string(verb)))
	}
}

func tag(nm name) string {
	if nm >= 0 {
		return tags[nm]
	}

	return ""
}

// errHandler is a function called on error.
type errHandler func(msg string, args ...interface{})

// ---- main.c ----
// #define weak_alias(old, new) extern __typeof(old) new __attribute__((__weak__, __alias__(#old)))
//
// int canonical;
//
// weak_alias(canonical, alias1);
// weak_alias(canonical, alias2);
//
// ---- main.o.go ----
// var Xcanonical ppint32
//
// const ___ccgo_meta_json = `{
// 	"Aliases": {},
// 	"Visibility": {},
// 	"WeakAliases": {
// 		"Xalias1": "Xcanonical",
// 		"Xalias2": "Xcanonical"
// 	}
// }`

// https://www.vishalchovatiya.com/default-handlers-in-c-weak_alias/
type jsonMeta struct {
	Aliases     map[string]string // alias: canonical
	Visibility  map[string]string
	WeakAliases map[string]string // alias: canonical
}

type ctx struct {
	anonTypes           map[cc.Type]string // C type: tXXX
	ast                 *cc.AST
	breakCtx            string
	cfg                 *cc.Config
	compoundStmtValue   string
	continueCtx         string
	defineTaggedStructs map[string]*cc.StructType
	defineTaggedUnions  map[string]*cc.UnionType
	eh                  errHandler
	enumerators         nameSet
	exprNestLevel       int
	exprStmtLevel       int
	externsDeclared     map[string]*cc.Declarator
	externsDefined      map[string]cc.Node
	externsMentioned    map[string]struct{}
	f                   *fnCtx
	fields              map[fielder]*nameSpace
	fn                  *cc.Declarator
	ifn                 string
	imports             map[string]string // import path: qualifier
	initPatch           func(int64, *buf)
	inlineFuncs         map[*cc.Declarator]*cc.FunctionDefinition
	jsonMeta
	macrosEmited  nameSet
	maxAlign      int
	out           io.Writer
	pvoid         cc.Type
	signedInts    [9]cc.Type
	switchCtx     map[*cc.LabeledStatement]string
	taggedEnums   nameSet
	taggedStructs nameSet
	taggedUnions  nameSet
	task          *Task
	typenames     nameSet
	unsignedInts  [9]cc.Type
	verify        map[cc.Type]struct{}
	void          cc.Type
	winapiFuncs   map[string]struct{}

	nextID int
	pass   int // 0: out of function, 1: func 1st pass, 2: func 2nd pass.

	closed    bool
	hasErrors bool
	hasMain   bool
	hasWMain  bool
}

func newCtx(task *Task, eh errHandler) *ctx {
	maxAlign := 8
	switch task.goarch {
	case "arm", "386":
		maxAlign = 4
	}
	return &ctx{
		anonTypes:           map[cc.Type]string{},
		cfg:                 task.cfg,
		defineTaggedStructs: map[string]*cc.StructType{},
		defineTaggedUnions:  map[string]*cc.UnionType{},
		eh:                  eh,
		externsDeclared:     map[string]*cc.Declarator{},
		externsDefined:      map[string]cc.Node{},
		externsMentioned:    map[string]struct{}{},
		fields:              map[fielder]*nameSpace{},
		imports:             map[string]string{},
		inlineFuncs:         map[*cc.Declarator]*cc.FunctionDefinition{},
		maxAlign:            maxAlign,
		task:                task,
		verify:              map[cc.Type]struct{}{},
		jsonMeta: jsonMeta{
			Aliases:     map[string]string{},
			Visibility:  map[string]string{},
			WeakAliases: map[string]string{},
		},
	}
}

func (c *ctx) setBreakCtx(s string) func() {
	save := c.breakCtx
	c.breakCtx = s
	return func() { c.breakCtx = save }
}

func (c *ctx) setContinueCtx(s string) func() {
	save := c.continueCtx
	c.continueCtx = s
	return func() { c.continueCtx = save }
}

func (c *ctx) setSwitchCtx(m map[*cc.LabeledStatement]string) func() {
	save := c.switchCtx
	c.switchCtx = m
	return func() { c.switchCtx = save }
}

func (c *ctx) id() int {
	if c.f != nil {
		return c.f.id()
	}

	c.nextID++
	return c.nextID
}

func (c *ctx) err(err error) {
	c.hasErrors = true
	c.eh("%s", err.Error())
}

func (c *ctx) w(s string, args ...interface{}) {
	if c.closed {
		return
	}

	if _, err := fmt.Fprintf(c.out, s, args...); err != nil {
		c.err(err)
		c.closed = true
	}
}

func (c *ctx) compile(ifn, ofn string) (err error) {
	f, err := os.Create(ofn)
	if err != nil {
		return err
	}

	defer func() {
		if err2 := f.Close(); err2 != nil {
			c.err(errorf("%v", err2))
			if err == nil {
				err = err2
			}
			return
		}

		if c.hasErrors || err != nil {
			return
		}

		if !c.task.noObjFmt {
			if err2 := exec.Command("gofmt", "-s", "-w", "-r", "(x) -> x", ofn).Run(); err2 != nil {
				// if dmesgs {
				// 	b, _ := os.ReadFile(ofn)
				// 	dmesg("%s: gofmt FAIL %v:\n%s", ofn, err2, b)
				// }
				c.err(errorf("%s: gofmt: %v", ifn, err2))
				if err == nil {
					err = err2
				}
			}
		}
		if *oTraceL {
			b, _ := os.ReadFile(ofn)
			fmt.Fprintf(os.Stderr, "%s\n", b)
		}
	}()

	w := bufio.NewWriter(f)
	c.out = w

	defer func() {
		if err := w.Flush(); err != nil {
			c.err(errorf("%v", err))
		}
	}()

	sources, err := sourcesFor(c.cfg, ifn, c.task)
	if err != nil {
		return err
	}

	if c.ast, err = cc.Translate(c.cfg, sources); err != nil {
		return err
	}

	for _, v := range []cc.Type{
		c.ast.Char,
		c.ast.Int,
		c.ast.Long,
		c.ast.LongLong,
		c.ast.SChar,
		c.ast.Short,
		c.ast.UChar,
		c.ast.UInt,
		c.ast.ULong,
		c.ast.ULongLong,
		c.ast.UShort,
	} {
		sz := v.Size()
		switch {
		case cc.IsSignedInteger(v):
			if c.signedInts[sz] == nil {
				c.signedInts[sz] = v
			}
		default:
			if c.unsignedInts[sz] == nil {
				c.unsignedInts[sz] = v
			}
		}
	}
	for _, v := range []int{1, 2, 4, 8} {
		if c.signedInts[v] == nil || c.unsignedInts[v] == nil {
			return errorf("cannot determine all required C integer types")
		}
	}

	for _, v := range c.ast.Scope.Nodes["main"] {
		if x, ok := v.(*cc.Declarator); ok && x.Type().Kind() == cc.Function {
			c.hasMain = true
		}
	}
	if !c.hasMain && c.task.goos == "windows" {
		for _, v := range c.ast.Scope.Nodes["wmain"] {
			if x, ok := v.(*cc.Declarator); ok && x.Type().Kind() == cc.Function {
				c.hasWMain = true
			}
		}
	}
	c.void = c.ast.Void
	c.pvoid = c.ast.PVoid
	c.ifn = ifn
	c.prologue(c)
	c.defines(c)

	for n := c.ast.TranslationUnit; n != nil; n = n.TranslationUnit {
		c.externalDeclaration(c, n.ExternalDeclaration)
	}
	if c.task.emitFuncAliases {
		c.emitFunctionAliases()
	}
	for len(c.defineTaggedStructs) != 0 {
		var a []string
		for k := range c.defineTaggedStructs {
			a = append(a, k)
		}
		sort.Strings(a)
		for _, k := range a {
			t := c.defineTaggedStructs[k]
			c.defineStructType(c, "\n\n", nil, t)
			delete(c.defineTaggedStructs, k)
		}
	}
	for len(c.defineTaggedUnions) != 0 {
		var a []string
		for k := range c.defineTaggedUnions {
			a = append(a, k)
		}
		sort.Strings(a)
		for _, k := range a {
			t := c.defineTaggedUnions[k]
			c.defineUnionType(c, "\n\n", nil, t)
			delete(c.defineTaggedUnions, k)
		}
	}
	c.verifyTypes()
	c.w("%s", sep(c.ast.EOF))
	switch {
	case c.hasMain && c.task.tlsQualifier != "":
		c.w("\n\nfunc %smain() {\n%s%[1]sStart(%[3]smain)\n}\n", tag(preserve), c.task.tlsQualifier, tag(external))
	case c.hasWMain && c.task.tlsQualifier != "":
		c.w("\n\nfunc %smain() {\n%s%[1]sStart(%[3]swmain)\n}\n", tag(preserve), c.task.tlsQualifier, tag(external))
	}
	var a []string
	for k := range c.externsDefined {
		// trc("externsDefined %s", k)
		delete(c.externsDeclared, k)
	}
	for k := range c.externsDeclared {
		// trc("externsDeclared %s", k)
		if _, ok := c.externsMentioned[k]; ok {
			// trc("externsMentioned %s", k)
			a = append(a, k)
		}
	}
	sort.Strings(a)
	for _, k := range a {
		switch d := c.externsDeclared[k]; t := d.Type().(type) {
		case *cc.FunctionType:
			c.w("\n\nfunc %s%s%s", tag(meta), k, c.signature(t, false, false, false))
		default:
			c.w("\n\nvar %s%s %s", tag(meta), k, c.typ2(d, t, false))
		}
	}
	b, err := json.MarshalIndent(&c.jsonMeta, "", "\t")
	if err != nil {
		return err
	}

	if c.task.winapiTest == "dump" {
		c.winapiTest(c)
	}
	s := string(b)
	a = strings.Split(s, "`")
	s = strutil.JoinFields(a, "|")
	c.w("\n\nconst %s%s = `%s`", tag(meta), jsonMetaRawName, s)
	return nil
}

func (c *ctx) winapiTest(w writer) {
	var a []string
	for k := range c.winapiFuncs {
		a = append(a, k)
	}
	if len(a) == 0 {
		return
	}

	slices.Sort(a)
	w.w("\n\nfunc init() {")
	w.w("\nfor %s_, nm := range []string{", tag(preserve))
	for _, nm := range a {
		w.w("\n%q,", nm)
	}
	w.w("\n}{")
	w.w("\nfunc() {")
	w.w("defer func() { if recover() != nil { println(nm)}}()")
	w.w("\ndll.NewProc(nm).Addr()")
	w.w("\n}()")
	w.w("\n}")
	w.w("\n}")
}

func (c *ctx) typeID(t cc.Type) string {
	var b strings.Builder
	c.typ0(&b, nil, t, false, false, false)
	return b.String()
}

func (c *ctx) verifyTypes() {
	if len(c.verify) == 0 {
		return
	}

	m := map[string]cc.Type{}
	for k := range c.verify {
		m[c.typeID(k)] = k
	}
	var a []string
	for k := range m {
		a = append(a, k)
	}
	sort.Strings(a)
	c.w("\n\nfunc init() {")
	for i, k := range a {
		t := m[k]
		v := fmt.Sprintf("%sv%d", tag(preserve), i)
		c.w("\n\tvar %s %s", v, c.verifyTyp(nil, t))
		if x, ok := t.(*cc.StructType); ok {
			t := x.Tag()
			if s := t.SrcStr(); s != "" {
				c.w("\n// struct %q", s)
			}
		}
		c.w("\nif g, e := %sunsafe.%sSizeof(%s), %[2]suintptr(%[4]d); g != e { panic(%[2]sg) }", tag(importQualifier), tag(preserve), v, t.Size())
		switch x := t.(type) {
		case *cc.StructType:
			groups := map[int64]struct{}{}
			for i := 0; i < x.NumFields(); i++ {
				f := x.FieldByIndex(i)
				switch {
				case f.IsBitfield():
					if f.InOverlapGroup() {
						continue
					}

					off := f.Offset()
					if _, ok := groups[off]; ok {
						break
					}

					groups[off] = struct{}{}
					sz := int64(f.GroupSize())
					nm := fmt.Sprintf("%s__ccgo%d", tag(field), off)
					c.w("\nif g, e := %sunsafe.%sSizeof(%s.%s), %[2]suintptr(%[5]d); g != e { panic(%[2]sg) }", tag(importQualifier), tag(preserve), v, nm, sz)
					c.w("\nif g, e := %sunsafe.%sOffsetof(%s.%s), %[2]suintptr(%[5]d); g != e { panic(%[2]sg) }", tag(importQualifier), tag(preserve), v, nm, off)
				default:
					if f.IsFlexibleArrayMember() {
						continue
					}

					if f.Type().Kind() == cc.Union {
						continue
					}

					off := f.Offset()
					sz := f.Type().Size()
					al := f.Type().FieldAlign()
					nm := tag(field) + c.fieldName(x, f)
					c.w("\nif g, e := %sunsafe.%sSizeof(%s.%s), %[2]suintptr(%[5]d); g != e { panic(%[2]sg) }", tag(importQualifier), tag(preserve), v, nm, sz)
					c.w("\nif g, e := %sunsafe.%sOffsetof(%s.%s), %[2]suintptr(%[5]d); g != e { panic(%[2]sg) }", tag(importQualifier), tag(preserve), v, nm, off)
					c.w("\nif g, e := %sunsafe.%sOffsetof(%s.%s) %% %[5]d, %[2]suintptr(0); g != e { panic(%[2]sg) }", tag(importQualifier), tag(preserve), v, nm, al)
				}
			}
		}
	}
	c.w("\n}")
}

func (c *ctx) defines(w writer) {
	var a []*cc.Macro
	for _, v := range c.ast.Macros {
		if !v.IsFnLike && v.IsConst {
			a = append(a, v)
		}
	}
	if len(a) == 0 {
		return
	}

	sort.Slice(a, func(i, j int) bool { return a[i].Name.SrcStr() < a[j].Name.SrcStr() })
	for _, m := range a {
		nm := m.Name.SrcStr()
		r0 := c.normalizedMacroReplacementList0(m)
		if !m.IsConst {
			continue
		}

		var r string
		if len(r0) == 1 {
			r = r0[0].SrcStr()
			if r0[0].Ch == rune(cc.IDENTIFIER) && r == nm { // Ignore #define foo foo
				continue
			}
		}

		if r != "" {
			if !c.task.header && c.task.prefixDefineSet {
				w.w("%s%sconst %s%s = %q;", sep(m.Name), c.posComment(m), tag(define), m.Name.Src(), r)
			}
			if c.task.header && r != "INFINITY" {
				if _, err := strconv.ParseUint(r, 0, 64); err == nil {
					w.w("%s%sconst %s%s = %s;", sep(m.Name), c.posComment(m), tag(macro), m.Name.Src(), r)
					c.macrosEmited.add(nm)
					continue
				}

				if _, err := strconv.ParseFloat(r, 64); err == nil {
					w.w("%s%sconst %s%s = %s;", sep(m.Name), c.posComment(m), tag(macro), m.Name.Src(), r)
					c.macrosEmited.add(nm)
					continue
				}

				if _, err := strconv.Unquote(r); err == nil {
					w.w("%s%sconst %s%s = %s;", sep(m.Name), c.posComment(m), tag(macro), m.Name.Src(), r)
					c.macrosEmited.add(nm)
					continue
				}
			}
		}

		switch x := m.Value().(type) {
		case cc.Int64Value:
			w.w("%s%sconst %s%s = %v;", sep(m.Name), c.posComment(m), tag(macro), m.Name.Src(), x)
			c.macrosEmited.add(nm)
		case cc.UInt64Value:
			w.w("%s%sconst %s%s = %v;", sep(m.Name), c.posComment(m), tag(macro), m.Name.Src(), x)
			c.macrosEmited.add(nm)
		case cc.Float64Value:
			if s := fmt.Sprint(x); s == r {
				w.w("%s%sconst %s%s = %s;", sep(m.Name), c.posComment(m), tag(macro), m.Name.Src(), s)
				c.macrosEmited.add(nm)
				break
			}

			w.w("%s%sconst %s%s = %v;", sep(m.Name), c.posComment(m), tag(macro), m.Name.Src(), x)
		case cc.StringValue:
			w.w("%s%sconst %s%s = %q;", sep(m.Name), c.posComment(m), tag(macro), m.Name.Src(), x[:len(x)-1])
			c.macrosEmited.add(nm)
		default:
			if r == "" {
				break
			}

			a := c.normalizedMacroReplacementList0(m)
			if len(a) == 0 {
				break
			}

			t := a[0]
			switch t.Ch {
			case rune(cc.PPNUMBER):
				dot := strings.Contains(r, ".")
				var exp bool
				fp := dot
				if !fp {
					if s := strings.ToLower(r); strings.Contains(s, "e+") || strings.Contains(s, "e-") {
						exp = true
						fp = true
					}
				}
				if !fp {
					switch {
					case strings.HasSuffix(r, "LL"):
						r = r[:len(r)-len("LL")]
					case strings.HasSuffix(r, "UL"):
						r = r[:len(r)-len("UL")]
					case strings.HasSuffix(r, "L"):
						r = r[:len(r)-len("L")]
					case strings.HasSuffix(r, "U"):
						r = r[:len(r)-len("U")]
					}
					if _, err := strconv.ParseUint(r, 0, 64); err == nil {
						w.w("%s%sconst %s%s = %v;", sep(m.Name), c.posComment(m), tag(macro), m.Name.Src(), r)
						c.macrosEmited.add(nm)
						break
					}
				}

				switch {
				case fp && strings.HasSuffix(r, "F16"):
					r = r[:len(r)-len("F16")]
				case fp && strings.HasSuffix(r, "F128"):
					r = r[:len(r)-len("F128")]
				case fp && strings.HasSuffix(r, "F32x"):
					r = r[:len(r)-len("F32x")]
				case fp && strings.HasSuffix(r, "F64x"):
					r = r[:len(r)-len("F64x")]
				case fp && strings.HasSuffix(r, "F32"):
					r = r[:len(r)-len("F32")]
				case fp && strings.HasSuffix(r, "F64"):
					r = r[:len(r)-len("F64")]
				case fp && strings.HasSuffix(r, "DD"):
					r = r[:len(r)-len("DD")]
				case fp && strings.HasSuffix(r, "DF"):
					r = r[:len(r)-len("DF")]
				case fp && strings.HasSuffix(r, "DL"):
					r = r[:len(r)-len("DL")]
				case fp && strings.HasSuffix(r, "D"):
					r = r[:len(r)-len("D")]
				case fp && strings.HasSuffix(r, "F"):
					r = r[:len(r)-len("F")]
				case fp && strings.HasSuffix(r, "L"):
					r = r[:len(r)-len("L")]
				}
				if _, err := strconv.ParseFloat(r, 64); err == nil {
					switch {
					case !dot && !exp && strings.HasPrefix(r, "0"):
						w.w("%s%sconst %s%s = %q;", sep(m.Name), c.posComment(m), tag(macro), m.Name.Src(), r)
					default:
						w.w("%s%sconst %s%s = %v;", sep(m.Name), c.posComment(m), tag(macro), m.Name.Src(), r)
					}
					c.macrosEmited.add(nm)
					break
				}

				w.w("%s%sconst %s%s = %q;", sep(m.Name), c.posComment(m), tag(macro), m.Name.Src(), r)
				c.macrosEmited.add(nm)
			case rune(cc.IDENTIFIER):
				w.w("%s%sconst %s%s = %q;", sep(m.Name), c.posComment(m), tag(macro), m.Name.Src(), r)
				c.macrosEmited.add(nm)
			case rune(cc.STRINGLITERAL):
				w.w("%s%sconst %s%s = %q;", sep(m.Name), c.posComment(m), tag(macro), m.Name.Src(), r[1:len(r)-1])
				c.macrosEmited.add(nm)
			case rune(cc.LONGSTRINGLITERAL):
				r = r[1:] // -leading "L"
				w.w("%s%sconst %s%s = %q;", sep(m.Name), c.posComment(m), tag(macro), m.Name.Src(), r[1:len(r)-1])
				c.macrosEmited.add(nm)
			case rune(cc.CHARCONST):
				if _, err := strconv.Unquote(r); err == nil {
					w.w("%s%sconst %s%s = %v;", sep(m.Name), c.posComment(m), tag(macro), m.Name.Src(), r)
					c.macrosEmited.add(nm)
				}
			case rune(cc.LONGCHARCONST):
				r = r[1:] // -leading "L"
				if _, err := strconv.Unquote(r); err == nil {
					w.w("%s%sconst %s%s = %v;", sep(m.Name), c.posComment(m), tag(macro), m.Name.Src(), r)
					c.macrosEmited.add(nm)
				}
			default:
				w.w("%s%sconst %s%s = %q;", sep(m.Name), c.posComment(m), tag(macro), m.Name.Src(), r)
				c.macrosEmited.add(nm)
			}
		}
	}
}

var home = os.Getenv("HOME")

func (c *ctx) posComment(n cc.Node) string {
	if !c.task.positions {
		return ""
	}

	return fmt.Sprintf("\n// %s:\n", c.pos(n))
}

func (c *ctx) pos(n cc.Node) (r token.Position) {
	if n == nil {
		return r
	}

	if r = token.Position(n.Position()); r.IsValid() {
		switch {
		case c.task.absolutePaths:
			if s, err := filepath.Abs(r.Filename); err == nil {
				r.Filename = s
			}
		case c.task.fullPaths:
			if strings.HasPrefix(r.Filename, home) {
				r.Filename = "$HOME" + r.Filename[len(home):]
			}
		default:
			r.Filename = filepath.Base(r.Filename)
		}
	}
	return r
}

func (c *ctx) prologue(w writer) {
	w.w(`// %s%s/%s by '%s %s'%s

//go:build ignore
// +build ignore

package %s
`,
		generatedFilePrefix,
		c.task.goos, c.task.goarch,
		safeArg(filepath.Base(c.task.args[0])),
		strings.Join(safeArgs(c.task.args[1:]), " "),
		generatedFileSuffix,
		objectFilePackageName,
	)
}

func safeArg(arg string) (r string) {
	r = strings.ReplaceAll(arg, "\n", "\\n")
	// Thanks go vet.
	r = strings.ReplaceAll(r, "//go:build", "\\/\\/go:build")
	r = strings.ReplaceAll(r, "// +build", "\\/\\/ \\x2bbuild")
	return strings.ReplaceAll(r, "\r", "\\r")
}

func safeArgs(args []string) (r []string) {
	r = make([]string, 0, len(args))
	for _, v := range args {
		r = append(r, safeArg(v))
	}
	return r
}

func (c *ctx) declaratorTag(d *cc.Declarator) string { return tag(c.declaratorKind(d)) }

func (c *ctx) declaratorKind(d *cc.Declarator) name {
	switch d.Linkage() {
	case cc.External:
		return external
	case cc.Internal:
		return staticInternal
	case cc.None:
		switch {
		case d.IsStatic():
			return staticNone
		default:
			return automatic
		}
	default:
		c.err(errorf("%v: internal error: %v", d.Position(), d.Linkage()))
		return -1
	}
}
