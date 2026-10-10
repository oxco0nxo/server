// Copyright 2022 The CCGO Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package ccgo // import "modernc.org/ccgo/v4/lib"

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"math/big"
	"strconv"
	"strings"

	"modernc.org/cc/v4"
	"modernc.org/mathutil"
)

type mode int

const (
	_           mode = iota
	exprBool         // C scalar type, Go bool
	exprCall         // C func pointer, Go function value
	exprDefault      //
	exprIndex        // C pointer, Go array
	exprLvalue       //
	exprSelect       // C struct, Go struct
	exprUintptr      // C pointer, Go uintptr
	exprVoid         // C void, no Go equivalent
)

const (
	ccgoFP = "__ccgo_fp"
	ccgoTS = "__ccgo_ts"
)

var (
	//lint:ignore U1000 debug
	indentLevel int
)

//lint:ignore U1000 debug
func indent() (r string) {
	r = strings.Repeat("· ", indentLevel)
	indentLevel++
	return r
}

//lint:ignore U1000 debug
func undent() (r string) {
	indentLevel--
	return strings.Repeat("· ", indentLevel)
}

func (c *ctx) nonTopExpr(w writer, n cc.ExpressionNode, to cc.Type, toMode mode) *buf {
	c.exprNestLevel++

	defer func() { c.exprNestLevel-- }()

	return c.expr(w, n, to, toMode)
}

func (c *ctx) topExpr(w writer, n cc.ExpressionNode, to cc.Type, toMode mode) *buf {
	sv := c.exprNestLevel

	defer func() { c.exprNestLevel = sv }()

	c.exprNestLevel = 0
	return c.expr(w, n, to, toMode)
}

func (c *ctx) expr(w writer, n cc.ExpressionNode, to cc.Type, toMode mode) (ret *buf) {
	// trc("\t%s%v: %s from=%v to=%v toMode=%v", indent(), n.Position(), cc.NodeSource(n), n.Type(), to, toMode)
	// defer func() {
	// 	trc("\t%s%v: %s from=%v to=%v toMode=%v -> ret=%s", undent(), n.Position(), cc.NodeSource(n), n.Type(), to, toMode, ret)
	// }()
	if toMode == 0 {
		c.err(errorf("internal error"))
		return &buf{}
	}

	if n == nil {
		if toMode != exprVoid {
			c.err(errorf("%v: TODO", pos(n)))
		}
		return &buf{}
	}

	if x, ok := n.(*cc.ExpressionList); ok && x == nil {
		if toMode != exprVoid {
			c.err(errorf("%v: TODO", pos(n)))
		}
		return &buf{}
	}

	if to == nil {
		to = n.Type()
	}
	// trc("%v: %q EXPR  pre call EXPR0 -> %s %s (%s) %T, isVolatileOrAtomicExpr %v", c.pos(n), cc.NodeSource(n), to, toMode, cc.NodeSource(n), n, c.isVolatileOrAtomicExpr(n))
	r, from, fromMode := c.expr0(w, n, to, toMode)
	// trc("%v: %q EXPR post call EXPR0 from %v %v -> to %v %v (%s) %T, r.volatileOrAtomicHandled %v r=%q", c.pos(n), cc.NodeSource(n), from, fromMode, to, toMode, cc.NodeSource(n), n, r.volatileOrAtomicHandled, r)
	// trc("%v: c.pass=%v n=%q r=%p r.volatileOrAtomicHandled=%v", c.pos(n), c.pass, cc.NodeSource(n), r, r.volatileOrAtomicHandled)
	if c.isVolatileOrAtomicExpr(n) && !r.volatileOrAtomicHandled {
		// trc("%v: c.pass=%v n=%q r=%p r.volatileOrAtomicHandled=%v", c.pos(n), c.pass, cc.NodeSource(n), r, r.volatileOrAtomicHandled)
		c.err(errorf("TODO %v: %q %p %T %s, toMode %v", c.pos(n), cc.NodeSource(n), n, n, n.Type(), toMode))
	}

	if from == nil || fromMode == 0 {
		// trc("IN %v: from %v, %v to %v %v, src '%s', buf '%s'", c.pos(n), from, fromMode, to, toMode, cc.NodeSource(n), r.bytes())
		c.err(errorf("TODO %T %v %v -> %v %v", n, from, fromMode, to, toMode))
		return r
	}

	return c.convert(n, w, r, from, to, fromMode, toMode)
}

func (c *ctx) convert(n cc.ExpressionNode, w writer, s *buf, from, to cc.Type, fromMode, toMode mode) (r *buf) {
	// trc("IN %v: from %v, %v to %v %v, src '%s', buf '%s'", c.pos(n), from, fromMode, to, toMode, cc.NodeSource(n), s.bytes())
	// defer func() {
	// 	trc("OUT %v: from %v, %v to %v %v, src '%s', bufs '%s' -> '%s'", c.pos(n), from, fromMode, to, toMode, cc.NodeSource(n), s.bytes(), r.bytes())
	// }()
	if to == nil {
		// trc("ERR %v: from %v: %v, to <nil>: %v '%q' node: %T src: '%q' (%v: %v: %v:)", c.pos(n), from, fromMode, toMode, s.bytes(), s.n, cc.NodeSource(n), origin(4), origin(3), origin(2))
		c.err(errorf("%v: TODO", pos(n)))
		return s
	}

	if assert && fromMode == exprUintptr && from.Kind() != cc.Ptr && from.Kind() != cc.Function {
		trc("%v: %v %v -> %v %v", c.pos(n), from, fromMode, to, toMode)
		c.err(errorf("TODO assertion failed"))
	}
	if assert && toMode == exprUintptr && to.Kind() != cc.Ptr {
		trc("%v: %v %v -> %v %v", c.pos(n), from, fromMode, to, toMode)
		c.err(errorf("TODO assertion failed"))
	}
	if from != nil && from.Kind() == cc.Enum {
		from = from.(*cc.EnumType).UnderlyingType()
	}
	if to.Kind() == cc.Enum {
		to = to.(*cc.EnumType).UnderlyingType()
	}

	if cc.IsScalarType(from) && fromMode == exprDefault && toMode == exprBool {
		var b buf
		b.w("(%s != 0)", s)
		return &b
	}

	if from == to || from != nil && from.IsCompatible(to) || fromMode == exprBool && cc.IsIntegerType(to) {
		if fromMode == toMode {
			return s
		}

		if from == c.ast.SizeT || to == c.ast.SizeT {
			if fromMode == toMode && toMode != exprVoid {
				return c.convertType(n, s, from, to, fromMode, toMode)
			}
		}

		if fromMode == toMode {
			return s
		}

		return c.convertMode(n, w, s, from, to, fromMode, toMode)
	}

	if fromMode == toMode {
		return c.convertType(n, s, from, to, fromMode, toMode)
	}

	if from != nil && from.Kind() == cc.Ptr {
		return c.convertFromPointer(n, s, from.(*cc.PointerType), to, fromMode, toMode)
	}

	if toMode == exprVoid || to.Kind() == cc.Void {
		return s
	}

	if to.Kind() == cc.Ptr {
		return c.convertToPointer(n, s, from, to.(*cc.PointerType), fromMode, toMode)
	}

	if fromMode == exprBool && cc.IsScalarType(to) && toMode == exprDefault {
		return c.convertBoolToScalar(n, s, from, to, fromMode, toMode)
	}

	// trc("%v: %s", n.Position(), cc.NodeSource(n))
	// trc("TODO %q %s %s -> %s %s", s, from, fromMode, to, toMode)
	c.err(errorf("TODO %q %s %s -> %s %s", s, from, fromMode, to, toMode))
	return s //TODO
}

func (c *ctx) convertBoolToScalar(n cc.ExpressionNode, s *buf, from cc.Type, to cc.Type, fromMode, toMode mode) (r *buf) {
	var b buf
	switch fromMode {
	case exprBool:
		switch toMode {
		case exprDefault:
			b.w("(%s(%s%sBool32(%s)))", c.typ(n, to), c.task.tlsQualifier, tag(preserve), s)
			return &b
		}
	}
	c.err(errorf("TODO %q %s %s -> %s %s", s, from, fromMode, to, toMode))
	return s //TODO
}

func (c *ctx) convertToPointer(n cc.ExpressionNode, s *buf, from cc.Type, to *cc.PointerType, fromMode, toMode mode) (r *buf) {
	var b buf
	switch fromMode {
	case exprDefault:
		switch toMode {
		case exprUintptr:
			b.w("%suintptr(%s)", tag(preserve), unsafeAddr(c.pin(n, s)))
			return &b
		}
	case exprBool:
		switch toMode {
		case exprDefault:
			b.w("%s%sBool%s(%s)", c.task.tlsQualifier, tag(preserve), c.helper(n, to), s)
			return &b
		}
	}

	// trc("%v: from %v, %v to %v %v, src '%s', buf '%s'", c.pos(n), from, fromMode, to, toMode, cc.NodeSource(n), s.bytes())
	c.err(errorf("TODO %q %s %s -> %s %s", s, from, fromMode, to, toMode))
	return s //TODO
}

func (c *ctx) pin(n cc.Node, b *buf) *buf {
	// trc("%v: %s (%v: %v: %v)", pos(n), cc.NodeSource(n), origin(4), origin(3), origin(2))
	switch x := b.n.(type) {
	case *cc.Declarator:
		if x.StorageDuration() == cc.Automatic {
			c.f.declInfos.takeAddress(x)
		}
	case *cc.PostfixExpression:
		if d := c.declaratorOf(x.PostfixExpression); d != nil && d.StorageDuration() == cc.Automatic {
			c.f.declInfos.takeAddress(d)
		}
	}
	return b
}

// type unchanged
func (c *ctx) convertMode(n cc.ExpressionNode, w writer, s *buf, from, to cc.Type, fromMode, toMode mode) (r *buf) {
	// defer func() { trc("%v: from %v: %v, to %v: %v %q -> %q", c.pos(n), from, fromMode, to, toMode, b, r) }()
	var b buf
	switch fromMode {
	case exprDefault:
		switch toMode {
		case exprLvalue:
			return s
		case exprCall:
			return s
		case exprVoid:
			return s
		case exprBool:
			b.w("(%s != 0)", s)
			return &b
		case exprIndex:
			switch x := from.(type) {
			case *cc.PointerType:
				switch y := from.Undecay().(type) {
				case *cc.ArrayType:
					b.w("((*%s)(%s))", c.typ(n, y), unsafePointer(s))
					return &b
				default:
					c.err(errorf("TODO %T", y))
				}
			case *cc.ArrayType:
				b.w("(*(*%s)(%s))", c.typ(n, x), unsafeAddr(s))
				return &b
			default:
				trc("%v:", n.Position())
				c.err(errorf("TODO %T", x))
			}
		case exprSelect:
			return s
		case exprUintptr:
			if from.Kind() == cc.Ptr {
				return s
			}
		}
	case exprUintptr:
		switch toMode {
		case exprDefault:
			return s
		case exprBool:
			b.w("(%s != 0)", s)
			return &b
		case exprCall:
			var b buf
			ft := from.(*cc.PointerType).Elem().(*cc.FunctionType)
			b.w("(*(*func%s)(%sunsafe.%sPointer(&struct{%[3]suintptr}{%s})))", c.signature(ft, false, false, true), tag(importQualifier), tag(preserve), s)
			return &b
		}
	case exprBool:
		switch toMode {
		case exprDefault:
			switch {
			case cc.IsIntegerType(to):
				b.w("%s%sBool%s(%s)", c.task.tlsQualifier, tag(preserve), c.helper(n, to), s)
				return &b
			}
		case exprVoid:
			return s
		}
	case exprVoid:
		switch toMode {
		case exprDefault:
			return s
		}
	}
	//TODO- trc("%v: from %v, %v to %v %v, src '%s', buf '%s'", c.pos(n), from, fromMode, to, toMode, cc.NodeSource(n), s.bytes())
	c.err(errorf("TODO %v: from %v, %v to %v %v, src '%s', buf '%s'", c.pos(n), from, fromMode, to, toMode, cc.NodeSource(n), s.bytes()))
	//TODO- c.err(errorf("TODO %q %s %s -> %s %s", s, from, fromMode, to, toMode))
	return s //TODO
}

// mode unchanged
func (c *ctx) convertType(n cc.ExpressionNode, s *buf, from, to cc.Type, fromMode, toMode mode) (r *buf) {
	if fromMode != toMode {
		panic(todo("%v: internal error, %s, %s, %v %v -> %v %v", n.Position(), cc.NodeSource(n), s.bytes(), from, fromMode, to, toMode))
	}

	if fromMode == exprVoid {
		return s
	}

	// defer func() { trc("%v: from %v: %v, to %v: %v %q -> %q", c.pos(n), from, fromMode, to, toMode, s, r) }()
	if from.Kind() == cc.Ptr && to.Kind() == cc.Ptr || to.Kind() == cc.Void {
		return s
	}

	var b buf
	if cc.IsScalarType(from) && cc.IsScalarType(to) {
		//b.w("(%s(%s))", c.typ(n, to), s)
		switch {
		case from.Kind() == cc.Int128:
			//TODO
		case from.Kind() == cc.UInt128:
			//TODO
		case to.Kind() == cc.Int128:
			//TODO
		case to.Kind() == cc.UInt128:
			//TODO
		default:
			switch {
			case !cc.IsComplexType(from) && !cc.IsComplexType(to):
				b.w("(%s(%s))", c.verifyTyp(n, to), s)
			default:
				b.w("(%s%s%sFrom%s(%s))", c.task.tlsQualifier, tag(preserve), c.helper(n, to), c.helper(n, from), s)
			}
			return &b
		}
	}

	if from.Kind() == cc.Function && to.Kind() == cc.Ptr && to.(*cc.PointerType).Elem().Kind() == cc.Function {
		return s
	}

	if x, ok := s.n.(interface{ Value() cc.Value }); ok {
		if c.isZero(x.Value()) && (to.Kind() == cc.Union || to.Kind() == cc.Struct) {
			b.w("(%s{})", c.typ(n, to))
			return &b
		}
	}

	if cc.IsScalarType(from) && to.Kind() == cc.Union && from.Size() == to.Size() && fromMode == exprDefault && toMode == exprDefault {
		e := c.expr(nil, s.n.(cc.ExpressionNode), nil, exprDefault)
		b.w("(*(*%s)(%s))", c.typ(n, to), unsafePointer(fmt.Sprintf("&struct{ %s__ccgo %s}{%s}", tag(preserve), c.typ(n, from), e)))
		return &b
	}

	c.err(errorf("%v: TODO %q from=%s %v %v %v -> to=%s %v %v %v (%v:)", pos(n), s, from, from.Kind(), from.Size(), fromMode, to, to.Kind(), to.Size(), toMode, c.pos(n)))
	// panic(todo("")) //TODO-DBG
	//trc("", errorf("ERROR %q %s %s -> %s %s (%v:)", s, from, fromMode, to, toMode, c.pos(n))) //TODO-DBG
	return s //TODO
}

func (c *ctx) isCharType(t cc.Type) bool {
	switch t.Kind() {
	case cc.Char, cc.UChar, cc.SChar:
		return true
	}

	return false
}

func (c *ctx) convertFromPointer(n cc.ExpressionNode, s *buf, from *cc.PointerType, to cc.Type, fromMode, toMode mode) (r *buf) {
	var b buf
	if to.Kind() == cc.Ptr {
		if fromMode == exprUintptr && toMode == exprDefault {
			return s
		}

		if fromMode == exprDefault && toMode == exprUintptr {
			b.w("%suintptr(%s)", tag(preserve), unsafeAddr(c.pin(n, s)))
			return &b
		}
	}

	if cc.IsIntegerType(to) {
		if toMode == exprDefault {
			b.w("(%s(%s))", c.typ(n, to), s)
			return &b
		}
	}

	if toMode == exprVoid || toMode == exprDefault {
		return s
	}

	c.err(errorf("TODO %q %s %s, %s -> %s %s, %s", s, from, from.Kind(), fromMode, to, to.Kind(), toMode))
	// trc("%v: TODO %q %s %s, %s -> %s %s, %s", cpos(n), s, from, from.Kind(), fromMode, to, to.Kind(), toMode)
	return s //TODO
}

func (c *ctx) reduceBitFieldValue(expr *buf, f *cc.Field, t cc.Type, mode mode) (r *buf) {
	if mode != exprDefault || f == nil {
		return expr
	}

	var b buf
	bits := f.ValueBits()
	if bits >= t.Size()*8 {
		return expr
	}

	m := ^uint64(0) >> (64 - bits)
	switch {
	case cc.IsSignedInteger(t):
		w := t.Size() * 8
		b.w("(((%s)&%#0x)<<%d>>%[3]d)", expr, m, w-bits)
	default:
		b.w("((%s)&%#0x)", expr, m)
	}
	return &b
}

func (c *ctx) expr0(w writer, n cc.ExpressionNode, t cc.Type, mod mode) (r *buf, rt cc.Type, rmode mode) {
	// trc("%v: %T (%q), %v, %v (%v: %v: %v:) (IN) isVolatileOrAtomicExpr %v", n.Position(), n, cc.NodeSource(n), t, mod, origin(4), origin(3), origin(2), c.isVolatileOrAtomicExpr(n))
	// defer func() {
	// 	trc("%v: %T (%q), %v, %v (RET)", n.Position(), n, cc.NodeSource(n), t, mod)
	// }()

	c.exprNestLevel++

	defer func(mod mode) {
		c.exprNestLevel--
		if r == nil || rt == nil || !cc.IsIntegerType(rt) {
			return
		}

		if x, ok := n.(*cc.PrimaryExpression); ok {
			switch x.Case {
			case cc.PrimaryExpressionIdent: // IDENTIFIER
				if x.Value() != nil {
					return
				}
			case
				cc.PrimaryExpressionInt,     // INTCONST
				cc.PrimaryExpressionFloat,   // FLOATCONST
				cc.PrimaryExpressionChar,    // CHARCONST
				cc.PrimaryExpressionLChar,   // LONGCHARCONST
				cc.PrimaryExpressionString,  // STRINGLITERAL
				cc.PrimaryExpressionLString: // LONGSTRINGLITERAL
				return
			case
				cc.PrimaryExpressionExpr,    // '(' ExpressionList ')'
				cc.PrimaryExpressionStmt,    // '(' CompoundStatement ')'
				cc.PrimaryExpressionGeneric: // GenericSelection
				// ok
			default:
				c.err(errorf("internal error %T %v", x, x.Case))
				return
			}
		}
		if bf := rt.BitField(); bf != nil && bf.ValueBits() > 32 {
			r = c.reduceBitFieldValue(r, bf, rt, mod)
		}
	}(mod)

out:
	switch {
	case mod == exprBool:
		mod = exprDefault
	case mod == exprDefault && n.Type().Undecay().Kind() == cc.Array:
		if d := c.declaratorOf(n); d == nil || !d.IsParam() {
			mod = exprUintptr
		}
	case mod == exprVoid:
		if _, ok := n.(*cc.ExpressionList); ok {
			break out
		}

		switch x := n.(type) {
		case *cc.AssignmentExpression:
			break out
		case *cc.PostfixExpression:
			switch x.Case {
			case cc.PostfixExpressionCall, cc.PostfixExpressionDec, cc.PostfixExpressionInc:
				break out
			}
		case *cc.UnaryExpression:
			switch x.Case {
			case cc.UnaryExpressionDec, cc.UnaryExpressionInc:
				break out
			}
		case *cc.PrimaryExpression:
			switch x.Case {
			case cc.PrimaryExpressionExpr:
				break out
			}
		}

	}
	if t == nil {
		t = n.Type()
	}

	switch x := n.(type) {
	case *cc.AdditiveExpression:
		return c.additiveExpression(w, x, t, mod)
	case *cc.AndExpression:
		return c.andExpression(w, x, t, mod)
	case *cc.AssignmentExpression:
		return c.assignmentExpression(w, x, t, mod)
	case *cc.CastExpression:
		return c.castExpression(w, x, t, mod)
	case *cc.ConstantExpression:
		if c.exprNestLevel == 1 {
			c.exprNestLevel--

			defer func() { c.exprNestLevel++ }()
		}

		return c.expr0(w, x.ConditionalExpression, t, mod)
	case *cc.ConditionalExpression:
		return c.conditionalExpression(w, x, t, mod)
	case *cc.EqualityExpression:
		return c.equalityExpression(w, x, t, mod)
	case *cc.ExclusiveOrExpression:
		return c.exclusiveOrExpression(w, x, t, mod)
	case *cc.ExpressionList:
		return c.expressionList(w, x, t, mod)
	case *cc.InclusiveOrExpression:
		return c.inclusiveOrExpression(w, x, t, mod)
	case *cc.LogicalAndExpression:
		return c.logicalAndExpression(w, x, t, mod)
	case *cc.LogicalOrExpression:
		return c.logicalOrExpression(w, x, t, mod)
	case *cc.MultiplicativeExpression:
		return c.multiplicativeExpression(w, x, t, mod)
	case *cc.PostfixExpression:
		return c.postfixExpression(w, x, t, mod)
	case *cc.PrimaryExpression:
		return c.primaryExpression(w, x, t, mod)
	case *cc.RelationalExpression:
		return c.relationExpression(w, x, t, mod)
	case *cc.ShiftExpression:
		return c.shiftExpression(w, x, t, mod)
	case *cc.UnaryExpression:
		return c.unaryExpression(w, x, t, mod)
	default:
		c.err(errorf("TODO %T", x))
		return nil, nil, 0
	}
}

func (c *ctx) andExpression(w writer, n *cc.AndExpression, t cc.Type, mode mode) (r *buf, rt cc.Type, rmode mode) {
	if n.AndExpression != nil && n.EqualityExpression != nil && (n.AndExpression.Value() == cc.Unknown || n.EqualityExpression.Value() == cc.Unknown) {
		c.exprNestLevel--

		defer func() { c.exprNestLevel++ }()
	}

	var b buf
	switch n.Case {
	case cc.AndExpressionEq: // EqualityExpression
		c.err(errorf("TODO %v", n.Case))
	case cc.AndExpressionAnd: // AndExpression '&' EqualityExpression
		x, y := c.binopArgs(w, n.AndExpression, n.EqualityExpression, n.Type())
		b.w("(%s & %s)", x, y)
		rt, rmode = n.Type(), exprDefault
	default:
		c.err(errorf("internal error %T %v", n, n.Case))
	}
	return &b, rt, rmode
}

func (c *ctx) exclusiveOrExpression(w writer, n *cc.ExclusiveOrExpression, t cc.Type, mode mode) (r *buf, rt cc.Type, rmode mode) {
	if n.ExclusiveOrExpression != nil && n.AndExpression != nil && (n.ExclusiveOrExpression.Value() == cc.Unknown || n.AndExpression.Value() == cc.Unknown) {
		c.exprNestLevel--

		defer func() { c.exprNestLevel++ }()
	}

	var b buf
	switch n.Case {
	case cc.ExclusiveOrExpressionAnd: // AndExpression
		c.err(errorf("TODO %v", n.Case))
	case cc.ExclusiveOrExpressionXor: // ExclusiveOrExpression '^' AndExpression
		x, y := c.binopArgs(w, n.ExclusiveOrExpression, n.AndExpression, n.Type())
		b.w("(%s ^ %s)", x, y)
		rt, rmode = n.Type(), exprDefault
	default:
		c.err(errorf("internal error %T %v", n, n.Case))
	}
	return &b, rt, rmode
}

