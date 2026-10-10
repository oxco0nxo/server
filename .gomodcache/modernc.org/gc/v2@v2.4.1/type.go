// Copyright 2022 The Gc Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package gc // import "modernc.org/gc/v2"

import (
	"fmt"
	"go/constant"
	"go/token"
	"strings"
)

// Singleton instances of some compile-time only pseudo types.
var (
	Invalid            Type = &InvalidType{}
	UntypedBoolType    Type = PredefinedType(UntypedBool)
	UntypedComplexType Type = PredefinedType(UntypedComplex)
	UntypedFloatType   Type = PredefinedType(UntypedFloat)
	UntypedIntType     Type = PredefinedType(UntypedInt)
	UntypedNilType     Type = PredefinedType(UntypedNil)
	UntypedStringType  Type = PredefinedType(UntypedString)
)

var (
	_ Type = (*AliasType)(nil)
	_ Type = (*ArrayType)(nil)
	_ Type = (*ChannelType)(nil)
	_ Type = (*FunctionType)(nil)
	_ Type = (*InterfaceType)(nil)
	_ Type = (*InvalidType)(nil)
	_ Type = (*MapType)(nil)
	_ Type = (*PointerType)(nil)
	_ Type = (*SliceType)(nil)
	_ Type = (*StructType)(nil)
	_ Type = (*TupleType)(nil)
	_ Type = PredefinedType(0)
)

// Type is the representation of a Go type.
//
// The dynamic type of a Type is one of
//
//	*AliasType
//	*ArrayType
//	*ChannelType
//	*FunctionType
//	*InterfaceType
//	*InvalidType
//	*InvalidType
//	*MapType
//	*PointerType
//	*SliceType
//	*StructType
//	*TupleType
//	*TypeName
//	PredefinedType
type Type interface {
	fmt.Stringer
	Node
	// Kind returns the specific kind of a type.
	Kind() Kind
}

type checker interface {
	check(c *ctx) Node
	enter(*ctx, Node) bool
	exit()
}

const (
	unchecked guard = iota
	checking
	checked
)

type typer struct {
	guard
	typ Type
}

func newTyper(t Type) typer { return typer{typ: t} }

// Type returns the type of a node or Invalid if the type is
// unknown/undetermined.
func (t typer) Type() Type {
	if t.typ != nil {
		return t.typ
	}

	switch t.guard {
	case unchecked:
		panic(todo("missed type check"))
	case checking:
		panic(todo("internal error: guard == %s", t.guard))
	default:
		return Invalid
	}
}

type guard byte

func (g *guard) enter(c *ctx, n Node) bool {
	if n == nil {
		return false
	}

	switch *g {
	case unchecked:
		*g = checking
		return true
	case checking:
		switch x := n.(type) {
		default:
			c.err(n, "guard.enter %T", x)
			return false
		}

		c.err(n, "type checking loop")
		return false
	case checked:
		return false
	default:
		panic(todo(""))
	}
}

func (g *guard) exit() { *g = checked }

// A Kind represents the specific kind of type that a Type represents. The zero
// Kind is not a valid kind.
type Kind int

// Values of type Kind
const (
	InvalidKind Kind = iota // <invalid type>

	Array          // array
	Bool           // bool
	Chan           // chan
	Complex128     // complex128
	Complex64      // complex64
	Defined        // typename
	Float32        // float32
	Float64        // float64
	Function       // function
	Int            // int
	Int16          // int16
	Int32          // int32
	Int64          // int64
	Int8           // int8
	Interface      // interface
	Map            // map
	Pointer        // pointer
	Slice          // slice
	String         // string
	Struct         // struct
	Tuple          // tuple
	Uint           // uint
	Uint16         // uint16
	Uint32         // uint32
	Uint64         // uint64
	Uint8          // uint8
	Uintptr        // uintptr
	UnsafePointer  // unsafe.Pointer
	UntypedBool    // untyped bool
	UntypedComplex // untyped complex
	UntypedFloat   // untyped float
	UntypedInt     // untyped int
	UntypedNil     // untyped nil
	UntypedString  // untyped string
)

type noSourcer struct{}

// Source implements Node. It returns nil.
func (noSourcer) Source(bool) []byte { return nil }

// Tokens implements Node. It returns nil.
func (noSourcer) Tokens() []Token { return nil }

// InvalidType represents an invalid type.
type InvalidType struct {
	noSourcer
}

// Position implements Node. Position returns a zero value.
func (t *InvalidType) Position() (r token.Position) { return r }

// Kind implements Type.
func (t *InvalidType) Kind() Kind { return InvalidKind }

func (t *InvalidType) String() string { return "<invalid-type>" }

// PredefinedType represents a predefined type.
type PredefinedType Kind

func (n PredefinedType) check(c *ctx) Node     { return n }
func (n PredefinedType) enter(*ctx, Node) bool { return false }
func (n PredefinedType) exit()                 {}

// Position implements Node. Position returns a zero value.
func (t PredefinedType) Position() (r token.Position) { return r }

