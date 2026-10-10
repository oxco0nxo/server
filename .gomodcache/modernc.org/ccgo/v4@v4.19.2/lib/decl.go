// Copyright 2022 The CCGO Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package ccgo // import "modernc.org/ccgo/v4/lib"

import (
	"fmt"
	"path/filepath"
	"slices"
	"sort"
	"strings"

	"modernc.org/cc/v4"
)

const (
	retvalName = "r"
	vaArgName  = "va"
)

type declInfo struct {
	d     *cc.Declarator
	bpOff int64

	addressTaken bool
}

func (n *declInfo) pinned() bool { return n.d.StorageDuration() == cc.Automatic && n.addressTaken }

type declInfos map[*cc.Declarator]*declInfo

func (n *declInfos) info(d *cc.Declarator) (r *declInfo) {
	m := *n
	if m == nil {
		m = declInfos{}
		*n = m
	}
	if r = m[d]; r == nil {
		r = &declInfo{d: d}
		m[d] = r
	}
	return r
}

func (n *declInfos) takeAddress(d *cc.Declarator) { n.info(d).addressTaken = true }

type scoper interface {
	LexicalScope() *cc.Scope
}

type flowCtx struct {
	parent *flowCtx
	stmt   scoper
}

func (c *flowCtx) new(stmt scoper) *flowCtx { return &flowCtx{parent: c, stmt: stmt} }

type inlineInfo struct {
	args           []*buf
	exit           string
	fd             *cc.FunctionDefinition
	mode           mode
	params         []*cc.Parameter
	parent         *inlineInfo
	replacedParams []string
	result         string
	vaOff          int64
}

type fnCtx struct {
	autovars         map[string][]string
	c                *ctx
	compoundLiterals map[cc.ExpressionNode]int64
	d                *cc.Declarator
	declInfos        declInfos
	flatScopes       map[*cc.Scope]struct{}
	fnResults        map[cc.ExpressionNode]int64
	inlineInfo       *inlineInfo
	locals           map[*cc.Declarator]string // storage: static or automatic, linkage: none -> C renamed
	maxVaListSize    int64
	t                *cc.FunctionType
	tlsAllocs        int64
	vlaSizes         map[*cc.Declarator]string

	nextID int

	callsAlloca bool
}

func (c *ctx) newFnCtx(d *cc.Declarator, t *cc.FunctionType, n *cc.CompoundStatement) (r *fnCtx) {
	fnScope := n.LexicalScope()
	// trc("%v: ==== fnScope %p, parent %p\n%s", n.Position(), fnScope, fnScope.Parent, dumpScope(fnScope))
	flatScopes := map[*cc.Scope]struct{}{}
next:
	for _, gotoStmt := range n.Gotos() {
		gotoScope := gotoStmt.LexicalScope()
		// trc("%v: '%s', gotoScope %p, parent %p\n%s", gotoStmt.Position(), cc.NodeSource(gotoStmt), gotoScope, gotoScope.Parent, dumpScope(gotoScope))
		var targetScope *cc.Scope
		switch x := gotoStmt.Label().(type) {
		case *cc.LabeledStatement:
			targetScope = x.LexicalScope()
			// trc("%v: '%s', targetScope %p, parent %p\n%s", x.Position(), cc.NodeSource(x), targetScope, targetScope.Parent, dumpScope(targetScope))
		default:
			c.err(errorf("TODO %T", x))
			continue next
		}

		m := map[*cc.Scope]struct{}{gotoScope: {}}
		// targetScope must be the same as gotoScope or any of its parent scopes.
		for sc := gotoScope; sc != nil && sc.Parent != nil; sc = sc.Parent {
			m[sc] = struct{}{}
			// trc("searching scope %p, parent %p\n%s", sc, sc.Parent, dumpScope(sc))
			if sc == targetScope {
				// trc("FOUND targetScope")
				continue next
			}
		}

		// Jumping into a block.
		for sc := targetScope; sc != nil && sc != fnScope; sc = sc.Parent {
			// trc("FLAT[%p]", sc)
			flatScopes[sc] = struct{}{}
			if _, ok := m[sc]; ok {
				// trc("FOUND common scope")
				break
			}
		}
	}
	var fc *flowCtx
	walkC(n, func(n cc.Node, mode int) {
		switch x := n.(type) {
		case *cc.Statement:
			switch x.Case {
			case cc.StatementSelection:
				switch mode {
				case walkPre:
					fc = fc.new(x.SelectionStatement)
				case walkPost:
					fc = fc.parent
				}
			case cc.StatementLabeled:
				switch x.LabeledStatement.Case {
				case cc.LabeledStatementCaseLabel, cc.LabeledStatementRange, cc.LabeledStatementDefault:
					switch y := fc.stmt.(type) {
					case *cc.SelectionStatement:
						if y.Case != cc.SelectionStatementSwitch {
							for c := fc; ; c = c.parent {
								flatScopes[c.stmt.LexicalScope()] = struct{}{}
								if x, ok := c.stmt.(*cc.SelectionStatement); ok && x.Case == cc.SelectionStatementSwitch {
									return
								}
							}
						}
					}
				}
			}
		}
	})
	return &fnCtx{
		c:          c,
		d:          d,
		flatScopes: flatScopes,
		t:          t,
	}
}

func (f *fnCtx) newAutovarName() (nm string) {
	// trc("%v: %v: %v:", origin(4), origin(3), origin(2))
	return fmt.Sprintf("%sv%d", tag(ccgoAutomatic), f.c.id())
}

func (f *fnCtx) newAutovar(n cc.Node, t cc.Type) (nm string) {
	nm = f.newAutovarName()
	f.registerAutoVar(nm, f.c.typ(n, t))
	// trc("%v: %s %v: %q (%v: %v: %v:)", pos(n), t, t.Kind(), nm, origin(4), origin(3), origin(2))
	return nm
}