func (c *ctx) inclusiveOrExpression(w writer, n *cc.InclusiveOrExpression, t cc.Type, mode mode) (r *buf, rt cc.Type, rmode mode) {
	if n.InclusiveOrExpression != nil && n.ExclusiveOrExpression != nil && (n.InclusiveOrExpression.Value() == cc.Unknown || n.ExclusiveOrExpression.Value() == cc.Unknown) {
		c.exprNestLevel--

		defer func() { c.exprNestLevel++ }()
	}

	var b buf
	switch n.Case {
	case cc.InclusiveOrExpressionXor: // ExclusiveOrExpression
		c.err(errorf("TODO %v", n.Case))
	case cc.InclusiveOrExpressionOr: // InclusiveOrExpression '|' ExclusiveOrExpression
		x, y := c.binopArgs(w, n.InclusiveOrExpression, n.ExclusiveOrExpression, n.Type())
		b.w("(%s | %s)", x, y)
		rt, rmode = n.Type(), exprDefault
	default:
		c.err(errorf("internal error %T %v", n, n.Case))
	}
	return &b, rt, rmode
}

func (c *ctx) shiftExpression(w writer, n *cc.ShiftExpression, t cc.Type, mode mode) (r *buf, rt cc.Type, rmode mode) {
	switch {
	case c.isNegative(n.AdditiveExpression.Value()):
		c.exprNestLevel++

		defer func() { c.exprNestLevel-- }()
	case n.ShiftExpression != nil && n.ShiftExpression.Value() == cc.Unknown ||
		n.AdditiveExpression.Value() == cc.Unknown ||
		n.ShiftExpression != nil && n.ShiftExpression.Value() == cc.Unknown:
		c.exprNestLevel--

		defer func() { c.exprNestLevel++ }()
	}
	var b buf
	switch n.Case {
	case cc.ShiftExpressionAdd: // AdditiveExpression
		c.err(errorf("TODO %v", n.Case))
	case cc.ShiftExpressionLsh: // ShiftExpression "<<" AdditiveExpression
		b.w("(%s << %s)", c.expr(w, n.ShiftExpression, n.Type(), exprDefault), c.expr(w, n.AdditiveExpression, nil, exprDefault))
		rt, rmode = n.Type(), exprDefault
	case cc.ShiftExpressionRsh: // ShiftExpression ">>" AdditiveExpression
		b.w("(%s >> %s)", c.expr(w, n.ShiftExpression, n.Type(), exprDefault), c.expr(w, n.AdditiveExpression, nil, exprDefault))
		rt, rmode = n.Type(), exprDefault
	default:
		c.err(errorf("internal error %T %v", n, n.Case))
	}
	return &b, rt, rmode
}

func (c *ctx) nonConstBool(w writer, n cc.ExpressionNode) *buf {
	var b buf
	switch {
	case n.Value() != cc.Unknown:
		b.w("(%sBool(%s))", c.task.tlsQualifier, c.topExpr(w, n, nil, exprBool))
	default:
		b.w("(%s)", c.topExpr(w, n, nil, exprBool))
	}
	return &b
}

func (c *ctx) logicalAndExpression(w writer, n *cc.LogicalAndExpression, t cc.Type, mode mode) (r *buf, rt cc.Type, rmode mode) {
	if n.LogicalAndExpression != nil && n.InclusiveOrExpression != nil && (n.LogicalAndExpression.Value() == cc.Unknown || n.InclusiveOrExpression.Value() == cc.Unknown) {
		c.exprNestLevel--

		defer func() { c.exprNestLevel++ }()
	}

	var b buf
	rt = c.ast.Int
	switch n.Case {
	case cc.LogicalAndExpressionOr: // InclusiveOrExpression
		c.err(errorf("TODO %v", n.Case))
	case cc.LogicalAndExpressionLAnd: // LogicalAndExpression "&&" InclusiveOrExpression
		_, rmode = n.Type(), exprBool
		var al, ar buf
		bl := c.nonConstBool(&al, n.LogicalAndExpression)
		br := c.nonConstBool(&ar, n.InclusiveOrExpression)
		switch {
		default:
			// case al.len() == 0 || ar.len() == 0:
			b.w("((%s) && (%s))", bl, br)
		case al.len() == 0 && ar.len() != 0:
			// Sequence point
			// if v = bl; v { ar };
			// v && br
			v := fmt.Sprintf("%sv%d", tag(ccgoAutomatic), c.id())
			c.f.registerAutoVar(v, tag(preserve)+"bool")
			w.w("\nif %s = %s; %s { %s };", v, bl, v, ar.bytes())
			b.w("((%s) && (%s))", v, br)
		case al.len() != 0 && ar.len() == 0:
			// Sequence point
			// al;
			// bl && br
			w.w("%s;", al.bytes())
			b.w("((%s) && (%s))", bl, br)
		case al.len() != 0 && ar.len() != 0:
			// Sequence point
			// al; if v = bl; v { ar };
			// v && br
			v := fmt.Sprintf("%sv%d", tag(ccgoAutomatic), c.id())
			c.f.registerAutoVar(v, tag(preserve)+"bool")
			w.w("%s;", al.bytes())
			w.w("\nif %s = %s; %s { %s };", v, bl, v, ar.bytes())
			b.w("((%s) && (%s))", v, br)
		}
	default:
		c.err(errorf("internal error %T %v", n, n.Case))
	}
	return &b, rt, rmode
}

func (c *ctx) logicalOrExpression(w writer, n *cc.LogicalOrExpression, t cc.Type, mode mode) (r *buf, rt cc.Type, rmode mode) {
	if n.LogicalOrExpression != nil && n.LogicalAndExpression != nil && (n.LogicalOrExpression.Value() == cc.Unknown || n.LogicalAndExpression.Value() == cc.Unknown) {
		c.exprNestLevel--

		defer func() { c.exprNestLevel++ }()
	}

	var b buf
	switch n.Case {
	case cc.LogicalOrExpressionLAnd: // LogicalAndExpression
		c.err(errorf("TODO %v", n.Case))
	case cc.LogicalOrExpressionLOr: // LogicalOrExpression "||" LogicalAndExpression
		_, rmode = n.Type(), exprBool
		var al, ar buf
		bl := c.nonConstBool(&al, n.LogicalOrExpression)
		br := c.nonConstBool(&ar, n.LogicalAndExpression)
		switch {
		default:
			// case al.len() == 0 || ar.len() == 0:
			b.w("((%s) || (%s))", bl, br)
		case al.len() == 0 && ar.len() != 0:
			// Sequence point
			// if v = bl; !v { ar };
			// v || br
			v := fmt.Sprintf("%sv%d", tag(ccgoAutomatic), c.id())
			c.f.registerAutoVar(v, tag(preserve)+"bool")
			w.w("\nif %s = %s; !%s { %s };", v, bl, v, ar.bytes())
			b.w("((%s) || (%s))", v, br)
		case al.len() != 0 && ar.len() == 0:
			// Sequence point
			// al;
			// bl || br
			w.w("%s;", al.bytes())
			b.w("((%s) || (%s))", bl, br)
		case al.len() != 0 && ar.len() != 0:
			// Sequence point
			// al; if v = bl; !v { ar };
			// v || br
			v := fmt.Sprintf("%sv%d", tag(ccgoAutomatic), c.id())
			c.f.registerAutoVar(v, tag(preserve)+"bool")
			w.w("%s;", al.bytes())
			w.w("\nif %s = %s; !%s { %s };", v, bl, v, ar.bytes())
			b.w("((%s) || (%s))", v, br)
		}
	default:
		c.err(errorf("internal error %T %v", n, n.Case))
	}
	return &b, c.ast.Int, rmode
}

func (c *ctx) unparen(n cc.ExpressionNode) cc.ExpressionNode {
	switch x := n.(type) {
	case *cc.ExpressionList:
		if x.ExpressionList == nil {
			return c.unparen(x.AssignmentExpression)
		}
	case *cc.PrimaryExpression:
		if x.Case == cc.PrimaryExpressionExpr {
			return c.unparen(x.ExpressionList)
		}
	}

	return n
}

func (c *ctx) isIntLit(n cc.ExpressionNode) (bool, interface{}) {
	switch x := c.unparen(n).(type) {
	case *cc.PrimaryExpression:
		switch x.Case {
		case cc.PrimaryExpressionInt:
			return true, x.Value()
		}
	}
	return false, nil
}

func (c *ctx) isNonNegativeIntLit(n cc.ExpressionNode) (bool, uint64) {
	if ok, x := c.isIntLit(n); ok {
		switch y := x.(type) {
		case cc.Int64Value:
			if y >= 0 {
				return true, uint64(y)
			}
		case cc.UInt64Value:
			return true, uint64(y)
		}
	}
	return false, 0
}

func (c *ctx) canIgnore(n cc.ExpressionNode) bool {
	switch x := c.unparen(n).(type) {
	case *cc.CastExpression:
		if x.Case != cc.CastExpressionCast || x.TypeName.Type().Kind() != cc.Void {
			return false
		}

		return c.canIgnore(x.CastExpression)
	case *cc.PrimaryExpression:
		switch x.Case {
		case
			cc.PrimaryExpressionChar,
			cc.PrimaryExpressionFloat,
			cc.PrimaryExpressionIdent,
			cc.PrimaryExpressionInt,
			cc.PrimaryExpressionLChar,
			cc.PrimaryExpressionLString:

			return true
		}
	case *cc.ExpressionList:
		for ; x != nil; x = x.ExpressionList {
			if !c.canIgnore(x.AssignmentExpression) {
				return false
			}
		}

		return true
	}
	return false
}

func (c *ctx) conditionalExpression(w writer, n *cc.ConditionalExpression, t cc.Type, mode mode) (r *buf, rt cc.Type, rmode mode) {
	c.exprNestLevel--

	defer func() {
		r.volatileOrAtomicHandled = true
		c.exprNestLevel++
	}()

	var b buf
	switch n.Case {
	case cc.ConditionalExpressionLOr: // LogicalOrExpression
		c.err(errorf("TODO %v", n.Case))
	case cc.ConditionalExpressionCond: // LogicalOrExpression '?' ExpressionList ':' ConditionalExpression
		if n.LogicalOrExpression.Pure() {
			switch val := n.LogicalOrExpression.Value(); {
			case c.isNonZero(val):
				if mode == exprVoid {
					if c.canIgnore(n.ExpressionList) {
						return &b, t, mode
					}

					mode = exprDefault
				}
				b.w("%s", c.expr(w, n.ExpressionList, t, mode))
				return &b, t, mode
			case c.isZero(val):
				if mode == exprVoid {
					if c.canIgnore(n.ConditionalExpression) {
						return &b, t, mode
					}

					mode = exprDefault
				}
				b.w("%s", c.expr(w, n.ConditionalExpression, t, mode))
				return &b, t, mode
			}
		}

		v := fmt.Sprintf("%sv%d", tag(ccgoAutomatic), c.id())
		switch mode {
		case exprCall:
			rt, rmode = n.Type(), mode
			vs := fmt.Sprintf("var %s func%s;", v, c.signature(n.Type().(*cc.PointerType).Elem().(*cc.FunctionType), false, false, true))
			switch {
			case c.f != nil:
				c.f.registerAutoVar(v, "func"+c.signature(n.Type().(*cc.PointerType).Elem().(*cc.FunctionType), false, false, true))
			default:
				w.w("%s", vs)
			}
			w.w("if %s {", c.topExpr(w, n.LogicalOrExpression, nil, exprBool))
			w.w("%s = %s;", v, c.topExpr(w, n.ExpressionList, n.Type(), mode))
			w.w("} else {")
			w.w("%s = %s;", v, c.topExpr(w, n.ConditionalExpression, n.Type(), mode))
			w.w("};")
			b.w("%s", v)
		case exprIndex:
			rt, rmode = n.Type(), exprUintptr
			vs := fmt.Sprintf("var %s %s;", v, c.typ(n, n.Type()))
			switch {
			case c.f != nil:
				c.f.registerAutoVar(v, c.typ(n, n.Type()))
			default:
				w.w("%s", vs)
			}
			w.w("if %s {", c.topExpr(w, n.LogicalOrExpression, nil, exprBool))
			w.w("%s = %s;", v, c.pin(n, c.topExpr(w, n.ExpressionList, n.Type(), exprUintptr)))
			w.w("} else {")
			w.w("%s = %s;", v, c.pin(n, c.topExpr(w, n.ConditionalExpression, n.Type(), exprUintptr)))
			w.w("};")
			b.w("%s", v)
		case exprVoid:
			rt, rmode = n.Type(), mode
			switch {
			case c.canIgnore(n.ExpressionList):
				w.w("if !(%s) {", c.topExpr(w, n.LogicalOrExpression, nil, exprBool))
				w.w("%s;", c.discardStr2(n.ConditionalExpression, c.topExpr(w, n.ConditionalExpression, n.Type(), exprVoid)))
				w.w("};")
			default:
				switch {
				case c.canIgnore(n.ConditionalExpression):
					w.w("if %s {", c.topExpr(w, n.LogicalOrExpression, nil, exprBool))
					w.w("%s;", c.discardStr2(n.ExpressionList, c.topExpr(w, n.ExpressionList, n.Type(), exprVoid)))
					w.w("};")
				default:
					w.w("if %s {", c.topExpr(w, n.LogicalOrExpression, nil, exprBool))
					w.w("%s;", c.discardStr2(n.ExpressionList, c.topExpr(w, n.ExpressionList, n.Type(), exprVoid)))
					w.w("} else {")
					w.w("%s;", c.discardStr2(n.ConditionalExpression, c.topExpr(w, n.ConditionalExpression, n.Type(), exprVoid)))
					w.w("};")
				}
			}
		default:
			rt, rmode = n.Type(), mode
			vs := fmt.Sprintf("var %s %s;", v, c.typ(n, n.Type()))
			switch {
			case c.f != nil:
				c.f.registerAutoVar(v, c.typ(n, n.Type()))
			default:
				w.w("%s", vs)
			}
			w.w("if %s {", c.topExpr(w, n.LogicalOrExpression, nil, exprBool))
			w.w("%s = %s;", v, c.topExpr(w, n.ExpressionList, n.Type(), exprDefault))
			w.w("} else {")
			w.w("%s = %s;", v, c.topExpr(w, n.ConditionalExpression, n.Type(), exprDefault))
			w.w("};")
			b.w("%s", v)
		}
	default:
		c.err(errorf("internal error %T %v", n, n.Case))
	}
	return &b, rt, rmode
}

func (c *ctx) discardStr(n cc.ExpressionNode) string {
	if n == nil || !c.mustConsume(n) {
		return ""
	}

	return fmt.Sprintf("%s_ = ", tag(preserve))
}

func (c *ctx) discardStr2(n cc.ExpressionNode, b *buf) string {
	s := c.strUnparen(strings.TrimSpace(string(b.bytes())))
	if s == "" {
		return ""
	}
	if _, err := strconv.ParseInt(s, 0, 64); err == nil {
		return ""
	}

	return fmt.Sprintf("%s%s", c.discardStr(n), b)
}

func (c *ctx) strUnparen(s string) string {
	for strings.HasPrefix(s, "(") && strings.HasSuffix(s, ")") {
		s = s[1 : len(s)-1]
	}
	return s
}

func (c *ctx) isNonZero(v cc.Value) bool {
	if v == nil || v == cc.Unknown {
		return false
	}

	switch x := v.(type) {
	case cc.Int64Value:
		return x != 0
	case cc.UInt64Value:
		return x != 0
	case cc.Float64Value:
		return x != 0
	case *cc.ZeroValue:
		return false
	case cc.Complex128Value:
		return x != 0
	case cc.Complex64Value:
		return x != 0
	case *cc.ComplexLongDoubleValue:
		return !c.isZero(x.Re) || !c.isZero(x.Im)
	case *cc.LongDoubleValue:
		return !(*big.Float)(x).IsInf() && (*big.Float)(x).Sign() != 0
	case *cc.UnknownValue:
		return false
	case cc.StringValue:
		return true
	case cc.UTF16StringValue:
		return true
	case cc.UTF32StringValue:
		return true
	case cc.VoidValue:
		return false
	default:
		return false
	}
}

// func (c *ctx) isSafeNonZero(n cc.ExpressionNode) bool {
// 	return c.isNonZero(n.Value()) && c.canIgnore(n)
// }

func (c *ctx) isSafeZero(n cc.ExpressionNode) bool {
	return c.isZero(n.Value()) && c.canIgnore(n)
}

func (c *ctx) isZero(v cc.Value) bool {
	if v == nil || v == cc.Unknown {
		return false
	}

	switch x := v.(type) {
	case cc.Int64Value:
		return x == 0
	case cc.UInt64Value:
		return x == 0
	case cc.Float64Value:
		return x == 0
	case *cc.ZeroValue:
		return true
	case cc.Complex128Value:
		return x == 0
	case cc.Complex64Value:
		return x == 0
	case *cc.ComplexLongDoubleValue:
		return c.isZero(x.Re) && c.isZero(x.Im)
	case *cc.LongDoubleValue:
		return !(*big.Float)(x).IsInf() && (*big.Float)(x).Sign() == 0
	case *cc.UnknownValue:
		return false
	case cc.StringValue:
		return false
	case cc.UTF16StringValue:
		return false
	case cc.UTF32StringValue:
		return false
	case cc.VoidValue:
		return false
	default:
		return false
	}
}

func (c *ctx) isNegative(v cc.Value) bool {
	if v == nil || v == cc.Unknown {
		return false
	}

	switch x := v.(type) {
	case cc.Int64Value:
		return x < 0
	case cc.UInt64Value:
		return false
	case cc.Float64Value:
		return x < 0
	case *cc.ZeroValue:
		return false
	case cc.Complex128Value:
		return false
	case cc.Complex64Value:
		return false
	case *cc.ComplexLongDoubleValue:
		return false
	case *cc.LongDoubleValue:
		return !(*big.Float)(x).IsInf() && (*big.Float)(x).Sign() < 0
	case *cc.UnknownValue:
		return false
	case cc.StringValue:
		return false
	case cc.UTF16StringValue:
		return false
	case cc.UTF32StringValue:
		return false
	case cc.VoidValue:
		return false
	default:
		return false
	}
}

func (c *ctx) castExpression(w writer, n *cc.CastExpression, t cc.Type, mode mode) (r *buf, rt cc.Type, rmode mode) {
	var b buf
	switch n.Case {
	case cc.CastExpressionUnary: // UnaryExpression
		return c.expr0(w, n.UnaryExpression, t, mode)
	case cc.CastExpressionCast: // '(' TypeName ')' CastExpression
		switch x := t.(type) {
		case *cc.PointerType:
			switch x.Elem().(type) {
			case *cc.FunctionType:
				if mode == exprCall {
					rt, rmode = n.Type(), exprUintptr
					b.w("%s", c.expr(w, n.CastExpression, n.Type(), exprDefault))
					return &b, rt, rmode
				}
			}
		}

		rt, rmode = n.Type(), mode
		if mode == exprVoid {
			w.w("%s;", c.discardStr2(n.CastExpression, c.expr(w, n.CastExpression, rt, rmode)))
			break
		}

		b.w("%s", c.expr(w, n.CastExpression, rt, rmode))
	default:
		c.err(errorf("internal error %T %v", n, n.Case))
	}
	b.n = n
	return &b, rt, rmode
}

func (c *ctx) multiplicativeExpression(w writer, n *cc.MultiplicativeExpression, t cc.Type, mode mode) (r *buf, rt cc.Type, rmode mode) {
	if n.MultiplicativeExpression != nil && n.CastExpression != nil && (n.MultiplicativeExpression.Value() == cc.Unknown || n.CastExpression.Value() == cc.Unknown) {
		c.exprNestLevel--

		defer func() { c.exprNestLevel++ }()
	}

	rt, rmode = n.Type(), exprDefault
	var b buf
	switch n.Case {
	case cc.MultiplicativeExpressionCast: // CastExpression
		c.err(errorf("TODO %v", n.Case))
	case cc.MultiplicativeExpressionMul: // MultiplicativeExpression '*' CastExpression
		x, y := c.binopArgs(w, n.MultiplicativeExpression, n.CastExpression, n.Type())
		b.w("(%s * %s)", x, y)
	case cc.MultiplicativeExpressionDiv: // MultiplicativeExpression '/' CastExpression
		x, y := c.binopArgs(w, n.MultiplicativeExpression, n.CastExpression, n.Type())
		b.w("(%s / %s)", x, y)
	case cc.MultiplicativeExpressionMod: // MultiplicativeExpression '%' CastExpression
		x, y := c.binopArgs(w, n.MultiplicativeExpression, n.CastExpression, n.Type())
		b.w("(%s %% %s)", x, y)
	default:
		c.err(errorf("internal error %T %v", n, n.Case))
	}
	return &b, rt, rmode
}

func (c *ctx) elemSize(n cc.ExpressionNode, op string) (r string) {
	switch sz := n.Type().(*cc.PointerType).Elem().Undecay().Size(); {
	case sz < 0:
		switch d := c.declaratorOf(n); {
		case c.f != nil && d != nil && c.f.vlaSizes[d] != "":
			return fmt.Sprintf("%s%suintptr(%s)", op, tag(preserve), c.f.vlaSizes[d])
		}
	case sz == 1:
		return ""
	case sz > 1:
		return fmt.Sprintf("%s%d", op, sz)
	}
	c.err(errorf("%v: TODO", pos(n)))
	return ""
}

func (c *ctx) additiveExpression(w writer, n *cc.AdditiveExpression, t cc.Type, mode mode) (r *buf, rt cc.Type, rmode mode) {
	if n.AdditiveExpression != nil && n.MultiplicativeExpression != nil && (n.AdditiveExpression.Value() == cc.Unknown || n.MultiplicativeExpression.Value() == cc.Unknown) {
		c.exprNestLevel--

		defer func() { c.exprNestLevel++ }()
	}

	rt, rmode = n.Type(), exprDefault
	var b buf
	switch n.Case {
	case cc.AdditiveExpressionMul: // MultiplicativeExpression
		c.err(errorf("TODO %v", n.Case))
	case cc.AdditiveExpressionAdd: // AdditiveExpression '+' MultiplicativeExpression
		switch x, y := n.AdditiveExpression.Type(), n.MultiplicativeExpression.Type(); {
		case cc.IsArithmeticType(x) && cc.IsArithmeticType(y):
			x, y := c.binopArgs(w, n.AdditiveExpression, n.MultiplicativeExpression, n.Type())
			b.w("(%s + %s)", x, y)
		case x.Kind() == cc.Ptr && cc.IsIntegerType(y):
			x, y := c.binopArgs(w, n.AdditiveExpression, n.MultiplicativeExpression, n.Type())
			b.w("(%s + ((%s)%s))", x, y, c.elemSize(n.AdditiveExpression, "*"))
		case cc.IsIntegerType(x) && y.Kind() == cc.Ptr:
			x, y := c.binopArgs(w, n.AdditiveExpression, n.MultiplicativeExpression, n.Type())
			b.w("(((%s)%s)+%s)", x, c.elemSize(n.MultiplicativeExpression, "*"), y)
		default:
			c.err(errorf("TODO %v + %v", x, y)) // -
		}
	case cc.AdditiveExpressionSub: // AdditiveExpression '-' MultiplicativeExpression
		switch x, y := n.AdditiveExpression.Type(), n.MultiplicativeExpression.Type(); {
		case cc.IsArithmeticType(x) && cc.IsArithmeticType(y):
			x, y := c.binopArgs(w, n.AdditiveExpression, n.MultiplicativeExpression, n.Type())
			b.w("(%s - %s)", x, y)
		case x.Kind() == cc.Ptr && y.Kind() == cc.Ptr:
			x, y := c.binopArgs(w, n.AdditiveExpression, n.MultiplicativeExpression, n.Type())
			b.w("((%s - %s)%s)", x, y, c.elemSize(n.AdditiveExpression, "/"))
		case x.Kind() == cc.Ptr && cc.IsIntegerType(y):
			x, y := c.binopArgs(w, n.AdditiveExpression, n.MultiplicativeExpression, n.Type())
			b.w("(%s - ((%s)%s))", x, y, c.elemSize(n.AdditiveExpression, "*"))
		default:
			c.err(errorf("TODO %v - %v", x, y))
		}
	default:
		c.err(errorf("internal error %T %v", n, n.Case))
	}
	return &b, rt, rmode
}

func (c *ctx) binopArgs(w writer, a, b cc.ExpressionNode, t cc.Type) (x, y *buf) {
	return c.checkVolatileExpr(w, a, t, exprDefault), c.checkVolatileExpr(w, b, t, exprDefault)
}

