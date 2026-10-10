// Copyright 2021 The Gc Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package gc // import "modernc.org/gc/v2"

import (
	"bytes"
	"go/token"
)

var (
	_ = []Node{
		(*AliasDecl)(nil),
		(*AliasType)(nil),
		(*Arguments)(nil),
		(*ArrayType)(nil),
		(*ArrayTypeNode)(nil),
		(*Assignment)(nil),
		(*BasicLit)(nil),
		(*BinaryExpr)(nil),
		(*Block)(nil),
		(*BreakStmt)(nil),
		(*ChannelType)(nil),
		(*ChannelTypeNode)(nil),
		(*CommCase)(nil),
		(*CommClause)(nil),
		(*CompositeLit)(nil),
		(*ConstDecl)(nil),
		(*ConstSpec)(nil),
		(*Constant)(nil),
		(*ContinueStmt)(nil),
		(*Conversion)(nil),
		(*DeferStmt)(nil),
		(*EmbeddedField)(nil),
		(*EmptyStmt)(nil),
		(*ExprCaseClause)(nil),
		(*ExprSwitchCase)(nil),
		(*ExprListItem)(nil),
		(*ExpressionStmt)(nil),
		(*ExpressionSwitchStmt)(nil),
		(*FallthroughStmt)(nil),
		(*FieldDecl)(nil),
		(*ForClause)(nil),
		(*ForStmt)(nil),
		(*FunctionDecl)(nil),
		(*FunctionLit)(nil),
		(*FunctionType)(nil),
		(*FunctionTypeNode)(nil),
		(*GenericOperand)(nil),
		(*GoStmt)(nil),
		(*GotoStmt)(nil),
		(*Ident)(nil),
		(*IdentListItem)(nil),
		(*IfStmt)(nil),
		(*ImportDecl)(nil),
		(*ImportSpec)(nil),
		(*IncDecStmt)(nil),
		(*Index)(nil),
		(*InterfaceType)(nil),
		(*InterfaceTypeNode)(nil),
		(*KeyedElement)(nil),
		(*LabeledStmt)(nil),
		(*LiteralValue)(nil),
		(*MapType)(nil),
		(*MapTypeNode)(nil),
		(*MethodDecl)(nil),
		(*MethodElem)(nil),
		(*MethodExpr)(nil),
		(*Package)(nil),
		(*PackageClause)(nil),
		(*ParameterDecl)(nil),
		(*Parameters)(nil),
		(*ParenExpr)(nil),
		(*ParenType)(nil),
		(*PointerType)(nil),
		(*PointerTypeNode)(nil),
		(*QualifiedIdent)(nil),
		(*RangeClause)(nil),
		(*ReturnStmt)(nil),
		(*SelectStmt)(nil),
		(*Selector)(nil),
		(*SendStmt)(nil),
		(*ShortVarDecl)(nil),
		(*Signature)(nil),
		(*SliceExpr)(nil),
		(*SliceType)(nil),
		(*SliceTypeNode)(nil),
		(*SourceFile)(nil),
		(*StructType)(nil),
		(*StructTypeNode)(nil),
		(*TypeArgs)(nil),
		(*TypeAssertion)(nil),
		(*TypeAssertion)(nil),
		(*TypeCaseClause)(nil),
		(*TypeDecl)(nil),
		(*TypeDef)(nil),
		(*TypeElem)(nil),
		(*TypeListItem)(nil),
		(*TypeNameNode)(nil),
		(*TypeParamDecl)(nil),
		(*TypeParameters)(nil),
		(*TypeSwitchGuard)(nil),
		(*TypeSwitchStmt)(nil),
		(*TypeTerm)(nil),
		(*UnaryExpr)(nil),
		(*VarDecl)(nil),
		(*VarSpec)(nil),
		(*Variable)(nil),
	}

	_ = []typeNode{
		(*ArrayTypeNode)(nil),
		(*ChannelTypeNode)(nil),
		(*FunctionTypeNode)(nil),
		(*InterfaceTypeNode)(nil),
		(*MapTypeNode)(nil),
		(*ParenType)(nil),
		(*PointerTypeNode)(nil),
		(*SliceTypeNode)(nil),
		(*StructTypeNode)(nil),
		(*TypeNameNode)(nil),
	}
)

type lexicalScoper struct {
	lexicalScope *Scope
}

func newLexicalScoper(s *Scope) lexicalScoper { return lexicalScoper{s} }

// LexicalScope returns the lexical scope n appears in.
func (n lexicalScoper) LexicalScope() *Scope { return n.lexicalScope }

type typeNode interface {
	Node
	isTypeNode()
}

type typeNoder struct{}

func (typeNoder) isTypeNode() {}

type simpleStmt interface {
	Node
	isSimpleStmt()
	semi(p *parser)
}

type simpleStmter struct{}

func (simpleStmter) isSimpleStmt() {}

// PackageClause describes the package clause.
//
//	PackageClause = "package" PackageName .
type PackageClause struct {
	Package     Token
	PackageName Token
	Semicolon   Token
}

// Position implements Node.
func (n *PackageClause) Position() (r token.Position) {
	return n.Package.Position()
}

// Source implements Node.
func (n *PackageClause) Source(full bool) []byte { return nodeSource(&bytes.Buffer{}, n, full).Bytes() }

// Tokens returns the tokens n consist of.
func (n *PackageClause) Tokens() []Token { return nodeTokens(n) }

// ImportSpec describes an import specification.
//
//	ImportSpec = [ "." | PackageName ] ImportPath .
type ImportSpec struct {
	Qualifier  Token
	ImportPath Token
	Semicolon  Token
}

// Position implements Node.
func (n *ImportSpec) Position() (r token.Position) {
	if n.Qualifier.IsValid() {
		return n.Qualifier.Position()
	}

	return n.ImportPath.Position()
}

// Source implements Node.
func (n *ImportSpec) Source(full bool) []byte { return nodeSource(&bytes.Buffer{}, n, full).Bytes() }

// Tokens returns the tokens n consist of.
func (n *ImportSpec) Tokens() []Token { return nodeTokens(n) }

// ImportDecl describes an import declaration.
//
//	ImportDecl = "import" ( ImportSpec | "(" { ImportSpec ";" } ")" ) .
type ImportDecl struct {
	Import      Token
	LParen      Token
	ImportSpecs []*ImportSpec
	RParen      Token
	Semicolon   Token
}

// Position implements Node.
func (n *ImportDecl) Position() (r token.Position) {
	return n.Import.Position()
}

// Source implements Node.
func (n *ImportDecl) Source(full bool) []byte { return nodeSource(&bytes.Buffer{}, n, full).Bytes() }

// Tokens returns the tokens n consist of.
func (n *ImportDecl) Tokens() []Token { return nodeTokens(n) }

// SourceFile describes a source file.
//
//	SourceFile = PackageClause ";" { ImportDecl ";" } { TopLevelDecl ";" } .
type SourceFile struct {
	PackageClause *PackageClause
	ImportDecls   []*ImportDecl
	TopLevelDecls []Node
	EOF           Token
	Scope         *Scope
	packageScope  *Scope
}

// Position implements Node.
func (n *SourceFile) Position() (r token.Position) {
	return n.PackageClause.Position()
}

// Source implements Node.
func (n *SourceFile) Source(full bool) []byte { return nodeSource(&bytes.Buffer{}, n, full).Bytes() }

// Tokens returns the tokens n consist of.
func (n *SourceFile) Tokens() []Token { return nodeTokens(n) }

// FunctionDecl describes a function declaration.
//
//	FunctionDecl = "func" FunctionName [ TypeParameters ] Signature [ FunctionBody ] .
type FunctionDecl struct {
	typer
	Func           Token
	FunctionName   Token
	TypeParameters *TypeParameters
	Signature      *Signature
	FunctionBody   *Block
	Semicolon      Token
}