func (f *fnCtx) registerAutoVar(nm, typ string) {
	if f.autovars == nil {
		f.autovars = map[string][]string{}
	}
	f.autovars[typ] = append(f.autovars[typ], nm)
}

func (f *fnCtx) registerLocal(d *cc.Declarator) {
	if f == nil {
		return
	}

	if f.locals == nil {
		f.locals = map[*cc.Declarator]string{}
	}
	f.locals[d] = ""
}

func (f *fnCtx) renameLocals() {
	var a []*cc.Declarator
	for k := range f.locals {
		a = append(a, k)
	}
	sort.Slice(a, func(i, j int) bool {
		x, y := a[i], a[j]
		if x.Name() < y.Name() {
			return true
		}

		if x.Name() > y.Name() {
			return false
		}

		return x.Visible() < y.Visible()
	})
	var r nameRegister
	for _, d := range a {
		f.locals[d] = r.put(f.c.declaratorTag(d) + d.Name())
	}
}

func (f *fnCtx) declareLocals() string {
	var a, use []string
	m := map[string][]string{}
	for k, v := range f.locals {
		if k.IsParam() {
			continue
		}

		if info := f.declInfos[k]; info != nil && info.pinned() {
			a = append(a, fmt.Sprintf("\nvar %s_ /* %s at bp%+d */ %s;", tag(preserve), k.Name(), info.bpOff, f.c.typ(k, k.Type())))
			continue
		}

		if k.IsTypename() {
			continue
		}

		if k.StorageDuration() != cc.Static && v != "" {
			ts := f.c.typ(k, k.Type())
			m[ts] = append(m[ts], v)
		}
	}
	for k, v := range f.autovars {
		m[k] = append(m[k], v...)
	}
	for k, v := range m {
		sort.Strings(v)
		a = append(a, fmt.Sprintf("\nvar %s %s;", strings.Join(v, ", "), k))
		use = append(use, v...)
	}
	var b strings.Builder
	sort.Strings(a)
	b.WriteString(strings.Join(a, ""))
	if len(use) != 0 {
		sort.Strings(use)
		l := strings.Repeat(tag(preserve)+"_, ", len(use))
		l = l[:len(l)-2]
		fmt.Fprintf(&b, "\n\t%s = %s;", l, strings.Join(use, ", "))
	}
	return b.String()
}

func (f *fnCtx) id() int { f.nextID++; return f.nextID }

func (c *ctx) externalDeclaration(w writer, n *cc.ExternalDeclaration) {
	switch n.Case {
	case cc.ExternalDeclarationFuncDef: // FunctionDefinition
		d := n.FunctionDefinition.Declarator
		// https://gcc.gnu.org/onlinedocs/gcc/Inline.html
		//
		// If you specify both inline and extern in the function definition, then the
		// definition is used only for inlining. In no case is the function compiled on
		// its own, not even if you refer to its address explicitly. Such an address
		// becomes an external reference, as if you had only declared the function, and
		// had not defined it.
		//
		// This combination of inline and extern has almost the effect of a macro. The
		// way to use it is to put a function definition in a header file with these
		// keywords, and put another copy of the definition (lacking inline and extern)
		// in a library file. The definition in the header file causes most calls to
		// the function to be inlined. If any uses of the function remain, they refer
		// to the single copy in the library.

		if d.Type().Attributes().AlwaysInline() ||
			d.Type().Attributes().GNUInline() ||
			d.IsInline() && c.isHeader(d) {
			c.inlineFuncs[d] = n.FunctionDefinition
			return
		}

		switch d.Linkage() {
		case cc.External:
			c.externsDefined[n.FunctionDefinition.Declarator.Name()] = n.FunctionDefinition
		}
		c.functionDefinition(w, n.FunctionDefinition, "")
	case cc.ExternalDeclarationDecl: // Declaration
		c.declaration(w, n.Declaration, true)
	case cc.ExternalDeclarationAsmStmt: // AsmStatement
		//TODO c.err(errorf("TODO %v", n.Case))
	case cc.ExternalDeclarationEmpty: // ';'
		//TODO c.err(errorf("TODO %v", n.Case))
	default:
		c.err(errorf("internal error %T %v", n, n.Case))
	}
}

func (c *ctx) isHeader(n cc.Node) bool {
	if n == nil {
		return false
	}

	return strings.HasSuffix(n.Position().Filename, ".h") ||
		c.task.goos == "windows" && strings.HasSuffix(n.Position().Filename, ".inl")
}

func (c *ctx) emitFunctionAliases() {
	m := map[string]string{}
	for alias, canonical := range c.Aliases {
		if x, ok := m[alias]; ok && x != canonical {
			c.err(errorf("conflicting aliases: %s -> %s and %s", alias, canonical, x))
			return
		}

		m[alias] = canonical
	}
	for alias, canonical := range c.WeakAliases {
		if x, ok := m[alias]; ok && x != canonical {
			c.err(errorf("conflicting aliases: %s -> %s and %s", alias, canonical, x))
			return
		}

		m[alias] = canonical
	}
	var a []string
	for alias := range m {
		a = append(a, alias)
	}
	slices.Sort(a)
	tx := tag(external)
	for _, alias := range a {
		canonical := m[alias][len(tx):]
		switch x := c.externsDefined[canonical].(type) {
		case *cc.FunctionDefinition:
			c.functionDefinition(c, x, alias[len(tx):])
		}
	}
}

func (c *ctx) functionDefinition(w writer, n *cc.FunctionDefinition, alias string) {
	if n.UsesVectors() {
		if !c.task.ignoreVectorFunctions {
			c.err(errorf("%v: function uses vector type(s)", n.Position()))
		}
		return
	}

	if c.task.header {
		return
	}

	c.functionDefinition0(w, sep(n), n, n.Declarator, n.CompoundStatement, false, alias)
}