func (c *ctx) checkVolatileExpr(w writer, n cc.ExpressionNode, t cc.Type, mode mode) (r *buf) {
	if !c.isVolatileOrAtomicExpr(n) {
		return c.expr(w, n, t, mode)

	}

	defer func() { r.volatileOrAtomicHandled = true }()

	switch mode {
	case exprDefault:
		switch x := n.(type) {
		case *cc.UnaryExpression:
			switch x.Case {
			case cc.UnaryExpressionMinus, cc.UnaryExpressionInc, cc.UnaryExpressionDec:
				return c.expr(w, n, t, mode)
			}
		case *cc.PrimaryExpression:
			switch x.Case {
			case
				cc.PrimaryExpressionChar,
				cc.PrimaryExpressionExpr,
				cc.PrimaryExpressionFloat,
				cc.PrimaryExpressionInt,
				cc.PrimaryExpressionLChar,
				cc.PrimaryExpressionLString:

				return c.expr(w, n, t, mode)
			}
		case *cc.AssignmentExpression:
			if d := c.declaratorOf(x.UnaryExpression); d != nil /*TODO && !d.AddressTaken() */ && d.StorageDuration() == cc.Automatic {
				r := c.expr(w, n, t, mode)
				r.volatileOrAtomicHandled = true
				return r
			}
		}

		b := c.atomicLoad(w, n, c.topExpr(w, n, n.Type().Pointer(), exprUintptr), n.Type())
		return c.convert(n, w, b, n.Type(), t, mode, mode)
	}
	// c.err(errorf("%v: TODO n=%q t=%s mode=%v", n.Position(), cc.NodeSource(n), t, mode))
	return c.expr(w, n, t, mode)
}

func (c *ctx) equalityExpression(w writer, n *cc.EqualityExpression, t cc.Type, mode mode) (r *buf, rt cc.Type, rmode mode) {
	c.exprNestLevel--

	defer func() { c.exprNestLevel++ }()

	var b buf
	if n.Case == cc.EqualityExpressionRel { // RelationalExpression
		c.err(errorf("TODO %v", n.Case))
		return &b, nil, exprBool
	}

	ct := c.usualArithmeticConversions(n.EqualityExpression.Type(), n.RelationalExpression.Type())
	rt, rmode = n.Type(), exprBool
out:
	switch n.Case {
	case cc.EqualityExpressionEq: // EqualityExpression "==" RelationalExpression
		if c.canIgnore(n.EqualityExpression) && c.canIgnore(n.RelationalExpression) && n.Value() != cc.Unknown {
			switch {
			case c.isZero(n.Value()):
				b.w("(false)")
				break out
			case c.isNonZero(n.Value()):
				b.w("(true)")
				break out
			}
		}

		x, y := c.binopArgs(w, n.EqualityExpression, n.RelationalExpression, ct)
		b.w("(%s == %s)", x, y)
	case cc.EqualityExpressionNeq: // EqualityExpression "!=" RelationalExpression
		x, y := c.binopArgs(w, n.EqualityExpression, n.RelationalExpression, ct)
		b.w("(%s != %s)", x, y)
	default:
		c.err(errorf("internal error %T %v", n, n.Case))
	}
	return &b, rt, rmode
}

func (c *ctx) relationExpression(w writer, n *cc.RelationalExpression, t cc.Type, mode mode) (r *buf, rt cc.Type, rmode mode) {
	c.exprNestLevel--

	defer func() { c.exprNestLevel++ }()

	var b buf
	if n.Case == cc.RelationalExpressionShift { // ShiftExpression
		c.err(errorf("TODO %v", n.Case))
		return &b, nil, exprBool
	}

	ct := c.usualArithmeticConversions(n.RelationalExpression.Type(), n.ShiftExpression.Type())
	rt, rmode = n.Type(), exprBool
	switch n.Case {
	case cc.RelationalExpressionLt: // RelationalExpression '<' ShiftExpression
		x, y := c.binopArgs(w, n.RelationalExpression, n.ShiftExpression, ct)
		b.w("(%s < %s)", x, y)
	case cc.RelationalExpressionGt: // RelationalExpression '>' ShiftExpression
		x, y := c.binopArgs(w, n.RelationalExpression, n.ShiftExpression, ct)
		b.w("(%s > %s)", x, y)
	case cc.RelationalExpressionLeq: // RelationalExpression "<=" ShiftExpression
		x, y := c.binopArgs(w, n.RelationalExpression, n.ShiftExpression, ct)
		b.w("(%s <= %s)", x, y)
	case cc.RelationalExpressionGeq: // RelationalExpression ">=" ShiftExpression
		x, y := c.binopArgs(w, n.RelationalExpression, n.ShiftExpression, ct)
		b.w("(%s >= %s)", x, y)
	default:
		c.err(errorf("internal error %T %v", n, n.Case))
	}
	return &b, rt, rmode
}

func (c *ctx) usualArithmeticConversions(a, b cc.Type) (r cc.Type) {
	if a.Kind() == cc.Ptr && (cc.IsIntegerType(b) || b.Kind() == cc.Ptr) {
		return a
	}

	if b.Kind() == cc.Ptr && (cc.IsIntegerType(a) || a.Kind() == cc.Ptr) {
		return b
	}

	return cc.UsualArithmeticConversions(a, b)
}

func (c *ctx) isBitField(n cc.ExpressionNode) bool {
	for {
		switch x := n.(type) {
		case *cc.PostfixExpression:
			switch x.Case {
			case cc.PostfixExpressionSelect: // PostfixExpression '.' IDENTIFIER
				return x.Field().IsBitfield()
			case cc.PostfixExpressionPSelect: // PostfixExpression "->" IDENTIFIER
				return x.Field().IsBitfield()
			default:
				return false
			}
		case *cc.PrimaryExpression:
			switch x.Case {
			case cc.PrimaryExpressionExpr: // '(' ExpressionList ')'
				n = x.ExpressionList
			default:
				return false
			}
		case *cc.UnaryExpression:
			switch x.Case {
			case cc.UnaryExpressionPostfix: // PostfixExpression
				n = x.PostfixExpression
			default:
				return false
			}
		default:
			trc("TODO %T", x)
			return false
		}
	}
}

func (c *ctx) preIncDecBitField(op string, w writer, n cc.ExpressionNode, mode mode) (r *buf, rt cc.Type, rmode mode) {
	var b buf
	var p *buf
	var f *cc.Field
	switch x := n.(type) {
	case *cc.PostfixExpression:
		switch x.Case {
		case cc.PostfixExpressionSelect:
			p = c.pin(n, c.expr(w, x.PostfixExpression, x.PostfixExpression.Type().Pointer(), exprUintptr))
			f = x.Field()
		case cc.PostfixExpressionPSelect:
			p = c.expr(w, x.PostfixExpression, nil, exprDefault)
			f = x.Field()
		default:
			trc("%v: BITFIELD %v", n.Position(), x.Case)
			c.err(errorf("TODO %T", x))
			return &b, rt, rmode
		}
	default:
		trc("%v: BITFIELD %v", n.Position(), mode)
		c.err(errorf("TODO %T", x))
		return &b, rt, rmode
	}

	switch mode {
	case exprDefault:
		v := fmt.Sprintf("%sv%d", tag(ccgoAutomatic), c.id())
		vs := fmt.Sprintf("var %s %s;", v, c.typ(n, f.Type()))
		switch {
		case c.f != nil:
			c.f.registerAutoVar(v, c.typ(n, f.Type()))
		default:
			w.w("%s", vs)
		}
		bf, _, _ := c.bitField(w, n, p, f, exprDefault, false) //TODO atomic bit fields
		w.w("\n%v = %sAssignBitFieldPtr%d%s(%s+%d, (%s)%s1, %d, %d, %#0x);", v, c.task.tlsQualifier, f.AccessBytes()*8, c.helper(n, f.Type()), p, f.Offset(), bf, op, f.ValueBits(), f.OffsetBits(), f.Mask())
		b.w("%s", v)
		return &b, f.Type(), exprDefault
	case exprVoid:
		sop := "Inc"
		if op == "-" {
			sop = "Dec"
		}
		w.w("\n%sPost%sBitFieldPtr%d%s(%s+%d, 1, %d, %d, %#0x);", c.task.tlsQualifier, sop, f.AccessBytes()*8, c.helper(n, f.Type()), p, f.Offset(), f.ValueBits(), f.OffsetBits(), f.Mask())
		return &b, n.Type(), exprVoid
	default:
		trc("%v: BITFIELD %v", n.Position(), mode)
		c.err(errorf("TODO %v", mode))
	}
	return &b, rt, rmode
}

func (c *ctx) unaryExpression(w writer, n *cc.UnaryExpression, t cc.Type, mode mode) (r *buf, rt cc.Type, rmode mode) {
	var b buf
out:
	switch n.Case {
	case cc.UnaryExpressionPostfix: // PostfixExpression
		c.err(errorf("TODO %v", n.Case))
	case cc.UnaryExpressionInc: // "++" UnaryExpression
		if c.isBitField(n.UnaryExpression) {
			return c.preIncDecBitField("+", w, n.UnaryExpression, mode)
		}

		rt, rmode = n.Type(), mode
		switch ue := n.UnaryExpression.Type(); {
		case ue.Kind() == cc.Ptr && ue.(*cc.PointerType).Elem().Undecay().Size() != 1:
			sz := ue.(*cc.PointerType).Elem().Undecay().Size()
			switch mode {
			case exprVoid:
				b.w("%s += %d", c.expr(w, n.UnaryExpression, nil, exprDefault), sz)
			case exprDefault:
				switch d := c.declaratorOf(n.UnaryExpression); {
				case d != nil:
					v := c.f.newAutovar(n, n.UnaryExpression.Type())
					ds := c.expr(w, n.UnaryExpression, nil, exprDefault)
					w.w("%s += %d;", ds, sz)
					w.w("\n%s = %s;", v, ds)
					b.w("%s", v)
				default:
					pt := n.UnaryExpression.Type().Pointer()
					v := c.f.newAutovar(n, n.UnaryExpression.Type())
					v2 := c.f.newAutovar(n, pt)
					w.w("%s = %s;", v2, c.expr(w, n.UnaryExpression, pt, exprUintptr))
					w.w("(*(*%s)(%s)) += %d;", c.typ(n, n.UnaryExpression.Type()), unsafePointer(v2), sz)
					w.w("%s = (*(*%s)(%s));", v, c.typ(n, n.UnaryExpression.Type()), unsafePointer(v2))
					b.w("%s", v)
				}
			default:
				c.err(errorf("TODO %v", mode)) // -
			}
		default:
			switch mode {
			case exprVoid:
				defer func() { r.volatileOrAtomicHandled = true }()
				if c.isVolatileOrAtomicExpr(n.UnaryExpression) {
					bp := c.expr(w, n.UnaryExpression, n.UnaryExpression.Type().Pointer(), exprUintptr)
					b.w("%sPreIncAtomic%sP(%s, 1)", c.task.tlsQualifier, c.helper(n, n.UnaryExpression.Type()), bp)
					break
				}

				b.w("%s++", c.expr(w, n.UnaryExpression, nil, exprDefault))
			case exprDefault:
				if c.isVolatileOrAtomicExpr(n.UnaryExpression) {
					bp := c.expr(w, n.UnaryExpression, n.UnaryExpression.Type().Pointer(), exprUintptr)
					b.w("%sPreIncAtomic%sP(%s, 1)", c.task.tlsQualifier, c.helper(n, n.UnaryExpression.Type()), bp)
					defer func() { r.volatileOrAtomicHandled = true }()
					break
				}

				switch d := c.declaratorOf(n.UnaryExpression); {
				case d != nil:
					v := c.f.newAutovar(n, n.UnaryExpression.Type())
					ds := c.expr(w, n.UnaryExpression, nil, exprDefault)
					w.w("%s++;", ds)
					w.w("\n%s = %s;", v, ds)
					b.w("%s", v)
				default:
					v := c.f.newAutovar(n, n.UnaryExpression.Type())
					v2 := c.f.newAutovar(n, n.UnaryExpression.Type().Pointer())
					w.w("%s = %s;", v2, c.expr(w, n.UnaryExpression, n.UnaryExpression.Type().Pointer(), exprUintptr))
					w.w("(*(*%s)(%s))++;", c.typ(n, n.UnaryExpression.Type()), unsafePointer(v2))
					w.w("%s = (*(*%s)(%s));", v, c.typ(n, n.UnaryExpression.Type()), unsafePointer(v2))
					b.w("%s", v)
				}
			default:
				c.err(errorf("TODO %v", mode)) // -
				// panic(todo(""))
			}
		}
	case cc.UnaryExpressionDec: // "--" UnaryExpression
		if c.isBitField(n.UnaryExpression) {
			return c.preIncDecBitField("-", w, n.UnaryExpression, mode)
		}

		rt, rmode = n.Type(), mode
		switch ue := n.UnaryExpression.Type(); {
		case ue.Kind() == cc.Ptr && ue.(*cc.PointerType).Elem().Undecay().Size() != 1:
			sz := ue.(*cc.PointerType).Elem().Undecay().Size()
			switch mode {
			case exprVoid:
				b.w("%s -= %d", c.expr(w, n.UnaryExpression, nil, exprDefault), sz)
			case exprDefault:
				switch d := c.declaratorOf(n.UnaryExpression); {
				case d != nil:
					v := c.f.newAutovar(n, n.UnaryExpression.Type())
					ds := c.expr(w, n.UnaryExpression, nil, exprDefault)
					w.w("%s -= %d;", ds, sz)
					w.w("\n%s = %s;", v, ds)
					b.w("%s", v)
				default:
					pt := n.UnaryExpression.Type().Pointer()
					v := c.f.newAutovar(n, n.UnaryExpression.Type())
					v2 := c.f.newAutovar(n, pt)
					w.w("%s = %s;", v2, c.expr(w, n.UnaryExpression, pt, exprUintptr))
					w.w("(*(*%s)(%s)) -= %d;", c.typ(n, n.UnaryExpression.Type()), unsafePointer(v2), sz)
					w.w("%s = (*(*%s)(%s));", v, c.typ(n, n.UnaryExpression.Type()), unsafePointer(v2))
					b.w("%s", v)
				}
			default:
				c.err(errorf("TODO %v", mode)) // -
			}
		default:
			switch mode {
			case exprVoid:
				if c.isVolatileOrAtomicExpr(n.UnaryExpression) {
					bp := c.expr(w, n.UnaryExpression, n.UnaryExpression.Type().Pointer(), exprUintptr)
					b.w("%sPreIncAtomic%sP(%s, -1)", c.task.tlsQualifier, c.helper(n, n.UnaryExpression.Type()), bp)
					defer func() { r.volatileOrAtomicHandled = true }()
					break
				}

				b.w("%s--", c.expr(w, n.UnaryExpression, nil, exprDefault))
			case exprDefault:
				if c.isVolatileOrAtomicExpr(n.UnaryExpression) {
					bp := c.expr(w, n.PostfixExpression, n.UnaryExpression.Type().Pointer(), exprUintptr)
					b.w("%sPreIncAtomic%sP(%s, -1)", c.task.tlsQualifier, c.helper(n, n.UnaryExpression.Type()), bp)
					defer func() { r.volatileOrAtomicHandled = true }()
					break
				}

				switch d := c.declaratorOf(n.UnaryExpression); {
				case d != nil:
					v := c.f.newAutovar(n, n.UnaryExpression.Type())
					ds := c.expr(w, n.UnaryExpression, nil, exprDefault)
					w.w("%s--;", ds)
					w.w("\n%s = %s;", v, ds)
					b.w("%s", v)
				default:
					v := c.f.newAutovar(n, n.UnaryExpression.Type())
					v2 := c.f.newAutovar(n, n.UnaryExpression.Type().Pointer())
					w.w("%s = %s;", v2, c.expr(w, n.UnaryExpression, n.UnaryExpression.Type().Pointer(), exprUintptr))
					w.w("(*(*%s)(%s))--;", c.typ(n, n.UnaryExpression.Type()), unsafePointer(v2))
					w.w("%s = (*(*%s)(%s));", v, c.typ(n, n.UnaryExpression.Type()), unsafePointer(v2))
					b.w("%s", v)
				}
			default:
				c.err(errorf("TODO %v", mode)) // -
			}
		}
	case cc.UnaryExpressionAddrof: // '&' CastExpression
		// trc("%v: nt %v, ct %v, '%s' %v", n.Token.Position(), n.Type(), n.CastExpression.Type(), cc.NodeSource(n), mode)
		switch n.Type().Undecay().(type) {
		case *cc.FunctionType:
			rt, rmode = n.Type(), mode
			b.w("%s", c.expr(w, n.CastExpression, nil, mode))
			break out
		}

		rt, rmode = n.Type(), exprUintptr
		b.w("%s", c.expr(w, n.CastExpression, rt, exprUintptr))
	case cc.UnaryExpressionDeref: // '*' CastExpression
		if c.isVolatileOrAtomicExpr(n) {
			switch {
			case n.Type().Kind() == cc.Void:
				defer func() { r.volatileOrAtomicHandled = true }()
			case mode == exprDefault, mode == exprVoid:
				defer func() { r.volatileOrAtomicHandled = true }()
				return c.atomicLoad(w, n, c.topExpr(w, n.CastExpression, n.CastExpression.Type(), exprDefault), n.Type()), n.Type(), mode
			}
		}
		// trc("%v: nt %v, ct %v, '%s' %v", n.Token.Position(), n.Type(), n.CastExpression.Type(), cc.NodeSource(n), mode)
		if ce, ok := n.CastExpression.(*cc.CastExpression); ok && ce.Case == cc.CastExpressionCast {
			if pfe, ok := ce.CastExpression.(*cc.PostfixExpression); ok && pfe.Case == cc.PostfixExpressionCall {
				if pe, ok := pfe.PostfixExpression.(*cc.PrimaryExpression); ok && pe.Case == cc.PrimaryExpressionIdent && pe.Token.SrcStr() == "__builtin_va_arg_impl" {
					if argumentExpressionListLen(pfe.ArgumentExpressionList) != 1 {
						c.err(errorf("internal error"))
						break out
					}

					p, ok := ce.Type().(*cc.PointerType)
					if !ok {
						c.err(errorf("internal error"))
						break out
					}

					rt, rmode = n.Type(), mode
					t := p.Elem()
					if !cc.IsScalarType(t) {
						b.w("(*((*%s)(%s)))", c.typ(n, t), unsafePointer(fmt.Sprintf("%sVaOther(&%s, %d)", c.task.tlsQualifier, c.expr(w, pfe.ArgumentExpressionList.AssignmentExpression, nil, exprDefault), t.Size())))
						break out
					}

					b.w("%sVa%s(&%s)", c.task.tlsQualifier, c.helper(n, t), c.expr(w, pfe.ArgumentExpressionList.AssignmentExpression, nil, exprDefault))
					break out
				}
			}
		}
		switch n.Type().Undecay().(type) {
		case *cc.FunctionType:
			rt, rmode = n.Type(), mode
			b.w("%s", c.expr(w, n.CastExpression, nil, mode))
			break out
		}

		switch mode {
		case exprDefault, exprLvalue, exprVoid:
			rt, rmode = n.Type(), mode
			b.w("(*(*%s)(%s))", c.typ(n, n.CastExpression.Type().(*cc.PointerType).Elem()), unsafePointer(c.expr(w, n.CastExpression, nil, exprDefault)))
		case exprSelect:
			rt, rmode = n.Type(), mode
			b.w("((*%s)(%s))", c.typ(n, n.CastExpression.Type().(*cc.PointerType).Elem()), unsafePointer(c.expr(w, n.CastExpression, nil, exprDefault)))
		case exprUintptr:
			defer func() { r.volatileOrAtomicHandled = true }()
			rt, rmode = n.CastExpression.Type(), mode
			b.w("%s", c.expr(w, n.CastExpression, nil, exprDefault))
		case exprCall:
			rt, rmode = n.CastExpression.Type().(*cc.PointerType).Elem(), exprUintptr
			b.w("(*(*%suintptr)(%s))", tag(preserve), unsafePointer(c.expr(w, n.CastExpression, nil, exprDefault)))
		default:
			// trc("%v: %s", n.Token.Position(), cc.NodeSource(n))
			c.err(errorf("TODO %v", mode))
		}
	case cc.UnaryExpressionPlus: // '+' CastExpression
		rt, rmode = n.Type(), exprDefault
		b.w("(+(%s))", c.checkVolatileExpr(w, n.CastExpression, n.Type(), exprDefault))
	case cc.UnaryExpressionMinus: // '-' CastExpression
		rt, rmode = n.Type(), exprDefault
		if c.exprNestLevel == 1 && cc.IsSignedInteger(n.CastExpression.Type()) && cc.IsSignedInteger(t) {
			c.exprNestLevel--

			defer func() { c.exprNestLevel++ }()
		}

		defer func() { r.volatileOrAtomicHandled = true }()
		b.w("(-(%s))", c.checkVolatileExpr(w, n.CastExpression, n.Type(), exprDefault))
	case cc.UnaryExpressionCpl: // '~' CastExpression
		rt, rmode = n.Type(), exprDefault
		b.w("(^(%s))", c.checkVolatileExpr(w, n.CastExpression, n.Type(), exprDefault))
	case cc.UnaryExpressionNot: // '!' CastExpression
		rt, rmode = n.Type(), exprBool
		b.w("(!(%s))", c.checkVolatileExpr(w, n.CastExpression, nil, exprBool))
	case cc.UnaryExpressionSizeofExpr: // "sizeof" UnaryExpression
		if t.Kind() == cc.Void {
			t = n.Type()
		}

		if c.f != nil {
			if _, ok := c.isVLA(n.UnaryExpression.Type()); ok {
				d := c.declaratorOf(n.UnaryExpression)
				if d == nil {
					c.err(errorf("%v: internal error", n.Position()))
					break
				}

				b.w("(%s)", c.f.vlaSizes[d])
				return &b, t, exprDefault
			}
		}

		rt, rmode = t, exprDefault
		if c.isValidType(n.UnaryExpression, n.UnaryExpression.Type(), true) {
			switch {
			case c.exprNestLevel == 1 && cc.IsScalarType(t) && !cc.IsComplexType(t):
				b.w("(%s(%v))", c.verifyTyp(n, t), n.Value())
			default:
				b.w("(%s%s%sFromInt64(%d))", c.task.tlsQualifier, tag(preserve), c.helper(n, t), n.Value())
			}
		}
	case cc.UnaryExpressionSizeofType: // "sizeof" '(' TypeName ')'
		if t.Kind() == cc.Void {
			t = n.Type()
		}
		if c.f != nil {
			if vt, ok := c.isVLA(n.TypeName.Type()); ok {
				k := ""
				if sz := vt.Elem().Size(); sz != 1 {
					k = fmt.Sprintf("*%d", sz)
				}
				b.w("((%s)%s)", c.expr(w, vt.SizeExpression(), c.ast.SizeT, exprDefault), k)
				return &b, c.ast.SizeT, exprDefault
			}
		}

		rt, rmode = t, exprDefault
		if c.isValidType(n.TypeName, n.TypeName.Type(), true) {
			switch {
			case c.exprNestLevel == 1 && cc.IsScalarType(t) && !cc.IsComplexType(t):
				b.w("(%s(%v))", c.verifyTyp(n, t), n.Value())
			default:
				b.w("(%s%s%sFromInt64(%d))", c.task.tlsQualifier, tag(preserve), c.helper(n, t), n.Value())
			}
		}
	case cc.UnaryExpressionLabelAddr: // "&&" IDENTIFIER
		c.err(errorf("TODO %v", n.Case))
	case cc.UnaryExpressionAlignofExpr: // "_Alignof" UnaryExpression
		if t.Kind() == cc.Void {
			t = n.Type()
		}
		rt, rmode = t, exprDefault
		switch {
		case c.exprNestLevel == 1 && cc.IsScalarType(t) && !cc.IsComplexType(t):
			b.w("(%s(%v))", c.verifyTyp(n, t), n.Value())
		default:
			b.w("(%s%s%sFromInt32(%d))", c.task.tlsQualifier, tag(preserve), c.helper(n, t), n.UnaryExpression.Type().Align())
		}
	case cc.UnaryExpressionAlignofType: // "_Alignof" '(' TypeName ')'
		if t.Kind() == cc.Void {
			t = n.Type()
		}
		rt, rmode = t, exprDefault
		switch {
		case c.exprNestLevel == 1 && cc.IsScalarType(t) && !cc.IsComplexType(t):
			b.w("(%s(%v))", c.verifyTyp(n, t), n.Value())
		default:
			b.w("(%s%s%sFromInt32(%d))", c.task.tlsQualifier, tag(preserve), c.helper(n, t), n.TypeName.Type().Align())
		}
	case cc.UnaryExpressionImag: // "__imag__" UnaryExpression
		c.err(errorf("TODO %v", n.Case))
	case cc.UnaryExpressionReal: // "__real__" UnaryExpression
		c.err(errorf("TODO %v", n.Case))
	default:
		c.err(errorf("internal error %T %v", n, n.Case))
	}
	return &b, rt, rmode
}

