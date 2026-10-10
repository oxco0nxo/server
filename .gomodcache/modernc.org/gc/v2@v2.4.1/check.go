// Copyright 2022 The Gc Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package gc // import "modernc.org/gc/v2"

import (
	"fmt"
	"go/constant"
	"go/token"
	"sort"
	"strconv"
)

var (
	universe = Scope{
		Nodes: map[string]Scoped{
			"bool":       {Node: PredefinedType(Bool)},
			"byte":       {Node: PredefinedType(Uint8)},
			"complex128": {Node: PredefinedType(Complex128)},
			"complex64":  {Node: PredefinedType(Complex64)},
			"false":      {Node: newPredefineConstant(UntypedBoolType, constant.MakeBool(false))},
			"float32":    {Node: PredefinedType(Float32)},
			"float64":    {Node: PredefinedType(Float64)},
			"int":        {Node: PredefinedType(Int)},
			"int16":      {Node: PredefinedType(Int16)},
			"int32":      {Node: PredefinedType(Int32)},
			"int64":      {Node: PredefinedType(Int64)},
			"int8":       {Node: PredefinedType(Int8)},
			"nil":        {Node: PredefinedType(UntypedNil)},
			"panic":      {Node: newPredefinedFunction([]Type{&InterfaceType{}}, nil)},
			"rune":       {Node: PredefinedType(Int32)},
			"string":     {Node: PredefinedType(String)},
			"true":       {Node: newPredefineConstant(UntypedBoolType, constant.MakeBool(true))},
			"uint":       {Node: PredefinedType(Uint)},
			"uint16":     {Node: PredefinedType(Uint16)},
			"uint32":     {Node: PredefinedType(Uint32)},
			"uint64":     {Node: PredefinedType(Uint64)},
			"uint8":      {Node: PredefinedType(Uint8)},
			"uintptr":    {Node: PredefinedType(Uintptr)},
		},
	}

	noScope = &Scope{}
)

func newPredefinedFunction(in, out []Type) (r *FunctionDecl) {
	r = &FunctionDecl{}
	r.typ = &FunctionType{Parameters: &TupleType{Types: in}, Result: &TupleType{Types: out}}
	r.guard = checked
	return r
}

func newPredefineConstant(t Type, v constant.Value) (r *Constant) {
	r = &Constant{}
	r.typ = t
	r.guard = checked
	r.val = v
	return r
}

// Scoped represents a node bound to a name and the offset where the visibility
// starts.  Declarations outside of a function/method reports their visibility
// starts at zero.
type Scoped struct {
	Node        Node
	VisibleFrom int
}

// Scope binds names to nodes.
type Scope struct {
	Nodes  map[string]Scoped
	Parent *Scope
}

// IsPackage reports whether s is a package scope.
func (s *Scope) IsPackage() bool { return s.Parent != nil && s.Parent.Parent == nil }

func (s *Scope) add(c *ctx, nm string, visibileFrom int, n Node) {
	if nm == "_" {
		*c.blanks = append(*c.blanks, n)
		return
	}

	if s.Nodes == nil {
		s.Nodes = map[string]Scoped{}
	}
	if x, ok := s.Nodes[nm]; ok {
		// trc("%v: %q (%v: %v: %v:)", n.Position(), nm, origin(4), origin(3), origin(2))
		c.err(n, "%s redeclared, previous declaration at %v:", nm, x.Node.Position())
		return
	}

	s.Nodes[nm] = Scoped{n, visibileFrom}
}

type packagesKey struct {
	pkg        *Package
	src        *SourceFile
	importPath string
}

type ctx0 struct {
	checker  PackageChecker
	errors   errList
	packages map[packagesKey]*Package
	pkg      *Package
	stack    []Node
	blanks   *[]Node
}

type ctx struct {
	*ctx0

	iota int64

	breakOk       bool
	continueOk    bool
	fallthroughOk bool
	pushNamed     bool
}

func newCtx(checker PackageChecker, pkg *Package) *ctx {
	return &ctx{
		ctx0: &ctx0{
			checker:  checker,
			packages: map[packagesKey]*Package{},
			pkg:      pkg,
		},
	}
}

func (c *ctx) inFor() *ctx {
	r := *c
	r.breakOk = true
	r.continueOk = true
	return &r
}

func (c *ctx) setFallthrough() *ctx {
	r := *c
	r.fallthroughOk = true
	return &r
}

func (c *ctx) setBreakOk() *ctx {
	r := *c
	r.breakOk = true
	return &r
}

func (c *ctx) setIota(n int64) *ctx {
	r := *c
	r.iota = n
	return &r
}

func (c *ctx) setPushNamed() *ctx {
	r := *c
	r.pushNamed = true
	return &r
}

func (c *ctx) err(n Node, s string, args ...interface{}) {
	var pos token.Position
	if n != nil {
		pos = n.Position()
	}
	c.errors.err(pos, s, args...)
}

func (c *ctx) packageLoader(pkg *Package, src *SourceFile, importPath Token) (r *Package) {
	pth, err := strconv.Unquote(importPath.Src())
	key := packagesKey{pkg, src, pth}
	if err != nil {
		c.packages[key] = nil
		c.err(importPath, errorf("%s", err))
		return nil
	}

	var ok bool
	if r, ok = c.packages[key]; ok {
		return r
	}

	p, err := c.checker.PackageLoader(pkg, src, pth)
	if err != nil {
		c.packages[key] = nil
		c.err(importPath, errorf("%s", err))
		return nil
	}

	c.packages[key] = p
	return p
}

func (c *ctx) symbolResolver(scope *Scope, pkg *Package, ident Token, passFileScope bool) (r Node) {
	fileScope := noScope
	if passFileScope {
		file := pkg.sourceFiles[ident.source]
		fileScope = file.Scope
	}
	var err error
	if r, err = c.checker.SymbolResolver(scope, fileScope, pkg, ident); err != nil {
		c.err(ident, errorf("%s", err))
		return nil
	}

	return r
}

func (c *ctx) symbol(expr Expression) Node {
	for {
		switch x := expr.(type) {
		case *QualifiedIdent:
			return x
		default:
			c.err(expr, errorf("TODO %T", x))
			return nil
		}
	}
}

func (c *ctx) checkExprOrType(p *Expression) Node {
	n := *p
	switch x := n.check(c).(type) {
	case Expression:
		*p = x
		return x
	default:
		return x
	}
}