func (c *ctx) functionDefinition0(w writer, sep string, pos cc.Node, d *cc.Declarator, cs *cc.CompoundStatement, local bool, alias string) {
	ft, ok := d.Type().(*cc.FunctionType)
	if !ok {
		c.err(errorf("%v: internal error %v", d.Position(), d.Type()))
		return
	}

	if d.Linkage() == cc.External && c.task.hidden.has(d.Name()) {
		return
	}

	if s := c.visbilityAttr(d.Type()); s != "" {
		c.Visibility[c.declaratorTag(d)+d.Name()] = s
	}
	c.fn = d
	defer func(d *cc.Declarator) { c.fn = d }(d)

	c.isValidType1(d, ft, true)
	f0, pass := c.f, c.pass
	var cft *cc.FunctionType
	if c.f != nil {
		cft = c.f.t
	}
	c.f = c.newFnCtx(d, ft, cs)
	defer func() {
		c.f = f0
		c.pass = pass
		if c.f != nil {
			c.f.t = cft
		}
	}()
	c.pass = 1
	for _, v := range ft.Parameters() {
		if v.Declarator != nil {
			c.f.registerLocal(v.Declarator)
		}
	}
	c.compoundStatement(discard{}, cs, true, "")
	c.f.renameLocals()
	var a []*cc.Declarator
	for d, n := range c.f.declInfos {
		if n.pinned() {
			a = append(a, d)
		}
	}
	sort.Slice(a, func(i, j int) bool {
		x := a[i].NameTok()
		y := a[j].NameTok()
		return x.Seq() < y.Seq()
	})
	for _, d := range a {
		info := c.f.declInfos[d]
		info.bpOff = roundup(c.f.tlsAllocs, bpAlign(d.Type()))
		c.f.tlsAllocs = info.bpOff + d.Type().Size()
	}
	c.pass = 2
	c.f.nextID = 0
	isMain := d.Linkage() == cc.External && d.Name() == "main"
	// trc("==== %v: sep `%s`", d.Position(), sep) //TODO-DBG
	s := c.cdoc(sep, d)
	switch {
	case local:
		// trc("s `%s`", s) //TODO-DBG
		w.w("%s%s%s := func%s", s, c.declaratorTag(d), d.Name(), c.signature(ft, true, false, true))
	default:
		nm := d.Name()
		if alias != "" {
			nm = alias
		}
		// trc("s `%s`", s) //TODO-DBG
		w.w("%sfunc %s%s%s ", s, c.declaratorTag(d), nm, c.signature(ft, true, isMain, true))
	}
	switch {
	case alias != "":
		w.w("{\n")
		if ft.Result() != nil && ft.Result().Kind() != cc.Void {
			w.w("return ")
		}
		w.w("%s%s(", c.declaratorTag(d), d.Name())
		w.w("%stls ", tag(ccgo))
		for _, v := range ft.Parameters() {
			switch info := c.f.declInfos.info(v.Declarator); {
			case info != nil && info.d != nil && info.pinned():
				w.w(", %s_%s ", tag(ccgo), v.Name())
			default:
				w.w(", %s", c.f.locals[v.Declarator])
			}
		}
		if ft.IsVariadic() {
			w.w(", %s%s", tag(ccgo), vaArgName)
		}
		w.w(")\n}")
	default:
		c.compoundStatement(w, cs, true, "")
	}
	w.w("\n\n")
}

func (c *ctx) cdoc(sep string, n cc.Node) (r string) {
	// trc("==== %v: %q", n.Position(), sep)

	// defer func() {
	// 	trc("%v: -> %q", n.Position(), r)
	// }()

	defer func() {
		if !strings.HasSuffix(r, "\n") {
			r += "\n"
		}
		if s := strings.TrimSpace(c.posComment(n)); s != "" {
			switch {
			case r == "":
				r = "\n" + s
			default:
				r = fmt.Sprintf("%s//\n%s\n", r, s)
			}
			if !strings.HasSuffix(r, "\n") {
				r += "\n"
			}
		}
	}()

	if strings.TrimSpace(sep) == "" || strings.HasSuffix(sep, "\n\n") {
		return sep
	}

	var b strings.Builder

	a := c.scanComments(sep, n)
	// trc("----")
	// for i, v := range a {
	// 	trc("%v: %q", i, v)
	// }
	split := -1
	for i, v := range a {
		if strings.HasPrefix(v, "/*") {
			continue
		}

		switch strings.Count(v, "\n") {
		case 0:
			// nop
		case 1:
			if strings.HasPrefix(v, "//") {
				break
			}

			if i > 0 && strings.HasSuffix(a[i-1], "\n") {
				split = i
			}
		default:
			split = i
		}
	}
	if split >= 0 {
		// trc("split at %v", split)
		b.WriteString(strings.Join(a[:split], ""))
		a = a[split+1:]
	}

	if len(a) == 0 {
		return b.String()
	}

	if !strings.HasPrefix(a[0], "//") && !strings.HasPrefix(a[0], "/*") && strings.Count(a[0], "\n") == 0 {
		b.WriteString(strings.Join(a, ""))
		return b.String()
	}

	// trc("----")
	// for i, v := range a {
	// 	trc("%v: %q", i, v)
	// }

	fmt.Fprintf(&b, "\n\n// C documentation\n//")
	for _, v := range a {
		switch {
		case strings.HasPrefix(v, "//"):
			fmt.Fprintf(&b, "\n//\t%s", strings.TrimRight(v, "\n"))
		case strings.HasPrefix(v, "/*"):
			for _, w := range strings.Split(v, "\n") {
				fmt.Fprintf(&b, "\n//\t%s", w)
			}
		}
	}
	return b.String()
}