func (c *ctx) mul(n cc.ExpressionNode) (r string) {
	switch x := n.Type().(type) {
	case *cc.PointerType:
		sz := x.Elem().Size()
		if sz == 1 {
			return ""
		}

		switch {
		case sz < 0:
			d := c.declaratorOf(n)
			switch _, ok := c.isVLA(x.Elem()); {
			case c.f != nil && d != nil && ok:
				return fmt.Sprintf("*%suintptr(%s)", tag(preserve), c.f.vlaSizes[d])
			default:
				c.err(errorf("%v: TODO", pos(n)))
			}
		default:
			return fmt.Sprintf("*%v", sz)
		}
	default:
		c.err(errorf("%v: TODO %T", pos(n), x))
	}
	return ""
}

func (c *ctx) indexOff(w writer, n cc.ExpressionNode, mul string) *buf {
	var b buf
	switch ok, idx := c.isNonNegativeIntLit(n); {
	case ok && idx == 0:
		// nop
	case ok:
		b.w("+((%d)%s)", idx, mul)
	default:
		b.w("+((%s)%s)", c.topExpr(w, n, c.pvoid, exprDefault), mul)
	}
	return &b
}

func (c *ctx) isUnionComplitSelect(n cc.ExpressionNode) (bool, *cc.PostfixExpression) {
	x, ok := c.unparen(n).(*cc.PostfixExpression)
	if !ok {
		return false, nil
	}

	if x.Case != cc.PostfixExpressionSelect {
		return false, nil
	}

	n = x.PostfixExpression
	y, ok := c.unparen(n).(*cc.PostfixExpression)
	if !ok {
		return false, nil
	}

	if y.Case != cc.PostfixExpressionComplit {
		return false, nil
	}

	return y.TypeName.Type().Kind() == cc.Union, x
}

func (c *ctx) postfixExpressionIndex(w writer, n, p, index cc.ExpressionNode, pt *cc.PointerType, nt, t cc.Type, mode mode) (r *buf, rt cc.Type, rmode mode) {
	var b buf
	elem := pt.Elem()
	mul := c.mul(p)
	rt, rmode = nt, mode
	switch x := p.(type) {
	case *cc.PostfixExpression:
		switch x.Case {
		case cc.PostfixExpressionSelect:
			if c.isVolatileOrAtomicExpr(x.PostfixExpression) {
				switch mode {
				case exprDefault:
					f := x.Field()
					if f.IsBitfield() {
						break
					}

					defer func() { r.volatileOrAtomicHandled = true }()
					bp := c.expr(w, x.PostfixExpression, x.PostfixExpression.Type().Pointer(), exprUintptr)
					if off := f.Offset(); off != 0 {
						bp.w("+%v*%s", off, mul)
					}
					b.w("%s", c.atomicLoad(w, n, bp, elem))
					return &b, elem, mode
				}
			}
		}
	}
	// trc("%v: %s[%s] %v", c.pos(p), cc.NodeSource(p), cc.NodeSource(index), mode)
	// defer func() { trc("%v: %s[%s] %v -> %q", c.pos(p), cc.NodeSource(p), cc.NodeSource(index), mode, r.bytes()) }()
	if c.isVolatileOrAtomicExpr(n) && mode == exprUintptr {
		defer func() { r.volatileOrAtomicHandled = true }()
	}
	if f := c.isLastStructOrUnionField(p); f != nil && f.IsFlexibleArrayMember() {
		// Flexible array member.
		//
		//  https://en.wikipedia.org/wiki/Flexible_array_member
		switch mode {
		case exprLvalue, exprDefault, exprSelect:
			b.w("(*(*%s)(%sunsafe.%sPointer(%s%s)))", c.typ(p, elem), tag(importQualifier), tag(preserve), c.expr(w, p, nil, exprDefault), c.indexOff(w, index, mul))
			return &b, nt, mode
		case exprUintptr:
			b.w("(%s%s)", c.expr(w, p, nil, exprDefault), c.indexOff(w, index, mul))
			return &b, nt.Pointer(), mode
		default:
			c.err(errorf("TODO %v", mode))
			return &b, t, mode
		}
	}

	if mode == exprVoid {
		mode = exprDefault
	}
	if c.isVolatileOrAtomicExpr(n) && mode == exprDefault {
		defer func() { r.volatileOrAtomicHandled = true }()
		switch p.Type().Kind() {
		case cc.Ptr:
			bp := c.expr(w, p, nil, exprDefault)
			bp.w("+%s%s", c.expr(w, index, c.pvoid, exprDefault), mul)
			return c.atomicLoad(w, n, bp, elem), elem, mode
		default:
			c.err(errorf("TODO %s %s", p.Type(), p.Type().Kind()))
			return &b, t, mode
		}
	}

	switch mode {
	case exprSelect, exprLvalue, exprDefault, exprIndex:
		switch x := pt.Undecay().(type) {
		case *cc.ArrayType:
			if d := c.declaratorOf(p); d != nil && !d.IsParam() {
				switch {
				case x.IsVLA():
					b.w("(*(*%s)(%s))", c.typ(p, x.Elem()), unsafe("Add", fmt.Sprintf("%s, (%s)%s", unsafePointer(fmt.Sprintf("%s", c.expr(w, p, nil, exprIndex))), c.topExpr(w, index, nil, exprDefault), mul)))
				default:
					b.w("%s[%s]", c.expr(w, p, nil, exprIndex), c.topExpr(w, index, nil, exprDefault))
				}
				break
			}

			if ok, x := c.isUnionComplitSelect(p); ok && x != nil {
				if b2, _, _ := c.postfixExpressionSelectComplit(w, x, x.Type(), exprDefault); b2 != nil {
					b.w("%s[%s]", b2, c.topExpr(w, index, nil, exprDefault))
					break
				}
			}

			b.w("(*(*%s)(%sunsafe.%sPointer(%s%s)))", c.typ(p, elem), tag(importQualifier), tag(preserve), c.expr(w, p, nil, exprDefault), c.indexOff(w, index, mul))
		case *cc.PointerType:
			b.w("(*(*%s)(%sunsafe.%sPointer(%s%s)))", c.typ(p, elem), tag(importQualifier), tag(preserve), c.expr(w, p, nil, exprDefault), c.indexOff(w, index, mul))
		default:
			// trc("%v: %s[%s] %v %T", c.pos(p), cc.NodeSource(p), cc.NodeSource(index), mode, x)
			c.err(errorf("TODO %T", x))
		}
	case exprCall:
		rt, rmode = t.(*cc.PointerType), exprUintptr
		switch x := pt.Undecay().(type) {
		case *cc.ArrayType:
			if d := c.declaratorOf(p); d != nil && !d.IsParam() {
				b.w("%s[%s]", c.expr(w, p, nil, exprIndex), c.expr(w, index, nil, exprDefault))
				break
			}

			b.w("(*(*%s)(%sunsafe.%sPointer(%s%s)))", c.typ(p, elem), tag(importQualifier), tag(preserve), c.expr(w, p, nil, exprDefault), c.indexOff(w, index, mul))
		case *cc.PointerType:
			b.w("(*(*%s)(%sunsafe.%sPointer(%s%s)))", c.typ(p, elem), tag(importQualifier), tag(preserve), c.expr(w, p, nil, exprDefault), c.indexOff(w, index, mul))
		default:
			// trc("%v: %s[%s] %v %T", c.pos(p), cc.NodeSource(p), cc.NodeSource(index), mode, x)
			c.err(errorf("TODO %T", x))
		}
	case exprUintptr:
		rt, rmode = nt.Pointer(), mode
		if elem.Kind() == cc.Array {
			if d := c.declaratorOf(p); d != nil && d.Type().Kind() == cc.Ptr {
				b.w("((%s)%s)", c.expr(w, p, nil, exprDefault), c.indexOff(w, index, mul))
				break
			}
		}

		b.w("(%s%s)", c.expr(w, p, nil, exprDefault), c.indexOff(w, index, mul))
	default:
		// trc("%v: %s[%s] %v", c.pos(p), cc.NodeSource(p), cc.NodeSource(index), mode)
		c.err(errorf("TODO %v", mode))
	}
	return &b, rt, rmode
}

func (c *ctx) postIncDecBitField(op string, w writer, n cc.ExpressionNode, mode mode) (r *buf, rt cc.Type, rmode mode) {
	var b buf
	var p *buf
	var f *cc.Field
	switch x := n.(type) {
	case *cc.PostfixExpression:
		switch x.Case {
		case cc.PostfixExpressionSelect:
			p = c.pin(n, c.expr(w, x.PostfixExpression, x.PostfixExpression.Type().Pointer(), exprUintptr))
			f = x.Field()
		case cc.PostfixExpressionPSelect:
			p = c.expr(w, x.PostfixExpression, nil, exprDefault)
			f = x.Field()
		default:
			trc("%v: BITFIELD %v", n.Position(), x.Case)
			c.err(errorf("TODO %T", x))
			return &b, rt, rmode
		}
	default:
		trc("%v: BITFIELD %v", n.Position(), mode)
		c.err(errorf("TODO %T", x))
		return &b, rt, rmode
	}

	switch mode {
	case exprDefault, exprVoid:
		b.w("%sPost%sBitFieldPtr%d%s(%s+%d, 1, %d, %d, %#0x)", c.task.tlsQualifier, op, f.AccessBytes()*8, c.helper(n, f.Type()), p, f.Offset(), f.ValueBits(), f.OffsetBits(), f.Mask())
		return &b, f.Type(), exprDefault
	default:
		trc("%v: BITFIELD %v", n.Position(), mode)
		c.err(errorf("TODO %v", mode))
	}
	return &b, rt, rmode
}

func (c *ctx) postfixExpression(w writer, n *cc.PostfixExpression, t cc.Type, mode mode) (r *buf, rt cc.Type, rmode mode) {
	var b buf
out:
	switch n.Case {
	case cc.PostfixExpressionPrimary: // PrimaryExpression
		c.err(errorf("TODO %v", n.Case))
	case cc.PostfixExpressionIndex: // PostfixExpression '[' ExpressionList ']'
		if x, ok := n.PostfixExpression.Type().(*cc.PointerType); ok {
			return c.postfixExpressionIndex(w, n, n.PostfixExpression, n.ExpressionList, x, n.Type(), t, mode)
		}

		if x, ok := n.ExpressionList.Type().(*cc.PointerType); ok {
			return c.postfixExpressionIndex(w, n, n.ExpressionList, n.PostfixExpression, x, n.Type(), t, mode)
		}

		c.err(errorf("TODO %v", n.Case))
	case cc.PostfixExpressionCall: // PostfixExpression '(' ArgumentExpressionList ')'
		//TODO __builtin_object_size 28_strings.c on darwin/amd64
		switch c.declaratorOf(n.PostfixExpression).Name() {
		case
			"__builtin_constant_p",
			"__ccgo__types_compatible_p":

			switch mode {
			case exprBool:
				rt, rmode = n.Type(), mode
				switch {
				case n.Value().(cc.Int64Value) == 0:
					b.w("(false)")
				default:
					b.w("(true)")
				}
			default:
				rt, rmode = n.Type(), exprDefault
				b.w("(%v)", n.Value())
			}
			break out
		case "__builtin_va_start":
			if argumentExpressionListLen(n.ArgumentExpressionList) != 2 || mode != exprVoid {
				c.err(errorf("internal error"))
				break out
			}

			rt, rmode = n.Type(), mode
			switch {
			case c.f.inlineInfo != nil:
				w.w("%s = %s", c.expr(w, n.ArgumentExpressionList.AssignmentExpression, nil, exprDefault), bpOff(c.f.inlineInfo.vaOff))
			default:
				w.w("%s = %s%s", c.expr(w, n.ArgumentExpressionList.AssignmentExpression, nil, exprDefault), tag(ccgo), vaArgName)
			}
			break out
		case "__builtin_va_end":
			if argumentExpressionListLen(n.ArgumentExpressionList) != 1 || mode != exprVoid {
				c.err(errorf("internal error"))
				break out
			}

			rt, rmode = n.Type(), mode
			w.w("%s_ = %s;", tag(preserve), c.expr(w, n.ArgumentExpressionList.AssignmentExpression, nil, exprDefault))
			break out
		case "__atomic_load_n":
			return c.atomicLoadN(w, n, t, mode)
		case "__atomic_store_n":
			return c.atomicStoreN(w, n, t, mode)
		case "__builtin_sub_overflow":
			return c.subOverflow(w, n, t, mode)
		case "__builtin_mul_overflow":
			return c.mulOverflow(w, n, t, mode)
		case "__builtin_add_overflow":
			return c.addOverflow(w, n, t, mode)
		case "__builtin_choose_expr":
			switch {
			case c.isNonZero(n.ArgumentExpressionList.AssignmentExpression.Value()):
				b.w("%s", c.expr(w, n.ArgumentExpressionList.ArgumentExpressionList.AssignmentExpression, nil, exprDefault))
			case c.isZero(n.ArgumentExpressionList.AssignmentExpression.Value()):
				b.w("%s", c.expr(w, n.ArgumentExpressionList.ArgumentExpressionList.ArgumentExpressionList.AssignmentExpression, nil, exprDefault))
			default:
				c.err(errorf("internal error"))
			}
			return &b, n.Type(), mode
		case "longjmp":
			jb := c.expr(w, n.ArgumentExpressionList.AssignmentExpression, c.pvoid, exprDefault)
			val := c.expr(w, n.ArgumentExpressionList.ArgumentExpressionList.AssignmentExpression, c.ast.Int, exprDefault)
			b.w("%stls.%[1]sLongjmp(%s, %s)", tag(preserve), jb, val)
			return &b, c.ast.Void, mode
		case
			"__atomic_fetch_add",
			"__atomic_fetch_sub",
			"__atomic_fetch_and",
			"__atomic_fetch_xor",
			"__atomic_fetch_or",
			"__atomic_fetch_nand":
			// type __atomic_fetch_add (type *ptr, type val, int memorder)
			return c.stdatomicFetchAdd(w, n, t, mode)
		case
			"__atomic_load",
			"__atomic_store":
			// void __atomic_load (type *ptr, type *ret, int memorder)
			// void __atomic_store (type *ptr, type *val, int memorder)
			return c.stdatomicLoad(w, n, t, mode)
		case "__atomic_exchange":
			// void __atomic_exchange (type *ptr, type *val, type *ret, int memorder)
			return c.stdatomicExchange(w, n, t, mode)
		case "__atomic_compare_exchange":
			// bool __atomic_compare_exchange (type *ptr, type *expected, type *desired, bool weak, int success_memorder, int failure_memorder)
			return c.stdatomicCompareExchange(w, n, t, mode)
		}

		switch mode {
		case exprSelect:
			switch n.Type().(type) {
			case *cc.StructType:
				return c.postfixExpressionCall(w, n, mode)
			case *cc.UnionType:
				v := fmt.Sprintf("%sv%d", tag(ccgoAutomatic), c.id())
				e, _, _ := c.postfixExpressionCall(w, n, mode)
				w.w("%s := %s;", v, e)
				b.w("%s", v)
				return &b, n.Type(), mode
			}
		default:
			return c.postfixExpressionCall(w, n, mode)
		}
	case cc.PostfixExpressionSelect: // PostfixExpression '.' IDENTIFIER
		return c.postfixExpressionSelect(w, n, t, mode)
	case cc.PostfixExpressionPSelect: // PostfixExpression "->" IDENTIFIER
		return c.postfixExpressionPSelect(w, n, t, mode)
	case cc.PostfixExpressionInc: // PostfixExpression "++"
		if c.isBitField(n.PostfixExpression) {
			return c.postIncDecBitField("Inc", w, n.PostfixExpression, mode)
		}

		rt, rmode = n.Type(), mode
		switch pe := n.PostfixExpression.Type(); {
		case pe.Kind() == cc.Ptr && pe.(*cc.PointerType).Elem().Undecay().Size() != 1:
			sz := pe.(*cc.PointerType).Elem().Undecay().Size()
			switch mode {
			case exprVoid:
				b.w("%s += %d", c.expr(w, n.PostfixExpression, nil, exprDefault), sz)
			case exprDefault, exprUintptr:
				v := c.f.newAutovar(n, n.PostfixExpression.Type())
				switch d := c.declaratorOf(n.PostfixExpression); {
				case d != nil:
					ds := c.expr(w, n.PostfixExpression, nil, exprDefault)
					w.w("%s = %s;", v, ds)
					w.w("%s += %d;", ds, sz)
					b.w("%s", v)
				default:
					v2 := c.f.newAutovar(n, n.PostfixExpression.Type().Pointer())
					w.w("%s = %s;", v2, c.expr(w, n.PostfixExpression, n.PostfixExpression.Type().Pointer(), exprUintptr))
					w.w("%s = (*(*%s)(%s));", v, c.typ(n, n.PostfixExpression.Type()), unsafePointer(v2))
					w.w("(*(*%s)(%s)) += %d;", c.typ(n, n.PostfixExpression.Type()), unsafePointer(v2), sz)
					b.w("%s", v)
				}
			default:
				c.err(errorf("TODO %v", mode)) // -
			}
		default:
			d := c.declaratorOf(n.PostfixExpression)
			switch mode {
			case exprVoid:
				defer func() {
					if r != nil {
						r.volatileOrAtomicHandled = true
					}
				}()
				et := n.PostfixExpression.Type()
				if c.isVolatileOrAtomicExpr(n.PostfixExpression) {
					bp := c.expr(w, n.PostfixExpression, et.Pointer(), exprUintptr)
					b.w("%sPostIncAtomic%sP(%s, 1)", c.task.tlsQualifier, c.helper(n, et), bp)
					break
				}

				b.w("%s++", c.expr(w, n.PostfixExpression, nil, exprDefault))
			case exprDefault:
				if c.isVolatileOrAtomicExpr(n.PostfixExpression) {
					bp := c.expr(w, n.PostfixExpression, d.Type().Pointer(), exprUintptr)
					b.w("%sPostIncAtomic%sP(%s, 1)", c.task.tlsQualifier, c.helper(n, d.Type()), bp)
					defer func() { r.volatileOrAtomicHandled = true }()
					break
				}

				v := c.f.newAutovar(n, n.PostfixExpression.Type())
				switch {
				case d != nil:
					ds := c.expr(w, n.PostfixExpression, nil, exprDefault)
					w.w("%s = %s;", v, ds)
					w.w("%s++;", ds)
					b.w("%s", v)
				default:
					v2 := c.f.newAutovar(n, n.PostfixExpression.Type().Pointer())
					w.w("%s = %s;", v2, c.expr(w, n.PostfixExpression, n.PostfixExpression.Type().Pointer(), exprUintptr))
					w.w("%s = (*(*%s)(%s));", v, c.typ(n, n.PostfixExpression.Type()), unsafePointer(v2))
					w.w("(*(*%s)(%s))++;", c.typ(n, n.PostfixExpression.Type()), unsafePointer(v2))
					b.w("%s", v)
				}
			default:
				c.err(errorf("TODO %v", mode)) // -
			}
		}
	case cc.PostfixExpressionDec: // PostfixExpression "--"
		if c.isBitField(n.PostfixExpression) {
			return c.postIncDecBitField("Dec", w, n.PostfixExpression, mode)
		}

		rt, rmode = n.Type(), mode
		switch pe := n.PostfixExpression.Type(); {
		case pe.Kind() == cc.Ptr && pe.(*cc.PointerType).Elem().Undecay().Size() != 1:
			sz := pe.(*cc.PointerType).Elem().Undecay().Size()
			switch mode {
			case exprVoid:
				b.w("%s -= %d", c.expr(w, n.PostfixExpression, nil, exprDefault), sz)
			case exprDefault:
				v := c.f.newAutovar(n, n.PostfixExpression.Type())
				switch d := c.declaratorOf(n.PostfixExpression); {
				case d != nil:
					ds := c.expr(w, n.PostfixExpression, nil, exprDefault)
					w.w("%s = %s;", v, ds)
					w.w("%s -= %d;", ds, sz)
					b.w("%s", v)
				default:
					v2 := c.f.newAutovar(n, n.PostfixExpression.Type().Pointer())
					w.w("%s = %s;", v2, c.expr(w, n.PostfixExpression, n.PostfixExpression.Type().Pointer(), exprUintptr))
					w.w("%s = (*(*%s)(%s));", v, c.typ(n, n.PostfixExpression.Type()), unsafePointer(v2))
					w.w("(*(*%s)(%s)) -= %d;", c.typ(n, n.PostfixExpression.Type()), unsafePointer(v2), sz)
					b.w("%s", v)
				}
			default:
				c.err(errorf("TODO %v", mode)) // -
			}
		default:
			switch mode {
			case exprVoid:
				if c.isVolatileOrAtomicExpr(n.PostfixExpression) {
					bp := c.expr(w, n.PostfixExpression, n.PostfixExpression.Type().Pointer(), exprUintptr)
					b.w("%sPostIncAtomic%sP(%s, -1)", c.task.tlsQualifier, c.helper(n, n.PostfixExpression.Type()), bp)
					defer func() { r.volatileOrAtomicHandled = true }()
					break
				}

				b.w("%s--", c.expr(w, n.PostfixExpression, nil, exprDefault))
			case exprDefault:
				if c.isVolatileOrAtomicExpr(n.PostfixExpression) {
					bp := c.expr(w, n.PostfixExpression, n.PostfixExpression.Type().Pointer(), exprUintptr)
					b.w("%sPostIncAtomic%sP(%s, -1)", c.task.tlsQualifier, c.helper(n, n.PostfixExpression.Type()), bp)
					defer func() { r.volatileOrAtomicHandled = true }()
					break
				}

				v := c.f.newAutovar(n, n.PostfixExpression.Type())
				switch d := c.declaratorOf(n.PostfixExpression); {
				case d != nil:
					ds := c.expr(w, n.PostfixExpression, nil, exprDefault)
					w.w("%s = %s;", v, ds)
					w.w("%s--;", ds)
					b.w("%s", v)
				default:
					v2 := c.f.newAutovar(n, n.PostfixExpression.Type().Pointer())
					w.w("%s = %s;", v2, c.expr(w, n.PostfixExpression, n.PostfixExpression.Type().Pointer(), exprUintptr))
					w.w("%s = (*(*%s)(%s));", v, c.typ(n, n.PostfixExpression.Type()), unsafePointer(v2))
					w.w("(*(*%s)(%s))--;", c.typ(n, n.PostfixExpression.Type()), unsafePointer(v2))
					b.w("%s", v)
				}
			default:
				c.err(errorf("%v: TODO", pos(n))) // -
			}
		}
	case cc.PostfixExpressionComplit: // '(' TypeName ')' '{' InitializerList ',' '}'
		var a []*cc.Initializer
		for l := n.InitializerList; l != nil; l = l.InitializerList {
			a = append(a, c.initalizerFlatten(l.Initializer, nil)...)
		}
		t := n.TypeName.Type()
		switch {
		case c.f != nil && cc.IsScalarType(t) && mode == exprUintptr:
			if c.f.compoundLiterals == nil {
				c.f.compoundLiterals = map[cc.ExpressionNode]int64{}
			}
			var bp int64
			switch c.pass {
			case 1:
				bp = roundup(c.f.tlsAllocs, bpAlign(t))
				c.f.compoundLiterals[n] = bp
				c.f.tlsAllocs += t.Size()
			case 2:
				bp = c.f.compoundLiterals[n]
			}
			w.w("*(*%s)(unsafe.Pointer(%s)) = %s;", c.typ(n, t), bpOff(bp), c.topExpr(w, a[0].AssignmentExpression, t, exprDefault))
			b.w("(%s)", bpOff(bp))
			return &b, t.Pointer(), mode
		case c.f != nil && mode == exprUintptr:
			if c.f.compoundLiterals == nil {
				c.f.compoundLiterals = map[cc.ExpressionNode]int64{}
			}
			var bp int64
			switch c.pass {
			case 1:
				bp = roundup(c.f.tlsAllocs, bpAlign(t))
				c.f.compoundLiterals[n] = bp
				c.f.tlsAllocs += t.Size()
			case 2:
				bp = c.f.compoundLiterals[n]
			}
			w.w("*(*%s)(unsafe.Pointer(%s)) = %s;", c.typ(n, t), bpOff(bp), c.initializer(w, n, a, t, 0, t.Kind() == cc.Array))
			b.w("(%s)", bpOff(bp))
			return &b, t.Pointer(), mode
		default:
			return c.initializer(w, n, a, t, 0, t.Kind() == cc.Array), t, exprDefault
		}
	default:
		c.err(errorf("internal error %T %v", n, n.Case))
	}
	return &b, rt, rmode
}