// Source implements Node. It returns nil.
func (t PredefinedType) Source(full bool) []byte { return nil }

func (t PredefinedType) String() string { return strings.ToLower(t.Kind().String()) }

// Tokens implements Node. It returns nil.
func (t PredefinedType) Tokens() []Token { return nil }

// Kind implements Type.
func (t PredefinedType) Kind() Kind { return Kind(t) }

// ArrayType represents an array type.
type ArrayType struct {
	noSourcer
	node *ArrayTypeNode
	Elem Type
	Len  int64
}

// Kind implements Type.
func (t *ArrayType) Kind() Kind { return Array }

// Position implements Node.
func (t *ArrayType) Position() (r token.Position) { return position(t.node) }

func (t *ArrayType) String() string { return fmt.Sprintf("[%v]%s", t.Len, t.Elem) }

func (n *ArrayTypeNode) check(c *ctx) Node {
	if !n.enter(c, n) {
		return n
	}

	defer n.exit()

	c.checkExpr(&n.ArrayLength)
	switch v := n.ArrayLength.Value(); v.Kind() {
	case constant.Int:
		et := c.checkType(n.ElementType)
		sz, ok := constant.Int64Val(v)
		if !ok || sz < 0 {
			c.err(n.ElementType, "invalid array length: %s", n.ElementType.Source(false))
			n.typ = Invalid
			break
		}

		n.typ = &ArrayType{node: n, Elem: et, Len: sz}
	default:
		c.err(n.ElementType, "invalid array length: %s", n.ElementType.Source(false))
		n.typ = Invalid
	}
	return n
}

// ChanDir represents a channel direction.
type ChanDir int

// Values of type ChanDir.
const (
	SendRecv ChanDir = iota
	SendOnly
	RecvOnly
)

// ChannelType represents a channel type.
type ChannelType struct {
	noSourcer
	node *ChannelTypeNode
	Dir  ChanDir
	Elem Type
}

// Kind implements Type.
func (t *ChannelType) Kind() Kind { return Chan }

// Position implements Node.
func (t *ChannelType) Position() (r token.Position) { return position(t.node) }

func (t *ChannelType) String() string {
	switch t.Dir {
	case SendRecv:
		return fmt.Sprintf("chan %s", t.Elem)
	case SendOnly:
		return fmt.Sprintf("chan<- %s", t.Elem)
	case RecvOnly:
		return fmt.Sprintf("<-chan %s", t.Elem)
	default:
		panic(todo("ChanDir(%d)", t.Dir))
	}
}

// Parameter represents a function input/output paramater.
type Parameter struct {
	typer
	Name string
}

// FunctionType represents a channel type.
type FunctionType struct {
	noSourcer
	node       Node
	Parameters *TupleType
	Result     *TupleType

	IsVariadic bool
}

// Kind implements Type.
func (t *FunctionType) Kind() Kind { return Function }

// Position implements Node.
func (t *FunctionType) Position() (r token.Position) { return position(t.node) }

func (t *FunctionType) String() string {
	var b strings.Builder
	b.WriteString("func(")
	for i, v := range t.Parameters.Types {
		if i != 0 {
			b.WriteString(", ")
		}
		fmt.Fprintf(&b, "%s", v)
	}
	b.WriteByte(')')
	if t.Result != nil {
		b.WriteByte(' ')
		if len(t.Result.Types) > 1 {
			b.WriteByte('(')
		}
		for i, v := range t.Result.Types {
			if i != 0 {
				b.WriteString(", ")
			}
			fmt.Fprintf(&b, "%s", v)
		}
		if len(t.Result.Types) > 1 {
			b.WriteByte(')')
		}
	}
	return b.String()
}

// InterfaceType represents an interface type.
type InterfaceType struct {
	noSourcer
	node  *InterfaceTypeNode
	Elems []Node //TODO
}

// Kind implements Type.
func (t *InterfaceType) Kind() Kind { return Interface }

// Position implements Node.
func (t *InterfaceType) Position() (r token.Position) { return position(t.node) }

func (t *InterfaceType) String() string {
	var b strings.Builder
	b.WriteString("interface{")
	for i := range t.Elems {
		if i != 0 {
			b.WriteString(" ")
		}
		b.WriteString("/*TODO*/")
	}
	b.WriteByte('}')
	return b.String()
}

func (n *InterfaceTypeNode) check(c *ctx) Node {
	if !n.enter(c, n) {
		return n
	}

	defer n.exit()

	t := &InterfaceType{}
	for _, v := range n.InterfaceElems {
		_ = v
		c.err(n, errorf("TODO %T", n))
	}
	n.typ = t
	return n
}

// MapType represents a map type.
type MapType struct {
	noSourcer
	node *MapTypeNode
	Elem Type
	Key  Type
}

// Kind implements Type.
func (t *MapType) Kind() Kind { return Map }

// Position implements Node.
func (t *MapType) Position() (r token.Position) { return position(t.node) }

func (t *MapType) String() string { return fmt.Sprintf("map[%s]%s", t.Key, t.Elem) }

// PointerType represents a pointer type.
type PointerType struct {
	noSourcer
	node Node
	Elem Type
}