func (c *ctx) scanComments(s string, n cc.Node) (r []string) {
	s0 := s
	for s != "" {
		switch s[0] {
		case '/':
			if len(s) == 1 {
				return append(r, s)
			}

			switch s[1] {
			case '/':
				x := strings.IndexByte(s, '\n')
				if x < 0 {
					return append(r, s)
				}

				r = append(r, s[:x+1])
				s = s[x+1:]
			case '*':
				x := strings.Index(s, "*/")
				if x < 0 {
					c.err(errorf("%v: scanComments: internal error", n.Position()))
					return []string{s0}
				}

				r = append(r, s[:x+2])
				s = s[x+2:]
			default:
				c.err(errorf("%v: scanComments: internal error", n.Position()))
				return []string{s0}
			}
		default:
			x := strings.IndexByte(s, '/')
			if x < 0 || x == len(s)-1 {
				return append(r, s)
			}

			r = append(r, s[:x])
			s = s[x:]
		}
	}
	return r
}

func (c *ctx) isVaList(t cc.Type) bool {
	d := t.Typedef()
	if d == nil {
		return false
	}

	nm := d.Name()
	return nm == "va_list" || nm == "__builtin_va_list"
}

func (c *ctx) typeIsOrHasFunctionPointer(m map[*cc.PointerType]struct{}, t cc.Type) (r bool) {
	switch x := t.(type) {
	case *cc.PointerType:
		if _, ok := m[x]; ok {
			return false
		}

		if m == nil {
			m = map[*cc.PointerType]struct{}{}
		}
		m[x] = struct{}{}
		return c.typeIsOrHasFunctionPointer(m, x.Elem())
	case *cc.FunctionType:
		return true
	case *cc.StructType:
		for i := 0; i < x.NumFields(); i++ {
			if c.typeIsOrHasFunctionPointer(m, x.FieldByIndex(i).Type()) {
				return true
			}
		}
	case *cc.UnionType:
		for i := 0; i < x.NumFields(); i++ {
			if c.typeIsOrHasFunctionPointer(m, x.FieldByIndex(i).Type()) {
				return true
			}
		}
	}
	return false
}

// https://github.com/golang/go/issues/44020

