// Copyright 2022 The Gc Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package gc // import "modernc.org/gc/v2"

import (
	"go/constant"
	"go/token"
	"math"
)

var (
	_ = []Expression{
		(*Arguments)(nil),
		(*BasicLit)(nil),
		(*BinaryExpr)(nil),
		(*CompositeLit)(nil),
		(*Constant)(nil),
		(*Conversion)(nil),
		(*FunctionLit)(nil),
		(*GenericOperand)(nil),
		(*Ident)(nil),
		(*Index)(nil),
		(*MethodExpr)(nil),
		(*ParenExpr)(nil),
		(*QualifiedIdent)(nil),
		(*Selector)(nil),
		(*SliceExpr)(nil),
		(*TypeAssertion)(nil),
		(*TypeSwitchGuard)(nil),
		(*UnaryExpr)(nil),
		(*Variable)(nil),
		(*invalidExprType)(nil),
	}
)

var (
	invalidExpr = &invalidExprType{typer: newTyper(Invalid)}
	unknown     = constant.MakeUnknown()
)

// Expression represents a computation.
type Expression interface {
	checker
	Node
	Type() Type
	Value() constant.Value
	SetValue(constant.Value)
}

type valuer struct{ val constant.Value }

func newValuer(v constant.Value) valuer { return valuer{v} }

// Value implements Expression
func (v valuer) Value() constant.Value {
	if v.val == nil {
		return unknown
	}

	return v.val
}

// SetValue implements Expression
func (v *valuer) SetValue(val constant.Value) { v.val = val }

type invalidExprType struct {
	guard
	typer
	valuer
}

func (n *invalidExprType) Kind() Kind                   { return InvalidKind }
func (n *invalidExprType) check(c *ctx) Node            { return n }
func (n *invalidExprType) Position() (r token.Position) { return r }
func (n *invalidExprType) Source(full bool) []byte      { return []byte("<invalid expression>") }
func (n *invalidExprType) Tokens() []Token              { return nil }