// Position implements Node.
func (n *FunctionDecl) Position() (r token.Position) {
	return n.Func.Position()
}

// Source implements Node.
func (n *FunctionDecl) Source(full bool) []byte { return nodeSource(&bytes.Buffer{}, n, full).Bytes() }

// Tokens returns the tokens n consist of.
func (n *FunctionDecl) Tokens() []Token { return nodeTokens(n) }

// Signature describes a function signature.
//
//	Signature = Parameters [ Result ] .
type Signature struct {
	typer
	Parameters *Parameters
	Result     Node
}

// Position implements Node.
func (n *Signature) Position() (r token.Position) {
	return n.Parameters.Position()
}

// Source implements Node.
func (n *Signature) Source(full bool) []byte { return nodeSource(&bytes.Buffer{}, n, full).Bytes() }

// Tokens returns the tokens n consist of.
func (n *Signature) Tokens() []Token { return nodeTokens(n) }

// Parameters describes function parameters or a function result.
//
//	Parameters = "(" [ ParameterList [ "," ] ] ")" .
//	ParameterList = ParameterDecl { "," ParameterDecl } .
type Parameters struct {
	LParen        Token
	ParameterList []*ParameterDecl
	Comma         Token
	RParen        Token
}

// Position implements Node.
func (n *Parameters) Position() (r token.Position) {
	return n.LParen.Position()
}

// Source implements Node.
func (n *Parameters) Source(full bool) []byte { return nodeSource(&bytes.Buffer{}, n, full).Bytes() }

// Tokens returns the tokens n consist of.
func (n *Parameters) Tokens() []Token { return nodeTokens(n) }

// TypeDecl describes a type declaration.
//
//	TypeDecl = "type" ( TypeSpec | "(" { TypeSpec ";" } ")" ) .
type TypeDecl struct {
	TypeTok   Token
	LParen    Token
	TypeSpecs []Node
	RParen    Token
	Semicolon Token
}

// Position implements Node.
func (n *TypeDecl) Position() (r token.Position) {
	return n.TypeTok.Position()
}

// Source implements Node.
func (n *TypeDecl) Source(full bool) []byte { return nodeSource(&bytes.Buffer{}, n, full).Bytes() }

// Tokens returns the tokens n consist of.
func (n *TypeDecl) Tokens() []Token { return nodeTokens(n) }

// TypeDef describes a type definition.
//
//	TypeDef = identifier [ TypeParameters ] Type .
type TypeDef struct {
	lexicalScoper
	typer
	Ident          Token
	TypeParameters *TypeParameters
	TypeNode       Node
	Semicolon      Token
}

// Kind implements Type.
func (t *TypeDef) Kind() Kind { return t.Type().Kind() }

func (t *TypeDef) String() string { return t.Ident.Src() }

// Position implements Node.
func (n *TypeDef) Position() (r token.Position) {
	return n.Ident.Position()
}

// Source implements Node.
func (n *TypeDef) Source(full bool) []byte { return nodeSource(&bytes.Buffer{}, n, full).Bytes() }

func (n *TypeDef) check(c *ctx) Node {
	if !n.enter(c, n) {
		return n
	}

	defer n.exit()

	switch c.pkg.ImportPath {
	case "unsafe":
		switch n.Ident.Src() {
		case "Pointer":
			n.typ = PredefinedType(UnsafePointer)
			return n
		}
	}

	n.typ = c.checkType(n.TypeNode)
	return n
}

// Tokens returns the tokens n consist of.
func (n *TypeDef) Tokens() []Token { return nodeTokens(n) }

// ParameterDecl describes a parameter declaration.
//
//	ParameterDecl = [ IdentifierList ] [ "..." ] Type .
type ParameterDecl struct {
	IdentifierList []*IdentListItem
	Ellipsis       Token
	Type           Node
	Comma          Token
}

// Position implements Node.
func (n *ParameterDecl) Position() (r token.Position) {
	switch {
	case len(n.IdentifierList) != 0:
		return n.IdentifierList[0].Position()
	case n.Ellipsis.IsValid():
		return n.Ellipsis.Position()
	default:
		return n.Type.Position()
	}
}

// Source implements Node.
func (n *ParameterDecl) Source(full bool) []byte { return nodeSource(&bytes.Buffer{}, n, full).Bytes() }

// Tokens returns the tokens n consist of.
func (n *ParameterDecl) Tokens() []Token { return nodeTokens(n) }

// IdentListItem describes an item of an identifier list.
type IdentListItem struct {
	Ident Token
	Comma Token
}

// Position implements Node.
func (n *IdentListItem) Position() (r token.Position) {
	return n.Ident.Position()
}

// Source implements Node.
func (n *IdentListItem) Source(full bool) []byte {
	return nodeSource(&bytes.Buffer{}, n, full).Bytes()
}

// Tokens returns the tokens n consist of.
func (n *IdentListItem) Tokens() []Token { return nodeTokens(n) }

// VarDecl describes a variable declaration.
//
//	VarDecl = "var" ( VarSpec | "(" { VarSpec ";" } ")" ) .
type VarDecl struct {
	lexicalScoper
	Var       Token
	LParen    Token
	VarSpecs  []*VarSpec
	RParen    Token
	Semicolon Token
}

// Position implements Node.
func (n *VarDecl) Position() (r token.Position) {
	return n.Var.Position()
}

// Source implements Node.
func (n *VarDecl) Source(full bool) []byte { return nodeSource(&bytes.Buffer{}, n, full).Bytes() }

// Tokens returns the tokens n consist of.
func (n *VarDecl) Tokens() []Token { return nodeTokens(n) }

// ConstDecl describes a constant declaration.
//
//	ConstDecl = "const" ( ConstSpec | "(" { ConstSpec ";" } ")" ) .
type ConstDecl struct {
	Const      Token
	LParen     Token
	ConstSpecs []*ConstSpec
	RParen     Token
	Semicolon  Token
}

// Position implements Node.
func (n *ConstDecl) Position() (r token.Position) {
	return n.Const.Position()
}

// Source implements Node.
func (n *ConstDecl) Source(full bool) []byte { return nodeSource(&bytes.Buffer{}, n, full).Bytes() }

// Tokens returns the tokens n consist of.
func (n *ConstDecl) Tokens() []Token { return nodeTokens(n) }

// Block describes a compound statement.
//
//	Block = "{" StatementList "}" .
type Block struct {
	LBrace        Token
	StatementList []Node
	RBrace        Token
	Semicolon     Token
	Scope         *Scope
}

// Position implements Node.
func (n *Block) Position() (r token.Position) {
	return n.LBrace.Position()
}

// Source implements Node.
func (n *Block) Source(full bool) []byte { return nodeSource(&bytes.Buffer{}, n, full).Bytes() }

// Tokens returns the tokens n consist of.
func (n *Block) Tokens() []Token { return nodeTokens(n) }

// StructTypeNode describes a struct type.
//
//	StructTyp = "struct" "{" { FieldDecl ";" } "}" .
type StructTypeNode struct {
	typer
	typeNoder
	Struct     Token
	LBrace     Token
	FieldDecls []Node
	RBrace     Token
}

// Position implements Node.
func (n *StructTypeNode) Position() (r token.Position) {
	return n.Struct.Position()
}

// Source implements Node.
func (n *StructTypeNode) Source(full bool) []byte {
	return nodeSource(&bytes.Buffer{}, n, full).Bytes()
}

// Tokens returns the tokens n consist of.
func (n *StructTypeNode) Tokens() []Token { return nodeTokens(n) }

// FieldDecl describes a field declaration.
//
// FieldDecl = (IdentifierList Type | EmbeddedField) [ Tag ] .
type FieldDecl struct {
	IdentifierList []*IdentListItem
	Type           Node
	EmbeddedField  *EmbeddedField
	Tag            Token
	Semicolon      Token
}