func (c *ctx) winapi(w writer, d *cc.Declarator, dl *cc.Declaration) {
	nm := d.Name()
	if c.task.hidden.has(nm) {
		return
	}

	if _, ok := c.winapiFuncs[nm]; ok {
		return
	}

	if strings.Contains(nm, "mingw_") || strings.Contains(nm, "_ms_") || strings.Contains(nm, "_stdio_common_") {
		return // not supported via a syscall
	}

	ft := d.Type().(*cc.FunctionType)
	if ft.IsVariadic() {
		return // not supported in a syscall on all windows targets
	}

	switch ft.Result().Kind() {
	case cc.Float, cc.Double, cc.LongDouble:
		return // not supported in a syscall on all windows targets
	}

	var off int64
	bpOffs := make([]int64, len(ft.Parameters()))
	var nms []string
	for i, v := range ft.Parameters() {
		if i == 0 && v.Type().Kind() == cc.Void {
			break
		}

		switch v.Type().Kind() {
		case cc.Float, cc.Double, cc.LongDouble:
			return // not supported in a syscall on all windows targets
		}

		if c.isVaList(v.Type()) {
			return // not supported in a syscall on all windows targets
		}

		nm := v.Name()
		if nm == "" {
			nm = fmt.Sprint(i)
		}
		nms = append(nms, nm)
		bpOffs[i] = -1
		switch v.Type().Kind() {
		case cc.Struct, cc.Union:
			switch c.task.goarch {
			case "amd64", "arm64":
				switch sz := v.Type().Size(); sz {
				case 1, 2, 4, 8:
					// ok
				default:
					bpOffs[i] = off
					off += roundup(sz, 16)
				}
			case "386":
				switch sz := v.Type().Size(); sz {
				case 1, 2, 4:
					// ok
				default:
					bpOffs[i] = off
					off += roundup(sz, 16) //TODO 16: verify 386 ABI requirements
				}
			default:
				c.err(fmt.Errorf("unsupported winapi target %s/%s", c.task.goos, c.task.goarch))
			}
		}
	}

	if c.winapiFuncs == nil {
		c.winapiFuncs = map[string]struct{}{}
	}
	c.winapiFuncs[nm] = struct{}{}
	w.w("\n\nvar %sproc%s = %[1]sdll.NewProc(%q)", tag(preserve), nm, c.task.tlsQualifier, nm)
	if c.task.winapiTest == "panic" {
		w.w("\nvar %s_ = %[1]sproc%s.%[1]sAddr()", tag(preserve), nm)
	}
	w.w("\n\n")
	if s := cc.NodeSource(dl); s != "" {
		w.w("\n// %s", s)
	}
	w.w("\nfunc %s%s%s {", c.declaratorTag(d), nm, c.winapiSignature(d, ft))
	for _, v := range ft.Parameters() {
		if c.typeIsOrHasFunctionPointer(nil, v.Type()) {
			w.w("\n%slibc.%[1]s%s__ccgo_SyscallFP(); panic(663)}", tag(preserve), tag(external))
			return
		}
	}

	w.w("\nif %s__ccgo_strace {", tag(preserve))
	if len(nms) != 0 {
		var args, args2 []string
		for _, v := range nms {
			args = append(args, fmt.Sprintf("%s=%%+v", v))
			args2 = append(args2, fmt.Sprintf("%s_%s", tag(preserve), v))
		}
		w.w("\ntrc(%q, %s)", strings.Join(args, " "), strings.Join(args2, ", "))
	} else {
		w.w("\ntrc(\"\")")
	}
	if ft.Result().Kind() != cc.Void {
		w.w("\ndefer func() { trc(`%s%s->%%+v`, r) }()", tag(external), nm)
	}
	w.w("\n}")
	switch {
	case ft.IsVariadic():
		panic(todo("internal error"))
		// if off != 0 {
		// 	c.err(fmt.Errorf("%v: internal error", origin(1)))
		// }
		// vnms = append(vnms, fmt.Sprintf("%sva_list", tag(preserve)))
		// w.w("\n%sr0, %[1]sr1, %[1]serr := %[1]ssyscall.%[1]sSyscallN(%[1]sproc%[2]s.%[1]sAddr(), %[1]s%[3]s__ccgo_variadicSyscallArgs(%s)...", tag(preserve), nm, tag(external), strings.Join(vnms, ", "))
	default:
		if off != 0 {
			w.w("\n%sbp := %[1]stls.%[1]sAlloc(%[3]v)", tag(preserve), c.task.tlsQualifier, off)
			w.w("\ndefer %stls.%[1]sFree(%[3]v)", tag(preserve), c.task.tlsQualifier, off)
			for i, v := range ft.Parameters() {
				if i == 0 && v.Type().Kind() == cc.Void {
					break
				}

				if bpOffs[i] < 0 {
					continue
				}

				pt := v.Type()
				switch c.task.goarch {
				case "amd64", "arm64", "386": //TODO
					w.w("\n*(*%s)(%s) = %s_%s", c.typ(nil, pt), unsafePointer(bpOff(bpOffs[i])), tag(preserve), nms[i])
				default:
					c.err(fmt.Errorf("%v: internal error", origin(1)))
				}
			}
		}
		w.w("\n%sr0, %[1]sr1, %[1]serr := %[1]ssyscall.%[1]sSyscallN(%[1]sproc%[2]s.%[1]sAddr()", tag(preserve), nm)
		for i, v := range ft.Parameters() {
			if i == 0 && v.Type().Kind() == cc.Void {
				break
			}

			nm := fmt.Sprintf("%s_%s", tag(preserve), nms[i])
			w.w(", ")
			switch v.Type().Kind() {
			case cc.Struct, cc.Union:
				switch c.task.goarch {
				case "amd64", "arm64":
					switch sz := v.Type().Size(); sz {
					case 1, 2, 4, 8:
						w.w("%suintptr(*(*int%v)(%s))", tag(preserve), sz*8, unsafePointer("&"+nm))
					default:
						w.w("%s", bpOff(bpOffs[i]))
					}
				case "386":
					switch sz := v.Type().Size(); sz {
					case 1, 2, 4:
						w.w("%suintptr(*(*int%v)(%s))", tag(preserve), sz*8, unsafePointer("&"+nm))
					default:
						w.w("%s", bpOff(bpOffs[i]))
					}
				default:
					c.err(fmt.Errorf("%v: internal error", origin(1)))
				}
			case cc.LongLong, cc.ULongLong:
				if c.task.goarch == "386" {
					w.w("%suintptr(%s), %[1]suintptr(%s>>32)", tag(preserve), nm)
					break
				}

				fallthrough
			default:
				rp := ""
				if v.Type().Kind() != cc.Ptr {
					w.w("%suintptr(", tag(preserve))
					rp = ")"
				}
				w.w("%s%s ", nm, rp)
			}
		}
	}
	w.w(")")
	w.w("\nif %serr != 0 {", tag(preserve))
	w.w("\nif %s__ccgo_strace {", tag(preserve))
	w.w("\ntrc(`r0=%%v r1=%%v err=%%v`, r0, r1, err)")
	w.w("}")
	switch {
	case c.task.winapiNoErrno:
		w.w("\n%stls.SetLastError(%[1]suint32(%[1]serr))", tag(preserve))
	default:
		w.w("\n%stls.%[1]ssetErrno(%[1]sint32(%[1]serr))", tag(preserve))
	}
	w.w("\n}")
	switch rt := ft.Result(); rt.Kind() {
	case cc.Void:
		// nop
	case cc.Struct, cc.Union:
		switch c.task.goarch {
		case "amd64", "arm64":
			switch rt.Size() {
			case 1, 2, 4, 8:
				w.w("\nreturn *(*%s)(%s)", c.typ2(nil, rt, true), unsafePointer(fmt.Sprintf("&%sr0", tag(preserve))))
			default:
				c.err(fmt.Errorf("%v: internal error: return struct/union of size %v from a syscall: %s", origin(1), rt.Size(), nm))
			}
		case "386":
			switch rt.Size() {
			case 1, 2, 4:
				w.w("\nreturn *(*%s)(%s)", c.typ2(nil, rt, true), unsafePointer(fmt.Sprintf("&%sr0", tag(preserve))))
			case 8:
				w.w("\n*(*uintptr)(unsafe.Pointer(&r)) = r0")
				w.w("\n*(*uintptr)(unsafe.Add(unsafe.Pointer(&r), 4)) = r1")
				w.w("\nreturn r")
			default:
				c.err(fmt.Errorf("%v: internal error: return struct/union of size %v from a syscall: %s", origin(1), rt.Size(), nm))
			}
		default:
			c.err(fmt.Errorf("%v: internal error: return struct/union of size %v from a syscall", origin(1), rt.Size()))
		}
	case cc.LongLong, cc.ULongLong:
		if c.task.goarch == "386" {
			w.w("\nreturn %s(%sr0)|%[1]s(%sr1>>32)", c.typ2(nil, rt, true), tag(preserve))
			break
		}

		fallthrough
	default:
		w.w("\nreturn %s(%sr0)", c.typ2(nil, rt, true), tag(preserve))
	}
	w.w("\n}\n")
}

