// Copyright 2022 The Gc Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package gc // import "modernc.org/gc/v2"

import (
	"fmt"
	"go/constant"
	"go/token"
)

var (
	trcError bool // testing
)

type parser struct {
	cfg          *ParseSourceFileConfig
	container    Node // *FunctionDecl or *MethodDecl or *FunctionLit
	lexicalScope *Scope
	loophacks    []bool
	s            *Scanner
	scopes       []*Scope

	loophack bool
}

func newParser(cfg *ParseSourceFileConfig, s *Scanner) *parser {
	s.Scan()
	return &parser{cfg: cfg, s: s, lexicalScope: cfg.packageScope}
}

func (p *parser) Err() error                          { return p.s.Err() }
func (p *parser) ch() Ch                              { return p.s.Tok.Ch }
func (p *parser) err(msg string, args ...interface{}) { p.errNode(p.s.Tok, msg, args...) }

func (p *parser) errNode(n Node, msg string, args ...interface{}) {
	if trcError {
		var p token.Position
		if n != nil {
			p = n.Position()
		}
		trc("%v: ERROR %s", p, fmt.Sprintf(msg, args...))
	}
	p.s.errs.err(n.Position(), msg, args...)
	if !p.cfg.AllErrors && len(p.s.errs) >= 10 {
		p.s.close()
	}
}

func (p *parser) pushScope(s *Scope) func() {
	s.Parent = p.lexicalScope
	p.scopes = append(p.scopes, p.lexicalScope)
	p.lexicalScope = s
	return func() {
		n := len(p.scopes)
		p.lexicalScope = p.scopes[n-1]
		p.scopes = p.scopes[:n-1]
	}
}

func (p *parser) must(c Ch) (r Token) {
	if p.ch() != c {
		p.err(errorf("expected %v, got %v", c.str(), p.ch().str()))
	}
	return p.shift()
}

func (p *parser) opt(c Ch) (r Token) {
	if p.ch() == c {
		r = p.shift()
	}
	return r
}

func (p *parser) shift() (r Token) {
	r = p.s.Tok
	p.s.Scan()
	switch p.ch() {
	case FOR, IF, SELECT, SWITCH:
		p.loophack = true
	case '(', '[':
		if p.loophack || len(p.loophacks) != 0 {
			p.loophacks = append(p.loophacks, p.loophack)
			p.loophack = false
		}
	case ')', ']':
		if n := len(p.loophacks); n != 0 {
			p.loophack = p.loophacks[n-1]
			p.loophacks = p.loophacks[:n-1]
		}
	case '{':
		if p.loophack {
			p.s.Tok.Ch = body
			p.loophack = false
		}
	}
	// trc("SHIFT %v (%v: %v: %v)", r, origin(4), origin(3), origin(2))
	return r
}

func (p *parser) lbrace(lbr *bool) (r Token) {
	switch p.ch() {
	case '{':
		return p.shift()
	case body:
		r = p.shift()
		r.Ch = '{'
		if lbr != nil {
			*lbr = true
		}
		return r
	default:
		p.err(errorf("TODO %v", p.s.Tok.Ch.str()))
		p.shift()
		return r
	}
}

func (p *parser) fixlbr(lbr bool) (r Token) {
	if lbr {
		p.loophack = true
	}
	return p.must('}')
}

func (p *parser) semi(enabled bool) (r Token) {
	if enabled {
		switch p.ch() {
		case ';':
			return p.shift()
		case ')', '}':
			// Specs: To allow complex statements to occupy a single line, a semicolon may
			// be omitted before a closing ")" or "}".
		default:
			p.err(errorf("TODO %v", p.s.Tok.Ch.str()))
			p.shift()
		}
	}
	r = p.s.Tok
	r.source = nil
	return r
}

// TopLevelDecl = Declaration
//
//	| FunctionDecl
//	| MethodDecl .
func (p *parser) topLevelDecls() (r []Node) {
	//              TopLevelDecl case CONST, FUNC, TYPE, VAR:
	for {
		switch p.ch() {
		case CONST:
			r = append(r, p.constDecl())
		case FUNC:
			f := p.shift()
			switch p.ch() {
			case IDENTIFIER:
				r = append(r, p.functionDecl(f))
			case '(':
				r = append(r, p.methodDecl(f))
			default:
				p.err(errorf("TODO %v", p.s.Tok.Ch.str()))
				p.shift()
				return r
			}
		case TYPE:
			r = append(r, p.typeDecl())
		case VAR:
			r = append(r, p.varDecl())
		default:
			return r
		}
	}
}

// ConstDecl = "const" "(" ")"
//
//	| "const" "(" ConstSpec ConstDecl_1 ";" ")"
//	| "const" "(" ConstSpec ConstDecl_2 ")"
//	| "const" ConstSpec .
//
// ConstDecl_1 =
//
//	| ConstDecl_1 ";" ConstSpec .
//
// ConstDecl_2 =
//
//	| ConstDecl_2 ";" ConstSpec .
func (p *parser) constDecl() (r *ConstDecl) {
	c := p.must(CONST)
	switch p.ch() {
	case '(':
		return &ConstDecl{Const: c, LParen: p.shift(), ConstSpecs: p.constSpecs(), RParen: p.must(')'), Semicolon: p.semi(true)}
	//                 ConstSpec
	case IDENTIFIER:
		return &ConstDecl{Const: c, ConstSpecs: []*ConstSpec{p.constSpec(false)}, Semicolon: p.semi(true)}
	default:
		p.err(errorf("TODO %v", p.s.Tok.Ch.str()))
		p.shift()
		return r
	}
}

func (p *parser) constSpecs() (r []*ConstSpec) {
	var iota int64
	var el []*ExprListItem
	var typ Node
	for p.ch() == IDENTIFIER {
		n := p.constSpec(true)
		r = append(r, n)
		n.iota = iota
		iota++
		if len(n.ExprList) != 0 {
			el = n.ExprList
		}
		n.exprList = el
		if n.Type != nil {
			typ = n.Type
		}
		n.typ = typ
	}
	return r
}

// ConstSpec = IdentifierList "=" ExpressionList
//
//	| IdentifierList Type "=" ExpressionList
//	| IdentifierList .
func (p *parser) constSpec(semi bool) (r *ConstSpec) {
	if p.ch() != IDENTIFIER {
		p.err(errorf("TODO %v", p.s.Tok.Ch.str()))
		p.shift()
		return r
	}

	r = &ConstSpec{IdentifierList: p.identifierList()}
	switch p.ch() {
	case '=':
		r.Eq = p.shift()
		r.ExprList = p.expressionList()
	//                      Type
	case '(', '*', '[', ARROW, CHAN, FUNC, IDENTIFIER, INTERFACE, MAP, STRUCT:
		r.Type = p.type1()
		r.Eq = p.shift()
		r.ExprList = p.expressionList()
	case ';':
		// ok
	default:
		p.err(errorf("TODO %v", p.s.Tok.Ch.str()))
		p.shift()
		return r
	}
	if len(r.IdentifierList) != len(r.ExprList) && len(r.ExprList) != 0 {
		p.err(errorf("TODO %v", p.s.Tok.Ch.str()))
		p.shift()
	}
	r.exprList = r.ExprList
	r.Semicolon = p.semi(semi)
	return r
}

// TypeDecl = "type" TypeSpec
//
//	| "type" "(" TypeDecl_1 ")" .
//
// TypeDecl_1 =
//
//	| TypeDecl_1 TypeSpec ";" .
func (p *parser) typeDecl() (r *TypeDecl) {
	t := p.must(TYPE)
	switch p.ch() {
	case '(':
		return &TypeDecl{TypeTok: t, LParen: p.shift(), TypeSpecs: p.typeSpecs(), RParen: p.must(')'), Semicolon: p.semi(true)}
	//                  TypeSpec
	case IDENTIFIER:
		return &TypeDecl{TypeTok: t, TypeSpecs: []Node{p.typeSpec(false)}, Semicolon: p.semi(true)}
	default:
		p.err(errorf("TODO %v", p.s.Tok.Ch.str()))
		p.shift()
		return r
	}
}

func (p *parser) typeSpecs() (r []Node) {
	//                  TypeSpec case IDENTIFIER:
	for p.ch() == IDENTIFIER {
		r = append(r, p.typeSpec(true))
	}
	return r
}

// TypeSpec = AliasDecl | TypeDef .
// AliasDecl = identifier "=" Type .
// TypeDef = identifier TypeDef_1 Type .
// TypeDef_1 =
//
//	| TypeParameters .
//
// TypeParameters = "[" TypeParamList TypeParameters_1 "]" .
// TypeParameters_1 =
//
//	| "," .
func (p *parser) typeSpec(semi bool) (r Node) {
	switch p.ch() {
	case IDENTIFIER:
		id := p.shift()
		// identifier .
		switch p.ch() {
		case '=':
			return &AliasDecl{lexicalScoper: newLexicalScoper(p.lexicalScope), Ident: id, Eq: p.shift(), TypeNode: p.type1(), Semicolon: p.semi(semi)}
		case '[':
			lbracket := p.shift()
			// identifier "[" .
			switch p.ch() {
			case ']':
				// identifier "[" . "]"
				return &TypeDef{lexicalScoper: newLexicalScoper(p.lexicalScope), Ident: id, TypeNode: &SliceTypeNode{LBracket: lbracket, RBracket: p.shift(), ElementType: p.type1()}, Semicolon: p.semi(semi)}
			case ELLIPSIS:
				// identifier "[" . "..."
				return &TypeDef{lexicalScoper: newLexicalScoper(p.lexicalScope), Ident: id, TypeNode: &ArrayTypeNode{LBracket: lbracket, Ellipsis: p.shift(), RBracket: p.must(']'), ElementType: p.type1()}, Semicolon: p.semi(semi)}
			default:
				expr := p.expression(nil)
				// identifier "[" expression .
				switch p.ch() {
				case ']':
					// identifier "[" expression . "]"
					return &TypeDef{lexicalScoper: newLexicalScoper(p.lexicalScope), Ident: id, TypeNode: &ArrayTypeNode{LBracket: lbracket, ArrayLength: expr.(Expression), RBracket: p.shift(), ElementType: p.type1()}, Semicolon: p.semi(semi)}
				default:
					switch x := expr.(type) {
					case Token:
						return &TypeDef{lexicalScoper: newLexicalScoper(p.lexicalScope), Ident: id, TypeParameters: p.typeParameters2(lbracket, x), TypeNode: p.type1(), Semicolon: p.semi(semi)}
					case *Ident:
						return &TypeDef{lexicalScoper: newLexicalScoper(p.lexicalScope), Ident: id, TypeParameters: p.typeParameters2(lbracket, x.Token), TypeNode: p.type1(), Semicolon: p.semi(semi)}
					default:
						p.err(errorf("TODO %v", p.s.Tok.Ch.str()))
						p.shift()
						return r
					}
				}
			}
			//                      Type case '(', '*', '[', ARROW, CHAN, FUNC, IDENTIFIER, INTERFACE, MAP, STRUCT:
		case '(', '*', ARROW, CHAN, FUNC, IDENTIFIER, INTERFACE, MAP, STRUCT:
			return &TypeDef{lexicalScoper: newLexicalScoper(p.lexicalScope), Ident: id, TypeNode: p.type1(), Semicolon: p.semi(semi)}
		default:
			p.err(errorf("TODO %v", p.s.Tok.Ch.str()))
			p.shift()
			return r
		}
	default:
		p.err(errorf("TODO %v", p.s.Tok.Ch.str()))
		p.shift()
		return r
	}
}

// TypeParameters = "[" TypeParamList TypeParameters_1 "]" .
// TypeParameters_1 =
//
//	| "," .
func (p *parser) typeParameters() (r *TypeParameters) {
	return &TypeParameters{LBracket: p.must('['), TypeParamList: p.typeParamDecls(), RBracket: p.must(']')}
}

// TypeParamDecl = IdentifierList TypeConstraint .
func (p *parser) typeParamDecls() (r []*TypeParamDecl) {
	for {
		switch p.ch() {
		//            IdentifierList
		case IDENTIFIER:
			r = append(r, &TypeParamDecl{IdentifierList: p.identifierList(), TypeConstraint: p.typeElem(false), Comma: p.opt(',')})
		default:
			return r
		}
	}
}

// identifier "[" identifier .
func (p *parser) typeParameters2(lbracket, id Token) (r *TypeParameters) {
	return &TypeParameters{LBracket: lbracket, TypeParamList: p.typeParamDecls2(p.identifierList2(id)), RBracket: p.must(']')}
}

func (p *parser) typeParamDecls2(il []*IdentListItem) (r []*TypeParamDecl) {
	r = []*TypeParamDecl{{IdentifierList: il, TypeConstraint: p.typeElem(false), Comma: p.opt(',')}}
	for {
		switch p.ch() {
		//            IdentifierList
		case IDENTIFIER:
			r = append(r, &TypeParamDecl{IdentifierList: p.identifierList(), TypeConstraint: p.typeElem(false), Comma: p.opt(',')})
		default:
			return r
		}
	}
}