// Position implements Node.
func (n *FieldDecl) Position() (r token.Position) {
	if len(n.IdentifierList) != 0 {
		return n.IdentifierList[0].Position()
	}

	return r
}

// Source implements Node.
func (n *FieldDecl) Source(full bool) []byte { return nodeSource(&bytes.Buffer{}, n, full).Bytes() }

// Tokens returns the tokens n consist of.
func (n *FieldDecl) Tokens() []Token { return nodeTokens(n) }

// EmbeddedField describes an embeded field.
//
//	EmbeddedField = [ "*" ] TypeName .
type EmbeddedField struct {
	Star     Token
	TypeName *TypeNameNode
}

// Position implements Node.
func (n *EmbeddedField) Position() (r token.Position) {
	if n.Star.IsValid() {
		return n.Star.Position()
	}

	return n.TypeName.Position()
}

// Source implements Node.
func (n *EmbeddedField) Source(full bool) []byte { return nodeSource(&bytes.Buffer{}, n, full).Bytes() }

// Tokens returns the tokens n consist of.
func (n *EmbeddedField) Tokens() []Token { return nodeTokens(n) }

// VarSpec describes a variable specification.
//
//	VarSpec = IdentifierList ( Type [ "=" ExpressionList ] | "=" ExpressionList ) .
type VarSpec struct {
	IdentifierList []*IdentListItem
	Type           Node
	Eq             Token
	ExprList       []*ExprListItem
	Semicolon      Token
}

// Position implements Node.
func (n *VarSpec) Position() (r token.Position) {
	if len(n.IdentifierList) != 0 {
		return n.IdentifierList[0].Position()
	}

	return r
}

// Source implements Node.
func (n *VarSpec) Source(full bool) []byte { return nodeSource(&bytes.Buffer{}, n, full).Bytes() }

// Tokens returns the tokens n consist of.
func (n *VarSpec) Tokens() []Token { return nodeTokens(n) }

// PointerTypeNode describes a pointer type.
//
//	PointerTypeNode = "*" BaseType .
type PointerTypeNode struct {
	typer
	typeNoder
	Star     Token
	BaseType Node
}

// Position implements Node.
func (n *PointerTypeNode) Position() (r token.Position) {
	return n.Star.Position()
}

// Source implements Node.
func (n *PointerTypeNode) Source(full bool) []byte {
	return nodeSource(&bytes.Buffer{}, n, full).Bytes()
}

// Tokens returns the tokens n consist of.
func (n *PointerTypeNode) Tokens() []Token { return nodeTokens(n) }

// TypeNameNode describes a type name.
//
//	TypeNameNode = QualifiedIdent [ TypeArgs ]
//		| identifier [ TypeArgs ] .
type TypeNameNode struct {
	typer
	typeNoder
	Name     *QualifiedIdent
	TypeArgs *TypeArgs
}

// Position implements Node.
func (n *TypeNameNode) Position() (r token.Position) {
	return n.Name.Position()
}

// Source implements Node.
func (n *TypeNameNode) Source(full bool) []byte { return nodeSource(&bytes.Buffer{}, n, full).Bytes() }

func (n *TypeNameNode) check(c *ctx) Node {
	if !n.enter(c, n) {
		return n
	}

	defer n.exit()

	n.typ = c.checkType(n.Name)
	return n
}

// Tokens returns the tokens n consist of.
func (n *TypeNameNode) Tokens() []Token { return nodeTokens(n) }

// QualifiedIdent describes an optionally qualified identifier.
//
//	QualifiedIdent = PackageName "." identifier .
type QualifiedIdent struct {
	lexicalScoper
	typer
	valuer
	PackageName Token
	Dot         Token
	Ident       Token
	resolvedIn  *Package
	resolvedTo  Node
}

// ResolvedIn returns the package n refers to. Valid after type checking.
func (n *QualifiedIdent) ResolvedIn() *Package { return n.resolvedIn }

// ResolvedTo returns the node n refers to. Valid after type checking.
func (n *QualifiedIdent) ResolvedTo() Node { return n.resolvedTo }

// Position implements Node.
func (n *QualifiedIdent) Position() (r token.Position) {
	if n.PackageName.IsValid() {
		return n.PackageName.Position()
	}

	return n.Ident.Position()
}

// Source implements Node.
func (n *QualifiedIdent) Source(full bool) []byte {
	return nodeSource(&bytes.Buffer{}, n, full).Bytes()
}

// Tokens returns the tokens n consist of.
func (n *QualifiedIdent) Tokens() []Token { return nodeTokens(n) }

// ConstSpec describes a constant specification.
//
//	ConstSpec = IdentifierList [ [ Type ] "=" ExpressionList ] .
type ConstSpec struct {
	IdentifierList []*IdentListItem
	Type           Node
	Eq             Token
	ExprList       []*ExprListItem
	Semicolon      Token
	iota           int64
	exprList       []*ExprListItem
	typ            Node
}

// Position implements Node.
func (n *ConstSpec) Position() (r token.Position) {
	if len(n.IdentifierList) != 0 {
		return n.IdentifierList[0].Position()
	}

	return r
}

// Source implements Node.
func (n *ConstSpec) Source(full bool) []byte { return nodeSource(&bytes.Buffer{}, n, full).Bytes() }

// Tokens returns the tokens n consist of.
func (n *ConstSpec) Tokens() []Token { return nodeTokens(n) }

// ExprListItem describes an item of an expression list.
//
// ExpressionList = Expression { "," Expression } .
type ExprListItem struct {
	Expr  Expression
	Comma Token
}

// Position implements Node.
func (n *ExprListItem) Position() (r token.Position) {
	return n.Expr.Position()
}

// Source implements Node.
func (n *ExprListItem) Source(full bool) []byte {
	return nodeSource(&bytes.Buffer{}, n, full).Bytes()
}

// Tokens returns the tokens n consist of.
func (n *ExprListItem) Tokens() []Token { return nodeTokens(n) }

// ExpressionStmt describes an expression statement.
type ExpressionStmt struct {
	Expr      Expression
	Semicolon Token
}

// Position implements Node.
func (n *ExpressionStmt) Position() (r token.Position) {
	return n.Expr.Position()
}

// Source implements Node.
func (n *ExpressionStmt) Source(full bool) []byte {
	return nodeSource(&bytes.Buffer{}, n, full).Bytes()
}

// Tokens returns the tokens n consist of.
func (n *ExpressionStmt) Tokens() []Token { return nodeTokens(n) }

// BinaryExpr describes a binary expression.
type BinaryExpr struct {
	typer
	valuer
	A  Expression
	Op Token
	B  Expression
}

// Position implements Node.
func (n *BinaryExpr) Position() (r token.Position) {
	return n.A.Position()
}

// Source implements Node.
func (n *BinaryExpr) Source(full bool) []byte {
	return nodeSource(&bytes.Buffer{}, n, full).Bytes()
}

// Tokens returns the tokens n consist of.
func (n *BinaryExpr) Tokens() []Token { return nodeTokens(n) }

// ShortVarDecl describes a short variable declaration.
//
//	ShortVarDecl = IdentifierList ":=" ExpressionList .
type ShortVarDecl struct {
	lexicalScoper
	simpleStmter
	IdentifierList []*IdentListItem
	Define         Token
	ExprList       []*ExprListItem
	Semicolon      Token
}

// Position implements Node.
func (n *ShortVarDecl) Position() (r token.Position) {
	return n.IdentifierList[0].Position()
}

// Source implements Node.
func (n *ShortVarDecl) Source(full bool) []byte { return nodeSource(&bytes.Buffer{}, n, full).Bytes() }

func (n *ShortVarDecl) semi(p *parser) { n.Semicolon = p.semi(true) }

// Tokens returns the tokens n consist of.
func (n *ShortVarDecl) Tokens() []Token { return nodeTokens(n) }