func (c *ctx) winapiSignature(n cc.Node, f *cc.FunctionType) string {
	var b strings.Builder
	fmt.Fprintf(&b, "(%stls *%s%sTLS", tag(ccgo), c.task.tlsQualifier, tag(preserve))
	for i, v := range f.Parameters() {
		if i == 0 && v.Type().Kind() == cc.Void {
			break
		}

		nm := v.Name()
		if nm == "" {
			nm = fmt.Sprint(i)
		}
		b.WriteString(", ")
		fmt.Fprintf(&b, "%s_%s ", tag(preserve), nm)
		b.WriteString(c.typ2(v, v.Type().Decay(), true))
	}
	if f.IsVariadic() {
		fmt.Fprintf(&b, ", %sva_list %[1]suintptr", tag(preserve))
	}
	b.WriteByte(')')
	if f.Result().Kind() != cc.Void {
		fmt.Fprintf(&b, " (%s%s ", tag(ccgo), retvalName)
		b.WriteString(c.typ2(nil, f.Result(), true))
		b.WriteByte(')')
	}
	return b.String()
}

func (c *ctx) signature(f *cc.FunctionType, paramNames, isMain, useNames bool) string {
	var b strings.Builder
	switch {
	case paramNames:
		fmt.Fprintf(&b, "(%stls *%s%sTLS", tag(ccgo), c.task.tlsQualifier, tag(preserve))
	default:
		fmt.Fprintf(&b, "(*%s%sTLS", c.task.tlsQualifier, tag(preserve))
	}
	if f.MaxArgs() != 0 {
		for i, v := range f.Parameters() {
			if !c.isValidParamType(v, v.Type()) {
				return ""
			}

			b.WriteString(", ")
			if paramNames {
				switch nm := v.Name(); {
				case nm == "":
					fmt.Fprintf(&b, "%sp%d ", tag(ccgo), i)
				default:
					switch info := c.f.declInfos.info(v.Declarator); {
					case info.pinned():
						fmt.Fprintf(&b, "%s_%s ", tag(ccgo), nm)
					default:
						fmt.Fprintf(&b, "%s ", c.f.locals[v.Declarator])
					}
				}
			}
			b.WriteString(c.typ2(v, v.Type().Decay(), useNames))
		}
	}
	switch {
	case isMain && len(f.Parameters()) == 0 || isMain && len(f.Parameters()) == 1 && f.Parameters()[0].Type().Kind() == cc.Void:
		fmt.Fprintf(&b, ", %sargc %sint32, %[1]sargv %suintptr", tag(ccgo), tag(preserve))
	case isMain && len(f.Parameters()) == 1:
		fmt.Fprintf(&b, ", %sargv %suintptr", tag(ccgo), tag(preserve))
	case f.IsVariadic():
		switch {
		case paramNames:
			fmt.Fprintf(&b, ", %s%s %suintptr", tag(ccgo), vaArgName, tag(preserve))
		default:
			fmt.Fprintf(&b, ", %suintptr", tag(preserve))
		}
	}
	b.WriteByte(')')
	if f.Result().Kind() != cc.Void {
		if paramNames {
			fmt.Fprintf(&b, "(%s%s ", tag(ccgo), retvalName)
		}
		b.WriteString(c.typ2(nil, f.Result(), useNames))
		if paramNames {
			b.WriteByte(')')
		}
	}
	return b.String()
}

func (c *ctx) declaration(w writer, n *cc.Declaration, external bool) {
	switch n.Case {
	case cc.DeclarationDecl: // DeclarationSpecifiers InitDeclaratorList AttributeSpecifierList ';'
		switch {
		case n.InitDeclaratorList == nil:
			if !external {
				break
			}

			if n.DeclarationSpecifiers == nil {
				break
			}

			sep := sep(n)
			switch x := n.DeclarationSpecifiers.Type().(type) {
			case *cc.EnumType:
				c.defineEnumType(w, sep, n, x)
			case *cc.StructType:
				c.defineStructType(w, sep, n, x)
			case *cc.UnionType:
				c.defineUnionType(w, sep, n, x)
			}
		default:
			sep0 := sep(n)
			if c.f == nil {
				sep0 = c.cdoc(sep0, n)
			}
			for l := n.InitDeclaratorList; l != nil; l = l.InitDeclaratorList {
				c.initDeclarator(w, sep0+sep(l.InitDeclarator), l.InitDeclarator, external, n)
				sep0 = ""
			}
		}
	case cc.DeclarationAssert: // StaticAssertDeclaration
		// nop
	case cc.DeclarationAuto: // "__auto_type" Declarator '=' Initializer ';'
		sep0 := sep(n)
		var info *declInfo
		d := n.Declarator
		if c.f == nil {
			sep0 = c.cdoc(sep0, n)
			info = c.f.declInfos.info(d)
		}
		nm := d.Name()
		linkName := c.declaratorTag(d) + nm
		switch c.pass {
		case 1:
			if d.Linkage() == cc.None {
				c.f.registerLocal(d)
			}
		case 2:
			if nm := c.f.locals[d]; nm != "" {
				linkName = nm
			}
		}
		c.initDeclaratorInit(w, sep0, info, d, n.Initializer, linkName)
	default:
		c.err(errorf("internal error %T %v", n, n.Case))
	}
}

func (c *ctx) aliasAttrDecl(t cc.Type) (r *cc.Declarator) {
	if a := t.Attributes(); a != nil {
		return a.AliasDecl()
	}

	return nil
}

func (c *ctx) aliasAttr(t cc.Type) (r string) {
	if a := t.Attributes(); a != nil {
		return a.Alias()
	}

	return ""
}

func (c *ctx) weakAttr(t cc.Type) (r bool) {
	if a := t.Attributes(); a != nil {
		return a.Weak()
	}

	return false
}

func (c *ctx) visbilityAttr(t cc.Type) (r string) {
	if a := t.Attributes(); a != nil {
		return a.Visibility()
	}

	return ""
}