// MethodDecl = "func" Receiver MethodName Signature MethodDecl_1 .
// MethodDecl_1 =
//
//	| FunctionBody .
func (p *parser) methodDecl(f Token) (r *MethodDecl) {
	defer func(n Node) { p.container = n }(p.container)
	r = &MethodDecl{Func: f, Receiver: p.parameters(), MethodName: p.must(IDENTIFIER)}
	p.container = r
	switch p.ch() {
	//                 Signature
	case '(':
		r.Signature = p.signature()
	//            TypeParameters case '[':
	default:
		p.err(errorf("TODO %v", p.s.Tok.Ch.str()))
		p.shift()
		return r
	}
	if p.ch() == '{' {
		r.FunctionBody = p.block(false)
	}
	r.Semicolon = p.semi(true)
	return r
}

// FunctionDecl = "func" FunctionName TypeParameters Signature FunctionDecl_1
//
//	| "func" FunctionName Signature FunctionDecl_2 .
//
// FunctionDecl_1 =
//
//	| FunctionBody .
//
// FunctionDecl_2 =
//
//	| FunctionBody .
func (p *parser) functionDecl(f Token) (r *FunctionDecl) {
	defer func(n Node) { p.container = n }(p.container)
	r = &FunctionDecl{Func: f, FunctionName: p.must(IDENTIFIER)}
	p.container = r
	// trc("%v: FUNC %q in scope %p(%v)", r.Position(), r.FunctionName.Src(), p.lexicalScope, p.lexicalScope.IsPackage())
	switch p.ch() {
	//                 Signature
	case '(':
		r.Signature = p.signature()
		//            TypeParameters
	case '[':
		r.TypeParameters = p.typeParameters()
		r.Signature = p.signature()
	default:
		p.err(errorf("TODO %v", p.s.Tok.Ch.str()))
		p.shift()
		return r
	}
	if p.ch() == '{' {
		r.FunctionBody = p.block(false)
	}
	r.Semicolon = p.semi(true)
	return r
}

// Block = "{" StatementList "}" .
func (p *parser) block(semi bool) (r *Block) {
	var s Scope
	defer p.pushScope(&s)()
	return &Block{Scope: &s, LBrace: p.must('{'), StatementList: p.statementList(), RBrace: p.must('}'), Semicolon: p.semi(semi)}
}

// Signature = Parameters Signature_1 .
// Signature_1 =
//
//	| Result .
func (p *parser) signature() (r *Signature) {
	r = &Signature{Parameters: p.parameters()}
	switch p.ch() {
	//                    Result
	case '(', '*', '[', ARROW, CHAN, FUNC, IDENTIFIER, INTERFACE, MAP, STRUCT:
		r.Result = p.result()
		return r
	default:
		return r
	}
}

// Result = Parameters | Type .
func (p *parser) result() (r Node) {
	switch p.ch() {
	//                      Type case '(', '*', '[', ARROW, CHAN, FUNC, IDENTIFIER, INTERFACE, MAP, STRUCT:
	//                Parameters case '(':
	case '*', '[', ARROW, CHAN, FUNC, IDENTIFIER, INTERFACE, MAP, STRUCT:
		return p.type1()
	case '(':
		return p.parameters()
	default:
		p.err(errorf("TODO %v", p.s.Tok.Ch.str()))
		p.shift()
		return r
	}
}

// Parameters = "(" Parameters_1 ")" .
// Parameters_1 =
//
//	| ParameterList Parameters_1_1 .
//
// Parameters_1_1 =
//
//	| "," .
func (p *parser) parameters() (r *Parameters) {
	return &Parameters{LParen: p.must('('), ParameterList: p.parameterList(), Comma: p.opt(','), RParen: p.must(')')}
}

// ParameterDecl = identifier "..." Type
//
//	| identifier Type
//	| "..." Type
//	| Type .
func (p *parser) parameterList() (r []*ParameterDecl) {
	defer func() {
		if len(r) == 0 {
			return
		}

		var hasNames, hasTypes bool
		for _, v := range r {
			if len(v.IdentifierList) != 0 {
				hasNames = true
			} else {
				hasTypes = true
			}
		}
		if hasNames == !hasTypes {
			return
		}

		// At least one ParameterDecl in r has an identifier and at least one other
		// does not. Example
		//
		//	(a, b c)
		//       00 111 <- r index
		//
		// Find the 'b c' parts and join with all preceding "type name only" to form '[a, b] c'.

		pos := r[0]
		var r2 []*ParameterDecl
		var names []*IdentListItem
		var done bool
		for len(r) != 0 {
			pd := r[0]
			r = r[1:]
			if len(pd.IdentifierList) == 0 { // a
				if pd.Ellipsis.IsValid() {
					p.errNode(pd.Ellipsis, errorf("TODO ..."))
					return
				}

				switch x := pd.Type.(type) {
				case Token:
					names = append(names, &IdentListItem{Ident: x, Comma: pd.Comma})
				case *Ident:
					names = append(names, &IdentListItem{Ident: x.Token, Comma: pd.Comma})
				default:
					p.errNode(x, errorf("TODO %T", x))
				}
				done = false
				continue
			}

			// b c
			pd.IdentifierList = append(names, pd.IdentifierList...)
			names = nil
			r2 = append(r2, pd)
			done = true
		}
		if !done {
			p.errNode(pos, errorf("TODO"))
		}
		r = r2
	}()

	for {
		switch p.ch() {
		case IDENTIFIER:
			id := p.shift()
			// identifier .
			switch p.ch() {
			case '(', '*', ARROW, CHAN, FUNC, IDENTIFIER, INTERFACE, MAP, STRUCT:
				r = append(r, &ParameterDecl{IdentifierList: []*IdentListItem{{Ident: id}}, Type: p.type1(), Comma: p.opt(',')})
			case ELLIPSIS:
				r = append(r, &ParameterDecl{IdentifierList: []*IdentListItem{{Ident: id}}, Ellipsis: p.shift(), Type: p.type1(), Comma: p.opt(',')})
			case ',':
				r = append(r, &ParameterDecl{Type: &Ident{lexicalScoper: newLexicalScoper(p.lexicalScope), Token: id}, Comma: p.shift()})
			case ')':
				return append(r, &ParameterDecl{Type: &Ident{lexicalScoper: newLexicalScoper(p.lexicalScope), Token: id}})
			case '.':
				r = append(r, &ParameterDecl{Type: p.typeName2(&TypeNameNode{Name: &QualifiedIdent{lexicalScoper: newLexicalScoper(p.lexicalScope), PackageName: id, Dot: p.shift(), Ident: p.must(IDENTIFIER)}}), Comma: p.opt(',')})
			case '[':
				lbracket := p.shift()
				// identifier "[" .
				switch p.ch() {
				case ']':
					// identifier "[" . "]"
					r = append(r, &ParameterDecl{IdentifierList: []*IdentListItem{{Ident: id}}, Type: &SliceTypeNode{LBracket: lbracket, RBracket: p.must(']'), ElementType: p.type1()}, Comma: p.opt(',')})

				default:
					switch x := p.exprOrType().(type) {
					case typeNode:
						p.err(errorf("TODO %v", p.ch().str()))
						p.shift()
						return r
					default:
						// identifier "[" expression .
						switch p.ch() {
						case ']':
							rbracket := p.shift()
							// identifier "[" expression "]" .
							switch p.ch() {
							case ')':
								// identifier "[" expression "]" . ")"
								return append(r, &ParameterDecl{Type: &TypeNameNode{Name: &QualifiedIdent{lexicalScoper: newLexicalScoper(p.lexicalScope), Ident: id}, TypeArgs: &TypeArgs{LBracket: lbracket, TypeList: []*TypeListItem{{Type: x}}, RBracket: rbracket}}, Comma: p.opt(',')})
							//                      Type
							case '(', '*', '[', ARROW, CHAN, FUNC, IDENTIFIER, INTERFACE, MAP, STRUCT:
								// identifier "[" expression "]" . Type
								r = append(r, &ParameterDecl{IdentifierList: []*IdentListItem{{Ident: id}}, Type: &ArrayTypeNode{LBracket: lbracket, ArrayLength: x.(Expression), RBracket: rbracket, ElementType: p.type1()}, Comma: p.opt(',')})
							default:
								p.err(errorf("TODO %v", p.ch().str()))
								p.shift()
								return r
							}
						case ',':
							// identifier "[" expression . ","
							r = append(r, &ParameterDecl{Type: p.typeName2(&TypeNameNode{Name: &QualifiedIdent{lexicalScoper: newLexicalScoper(p.lexicalScope), Ident: id}, TypeArgs: p.typeArgs2(lbracket, x)})})
						default:
							p.err(errorf("TODO %v", p.ch().str()))
							p.shift()
							return r
						}
					}
				}
			default:
				p.err(errorf("TODO %v", p.ch().str()))
				p.shift()
				return r
			}
		case ELLIPSIS:
			r = append(r, &ParameterDecl{Ellipsis: p.shift(), Type: p.type1(), Comma: p.opt(',')})
		//                      Type case '(', '*', '[', ARROW, CHAN, FUNC, IDENTIFIER, INTERFACE, MAP, STRUCT:
		case '(', '*', '[', ARROW, CHAN, FUNC, INTERFACE, MAP, STRUCT:
			r = append(r, &ParameterDecl{Type: p.type1(), Comma: p.opt(',')})
		default:
			return r
		}
	}
}

// VarDecl = "var" VarSpec
//
//	| "var" "(" VarDecl_1 ")" .
//
// VarDecl_1 =
//
//	| VarDecl_1 VarSpec ";" .
func (p *parser) varDecl() (r *VarDecl) {
	//                   VarDecl case VAR:
	v := p.must(VAR)
	switch p.ch() {
	case '(':
		return &VarDecl{lexicalScoper: newLexicalScoper(p.lexicalScope), Var: v, LParen: p.shift(), VarSpecs: p.varSpecs(), RParen: p.must(')'), Semicolon: p.semi(true)}
	//                   VarSpec
	case IDENTIFIER:
		return &VarDecl{lexicalScoper: newLexicalScoper(p.lexicalScope), Var: v, VarSpecs: []*VarSpec{p.varSpec(false)}, Semicolon: p.semi(true)}
	default:
		p.err(errorf("TODO %v", p.s.Tok.Ch.str()))
		p.shift()
		return r
	}
}

func (p *parser) varSpecs() (r []*VarSpec) {
	for {
		switch p.ch() {
		//                   VarSpec
		case IDENTIFIER:
			r = append(r, p.varSpec(true))
		default:
			return r
		}
	}
}

// VarSpec = IdentifierList Type VarSpec_1
//
//	| IdentifierList "=" ExpressionList .
//
// VarSpec_1 =
//
//	| "=" ExpressionList .
func (p *parser) varSpec(semi bool) (r *VarSpec) {
	switch p.ch() {
	//                   VarSpec
	case IDENTIFIER:
		r = &VarSpec{IdentifierList: p.identifierList()}
		switch p.ch() {
		case '=':
			r.Eq = p.shift()
			r.ExprList = p.expressionList()
		//                      Type
		case '(', '*', '[', ARROW, CHAN, FUNC, IDENTIFIER, INTERFACE, MAP, STRUCT:
			r.Type = p.type1()
			switch p.ch() {
			case '=':
				r.Eq = p.shift()
				r.ExprList = p.expressionList()
			}
		default:
			p.err(errorf("TODO %v", p.s.Tok.Ch.str()))
			p.shift()
			return r
		}
	default:
		p.err(errorf("TODO %v", p.s.Tok.Ch.str()))
		p.shift()
		return r
	}
	r.Semicolon = p.semi(semi)
	return r
}

// IdentifierList = identifier IdentifierList_1 .
// IdentifierList_1 =
//
//	| IdentifierList_1 "," identifier .
func (p *parser) identifierList() (r []*IdentListItem) {
	for p.ch() == IDENTIFIER {
		n := &IdentListItem{Ident: p.shift()}
		r = append(r, n)
		switch p.ch() {
		case ',':
			n.Comma = p.shift()
		default:
			return r
		}
	}
	return r
}

// identifier .
func (p *parser) identifierList2(id Token) (r []*IdentListItem) {
	n := &IdentListItem{Ident: id}
	r = append(r, n)
	if p.ch() != ',' {
		return r
	}

	n.Comma = p.shift()
	return append(r, p.identifierList()...)
}

// ImportDecl = "import" ImportSpec
//
//	| "import" "(" ImportDecl_1 ")" .
//
// ImportDecl_1 =
//
//	| ImportDecl_1 ImportSpec ";" .
func (p *parser) importDecls() (r []*ImportDecl) {
	for {
		switch p.ch() {
		//                ImportDecl
		case IMPORT:
			im := p.shift()
			switch p.ch() {
			case '(':
				r = append(r, &ImportDecl{Import: im, LParen: p.shift(), ImportSpecs: p.importSpecs(), RParen: p.must(')'), Semicolon: p.semi(true)})
			//                ImportSpec
			case '.', IDENTIFIER, STRING_LIT:
				r = append(r, &ImportDecl{Import: im, ImportSpecs: []*ImportSpec{p.importSpec(false)}, Semicolon: p.semi(true)})
			default:
				p.err(errorf("TODO %v", p.s.Tok.Ch.str()))
				p.shift()
				return r
			}
		default:
			return r
		}
	}
}