// MethodDecl describes a method declaration.
//
//	MethodDecl = "func" Receiver MethodName Signature [ FunctionBody ] .
type MethodDecl struct {
	typer
	Func         Token
	Receiver     *Parameters
	MethodName   Token
	Signature    *Signature
	FunctionBody *Block
	Semicolon    Token
}

// Position implements Node.
func (n *MethodDecl) Position() (r token.Position) {
	return n.Func.Position()
}

// Source implements Node.
func (n *MethodDecl) Source(full bool) []byte { return nodeSource(&bytes.Buffer{}, n, full).Bytes() }

// Tokens returns the tokens n consist of.
func (n *MethodDecl) Tokens() []Token { return nodeTokens(n) }

// ReturnStmt describes a return statement.
//
//	ReturnStmt = "return" [ ExpressionList ] .
type ReturnStmt struct {
	Return    Token
	ExprList  []*ExprListItem
	Semicolon Token
	container Node // *FunctionDecl or *MethodDecl or *FunctionLit
}

// Position implements Node.
func (n *ReturnStmt) Position() (r token.Position) {
	return n.Return.Position()
}

// Source implements Node.
func (n *ReturnStmt) Source(full bool) []byte { return nodeSource(&bytes.Buffer{}, n, full).Bytes() }

// Tokens returns the tokens n consist of.
func (n *ReturnStmt) Tokens() []Token { return nodeTokens(n) }

// Selector describes a selector.
//
//	Selector = PrimaryExpr "." identifier .
type Selector struct {
	typer
	valuer
	PrimaryExpr Expression
	Dot         Token
	Ident       Token
}

// Position implements Node.
func (n *Selector) Position() (r token.Position) {
	return n.PrimaryExpr.Position()
}

// Source implements Node.
func (n *Selector) Source(full bool) []byte { return nodeSource(&bytes.Buffer{}, n, full).Bytes() }

// Tokens returns the tokens n consist of.
func (n *Selector) Tokens() []Token { return nodeTokens(n) }

// Arguments describes a call or conversion.
//
//	Arguments = PrimaryExpr "(" [ ( ExpressionList | Type [ "," ExpressionList ] ) [ "..." ] [ "," ] ] ")" .
type Arguments struct {
	typer
	valuer
	PrimaryExpr Expression
	LParen      Token
	TypeArg     Node
	Comma       Token
	ExprList    []*ExprListItem
	Ellipsis    Token
	Comma2      Token
	RParen      Token
}

// Position implements Node.
func (n *Arguments) Position() (r token.Position) {
	return n.PrimaryExpr.Position()
}

// Source implements Node.
func (n *Arguments) Source(full bool) []byte { return nodeSource(&bytes.Buffer{}, n, full).Bytes() }

// Tokens returns the tokens n consist of.
func (n *Arguments) Tokens() []Token { return nodeTokens(n) }

// IfStmt describes an if statement.
//
//	IfStmt = "if" [ SimpleStmt ";" ] Expression Block [ "else" ( IfStmt | Block ) ] .
type IfStmt struct {
	If         Token
	SimpleStmt Node
	Semicolon  Token
	Expr       Expression
	Block      *Block
	Else       Token
	ElsePart   Node
	Semicolon2 Token
	Scope      *Scope // Implicit scope of the if statement
}

// Position implements Node.
func (n *IfStmt) Position() (r token.Position) {
	return n.If.Position()
}

// Source implements Node.
func (n *IfStmt) Source(full bool) []byte { return nodeSource(&bytes.Buffer{}, n, full).Bytes() }

// Tokens returns the tokens n consist of.
func (n *IfStmt) Tokens() []Token { return nodeTokens(n) }

// SliceTypeNode describes a slice type.
//
//	SliceTypeNode = "[" "]" ElementType .
type SliceTypeNode struct {
	typeNoder
	LBracket    Token
	RBracket    Token
	ElementType Node
}

// Position implements Node.
func (n *SliceTypeNode) Position() (r token.Position) {
	return n.LBracket.Position()
}

// Source implements Node.
func (n *SliceTypeNode) Source(full bool) []byte { return nodeSource(&bytes.Buffer{}, n, full).Bytes() }

// Tokens returns the tokens n consist of.
func (n *SliceTypeNode) Tokens() []Token { return nodeTokens(n) }

// Assignment describes a short variable declaration.
//
// Assignment = ExpressionList assign_op ExpressionList .
type Assignment struct {
	simpleStmter
	LExprList []*ExprListItem
	AssOp     Token
	RExprList []*ExprListItem
	Semicolon Token
}

// Position implements Node.
func (n *Assignment) Position() (r token.Position) {
	return n.LExprList[0].Position()
}

// Source implements Node.
func (n *Assignment) Source(full bool) []byte { return nodeSource(&bytes.Buffer{}, n, full).Bytes() }

func (n *Assignment) semi(p *parser) { n.Semicolon = p.semi(true) }

// Tokens returns the tokens n consist of.
func (n *Assignment) Tokens() []Token { return nodeTokens(n) }

// UnaryExpr describes an unary expression.
//
//	UnaryExpr = PrimaryExpr | unary_op UnaryExpr .
type UnaryExpr struct {
	typer
	valuer
	Op   Token
	Expr Expression
}

// Position implements Node.
func (n *UnaryExpr) Position() (r token.Position) {
	return n.Op.Position()
}

// Source implements Node.
func (n *UnaryExpr) Source(full bool) []byte { return nodeSource(&bytes.Buffer{}, n, full).Bytes() }

// Tokens returns the tokens n consist of.
func (n *UnaryExpr) Tokens() []Token { return nodeTokens(n) }

// CompositeLit describes a composite literal.
//
//	CompositeLit = LiteralType LiteralValue .
type CompositeLit struct {
	typer
	valuer
	LiteralType  Node
	LiteralValue *LiteralValue
}

// Position implements Node.
func (n *CompositeLit) Position() (r token.Position) {
	return n.LiteralType.Position()
}

// Source implements Node.
func (n *CompositeLit) Source(full bool) []byte { return nodeSource(&bytes.Buffer{}, n, full).Bytes() }

// Tokens returns the tokens n consist of.
func (n *CompositeLit) Tokens() []Token { return nodeTokens(n) }

// LiteralValue describes a composite literal value.
//
//	LiteralValue = "{" [ ElementList [ "," ] ] "}" .
type LiteralValue struct {
	LBrace      Token
	ElementList []*KeyedElement
	RBrace      Token
}

// Position implements Node.
func (n *LiteralValue) Position() (r token.Position) {
	return n.LBrace.Position()
}

// Source implements Node.
func (n *LiteralValue) Source(full bool) []byte { return nodeSource(&bytes.Buffer{}, n, full).Bytes() }

// Tokens returns the tokens n consist of.
func (n *LiteralValue) Tokens() []Token { return nodeTokens(n) }

// KeyedElement describes an optionally keyed element.
//
//	KeyedElement = [ Key ":" ] Element .
type KeyedElement struct {
	typer
	Key     Node
	Colon   Token
	Element Node
	Comma   Token
}

// Position implements Node.
func (n *KeyedElement) Position() (r token.Position) {
	if n.Key != nil {
		return n.Key.Position()
	}

	return n.Element.Position()
}

// Source implements Node.
func (n *KeyedElement) Source(full bool) []byte { return nodeSource(&bytes.Buffer{}, n, full).Bytes() }

// Tokens returns the tokens n consist of.
func (n *KeyedElement) Tokens() []Token { return nodeTokens(n) }

// InterfaceTypeNode describes an interface type.
//
//	InterfaceTypeNode = "interface" "{" { InterfaceElem ";" } "}" .
type InterfaceTypeNode struct {
	typer
	typeNoder
	Interface      Token
	LBrace         Token
	InterfaceElems []Node
	RBrace         Token
}

// Position implements Node.
func (n *InterfaceTypeNode) Position() (r token.Position) {
	return n.Interface.Position()
}

// Source implements Node.
func (n *InterfaceTypeNode) Source(full bool) []byte {
	return nodeSource(&bytes.Buffer{}, n, full).Bytes()
}