func (c *ctx) checkType(n Node) Type {
	switch x := n.(type) {
	case checker:
		switch y := x.check(c).(type) {
		case Type:
			return y
		case *StructTypeNode:
			return y.Type()
		case *TypeNameNode:
			return y.Type()
		case *Ident:
			return y.Type()
		case *PointerTypeNode:
			return y.Type()
		case *ArrayTypeNode:
			return y.Type()
		case *QualifiedIdent:
			return y.Type()
		case *TypeDef:
			return y.Type()
		case *Variable:
			return y.Type()
		case *AliasDecl:
			return y.Type()
		case *FunctionDecl:
			return y.Type()
		case *ParenType:
			return y.Type()
		case *Signature:
			return y.Type()
		case *PredefinedType:
			return y
		case *FunctionTypeNode:
			return y.Type()
		case *Constant:
			return y.Type()
		case *InterfaceTypeNode:
			return y.Type()
		default:
			c.err(n, errorf("TODO %T", y))
			return Invalid
		}
	default:
		c.err(n, errorf("TODO %T", x))
		return Invalid
	}
}

func (c *ctx) checkExpr(p *Expression) (constant.Value, Type) {
	n := *p
	switch x := n.check(c).(type) {
	case Expression:
		*p = x
		return x.Value(), x.Type()
	default:
		c.err(n, errorf("TODO %T", x))
		return unknown, Invalid
	}
}

func (c *ctx) exprToType(n Expression) Node {
	switch x := n.(type) {
	case *UnaryExpr:
		switch x.Op.Ch {
		case '*':
			switch z := x.Expr.(type) {
			case *QualifiedIdent:
				return &PointerTypeNode{Star: x.Op, BaseType: z}
			case *Ident:
				return &PointerTypeNode{Star: x.Op, BaseType: z}
			default:
				c.err(n, errorf("TODO %T %s %T", x, x.Op.Ch.str(), z))
				return Invalid
			}
		default:
			c.err(n, errorf("TODO %T %s", x, x.Op.Ch.str()))
			return Invalid
		}
	default:
		c.err(n, errorf("TODO %T", x))
		return Invalid
	}
}

func (n *SourceFile) check(c *ctx) {
	pkgName := n.PackageClause.PackageName.Src()
	switch {
	case c.pkg.Name == "":
		c.pkg.Name = pkgName
	default:
		if pkgName != c.pkg.Name {
			c.err(n.PackageClause.PackageName, "expected package name %q, got %q", c.pkg.Name, pkgName)
		}
	}
	n.checkImports(c)
	if len(c.errors) != 0 {
		return
	}

	n.collectTLDs(c)
}

func (n *SourceFile) collectTLDs(c *ctx) {
	for _, tld := range n.TopLevelDecls {
		switch x := tld.(type) {
		case *ConstDecl:
			for _, cs := range x.ConstSpecs {
				for i, id := range cs.IdentifierList {
					c.pkg.Scope.add(c, id.Ident.Src(), 0, &Constant{node: cs, Expr: cs.exprList[i].Expr, Ident: id.Ident})
				}
			}
		case *FunctionDecl:
			switch nm := x.FunctionName.Src(); nm {
			case "init":
				c.pkg.Inits = append(c.pkg.Inits, x)
			default:
				c.pkg.Scope.add(c, x.FunctionName.Src(), 0, x)
			}
		case *MethodDecl:
			if len(x.Receiver.ParameterList) == 0 {
				c.err(x, "missing receiver")
				break
			}

			switch rx := x.Receiver.ParameterList[0].Type.(type) {
			case *PointerTypeNode:
				switch t := rx.BaseType.(type) {
				case *TypeNameNode:
					if t.Name.PackageName.IsValid() {
						c.err(t.Name, "cannot define new methods on non-local type %s", t.Name.Source(false))
						break
					}

					c.pkg.Scope.add(c, fmt.Sprintf("%s.%s", t.Name.Ident.Src(), x.MethodName.Src()), 0, x)
				default:
					c.err(x, errorf("TODO %T", t))
				}
			case *TypeNameNode:
				if rx.Name.PackageName.IsValid() {
					c.err(rx.Name, "cannot define new methods on non-local type %s", rx.Name.Source(false))
					break
				}

				c.pkg.Scope.add(c, fmt.Sprintf("%s.%s", rx.Name.Ident.Src(), x.MethodName.Src()), 0, x)
			case Token:
				c.pkg.Scope.add(c, fmt.Sprintf("%s.%s", rx.Src(), x.MethodName.Src()), 0, x)
			default:
				c.err(x, errorf("TODO %T", rx))
			}
		case *TypeDecl:
			for _, ts := range x.TypeSpecs {
				switch y := ts.(type) {
				case *AliasDecl:
					c.pkg.Scope.add(c, y.Ident.Src(), 0, ts)
				case *TypeDef:
					c.pkg.Scope.add(c, y.Ident.Src(), 0, ts)
				default:
					c.err(y, errorf("TODO %T", y))
				}
			}
		case *VarDecl:
			c.varDecl(x)
		default:
			c.err(tld, "unexpected top level declaration node type: %T", x)
		}
	}
}

func (c *ctx) varDecl(n *VarDecl) (r []*Variable) {
	s := n.LexicalScope()
	for _, vs := range n.VarSpecs {
		for i, id := range vs.IdentifierList {
			var expr Expression
			if i < len(vs.ExprList) {
				expr = vs.ExprList[i].Expr
			}
			// The scope of a constant or variable identifier declared inside a function
			// begins at the end of the ConstSpec or VarSpec (ShortVarDecl for short
			// variable declarations) and ends at the end of the innermost containing
			// block.
			visibleFrom := vs.Semicolon.Offset()
			if s.IsPackage() {
				visibleFrom = 0
			}
			v := &Variable{Expr: expr, Ident: id.Ident, TypeNode: vs.Type}
			r = append(r, v)
			s.add(c, id.Ident.Src(), visibleFrom, v)
		}
	}
	return r
}

func (n *SourceFile) checkImports(c *ctx) {
	for _, id := range n.ImportDecls {
		for _, is := range id.ImportSpecs {
			pkg := c.packageLoader(c.pkg, n, is.ImportPath)
			if pkg == nil {
				return
			}

			qualifier := pkg.Name
			if is.Qualifier.IsValid() {
				qualifier = is.Qualifier.Src()
			}
			n.Scope.add(c, qualifier, 0, pkg)
			if _, ok := c.pkg.Scope.Nodes[qualifier]; !ok {
				c.pkg.Scope.add(c, qualifier, 0, pkg)
			}
		}
	}
}

// Package collects source files.
type Package struct {
	ImportPath  string
	Inits       []*FunctionDecl
	Blanks      []Node
	Name        string
	Scope       *Scope
	SourceFiles []*SourceFile
	sourceFiles map[*source]*SourceFile
}

// NewPackage returns a newly created Package or an error, if any.
func NewPackage(importPath string, files []*SourceFile) (r *Package, err error) {
	sourceFiles := map[*source]*SourceFile{}
	var ps *Scope
	for _, file := range files {
		switch {
		case ps == nil:
			ps = file.packageScope
		default:
			if ps != file.packageScope {
				return nil, fmt.Errorf("NewPackage: source file were created using different configurations")
			}
		}
		sourceFiles[file.PackageClause.Package.source] = file
	}
	if ps != nil {
		ps.Parent = &universe
	}
	return &Package{ImportPath: importPath, SourceFiles: files, Scope: ps, sourceFiles: sourceFiles}, nil
}