func (p *parser) importSpecs() (r []*ImportSpec) {
	for {
		switch p.ch() {
		//                ImportSpec
		case '.', IDENTIFIER, STRING_LIT:
			r = append(r, p.importSpec(true))
		default:
			return r
		}
	}
}

// ImportSpec = "." ImportPath
//
//	| PackageName ImportPath
//	| ImportPath .
func (p *parser) importSpec(semi bool) (r *ImportSpec) {
	//                ImportSpec case '.', IDENTIFIER, STRING_LIT:
	switch p.ch() {
	case '.', IDENTIFIER:
		return &ImportSpec{Qualifier: p.shift(), ImportPath: p.shift(), Semicolon: p.semi(semi)}
	case STRING_LIT:
		return &ImportSpec{ImportPath: p.shift(), Semicolon: p.semi(semi)}
	default:
		p.err(errorf("TODO %v", p.s.Tok.Ch.str()))
		p.shift()
		return r
	}
}

// ------------------------------------------------------------------ Statemens

// StatementList = StatementList_1 StatementList_2 .
// StatementList_1 =
//
//	| StatementList_1 StatementList_1_1 ";" .
//
// StatementList_1_1 =
//
//	| Statement .
//
// StatementList_2 =
//
//	| Statement .
func (p *parser) statementList() (r []Node) {
	for {
		switch p.ch() {
		//                 Statement
		case '!', '&', '(', '*', '+', '-', '[', '^', '{', ARROW, BREAK, CHAN, CONST, CONTINUE, DEFER, FALLTHROUGH, FLOAT_LIT, FOR, FUNC, GO, GOTO, IDENTIFIER, IF, IMAG_LIT, INTERFACE, INT_LIT, MAP, RETURN, RUNE_LIT, SELECT, STRING_LIT, STRUCT, SWITCH, TYPE, VAR:
			r = append(r, p.statement())
		case ';':
			r = append(r, &EmptyStmt{Semicolon: p.shift()})
		default:
			return r
		}
	}
}

func (p *parser) statement() (r Node) {
	switch p.ch() {
	case IDENTIFIER:
		switch x := p.exprOrSimpleStmt(false).(type) {
		case simpleStmt:
			x.semi(p)
			return x
		case nil:
			p.err(errorf("TODO %v", p.s.Tok.Ch.str()))
			p.shift()
			return r
		default:
			switch p.ch() {
			case ':':
				var id Token
				switch y := x.(type) {
				case Token:
					id = y
				case *Ident:
					id = y.Token
				default:
					p.errNode(y, errorf("TODO %v", p.s.Tok.Ch.str()))
					p.shift()
					return r
				}

				colon := p.shift()
				switch p.ch() {
				case '}':
					return &LabeledStmt{Label: id, Colon: colon, Statement: &EmptyStmt{}}
				default:
					return &LabeledStmt{Label: id, Colon: colon, Statement: p.statement()}
				}
			default:
				return &ExpressionStmt{Expr: x.(Expression), Semicolon: p.semi(true)}
			}
		}
	}
	return p.statement2()
}

func (p *parser) statement2() (r Node) {
	switch p.ch() {
	//                Expression
	case '!', '&', '(', '*', '+', '-', '[', '^', ARROW, CHAN, FLOAT_LIT, FUNC, IDENTIFIER, IMAG_LIT, INTERFACE, INT_LIT, MAP, RUNE_LIT, STRING_LIT, STRUCT:
		return p.simpleStmt(true)
	case '{':
		return p.block(true)
	case BREAK:
		return &BreakStmt{Break: p.shift(), Label: p.opt(IDENTIFIER), Semicolon: p.semi(true)}
	case CONST:
		return p.constDecl()
	case CONTINUE:
		return &ContinueStmt{Continue: p.shift(), Label: p.opt(IDENTIFIER), Semicolon: p.semi(true)}
	case DEFER:
		return &DeferStmt{Defer: p.shift(), Expr: p.expression(nil).(Expression), Semicolon: p.semi(true)}
	case FALLTHROUGH:
		return &FallthroughStmt{Fallthrough: p.shift(), Semicolon: p.semi(true)}
	case FOR:
		return p.forStmt()
	case GO:
		return &GoStmt{Go: p.shift(), Expr: p.expression(nil).(Expression), Semicolon: p.semi(true)}
	case GOTO:
		return &GotoStmt{Goto: p.shift(), Label: p.must(IDENTIFIER), Semicolon: p.semi(true)}
	case IF:
		return p.ifStmt(true)
	case RETURN:
		n := &ReturnStmt{Return: p.shift(), container: p.container}
		switch p.ch() {
		//                Expression
		case '!', '&', '(', '*', '+', '-', '[', '^', ARROW, CHAN, FLOAT_LIT, FUNC, IDENTIFIER, IMAG_LIT, INTERFACE, INT_LIT, MAP, RUNE_LIT, STRING_LIT, STRUCT:
			n.ExprList = p.expressionList()
		}
		n.Semicolon = p.semi(true)
		return n
	case SELECT:
		return &SelectStmt{Select: p.shift(), LBrace: p.must(body), CommClauses: p.commClauses(), RBrace: p.must('}'), Semicolon: p.semi(true)}
	case SWITCH:
		return p.switchStmt()
	case TYPE:
		return p.typeDecl()
	case VAR:
		return p.varDecl()
	case ';':
		return &EmptyStmt{Semicolon: p.shift()}
	default:
		p.err(errorf("TODO %v", p.s.Tok.Ch.str()))
		p.shift()
		return r
	}
}

// CommClause = CommCase ":" StatementList .
func (p *parser) commClauses() (r []*CommClause) {
	for {
		switch p.ch() {
		//                  CommCase
		case CASE, DEFAULT:
			r = append(r, &CommClause{CommCase: p.commCase(), Colon: p.must(':'), StatementList: p.statementList()})
		default:
			return r
		}
	}
}

// CommCase = "case" SendStmt
//
//	| "case" RecvStmt
//	| "default" .
//
// RecvStmt   = [ ExpressionList "=" | IdentifierList ":=" ] RecvExpr .
// SendStmt = Channel "<-" Expression .
func (p *parser) commCase() (r *CommCase) {
	switch p.ch() {
	case CASE:
		case1 := p.shift()
		switch p.ch() {
		//                SimpleStmt
		case '!', '&', '(', '*', '+', '-', '[', '^', ARROW, CHAN, FLOAT_LIT, FUNC, IDENTIFIER, IMAG_LIT, INTERFACE, INT_LIT, MAP, RUNE_LIT, STRING_LIT, STRUCT:
			return &CommCase{CaseOrDefault: case1, Statement: p.simpleStmt(false)}
		default:
			p.err(errorf("TODO %v", p.s.Tok.Ch.str()))
			p.shift()
			return r
		}
	case DEFAULT:
		return &CommCase{CaseOrDefault: p.shift()}
	default:
		p.err(errorf("TODO %v", p.s.Tok.Ch.str()))
		p.shift()
		return r
	}
}

func (p *parser) simpleStmt(semi bool) (r Node) {
	switch x := p.exprOrSimpleStmt(semi).(type) {
	case simpleStmt:
		return x
	case nil:
		return nil
	default:
		return &ExpressionStmt{Expr: x.(Expression), Semicolon: p.semi(semi)}
	}
}

// ForStmt = "for" ForClause LoopBody
//
//	| "for" RangeClause LoopBody
//	| "for" Condition LoopBody
//	| "for" LoopBody .
func (p *parser) forStmt() (r *ForStmt) {
	//        Condition case '!', '&', '(', '*', '+', '-', '[', '^', ARROW, CHAN, FLOAT_LIT, FUNC, IDENTIFIER, IMAG_LIT, INTERFACE, INT_LIT, MAP, RUNE_LIT, STRING_LIT, STRUCT:
	//        ForClause case '!', '&', '(', '*', '+', '-', ';', '[', '^', ARROW, CHAN, FLOAT_LIT, FUNC, IDENTIFIER, IMAG_LIT, INTERFACE, INT_LIT, MAP, RUNE_LIT, STRING_LIT, STRUCT:
	//      RangeClause case '!', '&', '(', '*', '+', '-', '[', '^', ARROW, CHAN, FLOAT_LIT, FUNC, IDENTIFIER, IMAG_LIT, INTERFACE, INT_LIT, MAP, RANGE, RUNE_LIT, STRING_LIT, STRUCT:
	var s Scope
	defer p.pushScope(&s)()
	r = &ForStmt{For: p.must(FOR), Scope: &s}
	switch p.ch() {
	// case ';':
	// 	p.err(errorf("TODO %v", p.s.Tok.Ch.str()))
	// 	p.shift()
	// 	return r
	// case RANGE:
	// 	p.err(errorf("TODO %v", p.s.Tok.Ch.str()))
	// 	p.shift()
	// 	return r
	case '!', '&', '(', '*', '+', '-', '[', '^', ARROW, CHAN, FLOAT_LIT, FUNC, IDENTIFIER, IMAG_LIT, INTERFACE, INT_LIT, MAP, RUNE_LIT, STRING_LIT, STRUCT:
		expr := p.expression(nil)
		switch p.ch() {
		case ',':
			comma := p.shift()
			el := append([]*ExprListItem{{Expr: expr.(Expression), Comma: comma}}, p.expressionList()...)
			switch p.ch() {
			case DEFINE:
				def := p.shift()
				switch p.ch() {
				case RANGE:
					r.RangeClause = &RangeClause{ExprList: el, Assign: def, Range: p.shift(), Expr: p.expression(nil).(Expression)}
				//                Expression
				case '!', '&', '(', '*', '+', '-', '[', '^', ARROW, CHAN, FLOAT_LIT, FUNC, IDENTIFIER, IMAG_LIT, INTERFACE, INT_LIT, MAP, RUNE_LIT, STRING_LIT, STRUCT:
					r.ForClause = p.forClause(&ShortVarDecl{lexicalScoper: newLexicalScoper(p.lexicalScope), IdentifierList: p.exprListToIDList(el), Define: def, ExprList: p.expressionList()})
				default:
					p.err(errorf("TODO %v", p.s.Tok.Ch.str()))
					p.shift()
					return r
				}
			case '=', ADD_ASSIGN, AND_ASSIGN, AND_NOT_ASSIGN, MUL_ASSIGN, OR_ASSIGN, QUO_ASSIGN, REM_ASSIGN, SHL_ASSIGN, SHR_ASSIGN, SUB_ASSIGN, XOR_ASSIGN:
				op := p.shift()
				switch p.ch() {
				case RANGE:
					r.RangeClause = &RangeClause{ExprList: el, Assign: op, Range: p.shift(), Expr: p.expression(nil).(Expression)}
				//                Expression
				case '!', '&', '(', '*', '+', '-', '[', '^', ARROW, CHAN, FLOAT_LIT, FUNC, IDENTIFIER, IMAG_LIT, INTERFACE, INT_LIT, MAP, RUNE_LIT, STRING_LIT, STRUCT:
					r.ForClause = p.forClause(&Assignment{LExprList: el, AssOp: op, RExprList: p.expressionList()})
				default:
					p.err(errorf("TODO %v", p.s.Tok.Ch.str()))
					p.shift()
					return r
				}
			default:
				p.err(errorf("TODO %v", p.s.Tok.Ch.str()))
				p.shift()
				return r
			}
		case DEFINE:
			def := p.shift()
			switch p.ch() {
			case RANGE:
				r.RangeClause = &RangeClause{ExprList: []*ExprListItem{{Expr: expr.(Expression)}}, Assign: def, Range: p.shift(), Expr: p.expression(nil).(Expression)}
			default:
				expr2 := p.expression(nil)
				switch p.ch() {
				case ';':
					r.ForClause = p.forClause(&ShortVarDecl{lexicalScoper: newLexicalScoper(p.lexicalScope), IdentifierList: p.exprToIDList(expr), Define: def, ExprList: []*ExprListItem{{Expr: expr2.(Expression)}}})
				default:
					p.err(errorf("TODO %v", p.s.Tok.Ch.str()))
					p.shift()
					return r
				}
			}
		case '=', ADD_ASSIGN, AND_ASSIGN, AND_NOT_ASSIGN, MUL_ASSIGN, OR_ASSIGN, QUO_ASSIGN, REM_ASSIGN, SHL_ASSIGN, SHR_ASSIGN, SUB_ASSIGN, XOR_ASSIGN:
			op := p.shift()
			switch p.ch() {
			case RANGE:
				r.RangeClause = &RangeClause{ExprList: []*ExprListItem{{Expr: expr.(Expression)}}, Assign: op, Range: p.shift(), Expr: p.expression(nil).(Expression)}
			default:
				expr2 := p.expression(nil)
				switch p.ch() {
				case ';':
					r.ForClause = p.forClause(&Assignment{LExprList: []*ExprListItem{{Expr: expr.(Expression)}}, AssOp: op, RExprList: []*ExprListItem{{Expr: expr2.(Expression)}}})
				default:
					p.err(errorf("TODO %v", p.s.Tok.Ch.str()))
					p.shift()
					return r
				}
			}
		case body, '{':
			r.ForClause = &ForClause{Condition: expr.(Expression)}
		case INC, DEC:
			r.ForClause = p.forClause(&IncDecStmt{Expr: expr.(Expression), Op: p.shift()})
		case ';':
			r.ForClause = p.forClause(expr)
		default:
			p.err(errorf("TODO %v", p.s.Tok.Ch.str()))
			p.shift()
			return r
		}
	case '{', body:
		// ok
	case ';':
		r.ForClause = p.forClause(nil)
	case RANGE:
		r.RangeClause = &RangeClause{Range: p.shift(), Expr: p.expression(nil).(Expression)}
	default:
		p.err(errorf("TODO %v", p.s.Tok.Ch.str()))
		p.shift()
		return r
	}
	r.Block = p.loopBody()
	r.Semicolon = p.semi(true)
	return r
}