// Tokens returns the tokens n consist of.
func (n *InterfaceTypeNode) Tokens() []Token { return nodeTokens(n) }

// ForStmt describes a for statement.
//
//	ForStmt = "for" [ Condition | ForClause | RangeClause ] Block .
type ForStmt struct {
	For         Token
	ForClause   *ForClause
	RangeClause *RangeClause
	Block       *Block
	Semicolon   Token
	Scope       *Scope // Implicit scope of the for statement
}

// Position implements Node.
func (n *ForStmt) Position() (r token.Position) {
	return n.For.Position()
}

// Source implements Node.
func (n *ForStmt) Source(full bool) []byte { return nodeSource(&bytes.Buffer{}, n, full).Bytes() }

// Tokens returns the tokens n consist of.
func (n *ForStmt) Tokens() []Token { return nodeTokens(n) }

// ForClause describes a for clause.
//
//	ForClause = [ InitStmt ] ";" [ Condition ] ";" [ PostStmt ] .
type ForClause struct {
	InitStmt   Node
	Semicolon  Token
	Condition  Expression
	Semicolon2 Token
	PostStmt   Node
}

// Position implements Node.
func (n *ForClause) Position() (r token.Position) {
	if n.InitStmt != nil {
		return n.InitStmt.Position()
	}

	return n.Semicolon.Position()
}

// Source implements Node.
func (n *ForClause) Source(full bool) []byte { return nodeSource(&bytes.Buffer{}, n, full).Bytes() }

// Tokens returns the tokens n consist of.
func (n *ForClause) Tokens() []Token { return nodeTokens(n) }

// RangeClause describes a range clause.
//
//	RangeClause = [ ExpressionList "=" | IdentifierList ":=" ] "range" Expression .
type RangeClause struct {
	ExprList []*ExprListItem
	Assign   Token
	Range    Token
	Expr     Expression
}

// Position implements Node.
func (n *RangeClause) Position() (r token.Position) {
	return n.ExprList[0].Position()
}

// Source implements Node.
func (n *RangeClause) Source(full bool) []byte { return nodeSource(&bytes.Buffer{}, n, full).Bytes() }

// Tokens returns the tokens n consist of.
func (n *RangeClause) Tokens() []Token { return nodeTokens(n) }

// MethodElem describes a method element.
//
// MethodElem     = MethodName Signature .
type MethodElem struct {
	MethodName Token
	Signature  *Signature
	Semicolon  Token
}

// Position implements Node.
func (n *MethodElem) Position() (r token.Position) {
	return n.MethodName.Position()
}

// Source implements Node.
func (n *MethodElem) Source(full bool) []byte { return nodeSource(&bytes.Buffer{}, n, full).Bytes() }

// Tokens returns the tokens n consist of.
func (n *MethodElem) Tokens() []Token { return nodeTokens(n) }

// MethodExpr describes a method expression.
//
//	MethodExpr    = ReceiverType "." MethodName .
type MethodExpr struct {
	typer
	valuer
	Receiver Node
	Dot      Token
	Ident    Token
}

// Position implements Node.
func (n *MethodExpr) Position() (r token.Position) {
	return n.Receiver.Position()
}

// Source implements Node.
func (n *MethodExpr) Source(full bool) []byte { return nodeSource(&bytes.Buffer{}, n, full).Bytes() }

// Tokens returns the tokens n consist of.
func (n *MethodExpr) Tokens() []Token { return nodeTokens(n) }

// TypeParameters describes type parameters.
//
//	TypeParameters = "[" TypeParamList [ "," ] "]" .
type TypeParameters struct {
	LBracket      Token
	TypeParamList []*TypeParamDecl
	RBracket      Token
}

// Position implements Node.
func (n *TypeParameters) Position() (r token.Position) {
	return n.LBracket.Position()
}

// Source implements Node.
func (n *TypeParameters) Source(full bool) []byte {
	return nodeSource(&bytes.Buffer{}, n, full).Bytes()
}

// Tokens returns the tokens n consist of.
func (n *TypeParameters) Tokens() []Token { return nodeTokens(n) }

// TypeParamDecl describes an item of a type parameter list.
//
//	TypeParamDecl = IdentifierList TypeConstraint .
type TypeParamDecl struct {
	IdentifierList []*IdentListItem
	TypeConstraint *TypeElem
	Comma          Token
}

// Position implements Node.
func (n *TypeParamDecl) Position() (r token.Position) {
	if len(n.IdentifierList) != 0 {
		return n.IdentifierList[0].Position()
	}

	return r
}

// Source implements Node.
func (n *TypeParamDecl) Source(full bool) []byte { return nodeSource(&bytes.Buffer{}, n, full).Bytes() }

// Tokens returns the tokens n consist of.
func (n *TypeParamDecl) Tokens() []Token { return nodeTokens(n) }

// TypeElem describes a type element.
//
//	TypeElem = TypeTerm { "|" TypeTerm } .
type TypeElem struct {
	TypeTerms []*TypeTerm
	Semicolon Token
}

// Position implements Node.
func (n *TypeElem) Position() (r token.Position) {
	if len(n.TypeTerms) != 0 {
		return n.TypeTerms[0].Position()
	}

	return r
}

// Source implements Node.
func (n *TypeElem) Source(full bool) []byte { return nodeSource(&bytes.Buffer{}, n, full).Bytes() }

// Tokens returns the tokens n consist of.
func (n *TypeElem) Tokens() []Token { return nodeTokens(n) }

// TypeTerm describes a type term.
//
//	TypeTerm = Type | UnderlyingType .
//	UnderlyingType = "~" Type .
type TypeTerm struct {
	Tilde Token
	Type  Node
	Pipe  Token
}

// Position implements Node.
func (n *TypeTerm) Position() (r token.Position) {
	if n.Tilde.IsValid() {
		return n.Tilde.Position()
	}

	return n.Type.Position()
}

// Source implements Node.
func (n *TypeTerm) Source(full bool) []byte { return nodeSource(&bytes.Buffer{}, n, full).Bytes() }

// Tokens returns the tokens n consist of.
func (n *TypeTerm) Tokens() []Token { return nodeTokens(n) }

// Index describes an index.
//
//	Index = "[" Expression "]" .
type Index struct {
	typer
	valuer
	PrimaryExpr Expression
	LBracket    Token
	Expr        Expression
	RBracket    Token
}

// Position implements Node.
func (n *Index) Position() (r token.Position) {
	return n.PrimaryExpr.Position()
}

// Source implements Node.
func (n *Index) Source(full bool) []byte { return nodeSource(&bytes.Buffer{}, n, full).Bytes() }

// Tokens returns the tokens n consist of.
func (n *Index) Tokens() []Token { return nodeTokens(n) }

// DeferStmt describes a defer statement.
//
//	DeferStmt = "defer" Expression .
type DeferStmt struct {
	Defer     Token
	Expr      Expression
	Semicolon Token
}

// Position implements Node.
func (n *DeferStmt) Position() (r token.Position) {
	return n.Defer.Position()
}

// Source implements Node.
func (n *DeferStmt) Source(full bool) []byte { return nodeSource(&bytes.Buffer{}, n, full).Bytes() }

// Tokens returns the tokens n consist of.
func (n *DeferStmt) Tokens() []Token { return nodeTokens(n) }

// EmptyStmt describes an empty statement.
//
//	EmptyStmt = .
type EmptyStmt struct {
	Semicolon Token
}

// Position implements Node.
func (n *EmptyStmt) Position() (r token.Position) {
	return n.Semicolon.Position()
}

// Source implements Node.
func (n *EmptyStmt) Source(full bool) []byte { return nodeSource(&bytes.Buffer{}, n, full).Bytes() }

// Tokens returns the tokens n consist of.
func (n *EmptyStmt) Tokens() []Token { return nodeTokens(n) }