func (c *ctx) initDeclarator(w writer, sep string, n *cc.InitDeclarator, isExternal bool, dl *cc.Declaration) {
	d := n.Declarator
	if sc := d.LexicalScope(); sc.Parent == nil {
		hasInitializer := false
		for _, v := range sc.Nodes[d.Name()] {
			if x, ok := v.(*cc.Declarator); ok && x.HasInitializer() {
				hasInitializer = true
				break
			}
		}
		if hasInitializer && !d.HasInitializer() {
			return
		}
	}

	dt := d.Type()
	if s := c.visbilityAttr(dt); s != "" {
		c.Visibility[c.declaratorTag(d)+d.Name()] = s
	}

	if c.aliasAttr(dt) != "" {
		canonicalLinkName := tag(external) + c.aliasAttr(dt)
		if canonical := c.aliasAttrDecl(dt); canonical != nil {
			canonicalLinkName = c.declaratorTag(canonical) + canonical.Name()
		}
		switch {
		case c.weakAttr(dt):
			c.WeakAliases[c.declaratorTag(d)+d.Name()] = canonicalLinkName
		default:
			c.Aliases[c.declaratorTag(d)+d.Name()] = canonicalLinkName
		}
		return
	}

	if dt.Kind() == cc.Function && d.Linkage() == cc.External || d.IsExtern() && !dt.IsIncomplete() {
		c.externsDeclared[d.Name()] = d
	}

	if dt.Kind() == cc.Function && d.Linkage() == cc.External && len(c.task.winapi) != 0 {
		b := filepath.Base(d.Position().Filename)
		if _, ok := c.task.winapi[b]; ok {
			c.winapi(w, d, dl)
			return
		}
	}
	if dt.Kind() == cc.Function || d.IsExtern() && dt.IsIncomplete() {
		return
	}

	if c.f != nil {
		if x, ok := dt.(*cc.PointerType); ok {
			dt = x.Elem()
		}
		if x, ok := c.isVLA(dt); ok {
			v := c.f.newAutovar(n, c.ast.SizeT)
			if c.f.vlaSizes == nil {
				c.f.vlaSizes = map[*cc.Declarator]string{}
			}
			c.f.vlaSizes[d] = v
			k := ""
			if sz := x.Elem().Size(); sz != 1 {
				k = fmt.Sprintf("*%d", sz)
			}
			w.w("%s = (%s)%s;", c.f.vlaSizes[d], c.topExpr(w, x.SizeExpression(), c.ast.SizeT, exprDefault), k)
		}
	}

	if n.Asm != nil {
		w.w("//TODO %q // %v:\n", d.Name(), c.pos(n))
		if d.LexicalScope().Parent == nil {
			return
		}

		w.w("%spanic(0) // assembler statements not supported", tag(preserve))
	}

	nm := d.Name()
	linkName := c.declaratorTag(d) + nm
	var info *declInfo
	if c.f != nil {
		info = c.f.declInfos.info(d)
	}
	switch c.pass {
	case 1:
		if d.Linkage() == cc.None {
			c.f.registerLocal(d)
		}
	case 2:
		if nm := c.f.locals[d]; nm != "" {
			linkName = nm
		}
	}
	switch n.Case {
	case cc.InitDeclaratorDecl: // Declarator Asm
		switch {
		case d.IsTypename():
			if isExternal && c.typenames.add(nm) && !d.Type().IsIncomplete() && c.isValidType(d, d.Type(), false) {
				if c.task.header && (strings.HasPrefix(nm, "__builtin_") || strings.HasPrefix(nm, "__predefined_")) {
					break
				}

				w.w("\n\n%s%stype %s%s = %s;", sep, c.posComment(n), tag(typename), nm, c.typedef(d, d.Type()))
				c.defineType(w, sep, n, d.Type())
			}
			if !isExternal {
				return
			}
		default:
			if d.IsExtern() {
				return
			}

			c.defineType(w, sep, n, d.Type())
			switch {
			case d.IsStatic():
				switch c.pass {
				case 1:
					// nop
				case 2:
					if nm := c.f.locals[d]; nm != "" {
						w.w("%s%svar %s %s;", sep, c.posComment(n), nm, c.typ(d, d.Type()))
						break
					}

					fallthrough
				default:
					w.w("%s%svar %s %s;", sep, c.posComment(n), linkName, c.typ(d, d.Type()))
				}
			default:
				switch c.pass {
				case 0:
					w.w("%s%svar %s %s;", sep, c.posComment(n), linkName, c.typ(d, d.Type()))
				case 2:
					t, ok := c.isVLA(d.Type())
					if !ok {
						break
					}

					if t.SizeExpression() == nil {
						c.err(errorf("%v: internal error", d.Position()))
						break
					}

					linkName := c.f.locals[d]
					w.w("%s = %srealloc(%stls, %[1]s, %[4]s);", linkName, tag(external), tag(ccgo), c.f.vlaSizes[d])
				}
			}
		}
	case cc.InitDeclaratorInit: // Declarator Asm '=' Initializer
		c.initDeclaratorInit(w, sep, info, d, n.Initializer, linkName)
	default:
		c.err(errorf("internal error %T %v", n, n.Case))
	}
}