func (p *parser) forClause(init Node) (r *ForClause) {
	r = &ForClause{InitStmt: init, Semicolon: p.must(';')}
	switch p.ch() {
	case ';':
		// ok
	//                Expression
	case '!', '&', '(', '*', '+', '-', '[', '^', ARROW, CHAN, FLOAT_LIT, FUNC, IDENTIFIER, IMAG_LIT, INTERFACE, INT_LIT, MAP, RUNE_LIT, STRING_LIT, STRUCT:
		r.Condition = p.expression(nil).(Expression)
	default:
		p.err(errorf("TODO %v", p.s.Tok.Ch.str()))
		p.shift()
		return r
	}
	r.Semicolon2 = p.must(';')
	switch p.ch() {
	//                SimpleStmt
	case '!', '&', '(', '*', '+', '-', '[', '^', ARROW, CHAN, FLOAT_LIT, FUNC, IDENTIFIER, IMAG_LIT, INTERFACE, INT_LIT, MAP, RUNE_LIT, STRING_LIT, STRUCT:
		r.PostStmt = p.simpleStmt(false)
	}
	return r
}

// SwitchStmt = ExprSwitchStmt | TypeSwitchStmt .
// ExprSwitchStmt = "switch" ExprSwitchStmt_1 ExprSwitchStmt_2 body ExprSwitchStmt_3 "}" .
// ExprSwitchStmt_1 =
//
//	| SimpleStmt ";" .
//
// ExprSwitchStmt_2 =
//
//	| Expression .
//
// ExprSwitchStmt_3 =
//
//	| ExprSwitchStmt_3 ExprCaseClause .
//
// TypeSwitchStmt = "switch" TypeSwitchStmt_1 TypeSwitchGuard body TypeSwitchStmt_2 "}" .
// TypeSwitchStmt_1 =
//
//	| SimpleStmt ";" .
//
// TypeSwitchStmt_2 =
//
//	| TypeSwitchStmt_2 TypeCaseClause .
func (p *parser) switchStmt() (r Node) {
	var s Scope
	defer p.pushScope(&s)()
	sw := p.must(SWITCH)
	// "switch" .
	switch p.ch() {
	//                Expression case '!', '&', '(', '*', '+', '-', '[', '^', ARROW, CHAN, FLOAT_LIT, FUNC, IDENTIFIER, IMAG_LIT, INTERFACE, INT_LIT, MAP, RUNE_LIT, STRING_LIT, STRUCT:
	//                SimpleStmt case '!', '&', '(', '*', '+', '-', '[', '^', ARROW, CHAN, FLOAT_LIT, FUNC, IDENTIFIER, IMAG_LIT, INTERFACE, INT_LIT, MAP, RUNE_LIT, STRING_LIT, STRUCT:
	case '!', '&', '(', '*', '+', '-', '[', '^', ARROW, CHAN, FLOAT_LIT, FUNC, IDENTIFIER, IMAG_LIT, INTERFACE, INT_LIT, MAP, RUNE_LIT, STRING_LIT, STRUCT:
		switch x := p.exprOrSimpleStmt(false).(type) {
		case simpleStmt:
			switch p.ch() {
			case body:
				switch y := x.(type) {
				case *ShortVarDecl:
					if len(y.ExprList) != 1 {
						p.err(errorf("TODO %v", p.s.Tok.Ch.str()))
						p.shift()
						return r
					}

					switch z := y.ExprList[0].Expr.(type) {
					case *TypeSwitchGuard:
						if len(y.IdentifierList) != 1 {
							p.err(errorf("TODO %v", p.s.Tok.Ch.str()))
							p.shift()
							return r
						}

						// "switch" 'foo := bar.(type)' body
						z.Ident = y.IdentifierList[0].Ident
						z.Define = y.Define
						return &TypeSwitchStmt{Scope: &s, Switch: sw, TypeSwitchGuard: z, LBrace: p.body(), TypeCaseClauses: p.typeCaseClauses(), RBrace: p.must('}'), Semicolon2: p.semi(true)}
					default:
						p.err(errorf("TODO %v", p.s.Tok.Ch.str()))
						p.shift()
						return r
					}
				default:
					p.err(errorf("TODO %v", p.s.Tok.Ch.str()))
					p.shift()
					return r
				}
			case ';':
				semi := p.shift()
				// "switch" foo := bar ";" .
				switch p.ch() {
				case body:
					return &ExpressionSwitchStmt{Scope: &s, Switch: sw, SimpleStmt: x, Semicolon: semi, LBrace: p.body(), ExprCaseClauses: p.exprCaseClauses(), RBrace: p.must('}'), Semicolon2: p.semi(true)}
					//                Expression
				case '!', '&', '(', '*', '+', '-', '[', '^', ARROW, CHAN, FLOAT_LIT, FUNC, IDENTIFIER, IMAG_LIT, INTERFACE, INT_LIT, MAP, RUNE_LIT, STRING_LIT, STRUCT:
					expr := p.expression(nil)
					if y, ok := expr.(*TypeSwitchGuard); ok {
						// "switch" foo := bar ";" Expression.(type) .
						return &TypeSwitchStmt{Scope: &s, Switch: sw, SimpleStmt: x, Semicolon: semi, TypeSwitchGuard: y, LBrace: p.body(), TypeCaseClauses: p.typeCaseClauses(), RBrace: p.must('}'), Semicolon2: p.semi(true)}
					}

					switch p.ch() {
					case DEFINE:
						var tok Token
						switch z := expr.(type) {
						case Token:
							tok = z
						case *Ident:
							tok = z.Token
						default:
							p.err(errorf("TODO %v", p.s.Tok.Ch.str()))
							p.shift()
							return r
						}

						def := p.shift()
						switch y := p.primaryExpression().(type) {
						case *TypeSwitchGuard:
							y.Ident = tok
							y.Define = def
							return &TypeSwitchStmt{Scope: &s, Switch: sw, SimpleStmt: x, Semicolon: semi, TypeSwitchGuard: y, LBrace: p.body(), TypeCaseClauses: p.typeCaseClauses(), RBrace: p.must('}'), Semicolon2: p.semi(true)}
						default:
							p.err(errorf("TODO %v", p.s.Tok.Ch.str()))
							p.shift()
							return r
						}
					case body:
						return &ExpressionSwitchStmt{Scope: &s, Switch: sw, SimpleStmt: x, Semicolon: semi, Expr: expr.(Expression), LBrace: p.body(), ExprCaseClauses: p.exprCaseClauses(), RBrace: p.must('}'), Semicolon2: p.semi(true)}
					default:
						p.err(errorf("TODO %v", p.s.Tok.Ch.str()))
						p.shift()
						return r
					}
				default:
					p.err(errorf("TODO %v", p.s.Tok.Ch.str()))
					p.shift()
					return r
				}
			case '.':
				switch y := x.(type) {
				case *ShortVarDecl:
					p.err(errorf("TODO %v", p.s.Tok.Ch.str()))
					p.shift()
					return r
					//TODO if len(y.IdentifierList) != 1 || len(y.ExpressionList) != 1 {
					//TODO 	p.err(errorf("TODO %v", p.s.Tok.Ch.str()))
					//TODO 	p.shift()
					//TODO 	return r
					//TODO }

					//TODO g := &TypeSwitchGuard{Ident: y.IdentifierList[0].Ident, Define: y.Define, PrimaryExpr: y.ExpressionList[0], Dot: p.shift(), LParen: p.must('('), Type: p.must(TYPE), RParen: p.must(')')}
					//TODO return &TypeSwitchStmt{Scope: &s, Switch: sw, TypeSwitchGuard: g, LBrace: p.body(), TypeCaseClauses: p.typeCaseClauses(), RBrace: p.must('}'), Semicolon2: p.semi(true)}
				default:
					p.err(errorf("TODO %T", y))
					p.shift()
					return r
				}
			default:
				p.err(errorf("TODO %v", p.s.Tok.Ch.str()))
				p.shift()
				return r
			}
		default:
			switch y := x.(type) {
			case *TypeSwitchGuard:
				return &TypeSwitchStmt{Scope: &s, Switch: sw, TypeSwitchGuard: y, LBrace: p.body(), TypeCaseClauses: p.typeCaseClauses(), RBrace: p.must('}'), Semicolon2: p.semi(true)}
			default:
				switch p.ch() {
				case body:
					return &ExpressionSwitchStmt{Scope: &s, Switch: sw, Expr: x.(Expression), LBrace: p.body(), ExprCaseClauses: p.exprCaseClauses(), RBrace: p.must('}'), Semicolon2: p.semi(true)}
				case ';':
					semi := p.shift()
					switch p.ch() {
					case body:
						p.err(errorf("TODO %v", p.s.Tok.Ch.str()))
						p.shift()
						return r
					default:
						switch z := p.expression(nil).(type) {
						case *TypeSwitchGuard:
							return &TypeSwitchStmt{Scope: &s, Switch: sw, SimpleStmt: y, Semicolon: semi, TypeSwitchGuard: z, LBrace: p.body(), TypeCaseClauses: p.typeCaseClauses(), RBrace: p.must('}'), Semicolon2: p.semi(true)}
						default:
							p.err(errorf("TODO %v", p.s.Tok.Ch.str()))
							p.shift()
							return r
						}
					}
				default:
					p.err(errorf("TODO %v", p.s.Tok.Ch.str()))
					p.shift()
					return r
				}
			}
		}
	case body:
		// ExprSwitchStmt = "switch" ExprSwitchStmt_1 ExprSwitchStmt_2 body ExprSwitchStmt_3 "}" .
		// ExprSwitchStmt_1 =
		// 	| SimpleStmt ";" .
		// ExprSwitchStmt_2 =
		// 	| Expression .
		// ExprSwitchStmt_3 =
		// 	| ExprSwitchStmt_3 ExprCaseClause .
		return &ExpressionSwitchStmt{Scope: &s, Switch: sw, LBrace: p.body(), ExprCaseClauses: p.exprCaseClauses(), RBrace: p.must('}'), Semicolon2: p.semi(true)}
	case ';':
		semi := p.shift()
		switch p.ch() {
		case body:
			return &ExpressionSwitchStmt{Scope: &s, Switch: sw, SimpleStmt: &EmptyStmt{}, Semicolon: semi, LBrace: p.body(), ExprCaseClauses: p.exprCaseClauses(), RBrace: p.must('}'), Semicolon2: p.semi(true)}
		default:
			p.err(errorf("TODO %v", p.s.Tok.Ch.str()))
			p.shift()
			return r
		}
	default:
		p.err(errorf("TODO %v", p.s.Tok.Ch.str()))
		p.shift()
		return r
	}
}

// TypeCaseClause = TypeSwitchCase ":" StatementList .
func (p *parser) typeCaseClauses() (r []*TypeCaseClause) {
	for {
		switch p.ch() {
		//            TypeSwitchCase
		case CASE, DEFAULT:
			var s Scope
			defer p.pushScope(&s)()
			r = append(r, &TypeCaseClause{TypeSwitchCase: p.typeSwitchCase(), Colon: p.must(':'), StatementList: p.statementList()})
		default:
			return r
		}
	}
}

// TypeSwitchCase = "case" TypeList
//
//	| "default" .
func (p *parser) typeSwitchCase() (r *TypeSwitchCase) {
	switch p.ch() {
	case CASE:
		return &TypeSwitchCase{CaseOrDefault: p.shift(), TypeList: p.typeList()}
	case DEFAULT:
		return &TypeSwitchCase{CaseOrDefault: p.shift()}
	default:
		p.err(errorf("TODO %v", p.s.Tok.Ch.str()))
		p.shift()
		return r
	}
}

func (p *parser) typeList() (r []*TypeListItem) {
	for {
		switch p.ch() {
		//                      Type
		case '(', '*', '[', ARROW, CHAN, FUNC, IDENTIFIER, INTERFACE, MAP, STRUCT:
			r = append(r, &TypeListItem{Type: p.type1(), Comma: p.opt(',')})
		default:
			return r
		}
	}
}

func (p *parser) typeList2(typ Node) (r []*TypeListItem) {
	if p.ch() != ',' {
		return []*TypeListItem{{Type: typ}}
	}

	r = []*TypeListItem{{Type: typ, Comma: p.shift()}}
	for {
		switch p.ch() {
		//                      Type
		case '(', '*', '[', ARROW, CHAN, FUNC, IDENTIFIER, INTERFACE, MAP, STRUCT:
			r = append(r, &TypeListItem{Type: p.type1(), Comma: p.opt(',')})
		default:
			return r
		}
	}
}