// FunctionLit describes a function literal.
//
//	FunctionLit = "func" Signature FunctionBody .
type FunctionLit struct {
	typer
	valuer
	Func         Token
	Signature    *Signature
	FunctionBody *Block
}

// Position implements Node.
func (n *FunctionLit) Position() (r token.Position) {
	return n.Func.Position()
}

// Source implements Node.
func (n *FunctionLit) Source(full bool) []byte { return nodeSource(&bytes.Buffer{}, n, full).Bytes() }

// Tokens returns the tokens n consist of.
func (n *FunctionLit) Tokens() []Token { return nodeTokens(n) }

// ExpressionSwitchStmt describes an expression switch statement.
//
//	ExprSwitchStmt = "switch" [ SimpleStmt ";" ] [ Expression ] "{" { ExprCaseClause } "}" .
type ExpressionSwitchStmt struct {
	Switch          Token
	SimpleStmt      Node
	Semicolon       Token
	Expr            Expression
	LBrace          Token
	ExprCaseClauses []*ExprCaseClause
	RBrace          Token
	Semicolon2      Token
	Scope           *Scope // Implicit scope of the switch statement
}

// Position implements Node.
func (n *ExpressionSwitchStmt) Position() (r token.Position) {
	return n.Switch.Position()
}

// Source implements Node.
func (n *ExpressionSwitchStmt) Source(full bool) []byte {
	return nodeSource(&bytes.Buffer{}, n, full).Bytes()
}

// Tokens returns the tokens n consist of.
func (n *ExpressionSwitchStmt) Tokens() []Token { return nodeTokens(n) }

// TypeSwitchStmt describes a type switch statement.
//
//	TypeSwitchStmt  = "switch" [ SimpleStmt ";" ] TypeSwitchGuard "{" { TypeCaseClause } "}" .
type TypeSwitchStmt struct {
	Switch          Token
	SimpleStmt      Node
	Semicolon       Token
	TypeSwitchGuard *TypeSwitchGuard
	LBrace          Token
	TypeCaseClauses []*TypeCaseClause
	RBrace          Token
	Semicolon2      Token
	Scope           *Scope // Implicit scope of the switch statement
}

// Position implements Node.
func (n *TypeSwitchStmt) Position() (r token.Position) {
	return n.Switch.Position()
}

// Source implements Node.
func (n *TypeSwitchStmt) Source(full bool) []byte {
	return nodeSource(&bytes.Buffer{}, n, full).Bytes()
}

// Tokens returns the tokens n consist of.
func (n *TypeSwitchStmt) Tokens() []Token { return nodeTokens(n) }

// TypeSwitchGuard describes a type switch guard.
//
//	TypeSwitchGuard = [ identifier ":=" ] PrimaryExpr "." "(" "type" ")" .
type TypeSwitchGuard struct {
	typer
	valuer
	Ident       Token
	Define      Token
	PrimaryExpr Expression
	Dot         Token
	LParen      Token
	TypeToken   Token
	RParen      Token
}

// Position implements Node.
func (n *TypeSwitchGuard) Position() (r token.Position) {
	if n.Ident.IsValid() {
		return n.Ident.Position()
	}

	return n.PrimaryExpr.Position()
}

// Source implements Node.
func (n *TypeSwitchGuard) Source(full bool) []byte {
	return nodeSource(&bytes.Buffer{}, n, full).Bytes()
}

// Tokens returns the tokens n consist of.
func (n *TypeSwitchGuard) Tokens() []Token { return nodeTokens(n) }

// TypeCaseClause describes a type switch case clause.
//
//	TypeCaseClause  = TypeSwitchCase ":" StatementList .
type TypeCaseClause struct {
	TypeSwitchCase *TypeSwitchCase
	Colon          Token
	StatementList  []Node
}

// Position implements Node.
func (n *TypeCaseClause) Position() (r token.Position) {
	return n.TypeSwitchCase.Position()
}

// Source implements Node.
func (n *TypeCaseClause) Source(full bool) []byte {
	return nodeSource(&bytes.Buffer{}, n, full).Bytes()
}

// Tokens returns the tokens n consist of.
func (n *TypeCaseClause) Tokens() []Token { return nodeTokens(n) }

// TypeSwitchCase describes an expression switch case.
//
//	TypeSwitchCase  = "case" TypeList | "default" .
type TypeSwitchCase struct {
	CaseOrDefault Token
	TypeList      []*TypeListItem
}

// Position implements Node.
func (n *TypeSwitchCase) Position() (r token.Position) {
	return n.CaseOrDefault.Position()
}

// Source implements Node.
func (n *TypeSwitchCase) Source(full bool) []byte {
	return nodeSource(&bytes.Buffer{}, n, full).Bytes()
}

// Tokens returns the tokens n consist of.
func (n *TypeSwitchCase) Tokens() []Token { return nodeTokens(n) }

// TypeAssertion describes a type assertion.
//
//	TypeAssertion = PrimaryExpr "." "(" Type ")" .
type TypeAssertion struct {
	typer
	valuer
	PrimaryExpr Expression
	Dot         Token
	LParen      Token
	AssertType  Node
	RParen      Token
}

// Position implements Node.
func (n *TypeAssertion) Position() (r token.Position) {
	return n.PrimaryExpr.Position()
}

// Source implements Node.
func (n *TypeAssertion) Source(full bool) []byte { return nodeSource(&bytes.Buffer{}, n, full).Bytes() }

// Tokens returns the tokens n consist of.
func (n *TypeAssertion) Tokens() []Token { return nodeTokens(n) }

// CommClause describes an select statement communication clause.
//
//	CommClause = CommCase ":" StatementList .
type CommClause struct {
	CommCase      *CommCase
	Colon         Token
	StatementList []Node
}

// Position implements Node.
func (n *CommClause) Position() (r token.Position) {
	return n.CommCase.Position()
}

// Source implements Node.
func (n *CommClause) Source(full bool) []byte { return nodeSource(&bytes.Buffer{}, n, full).Bytes() }

// Tokens returns the tokens n consist of.
func (n *CommClause) Tokens() []Token { return nodeTokens(n) }

// CommCase describes an communication clause case.
//
//	CommCase   = "case" ( SendStmt | RecvStmt ) | "default" .
type CommCase struct {
	CaseOrDefault Token
	Statement     Node
}

// Position implements Node.
func (n *CommCase) Position() (r token.Position) {
	return n.CaseOrDefault.Position()
}

// Source implements Node.
func (n *CommCase) Source(full bool) []byte { return nodeSource(&bytes.Buffer{}, n, full).Bytes() }

// Tokens returns the tokens n consist of.
func (n *CommCase) Tokens() []Token { return nodeTokens(n) }

// ExprCaseClause describes an expression switch case clause.
//
//	ExprCaseClause = ExprSwitchCase ":" StatementList .
type ExprCaseClause struct {
	ExprSwitchCase *ExprSwitchCase
	Colon          Token
	StatementList  []Node
}

// Position implements Node.
func (n *ExprCaseClause) Position() (r token.Position) {
	return n.ExprSwitchCase.Position()
}

// Source implements Node.
func (n *ExprCaseClause) Source(full bool) []byte {
	return nodeSource(&bytes.Buffer{}, n, full).Bytes()
}

// Tokens returns the tokens n consist of.
func (n *ExprCaseClause) Tokens() []Token { return nodeTokens(n) }

// ExprSwitchCase describes an expression switch case.
//
//	ExprSwitchCase = "case" ExpressionList | "default" .
type ExprSwitchCase struct {
	CaseOrDefault Token
	ExprList      []*ExprListItem
}

// Position implements Node.
func (n *ExprSwitchCase) Position() (r token.Position) {
	return n.CaseOrDefault.Position()
}

// Source implements Node.
func (n *ExprSwitchCase) Source(full bool) []byte {
	return nodeSource(&bytes.Buffer{}, n, full).Bytes()
}