// Position implements Node. Position return a zero value.
func (n *Package) Position() (r token.Position) {
	if len(n.SourceFiles) != 0 {
		r = n.SourceFiles[0].PackageClause.Position()
	}
	return r
}

// Source implements Node. Source returns a zero value.
func (n *Package) Source(full bool) []byte { return nil }

// Tokens returns the tokens n consist of. Tokens returns nil.
func (n *Package) Tokens() []Token { return nil }

// Check type checks n.
func (n *Package) Check(checker PackageChecker) error {
	c := newCtx(checker, n)
	c.blanks = &n.Blanks
	for _, file := range n.SourceFiles {
		file.check(c)
	}
	n.checkDeclarations(c)
	n.checkFunctionBodies(c)
	return c.errors.Err()
}

func (n *Package) checkFunctionBodies(c *ctx) {
	if !c.checker.CheckFunctions() {
		return
	}

	var tldNames []string
	for tldName := range n.Scope.Nodes {
		tldNames = append(tldNames, tldName)
	}
	sort.Strings(tldNames)
	for _, tldName := range tldNames {
		var blanks []Node
		c.blanks = &blanks
		switch x := n.Scope.Nodes[tldName].Node.(type) {
		case *FunctionDecl:
			x.checkBody(c)
		case *MethodDecl:
			c.err(x, errorf("TODO %T", x))
		}
		for _, v := range blanks {
			switch x := v.(type) {
			case checker:
				x.check(c)
			default:
				c.err(x, errorf("TODO %T", x))
			}
		}
	}
}

func (n *Package) checkDeclarations(c *ctx) {
	var tldNames []string
	for tldName := range n.Scope.Nodes {
		tldNames = append(tldNames, tldName)
	}
	sort.Strings(tldNames)
	for _, tldName := range tldNames {
		switch x := n.Scope.Nodes[tldName].Node.(type) {
		case checker:
			x.check(c)
		case *Package:
			// nop
		default:
			c.err(x, errorf("TODO %T", x))
		}
	}
	for _, v := range n.Blanks {
		switch x := v.(type) {
		case checker:
			x.check(c)
		default:
			c.err(x, errorf("TODO %T", x))
		}
	}
}

func (n *Arguments) check(c *ctx) Node {
	if !n.enter(c, n) {
		return n
	}

	defer n.exit()

	var resolvedTo Node
	var resolvedIn *Package
	x0 := c.checkExprOrType(&n.PrimaryExpr)
more:
	switch x := x0.(type) {
	case *Ident:
		resolvedTo = x.ResolvedTo()
	case *QualifiedIdent:
		resolvedIn = x.ResolvedIn()
		resolvedTo = x.ResolvedTo()
	case *ParenType:
		if len(n.ExprList) != 1 {
			c.err(n, errorf("TODO %T", x))
			return n
		}

		r := Expression(&Conversion{ConvertType: x, LParen: n.LParen, Expr: n.ExprList[0].Expr, Comma: n.ExprList[0].Comma, RParen: n.RParen})
		c.checkExpr(&r)
		return r
	case *Selector:
		resolvedTo = x
	case *ParenExpr:
		x0 = x.Expr
		goto more
	case *UnaryExpr:
		x.check(c)
		switch x.Op.Ch {
		case '*':
			if ft, ok := x.Type().(*FunctionType); ok {
				return n.checkFn(c, ft, nil, nil)
			}

			c.err(n, errorf("TODO %T, %s", x.Type(), x.Type()))
			return n
		default:
			c.err(n, errorf("TODO %q", x.Op.Src()))
			return n
		}
	default:
		c.err(n, errorf("TODO %T", x))
		return n
	}

	switch x := resolvedTo.(type) {
	case
		*AliasDecl,
		*TypeDef,
		PredefinedType:

		if len(n.ExprList) != 1 {
			c.err(n, errorf("TODO %T", x))
			return n
		}

		r := Expression(&Conversion{ConvertType: n.PrimaryExpr, LParen: n.LParen, Expr: n.ExprList[0].Expr, Comma: n.ExprList[0].Comma, RParen: n.RParen})
		c.checkExpr(&r)
		return r
	case *FunctionDecl:
		return n.checkFn(c, x.Type().(*FunctionType), resolvedIn, x)
	case *Variable:
		if ft, ok := x.Type().(*FunctionType); ok {
			return n.checkFn(c, ft, nil, nil)
		}

		c.err(n, errorf("TODO %T, %s", x.Type(), x.Type()))
		return n
	case *Selector:
		if ft, ok := x.Type().(*FunctionType); ok {
			return n.checkFn(c, ft, nil, nil)
		}

		c.err(n, errorf("TODO %T", x))
		return n
	default:
		c.err(n, errorf("TODO %T", x))
		return n
	}
}

func (n *Arguments) checkFn(c *ctx, ft *FunctionType, resolvedIn *Package, fd *FunctionDecl) Node {
	switch len(ft.Result.Types) {
	case 1:
		n.typ = ft.Result.Types[0]
	default:
		n.typ = ft.Result
	}
	var variadic Type
	if len(ft.Parameters.Types) != len(n.ExprList) {
		if !ft.IsVariadic || len(n.ExprList) < len(ft.Parameters.Types) {
			// trc("%v: %v %v %v '%s'", n.LParen.Position(), ft.IsVariadic, len(n.ExprList), len(ft.Parameters.Types), n.Source(false))
			c.err(n, errorf("TODO %T", n))
			return n
		}

		variadic = ft.Parameters.Types[len(ft.Parameters.Types)-1]
	}

	for i := range n.ExprList {
		c.checkExpr(&n.ExprList[i].Expr)
	}

	if resolvedIn != nil && fd != nil {
		switch resolvedIn.ImportPath {
		case "unsafe":
			switch fd.FunctionName.Src() {
			case "Alignof", "Offsetof", "Sizeof":
				return n
			case "Add":
				if !c.isAssignable(n.ExprList[0].Expr, n.ExprList[0].Expr.Type(), ft.Parameters.Types[0]) {
					c.err(n, errorf("TODO %T", n))
				}
				if !isAnyIntegerType(n.ExprList[1].Expr.Type()) {
					c.err(n, errorf("TODO %T", n))
				}
				return n
			}
		}
	}

	for i, exprItem := range n.ExprList {
		et := c.singleType(exprItem.Expr, exprItem.Expr.Type())
		pt := variadic
		if i < len(ft.Parameters.Types) {
			pt = ft.Parameters.Types[i]
		}
		if !c.isAssignable(exprItem, et, pt) {
			c.err(exprItem.Expr, errorf("TODO %v -> %v", et, pt))
			continue
		}

		c.convertValue(exprItem, exprItem.Expr.Value(), pt)
	}
	return n
}