// ExprCaseClause = ExprSwitchCase ":" StatementList .
func (p *parser) exprCaseClauses() (r []*ExprCaseClause) {
	for {
		switch p.ch() {
		//            ExprSwitchCase
		case CASE, DEFAULT:
			var s Scope
			defer p.pushScope(&s)()
			r = append(r, &ExprCaseClause{ExprSwitchCase: p.exprSwitchCase(), Colon: p.must(':'), StatementList: p.statementList()})
		default:
			return r
		}
	}
}

// ExprSwitchCase = "case" ExpressionList
//
//	| "default" .
func (p *parser) exprSwitchCase() (r *ExprSwitchCase) {
	switch p.ch() {
	case CASE:
		return &ExprSwitchCase{CaseOrDefault: p.shift(), ExprList: p.expressionList()}
	case DEFAULT:
		return &ExprSwitchCase{CaseOrDefault: p.shift()}
	default:
		p.err(errorf("TODO %v", p.s.Tok.Ch.str()))
		p.shift()
		return r
	}
}

// IfStmt = "if" IfStmt_1 Expression LoopBody IfStmt_2 .
// IfStmt_1 =
//
//	| SimpleStmt ";" .
//
// IfStmt_2 =
//
//	| "else" IfStmt_2_1 .
//
// IfStmt_2_1 = IfStmt | Block .
func (p *parser) ifStmt(semi bool) (r *IfStmt) {
	//                Expression case '!', '&', '(', '*', '+', '-', '[', '^', ARROW, CHAN, FLOAT_LIT, FUNC, IDENTIFIER, IMAG_LIT, INTERFACE, INT_LIT, MAP, RUNE_LIT, STRING_LIT, STRUCT:
	//                SimpleStmt case '!', '&', '(', '*', '+', '-', '[', '^', ARROW, CHAN, FLOAT_LIT, FUNC, IDENTIFIER, IMAG_LIT, INTERFACE, INT_LIT, MAP, RUNE_LIT, STRING_LIT, STRUCT:
	var s Scope
	defer p.pushScope(&s)()
	r = &IfStmt{If: p.must(IF), Scope: &s}
	switch x := p.exprOrSimpleStmt(false).(type) {
	case simpleStmt:
		r.SimpleStmt = x
		r.Semicolon = p.must(';')
		r.Expr = p.expression(nil).(Expression)
	default:
		switch p.ch() {
		case ';':
			r.SimpleStmt = x
			r.Semicolon = p.shift()
			r.Expr = p.expression(nil).(Expression)
		case body:
			r.Expr = x.(Expression)
		default:
			p.err(errorf("TODO %v", p.s.Tok.Ch.str()))
			p.shift()
			return r
		}
	}
	r.Block = p.loopBody()
	switch p.ch() {
	case ELSE:
		r.Else = p.shift()
		switch p.ch() {
		case '{':
			r.ElsePart = p.block(false)
		case IF:
			r.ElsePart = p.ifStmt(false)
		default:
			p.err(errorf("TODO %v", p.s.Tok.Ch.str()))
			p.shift()
			return r
		}
	}
	r.Semicolon2 = p.semi(semi)
	return r
}

// LoopBody = body StatementList "}" .
func (p *parser) loopBody() (r *Block) {
	return &Block{LBrace: p.body(), StatementList: p.statementList(), RBrace: p.must('}')}
}

func (p *parser) body() (r Token) {
	switch p.ch() {
	case body:
		r = p.shift()
		r.Ch = '{'
	default:
		p.err(errorf("TODO %v", p.s.Tok.Ch.str()))
		p.shift()
	}
	return r
}

func (p *parser) exprOrSimpleStmt(semi bool) (r Node) {
	//                Expression case '!', '&', '(', '*', '+', '-', '[', '^', ARROW, CHAN, FLOAT_LIT, FUNC, IDENTIFIER, IMAG_LIT, INTERFACE, INT_LIT, MAP, RUNE_LIT, STRING_LIT, STRUCT:
	//                SimpleStmt case '!', '&', '(', '*', '+', '-', '[', '^', ARROW, CHAN, FLOAT_LIT, FUNC, IDENTIFIER, IMAG_LIT, INTERFACE, INT_LIT, MAP, RUNE_LIT, STRING_LIT, STRUCT:
	switch p.ch() {
	//                Expression
	case '!', '&', '(', '*', '+', '-', '[', '^', ARROW, CHAN, FLOAT_LIT, FUNC, IDENTIFIER, IMAG_LIT, INTERFACE, INT_LIT, MAP, RUNE_LIT, STRING_LIT, STRUCT:
		expr := p.expression(nil)
		switch p.ch() {
		case body:
			return expr
		case DEFINE:
			return &ShortVarDecl{lexicalScoper: newLexicalScoper(p.lexicalScope), IdentifierList: p.exprListToIDList([]*ExprListItem{{Expr: expr.(Expression)}}), Define: p.shift(), ExprList: p.expressionList(), Semicolon: p.semi(semi)}
		//                 assign_op
		case '=', ADD_ASSIGN, AND_ASSIGN, AND_NOT_ASSIGN, MUL_ASSIGN, OR_ASSIGN, QUO_ASSIGN, REM_ASSIGN, SHL_ASSIGN, SHR_ASSIGN, SUB_ASSIGN, XOR_ASSIGN:
			return &Assignment{LExprList: []*ExprListItem{{Expr: expr.(Expression)}}, AssOp: p.shift(), RExprList: p.expressionList(), Semicolon: p.semi(semi)}
		case ',':
			comma := p.shift()
			el := append([]*ExprListItem{{Expr: expr.(Expression), Comma: comma}}, p.expressionList()...)
			switch p.ch() {
			case DEFINE:
				return &ShortVarDecl{lexicalScoper: newLexicalScoper(p.lexicalScope), IdentifierList: p.exprListToIDList(el), Define: p.shift(), ExprList: p.expressionList(), Semicolon: p.semi(semi)}
			//                 assign_op
			case '=', ADD_ASSIGN, AND_ASSIGN, AND_NOT_ASSIGN, MUL_ASSIGN, OR_ASSIGN, QUO_ASSIGN, REM_ASSIGN, SHL_ASSIGN, SHR_ASSIGN, SUB_ASSIGN, XOR_ASSIGN:
				return &Assignment{LExprList: el, AssOp: p.shift(), RExprList: p.expressionList(), Semicolon: p.semi(semi)}
			default:
				p.err(errorf("TODO %v", p.s.Tok.Ch.str()))
				p.shift()
				return r
			}
		case ';', '}', ']', ':':
			return expr
		case INC, DEC:
			return &IncDecStmt{Expr: expr.(Expression), Op: p.shift(), Semicolon: p.semi(semi)}
		case ARROW:
			return &SendStmt{Channel: expr, Arrow: p.shift(), Expr: p.expression(nil).(Expression), Semicolon: p.semi(semi)}

		case '.':
			// Expression "." "(" TYPE
			return expr
		default:
			p.err(errorf("TODO %v", p.s.Tok.Ch.str()))
			p.shift()
			return invalidExpr
		}
	case ';':
		return &EmptyStmt{Semicolon: p.semi(semi)}
	default:
		p.err(errorf("TODO %v", p.s.Tok.Ch.str()))
		p.shift()
		return invalidExpr
	}
}

func (p *parser) exprToIDList(e Node) (r []*IdentListItem) {
	switch x := e.(type) {
	case Token: //TODO-
		switch x.Ch {
		case IDENTIFIER:
			return []*IdentListItem{{Ident: x}}
		default:
			p.errNode(e, errorf("TODO %v", x))
			return r
		}
	case *Ident:
		return []*IdentListItem{{Ident: x.Token}}
	default:
		p.errNode(e, errorf("TODO %T", x))
		return r
	}
}

func (p *parser) exprListToIDList(l []*ExprListItem) (r []*IdentListItem) {
	for _, v := range l {
		switch x := v.Expr.(type) {
		case *Ident:
			r = append(r, &IdentListItem{Ident: x.Token, Comma: v.Comma})
		default:
			p.errNode(x, errorf("TODO %T", x))
			return r
		}
	}
	return r
}

// ---------------------------------------------------------------------- Types

func (p *parser) type1() (r Node) {
	//                      Type case '(', '*', '[', ARROW, CHAN, FUNC, IDENTIFIER, INTERFACE, MAP, STRUCT:
	switch p.ch() {
	case '(':
		return &ParenType{LParen: p.shift(), TypeNode: p.type1(), RParen: p.must(')')}
	case '*':
		return &PointerTypeNode{Star: p.shift(), BaseType: p.type1()}
	case '[':
		lbracket := p.shift()
		switch p.ch() {
		case ']':
			return &SliceTypeNode{LBracket: lbracket, RBracket: p.shift(), ElementType: p.type1()}
		//                Expression
		case '!', '&', '(', '*', '+', '-', '[', '^', ARROW, CHAN, FLOAT_LIT, FUNC, IDENTIFIER, IMAG_LIT, INTERFACE, INT_LIT, MAP, RUNE_LIT, STRING_LIT, STRUCT:
			return &ArrayTypeNode{LBracket: lbracket, ArrayLength: p.expression(nil).(Expression), RBracket: p.must(']'), ElementType: p.type1()}
		case ELLIPSIS:
			return &ArrayTypeNode{LBracket: lbracket, Ellipsis: p.shift(), RBracket: p.must(']'), ElementType: p.type1()}
		default:
			p.err(errorf("TODO %v", p.s.Tok.Ch.str()))
			p.shift()
			return r
		}
	case ARROW, CHAN:
		return p.channelType()
	case FUNC:
		// FunctionType = "func" Signature .
		return &FunctionTypeNode{Func: p.shift(), Signature: p.signature()}
	case IDENTIFIER:
		return p.typeName()
	case INTERFACE:
		// InterfaceType = "interface" lbrace "#fixlbr" "}"
		// 	| "interface" lbrace InterfaceElem InterfaceType_1 InterfaceType_2 "#fixlbr" "}" .
		var lbr bool
		return &InterfaceTypeNode{Interface: p.shift(), LBrace: p.lbrace(&lbr), InterfaceElems: p.interfaceElems(), RBrace: p.fixlbr(lbr)}
	case MAP:
		return &MapTypeNode{Map: p.shift(), LBracket: p.must('['), KeyType: p.type1(), RBracket: p.must(']'), ElementType: p.type1()}
	case STRUCT:
		// StructType = "struct" lbrace "#fixlbr" "}"
		// 	| "struct" lbrace FieldDecl StructType_1 StructType_2 "#fixlbr" "}" .
		// StructType_1 =
		// 	| StructType_1 ";" FieldDecl .
		// StructType_2 =
		// 	| ";" .
		var lbr bool
		n := &StructTypeNode{Struct: p.shift(), LBrace: p.lbrace(&lbr)}
		if p.ch() == '}' {
			n.RBrace = p.fixlbr(lbr)
			return n
		}

		for {
			switch p.ch() {
			//             EmbeddedField case '*', IDENTIFIER:
			//            IdentifierList case IDENTIFIER:
			case '*', IDENTIFIER:
				n.FieldDecls = append(n.FieldDecls, p.fieldDecl())
			default:
				n.RBrace = p.fixlbr(lbr)
				return n
			}
		}
	default:
		p.err(errorf("TODO %v", p.s.Tok.Ch.str()))
		p.shift()
		return r
	}
}

// ChannelType = "<-" "chan" ElementType
//
//	| "chan" "<-" ElementType
//	| "chan" ElementType .
func (p *parser) channelType() (r *ChannelTypeNode) {
	switch p.ch() {
	case ARROW:
		return &ChannelTypeNode{ArrowPre: p.shift(), Chan: p.shift(), ElementType: p.type1()}
	case CHAN:
		return &ChannelTypeNode{Chan: p.shift(), ArrayPost: p.opt(ARROW), ElementType: p.type1()}
	default:
		p.err(errorf("TODO %v", p.s.Tok.Ch.str()))
		p.shift()
		return r
	}
}