// Tokens returns the tokens n consist of.
func (n *ExprSwitchCase) Tokens() []Token { return nodeTokens(n) }

// SliceExpr describes a slice expression.
//
//	SliceExpr = "[" [ Expression ] ":" [ Expression ] "]" | "[" [ Expression ] ":" Expression ":" Expression "]" .
type SliceExpr struct {
	typer
	valuer
	PrimaryExpr Expression
	LBracket    Token
	Expr        Expression
	Colon       Token
	Expr2       Expression
	Colon2      Token
	Expr3       Expression
	RBracket    Token
}

// Position implements Node.
func (n *SliceExpr) Position() (r token.Position) {
	return n.LBracket.Position()
}

// Source implements Node.
func (n *SliceExpr) Source(full bool) []byte { return nodeSource(&bytes.Buffer{}, n, full).Bytes() }

// Tokens returns the tokens n consist of.
func (n *SliceExpr) Tokens() []Token { return nodeTokens(n) }

// SelectStmt describes a select statement.
//
//	SelectStmt = "select" "{" { CommClause } "}" .
type SelectStmt struct {
	Select      Token
	LBrace      Token
	CommClauses []*CommClause
	RBrace      Token
	Semicolon   Token
}

// Position implements Node.
func (n *SelectStmt) Position() (r token.Position) {
	return n.Select.Position()
}

// Source implements Node.
func (n *SelectStmt) Source(full bool) []byte { return nodeSource(&bytes.Buffer{}, n, full).Bytes() }

// Tokens returns the tokens n consist of.
func (n *SelectStmt) Tokens() []Token { return nodeTokens(n) }

// SendStmt describes a send statement.
//
//	SendStmt = Channel "<-" Expression .
type SendStmt struct {
	simpleStmter
	Channel   Node
	Arrow     Token
	Expr      Expression
	Semicolon Token
}

// Position implements Node.
func (n *SendStmt) Position() (r token.Position) {
	return n.Channel.Position()
}

// Source implements Node.
func (n *SendStmt) Source(full bool) []byte { return nodeSource(&bytes.Buffer{}, n, full).Bytes() }

func (n *SendStmt) semi(p *parser) { n.Semicolon = p.semi(true) }

// Tokens returns the tokens n consist of.
func (n *SendStmt) Tokens() []Token { return nodeTokens(n) }

// BreakStmt describes a continue statement.
//
//	BreakStmt = "break" [ Label ] .
type BreakStmt struct {
	Break     Token
	Label     Token
	Semicolon Token
}

// Position implements Node.
func (n *BreakStmt) Position() (r token.Position) {
	return n.Break.Position()
}

// Source implements Node.
func (n *BreakStmt) Source(full bool) []byte { return nodeSource(&bytes.Buffer{}, n, full).Bytes() }

// Tokens returns the tokens n consist of.
func (n *BreakStmt) Tokens() []Token { return nodeTokens(n) }

// ContinueStmt describes a continue statement.
//
//	ContinueStmt = "continue" [ Label ] .
type ContinueStmt struct {
	Continue  Token
	Label     Token
	Semicolon Token
}

// Position implements Node.
func (n *ContinueStmt) Position() (r token.Position) {
	return n.Continue.Position()
}

// Source implements Node.
func (n *ContinueStmt) Source(full bool) []byte { return nodeSource(&bytes.Buffer{}, n, full).Bytes() }

// Tokens returns the tokens n consist of.
func (n *ContinueStmt) Tokens() []Token { return nodeTokens(n) }

// FallthroughStmt describes a fallthrough statement.
//
//	FallthroughStmt = "fallthrough" .
type FallthroughStmt struct {
	Fallthrough Token
	Semicolon   Token
}

// Position implements Node.
func (n *FallthroughStmt) Position() (r token.Position) {
	return n.Fallthrough.Position()
}

// Source implements Node.
func (n *FallthroughStmt) Source(full bool) []byte {
	return nodeSource(&bytes.Buffer{}, n, full).Bytes()
}

// Tokens returns the tokens n consist of.
func (n *FallthroughStmt) Tokens() []Token { return nodeTokens(n) }

// Conversion describes a conversion.
//
//	Conversion = Type "(" Expression [ "," ] ")" .
type Conversion struct {
	typer
	valuer
	ConvertType Node
	LParen      Token
	Expr        Expression
	Comma       Token
	RParen      Token
}

// Position implements Node.
func (n *Conversion) Position() (r token.Position) {
	return n.ConvertType.Position()
}

// Source implements Node.
func (n *Conversion) Source(full bool) []byte { return nodeSource(&bytes.Buffer{}, n, full).Bytes() }

// Tokens returns the tokens n consist of.
func (n *Conversion) Tokens() []Token { return nodeTokens(n) }

// AliasDecl describes a type alias.
//
//	AliasDecl = identifier "=" Type .
type AliasDecl struct {
	lexicalScoper
	typer
	Ident     Token
	Eq        Token
	TypeNode  Node
	Semicolon Token
}

// Position implements Node.
func (n *AliasDecl) Position() (r token.Position) {
	return n.Ident.Position()
}

// Source implements Node.
func (n *AliasDecl) Source(full bool) []byte { return nodeSource(&bytes.Buffer{}, n, full).Bytes() }

// Tokens returns the tokens n consist of.
func (n *AliasDecl) Tokens() []Token { return nodeTokens(n) }

// ArrayTypeNode describes a channel type.
//
//	ArrayType   = "[" ArrayLength "]" ElementType .
//	ArrayLength = Expression | "..."
type ArrayTypeNode struct {
	typer
	typeNoder
	LBracket    Token
	ArrayLength Expression
	Ellipsis    Token
	RBracket    Token
	ElementType Node
}

// Position implements Node.
func (n *ArrayTypeNode) Position() (r token.Position) {
	return n.LBracket.Position()
}

// Source implements Node.
func (n *ArrayTypeNode) Source(full bool) []byte { return nodeSource(&bytes.Buffer{}, n, full).Bytes() }

// Tokens returns the tokens n consist of.
func (n *ArrayTypeNode) Tokens() []Token { return nodeTokens(n) }

// ChannelTypeNode describes a channel type.
//
//	ChannelTypeNode = ( "chan" | "chan" "<-" | "<-" "chan" ) ElementType .
type ChannelTypeNode struct {
	typeNoder
	ArrowPre    Token
	Chan        Token
	ArrayPost   Token
	ElementType Node
}

// Position implements Node.
func (n *ChannelTypeNode) Position() (r token.Position) {
	if n.ArrowPre.IsValid() {
		return n.ArrowPre.Position()
	}

	return n.Chan.Position()
}

// Source implements Node.
func (n *ChannelTypeNode) Source(full bool) []byte {
	return nodeSource(&bytes.Buffer{}, n, full).Bytes()
}

// Tokens returns the tokens n consist of.
func (n *ChannelTypeNode) Tokens() []Token { return nodeTokens(n) }

// FunctionTypeNode describes a function type.
//
//	FunctionTypeNode = "func" Signature .
type FunctionTypeNode struct {
	typer
	typeNoder
	Func      Token
	Signature *Signature
}

// Position implements Node.
func (n *FunctionTypeNode) Position() (r token.Position) {
	return n.Func.Position()
}

// Source implements Node.
func (n *FunctionTypeNode) Source(full bool) []byte {
	return nodeSource(&bytes.Buffer{}, n, full).Bytes()
}

// Tokens returns the tokens n consist of.
func (n *FunctionTypeNode) Tokens() []Token { return nodeTokens(n) }

// MapTypeNode describes a map type.
//
//	MapTypeNode = "map" "[" KeyType "]" ElementType .
type MapTypeNode struct {
	typeNoder
	Map         Token
	LBracket    Token
	KeyType     Node
	RBracket    Token
	ElementType Node
}

// Position implements Node.
func (n *MapTypeNode) Position() (r token.Position) {
	return n.Map.Position()
}