func (c *ctx) mulOverflow(w writer, n *cc.PostfixExpression, t cc.Type, mode mode) (r *buf, rt cc.Type, rmode mode) {
	var b buf
	args := argumentExpressionList(n.ArgumentExpressionList)
	if len(args) != 3 {
		c.err(errorf("%v: invalid number of arguments to __builtin_mul_overflow", n.ArgumentExpressionList.Position()))
		return &b, t, mode
	}

	switch {
	case cc.IsScalarType(args[0].Type()):
		// ok
	default:
		c.err(errorf("%v: invalid first argument to __builtin_mul_overflow: %s", n.ArgumentExpressionList.Position(), args[0].Type()))
		return &b, t, mode
	}

	switch {
	case cc.IsScalarType(args[1].Type()):
		// ok
	default:
		c.err(errorf("%v: invalid second argument to __builtin_mul_overflow: %s", n.ArgumentExpressionList.Position(), args[1].Type()))
		return &b, t, mode
	}

	if args[2].Type().Kind() != cc.Ptr {
		c.err(errorf("%v: invalid third argument to __builtin_mul_overflow: %s", n.ArgumentExpressionList.Position(), args[2].Type()))
		return &b, t, mode
	}

	to := args[2].Type().(*cc.PointerType).Elem()
	if !c.sameSignednessIntegers(args[0].Type(), args[1].Type(), to) {
		c.err(errorf("TODO %s * %s -> %s", args[0].Type(), args[1].Type(), to))
	}
	b.w("%s__builtin_mul_overflow%s(%stls, %s, %s, %s)", tag(external), c.helper(n, to), tag(ccgo), c.expr(w, args[0], to, exprDefault), c.expr(w, args[1], to, exprDefault), c.expr(w, args[2], nil, exprDefault))
	return &b, c.ast.Int, exprDefault
}

func (c *ctx) sameSignednessIntegers(n ...cc.Type) bool {
	if !cc.IsIntegerType(n[0]) {
		return false
	}

	n0 := cc.IsSignedInteger(n[0])
	for _, v := range n[1:] {
		if !cc.IsIntegerType(v) || cc.IsSignedInteger(v) != n0 {
			return false
		}
	}
	return true
}

func (c *ctx) addOverflow(w writer, n *cc.PostfixExpression, t cc.Type, mode mode) (r *buf, rt cc.Type, rmode mode) {
	var b buf
	args := argumentExpressionList(n.ArgumentExpressionList)
	if len(args) != 3 {
		c.err(errorf("%v: invalid number of arguments to __builtin_add_overflow", n.ArgumentExpressionList.Position()))
		return &b, t, mode
	}

	switch {
	case cc.IsScalarType(args[0].Type()):
		// ok
	default:
		c.err(errorf("%v: invalid first argument to __builtin_add_overflow: %s", n.ArgumentExpressionList.Position(), args[0].Type()))
		return &b, t, mode
	}

	switch {
	case cc.IsScalarType(args[1].Type()):
		// ok
	default:
		c.err(errorf("%v: invalid second argument to __builtin_add_overflow: %s", n.ArgumentExpressionList.Position(), args[1].Type()))
		return &b, t, mode
	}

	if args[2].Type().Kind() != cc.Ptr {
		c.err(errorf("%v: invalid third argument to __builtin_add_overflow: %s", n.ArgumentExpressionList.Position(), args[2].Type()))
		return &b, t, mode
	}

	to := args[2].Type().(*cc.PointerType).Elem()
	if !c.sameSignednessIntegers(args[0].Type(), args[1].Type(), to) {
		c.err(errorf("TODO %s + %s -> %s", args[0].Type(), args[1].Type(), to))
	}
	b.w("%s__builtin_add_overflow%s(%stls, %s, %s, %s)", tag(external), c.helper(n, to), tag(ccgo), c.expr(w, args[0], to, exprDefault), c.expr(w, args[1], to, exprDefault), c.expr(w, args[2], nil, exprDefault))
	return &b, c.ast.Int, exprDefault
}

func (c *ctx) subOverflow(w writer, n *cc.PostfixExpression, t cc.Type, mode mode) (r *buf, rt cc.Type, rmode mode) {
	var b buf
	args := argumentExpressionList(n.ArgumentExpressionList)
	if len(args) != 3 {
		c.err(errorf("%v: invalid number of arguments to __builtin_sub_overflow", n.ArgumentExpressionList.Position()))
		return &b, t, mode
	}

	switch {
	case cc.IsScalarType(args[0].Type()):
		// ok
	default:
		c.err(errorf("%v: invalid first argument to __builtin_sub_overflow: %s", n.ArgumentExpressionList.Position(), args[0].Type()))
		return &b, t, mode
	}

	switch {
	case cc.IsScalarType(args[1].Type()):
		// ok
	default:
		c.err(errorf("%v: invalid second argument to __builtin_sub_overflow: %s", n.ArgumentExpressionList.Position(), args[1].Type()))
		return &b, t, mode
	}

	if args[2].Type().Kind() != cc.Ptr {
		c.err(errorf("%v: invalid third argument to __builtin_add_overflow: %s", n.ArgumentExpressionList.Position(), args[2].Type()))
		return &b, t, mode
	}

	to := args[2].Type().(*cc.PointerType).Elem()
	if !c.sameSignednessIntegers(args[0].Type(), args[1].Type(), to) {
		c.err(errorf("TODO %s - %s -> %s", args[0].Type(), args[1].Type(), to))
	}
	b.w("%s__builtin_sub_overflow%s(%stls, %s, %s, %s)", tag(external), c.helper(n, to), tag(ccgo), c.expr(w, args[0], to, exprDefault), c.expr(w, args[1], to, exprDefault), c.expr(w, args[2], nil, exprDefault))
	return &b, c.ast.Int, exprDefault
}

func (c *ctx) atomicLoadN(w writer, n *cc.PostfixExpression, t cc.Type, mode mode) (r *buf, rt cc.Type, rmode mode) {
	var b buf

	args := argumentExpressionList(n.ArgumentExpressionList)
	if len(args) != 2 {
		c.err(errorf("%v: invalid number of arguments to __atomic_load_n", n.ArgumentExpressionList.Position()))
		return &b, t, mode
	}

	pt, ok := args[0].Type().(*cc.PointerType)
	if !ok {
		c.err(errorf("%v: invalid first argument to __atomic_load_n: %s", n.ArgumentExpressionList.Position(), args[0].Type()))
		return &b, t, mode
	}

	rt = pt.Elem()
	switch {
	case cc.IsIntegerType(rt):
		switch rt.Size() {
		case 1, 2, 4, 8:
			b.w("%sAtomicLoadN%s(%s, %s)", c.task.tlsQualifier, c.helper(n, rt), c.expr(w, args[0], nil, exprDefault), c.expr(w, args[1], nil, exprDefault))
		default:
			if !c.task.ignoreUnsupportedAtomicSizes {
				c.err(errorf("%v: invalid pointee size of first argument to __atomic_load_n: %s", n.ArgumentExpressionList.Position(), args[0].Type()))
				return &b, t, mode
			}

			b.w("(*(*%s)(unsafe.Pointer(%s)))", c.typ(n, rt), c.expr(w, args[0], nil, exprDefault))
		}
	default:
		b.w("%sAtomicLoadN%s(%s, %s)", c.task.tlsQualifier, c.helper(n, rt), c.expr(w, args[0], nil, exprDefault), c.expr(w, args[1], nil, exprDefault))
	}
	return &b, rt, mode
}

func (c *ctx) atomicStoreN(w writer, n *cc.PostfixExpression, t cc.Type, mode mode) (r *buf, rt cc.Type, rmode mode) {
	var b buf
	if mode != exprVoid {
		c.err(errorf("%v: __atomic_store_n used as a value", n.Position()))
		return &b, t, mode
	}

	args := argumentExpressionList(n.ArgumentExpressionList)
	if len(args) != 3 {
		c.err(errorf("%v: invalid number of arguments to __atomic_store_n", n.ArgumentExpressionList.Position()))
		return &b, t, mode
	}

	pt := args[0].Type()
	if pt.Kind() != cc.Ptr {
		c.err(errorf("%v: invalid first argument to __atomic_store_n: %s", n.ArgumentExpressionList.Position(), args[0].Type()))
		return &b, t, mode
	}

	switch et := pt.(*cc.PointerType).Elem(); {
	case cc.IsScalarType(et):
		b.w("%sAtomicStoreN%s(%s, %s, %s)", c.task.tlsQualifier, c.helper(n, et), c.expr(w, args[0], nil, exprDefault), c.expr(w, args[1], et, exprDefault), c.expr(w, args[2], nil, exprDefault))
	default:
		c.err(errorf("%v: invalid second argument to __atomic_store_n: %s", n.ArgumentExpressionList.Position(), et))
	}
	return &b, t, mode
}

// type __atomic_fetch_* (type *ptr, type val, int memorder)
//
// __atomic_fetch_add
// __atomic_fetch_sub
// __atomic_fetch_and
// __atomic_fetch_xor
// __atomic_fetch_or
// __atomic_fetch_nand
func (c *ctx) stdatomicFetchAdd(w writer, n *cc.PostfixExpression, t cc.Type, mode mode) (r *buf, rt cc.Type, rmode mode) {
	var b buf
	args := argumentExpressionList(n.ArgumentExpressionList)
	if len(args) != 3 {
		c.err(errorf("%v: invalid number of arguments to atomic operation, expected 3", n.ArgumentExpressionList.Position()))
		return &b, t, mode
	}

	pt := args[0].Type()
	if pt.Kind() != cc.Ptr {
		c.err(errorf("%v: invalid first argument to atomic operation: %s", n.ArgumentExpressionList.Position(), args[0].Type()))
		return &b, t, mode
	}

	var tls string
	switch {
	case c.f == nil:
		tls = fmt.Sprintf("%snil", tag(preserve))
	default:
		tls = fmt.Sprintf("%stls", tag(ccgo))
	}

	et := pt.(*cc.PointerType).Elem()
	switch {
	case cc.IsScalarType(et):
		b.w("%s%s(%s, %s, %s, %s)", c.expr(w, n.PostfixExpression, nil, exprCall), c.helper(n, et), tls, c.expr(w, args[0], nil, exprDefault), c.expr(w, args[1], et, exprDefault), c.expr(w, args[2], nil, exprDefault))
	default:
		c.err(errorf("%v: invalid first argument to atomic operation: pointer to %s", n.ArgumentExpressionList.Position(), et))
	}
	return &b, et, mode
}

// void __atomic_load (type *ptr, type *ret, int memorder)
func (c *ctx) stdatomicLoad(w writer, n *cc.PostfixExpression, t cc.Type, mode mode) (r *buf, rt cc.Type, rmode mode) {
	var b buf
	args := argumentExpressionList(n.ArgumentExpressionList)
	if len(args) != 3 {
		c.err(errorf("%v: invalid number of arguments to atomic operation, expected 3", n.ArgumentExpressionList.Position()))
		return &b, t, mode
	}

	pt := args[0].Type()
	if pt.Kind() != cc.Ptr {
		c.err(errorf("%v: invalid first argument to atomic operation: %s", n.ArgumentExpressionList.Position(), args[0].Type()))
		return &b, t, mode
	}

	pt2 := args[1].Type()
	if pt2.Kind() != cc.Ptr {
		c.err(errorf("%v: invalid second argument to atomic operation: %s", n.ArgumentExpressionList.Position(), args[1].Type()))
		return &b, t, mode
	}

	var tls string
	switch {
	case c.f == nil:
		tls = fmt.Sprintf("%snil", tag(preserve))
	default:
		tls = fmt.Sprintf("%stls", tag(ccgo))
	}

	et := pt.(*cc.PointerType).Elem()
	switch {
	case cc.IsScalarType(et):
		b.w("%s%s(%s, %s, %s, %s)", c.expr(w, n.PostfixExpression, nil, exprCall), c.helper(n, et), tls, c.expr(w, args[0], nil, exprDefault), c.expr(w, args[1], nil, exprDefault), c.expr(w, args[2], nil, exprDefault))
	default:
		c.err(errorf("%v: invalid first argument to atomic operation: pointer to %s", n.ArgumentExpressionList.Position(), et))
	}
	return &b, c.void, exprVoid
}

// void __atomic_exchange (type *ptr, type *val, type *ret, int memorder)
func (c *ctx) stdatomicExchange(w writer, n *cc.PostfixExpression, t cc.Type, mode mode) (r *buf, rt cc.Type, rmode mode) {
	var b buf
	args := argumentExpressionList(n.ArgumentExpressionList)
	if len(args) != 4 {
		c.err(errorf("%v: invalid number of arguments to atomic operation, expected 4", n.ArgumentExpressionList.Position()))
		return &b, t, mode
	}

	pt := args[0].Type()
	if pt.Kind() != cc.Ptr {
		c.err(errorf("%v: invalid first argument to atomic operation: %s", n.ArgumentExpressionList.Position(), args[0].Type()))
		return &b, t, mode
	}

	pt2 := args[1].Type()
	if pt2.Kind() != cc.Ptr {
		c.err(errorf("%v: invalid second argument to atomic operation: %s", n.ArgumentExpressionList.Position(), args[1].Type()))
		return &b, t, mode
	}

	pt3 := args[1].Type()
	if pt3.Kind() != cc.Ptr {
		c.err(errorf("%v: invalid third argument to atomic operation: %s", n.ArgumentExpressionList.Position(), args[2].Type()))
		return &b, t, mode
	}

	var tls string
	switch {
	case c.f == nil:
		tls = fmt.Sprintf("%snil", tag(preserve))
	default:
		tls = fmt.Sprintf("%stls", tag(ccgo))
	}

	et := pt.(*cc.PointerType).Elem()
	switch {
	case cc.IsScalarType(et):
		b.w("%s%s(%s, %s, %s, %s, %s)", c.expr(w, n.PostfixExpression, nil, exprCall), c.helper(n, et), tls, c.expr(w, args[0], nil, exprDefault), c.expr(w, args[1], nil, exprDefault), c.expr(w, args[2], nil, exprDefault), c.expr(w, args[3], nil, exprDefault))
	default:
		c.err(errorf("%v: invalid first argument to atomic operation: pointer to %s", n.ArgumentExpressionList.Position(), et))
	}
	return &b, c.void, exprVoid
}

// bool __atomic_compare_exchange (type *ptr, type *expected, type *desired, bool weak, int success_memorder, int failure_memorder)
func (c *ctx) stdatomicCompareExchange(w writer, n *cc.PostfixExpression, t cc.Type, mode mode) (r *buf, rt cc.Type, rmode mode) {
	var b buf
	args := argumentExpressionList(n.ArgumentExpressionList)
	if len(args) != 6 {
		c.err(errorf("%v: invalid number of arguments to atomic operation, expected 6", n.ArgumentExpressionList.Position()))
		return &b, t, mode
	}

	pt := args[0].Type()
	if pt.Kind() != cc.Ptr {
		c.err(errorf("%v: invalid first argument to atomic operation: %s", n.ArgumentExpressionList.Position(), args[0].Type()))
		return &b, t, mode
	}

	pt2 := args[1].Type()
	if pt2.Kind() != cc.Ptr {
		c.err(errorf("%v: invalid second argument to atomic operation: %s", n.ArgumentExpressionList.Position(), args[1].Type()))
		return &b, t, mode
	}

	pt3 := args[1].Type()
	if pt3.Kind() != cc.Ptr {
		c.err(errorf("%v: invalid third argument to atomic operation: %s", n.ArgumentExpressionList.Position(), args[2].Type()))
		return &b, t, mode
	}

	var tls string
	switch {
	case c.f == nil:
		tls = fmt.Sprintf("%snil", tag(preserve))
	default:
		tls = fmt.Sprintf("%stls", tag(ccgo))
	}

	et := pt.(*cc.PointerType).Elem()
	switch {
	case cc.IsScalarType(et):
		b.w(
			"%s%s(%s, %s, %s, %s, %s, %s, %s)",
			c.expr(w, n.PostfixExpression, nil, exprCall), c.helper(n, et), tls,
			c.expr(w, args[0], nil, exprDefault),
			c.expr(w, args[1], nil, exprDefault),
			c.expr(w, args[2], nil, exprDefault),
			c.expr(w, args[3], nil, exprDefault),
			c.expr(w, args[4], nil, exprDefault),
			c.expr(w, args[5], nil, exprDefault),
		)
	default:
		c.err(errorf("%v: invalid first argument to atomic operation: pointer to %s", n.ArgumentExpressionList.Position(), et))
	}
	return &b, et, mode
}

func (c *ctx) uintFromSize(sz int64) cc.Type {
	switch sz {
	case 1:
		return c.ast.UChar
	case 2:
		return c.ast.UShort
	case 4:
		return c.ast.UInt
	case 8:
		return c.ast.ULongLong
	}
	return nil
}

func (c *ctx) bitField(w writer, n cc.Node, p *buf, f *cc.Field, mode mode, atomic bool) (r *buf, rt cc.Type, rmode mode) {
	//TODO do not pin expr.fld
	rt = f.Type()
	if f.ValueBits() < c.ast.Int.Size()*8 {
		rt = c.ast.Int
	}
	var b buf
	switch mode {
	case exprDefault, exprVoid:
		rmode = exprDefault
		if atomic {
			if ut := c.uintFromSize(f.AccessBytes()); ut != nil {
				if f.Offset() != 0 {
					p.w("+%v", f.Offset())
				}
				b.w("((%s(%s((%s)&%#0x)>>%d)", c.typ(n, rt), c.typ(n, f.Type()), c.atomicLoad(w, n, p, ut), f.Mask(), f.OffsetBits())
				break
			}
		}

		b.w("((%s(%s((*(*uint%d)(%sunsafe.%sPointer(%s +%d))&%#0x)>>%d)", c.typ(n, rt), c.typ(n, f.Type()), f.AccessBytes()*8, tag(importQualifier), tag(preserve), p, f.Offset(), f.Mask(), f.OffsetBits())
		if cc.IsSignedInteger(f.Type()) && !c.isPositiveEnum(f.Type()) {
			w := f.Type().Size() * 8
			b.w("<<%d>>%[1]d", w-f.ValueBits())
		}
		b.w(")))")
	case exprUintptr:
		rt, rmode = rt.Pointer(), mode
		b.w("(uintptr)(%sunsafe.%sPointer(%s +%d))", tag(importQualifier), tag(preserve), p, f.Offset())
	default:
		c.err(errorf("TODO %v", mode))
	}
	return &b, rt, rmode
}

// t is enum type and all its enum consts are >= 0.
func (c *ctx) isPositiveEnum(t cc.Type) bool {
	switch x := t.(type) {
	case *cc.EnumType:
		return x.Min() >= 0
	}

	return false
}

// PostfixExpression "->" IDENTIFIER
func (c *ctx) postfixExpressionPSelect(w writer, n *cc.PostfixExpression, t cc.Type, mode mode) (r *buf, rt cc.Type, rmode mode) {
	var b buf
	f := n.Field()
	isVolatileOrAtomicExpr := c.isVolatileOrAtomicExpr(n)
	// trc("%v: %q %p %s %v vol %v, %v bf %v", n.Position(), cc.NodeSource(n), n, t, mode, isVolatileOrAtomicExpr, c.isVolatileOrAtomicExpr(n.PostfixExpression), f.IsBitfield())
	// defer trc("%p RET", n)
	if f.IsBitfield() {
		return c.bitField(w, n, c.expr(w, n.PostfixExpression, nil, exprDefault), n.Field(), mode, isVolatileOrAtomicExpr)
	}

	pe, ok := n.PostfixExpression.Type().(*cc.PointerType)
	if !ok {
		c.err(errorf("TODO %T", n.PostfixExpression.Type()))
		return &b, rt, rmode
	}

	parentFields, unionOk := c.collectParentFields(n, f, pe.Elem())
	if u, ok := pe.Elem().(*cc.UnionType); !unionOk || (ok && f != firstPositiveSizedField(u)) {
		switch mode {
		case exprSelect, exprLvalue, exprDefault:
			rt, rmode = n.Type(), mode
			switch {
			case f.Offset() != 0:
				b.w("(*(*%s)(%sunsafe.%sAdd(%s, %d)))", c.typ(n, f.Type()), tag(importQualifier), tag(preserve), unsafePointer(c.expr(w, n.PostfixExpression, nil, exprDefault)), f.Offset())
			default:
				b.w("(*(*%s)(%s))", c.typ(n, f.Type()), unsafePointer(c.expr(w, n.PostfixExpression, nil, exprDefault)))
			}
		case exprUintptr:
			rt, rmode = c.pvoid, mode
			switch {
			case f.Offset() != 0:
				b.w("((%s)+%v)", c.expr(w, n.PostfixExpression, nil, exprDefault), f.Offset())
			default:
				b.w("(%s)", c.expr(w, n.PostfixExpression, nil, exprDefault))
			}
		default:
			c.err(errorf("TODO %v", mode))
		}
		return &b, rt, rmode
	}

	if mode == exprVoid {
		mode = exprDefault
	}
	switch mode {
	case exprDefault:
		if isVolatileOrAtomicExpr {
			pt := n.PostfixExpression.Type()
			rt, rmode = f.Type(), mode
			p := c.expr(w, n.PostfixExpression, pt, exprDefault)
			if off := f.Offset(); off != 0 {
				p.w("+%v", off)
			}
			b.w("%s", c.atomicLoad(w, n, p, rt))
			defer func() { r.volatileOrAtomicHandled = true }()
			break
		}

		fallthrough
	case exprLvalue, exprIndex, exprSelect:
		rt, rmode = n.Type(), mode
		b.w("((*%s)(%s).", c.typ(n, pe.Elem()), unsafePointer(c.expr(w, n.PostfixExpression, nil, exprDefault)))
		switch {
		case f.Parent() != nil:
			c.parentFields(parentFields, &b, n.Token, f, pe.Elem())
		default:
			b.w("%s%s", tag(field), c.fieldName(n.PostfixExpression.Type(), f))
		}
		b.w(")")
	case exprUintptr:
		if isVolatileOrAtomicExpr {
			defer func() { r.volatileOrAtomicHandled = true }()
		}
		rt, rmode = n.Type().Pointer(), mode
		b.w("((%s)%s)", c.expr(w, n.PostfixExpression, nil, exprDefault), fldOff(f.Offset()))
	case exprCall:
		rt, rmode = n.Type().(*cc.PointerType), exprUintptr
		b.w("((*%s)(%s).", c.typ(n, pe.Elem()), unsafePointer(c.expr(w, n.PostfixExpression, nil, exprDefault)))
		switch {
		case f.Parent() != nil:
			c.parentFields(parentFields, &b, n.Token, f, pe.Elem())
		default:
			b.w("%s%s", tag(field), c.fieldName(n.PostfixExpression.Type(), f))
		}
		b.w(")")
	default:
		c.err(errorf("TODO %v", mode))
	}
	return &b, rt, rmode
}