// FieldDecl = IdentifierList Type FieldDecl_1
//
//	| EmbeddedField FieldDecl_2 .
//
// FieldDecl_1 =
//
//	| Tag .
//
// FieldDecl_2 =
//
//	| Tag .
func (p *parser) fieldDecl() (r *FieldDecl) {
	switch p.ch() {
	//             EmbeddedField case '*', IDENTIFIER:
	//            IdentifierList case IDENTIFIER:
	case IDENTIFIER:
		id := p.shift()
		//  identifier .
		switch p.ch() {
		case ',':
			//  identifier . ","
			r = &FieldDecl{IdentifierList: p.identifierList2(id), Type: p.type1()}
		case ';', '}':
			//  identifier . ";"
			r = &FieldDecl{EmbeddedField: &EmbeddedField{TypeName: &TypeNameNode{Name: &QualifiedIdent{lexicalScoper: newLexicalScoper(p.lexicalScope), Ident: id}}}}
		case '.':
			//  identifier . "."
			r = &FieldDecl{EmbeddedField: &EmbeddedField{TypeName: p.typeName2(&TypeNameNode{Name: &QualifiedIdent{lexicalScoper: newLexicalScoper(p.lexicalScope), PackageName: id, Dot: p.shift(), Ident: p.must(IDENTIFIER)}})}}
		case '[':
			lbracket := p.shift()
			//  identifier "[" .
			switch p.ch() {
			case ']':
				//  identifier "[" . "]"
				r = &FieldDecl{IdentifierList: p.identifierList2(id), Type: &SliceTypeNode{LBracket: lbracket, RBracket: p.must(']'), ElementType: p.type1()}}
				// case ']', FLOAT_LIT, IMAG_LIT, INT_LIT, RUNE_LIT, STRING_LIT:
				// 	// . identifier "[" "]"
				// 	r = &FieldDecl{IdentifierList: p.identifierList(), Type: p.type1()}
			case IDENTIFIER:
				// . identifier "[" . identifier
				switch x := p.exprOrType().(type) {
				case typeNode:
					// . identifier "[" Type .
					p.err(errorf("TODO %v", p.s.Tok.Ch.str()))
					p.shift()
					return r
				default:
					// . identifier "[" Expression .
					switch p.ch() {
					case ']':
						rbracket := p.shift()
						// . identifier "[" Expression "]" .
						switch p.ch() {
						case ';', '}':
							// . identifier "[" Expression "]" . ";"
							r = &FieldDecl{EmbeddedField: &EmbeddedField{TypeName: &TypeNameNode{Name: &QualifiedIdent{lexicalScoper: newLexicalScoper(p.lexicalScope), Ident: id}, TypeArgs: &TypeArgs{LBracket: lbracket, TypeList: []*TypeListItem{{Type: x}}, RBracket: rbracket}}}}
						default:
							r = &FieldDecl{IdentifierList: []*IdentListItem{{Ident: id}}, Type: &ArrayTypeNode{LBracket: lbracket, ArrayLength: x.(Expression), RBracket: rbracket, ElementType: p.type1()}}
						}
					case ',':
						// . identifier "[" Expression . ","
						r = &FieldDecl{EmbeddedField: &EmbeddedField{TypeName: &TypeNameNode{Name: &QualifiedIdent{lexicalScoper: newLexicalScoper(p.lexicalScope), Ident: id}, TypeArgs: &TypeArgs{LBracket: lbracket, TypeList: p.typeList2(x), RBracket: p.must(']')}}}}
					default:
						p.err(errorf("TODO %v", p.s.Tok.Ch.str()))
						p.shift()
						return r
					}
				}
			default:
				//  identifier "[" . Expression
				r = &FieldDecl{IdentifierList: p.identifierList2(id), Type: &ArrayTypeNode{LBracket: lbracket, ArrayLength: p.expression(nil).(Expression), RBracket: p.must(']'), ElementType: p.type1()}}
			}
		default:
			r = &FieldDecl{IdentifierList: p.identifierList2(id)}
			switch p.ch() {
			//                      Type
			case '(', '*', '[', ARROW, CHAN, FUNC, IDENTIFIER, INTERFACE, MAP, STRUCT:
				r.Type = p.type1()
			//                       Tag
			case STRING_LIT:
				// ok
			default:
				p.err(errorf("TODO %v", p.s.Tok.Ch.str()))
				p.shift()
				return r
			}
		}
	case '*':
		r = &FieldDecl{EmbeddedField: &EmbeddedField{Star: p.shift(), TypeName: p.typeName()}}
	default:
		p.err(errorf("TODO %v", p.s.Tok.Ch.str()))
		p.shift()
		return r
	}
	if p.ch() == STRING_LIT {
		r.Tag = p.shift()
	}
	r.Semicolon = p.semi(true)
	return r
}

func (p *parser) interfaceElems() (r []Node) {
	for {
		switch p.ch() {
		// InterfaceElem = MethodElem | TypeElem .
		//                MethodElem case IDENTIFIER:
		//                  TypeElem case '(', '*', '[', '~', ARROW, CHAN, FUNC, IDENTIFIER, INTERFACE, MAP, STRUCT:
		case IDENTIFIER:
			id := p.shift()
			// identifier .
			switch p.ch() {
			case '(':
				// identifier . "("
				r = append(r, &MethodElem{MethodName: id, Signature: p.signature(), Semicolon: p.semi(true)})
			case '|':
				// identifier . "|"
				r = append(r, p.typeElem2(&TypeTerm{Type: id, Pipe: p.shift()}, true))
			case ';', '}':
				// identifier . ";"
				r = append(r, p.typeElem2(&TypeTerm{Type: id}, true))
			case '.':
				// identifier . "."
				r = append(r, p.typeElem2(&TypeTerm{Type: p.typeName2(&TypeNameNode{Name: &QualifiedIdent{lexicalScoper: newLexicalScoper(p.lexicalScope), PackageName: id, Dot: p.shift(), Ident: p.must(IDENTIFIER)}})}, true))
			case '[':
				r = append(r, p.typeElem2(&TypeTerm{Type: p.typeName2(&TypeNameNode{Name: &QualifiedIdent{lexicalScoper: newLexicalScoper(p.lexicalScope), Ident: id}})}, true))
			default:
				p.err(errorf("TODO %v", p.ch().str()))
				p.shift()
				return r
			}
		case '(', '*', '[', '~', ARROW, CHAN, FUNC, INTERFACE, MAP, STRUCT:
			r = append(r, p.typeElem(true))
		default:
			return r
		}
	}
}

// TypeElem = TypeTerm TypeElem_1 .
// TypeElem_1 =
//
//	| TypeElem_1 "|" TypeTerm .
func (p *parser) typeElem(semi bool) (r *TypeElem) {
	r = &TypeElem{}
	for {
		switch p.ch() {
		//                  TypeTerm
		case '(', '*', '[', '~', ARROW, CHAN, FUNC, IDENTIFIER, INTERFACE, MAP, STRUCT:
			n := p.typeTerm()
			r.TypeTerms = append(r.TypeTerms, n)
			switch p.ch() {
			case '|':
				n.Pipe = p.shift()
			}
		default:
			r.Semicolon = p.semi(semi)
			return r
		}
	}
}

func (p *parser) typeElem2(typeTerm *TypeTerm, semi bool) (r *TypeElem) {
	if p.ch() == '|' {
		typeTerm.Pipe = p.shift()
	}
	r = &TypeElem{TypeTerms: []*TypeTerm{typeTerm}}
	for {
		switch p.ch() {
		//                  TypeTerm
		case '(', '*', '[', '~', ARROW, CHAN, FUNC, IDENTIFIER, INTERFACE, MAP, STRUCT:
			n := p.typeTerm()
			r.TypeTerms = append(r.TypeTerms, n)
			switch p.ch() {
			case '|':
				n.Pipe = p.shift()
			}
		default:
			r.Semicolon = p.semi(semi)
			return r
		}
	}
}

// TypeTerm = Type | UnderlyingType .
func (p *parser) typeTerm() (r *TypeTerm) {
	switch p.ch() {
	//                      Type
	case '(', '*', '[', ARROW, CHAN, FUNC, IDENTIFIER, INTERFACE, MAP, STRUCT:
		return &TypeTerm{Type: p.type1()}
	//            UnderlyingType
	case '~':
		return &TypeTerm{Tilde: p.shift(), Type: p.type1()}
	default:
		p.err(errorf("TODO %v", p.s.Tok.Ch.str()))
		p.shift()
		return r
	}
}

// TypeName = QualifiedIdent TypeName_1
//
//	| identifier TypeName_2 .
//
// TypeName_1 =
//
//	| TypeArgs .
//
// TypeName_2 =
//
//	| TypeArgs .
func (p *parser) typeName() (r *TypeNameNode) {
	switch p.ch() {
	case IDENTIFIER:
		id := p.shift()
		switch p.ch() {
		case '.':
			r = &TypeNameNode{Name: &QualifiedIdent{lexicalScoper: newLexicalScoper(p.lexicalScope), PackageName: id, Dot: p.shift(), Ident: p.must(IDENTIFIER)}}
		default:
			r = &TypeNameNode{Name: &QualifiedIdent{lexicalScoper: newLexicalScoper(p.lexicalScope), Ident: id}}
		}
	default:
		p.err(errorf("TODO %v", p.s.Tok.Ch.str()))
		p.shift()
		return r
	}
	return p.typeName2(r)
}

func (p *parser) typeName2(in *TypeNameNode) (r *TypeNameNode) {
	r = in
	switch p.ch() {
	//                  TypeArgs
	case '[':
		r.TypeArgs = p.typeArgs()
		return r
	default:
		return r
	}
}

// TypeArgs = "[" TypeList TypeArgs_1 "]" .
// TypeArgs_1 =
//
//	| "," .
func (p *parser) typeArgs() (r *TypeArgs) {
	return &TypeArgs{LBracket: p.must('['), TypeList: p.typeList(), Comma: p.opt(','), RBracket: p.must(']')}
}

func (p *parser) typeArgs2(lbracket Token, typ Node) (r *TypeArgs) {
	return &TypeArgs{LBracket: lbracket, TypeList: p.typeList2(typ), Comma: p.opt(','), RBracket: p.must(']')}
}

// ---------------------------------------------------------------- Expressions

// ExpressionList = Expression ExpressionList_1 .
// ExpressionList_1 =
//
//	| ExpressionList_1 "," Expression .
func (p *parser) expressionList() (r []*ExprListItem) {
	for {
		switch p.ch() {
		//                Expression
		case '!', '&', '(', '*', '+', '-', '[', '^', ARROW, CHAN, FLOAT_LIT, FUNC, IDENTIFIER, IMAG_LIT, INTERFACE, INT_LIT, MAP, RUNE_LIT, STRING_LIT, STRUCT:
			n := &ExprListItem{Expr: p.expression(nil).(Expression)}
			r = append(r, n)
			switch p.ch() {
			case ',':
				n.Comma = p.shift()
			default:
				return r
			}
		default:
			return r
		}
	}
}

// Expression = LogicalAndExpression Expression_1 .
// Expression_1 =
//
//	| Expression_1 "||" LogicalAndExpression .
func (p *parser) expression(expr Node) (r Node) {
	r = p.logicalAndExpression(expr)
	for p.ch() == LOR {
		r = &BinaryExpr{A: r.(Expression), Op: p.shift(), B: p.logicalAndExpression(nil).(Expression)}
	}
	return r
}

// LogicalAndExpression = RelationalExpression LogicalAndExpression_1 .
// LogicalAndExpression_1 =
//
//	| LogicalAndExpression_1 "&&" RelationalExpression .
func (p *parser) logicalAndExpression(expr Node) (r Node) {
	r = p.relationalExpression(expr)
	for p.ch() == LAND {
		r = &BinaryExpr{A: r.(Expression), Op: p.shift(), B: p.relationalExpression(nil).(Expression)}
	}
	return r
}

// RelationalExpression = AdditiveExpression RelationalExpression_1 .
// RelationalExpression_1 =
//
//	| RelationalExpression_1 rel_op AdditiveExpression .
func (p *parser) relationalExpression(expr Node) (r Node) {
	r = p.additiveExpression(expr)
	for {
		switch p.ch() {
		//                    rel_op
		case '<', '>', EQ, GE, LE, NE:
			r = &BinaryExpr{A: r.(Expression), Op: p.shift(), B: p.additiveExpression(nil).(Expression)}
		default:
			return r
		}
	}
}

// AdditiveExpression = MultiplicativeExpression AdditiveExpression_1 .
// AdditiveExpression_1 =
//
//	| AdditiveExpression_1 add_op MultiplicativeExpression .
func (p *parser) additiveExpression(expr Node) (r Node) {
	r = p.multiplicativeExpression(expr)
	for {
		switch p.ch() {
		//                    add_op
		case '+', '-', '^', '|':
			r = &BinaryExpr{A: r.(Expression), Op: p.shift(), B: p.multiplicativeExpression(nil).(Expression)}
		default:
			return r
		}
	}
}

// MultiplicativeExpression = UnaryExpr MultiplicativeExpression_1 .
// MultiplicativeExpression_1 =
//
//	| MultiplicativeExpression_1 mul_op UnaryExpr .
func (p *parser) multiplicativeExpression(expr Node) (r Node) {
	r = expr
	if expr == nil {
		r = p.unaryExpression()
	}
	for {
		switch p.ch() {
		//                    mul_op
		case '%', '&', '*', '/', AND_NOT, SHL, SHR:
			r = &BinaryExpr{A: r.(Expression), Op: p.shift(), B: p.unaryExpression().(Expression)}
		default:
			return r
		}
	}
}