func (c *ctx) isAssignable(n Node, expr, to Type) bool {
	if expr == Invalid || to == Invalid {
		return false
	}

	if expr == to ||
		c.isIdentical(n, expr, to) ||
		to.Kind() == Interface && len(to.(*InterfaceType).Elems) == 0 {
		return true
	}

	switch expr.Kind() {
	case UntypedInt, UntypedFloat, UntypedComplex:
		return isArithmeticType(to)
	case UntypedBool:
		return to.Kind() == Bool
	case UntypedString:
		return to.Kind() == String
	case UntypedNil:
		switch to.Kind() {
		case Pointer, Slice, Map, Function, Interface, Chan:
			return true
		default:
			return false
		}
	}

	switch {
	case expr.Kind() == to.Kind():
		switch expr.(type) {
		case PredefinedType:
			return true
		}
	}

	c.err(n, errorf("TODO %s -> %s", expr, to))
	return false
}

func (c *ctx) isIdentical(n Node, t, u Type) bool {
	if t.Kind() != u.Kind() {
		return false
	}

	if t == u {
		return true
	}

	switch x := t.(type) {
	case *ArrayType:
		switch y := u.(type) {
		case *ArrayType:
			return x.Len == y.Len && c.isIdentical(n, x.Elem, y.Elem)
		}
	case *StructType:
		switch y := u.(type) {
		case *StructType:
			if len(x.Fields) != len(y.Fields) {
				return false
			}

			for i, v := range x.Fields {
				w := y.Fields[i]
				if v.Name != w.Name || !c.isIdentical(n, v.Type(), w.Type()) {
					return false
				}
			}

			return true
		}
	case *FunctionType:
		switch y := u.(type) {
		case *FunctionType:
			in, out := x.Parameters.Types, x.Result.Types
			in2, out2 := y.Parameters.Types, y.Result.Types
			if len(in) != len(in2) || len(out) != len(out2) {
				return false
			}

			for i, v := range in {
				if !c.isIdentical(n, v, in2[i]) {
					return false
				}
			}

			for i, v := range out {
				if !c.isIdentical(n, v, out2[i]) {
					return false
				}
			}

			return true
		}
	case *PointerType:
		switch y := u.(type) {
		case *PointerType:
			return c.isIdentical(n, x.Elem, y.Elem)
		}
	}

	c.err(n, errorf("TODO %s -> %s", t, u))
	return false
}

func (n *BasicLit) check(c *ctx) Node {
	if !n.enter(c, n) {
		return n
	}

	defer n.exit()

	c.err(n, errorf("internal error: %T %T %T", n, n.Type(), n.Value()))
	return n
}

func (n *BinaryExpr) check(c *ctx) Node {
	if !n.enter(c, n) {
		return n
	}

	defer n.exit()

	x, a := c.checkExpr(&n.A)
	y, b := c.checkExpr(&n.B)
	switch n.Op.Ch {
	case SHL, SHR:
		// The right operand in a shift expression must have integer type or be an
		// untyped constant representable by a value of type uint.
		switch {
		case isIntegerType(b):
			// ok
		case y.Kind() != constant.Unknown:
			if isAnyArithmeticType(b) {
				c.convertValue(n.B, y, PredefinedType(Uint))
				break
			}

			trc("y %T %v, b %s", y, y.Kind(), b)
			c.err(n.Op, errorf("TODO %v", n.Op.Ch.str()))
			return n
		default:
			c.err(n.Op, errorf("TODO %v", n.Op.Ch.str()))
			return n
		}

		// If the left operand of a non-constant shift expression is an untyped
		// constant, it is first implicitly converted to the type it would assume if
		// the shift expression were replaced by its left operand alone.
		switch {
		case y.Kind() == constant.Unknown && x.Kind() != constant.Unknown:
			c.err(n.Op, errorf("TODO %v", n.Op.Ch.str()))
			return n
		default:
			n.typ = a
			n.val = constant.BinaryOp(x, xlat[n.Op.Ch], y)
		}
	case LOR, LAND:
		if !isAnyBoolType(a) || !isAnyBoolType(b) {
			c.err(n.Op, errorf("TODO %v %v", a, b))
			break
		}

		n.typ = UntypedBoolType
		if a.Kind() == Bool || b.Kind() == Bool {
			n.typ = PredefinedType(Bool)
		}
		if x.Kind() != constant.Unknown && y.Kind() != constant.Unknown {
			n.val = constant.BinaryOp(x, xlat[n.Op.Ch], y)
		}
	case EQ, NE:
		if !c.isComparable(n, a, b) {
			c.err(n.Op, errorf("TODO %v", n.Op.Ch.str()))
			break
		}

		n.typ = UntypedBoolType
		if x.Kind() != constant.Unknown && y.Kind() != constant.Unknown {
			n.val = constant.MakeBool(constant.Compare(x, xlat[n.Op.Ch], y))
		}
	case '<', LE, '>', GE:
		if !c.isOrdered(n, a, b) {
			c.err(n.Op, errorf("TODO %v", n.Op.Ch.str()))
			break
		}

		n.typ = UntypedBoolType
		if x.Kind() != constant.Unknown && y.Kind() != constant.Unknown {
			n.val = constant.MakeBool(constant.Compare(x, xlat[n.Op.Ch], y))
		}
	default:
		if !isAnyArithmeticType(a) || !isAnyArithmeticType(b) {
			c.err(n.Op, errorf("TODO %v %v", a, b))
			break
		}

		// For other binary operators, the operand types must be identical unless the
		// operation involves shifts or untyped constants.

		// Except for shift operations, if one operand is an untyped constant and the
		// other operand is not, the constant is implicitly converted to the type of
		// the other operand.
		switch {
		case x.Kind() == constant.Unknown && y.Kind() == constant.Unknown:
			if a.Kind() != b.Kind() {
				c.err(n.Op, errorf("TODO %v", n.Op.Ch.str()))
				break
			}

			n.typ = a
		case x.Kind() == constant.Unknown && y.Kind() != constant.Unknown:
			c.convertValue(n, y, a)
			n.typ = a
		case x.Kind() != constant.Unknown && y.Kind() == constant.Unknown:
			c.convertValue(n, x, b)
			n.typ = b
		default: // case x.Kind() != constant.Unknown && y.Kind() != constant.Unknown:
			n.val = constant.BinaryOp(x, xlat[n.Op.Ch], y)
			switch n.val.Kind() {
			case constant.Int:
				n.typ = UntypedIntType
			case constant.Float:
				n.typ = UntypedFloatType
			case constant.Complex:
				n.typ = UntypedComplexType
			default:
				c.err(n.Op, errorf("TODO %v", n.Op.Ch.str()))
			}
		}
	}
	return n
}