// Source implements Node.
func (n *MapTypeNode) Source(full bool) []byte { return nodeSource(&bytes.Buffer{}, n, full).Bytes() }

// Tokens returns the tokens n consist of.
func (n *MapTypeNode) Tokens() []Token { return nodeTokens(n) }

// GoStmt describes a go statement.
//
// GoStmt = "go" Expression .
type GoStmt struct {
	Go        Token
	Expr      Expression
	Semicolon Token
}

// Position implements Node.
func (n *GoStmt) Position() (r token.Position) {
	return n.Go.Position()
}

// Source implements Node.
func (n *GoStmt) Source(full bool) []byte { return nodeSource(&bytes.Buffer{}, n, full).Bytes() }

// Tokens returns the tokens n consist of.
func (n *GoStmt) Tokens() []Token { return nodeTokens(n) }

// GenericOperand describes an operand name and type arguments.
//
// GenericOperand = OperandName TypeArgs .
type GenericOperand struct {
	typer
	valuer
	OperandName Node
	TypeArgs    *TypeArgs
}

// Position implements Node.
func (n *GenericOperand) Position() (r token.Position) {
	return n.OperandName.Position()
}

// Source implements Node.
func (n *GenericOperand) Source(full bool) []byte {
	return nodeSource(&bytes.Buffer{}, n, full).Bytes()
}

// Tokens returns the tokens n consist of.
func (n *GenericOperand) Tokens() []Token { return nodeTokens(n) }

// GotoStmt describes a goto statement.
//
//	GotoStmt = "goto" Label .
type GotoStmt struct {
	Goto      Token
	Label     Token
	Semicolon Token
}

// Position implements Node.
func (n *GotoStmt) Position() (r token.Position) {
	return n.Goto.Position()
}

// Source implements Node.
func (n *GotoStmt) Source(full bool) []byte { return nodeSource(&bytes.Buffer{}, n, full).Bytes() }

// Tokens returns the tokens n consist of.
func (n *GotoStmt) Tokens() []Token { return nodeTokens(n) }

// LabeledStmt describes a labeled statement.
//
//	LabeledStmt = Label ":" Statement .
type LabeledStmt struct {
	Label     Token
	Colon     Token
	Statement Node
}

// Position implements Node.
func (n *LabeledStmt) Position() (r token.Position) {
	return n.Label.Position()
}

// Source implements Node.
func (n *LabeledStmt) Source(full bool) []byte { return nodeSource(&bytes.Buffer{}, n, full).Bytes() }

// Tokens returns the tokens n consist of.
func (n *LabeledStmt) Tokens() []Token { return nodeTokens(n) }

// TypeArgs describes a type name.
//
//	TypeArgs = "[" TypeList [ "," ] "]" .
type TypeArgs struct {
	LBracket Token
	TypeList []*TypeListItem
	RBracket Token
	Comma    Token
}

// Position implements Node.
func (n *TypeArgs) Position() (r token.Position) {
	return n.LBracket.Position()
}

// Source implements Node.
func (n *TypeArgs) Source(full bool) []byte { return nodeSource(&bytes.Buffer{}, n, full).Bytes() }

// Tokens returns the tokens n consist of.
func (n *TypeArgs) Tokens() []Token { return nodeTokens(n) }

// TypeListItem describes an item of a type list.
type TypeListItem struct {
	Type  Node
	Comma Token
}

// Position implements Node.
func (n *TypeListItem) Position() (r token.Position) {
	return n.Type.Position()
}

// Source implements Node.
func (n *TypeListItem) Source(full bool) []byte { return nodeSource(&bytes.Buffer{}, n, full).Bytes() }

// Tokens returns the tokens n consist of.
func (n *TypeListItem) Tokens() []Token { return nodeTokens(n) }

// IncDecStmt describes an increment or decrement statemen.
//
//	IncDecStmt = Expression ( "++" | "--" ) .
type IncDecStmt struct {
	simpleStmter
	Expr      Expression
	Op        Token
	Semicolon Token
}

// Position implements Node.
func (n *IncDecStmt) Position() (r token.Position) {
	return n.Expr.Position()
}

// Source implements Node.
func (n *IncDecStmt) Source(full bool) []byte { return nodeSource(&bytes.Buffer{}, n, full).Bytes() }

func (n *IncDecStmt) semi(p *parser) { n.Semicolon = p.semi(true) }

// Tokens returns the tokens n consist of.
func (n *IncDecStmt) Tokens() []Token { return nodeTokens(n) }

// ParenExpr describes a parenthesized expression.
//
// ParenExpr = "(" Expression ")" .
type ParenExpr struct {
	typer
	valuer
	LParen Token
	Expr   Expression
	RParen Token
}

// Position implements Node.
func (n *ParenExpr) Position() (r token.Position) {
	return n.LParen.Position()
}

// Source implements Node.
func (n *ParenExpr) Source(full bool) []byte { return nodeSource(&bytes.Buffer{}, n, full).Bytes() }

// Tokens returns the tokens n consist of.
func (n *ParenExpr) Tokens() []Token { return nodeTokens(n) }

// ParenType describes a parenthesized type.
//
// ParenType = "(" Type ")" .
type ParenType struct {
	typer
	typeNoder
	LParen   Token
	TypeNode Node
	RParen   Token
}

// Position implements Node.
func (n *ParenType) Position() (r token.Position) {
	return n.LParen.Position()
}

// Source implements Node.
func (n *ParenType) Source(full bool) []byte { return nodeSource(&bytes.Buffer{}, n, full).Bytes() }

// Tokens returns the tokens n consist of.
func (n *ParenType) Tokens() []Token { return nodeTokens(n) }

// Constant represents a Go constant.
type Constant struct {
	node *ConstSpec
	typer
	valuer
	Expr  Expression
	Ident Token
}

// Position implements Node.
func (n *Constant) Position() (r token.Position) { return n.Ident.Position() }

// Source implements Node.
func (n *Constant) Source(full bool) []byte {
	return nodeSource(&bytes.Buffer{}, n.Expr, full).Bytes()
}

// Tokens returns the tokens n consist of.
func (n *Constant) Tokens() []Token { return nodeTokens(n) }

// Variable represents a Go variable.
type Variable struct {
	typer
	valuer
	Expr     Expression
	Ident    Token
	TypeNode Node

	isParamater bool
}

// IsParameter reports whether n is a function/method parameter, result
// variable or receiver.
func (n *Variable) IsParameter() bool { return n.isParamater }

// Position implements Node.
func (n *Variable) Position() (r token.Position) { return n.Ident.Position() }

// Source implements Node.
func (n *Variable) Source(full bool) []byte {
	return nodeSource(&bytes.Buffer{}, n.Expr, full).Bytes()
}

// Tokens returns the tokens n consist of.
func (n *Variable) Tokens() []Token { return nodeTokens(n) }

// BasicLit represents a basic literal.
type BasicLit struct {
	typer
	valuer
	Token Token
}

// Position implements Node.
func (n *BasicLit) Position() (r token.Position) { return n.Token.Position() }

// Source implements Node.
func (n *BasicLit) Source(full bool) []byte { return n.Token.src() }

// Tokens returns the tokens n consist of.
func (n *BasicLit) Tokens() []Token { return nodeTokens(n) }

// Ident represents an unqualified operand/type name.
type Ident struct {
	lexicalScoper
	typer
	valuer
	Token      Token
	resolvedTo Node
}

// ResolvedTo returns the node n refers to. Valid after type checking.
func (n *Ident) ResolvedTo() Node { return n.resolvedTo }

// Position implements Node.
func (n *Ident) Position() (r token.Position) { return n.Token.Position() }

// Source implements Node.
func (n *Ident) Source(full bool) []byte { return n.Token.src() }

// Tokens returns the tokens n consist of.
func (n *Ident) Tokens() []Token { return []Token{n.Token} }