func (c *ctx) convertValue(n Node, v constant.Value, to Type) (r constant.Value) {
	if v.Kind() == constant.Unknown {
		return unknown
	}

	switch to.Kind() {
	case Int:
		w := constant.ToInt(v)
		if w.Kind() == constant.Unknown {
			c.err(n, "cannot convert %s to %s", v, to)
			return unknown
		}

		i64, ok := constant.Int64Val(w)
		if !ok {
			c.err(n, "value %s overflows %s", v, to)
			return unknown
		}

		switch c.checker.GOARCH() {
		case "386", "arm":
			if i64 < math.MinInt32 || i64 > math.MaxInt32 {
				c.err(n, "value %s overflows %s", v, to)
				return unknown
			}
		}
		return w
	case Int8:
		w := constant.ToInt(v)
		if w.Kind() == constant.Unknown {
			c.err(n, "cannot convert %s to %s", v, to)
			return unknown
		}

		i64, ok := constant.Int64Val(w)
		if !ok {
			c.err(n, "value %s overflows %s", v, to)
			return unknown
		}

		if i64 < math.MinInt8 || i64 > math.MaxInt8 {
			c.err(n, "value %s overflows %s", v, to)
			return unknown
		}

		return w
	case Int16:
		w := constant.ToInt(v)
		if w.Kind() == constant.Unknown {
			c.err(n, "cannot convert %s to %s", v, to)
			return unknown
		}

		i64, ok := constant.Int64Val(w)
		if !ok {
			c.err(n, "value %s overflows %s", v, to)
			return unknown
		}

		if i64 < math.MinInt16 || i64 > math.MaxInt16 {
			c.err(n, "value %s overflows %s", v, to)
			return unknown
		}

		return w
	case Int32:
		w := constant.ToInt(v)
		if w.Kind() == constant.Unknown {
			c.err(n, "cannot convert %s to %s", v, to)
			return unknown
		}

		i64, ok := constant.Int64Val(w)
		if !ok {
			c.err(n, "value %s overflows %s", v, to)
			return unknown
		}

		if i64 < math.MinInt32 || i64 > math.MaxInt32 {
			c.err(n, "value %s overflows %s", v, to)
			return unknown
		}

		return w
	case Int64:
		w := constant.ToInt(v)
		if w.Kind() == constant.Unknown {
			c.err(n, "cannot convert %s to %s", v, to)
			return unknown
		}

		if _, ok := constant.Int64Val(w); !ok {
			c.err(n, "value %s overflows %s", v, to)
			return unknown
		}

		return w
	case Uint, Uintptr:
		w := constant.ToInt(v)
		if w.Kind() == constant.Unknown {
			c.err(n, "cannot convert %s to %s", v, to)
			return unknown
		}

		u64, ok := constant.Uint64Val(w)
		if !ok {
			c.err(n, "value %s overflows %s", v, to)
			return unknown
		}

		switch c.checker.GOARCH() {
		case "386", "arm":
			if u64 > math.MaxUint32 {
				c.err(n, "value %s overflows %s", v, to)
				return unknown
			}
		}
		return w
	case Uint8:
		w := constant.ToInt(v)
		if w.Kind() == constant.Unknown {
			c.err(n, "cannot convert %s to %s", v, to)
			return unknown
		}

		u64, ok := constant.Uint64Val(w)
		if !ok {
			c.err(n, "value %s overflows %s", v, to)
			return unknown
		}

		if u64 > math.MaxUint8 {
			c.err(n, "value %s overflows %s", v, to)
			return unknown
		}

		return w
	case Uint16:
		w := constant.ToInt(v)
		if w.Kind() == constant.Unknown {
			c.err(n, "cannot convert %s to %s", v, to)
			return unknown
		}

		u64, ok := constant.Uint64Val(w)
		if !ok {
			c.err(n, "value %s overflows %s", v, to)
			return unknown
		}

		if u64 > math.MaxUint16 {
			c.err(n, "value %s overflows %s", v, to)
			return unknown
		}

		return w
	case Uint32:
		w := constant.ToInt(v)
		if w.Kind() == constant.Unknown {
			c.err(n, "cannot convert %s to %s", v, to)
			return unknown
		}

		u64, ok := constant.Uint64Val(w)
		if !ok {
			c.err(n, "value %s overflows %s", v, to)
			return unknown
		}

		if u64 > math.MaxUint32 {
			c.err(n, "value %s overflows %s", v, to)
			return unknown
		}

		return w
	case Uint64:
		w := constant.ToInt(v)
		if w.Kind() == constant.Unknown {
			c.err(n, "cannot convert %s to %s", v, to)
			return unknown
		}

		if _, ok := constant.Uint64Val(w); !ok {
			c.err(n, "value %s overflows %s", v, to)
			return unknown
		}

		return w
	case Float32, Float64:
		return constant.ToFloat(v)
	case Interface:
		to := to.(*InterfaceType)
		if len(to.Elems) == 0 {
			return unknown
		}
	case Bool:
		if v.Kind() == constant.Bool {
			return v
		}
	}
	c.err(n, errorf("TODO %v %v -> %v", v, v.Kind(), to.Kind()))
	return unknown
}

func (c *ctx) convertType(n Node, from, to Type) {
	if isArithmeticType(from) && isArithmeticType(to) {
		return
	}

	switch from.Kind() {
	case UnsafePointer:
		switch to.Kind() {
		case Pointer, UnsafePointer, Uintptr:
			// ok
		default:
			c.err(n, errorf("cannot convert %s to %s", from, to))
		}
	case UntypedInt, UntypedFloat, UntypedComplex:
		switch {
		case isArithmeticType(to):
			// ok
		default:
			c.err(n, errorf("cannot convert %s to %s", from, to))
		}
	case Pointer, Uintptr:
		switch to.Kind() {
		case UnsafePointer:
			// ok
		default:
			c.err(n, errorf("cannot convert %s to %s", from, to))
		}
	default:
		c.err(n, errorf("TODO %v -> %v", from.Kind(), to.Kind()))
	}
}

func (c *ctx) convert(expr Expression, to Type) constant.Value {
	c.convertType(expr, expr.Type(), to)
	return c.convertValue(expr, expr.Value(), to)
}

func (c *ctx) defaultType(t Type) Type {
	switch t.Kind() {
	case UntypedInt:
		return PredefinedType(Int)
	case UntypedFloat:
		return PredefinedType(Float64)
	case UntypedBool:
		return PredefinedType(Bool)
	case UntypedString:
		return PredefinedType(String)
	case UntypedComplex:
		return PredefinedType(Complex128)
	default:
		return t
	}
}

func (c *ctx) singleType(n Node, t Type) Type {
	switch x := t.(type) {
	case *TupleType:
		if len(x.Types) != 1 {
			c.err(n, errorf("expected a single expression"))
			return Invalid
		}

		return x.Types[0]
	default:
		return t
	}
}