// PostfixExpression '.' IDENTIFIER
func (c *ctx) postfixExpressionSelect(w writer, n *cc.PostfixExpression, t cc.Type, mode mode) (r *buf, rt cc.Type, rmode mode) {
	// defer func() {
	// 	trc("%v: %q %s %v -> %q %s %v", n.Position(), cc.NodeSource(n), t, mode, r, rt, rmode)
	// }()
	var b buf
	b.n = n
	f := n.Field()
	isVolatileOrAtomicExpr := c.isVolatileOrAtomicExpr(n)
	// trc("%v: %q %p %s %v vol %v, %v bf %v", n.Position(), cc.NodeSource(n), n, t, mode, isVolatileOrAtomicExpr, c.isVolatileOrAtomicExpr(n.PostfixExpression), f.IsBitfield())
	// defer trc("%p RET", n)
	if f.IsBitfield() {
		return c.bitField(w, n, c.pin(n, c.expr(w, n.PostfixExpression, n.PostfixExpression.Type().Pointer(), exprUintptr)), f, mode, isVolatileOrAtomicExpr)
	}

	if mode == exprVoid {
		mode = exprDefault
	}
	if b, rt, rmode := c.postfixExpressionSelectComplit(w, n, t, mode); b != nil {
		return b, rt, rmode
	}

	if b, rt, rmode := c.postfixExpressionSelectUnionField(w, n, t, mode); b != nil {
		return b, rt, rmode
	}

	isCall := c.isCall2(n.PostfixExpression)
	if isCall && mode == exprUintptr {
		if c.f.fnResults == nil {
			c.f.fnResults = map[cc.ExpressionNode]int64{}
		}
		var bp int64
		switch c.pass {
		case 1:
			bp = roundup(c.f.tlsAllocs, bpAlign(t))
			c.f.fnResults[n] = bp
			c.f.tlsAllocs += t.Size()
		case 2:
			bp = c.f.fnResults[n]
		}
		w.w("(*(*%s)(%s)) = %s.%s%s;", c.typ(n, f.Type()), unsafePointer(bpOff(bp)), c.expr(w, n.PostfixExpression, nil, exprDefault), tag(field), c.fieldName(n.PostfixExpression.Type(), f))
		b.w("(%s)", bpOff(bp))
		return &b, c.pvoid, mode
	}

	if u, ok := n.PostfixExpression.Type().(*cc.UnionType); !isCall && ok && u.Size() == f.Type().Size() {
		switch mode {
		case exprLvalue, exprDefault, exprSelect:
			b.w("(*(*%s)(%s))", c.typ(n, f.Type()), unsafePointer(c.expr(w, n.PostfixExpression, n.PostfixExpression.Type().Pointer(), exprUintptr)))
			return &b, n.Type(), mode
		case exprUintptr:
			if isVolatileOrAtomicExpr {
				defer func() { r.volatileOrAtomicHandled = true }()
			}
		}
	}

	parentFields, unionOk := c.collectParentFields(n, f, n.PostfixExpression.Type())
	if u, ok := n.PostfixExpression.Type().(*cc.UnionType); !unionOk || (ok && f != firstPositiveSizedField(u)) {
		switch mode {
		case exprLvalue, exprDefault, exprSelect:
			rt, rmode = n.Type(), mode
			switch {
			case f.Offset() != 0:
				b.w("(*(*%s)(%sunsafe.%sAdd(%[2]sunsafe.%sPointer(&(%s)), %d)))", c.typ(n, f.Type()), tag(importQualifier), tag(preserve), c.expr(w, n.PostfixExpression, nil, exprSelect), f.Offset())
			default:
				b.w("(*(*%s)(%s))", c.typ(n, f.Type()), unsafeAddr(c.expr(w, n.PostfixExpression, nil, exprSelect)))
			}
		case exprCall:
			rt, rmode = n.Type().(*cc.PointerType), exprUintptr
			switch {
			case f.Offset() != 0:
				b.w("(*(*%s)(%sunsafe.%sAdd(%[2]sunsafe.%sPointer(&(%s)), %d)))", c.typ(n, f.Type()), tag(importQualifier), tag(preserve), c.expr(w, n.PostfixExpression, nil, exprSelect), f.Offset())
			default:
				b.w("(*(*%s)(%s))", c.typ(n, f.Type()), unsafeAddr(c.expr(w, n.PostfixExpression, nil, exprSelect)))
			}
		case exprUintptr:
			pt := n.PostfixExpression.Type().Pointer()
			b.w("((%s)%s)", c.expr(w, n.PostfixExpression, pt, mode), fldOff(f.Offset()))
			return &b, f.Type().Pointer(), mode
		case exprIndex:
			switch x := n.Type().Undecay().(type) {
			case *cc.ArrayType:
				rt, rmode = n.Type(), mode
				switch {
				case f.Offset() != 0:
					b.w("((*%s)(%sunsafe.%sAdd(%s, %d)))", c.typ(n, f.Type()), tag(importQualifier), tag(preserve), unsafeAddr(c.expr(w, n.PostfixExpression, nil, exprSelect)), f.Offset())
				default:
					b.w("((*%s)(%s))", c.typ(n, f.Type()), unsafeAddr(c.expr(w, n.PostfixExpression, nil, exprSelect)))
				}
			default:
				c.err(errorf("TODO %T", x))
			}
		default:
			c.err(errorf("TODO %v", mode))
		}
		return &b, rt, rmode
	}

	if mode == exprVoid {
		mode = exprDefault
	}
	switch {
	case isVolatileOrAtomicExpr:
		switch mode {
		case exprDefault:
			rt, rmode = f.Type(), mode
			p := c.expr(w, n.PostfixExpression, n.PostfixExpression.Type().Pointer(), exprUintptr)
			if off := f.Offset(); off != 0 {
				p.w("+%v", off)
			}
			b.w("%s", c.atomicLoad(w, n, p, rt))
			defer func() { r.volatileOrAtomicHandled = true }()
			return &b, rt, rmode
		}
	case c.isVolatileOrAtomicExpr(n.PostfixExpression):
		switch mode {
		case exprDefault:
			rt, rmode = f.Type(), mode
			p := c.expr(w, n.PostfixExpression, n.PostfixExpression.Type().Pointer(), exprUintptr)
			if off := f.Offset(); off != 0 {
				p.w("+%v", off)
			}
			b.w("%s", c.atomicLoad(w, n, p, rt))
			defer func() { r.volatileOrAtomicHandled = true }()
			return &b, rt, rmode
		default:
			c.err(errorf("TODO %v", mode))
			return &b, rt, rmode
		}
	}

	switch mode {
	case exprDefault, exprLvalue, exprIndex, exprSelect:
		rt, rmode = n.Type(), mode
		b.w("(%s.", c.expr(w, n.PostfixExpression, nil, exprSelect))
		switch {
		case f.Parent() != nil:
			c.parentFields(parentFields, &b, n.Token, f, n.PostfixExpression.Type())
		default:
			b.w("%s%s", tag(field), c.fieldName(n.PostfixExpression.Type(), f))
		}
		b.w(")")
	case exprUintptr:
		defer func() { r.volatileOrAtomicHandled = true }()
		pt := n.PostfixExpression.Type().Pointer()
		b.w("((%s)%s)", c.expr(w, n.PostfixExpression, pt, mode), fldOff(f.Offset()))
		return &b, f.Type().Pointer(), mode
	case exprCall:
		rt, rmode = n.Type().(*cc.PointerType), exprUintptr
		b.w("(%s.", c.expr(w, n.PostfixExpression, nil, exprSelect))
		switch {
		case f.Parent() != nil:
			c.parentFields(parentFields, &b, n.Token, f, n.PostfixExpression.Type())
		default:
			b.w("%s%s", tag(field), c.fieldName(n.PostfixExpression.Type(), f))
		}
		b.w(")")
	default:
		c.err(errorf("TODO %v", mode))
	}
	return &b, rt, rmode
}

func (c *ctx) postfixExpressionSelectComplit(w writer, n *cc.PostfixExpression, t cc.Type, mode mode) (r *buf, rt cc.Type, rmode mode) {
	// PostfixExpression '.' IDENTIFIER
	// (struct/union type){expr}.field

	// defer func() {
	// 	trc("%v: %p %v:", n.Position(), r, origin(1))
	// }()

	if c.f == nil {
		return nil, nil, 0
	}

	x, ok := c.unparen(n.PostfixExpression).(*cc.PostfixExpression)
	if !ok {
		return nil, nil, 0
	}

	if _, ok := x.Type().(*cc.UnionType); !ok {
		return nil, nil, 0
	}

	if x.Case != cc.PostfixExpressionComplit {
		return nil, nil, 0
	}

	f := n.Field()
	ft := f.Type()
	if !(cc.IsScalarType(ft) || ft.Undecay().Kind() == cc.Array && cc.IsScalarType(ft.Undecay().(*cc.ArrayType).Elem())) {
		return nil, nil, 0
	}

	switch mode {
	case exprDefault:
		// ok
	default:
		trc("", mode, origin(7), origin(6), origin(5), origin(4))
		return nil, nil, 0
	}

	ct := x.TypeName.Type()
	var a []*cc.Initializer
	for l := x.InitializerList; l != nil; l = l.InitializerList {
		a = append(a, c.initalizerFlatten(l.Initializer, nil)...)
	}

	switch len(a) {
	case 0:
		return nil, nil, 0
	case 1:
		if f.Offset() != 0 {
			return nil, nil, 0
		}

		e := a[0].AssignmentExpression
		f := a[0].Field()
		var b buf
		switch d := c.declaratorOf(e); {
		case d != nil && d.Type() == f.Type():
			b.w("(*(*%s)(%s))", c.typ(n, ft), unsafePointer(fmt.Sprintf("&%s", c.topExpr(w, e, nil, exprDefault))))
		default:
			v := c.f.newAutovar(n, f.Type())
			w.w("%s = %s;", v, c.topExpr(w, e, f.Type(), exprDefault))
			b.w("(*(*%s)(%s))", c.typ(n, ft), unsafePointer(fmt.Sprintf("&%s", v)))
		}
		return &b, ft, mode
	default:
		if f.Offset() != 0 {
			return nil, nil, 0
		}

		var commonField *cc.Field
		for _, v := range a {
			f := v.Field()
			if f == nil {
				if f = v.Parent().Field(); f == nil {
					return nil, nil, 0
				}
			}

			if f != commonField && commonField != nil {
				return nil, nil, 0
			}

			commonField = f
		}

		if commonField.Offset() != 0 {
			return nil, nil, 0
		}

		var b buf
		v := c.f.newAutovar(n, commonField.Type())
		w.w("%s = %s;", v, c.initializer(w, n, a, commonField.Type(), 0, ct.Undecay().Kind() == cc.Array))
		b.w("(*(*%s)(%s))", c.typ(n, ft), unsafePointer(fmt.Sprintf("&%s", v)))
		return &b, ft, mode
	}
}

func (c *ctx) postfixExpressionSelectUnionField(w writer, n *cc.PostfixExpression, t cc.Type, mode mode) (r *buf, rt cc.Type, rmode mode) {
	// PostfixExpression '.' IDENTIFIER
	switch mode {
	case exprDefault, exprSelect, exprLvalue, exprIndex:
		// ok
	default:
		return nil, nil, 0
	}

	f := n.Field()
	switch x := f.Type().Undecay().(type) {
	case *cc.ArrayType:
		return nil, nil, 0
	case *cc.PointerType:
		if x.Elem().Undecay().Kind() == cc.Array {
			return nil, nil, 0
		}
	}

	n0 := n
	var path []*cc.PostfixExpression
	union := -1
	for {
		if n.PostfixExpression.Type().Kind() == cc.Union {
			union = len(path)
		}
		path = append(path, n)
		x, ok := n.PostfixExpression.(*cc.PostfixExpression)
		if !ok {
			break
		}

		if x.Case != cc.PostfixExpressionSelect {
			break
		}

		n = x
	}
	if union < 0 || union == len(path)-1 {
		return nil, nil, 0
	}

	var b buf
	var off int64
	for _, v := range path[:union+1] {
		off += v.Field().Offset()
	}
	s := ""
	if off != 0 {
		s = fmt.Sprintf("+%d", off)
	}
	b.w("(*(*%s)(%s))", c.typ(n0.Token, f.Type()), unsafePointer(fmt.Sprintf("%s%s", c.topExpr(w, path[union].PostfixExpression, c.pvoid, exprUintptr), s)))
	return &b, f.Type(), mode
}

func (c *ctx) collectParentFields(n cc.Node, f *cc.Field, in cc.Type) (r []*cc.Field, ok bool) {
	g := f
	ok = true
	for p := g.ParentField(); p != nil; p = p.ParentField() {
		_, isUnion := p.Type().(*cc.UnionType)
		if isUnion && g.Index() != 0 {
			ok = false
		}
		if p.Type() == in || p.Type().IsCompatible(in) {
			break
		}

		r = append(r, p)
		g = p
	}
	return r, ok
}

func (c *ctx) parentFields(a []*cc.Field, b *buf, n cc.Node, f *cc.Field, in cc.Type) {
	for i := len(a) - 1; i >= 0; i-- {
		b.w("%s%s.", tag(field), c.fieldName(nil, a[i]))
	}
	b.w("%s%s", tag(field), c.fieldName(nil, f))
}

func (c *ctx) isLastStructOrUnionField(n cc.ExpressionNode) *cc.Field {
	for {
		switch x := n.(type) {
		case *cc.PostfixExpression:
			var f *cc.Field
			var t cc.Type
			switch x.Case {
			case cc.PostfixExpressionSelect: // PostfixExpression '.' IDENTIFIER
				f = x.Field()
				t = x.PostfixExpression.Type()
			case cc.PostfixExpressionPSelect: // PostfixExpression "->" IDENTIFIER
				f = x.Field()
				t = x.PostfixExpression.Type().(*cc.PointerType).Elem()
			}
			switch x := t.(type) {
			case *cc.StructType:
				if f.Index() == x.NumFields()-1 {
					return f
				}
			case *cc.UnionType:
				return f
			}
		}

		return nil
	}
}

func (c *ctx) declaratorOf(n cc.ExpressionNode) (r *cc.Declarator) {
	// defer func(n cc.ExpressionNode) { trc("%v: %q %T %p", n.Position(), cc.NodeSource(n), n, r) }(n)
	for n != nil {
		n = c.unparen(n)
		switch x := n.(type) {
		case *cc.PrimaryExpression:
			switch x.Case {
			case cc.PrimaryExpressionIdent: // IDENTIFIER
				switch y := x.ResolvedTo().(type) {
				case *cc.Declarator:
					return y
				case *cc.Parameter:
					return y.Declarator
				case *cc.Enumerator, nil:
					return nil
				default:
					c.err(errorf("TODO %T", y))
					return nil
				}
			case cc.PrimaryExpressionExpr: // '(' ExpressionList ')'
				n = x.ExpressionList
			default:
				return nil
			}
		case *cc.PostfixExpression:
			switch x.Case {
			case cc.PostfixExpressionPrimary: // PrimaryExpression
				n = x.PrimaryExpression
			default:
				return nil
			}
		case *cc.ExpressionList:
			if x == nil {
				return nil
			}

			for l := x; l != nil; l = l.ExpressionList {
				n = l.AssignmentExpression
			}
		case *cc.CastExpression:
			switch x.Case {
			case cc.CastExpressionUnary: // UnaryExpression
				n = x.UnaryExpression
			case cc.CastExpressionCast:
				if x.Type() != x.CastExpression.Type() {
					return nil
				}

				n = x.CastExpression
			default:
				return nil
			}
		case *cc.UnaryExpression:
			switch x.Case {
			case cc.UnaryExpressionPostfix: // PostfixExpression
				n = x.PostfixExpression
			default:
				return nil
			}
		case *cc.ConditionalExpression:
			switch x.Case {
			case cc.ConditionalExpressionLOr: // LogicalOrExpression
				n = x.LogicalOrExpression
			default:
				return nil
			}
		case *cc.AdditiveExpression:
			switch x.Case {
			case cc.AdditiveExpressionMul: // MultiplicativeExpression
				n = x.MultiplicativeExpression
			default:
				return nil
			}
		case *cc.InclusiveOrExpression:
			switch x.Case {
			case cc.InclusiveOrExpressionXor: // ExclusiveOrExpression
				n = x.ExclusiveOrExpression
			default:
				return nil
			}
		case *cc.ShiftExpression:
			switch x.Case {
			case cc.ShiftExpressionAdd:
				n = x.AdditiveExpression
			default:
				return nil
			}
		case *cc.AndExpression:
			switch x.Case {
			case cc.AndExpressionEq:
				n = x.EqualityExpression
			default:
				return nil
			}
		case *cc.MultiplicativeExpression:
			switch x.Case {
			case cc.MultiplicativeExpressionCast:
				n = x.CastExpression
			default:
				return nil
			}
		case *cc.EqualityExpression:
			switch x.Case {
			case cc.EqualityExpressionRel:
				n = x.RelationalExpression
			default:
				return nil
			}
		case *cc.RelationalExpression:
			switch x.Case {
			case cc.RelationalExpressionShift:
				n = x.ShiftExpression
			default:
				return nil
			}
		case *cc.LogicalOrExpression:
			switch x.Case {
			case cc.LogicalOrExpressionLAnd:
				n = x.LogicalAndExpression
			default:
				return nil
			}
		case *cc.AssignmentExpression:
			switch x.Case {
			case cc.AssignmentExpressionCond:
				n = x.ConditionalExpression
			default:
				return nil
			}
		case *cc.LogicalAndExpression:
			switch x.Case {
			case cc.LogicalAndExpressionOr:
				n = x.InclusiveOrExpression
			default:
				return nil
			}
		case *cc.ExclusiveOrExpression:
			switch x.Case {
			case cc.ExclusiveOrExpressionAnd:
				n = x.AndExpression
			default:
				return nil
			}
		case *cc.ConstantExpression:
			n = x.ConditionalExpression
		default:
			panic(todo("%T", n))
		}
	}
	return nil
}

func (c *ctx) postfixExpressionCall(w writer, n *cc.PostfixExpression, mode mode) (r *buf, rt cc.Type, rmode mode) {
	// trc("%v: call %q, callee %q, mode %v, pe type %s", n.Token.Position(), cc.NodeSource(n), cc.NodeSource(n.PostfixExpression), mode, n.PostfixExpression.Type())
	// defer func() {
	// 	trc("call %q -> %q %v %v", cc.NodeSource(n), r, rt, rmode)
	// }()
	pet := n.PostfixExpression.Type()
	var b buf
	var ft *cc.FunctionType
	var d *cc.Declarator
	var inlineFD *cc.FunctionDefinition
	switch d = c.declaratorOf(n.PostfixExpression); {
	case d != nil:
		switch d.Name() {
		case "alloca", "__builtin_alloca":
			if d.Linkage() == cc.External {
				c.f.callsAlloca = true
			}
		}
		if !c.task.hidden.has(d.Name()) {
			inlineFD = c.inlineFuncs[d]
		}
		switch x := d.Type().(type) {
		case *cc.PointerType:
			var ok bool
			if ft, ok = x.Elem().(*cc.FunctionType); !ok {
				c.err(errorf("TODO %T", x.Elem()))
				return
			}
		case *cc.FunctionType:
			defer func() {
				if r != nil {
					r.volatileOrAtomicHandled = true
				}
			}()
			ft = x
		default:
			c.err(errorf("TODO %T", d.Type()))
			return
		}
	default:
		pt, ok := pet.(*cc.PointerType)
		if !ok {
			c.err(errorf("TODO %T", pet))
			return
		}

		if ft, ok = pt.Elem().(*cc.FunctionType); !ok {
			c.err(errorf("TODO %T", pt.Elem()))
			return
		}
	}

	//	void f() {}
	//	void g(void) {}
	//	void h(int i) {}
	//
	//	int main () {
	//		f();
	//		g();
	//		h(42);
	//	}
	//
	// "f()"   ft.MinArgs()=0 ft.MaxArgs()=-1
	// "g()"   ft.MinArgs()=0 ft.MaxArgs()=0
	// "h(42)" ft.MinArgs()=1 ft.MaxArgs()=1

	if mode == exprCall {
		var rft *cc.FunctionType
		switch x := ft.Result().(type) {
		case *cc.PointerType:
			switch y := x.Elem().(type) {
			case *cc.FunctionType:
				rft = y
			default:
				c.err(errorf("TODO %T", y))
			}
		default:
			c.err(errorf("TODO %T", x))
		}
		if rft != nil {
			b.w("(*(*func%s)(%sunsafe.%sPointer(&struct{%[3]suintptr}{", c.signature(rft, false, false, true), tag(importQualifier), tag(preserve))

			defer func() {
				r.w("})))")
			}()
		}
	}

	var args []cc.ExpressionNode
	for l := n.ArgumentExpressionList; l != nil; l = l.ArgumentExpressionList {
		args = append(args, l.AssignmentExpression)
	}
	if len(args) < ft.MinArgs() {
		c.err(errorf("%v: too few arguments to function '%s', type '%v' in '%v'", c.pos(n.PostfixExpression), cc.NodeSource(n.PostfixExpression), ft, cc.NodeSource(n)))
		return &b, nil, 0
	}

	if len(args) > ft.MaxArgs() && ft.MaxArgs() >= 0 {
		c.err(errorf("%v: too many arguments to function '%s', type '%v' in '%v'", c.pos(n.PostfixExpression), cc.NodeSource(n.PostfixExpression), ft, cc.NodeSource(n)))
		return &b, nil, 0
	}

	// trc("%v: len(args) %v, ft.MaxArgs %v, ft.IsVariadic() %v, d != nil %v, d.IsSynthetic() %v, d.IsFuncDef() %v", n.Position(), len(args), ft.MaxArgs(), ft.IsVariadic(), d != nil, d.IsSynthetic(), d.IsFuncDef())
	if len(args) > ft.MaxArgs() && !ft.IsVariadic() && d != nil && !d.IsSynthetic() && d.IsFuncDef() {
		max := mathutil.Max(ft.MaxArgs(), 0)
		for _, v := range args[max:] {
			w.w("%s_ = %s;", tag(preserve), c.expr(w, v, nil, exprDefault))
		}
		args = args[:max]
	}
	ftp := ft.Parameters()
	params := c.normalizeParams(ftp)
	var xargs []*buf
	var xtypes []cc.Type
	for i, v := range args {
		mode := exprDefault
		var t cc.Type
		unconvert := false
		switch {
		case i < len(params):
			t = params[i].Type()
			if t.Kind() == cc.Enum {
				switch x := v.Value().(type) {
				case cc.Int64Value:
					unconvert = x >= 0
				case cc.UInt64Value:
					unconvert = true
				}
			}
		default:
			switch t = v.Type(); {
			case cc.IsIntegerType(t):
				t = cc.IntegerPromotion(t)
			case t.Kind() == cc.Float:
				t = c.ast.Double
			}
		}
		switch v.Type().Undecay().Kind() {
		case cc.Function:
			if d := c.declaratorOf(v); d != nil && d.IsFuncDef() {
				mode = exprUintptr
			}
		}
		var xarg *buf
		switch {
		case c.isVolatileOrAtomicExpr(v):
			xarg = c.checkVolatileExpr(w, v, t, mode)
		default:
			xarg = c.topExpr(w, v, t, mode)
		}
		if unconvert {
			xarg = c.uncovertEnum(xarg)
		}
		xargs = append(xargs, xarg)
		xtypes = append(xtypes, t)
	}
	switch {
	case inlineFD != nil:
		if len(params) != len(args) && !ft.IsVariadic() {
			c.err(errorf("TODO %v: %s %v", inlineFD.Position(), inlineFD.Declarator.Name(), inlineFD.Declarator.Type()))
			return
		}

		for _, v := range ft.Parameters() {
			if v.Declarator != nil {
				c.f.registerLocal(v.Declarator)
			}
		}

		var vaOff int64
		if c.pass == 1 && ft.IsVariadic() {
			n := 8 * (len(args) - ft.MinArgs() + 2)
			c.f.tlsAllocs = roundup(c.f.tlsAllocs, 8)
			vaOff = c.f.tlsAllocs
			vaOff = roundup(vaOff, 16)
			c.f.tlsAllocs += int64(n)
		}

		sv := c.f.inlineInfo
		nfo := &inlineInfo{
			args:   xargs,
			fd:     inlineFD,
			mode:   mode,
			params: params,
			parent: c.f.inlineInfo,
			vaOff:  vaOff,
		}
		c.f.inlineInfo = nfo

		defer func() { c.f.inlineInfo = sv }()

		for i, v := range params {
			var rp string
			switch d := v.Declarator; {
			case d.ReadCount() > 1 || d.WriteCount() != 0 || d.AddressTaken():
				rp = c.f.newAutovar(v, v.Type())
				w.w("%s = %s;", rp, xargs[i])
			case d.ReadCount() == 1:
				rp = string(xargs[i].b)
			case d.ReadCount() == 0:
				w.w("%s;", c.discardStr2(args[i], xargs[i]))
			// case d.WriteCount() != 0 || d.AddressTaken() || d.ReadCount() > 1:
			// 	panic(todo("%v:", n.Position()))
			default:
				panic(todo("r %v w %v a %v", d.ReadCount(), d.WriteCount(), d.AddressTaken()))
			}
			nfo.replacedParams = append(nfo.replacedParams, rp)
		}
		if ft.IsVariadic() {
			args := xargs[ft.MinArgs():]
			for i, v := range args {
				if i == 0 {
					w.w("\n%s%sVaList(%s", c.task.tlsQualifier, tag(preserve), bpOff(vaOff))
				}
				w.w(", %s ", v)
				if i == len(args)-1 {
					w.w(");")
				}
			}
		}

		if c.pass == 2 {
			for i, v := range ft.Parameters() {
				if d := v.Declarator; d != nil && c.f.declInfos.info(d).pinned() {
					w.w("*(*%s)(%s) = %s;", c.typ(n, d.Type()), unsafePointer(bpOff(c.f.declInfos.info(d).bpOff)), nfo.replacedParams[i])
				}
			}
		}
		for l := inlineFD.CompoundStatement.BlockItemList; l != nil; l = l.BlockItemList {
			c.blockItem(w, l.BlockItem)
		}
		if nfo.exit != "" {
			w.w("%s:", nfo.exit)
			if nfo.result != "" {
				b.w("(%s)", nfo.result)
			}
		}
		rt, rmode = ft.Result(), exprDefault
		if rt.Kind() == cc.Void {
			rmode = exprVoid
		}
		return &b, rt, rmode
	case c.f == nil:
		b.w("%s(%snil", c.expr(w, n.PostfixExpression, nil, exprCall), tag(preserve))
	default:
		b.w("%s(%stls", c.expr(w, n.PostfixExpression, nil, exprCall), tag(ccgo))
	}
	switch {
	case ft.IsVariadic():
		for _, v := range xargs[:ft.MinArgs()] {
			b.w(", %s", v)
		}
		switch {
		case len(xargs) == ft.MinArgs():
			b.w(", 0")
		default:
			b.w(", %s%sVaList(%s", c.task.tlsQualifier, tag(preserve), bpOff(c.f.tlsAllocs+8))
			var sz int64
			xt := xtypes[ft.MinArgs():]
			for i, v := range xargs[ft.MinArgs():] {
				sz += roundup(xt[i].Size(), 8)
				b.w(", %s", v)
			}
			c.f.maxVaListSize = mathutil.MaxInt64(c.f.maxVaListSize, sz)
			b.w(")")
		}
	default:
		for _, v := range xargs {
			b.w(", %s", v)
		}
	}
	b.w(")")
	rt, rmode = ft.Result(), exprDefault
	if rt.Kind() == cc.Void {
		rmode = exprVoid
	}
	return &b, rt, rmode
}