func newPointer(pkg *Package, t Type) *PointerType {
	return &PointerType{node: t, Elem: t}
}

// Kind implements Type.
func (t *PointerType) Kind() Kind { return Pointer }

// Position implements Node.
func (t *PointerType) Position() (r token.Position) { return position(t.node) }

func (t *PointerType) String() string { return fmt.Sprintf("*%s", t.Elem) }

func (n *PointerTypeNode) check(c *ctx) Node {
	if !n.enter(c, n) {
		return n
	}

	defer n.exit()

	pushNamed := c.pushNamed
	defer func() { c.pushNamed = pushNamed }()
	c.pushNamed = true
	n.typ = &PointerType{Elem: c.checkType(n.BaseType), node: n}
	return n
}

// SliceType represents a slice type.
type SliceType struct {
	noSourcer
	node *SliceTypeNode
	Elem Type
}

// Kind implements Type.
func (t *SliceType) Kind() Kind { return Slice }

// Position implements Node.
func (t *SliceType) Position() (r token.Position) { return position(t.node) }

func (t *SliceType) String() string { return fmt.Sprintf("[]%s", t.Elem) }

// Field represents a struct field.
type Field struct {
	typer
	Name  string
	index int
}

// NewField returns a newly created struct field.
func NewField(name string, typ Type) *Field { return &Field{typer: newTyper(typ), Name: name} }

// Index returns n's zero-base index.
func (n *Field) Index() int { return n.index }

// StructType represents a struct type.
type StructType struct {
	noSourcer
	node   *StructTypeNode
	Fields []*Field
	m      map[string]*Field
}

// Kind implements Type.
func (t *StructType) Kind() Kind { return Struct }

// Position implements Node.
func (t *StructType) Position() (r token.Position) { return position(t.node) }

func (t *StructType) String() string {
	var b strings.Builder
	b.WriteString("struct{")
	for i, v := range t.Fields {
		if i != 0 {
			b.WriteString("; ")
		}
		fmt.Fprintf(&b, "%s %s", v.Name, v.Type())
	}
	b.WriteByte('}')
	return b.String()
}

// FieldByName returns the field named nm or nil, if no such field exists.
func (t *StructType) FieldByName(nm string) *Field {
	if t.m == nil {
		t.m = map[string]*Field{}
		for _, f := range t.Fields {
			if nm := f.Name; nm != "" && nm != "_" {
				t.m[nm] = f
			}
		}
	}
	return t.m[nm]
}

func (n *StructTypeNode) check(c *ctx) Node {
	t := &StructType{node: n}
	for _, v := range n.FieldDecls {
		switch x := v.(type) {
		case *FieldDecl:
			switch {
			case x.EmbeddedField != nil:
				ef := x.EmbeddedField
				ft := c.checkType(ef.TypeName)
				if ef.Star.IsValid() {
					ft = newPointer(c.pkg, ft)
				}
				t.Fields = append(t.Fields, &Field{typer: newTyper(ft), Name: ef.TypeName.Name.Ident.Src()})
			default:
				ft := newTyper(c.checkType(x.Type))
				for _, id := range x.IdentifierList {
					t.Fields = append(t.Fields, &Field{typer: ft, Name: id.Ident.Src()})
				}
			}
		default:
			c.err(v, errorf("TODO %T", x))
		}
	}
	for i, v := range t.Fields {
		v.index = i
	}
	n.typ = t
	return n
}

// AliasType represents an alias type.
type AliasType struct {
	noSourcer
	typer
	node *AliasDecl
}

// Kind implements Type.
func (t *AliasType) Kind() Kind { return t.Type().Kind() }

// Position implements Node.
func (t *AliasType) Position() (r token.Position) { return position(t.node) }

func (t *AliasType) String() string { return t.node.Ident.Src() }

func (n *AliasDecl) check(c *ctx) Node {
	if !n.enter(c, n) {
		return n
	}

	defer n.exit()

	n.typ = c.setPushNamed().checkType(n.TypeNode)
	return n
}

// TupleType represents an ordered list of types.
type TupleType struct {
	Types []Type
}

// Kind implements Type.
func (t *TupleType) Kind() Kind { return Tuple }

// Position implements Node.
func (t *TupleType) Position() (r token.Position) {
	if len(t.Types) != 0 {
		r = t.Types[0].Position()
	}
	return r
}

// Source implements Node. It returns nil.
func (t *TupleType) Source(bool) []byte { return nil }

// Tokens implements Node. It returns nil.
func (t *TupleType) Tokens() []Token { return nil }

func (t *TupleType) String() string {
	var a []string
	for _, v := range t.Types {
		a = append(a, v.String())
	}
	return fmt.Sprintf("(%s)", strings.Join(a, ", "))
}

func (t *TupleType) isAssignable(c *ctx, to *TupleType) bool {
	if len(t.Types) != len(to.Types) {
		return false
	}

	for i, v := range t.Types {
		if !c.isAssignable(v, v, to.Types[i]) {
			return false
		}
	}

	return true
}