// UnaryExpr = PrimaryExpr
//
//	| unary_op UnaryExpr .
func (p *parser) unaryExpression() (r Node) {
	switch p.ch() {
	//               PrimaryExpr case '(', '*', '[', ARROW, CHAN, FLOAT_LIT, FUNC, IDENTIFIER, IMAG_LIT, INTERFACE, INT_LIT, MAP, RUNE_LIT, STRING_LIT, STRUCT:
	case '(', '[', CHAN, FLOAT_LIT, FUNC, IDENTIFIER, IMAG_LIT, INTERFACE, INT_LIT, MAP, RUNE_LIT, STRING_LIT, STRUCT:
		return p.primaryExpression()
	//                  unary_op case '!', '&', '*', '+', '-', '^', ARROW:
	case '!', '&', '*', '+', '-', '^':
		return &UnaryExpr{Op: p.shift(), Expr: p.unaryExpression().(Expression)}
	case ARROW:
		arrow := p.shift()
		// "<-" .
		switch p.ch() {
		case CHAN:
			p.err(errorf("TODO %v", p.s.Tok.Ch.str()))
			p.shift()
			return invalidExpr
		default:
			return &UnaryExpr{Op: arrow, Expr: p.unaryExpression().(Expression)}
		}
	default:
		p.err(errorf("TODO %v", p.s.Tok.Ch.str()))
		p.shift()
		return invalidExpr
	}
}

// PrimaryExpr = Operand PrimaryExpr_1
//
//	| Conversion PrimaryExpr_2
//	| MethodExpr PrimaryExpr_3 .
//
// PrimaryExpr_1 =
//
//	| PrimaryExpr_1 PrimaryExpr_1_1 .
//
// PrimaryExpr_1_1 = Arguments
//
//	| Index
//	| Selector
//	| Slice
//	| TypeAssertion .
//
// PrimaryExpr_2 =
//
//	| PrimaryExpr_2 PrimaryExpr_2_1 .
//
// PrimaryExpr_2_1 = Arguments
//
//	| Index
//	| Selector
//	| Slice
//	| TypeAssertion .
//
// PrimaryExpr_3 =
//
//	| PrimaryExpr_3 PrimaryExpr_3_1 .
//
// PrimaryExpr_3_1 = Arguments
//
//	| Index
//	| Selector
//	| Slice
//	| TypeAssertion .
func (p *parser) primaryExpression() (r Node) {
	checkForLiteral := false
	switch p.ch() {
	//                   Operand case '(', '*', '[', ARROW, CHAN, FLOAT_LIT, FUNC, IDENTIFIER, IMAG_LIT, INTERFACE, INT_LIT, MAP, RUNE_LIT, STRING_LIT, STRUCT:
	case FLOAT_LIT, INT_LIT, IMAG_LIT, RUNE_LIT, STRING_LIT:
		tok := p.shift()
		v := constant.MakeFromLiteral(tok.Src(), xlat[tok.Ch], 0)
		if v.Kind() == constant.Unknown {
			p.errNode(tok, "invalid literal: %s", tok.Src())
		}
		var typ Type
		switch tok.Ch {
		case FLOAT_LIT:
			typ = UntypedFloatType
		case INT_LIT:
			typ = UntypedIntType
		case IMAG_LIT:
			typ = UntypedComplexType
		case RUNE_LIT:
			typ = UntypedIntType
		case STRING_LIT:
			typ = UntypedStringType
		}
		lit := &BasicLit{typer: newTyper(typ), valuer: newValuer(v), Token: tok}
		lit.guard = checked
		r = lit
	//                Conversion case '(', '*', '[', ARROW, CHAN, FUNC, IDENTIFIER, INTERFACE, MAP, STRUCT:
	//                MethodExpr case '(', '*', '[', ARROW, CHAN, FUNC, IDENTIFIER, INTERFACE, MAP, STRUCT:
	case '(':
		lparen := p.shift()
		switch x := p.exprOrType().(type) {
		case typeNode:
			r = &ParenType{LParen: lparen, TypeNode: x, RParen: p.must(')')}
			switch p.ch() {
			case '(', '.':
				// ok
			default:
				p.err(errorf("TODO %v", p.s.Tok.Ch.str()))
				p.shift()
				return invalidExpr
			}
		default:
			r = &ParenExpr{LParen: lparen, Expr: x.(Expression), RParen: p.must(')')}
		}
	case '*':
		p.err(errorf("TODO %v", p.s.Tok.Ch.str()))
		p.shift()
		return invalidExpr
	case ARROW:
		p.err(errorf("TODO %v", p.s.Tok.Ch.str()))
		p.shift()
		return invalidExpr
	case '[', CHAN, INTERFACE, MAP, STRUCT:
		t := p.type1()
		switch p.ch() {
		case '{', body:
			r = &CompositeLit{LiteralType: t, LiteralValue: p.literalValue1()}
		case '(':
			r = &Conversion{ConvertType: t, LParen: p.shift(), Expr: p.expression(nil).(Expression), Comma: p.opt(','), RParen: p.must(')')}
		case '.':
			r = t
		default:
			p.err(errorf("TODO %v", p.s.Tok.Ch.str()))
			p.shift()
			return invalidExpr
		}
	case FUNC:
		f := p.shift()
		sig := p.signature()
		switch p.ch() {
		case '{', body:
			r = p.functionLit(f, sig)
		case '(':
			r = &FunctionTypeNode{Func: f, Signature: sig}
		default:
			p.err(errorf("TODO %v", p.s.Tok.Ch.str()))
			p.shift()
			return invalidExpr
		}
	case IDENTIFIER:
		checkForLiteral = true
		id := p.shift()
		//  identifier .
		switch p.ch() {
		case '.':
			dot := p.shift()
			//  identifier "." .
			switch p.ch() {
			case IDENTIFIER:
				//  identifier "." . identifier
				r = &QualifiedIdent{lexicalScoper: newLexicalScoper(p.lexicalScope), PackageName: id, Dot: dot, Ident: p.shift()}
			case '(':
				lparen := p.shift()
				//  identifier "." "(" .
				switch p.ch() {
				case TYPE:
					//  identifier "." "(" . "type"
					return &TypeSwitchGuard{PrimaryExpr: &Ident{lexicalScoper: newLexicalScoper(p.lexicalScope), Token: id}, Dot: dot, LParen: lparen, TypeToken: p.shift(), RParen: p.must(')')}
				default:
					//  identifier "." "(" . identifier
					return p.primaryExpression2(&TypeAssertion{PrimaryExpr: &Ident{lexicalScoper: newLexicalScoper(p.lexicalScope), Token: id}, Dot: dot, LParen: lparen, AssertType: p.type1(), RParen: p.must(')')}, false)
				}
			default:
				p.err(errorf("TODO %v", p.s.Tok.Ch.str()))
				p.shift()
				return invalidExpr
			}
		default:
			r = &Ident{lexicalScoper: newLexicalScoper(p.lexicalScope), Token: id}
		}

		// QualifiedIdent .
		switch p.ch() {
		case '[':
			lbracket := p.shift()
			// QualifiedIdent "[" .
			switch p.ch() {
			//                Expression case '!', '&', '(', '*', '+', '-', '[', '^', ARROW, CHAN, FLOAT_LIT, FUNC, IDENTIFIER, IMAG_LIT, INTERFACE, INT_LIT, MAP, RUNE_LIT, STRING_LIT, STRUCT:
			//                      Type case '(', '*', '[', ARROW, CHAN, FUNC, IDENTIFIER, INTERFACE, MAP, STRUCT:
			case '!', '&', '(', '*', '+', '-', '[', '^', ARROW, CHAN, FLOAT_LIT, FUNC, IDENTIFIER, IMAG_LIT, INTERFACE, INT_LIT, MAP, RUNE_LIT, STRING_LIT, STRUCT:
				switch x := p.exprOrType().(type) {
				case typeNode:
					r = &GenericOperand{OperandName: r, TypeArgs: p.typeArgs2(lbracket, x)}
				default:
					// QualifiedIdent "[" Expression .
					switch p.ch() {
					case ']':
						r = &Index{PrimaryExpr: r.(Expression), LBracket: lbracket, Expr: x.(Expression), RBracket: p.shift()}
					case ':':
						r = p.slice2(r, lbracket, x)
					case ',':
						r = &GenericOperand{OperandName: r, TypeArgs: p.typeArgs2(lbracket, x)}
					default:
						p.err(errorf("TODO %v", p.s.Tok.Ch.str()))
						p.shift()
						return invalidExpr
					}
				}
			case ':':
				r = p.slice2(r, lbracket, nil)
			default:
				p.err(errorf("TODO %v", p.s.Tok.Ch.str()))
				p.shift()
				return invalidExpr
			}
		}
	default:
		p.err(errorf("TODO %v", p.s.Tok.Ch.str()))
		p.shift()
		return invalidExpr
	}
	return p.primaryExpression2(r, checkForLiteral)
}

func (p *parser) functionLit(f Token, sig *Signature) (r *FunctionLit) {
	defer func(n Node) { p.container = n }(p.container)
	var lbr bool
	r = &FunctionLit{Func: f, Signature: sig}
	p.container = r
	r.FunctionBody = &Block{LBrace: p.lbrace(&lbr), StatementList: p.statementList(), RBrace: p.fixlbr(lbr)}
	return r
}

func (p *parser) primaryExpression2(pe Node, checkForLiteral bool) (r Node) {
	if checkForLiteral {
		switch p.ch() {
		case '[':
			pe = p.indexOrSlice(pe)
			if _, ok := pe.(*Index); ok && p.ch() == '{' {
				pe = &CompositeLit{LiteralType: pe, LiteralValue: p.literalValue2()}
			}
		case '{':
			pe = &CompositeLit{LiteralType: pe, LiteralValue: p.literalValue2()}
		}
	}
	r = pe
	for {
		switch p.ch() {
		//                 Arguments
		case '(':
			switch x := r.(type) {
			case typeNode:
				r = &Conversion{ConvertType: x, LParen: p.shift(), Expr: p.expression(nil).(Expression), Comma: p.opt(','), RParen: p.must(')')}
			default:
				r = p.arguments(r)
			}
			//                     Index case '[':
			//                     Slice case '[':
		case '[':
			r = p.indexOrSlice(r)
			//                  Selector case '.':
			//             TypeAssertion case '.':
		case '.':
			dot := p.shift()
			switch p.ch() {
			case IDENTIFIER:
				switch x := r.(type) {
				case typeNode:
					r = &MethodExpr{Receiver: x, Dot: dot, Ident: p.shift()}
				default:
					r = &Selector{PrimaryExpr: r.(Expression), Dot: dot, Ident: p.shift()}
				}
			case '(':
				lparen := p.shift()
				switch p.ch() {
				case TYPE:
					return &TypeSwitchGuard{PrimaryExpr: r.(Expression), Dot: dot, LParen: lparen, TypeToken: p.shift(), RParen: p.must(')')}
				default:
					r = &TypeAssertion{PrimaryExpr: r.(Expression), Dot: dot, LParen: lparen, AssertType: p.type1(), RParen: p.must(')')}
				}
			default:
				p.err(errorf("TODO %v", p.s.Tok.Ch.str()))
				p.shift()
				return r
			}
		default:
			return r
		}
	}
}

func (p *parser) indexOrSlice(pe Node) (r Node) {
	if p.ch() != '[' {
		p.err(errorf("TODO %v", p.s.Tok.Ch.str()))
		p.shift()
		return r
	}

	lbracket := p.shift()
	// "[" .
	var expr Node
	switch p.ch() {
	//                Expression
	case '!', '&', '(', '*', '+', '-', '[', '^', ARROW, CHAN, FLOAT_LIT, FUNC, IDENTIFIER, IMAG_LIT, INTERFACE, INT_LIT, MAP, RUNE_LIT, STRING_LIT, STRUCT:
		expr = p.expression(nil)
	}
	if p.ch() == ']' {
		// "[" Expression . "]"
		return &Index{PrimaryExpr: pe.(Expression), LBracket: lbracket, Expr: expr.(Expression), RBracket: p.shift()}
	}
	return p.slice2(pe, lbracket, expr)
}

// PrimaryExpr "[" Expression . ":"
func (p *parser) slice2(peNode Node, lbracket Token, exprNode Node) (r *SliceExpr) {
	var pe, expr Expression
	if peNode != nil {
		pe = peNode.(Expression)
	}
	if exprNode != nil {
		expr = exprNode.(Expression)
	}
	var expr2 Expression
	colon := p.must(':')
	switch p.ch() {
	//                Expression
	case '!', '&', '(', '*', '+', '-', '[', '^', ARROW, CHAN, FLOAT_LIT, FUNC, IDENTIFIER, IMAG_LIT, INTERFACE, INT_LIT, MAP, RUNE_LIT, STRING_LIT, STRUCT:
		expr2 = p.expression(nil).(Expression)
	}
	if p.ch() == ']' {
		// "[" Expression ":" [ Expression ] . "]"
		return &SliceExpr{PrimaryExpr: pe, LBracket: lbracket, Expr: expr, Colon: colon, Expr2: expr2, RBracket: p.shift()}
	}

	return &SliceExpr{PrimaryExpr: pe, LBracket: lbracket, Expr: expr, Colon: colon, Expr2: expr2, Colon2: p.must(':'), Expr3: p.expression(nil).(Expression), RBracket: p.shift()}
}