func (c *ctx) initDeclaratorInit(w writer, sep string, info *declInfo, d *cc.Declarator, initializer *cc.Initializer, linkName string) {
	t := d.Type()
	if t.Kind() == cc.Struct && t.(*cc.StructType).HasFlexibleArrayMember() {
		t = initializer.Type()
	}
	if d.StorageDuration() == cc.Static {
		if d.Linkage() == cc.None && (d.ReadCount() == 0 || c.f.inlineInfo != nil) && d.Name() == "__func__" {
			return
		}

		var initPatches []initPatch
		c.initPatch = func(off int64, b *buf) { initPatches = append(initPatches, initPatch{d, off, b}) }

		defer func() {
			c.initPatch = nil
			if len(initPatches) == 0 {
				return
			}

			var b buf
			b.w("{")
			b.w("\n\tp := %sunsafe.%sPointer(&%s%s)", tag(importQualifier), tag(preserve), c.declaratorTag(d), d.Name())
			for _, v := range initPatches {
				b.w("\n\t*(*uintptr)(%sunsafe.%sAdd(p, %v)) = %s", tag(importQualifier), tag(preserve), v.off, v.b)
			}
			b.w("\n};")
			switch d.Linkage() {
			case cc.External, cc.Internal:
				w.w("\n\nfunc init() %s", &b)
				w.w("\n\n")
			case cc.None:
				w.w("\n\nvar %s_ = func() %s", tag(preserve), &b)
			default:
				c.err(errorf("TODO %v", d.Linkage()))
			}
		}()
	}

	c.defineType(w, sep, d, t)
	switch {
	case d.Linkage() == cc.Internal:
		w.w("%s%svar %s = %s;", sep, c.posComment(d), linkName, c.initializerOuter(w, initializer, t))
	case d.IsStatic():
		switch c.pass {
		case 1:
			// nop
		case 2:
			if nm := c.f.locals[d]; nm != "" {
				switch {
				case cc.IsIntegerType(t) && initializer.AssignmentExpression != nil && c.isZero(initializer.AssignmentExpression.Value()):
					w.w("%s%svar %s %s;", sep, c.posComment(d), nm, c.typ(d, t))
				default:
					w.w("%s%svar %s = %s;", sep, c.posComment(d), nm, c.initializerOuter(w, initializer, t))
				}
				break
			}

			fallthrough
		default:
			w.w("%s%svar %s = %s;", sep, c.posComment(d), linkName, c.initializerOuter(w, initializer, t))
		}
	default:
		switch {
		case info != nil && info.pinned():
			switch {
			case t.Kind() == cc.Union && initializer.Type().Size() == t.Size():
				w.w("%s%s*(*%s)(%s) = %[3]s{};", sep, c.posComment(d), c.typ(d, t), unsafePointer(bpOff(info.bpOff)))
				u := c.unbracedInitilizer(initializer)
				w.w("%s%s*(*%s)(%s) = %s;", sep, c.posComment(d), c.typ(d, u.Type()), unsafePointer(bpOff(info.bpOff)), c.initializerOuter(w, u, u.Type()))
			default:
				if b := c.initCode(w,
					func(off int64) string {
						return unsafePointer(bpOff(info.bpOff + off))
					},
					initializer, t); b != nil {
					switch t.Kind() {
					case cc.Struct, cc.Union, cc.Array:
						w.w("%s%s*(*%s)(%s) = %[3]s{};", sep, c.posComment(d), c.typ(d, t), unsafePointer(bpOff(info.bpOff)))
					}
					w.w("%s%s%s;", sep, c.posComment(d), b)
					break
				}

				w.w("%s%s*(*%s)(%s) = %s;", sep, c.posComment(d), c.typ(d, t), unsafePointer(bpOff(info.bpOff)), c.initializerOuter(w, initializer, t))
			}
		default:
			switch {
			case d.LexicalScope().Parent == nil:
				switch {
				case cc.IsScalarType(t) && initializer.AssignmentExpression != nil && c.isZero(initializer.AssignmentExpression.Value()):
					w.w("%s%svar %s %s;", sep, c.posComment(d), linkName, c.typ(d, t))
				default:
					w.w("%s%svar %s = %s;", sep, c.posComment(d), linkName, c.initializerOuter(w, initializer, t))
				}
			default:
				if c.unbracedInitilizer(initializer).Case != cc.InitializerExpr {
					if b := c.initCode(w,
						func(off int64) string {
							return unsafe("Add", fmt.Sprintf("%s, %d", unsafePointer(fmt.Sprintf("&%s", linkName)), off))
						},
						initializer, t); b != nil {
						w.w("%s%s%s;", sep, c.posComment(d), b)
						break
					}
				}

				w.w("%s%s%s = %s;", sep, c.posComment(d), linkName, c.initializerOuter(w, initializer, t))
			}
		}
	}

}

func (c *ctx) isVLA(t cc.Type) (*cc.ArrayType, bool) {
	if x, ok := t.Undecay().(*cc.ArrayType); ok && x.IsVLA() {
		return x, true
	}

	return nil, false
}

func (c *ctx) initCode(w writer, ref func(int64) string, n *cc.Initializer, t cc.Type) *buf {
	var b buf
	switch t.Kind() {
	case cc.Struct:
		var st *cc.StructType
		switch x := t.(type) {
		case *cc.StructType:
			st = x
		default:
			return nil
		}

		ok := true
	loop:
		for i := 0; i < st.NumFields(); i++ {
			switch f := st.FieldByIndex(i); {
			case f.Type().Kind() == cc.Union:
				ok = false
				break loop
			}
		}
		if ok {
			return nil
		}

		a := c.initalizerFlatten(n, nil)
		for _, v := range a {
			e := v.AssignmentExpression
			b.w("*(*%s)(%s) = %s;", c.typ(e, v.Type()), ref(v.Offset()), c.topExpr(w, e, v.Type(), exprDefault))
		}
		return &b
	}
	return nil
}

func (c *ctx) unbracedInitilizer(n *cc.Initializer) *cc.Initializer {
	switch n.Case {
	case cc.InitializerExpr:
		return n
	case cc.InitializerInitList:
		switch {
		case n.InitializerList == nil:
			return n
		case n.InitializerList.Initializer.Case == cc.InitializerExpr && n.InitializerList.InitializerList == nil:
			return n.InitializerList.Initializer
		default:
			return n
		}
	default:
		c.err(errorf("internal error %T %v", n, n.Case))
		return n
	}
}