// - Boolean values are comparable. Two boolean values are equal if they are
// either both true or both false.
//
// - Integer values are comparable and ordered, in the usual way.
//
// - Floating-point values are comparable and ordered, as defined by the
// IEEE-754 standard.
//
// - Complex values are comparable. Two complex values u and v are equal if
// both real(u) == real(v) and imag(u) == imag(v).
//
// - String values are comparable and ordered, lexically byte-wise.
//
// - Pointer values are comparable. Two pointer values are equal if they point
// to the same variable or if both have value nil. Pointers to distinct
// zero-size variables may or may not be equal.
//
// - Channel values are comparable. Two channel values are equal if they were
// created by the same call to make or if both have value nil.
//
// - Interface values are comparable. Two interface values are equal if they
// have identical dynamic types and equal dynamic values or if both have value
// nil.
//
// - A value x of non-interface type X and a value t of interface type T are
// comparable when values of type X are comparable and X implements T. They are
// equal if t's dynamic type is identical to X and t's dynamic value is equal
// to x.
//
// - Struct values are comparable if all their fields are comparable. Two
// struct values are equal if their corresponding non-blank fields are equal.
//
// - Array values are comparable if values of the array element type are
// comparable. Two array values are equal if their corresponding elements are
// equal.

func (c *ctx) isComparable(n Node, t, u Type) bool {
	if t.Kind() == InvalidKind || u.Kind() == InvalidKind {
		return false
	}

	if isAnyArithmeticType(t) && isAnyArithmeticType(u) {
		return true
	}

	c.err(n, errorf("TODO %v %v", t, u))
	return false
}

func (c *ctx) isOrdered(n Node, t, u Type) bool {
	if t.Kind() == InvalidKind || u.Kind() == InvalidKind {
		return false
	}

	if isAnyComplexType(t) || isAnyComplexType(u) {
		return false
	}

	if isAnyArithmeticType(t) && isAnyArithmeticType(u) {
		return true
	}

	c.err(n, errorf("TODO %v %v", t, u))
	return false
}

func (n *CompositeLit) check(c *ctx) Node {
	if !n.enter(c, n) {
		return n
	}

	defer n.exit()

	n.typ = c.checkType(n.LiteralType)
	n.LiteralValue.check(c, n.Type())
	return n
}

func (n *LiteralValue) check(c *ctx, t Type) {
	switch x := t.(type) {
	case *ArrayType:
		n.checkArray(c, x)
	case *StructType:
		n.checkStruct(c, x)
	default:
		c.err(n, errorf("TODO %T", x))
	}
}

func (n *LiteralValue) checkStruct(c *ctx, strct *StructType) {
	var ix int
	for _, ke := range n.ElementList {
		if ix >= len(strct.Fields) {
			c.err(n, errorf("TODO %v/%v", ix, len(strct.Fields)))
			return
		}

		ke.checkStructElem(c, strct, strct.Fields[ix], &ix)
		ix++
	}
}

func (n *KeyedElement) checkStructElem(c *ctx, strct *StructType, f *Field, ix *int) {
	if !n.enter(c, n) {
		return
	}

	defer n.exit()

	n.typ = f.Type()
	switch x := n.Key.(type) {
	case nil:
		// ok
	case *Ident:
		f = strct.FieldByName(x.Token.Src())
		if f == nil {
			c.err(n.Key, errorf("TODO %T", x))
			return
		}

		x.typ = f.Type()
		n.typ = x.typ
		x.guard = checked
		*ix = f.Index()
	default:
		c.err(n.Key, errorf("TODO %T", x))
	}
	switch x := n.Element.(type) {
	case Expression:
		elemVal, elemType := c.checkExpr(&x)
		if elemType.Kind() != InvalidKind {
			n.Element = x
		}
		if !c.isAssignable(n.Element, elemType, f.Type()) {
			c.err(n.Element, errorf("TODO %T", x))
			break
		}

		if elemVal.Kind() != constant.Unknown {
			c.convertValue(n.Element, elemVal, f.Type())
		}
	default:
		c.err(n.Element, errorf("TODO %T", x))
	}
}

func (n *LiteralValue) checkArray(c *ctx, arr *ArrayType) {
	el := arr.Elem
	var ix int64
	for _, ke := range n.ElementList {
		ke.checkArrayElem(c, arr, el, &ix)
		ix++
	}
}

func (n *KeyedElement) checkArrayElem(c *ctx, arr *ArrayType, arrElem Type, ix *int64) {
	if !n.enter(c, n) {
		return
	}

	defer n.exit()

	n.typ = arrElem
	switch x := n.Key.(type) {
	case nil:
		// ok
	case Expression:
		keyVal, keyType := c.checkExpr(&x)
		if keyType.Kind() != InvalidKind {
			n.Key = x
		}
		switch keyVal.Kind() {
		case constant.Int:
			i64, ok := constant.Int64Val(keyVal)
			if !ok {
				c.err(n.Key, errorf("TODO"))
				break
			}

			if i64 >= arr.Len {
				c.err(n.Key, errorf("TODO"))
				return
			}

			*ix = i64
		default:
			c.err(n.Key, errorf("TODO %v", keyVal.Kind()))
		}
	default:
		c.err(n.Key, errorf("TODO %T", x))
	}
	switch x := n.Element.(type) {
	case Expression:
		elemVal, elemType := c.checkExpr(&x)
		if elemType.Kind() != InvalidKind {
			n.Element = x
		}
		if !c.isAssignable(n.Element, elemType, arrElem) {
			c.err(n.Element, errorf("TODO %T", x))
			break
		}

		if elemVal.Kind() != constant.Unknown {
			c.convertValue(n.Element, elemVal, arrElem)
		}
	case *LiteralValue:
		x.check(c, arrElem)
	default:
		c.err(n.Element, errorf("TODO %T", x))
	}
}

func (n *Constant) check(c *ctx) Node {
	if !n.enter(c, n) {
		return n
	}

	defer n.exit()

	n.val, n.typ = c.checkExpr(&n.Expr)
	return n
}

func (n *Conversion) check(c *ctx) Node {
	if !n.enter(c, n) {
		return n
	}

	defer n.exit()

	n.typ = c.checkType(n.ConvertType)
	c.checkExpr(&n.Expr)
	if n.Type().Kind() != InvalidKind && n.Expr.Type().Kind() != InvalidKind {
		n.val = c.convert(n.Expr, n.Type())
	}
	return n
}

func (n *ParenType) check(c *ctx) Node {
	if !n.enter(c, n) {
		return n
	}

	defer n.exit()

	n.typ = c.checkType(n.TypeNode)
	return n
}

func (n *FunctionLit) check(c *ctx) Node {
	if !n.enter(c, n) {
		return n
	}

	defer n.exit()

	c.err(n, errorf("TODO %T", n))
	return n

	//TODO c.err(n, errorf("TODO %T", n))
}

func (n *GenericOperand) check(c *ctx) Node {
	if !n.enter(c, n) {
		return n
	}

	defer n.exit()

	c.err(n, errorf("TODO %T", n))
	return n

	//TODO c.err(n, errorf("TODO %T", n))
}