func (c *ctx) uncovertEnum(b *buf) *buf {
	s := string(b.bytes())
	for strings.HasPrefix(s, "(") && strings.HasSuffix(s, ")") {
		s = s[1 : len(s)-1]
	}
	if strings.HasSuffix(s, ")") && (strings.HasPrefix(s, tag(preserve)+"int32(") || strings.HasPrefix(s, tag(preserve)+"uint32(")) {
		b.b = []byte("(" + s[strings.IndexByte(s, '(')+1:])
	}
	return b
}

func (c *ctx) normalizeParams(params []*cc.Parameter) []*cc.Parameter {
	if len(params) == 1 && params[0].Type().Kind() == cc.Void {
		return params[1:]
	}

	return params
}

func (c *ctx) atomicStore(w writer, n cc.Node, p, v *buf, t cc.Type, mode mode) *buf {
	if assert && p.len() == 0 {
		c.err(errorf("TODO assertion failed %v:", n.Position()))
	}
	var b buf
	switch t.Kind() {
	case cc.Struct, cc.Union:
		switch t.Size() {
		//TODO case 1,2,4,8:
		default:
			switch mode {
			case exprVoid:
				w.w("(*(*%s)(%s)) = %s;", c.typ(n, t), unsafePointer(p), v)
			default:
				nm := c.f.newAutovarName()
				w.w("%s := %s;", nm, v)
				w.w("(*(*%s)(%s)) = %s;", c.typ(n, t), unsafePointer(p), nm)
				b.w("(%s)", nm)
			}
			return &b
		}
	}

	b.w("%sAtomicStoreP%s(%s, %s)", c.task.tlsQualifier, c.helper(n, t), p, v)
	return &b
}

func (c *ctx) atomicLoad(w writer, n cc.Node, p *buf, t cc.Type) *buf {
	if assert && p.len() == 0 {
		c.err(errorf("TODO assertion failed %v:", n.Position()))
	}
	var b buf
	switch t.Kind() {
	case cc.Struct, cc.Union:
		switch t.Size() {
		case 1, 2, 4, 8:
			if c.f != nil {
				nm := c.f.newAutovarName()
				w.w("%s := %sAtomicLoadPUint%d(%s);", nm, c.task.tlsQualifier, 8*t.Size(), p)
				b.w("(*(*%s)(%s))", c.typ(n, t), unsafeAddr(nm))
				return &b
			}

			fallthrough
		default:
			b.w("(*(*%s)(%s))", c.typ(n, t), unsafePointer(p))
		}
	}

	b.w("%sAtomicLoadP%s(%s)", c.task.tlsQualifier, c.helper(n, t), p)
	return &b
}

func (c *ctx) assignmentExpression(w writer, n *cc.AssignmentExpression, t cc.Type, mode mode) (r *buf, rt cc.Type, rmode mode) {
	var b buf
	switch n.Case {
	case cc.AssignmentExpressionCond: // ConditionalExpression
		c.err(errorf("TODO %v", n.Case))
	case cc.AssignmentExpressionAssign: // UnaryExpression '=' AssignmentExpression
		lv := c.isVolatileOrAtomicExpr(n.UnaryExpression)
		ut := n.UnaryExpression.Type()
		switch {
		case lv:
			switch mode {
			case exprVoid, exprDefault:
				defer func() { r.volatileOrAtomicHandled = true }()
				return c.atomicStore(w, n, c.topExpr(w, n.UnaryExpression, ut.Pointer(), exprUintptr), c.topExpr(w, n.AssignmentExpression, ut, exprDefault), ut, mode), ut, mode
			default:
				trc("%v: TODO %q, t %s, mode %v, case %v", n.Position(), cc.NodeSource(n), t, mode, n.Case)
			}
		}

		switch x := n.UnaryExpression.(type) {
		case *cc.PostfixExpression:
			switch x.Case {
			case cc.PostfixExpressionSelect:
				f := x.Field()
				if !f.IsBitfield() {
					break
				}

				//TODO do not pin/use pointer
				p := c.pin(n, c.expr(w, x.PostfixExpression, x.PostfixExpression.Type().Pointer(), exprUintptr))
				switch mode {
				case exprDefault:
					b.w("%sAssignBitFieldPtr%d%s(%s+%d, %s, %d, %d, %#0x)", c.task.tlsQualifier, f.AccessBytes()*8, c.helper(n, f.Type()), p, f.Offset(), c.expr(w, n.AssignmentExpression, f.Type(), exprDefault), f.ValueBits(), f.OffsetBits(), f.Mask())
					return &b, f.Type(), exprDefault
				case exprVoid:
					b.w("%sSetBitFieldPtr%d%s(%s+%d, %s, %d, %#0x)", c.task.tlsQualifier, f.AccessBytes()*8, c.helper(n, f.Type()), p, f.Offset(), c.expr(w, n.AssignmentExpression, f.Type(), exprDefault), f.OffsetBits(), f.Mask())
					return &b, n.Type(), exprVoid
				default:
					trc("%v: BITFIELD", n.Position())
					c.err(errorf("TODO %v", mode))
					return &b, rt, rmode
				}
			case cc.PostfixExpressionPSelect:
				f := x.Field()
				if !f.IsBitfield() {
					break
				}

				switch mode {
				case exprDefault:
					b.w("%sAssignBitFieldPtr%d%s(%s+%d, %s, %d, %d, %#0x)", c.task.tlsQualifier, f.AccessBytes()*8, c.helper(n, f.Type()), c.expr(w, x.PostfixExpression, nil, exprDefault), f.Offset(), c.expr(w, n.AssignmentExpression, f.Type(), exprDefault), f.ValueBits(), f.OffsetBits(), f.Mask())
					return &b, f.Type(), exprDefault
				case exprVoid:
					b.w("%sSetBitFieldPtr%d%s(%s+%d, %s, %d, %#0x)", c.task.tlsQualifier, f.AccessBytes()*8, c.helper(n, f.Type()), c.expr(w, x.PostfixExpression, nil, exprDefault), f.Offset(), c.expr(w, n.AssignmentExpression, f.Type(), exprDefault), f.OffsetBits(), f.Mask())
					return &b, n.Type(), exprVoid
				default:
					trc("%v: BITFIELD", n.Position())
					c.err(errorf("TODO %v", mode))
					return &b, rt, rmode
				}
			}
		}

		switch mode {
		case exprDefault, exprSelect:
			rt, rmode = n.Type(), mode
			v := c.f.newAutovar(n, n.UnaryExpression.Type())
			w.w("%s = %s;", v, c.checkVolatileExpr(w, n.AssignmentExpression, n.UnaryExpression.Type(), exprDefault))
			w.w("%s = %s;", c.expr(w, n.UnaryExpression, nil, exprDefault), v)
			b.w("%s", v)
		case exprVoid:
			switch x := n.UnaryExpression.(type) {
			case *cc.PostfixExpression:
				switch x.Case {
				case cc.PostfixExpressionSelect:
					if c.isVolatileOrAtomicExpr(x.PostfixExpression) {
						f := x.Field()
						if f.IsBitfield() {
							c.err(errorf("TODO %v", mode))
							break
						}

						bp := c.expr(w, x.PostfixExpression, x.PostfixExpression.Type().Pointer(), exprUintptr)
						if off := f.Offset(); off != 0 {
							bp.w("+%v", off)
						}
						w.w("%s", c.atomicStore(w, n, bp, c.expr(w, n.AssignmentExpression, f.Type(), exprDefault), f.Type(), mode))
						return &b, n.Type(), mode
					}
				case cc.PostfixExpressionPSelect:
					if c.isVolatileOrAtomicExpr(x) || c.isVolatileOrAtomicExpr(x.PostfixExpression) {
						f := x.Field()
						if f.IsBitfield() {
							c.err(errorf("TODO %v", mode))
							break
						}

						bp := c.expr(w, x.PostfixExpression, nil, exprDefault)
						if off := f.Offset(); off != 0 {
							bp.w("+%v", off)
						}
						defer func() { r.volatileOrAtomicHandled = true }()
						w.w("%s", c.atomicStore(w, n, bp, c.expr(w, n.AssignmentExpression, f.Type(), exprDefault), f.Type(), mode))
						return &b, n.Type(), mode
					}
				}
			}

			b.w("%s = ", c.expr(w, n.UnaryExpression, nil, exprLvalue))
			c.exprNestLevel--

			defer func() { c.exprNestLevel++ }()

			b.w("%s", c.expr(w, n.AssignmentExpression, n.UnaryExpression.Type(), exprDefault))
			w.w("%s;", &b)
			b.reset()
			return &b, n.Type(), exprVoid
		default:
			c.err(errorf("TODO %v", mode))
			// panic(todo(""))
		}
	case cc.AssignmentExpressionMul, // UnaryExpression "*=" AssignmentExpression
		cc.AssignmentExpressionDiv, // UnaryExpression "/=" AssignmentExpression
		cc.AssignmentExpressionMod, // UnaryExpression "%=" AssignmentExpression
		cc.AssignmentExpressionAdd, // UnaryExpression "+=" AssignmentExpression
		cc.AssignmentExpressionSub, // UnaryExpression "-=" AssignmentExpression
		cc.AssignmentExpressionLsh, // UnaryExpression "<<=" AssignmentExpression
		cc.AssignmentExpressionRsh, // UnaryExpression ">>=" AssignmentExpression
		cc.AssignmentExpressionAnd, // UnaryExpression "&=" AssignmentExpression
		cc.AssignmentExpressionXor, // UnaryExpression "^=" AssignmentExpression
		cc.AssignmentExpressionOr:  // UnaryExpression "|=" AssignmentExpression

		rt, rmode = n.Type(), mode
		op := n.Token.SrcStr()
		op = op[:len(op)-1]
		x, y := n.UnaryExpression.Type(), n.AssignmentExpression.Type()
		ct := c.usualArithmeticConversions(x, y)
		ut := n.UnaryExpression.Type()
		switch x := n.UnaryExpression.(type) {
		case *cc.PostfixExpression:
			switch x.Case {
			case cc.PostfixExpressionSelect:
				f := x.Field()
				if !f.IsBitfield() {
					break
				}

				p := c.pin(n, c.expr(w, x.PostfixExpression, x.PostfixExpression.Type().Pointer(), exprUintptr))
				bf, _, _ := c.bitField(w, n, p, f, exprDefault, false) //TODO atomic bit fields
				switch mode {
				case exprDefault, exprVoid:
					b.w("%sAssignBitFieldPtr%d%s(%s+%d, %s(%s(%s)%s%[7]s(%[10]s)), %d, %d, %#0x)",
						c.task.tlsQualifier, f.AccessBytes()*8, c.helper(n, ut), p, f.Offset(),
						c.typ(n, ut), c.typ(n, ct), bf,
						op,
						c.expr(w, n.AssignmentExpression, ut, exprDefault),
						f.ValueBits(), f.OffsetBits(), f.Mask(),
					)
					return c.reduceBitFieldValue(&b, f, f.Type(), rmode), rt, exprDefault
				default:
					trc("%v: BITFIELD %v", n.Position(), mode)
					c.err(errorf("TODO %v", mode))
					return &b, rt, rmode
				}
			case cc.PostfixExpressionPSelect:
				f := x.Field()
				if !f.IsBitfield() {
					break
				}

				p := c.expr(w, x.PostfixExpression, nil, exprDefault)
				bf, _, _ := c.bitField(w, n, p, f, exprDefault, false) //TODO atomic bit fields
				switch mode {
				case exprDefault, exprVoid:
					b.w("%sAssignBitFieldPtr%d%s(%s+%d, %s(%s(%s)%s%[7]s(%[10]s)), %d, %d, %#0x)",
						c.task.tlsQualifier, f.AccessBytes()*8, c.helper(n, ut), p, f.Offset(),
						c.typ(n, ut), c.typ(n, ct), bf,
						op,
						c.expr(w, n.AssignmentExpression, ut, exprDefault),
						f.ValueBits(), f.OffsetBits(), f.Mask(),
					)
					return c.reduceBitFieldValue(&b, f, f.Type(), rmode), rt, exprDefault
				default:
					trc("%v: BITFIELD", n.Position())
					c.err(errorf("TODO %v", mode))
					return &b, rt, rmode
				}
			}
		}

		defer func() { r.volatileOrAtomicHandled = true }() //TODO-
		var k, v string
		switch n.Case {
		case cc.AssignmentExpressionAdd: // UnaryExpression "+=" AssignmentExpression
			switch {
			case x.Kind() == cc.Ptr && cc.IsIntegerType(y):
				k = c.elemSize(n.UnaryExpression, "*")
			case cc.IsIntegerType(x) && y.Kind() == cc.Ptr:
				c.err(errorf("%v: TODO", pos(n))) // -
			}
		case cc.AssignmentExpressionSub: // UnaryExpression "-=" AssignmentExpression
			switch {
			case x.Kind() == cc.Ptr && cc.IsIntegerType(y):
				k = c.elemSize(n.UnaryExpression, "*")
			case x.Kind() == cc.Ptr && y.Kind() == cc.Ptr:
				k = c.elemSize(n.UnaryExpression, "/")
			}
		}
		switch mode {
		case exprDefault, exprVoid:
			switch d := c.declaratorOf(n.UnaryExpression); {
			case d != nil:
				if c.isVolatileOrAtomicExpr(n.UnaryExpression) {
					p := fmt.Sprintf("%sp%d", tag(ccgo), c.id())
					c.f.registerAutoVar(p, c.typ(n, ut.Pointer()))
					w.w("\n%s = %s;", p, c.expr(w, n.UnaryExpression, ut.Pointer(), exprUintptr))
					var bp, v buf
					bp.w("%s", p)
					v.w("%s %s %s%s", c.topExpr(w, n.AssignmentExpression, ut, exprDefault), op, c.atomicLoad(w, n, &bp, ut), k)
					defer func() { r.volatileOrAtomicHandled = true }()
					return c.atomicStore(w, n, &bp, &v, ut, mode), ut, mode
				}

				v = fmt.Sprintf("%s", c.expr(w, n.UnaryExpression, nil, exprDefault))
				switch {
				case ct.Kind() == ut.Kind():
					w.w("\n%s %s= %s%s;", v, op, c.topExpr(w, n.AssignmentExpression, ct, exprDefault), k)
				default:
					w.w("\n%s = %s((%s(%s)) %s ((%s)%s));", v, c.typ(n, ut), c.typ(n, ct), v, op, c.expr(w, n.AssignmentExpression, ct, exprDefault), k)
				}
			default:
				switch {
				case ct.Kind() == ut.Kind():
					switch {
					case mode == exprDefault:
						p := fmt.Sprintf("%sp%d", tag(ccgo), c.id())
						c.f.registerAutoVar(p, c.typ(n, ut.Pointer()))
						w.w("\n%s = %s;", p, c.expr(w, n.UnaryExpression, ut.Pointer(), exprUintptr))
						v = fmt.Sprintf("(*(*%s)(%s))", c.typ(n, ut), unsafePointer(p))
						w.w("\n%s %s= %s%s;", v, op, c.topExpr(w, n.AssignmentExpression, ct, exprDefault), k)
					default:
						switch x, ok := n.UnaryExpression.(*cc.PostfixExpression); {
						case ok && x.Case == cc.PostfixExpressionSelect:
							w.w("\n%s %s= %s%s", c.expr(w, n.UnaryExpression, n.UnaryExpression.Type(), exprDefault), op, c.topExpr(w, n.AssignmentExpression, ct, exprDefault), k)
						default:
							w.w("\n(*(*%s)(%s)) %s= %s%s;", c.typ(n, ut), unsafePointer(c.topExpr(w, n.UnaryExpression, ut.Pointer(), exprUintptr)), op, c.topExpr(w, n.AssignmentExpression, ct, exprDefault), k)
						}
					}
				default:
					p := fmt.Sprintf("%sp%d", tag(ccgo), c.id())
					c.f.registerAutoVar(p, c.typ(n, ut.Pointer()))
					w.w("\n%s = %s;", p, c.expr(w, n.UnaryExpression, ut.Pointer(), exprUintptr))
					v = fmt.Sprintf("(*(*%s)(%s))", c.typ(n, ut), unsafePointer(p))
					w.w("\n%s = %s((%s(%s)) %s ((%s)%s));", v, c.typ(n, ut), c.typ(n, ct), v, op, c.expr(w, n.AssignmentExpression, ct, exprDefault), k)
				}
			}
			if mode == exprDefault {
				b.w("%s", v)
			}
		default:
			c.err(errorf("TODO %v", mode))
		}
	default:
		c.err(errorf("internal error %T %v", n, n.Case))
	}
	return &b, rt, rmode
}

func (c *ctx) expressionList(w writer, n *cc.ExpressionList, t cc.Type, mode mode) (r *buf, rt cc.Type, rmode mode) {
	for ; n != nil; n = n.ExpressionList {
		switch {
		case n.ExpressionList == nil:
			// trc("%v: %T", n.AssignmentExpression.Position(), n.AssignmentExpression)
			return c.expr0(w, n.AssignmentExpression, t, mode)
		default:
			w.w("%s%s;", sep(n.AssignmentExpression), c.discardStr2(n.AssignmentExpression, c.topExpr(w, n.AssignmentExpression, nil, exprVoid)))
		}
	}
	c.err(errorf("TODO internal error", n))
	return r, rt, rmode
}