// Arguments = "(" ")"
//
//	| "(" ExpressionList Arguments_1 Arguments_2 ")"
//	| "(" Type "," ExpressionList Arguments_3 Arguments_4 ")"
//	| "(" Type Arguments_5 Arguments_6 ")" .
//
// Arguments_1 =
//
//	| "..." .
//
// Arguments_2 =
//
//	| "," .
//
// Arguments_3 =
//
//	| "..." .
//
// Arguments_4 =
//
//	| "," .
//
// Arguments_5 =
//
//	| "..." .
//
// Arguments_6 =
//
//	| "," .
func (p *parser) arguments(pe Node) (r *Arguments) {
	r = &Arguments{PrimaryExpr: pe.(Expression), LParen: p.must('(')}
	if p.ch() == ')' {
		r.RParen = p.shift()
		return r
	}

	switch x := p.exprOrType().(type) {
	case typeNode:
		r.TypeArg = x
		switch p.ch() {
		case ',':
			r.Comma = p.shift()
			r.ExprList = p.expressionList()
		case ')':
			// ok
		default:
			p.err(errorf("TODO %v", p.s.Tok.Ch.str()))
			p.shift()
			return r
		}
	default:
		switch p.ch() {
		case ')':
			r.ExprList = []*ExprListItem{{Expr: x.(Expression)}}
			r.RParen = p.shift()
			return r
		case ',':
			comma := p.shift()
			switch p.ch() {
			case ')':
				r.ExprList = []*ExprListItem{{Expr: x.(Expression), Comma: comma}}
			default:
				r.ExprList = append([]*ExprListItem{{Expr: x.(Expression), Comma: comma}}, p.expressionList()...)
			}
		case ELLIPSIS:
			r.ExprList = []*ExprListItem{{Expr: x.(Expression)}}
			r.Ellipsis = p.shift()
		default:
			p.err(errorf("TODO %v", p.s.Tok.Ch.str()))
			p.shift()
			return r
		}
	}
	if p.ch() == ELLIPSIS {
		r.Ellipsis = p.shift()
	}
	if p.ch() == ',' {
		r.Comma2 = p.shift()
	}
	r.RParen = p.must(')')
	return r
}

func (p *parser) exprOrType() (r Node) {
	switch p.ch() {
	//                Expression case '!', '&', '(', '*', '+', '-', '[', '^', ARROW, CHAN, FLOAT_LIT, FUNC, IDENTIFIER, IMAG_LIT, INTERFACE, INT_LIT, MAP, RUNE_LIT, STRING_LIT, STRUCT:
	case '!', '&', '+', '-', '^', FLOAT_LIT, IMAG_LIT, INT_LIT, RUNE_LIT, STRING_LIT:
		return p.expression(nil)
	//                      Type case '(', '*', '[', ARROW, CHAN, FUNC, IDENTIFIER, INTERFACE, MAP, STRUCT:
	case '(':
		lparen := p.shift()
		// "(" .
		switch x := p.exprOrType().(type) {
		case typeNode:
			// "(" Type .
			switch p.ch() {
			case ')':
				r = &ParenType{LParen: lparen, TypeNode: x, RParen: p.shift()}
				switch p.ch() {
				case '(':
					return p.expression(p.primaryExpression2(r, false))
				default:
					p.err(errorf("TODO %v", p.s.Tok.Ch.str()))
					p.shift()
					return invalidExpr
				}
			default:
				p.err(errorf("TODO %v", p.s.Tok.Ch.str()))
				p.shift()
				return invalidExpr
			}
		default:
			// "(" Expression .
			return p.expression(p.primaryExpression2(&ParenExpr{LParen: lparen, Expr: x.(Expression), RParen: p.must(')')}, false))
		}
	case '*':
		star := p.shift()
		// "*" .
		switch x := p.exprOrType().(type) {
		case typeNode:
			return &PointerTypeNode{Star: star, BaseType: x}
		default:
			e0 := x.(Expression)
			e := e0
			ep := &e0
			for {
				switch y := e.(type) {
				case *BinaryExpr:
					e = y.A
					ep = &y.A
				default:
					*ep = &UnaryExpr{Op: star, Expr: y}
					return e0
				}
			}
		}
	case ARROW:
		arrow := p.shift()
		switch p.ch() {
		case CHAN:
			// ChannelType = "<-" "chan" ElementType
			// 	| "chan" "<-" ElementType
			// 	| "chan" ElementType .
			r = &ChannelTypeNode{ArrowPre: arrow, Chan: p.shift(), ElementType: p.type1()}
			switch p.ch() {
			case ')', ',':
				return r
			default:
				p.err(errorf("TODO %v", p.s.Tok.Ch.str()))
				p.shift()
				return invalidExpr
			}
		default:
			return p.expression(&UnaryExpr{Op: arrow, Expr: p.unaryExpression().(Expression)})
		}
	case '[', CHAN, INTERFACE, MAP, STRUCT:
		t := p.type1()
		switch p.ch() {
		case '{', body:
			return p.expression(p.primaryExpression2(&CompositeLit{LiteralType: t, LiteralValue: p.literalValue1()}, false))
		case '(':
			return p.expression(p.primaryExpression2(&Conversion{ConvertType: t, LParen: p.shift(), Expr: p.expression(nil).(Expression), Comma: p.opt(','), RParen: p.must(')')}, false))
		case ',', ')', ']':
			return t
		default:
			p.err(errorf("TODO %v", p.s.Tok.Ch.str()))
			p.shift()
			return invalidExpr
		}
	case FUNC:
		f := p.shift()
		sig := p.signature()
		switch p.ch() {
		case '{', body:
			return p.expression(p.primaryExpression2(p.functionLit(f, sig), false))
		case ')', ']':
			return &FunctionTypeNode{Func: f, Signature: sig}
		default:
			p.err(errorf("TODO %v", p.s.Tok.Ch.str()))
			p.shift()
			return invalidExpr
		}
	case IDENTIFIER:
		id := p.shift()
		//  identifier .
		switch p.ch() {
		case '.':
			dot := p.shift()
			//  identifier "." .
			switch p.ch() {
			case IDENTIFIER:
				//  identifier "." identifier
				r = &QualifiedIdent{lexicalScoper: newLexicalScoper(p.lexicalScope), PackageName: id, Dot: dot, Ident: p.shift()}
			case '(':
				lparen := p.shift()
				//  identifier "." "(" .
				switch p.ch() {
				case TYPE:
					return &TypeSwitchGuard{PrimaryExpr: &Ident{lexicalScoper: newLexicalScoper(p.lexicalScope), Token: id}, Dot: dot, LParen: lparen, TypeToken: p.shift(), RParen: p.must(')')}
				default:
					//  identifier "." "(" . identifier
					return p.expression(p.primaryExpression2(&TypeAssertion{PrimaryExpr: &Ident{lexicalScoper: newLexicalScoper(p.lexicalScope), Token: id}, Dot: dot, LParen: lparen, AssertType: p.type1(), RParen: p.must(')')}, false))
				}
			default:
				p.err(errorf("TODO %v", p.s.Tok.Ch.str()))
				p.shift()
				return invalidExpr
			}
		default:
			r = &Ident{lexicalScoper: newLexicalScoper(p.lexicalScope), Token: id}
		}
		// QualifiedIdent .
		switch p.ch() {
		case '[':
			lbracket := p.shift()
			// QualifiedIdent "[" .
			switch p.ch() {
			//                Expression case '!', '&', '(', '*', '+', '-', '[', '^', ARROW, CHAN, FLOAT_LIT, FUNC, IDENTIFIER, IMAG_LIT, INTERFACE, INT_LIT, MAP, RUNE_LIT, STRING_LIT, STRUCT:
			//                      Type case '(', '*', '[', ARROW, CHAN, FUNC, IDENTIFIER, INTERFACE, MAP, STRUCT:
			case '!', '&', '(', '*', '+', '-', '[', '^', ARROW, CHAN, FLOAT_LIT, FUNC, IDENTIFIER, IMAG_LIT, INTERFACE, INT_LIT, MAP, RUNE_LIT, STRING_LIT, STRUCT:
				switch x := p.exprOrType().(type) {
				case typeNode:
					id := &Ident{lexicalScoper: newLexicalScoper(p.lexicalScope), Token: id}
					ta := &TypeArgs{LBracket: lbracket, TypeList: p.typeList2(x), RBracket: p.must(']')}
					return p.expression(p.primaryExpression2(&GenericOperand{OperandName: id, TypeArgs: ta}, true))
				default:
					// QualifiedIdent "[" Expression .
					switch p.ch() {
					case ']':
						return p.expression(p.primaryExpression2(&Index{PrimaryExpr: r.(Expression), LBracket: lbracket, Expr: x.(Expression), RBracket: p.shift()}, true))
					case ':':
						return p.expression(p.primaryExpression2(p.slice2(r, lbracket, x), false))
					case ',':
						typeArgs := p.typeArgs2(lbracket, x)
						var tn *TypeNameNode
						switch y := r.(type) {
						case *QualifiedIdent:
							tn = &TypeNameNode{Name: y, TypeArgs: typeArgs}
						case Token:
							tn = &TypeNameNode{Name: &QualifiedIdent{lexicalScoper: newLexicalScoper(p.lexicalScope), Ident: y}, TypeArgs: typeArgs}
						case *Ident:
							tn = &TypeNameNode{Name: &QualifiedIdent{lexicalScoper: newLexicalScoper(p.lexicalScope), Ident: y.Token}, TypeArgs: typeArgs}
						default:
							p.err(errorf("TODO %v", p.s.Tok.Ch.str()))
							p.shift()
							return invalidExpr
						}

						switch p.ch() {
						case ')', ']', ',':
							return tn
						case '{':
							return p.expression(p.primaryExpression2(&GenericOperand{OperandName: r, TypeArgs: typeArgs}, true))
						case '(':
							return p.expression(p.primaryExpression2(&GenericOperand{OperandName: r, TypeArgs: typeArgs}, false))
						default:
							p.err(errorf("TODO %v", p.s.Tok.Ch.str()))
							p.shift()
							return invalidExpr
						}
					default:
						p.err(errorf("TODO %v", p.s.Tok.Ch.str()))
						p.shift()
						return invalidExpr
					}
				}
			case ':':
				return p.expression(p.primaryExpression2(p.slice2(r, lbracket, nil), false))
			default:
				p.err(errorf("TODO %v", p.s.Tok.Ch.str()))
				p.shift()
				return invalidExpr
			}
		default:
			return p.expression(p.primaryExpression2(r, true))
		}
	default:
		p.err(errorf("TODO %v", p.s.Tok.Ch.str()))
		p.shift()
		return invalidExpr
	}
}

// LiteralValue1 = lbrace ElementList LiteralValue1_1 "#fixlbr" "}"
//
//	| lbrace "#fixlbr" "}" .
//
// LiteralValue1_1 =
//
//	| "," .
func (p *parser) literalValue1() (r *LiteralValue) {
	var lbr bool
	return &LiteralValue{LBrace: p.lbrace(&lbr), ElementList: p.elementList(), RBrace: p.fixlbr(lbr)}
}

// LiteralValue2 = "{" "}"
//
//	| "{" ElementList LiteralValue2_1 "}" .
//
// LiteralValue2_1 =
//
//	| "," .
func (p *parser) literalValue2() (r *LiteralValue) {
	return &LiteralValue{LBrace: p.must('{'), ElementList: p.elementList(), RBrace: p.must('}')}
}

// ElementList = KeyedElement ElementList_1 .
// ElementList_1 =
//
//	| ElementList_1 "," KeyedElement .
func (p *parser) elementList() (r []*KeyedElement) {
	for p.ch() != '}' {
		switch p.ch() {
		//              KeyedElement
		case '!', '&', '(', '*', '+', '-', '[', '^', '{', ARROW, CHAN, FLOAT_LIT, FUNC, IDENTIFIER, IMAG_LIT, INTERFACE, INT_LIT, MAP, RUNE_LIT, STRING_LIT, STRUCT, body:
			n := p.keyedElement()
			r = append(r, n)
			if p.ch() == ',' {
				n.Comma = p.shift()
			}
		default:
			p.err(errorf("TODO %v", p.s.Tok.Ch.str()))
			p.shift()
			return r
		}
	}
	return r
}

// KeyedElement = Key ":" Element
//
//	| Element .
func (p *parser) keyedElement() (r *KeyedElement) {
	ke := p.keyOrElement()
	switch p.ch() {
	case ':':
		return &KeyedElement{Key: ke, Colon: p.shift(), Element: p.keyOrElement()}
	default:
		return &KeyedElement{Element: ke}
	}
}

// Key = Expression | LiteralValue1 .
// Element = Expression | LiteralValue1 .
func (p *parser) keyOrElement() (r Node) {
	switch p.ch() {
	//                Expression
	case '!', '&', '(', '*', '+', '-', '[', '^', ARROW, CHAN, FLOAT_LIT, FUNC, IDENTIFIER, IMAG_LIT, INTERFACE, INT_LIT, MAP, RUNE_LIT, STRING_LIT, STRUCT:
		return p.expression(nil)
	//             LiteralValue1
	case '{', body:
		return p.literalValue1()
	default:
		p.err(errorf("TODO %v", p.s.Tok.Ch.str()))
		p.shift()
		return r
	}
}