func (n *Ident) check(c *ctx) Node {
	if !n.enter(c, n) {
		return n
	}

	defer n.exit()

	// trc("%v: %q lexical scope %p", n.Position(), n.Token.Src(), n.LexicalScope())
	switch n.resolvedTo = c.symbolResolver(n.lexicalScope, c.pkg, n.Token, true); x := n.ResolvedTo().(type) {
	case Type:
		n.typ = x
	case *TypeDef:
		n.typ = c.checkType(x)
	case *AliasDecl:
		n.typ = c.checkType(x)
	case *FunctionDecl:
		n.typ = c.checkType(x)
	case *Variable:
		n.typ = c.checkType(x)
	case *Constant:
		n.typ = c.checkType(x)
	default:
		c.err(n, errorf("TODO %T", x))
	}
	return n
}

func (n *Index) check(c *ctx) Node {
	if !n.enter(c, n) {
		return n
	}

	defer n.exit()

	_, pt := c.checkExpr(&n.PrimaryExpr)
	v, xt := c.checkExpr(&n.Expr)
	switch x := pt.(type) {
	case *PointerType:
		switch y := x.Elem.(type) {
		case *ArrayType:
			return n.checkArray(c, y, v, xt)
		case *TypeDef:
			switch z := y.Type().(type) {
			case *ArrayType:
				return n.checkArray(c, z, v, xt)
			default:
				c.err(n, errorf("TODO %T", z))
			}
		default:
			c.err(n, errorf("TODO %T", y))
		}
	case *ArrayType:
		return n.checkArray(c, x, v, xt)
	default:
		c.err(n, errorf("TODO %T", x))
	}
	return n
}

func (n *Index) checkArray(c *ctx, at *ArrayType, xv constant.Value, xt Type) Node {
	n.typ = at.Elem
	if xv.Kind() == constant.Unknown {
		if !isAnyIntegerType(xt) {
			c.err(n, errorf("TODO %T", n))
		}
		return n
	}

	if !isAnyArithmeticType(xt) {
		c.err(n, errorf("TODO %T", xv))
		return n
	}

	u64, ok := constant.Uint64Val(xv)
	if !ok {
		c.err(n, errorf("TODO %T", xv))
		return n
	}

	if u64 >= uint64(at.Len) {
		c.err(n, errorf("TODO %T", xv))
	}
	return n
}

func (n *MethodExpr) check(c *ctx) Node {
	if !n.enter(c, n) {
		return n
	}

	defer n.exit()

	c.err(n, errorf("TODO %T", n))
	return n

	//TODO c.err(n, errorf("TODO %T", n))
}

func (n *ParenExpr) check(c *ctx) Node {
	if !n.enter(c, n) {
		return n
	}

	defer n.exit()

	switch x := c.checkExprOrType(&n.Expr).(type) {
	case *PointerTypeNode:
		r := &ParenType{LParen: n.LParen, TypeNode: c.exprToType(n.Expr), RParen: n.RParen}
		c.checkType(r)
		return r
	case *Ident:
		switch y := x.ResolvedTo().(type) {
		case *Variable:
			n.typ = y.Type()
		default:
			c.err(n, errorf("TODO %T %T", x, y))
		}
	case *BinaryExpr:
		n.typ = x.Type()
	case *UnaryExpr:
		n.typ = x.Type()
	default:
		c.err(n, errorf("TODO %T", x))
	}
	return n
}

func (n *QualifiedIdent) check(c *ctx) Node {
	if !n.enter(c, n) {
		return n
	}

	defer n.exit()

	switch {
	case n.PackageName.IsValid():
		switch x := c.symbolResolver(n.lexicalScope, c.pkg, n.PackageName, true).(type) {
		case *Package:
			n.resolvedIn = x
			n.resolvedTo = c.symbolResolver(x.Scope, x, n.Ident, false)
		case *Variable:
			r := Expression(&Selector{PrimaryExpr: &Ident{lexicalScoper: n.lexicalScoper, Token: n.PackageName}, Dot: n.Dot, Ident: n.Ident})
			c.checkExpr(&r)
			return r
		default:
			c.err(n, errorf("TODO %T", x))
		}
	default:
		n.resolvedIn = c.pkg
		n.resolvedTo = c.symbolResolver(n.lexicalScope, c.pkg, n.Ident, true)
	}

	switch x := n.ResolvedTo().(type) {
	case PredefinedType:
		n.typ = x
	case *TypeDef:
		n.typ = c.checkType(x)
	case *FunctionDecl:
		n.typ = c.checkType(x)
	case *AliasDecl:
		n.typ = c.checkType(x)
	case *Variable:
		n.typ = c.checkType(x)
	default:
		c.err(n, errorf("TODO %T", x))
	}
	return n

}

func (n *Selector) check(c *ctx) Node {
	if !n.enter(c, n) {
		return n
	}

	defer n.exit()

	_, t := c.checkExpr(&n.PrimaryExpr)
	if x, ok := t.(*TypeDef); ok {
		t = x.Type()
	}
	if x, ok := t.(*PointerType); ok {
		t = x.Elem
	}
	if x, ok := t.(*TypeDef); ok {
		t = x.Type()
	}
	if x, ok := t.(*StructType); ok {
		if f := x.FieldByName(n.Ident.Src()); f != nil {
			n.typ = f.Type()
			return n
		}
	}

	c.err(n.Dot, errorf("TODO %T %v", t, t))
	return n
}

func (n *SliceExpr) check(c *ctx) Node {
	if !n.enter(c, n) {
		return n
	}

	defer n.exit()

	c.err(n, errorf("TODO %T", n))
	return n

	//TODO c.err(n, errorf("TODO %T", n))
}

func (n *TypeAssertion) check(c *ctx) Node {
	if !n.enter(c, n) {
		return n
	}

	defer n.exit()

	c.err(n, errorf("TODO %T", n))
	return n

	//TODO c.err(n, errorf("TODO %T", n))
}

func (n *TypeSwitchGuard) check(c *ctx) Node {
	if !n.enter(c, n) {
		return n
	}

	defer n.exit()

	c.err(n, errorf("TODO %T", n))
	return n

	//TODO c.err(n, errorf("TODO %T", n))
}