func (c *ctx) primaryExpression(w writer, n *cc.PrimaryExpression, t cc.Type, mode mode) (r *buf, rt cc.Type, rmode mode) {
	isVolatileOrAtomicExpr := c.isVolatileOrAtomicExpr(n)
	if isVolatileOrAtomicExpr {
		switch {
		case mode == exprUintptr:
			defer func() { r.volatileOrAtomicHandled = true }()
		}
	}

	defer func() {
		if r != nil {
			r.n = n
		}
	}()

	var b buf
out:
	switch n.Case {
	case cc.PrimaryExpressionIdent: // IDENTIFIER
		rt, rmode = n.Type(), mode
		switch x := n.ResolvedTo().(type) {
		case *cc.Declarator:
			nm := x.Name()
			if c.f != nil {
				if c.f.inlineInfo != nil && nm == "__func__" {
					b.w("%q", c.f.d.Name()+"\x00")
					return &b, rt, exprDefault
				}

				for nfo := c.f.inlineInfo; nfo != nil; nfo = nfo.parent {
					for i, v := range nfo.params {
						if v.Declarator == x {
							switch {
							case mode == exprVoid:
								w.w("%s_ = %s;", tag(preserve), nfo.replacedParams[i])
							case mode == exprUintptr:
								c.f.declInfos.takeAddress(x)
								b.w("(%s)", bpOff(c.f.declInfos.info(x).bpOff))
								return &b, v.Type().Pointer(), exprUintptr
							default:
								b.w("(%s)", nfo.replacedParams[i])
							}
							return &b, v.Type(), exprDefault
						}
					}
				}
			}

			if isVolatileOrAtomicExpr {
				if mode == exprDefault {
					defer func() { r.volatileOrAtomicHandled = true }()
					bp := c.expr(w, n, x.Type().Pointer(), exprUintptr)
					return c.atomicLoad(w, n, bp, x.Type()), x.Type(), mode
				}
			}

			linkName := c.declaratorTag(x) + nm
			if c.pass == 2 {
				if nm := c.f.locals[x]; nm != "" {
					linkName = nm
				}
			}
			c.externsMentioned[nm] = struct{}{}
			b.n = x
			var info *declInfo
			if c.f != nil {
				info = c.f.declInfos.info(x)
			}
			switch {
			case info != nil && info.pinned():
				switch mode {
				case exprLvalue, exprSelect, exprIndex:
					b.w("(*(*%s)(%s))", c.typ(n, x.Type()), unsafePointer(bpOff(info.bpOff)))
				case exprUintptr:
					rt = x.Type().Pointer()
					b.w("%s", bpOff(info.bpOff))
				case exprDefault, exprVoid:
					rmode = exprDefault
					switch _, ok := n.Type().Undecay().(*cc.ArrayType); {
					case ok && !x.IsParam():
						b.w("%s", bpOff(info.bpOff))
					default:
						b.w("(*(*%s)(%s))", c.typ(n, x.Type()), unsafePointer(bpOff(info.bpOff)))
					}
				case exprCall:
					switch y := x.Type().Undecay().(type) {
					case *cc.PointerType:
						if ft, ok := y.Elem().(*cc.FunctionType); ok {
							b.w("(*(*func%s)(%s))", c.signature(ft, false, false, true), unsafePointer(bpOff(info.bpOff)))
							break
						}

						c.err(errorf("TODO %T:", y.Elem()))
					case *cc.FunctionType:
						b.w("(*(*func%s)(%s))", c.signature(y, false, false, true), unsafePointer(bpOff(info.bpOff)))
					default:
						c.err(errorf("TODO %T", y))
					}
				default:
					c.err(errorf("TODO %v %v:", mode, n.Position()))
				}
			default:
				switch mode {
				case exprVoid:
					r, rt, _ = c.primaryExpression(w, n, t, exprDefault)
					return r, rt, exprDefault
				case exprDefault:
					switch x.Type().Kind() {
					case cc.Array:
						p := &buf{n: x}
						p.w("%s", linkName)
						b.w("%suintptr(%s)", tag(preserve), unsafeAddr(c.pin(n, p)))
					case cc.Function:
						b.w("%s%s(%s)", tag(preserve), ccgoFP, linkName)
					default:
						switch {
						case n.Type().Kind() != t.Kind() && t.Kind() != cc.Void && t.Kind() != cc.Ptr:
							if isVolatileOrAtomicExpr {
								rt = x.Type()
								defer func() { r.volatileOrAtomicHandled = true }()
								b.w("(%s(%s))", c.verifyTyp(n, t), c.atomicLoad(w, n, c.expr(w, n, rt.Pointer(), exprUintptr), rt))
								break
							}

							b.w("(%s(%s))", c.verifyTyp(n, t), linkName)
						default:
							if isVolatileOrAtomicExpr {
								rt = x.Type()
								defer func() { r.volatileOrAtomicHandled = true }()
								return c.atomicLoad(w, n, c.expr(w, n, rt.Pointer(), exprUintptr), rt), rt, mode
							}

							b.w("(%s)", linkName)
						}
					}
				case exprLvalue, exprSelect:
					b.w("%s", linkName)
				case exprCall:
					switch y := x.Type().(type) {
					case *cc.FunctionType:
						if !c.task.strictISOMode && !c.task.freeStanding && !c.task.noBuiltin {
							if _, ok := forcedBuiltins[nm]; ok {
								nm = "__builtin_" + nm
								linkName = c.declaratorTag(x) + nm
							}
						}
						b.w("%s", linkName)
					case *cc.PointerType:
						switch z := y.Elem().(type) {
						case *cc.FunctionType:
							rmode = exprUintptr
							b.w("%s", linkName)
						default:
							// trc("%v: %s", x.Position(), cc.NodeSource(n))
							c.err(errorf("TODO %T", z))
						}
					default:
						c.err(errorf("TODO %T", y))
					}
				case exprIndex:
					switch x.Type().Undecay().Kind() {
					case cc.Array:
						b.w("%s", linkName)
					default:
						panic(todo(""))
						c.err(errorf("TODO %v", mode))
					}
				case exprUintptr:
					rt = x.Type().Pointer()
					switch {
					case x.Type().Kind() == cc.Function:
						b.w("%s%s(%s)", tag(preserve), ccgoFP, linkName)
					default:
						switch _, ok := c.isVLA(x.Type()); {
						case ok && c.f != nil:
							b.w("(%s)", linkName)
						default:
							p := &buf{n: x}
							p.w("%s", linkName)
							b.w("%suintptr(%s)", tag(preserve), unsafeAddr(c.pin(n, p)))
						}
					}
				default:
					c.err(errorf("TODO %v", mode))
				}
			}
		case *cc.Enumerator:
			switch {
			case x.ResolvedIn().Parent == nil:
				rt, rmode = t, exprDefault
				switch {
				case !cc.IsSignedInteger(t) && c.isNegative(n.Value()):
					b.w("(%s%s%sFrom%s(%s%s))", c.task.tlsQualifier, tag(preserve), c.helper(n, t), c.helper(n, n.Type()), tag(enumConst), x.Token.Src())
				default:
					b.w("(%s(%s%s))", c.verifyTyp(n, t), tag(enumConst), x.Token.Src())
				}
			default:
				rt, rmode = n.Type(), exprDefault
				b.w("%v", n.Value())
			}
		case nil:
			switch mode {
			case exprCall:
				b.w("%s%s", tag(external), n.Token.Src())
				break out
			default:
				c.err(errorf("TODO %v: %v", n.Position(), mode))
				break out
			}
		default:
			c.err(errorf("TODO %T", x))
		}
	case cc.PrimaryExpressionInt: // INTCONST
		return c.primaryExpressionIntConst(w, n, t, mode)
	case cc.PrimaryExpressionFloat: // FLOATCONST
		return c.primaryExpressionFloatConst(w, n, t, mode)
	case cc.PrimaryExpressionChar: // CHARCONST
		return c.primaryExpressionCharConst(w, n, t, mode)
	case cc.PrimaryExpressionLChar: // LONGCHARCONST
		return c.primaryExpressionLCharConst(w, n, t, mode)
	case cc.PrimaryExpressionString: // STRINGLITERAL
		return c.primaryExpressionStringConst(w, n, t, mode)
	case cc.PrimaryExpressionLString: // LONGSTRINGLITERAL
		return c.primaryExpressionLStringConst(w, n, t, mode)
	case cc.PrimaryExpressionExpr: // '(' ExpressionList ')'
		if c.exprNestLevel == 1 && n.Type() == t {
			c.exprNestLevel--

			defer func() { c.exprNestLevel++ }()
		}

		defer func() { r.volatileOrAtomicHandled = true }()
		return c.expr0(w, n.ExpressionList, nil, mode)
	case cc.PrimaryExpressionStmt: // '(' CompoundStatement ')'
		c.exprStmtLevel++

		defer func() { c.exprStmtLevel-- }()

		// trc("%v: %v %s", n.Position(), n.Type(), cc.NodeSource(n))
		switch n.Type().Kind() {
		case cc.Void:
			rt, rmode = n.Type(), exprVoid
			c.compoundStatement(w, n.CompoundStatement, false, "")
		default:
			rt, rmode = n.Type(), exprDefault
			v := c.f.newAutovar(n, n.Type())
			c.compoundStatement(w, n.CompoundStatement, false, v)
			if c.exprStmtLevel == 1 {
				b.w("%s", v)
			}
		}
	case cc.PrimaryExpressionGeneric: // GenericSelection
		return c.expr0(w, n.GenericSelection.Associated().AssignmentExpression, n.Type(), mode)
	default:
		c.err(errorf("internal error %T %v", n, n.Case))
	}
	return &b, rt, rmode
}

func (c *ctx) utf16(n cc.Node, s cc.UTF16StringValue) string {
	b := bytes.NewBuffer(make([]byte, 0, 2*len(s)))
	bo := c.ast.ABI.ByteOrder
	for _, v := range s {
		if err := binary.Write(b, bo, v); err != nil {
			c.err(errorf("%v: %v", n.Position(), err))
			return ""
		}
	}
	return b.String()
}

func (c *ctx) utf32(n cc.Node, s cc.UTF32StringValue) string {
	b := bytes.NewBuffer(make([]byte, 0, 4*len(s)))
	bo := c.ast.ABI.ByteOrder
	for _, v := range s {
		if err := binary.Write(b, bo, v); err != nil {
			c.err(errorf("%v: %v", n.Position(), err))
			return ""
		}
	}
	return b.String()
}

func (c *ctx) primaryExpressionLStringConst(w writer, n *cc.PrimaryExpression, t cc.Type, mode mode) (r *buf, rt cc.Type, rmode mode) {
	var b buf
	switch x := n.Type().Undecay().(type) {
	case *cc.ArrayType:
		switch y := t.(type) {
		case *cc.ArrayType:
			switch z := n.Value().(type) {
			case cc.UTF16StringValue:
				for len(z) != 0 && z[len(z)-1] == 0 {
					z = z[:len(z)-1]
				}
				b.w("%s{", c.typ(n, y))
				for _, c := range z {
					b.w("%s, ", strconv.QuoteRuneToASCII(rune(c)))
				}
				b.w("}")
			case cc.UTF32StringValue:
				for len(z) != 0 && z[len(z)-1] == 0 {
					z = z[:len(z)-1]
				}
				b.w("%s{", c.typ(n, y))
				for _, c := range z {
					b.w("%s, ", strconv.QuoteRune(c))
				}
				b.w("}")
			default:
				c.err(errorf("TODO %T", z))
			}
		case *cc.PointerType:
			switch z := n.Value().(type) {
			case cc.UTF16StringValue:
				b.w("%q", c.utf16(n, z))
			case cc.UTF32StringValue:
				b.w("%q", c.utf32(n, z))
			default:
				c.err(errorf("TODO %T", z))
			}
		default:
			c.err(errorf("TODO %T", y))
		}
	default:
		c.err(errorf("TODO %T", x))
	}
	return &b, t, exprDefault
}

func (c *ctx) primaryExpressionStringConst(w writer, n *cc.PrimaryExpression, t cc.Type, mode mode) (r *buf, rt cc.Type, rmode mode) {
	s := n.Value()
	var b buf
	switch x := n.Type().Undecay().(type) {
	case *cc.ArrayType:
		switch {
		case c.isCharType(x.Elem()):
			s := string(s.(cc.StringValue))
			//TODO bug in cc/v4/cpp.go:
			//
			//	int main() {
			//	  /* Test data is:
			//	   *   <?xml version='1.0' encoding='utf-16'?>
			//	   *   <a><![CDATA[hello]]></a>
			//	   */
			//	  const char text[]
			//	      = "\0<\0?\0x\0m\0l\0"
			//	        " \0v\0e\0r\0s\0i\0o\0n\0=\0'\0\x31\0.\0\x30\0'\0"
			//	        " \0e\0n\0c\0o\0d\0i\0n\0g\0=\0'\0u\0t\0f\0-\0"
			//	        "1\0"
			//	        "6\0'"
			//	        "\0?\0>\0\n"
			//	        "\0<\0a\0>\0<\0!\0[\0C\0D\0A\0T\0A\0[\0h\0e\0l\0l\0o\0]\0]\0>\0<\0/\0a\0>";
			//
			//	  char *p = text;
			//	  for(int i = 0; i < sizeof(text); i++) {
			//		  __builtin_printf("%3d: 0x%02x\n", i, (unsigned)*p++);
			//	  }
			//	}
		out:
			switch t.Kind() {
			case cc.Array:
				to := t.(*cc.ArrayType)
				max := to.Len()
				a := []byte(s)
				for len(a) != 0 && a[len(a)-1] == 0 {
					a = a[:len(a)-1]
				}
				b.w("%s{", c.typ(n, to))
				for i := 0; i < len(a) && int64(i) < max; i++ {
					b.w("%s, ", c.stringCharConst(a[i], to.Elem()))
				}
				b.w("}")
			case cc.Ptr:
				el := t.(*cc.PointerType)
				switch sz := el.Elem().Size(); sz {
				case 1:
					b.w("%q", s)
					break out
				case 2, 4:
					if s == "\x00" {
						b.w("%q", strings.Repeat("\x00", int(sz)))
						break out
					}
				}

				c.err(errorf("%v: TODO %s %s %s %q", pos(n), x, t, el, s))
			default:
				if cc.IsIntegerType(t) {
					b.w("(%s(%q))", c.typ(n, t), s)
					break
				}

				trc("%v: %s <- %q, convert to %s", n.Position(), x, s, t)
				c.err(errorf("TODO %s", t))
			}
		default:
			c.err(errorf("%v: TODO", pos(n)))
		}
	default:
		c.err(errorf("TODO %T", x))
	}
	return &b, t, exprDefault
}

func (c *ctx) primaryExpressionLCharConst(w writer, n *cc.PrimaryExpression, t cc.Type, mode mode) (r *buf, rt cc.Type, rmode mode) {
	rt, rmode = t, exprDefault
	var b buf
	src := n.Token.SrcStr()
	lit := src[1:] // L
	val, err := strconv.Unquote(lit)
	if err != nil {
		switch {
		case strings.HasPrefix(lit, `'\`) && isOctalString(lit[2:len(lit)-1]):
			lit = fmt.Sprintf(`'\%03o'`, n.Value())
			if _, err = strconv.Unquote(lit); err != nil {
				lit = fmt.Sprintf(`'\u%04x'`, n.Value())
			}
		case src == `'\"'`:
			lit = `'"'`
		}
		if val, err = strconv.Unquote(lit); err != nil {
			c.err(errorf("TODO `%s` -> %s", lit, err))
			return &b, rt, rmode
		}
	}

	ch := []rune(val)[0]
	switch x := n.Value().(type) {
	case cc.Int64Value:
		if rune(x) != ch {
			c.err(errorf("TODO `%s` -> |% x|, exp %#0x", lit, val, x))
			return &b, rt, rmode
		}
	case cc.UInt64Value:
		if rune(x) != ch {
			c.err(errorf("TODO `%s` -> |% x|, exp %#0x", lit, val, x))
			return &b, rt, rmode
		}
	}

	isPositive := true
	v := n.Value()
	var want uint64
	switch x := v.(type) {
	case cc.Int64Value:
		isPositive = x >= 0
		want = uint64(x)
	case cc.UInt64Value:
		want = uint64(x)
	default:
		c.err(errorf("TODO %T", x))
		return &b, rt, rmode
	}

	switch {
	case c.exprNestLevel == 1:
		cv := v.Convert(t)
		if cv == v {
			b.w("(%s(%s))", c.verifyTyp(n, t), lit)
			break
		}

		if i, ok := v.(cc.Int64Value); ok && !cc.IsSignedInteger(t) && i >= 0 && cv == cc.UInt64Value(want) {
			b.w("(%s(%s))", c.verifyTyp(n, t), lit)
			break
		}

		if t.Kind() == cc.Ptr && isPositive && cv == cc.UInt64Value(want) {
			b.w("(%s(%s))", c.verifyTyp(n, t), lit)
			break
		}

		fallthrough
	default:
		b.w("(%s%s%sFromInt32(%s))", c.task.tlsQualifier, tag(preserve), c.helper(n, t), lit)
	}
	return &b, rt, rmode
}

func (c *ctx) primaryExpressionCharConst(w writer, n *cc.PrimaryExpression, t cc.Type, mode mode) (r *buf, rt cc.Type, rmode mode) {
	rt, rmode = t, exprDefault
	var b buf
	src := n.Token.SrcStr()
	lit := src
	val, err := strconv.Unquote(lit)
	if err != nil {
		switch {
		case strings.HasPrefix(src, `'\`) && isOctalString(src[2:len(src)-1]):
			lit = fmt.Sprintf(`'\%03o'`, n.Value())
		case src == `'\"'`:
			lit = `'"'`
		case src == `'\?'`:
			lit = `'?'`
		}
		if val, err = strconv.Unquote(lit); err != nil {
			c.err(errorf("TODO `%s` -> %s", lit, err))
			return &b, rt, rmode
		}
	}
	if len(val) != 1 {
		c.err(errorf("TODO `%s` -> |% x|", lit, val))
		return &b, rt, rmode
	}

	ch := val[0]
	switch x := n.Value().(type) {
	case cc.Int64Value:
		if byte(x) != ch {
			c.err(errorf("TODO `%s` -> |% x|, exp %#0x", lit, val, x))
			return &b, rt, rmode
		}
	case cc.UInt64Value:
		if byte(x) != ch {
			c.err(errorf("TODO `%s` -> |% x|, exp %#0x", lit, val, x))
			return &b, rt, rmode
		}
	}

	isPositive := true
	v := n.Value()
	var want uint64
	switch x := v.(type) {
	case cc.Int64Value:
		isPositive = x >= 0
		want = uint64(x)
	case cc.UInt64Value:
		want = uint64(x)
	default:
		c.err(errorf("TODO %T", x))
		return &b, rt, rmode
	}

	switch {
	case c.exprNestLevel == 1:
		cv := v.Convert(t)
		if cv == v {
			b.w("(%s(%s))", c.verifyTyp(n, t), lit)
			break
		}

		if i, ok := v.(cc.Int64Value); ok && !cc.IsSignedInteger(t) && i >= 0 && cv == cc.UInt64Value(want) {
			b.w("(%s(%s))", c.verifyTyp(n, t), lit)
			break
		}

		if t.Kind() == cc.Ptr && isPositive && cv == cc.UInt64Value(want) {
			b.w("(%s(%s))", c.verifyTyp(n, t), lit)
			break
		}

		fallthrough
	default:
		b.w("(%s%s%sFromUint8(%s))", c.task.tlsQualifier, tag(preserve), c.helper(n, t), lit)
	}
	return &b, rt, rmode
}

func (c *ctx) normalizedMacroReplacementList0(m *cc.Macro) (r []cc.Token) {
	for _, v := range m.ReplacementList() {
		switch v.Ch {
		case ' ', '\n', '\t', '\r', '\f':
			// nop
		default:
			r = append(r, v)
		}
	}
	for len(r) != 0 && r[0].Ch == '(' && r[len(r)-1].Ch == ')' {
		r = r[1 : len(r)-1]
	}
	if len(r) == 1 {
		return r[:1]
	}

	return nil
}

func (c *ctx) normalizedMacroReplacementList(m *cc.Macro) string {
	if a := c.normalizedMacroReplacementList0(m); len(a) == 1 {
		return a[0].SrcStr()
	}

	return ""
}

func (c *ctx) macro(n *cc.PrimaryExpression) (nm, lit string) {
	m := n.Macro()
	if m == nil {
		return "", ""
	}

	nm = m.Name.SrcStr()
	if !c.macrosEmited.has(nm) {
		return "", ""
	}

	if lit := c.normalizedMacroReplacementList(m); lit != "" {
		return nm, lit
	}

	return "", ""
}

func (c *ctx) primaryExpressionIntConst(w writer, n *cc.PrimaryExpression, t cc.Type, mode mode) (r *buf, rt cc.Type, rmode mode) {
	rt, rmode = t, exprDefault
	var b buf
	src := n.Token.SrcStr()
	lit := strings.TrimRight(src, "uUlL")
	isPositive := true
	v := n.Value()
	var want uint64
	switch x := v.(type) {
	case cc.Int64Value:
		isPositive = x >= 0
		want = uint64(x)
	case cc.UInt64Value:
		want = uint64(x)
	default:
		c.err(errorf("TODO %T", x))
		return &b, rt, rmode
	}

	val, err := strconv.ParseUint(lit, 0, 64)
	if err != nil {
		c.err(errorf("TODO `%s` -> %s", lit, err))
		return &b, rt, rmode
	}

	if val != want {
		c.err(errorf("TODO `%s` -> got %v, want %v", lit, val, want))
		return &b, rt, rmode
	}

	if nm, s := c.macro(n); s == lit {
		// trc("%v: %q %q -> %s%s", n.Position(), cc.NodeSource(n), lit, tag(macro), nm) //TODO-DBG
		lit = fmt.Sprintf("%s%s", tag(macro), nm)
	}
	if t.Kind() == cc.Void {
		b.w("(%s)", lit)
		return &b, rt, rmode
	}

	cv := v.Convert(t)
	switch {
	case c.exprNestLevel == 1:
		if cv == v {
			switch {
			case c.f != nil && c.isZero(v):
				b.w("(%s)", lit)
			default:
				b.w("(%s(%s))", c.verifyTyp(n, t), lit)
			}
			break
		}

		if i, ok := v.(cc.Int64Value); ok && !cc.IsSignedInteger(t) && i >= 0 && cv == cc.UInt64Value(want) {
			b.w("(%s(%s))", c.verifyTyp(n, t), lit)
			break
		}

		if t.Kind() == cc.Ptr && isPositive && cv == cc.UInt64Value(want) {
			b.w("(%s(%s))", c.verifyTyp(n, t), lit)
			break
		}

		fallthrough
	default:
		b.w("(%s%s%sFrom%s(%s))", c.task.tlsQualifier, tag(preserve), c.helper(n, t), c.helper(n, n.Type()), lit)
	}
	return &b, rt, rmode
}

func (c *ctx) primaryExpressionFloatConst(w writer, n *cc.PrimaryExpression, t cc.Type, mode mode) (r *buf, rt cc.Type, rmode mode) {
	var b buf
	rt, rmode = t, exprDefault
out:
	switch x := n.Value().(type) {
	case *cc.LongDoubleValue:
		b.w("(%s%s%sFrom%s(%v))", c.task.tlsQualifier, tag(preserve), c.helper(n, t), c.helper(n, c.ast.Double), (*big.Float)(x))
	case cc.Float64Value:
		lit := string(n.Token.Src())
		if c.exprNestLevel == 1 && !strings.HasSuffix(strings.ToLower(lit), "f") {
			switch t.Kind() {
			case cc.Double, cc.Float:
				switch nm, s := c.macro(n); {
				case s == n.Token.SrcStr():
					b.w("(%s(%s%s))", c.verifyTyp(n, t), tag(macro), nm)
				default:
					b.w("(%s(%v))", c.verifyTyp(n, t), x)
				}
				break out
			}
		}

		switch nm, s := c.macro(n); {
		case s == n.Token.SrcStr():
			b.w("(%s%s%sFrom%s(%s%s))", c.task.tlsQualifier, tag(preserve), c.helper(n, t), c.helper(n, n.Type()), tag(macro), nm)
		default:
			b.w("(%s%s%sFrom%s(%v))", c.task.tlsQualifier, tag(preserve), c.helper(n, t), c.helper(n, n.Type()), x)
		}
	case cc.Complex64Value:
		b.w("(%s%s%sFrom%s(%v))", c.task.tlsQualifier, tag(preserve), c.helper(n, t), c.helper(n, n.Type()), x)
	case cc.Complex128Value:
		b.w("(%s%s%sFrom%s(%v))", c.task.tlsQualifier, tag(preserve), c.helper(n, t), c.helper(n, n.Type()), x)
	default:
		c.err(errorf("TODO %s %T from=%v t=%v mode=%v", cc.NodeSource(n), x, n.Type(), t, mode))
	}
	return &b, rt, rmode
}

func (c *ctx) stringCharConst(b byte, t cc.Type) string {
	switch {
	case b >= ' ' && b < 0x7f:
		return strconv.QuoteRuneToASCII(rune(b))
	case cc.IsSignedInteger(t):
		return fmt.Sprint(int8(b))
	default:
		return fmt.Sprint(b)
	}
}

func (c *ctx) isVolatileOrAtomicExpr(n cc.ExpressionNode) bool {
	if !n.Type().Attributes().IsVolatile() {
		return false
	}

	if d := c.declaratorOf(n); d != nil /*TODO && !d.AddressTaken() */ && d.StorageDuration() == cc.Automatic {
		return false
	}

	switch x := n.(type) {
	case *cc.AssignmentExpression:
		return c.isVolatileOrAtomicExpr(x.UnaryExpression)
	case *cc.PostfixExpression:
		switch x.Case {
		case cc.PostfixExpressionDec, cc.PostfixExpressionInc:
			if d := c.declaratorOf(x.PostfixExpression); d != nil /* && !d.AddressTaken() */ && d.StorageDuration() == cc.Automatic {
				return false
			}
		}
	}

	return true
}