func (n *UnaryExpr) check(c *ctx) Node {
	if !n.enter(c, n) {
		return n
	}

	defer n.exit()

	var resolvedTo Node
	switch x := c.checkExprOrType(&n.Expr).(type) {
	case *QualifiedIdent:
		resolvedTo = x.ResolvedTo()
	case *Ident:
		resolvedTo = x.ResolvedTo()
	case
		PredefinedType,
		*Arguments,
		*BasicLit,
		*CompositeLit,
		*Conversion,
		*Index,
		*ParenExpr,
		*Selector,
		*UnaryExpr:

		// ok
	default:
		c.err(n, errorf("TODO %T", x))
		return n
	}

	switch x := resolvedTo.(type) {
	case nil:
		// ok
	case Type, *AliasDecl:
		switch n.Op.Ch {
		case '*':
			r := &PointerTypeNode{Star: n.Op, BaseType: n.Expr}
			c.checkType(r)
			return r
		default:
			c.err(n, errorf("TODO %T", x))
			return n
		}
	case *Variable:
		// ok
	default:
		c.err(n.Expr, errorf("TODO %T", x))
		return n
	}

	t := c.singleType(n.Expr, n.Expr.Type())
	n.typ = t
	n.val = n.Expr.Value()
	if !n.Op.IsValid() {
		return n
	}

	v := n.Value()
	switch n.Op.Ch {
	case '&':
		n.typ = newPointer(c.pkg, t)
	case '-', '+':
		if !isAnyArithmeticType(t) {
			c.err(n, errorf("TODO %s %v", n.Op.Ch.str(), t))
			break
		}

		if v.Kind() == constant.Unknown {
			break
		}

		w := constant.UnaryOp(xlat[n.Op.Ch], v, 0) //TODO prec
		if w.Kind() == constant.Unknown {
			c.err(n, errorf("TODO %s", n.Op.Ch.str()))
			break
		}

		n.val = w
	case '^':
		if !isAnyIntegerType(t) {
			c.err(n, errorf("TODO %T", n))
		}

		if v.Kind() == constant.Unknown {
			break
		}

		w := constant.UnaryOp(xlat[n.Op.Ch], v, 0)
		if w.Kind() == constant.Unknown {
			c.err(n, errorf("TODO %s", n.Op.Ch.str()))
			break
		}

		n.val = w
	case '*':
		switch x := n.Type().(type) {
		case *PointerType:
			n.typ = x.Elem
		default:
			c.err(n, errorf("TODO %T", x))
		}
	case '!':
		if !isAnyBoolType(t) {
			c.err(n, errorf("TODO %T", n))
		}

		if v.Kind() == constant.Unknown {
			break
		}

		w := constant.UnaryOp(xlat[n.Op.Ch], v, 0)
		if w.Kind() == constant.Unknown {
			c.err(n, errorf("TODO %s", n.Op.Ch.str()))
			break
		}

		n.val = w
	default:
		c.err(n, errorf("TODO %T %s", n, n.Op.Ch.str()))
	}
	return n
}

func (n *Variable) check(c *ctx) Node {
	if !n.enter(c, n) {
		return n
	}

	defer n.exit()

	if n.TypeNode != nil {
		n.typ = c.checkType(n.TypeNode)
	}
	if n.Expr != nil {
		c.checkExpr(&n.Expr)
	}
	switch {
	case n.TypeNode == nil && n.Expr == nil:
		c.err(n, errorf("TODO %T", n))
	case n.TypeNode == nil && n.Expr != nil:
		n.typ = c.defaultType(n.Expr.Type())
	case n.TypeNode != nil && n.Expr == nil:
		// nop
	default: //case n.Type != nil && n.Expr != nil:
		c.err(n, errorf("TODO %T", n))
	}
	return n
}

func (n *Signature) check(c *ctx) Node {
	if !n.enter(c, n) {
		return n
	}

	defer n.exit()

	in := n.Parameters.ParameterList
	r := &FunctionType{Parameters: tuple(c, in), Result: &TupleType{}}
	if len(in) != 0 {
		r.IsVariadic = in[len(in)-1].Ellipsis.IsValid()
	}
	switch x := n.Result.(type) {
	case *TypeNameNode:
		r.Result.Types = append(r.Result.Types, c.checkType(x))
	case nil:
		// ok
	case *Parameters:
		r.Result = tuple(c, x.ParameterList)
	default:
		c.err(x, errorf("TODO %T", x))
	}
	n.typ = r
	return n
}

func tuple(c *ctx, a []*ParameterDecl) (r *TupleType) {
	r = &TupleType{}
	for _, v := range a {
		r.Types = append(r.Types, c.checkType(v.Type))
	}
	return r
}

func (n *FunctionDecl) check(c *ctx) Node {
	if !n.enter(c, n) {
		return n
	}

	defer n.exit()

	if ft, ok := c.checkType(n.Signature).(*FunctionType); ok {
		ft.node = n
		n.typ = ft
	}
	return n
}

func (n *FunctionTypeNode) check(c *ctx) Node {
	if !n.enter(c, n) {
		return n
	}

	defer n.exit()

	if ft, ok := c.checkType(n.Signature).(*FunctionType); ok {
		ft.node = n
		n.typ = ft
	}
	return n
}

func (n *FunctionDecl) checkBody(c *ctx) {
	body := n.FunctionBody
	if body == nil {
		return
	}

	s := body.Scope
	// trc("%v: function %q body scope %p(%v), parent %p(%v)", n.Position(), n.FunctionName.Src(), s, s.IsPackage(), s.Parent, s.Parent != nil && s.Parent.IsPackage())
	vis := body.LBrace.Offset()
	ft, ok := n.Type().(*FunctionType)
	if !ok {
		return
	}

	types := ft.Parameters.Types
	for _, v := range n.Signature.Parameters.ParameterList {
		for _, w := range v.IdentifierList {
			id := w.Ident
			v := &Variable{Ident: id, isParamater: true}
			v.typ = types[0]
			types = types[1:]
			v.guard = checked
			s.add(c, id.Src(), vis, v)
		}
	}
	types = ft.Result.Types
	switch x := n.Signature.Result.(type) {
	case *Parameters:
		for _, v := range x.ParameterList {
			for _, w := range v.IdentifierList {
				id := w.Ident
				v := &Variable{Ident: id, isParamater: true}
				v.typ = types[0]
				types = types[1:]
				v.guard = checked
				s.add(c, id.Src(), vis, v)
			}
		}
	case
		*TypeNameNode,
		nil:

		// ok
	default:
		c.err(n, errorf("TODO %T", x))
	}
	body.check(c)
}

func (n *MethodDecl) check(c *ctx) Node {
	if !n.enter(c, n) {
		return n
	}

	defer n.exit()

	c.err(n, errorf("TODO %T", n))
	return n

	//TODO c.err(n, errorf("TODO %T", n))
}

func (n *Block) check(c *ctx) {
	for _, v := range n.StatementList {
		c.checkStatement(v)
	}
}

func (c *ctx) checkStatement(n Node) {
	if n == nil {
		return
	}

	switch x := n.(type) {
	case *ExpressionStmt:
		x.check(c)
	case *ReturnStmt:
		x.check(c)
	case *IfStmt:
		x.check(c)
	case *VarDecl:
		for _, v := range c.varDecl(x) {
			v.check(c)
		}
	case *Assignment:
		x.check(c)
	case *ShortVarDecl:
		x.check(c)
	case *ForStmt:
		x.check(c)
	case *IncDecStmt:
		x.check(c)
	case *DeferStmt:
		x.check(c)
	case *BreakStmt:
		x.check(c)
	case *ContinueStmt:
		x.check(c)
	case *Block:
		x.check(c)
	case *ExpressionSwitchStmt:
		x.check(c)
	case *FallthroughStmt:
		x.check(c)
	case *GotoStmt:
		//TODO x.check(c)
	case *LabeledStmt:
		//TODO
		c.checkStatement(x.Statement)
	case *TypeDecl:
		for _, ts := range x.TypeSpecs {
			switch y := ts.(type) {
			case *AliasDecl:
				y.LexicalScope().add(c, y.Ident.Src(), y.Ident.Offset(), ts)
				c.checkType(y)
			case *TypeDef:
				y.LexicalScope().add(c, y.Ident.Src(), y.Ident.Offset(), ts)
				c.checkType(y)
			default:
				c.err(y, errorf("TODO %T", y))
			}
		}
	case *EmptyStmt:
		// nop
	default:
		c.err(n, errorf("TODO %T", x))
	}
}

func (n *ExpressionSwitchStmt) check(c *ctx) {
	c.checkStatement(n.SimpleStmt)
	t := Type(PredefinedType(Bool))
	if n.Expr != nil {
		_, t = c.checkExpr(&n.Expr)
	}
	c = c.setBreakOk()
	for _, v := range n.ExprCaseClauses {
		v.check(c, t)
	}
}

func (n *ExprCaseClause) check(c *ctx, t Type) {
	n.ExprSwitchCase.check(c, t)
	c = c.setFallthrough()
	for _, v := range n.StatementList {
		c.checkStatement(v)
	}
}

func (n *ExprSwitchCase) check(c *ctx, t Type) {
	switch n.CaseOrDefault.Ch {
	case CASE:
		for i := range n.ExprList {
			_, et := c.checkExpr(&n.ExprList[i].Expr)
			if !c.isAssignable(n.ExprList[i].Expr, et, t) {
				c.err(n, errorf("TODO %T", n))
			}
		}
	case DEFAULT:
		// nop
	default:
		c.err(n, errorf("TODO %T", n))
	}
}

func (n *FallthroughStmt) check(c *ctx) {
	if !c.fallthroughOk {
		c.err(n, errorf("TODO %T", n))
		return
	}
}

func (n *ContinueStmt) check(c *ctx) {
	if !c.continueOk {
		c.err(n, errorf("TODO %T", n))
		return
	}

	if n.Label.IsValid() {
		c.err(n, errorf("TODO %T", n))
	}
}

func (n *BreakStmt) check(c *ctx) {
	if !c.breakOk {
		c.err(n, errorf("TODO %T", n))
		return
	}

	if n.Label.IsValid() {
		c.err(n, errorf("TODO %T", n))
	}
}

func (n *DeferStmt) check(c *ctx) {
	c.checkExpr(&n.Expr)
	// The expression must be a function or method call; it cannot be
	// parenthesized.
	switch x := n.Expr.(type) {
	case *Arguments:
		// ok
	default:
		c.err(n, errorf("TODO %T", x))
	}
}

func (n *IncDecStmt) check(c *ctx) {
	if _, t := c.checkExpr(&n.Expr); !isAnyArithmeticType(t) {
		c.err(n, errorf("TODO %T", n))
	}
}

func (n *ForStmt) check(c *ctx) {
	switch {
	case n.ForClause != nil:
		fc := n.ForClause
		c.checkStatement(fc.InitStmt)
		if fc.Condition != nil {
			if _, t := c.checkExpr(&fc.Condition); !isAnyBoolType(t) {
				c.err(n, errorf("TODO %T", n))
			}
		}
		c.checkStatement(fc.PostStmt)
	case n.RangeClause != nil:
		c.err(n.RangeClause, errorf("TODO %T", n))
	}
	n.Block.check(c.inFor())
}

func (n *ShortVarDecl) check(c *ctx) {
	for i := range n.ExprList {
		c.checkExpr(&n.ExprList[i].Expr)
	}
	if g, e := len(n.IdentifierList), len(n.ExprList); g != e {
		c.err(n, errorf("TODO %T", n))
		return
	}

	visibleFrom := n.Semicolon.Offset()
	s := n.LexicalScope()
	for i, v := range n.IdentifierList {
		id := v.Ident
		expr := n.ExprList[i].Expr
		switch et := expr.Type(); et.Kind() {
		case InvalidKind:
		default:
			v := &Variable{Expr: expr, Ident: id, typer: newTyper(c.defaultType(et))}
			s.add(c, id.Src(), visibleFrom, v)
		}
	}
}

func (n *Assignment) check(c *ctx) {
	var skip []bool
	for i, v := range n.LExprList {
		if x, ok := v.Expr.(*Ident); ok && x.Token.Src() == "_" {
			x.guard = checked
			skip = append(skip, true)
			continue
		}

		c.checkExpr(&n.LExprList[i].Expr)
		skip = append(skip, false)
	}
	for i := range n.RExprList {
		c.checkExpr(&n.RExprList[i].Expr)
	}
	if g, e := len(n.LExprList), len(n.RExprList); g != e {
		c.err(n, errorf("TODO %T", n))
		return
	}

	for i, v := range n.LExprList {
		if skip[i] {
			continue
		}

		expr := n.RExprList[i].Expr
		if !c.isAssignable(expr, expr.Type(), v.Expr.Type()) {
			c.err(expr, errorf("TODO %v -> %v", expr.Type(), v.Expr.Type()))
		}
	}
}

func (n *IfStmt) check(c *ctx) {
	c.checkStatement(n.SimpleStmt)
	if _, t := c.checkExpr(&n.Expr); !isAnyBoolType(t) {
		c.err(n, errorf("TODO %T", n))
	}
	n.Block.check(c)
	switch x := n.ElsePart.(type) {
	case nil:
		// nop
	case *IfStmt:
		x.check(c)
	case *Block:
		x.check(c)
	default:
		c.err(n, errorf("TODO %T", x))
	}
}

func (n *ReturnStmt) check(c *ctx) {
	if len(n.ExprList) == 0 {
		return
	}

	for i := range n.ExprList {
		c.checkExpr(&n.ExprList[i].Expr)
	}
	var ft0 Type
	switch x := n.container.(type) {
	case *FunctionDecl:
		ft0 = x.Signature.Type()
	case *MethodDecl:
		ft0 = x.Signature.Type()
	case *FunctionLit:
		ft0 = x.Signature.Type()
	}
	ft, ok := ft0.(*FunctionType)
	if !ok {
		return
	}

	if g, e := len(n.ExprList), len(ft.Result.Types); g != e {
		c.err(n.ExprList[0].Expr, "expected %d expression, got %d", e, g)
		return
	}

	for i, v := range n.ExprList {
		et := v.Expr.Type()
		if et == Invalid {
			continue
		}

		if g, e := et, ft.Result.Types[i]; !c.isAssignable(v.Expr, g, e) {
			c.err(n, errorf("TODO %T", n))
		}
	}
}

func (n *ExpressionStmt) check(c *ctx) { n.Expr.check(c) }
