// Copyright 2022 The CCGO Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// ~/src/modernc.org/ccorpus2/

package ccgo // import "modernc.org/ccgo/v4/lib"

import (
	"bytes"
	"fmt"
	"reflect"
	"sort"

	"modernc.org/cc/v4"
)

var (
	zeroFuncPtr = []byte(fmt.Sprintf("(%suintptr(0))", tag(preserve)))
)

type initPatch struct {
	d   *cc.Declarator
	off int64
	b   *buf
}

func (c *ctx) initializerOuter(w writer, n *cc.Initializer, t cc.Type) (r *buf) {
	a := c.initalizerFlatten(n, nil)
	return c.initializer(w, n, a, t, 0, false)
}

func (c *ctx) initalizerFlatten(n *cc.Initializer, a []*cc.Initializer) (r []*cc.Initializer) {
	r = a
	switch n.Case {
	case cc.InitializerExpr: // AssignmentExpression
		return append(r, n)
	case cc.InitializerInitList: // '{' InitializerList ',' '}'
		for l := n.InitializerList; l != nil; l = l.InitializerList {
			r = append(r, c.initalizerFlatten(l.Initializer, nil)...)
		}
	default:
		c.err(errorf("internal error %T %v", n, n.Case))
	}
	return r
}

func (c *ctx) initializer(w writer, n cc.Node, a []*cc.Initializer, t cc.Type, off0 int64, arrayElem bool) (r *buf) {
	if cc.IsScalarType(t) {
		if len(a) == 0 {
			c.err(errorf("TODO"))
			return nil
		}

		in := a[0]
		if in.Offset()-off0 != 0 && in.Len() == 1 {
			c.err(errorf("TODO"))
			return nil
		}

		if t.Kind() == cc.Ptr && in.AssignmentExpression.Type().Undecay().Kind() == cc.Array {
			switch x := c.unparen(in.AssignmentExpression).(type) {
			case *cc.PostfixExpression:
				if x.Case != cc.PostfixExpressionComplit {
					break
				}

				t := in.AssignmentExpression.Type().Undecay().(*cc.ArrayType)
				r = c.topExpr(w, in.AssignmentExpression, t, exprDefault)
				switch {
				case c.initPatch != nil:
					nm := fmt.Sprintf("%s__ccgo_init_%d", tag(preserve), c.id())
					w.w("\nvar %s = %s;\n\n", nm, r)
					var b buf
					b.w("%suintptr(%s)", tag(preserve), unsafeAddr(nm))
					return &b
				default:
					return r
				}
			}
		}
		r = c.topExpr(w, in.AssignmentExpression, t, exprDefault)
		if t.Kind() == cc.Ptr && c.initPatch != nil && (t.Kind() == cc.Ptr && t.(*cc.PointerType).Elem().Kind() == cc.Function || c.mentionsFunc(in.AssignmentExpression)) {
			c.initPatch(off0, r)
			var b buf
			b.w("%s", zeroFuncPtr)
			return &b
		}

		return r
	}

	switch x := t.(type) {
	case *cc.ArrayType:
		if len(a) == 1 && a[0].Type().Kind() == cc.Array && a[0].Value() != cc.Unknown {
			return c.expr(w, a[0].AssignmentExpression, t, exprDefault)
		}

		return c.initializerArray(w, n, a, x, off0)
	case *cc.StructType:
		if len(a) == 1 && a[0].Type().Kind() == cc.Struct {
			return c.expr(w, a[0].AssignmentExpression, t, exprDefault)
		}

		return c.initializerStruct(w, n, a, x, off0)
	case *cc.UnionType:
		if len(a) == 1 && a[0].Type().Kind() == cc.Union && a[0].Type().Size() == x.Size() {
			r := c.expr(w, a[0].AssignmentExpression, t, exprDefault)
			r.n = a[0].AssignmentExpression
			return r
		}

		return c.initializerUnion(w, n, a, x, off0, arrayElem)
	default:
		c.err(errorf("TODO %T", x))
		return nil
	}
}

func (c *ctx) mentionsFunc(n cc.ExpressionNode) bool {
	if n == nil {
		return false
	}

	if n.Type().Kind() == cc.Function || n.Type().Kind() == cc.Ptr && n.Type().(*cc.PointerType).Elem().Kind() == cc.Function {
		return true
	}

	t := reflect.TypeOf(n)
	v := reflect.ValueOf(n)
	var zero reflect.Value
	if t.Kind() == reflect.Pointer {
		t = t.Elem()
		v = v.Elem()
		if v == zero {
			return false
		}
	}

	if t.Kind() != reflect.Struct {
		return false
	}

	nf := t.NumField()
	for i := 0; i < nf; i++ {
		f := t.Field(i)
		if !f.IsExported() {
			continue
		}

		if v == zero || v.IsZero() {
			continue
		}

		if m, ok := v.Field(i).Interface().(cc.ExpressionNode); ok && c.mentionsFunc(m) {
			return true
		}
	}
	return false
}

func (c *ctx) isZeroInitializerSlice(s []*cc.Initializer) bool {
	for _, v := range s {
		if !c.isZero(v.AssignmentExpression.Value()) {
			return false
		}
	}

	return true
}

func (c *ctx) initializerArray(w writer, n cc.Node, a []*cc.Initializer, t *cc.ArrayType, off0 int64) (r *buf) {
	var b buf
	b.w("%s{", c.typ(n, t))
	if c.isZeroInitializerSlice(a) {
		b.w("}")
		return &b
	}

	et := t.Elem()
	esz := et.Size()
	s := sortInitializers(a, func(n int64) int64 { n -= off0; return n - n%esz })
	ranged := false
	for _, v := range s {
		if v[0].Len() != 1 {
			ranged = true
			break
		}
	}
	switch {
	case ranged:
		type expanded struct {
			s   *cc.Initializer
			off int64
		}
		m := map[int64]*expanded{}
		for _, vs := range s {
			for _, v := range vs {
				off := v.Offset() - off0
				off -= off % esz
				x := off / esz
				switch ln := v.Len(); {
				case ln != 1:
					for i := int64(0); i < ln; i++ {
						if ex, ok := m[x]; !ok || ex.s.Order() < v.Order() {
							m[x] = &expanded{v, off0 + off + i*esz}
						}
						x++
					}
				default:
					if ex, ok := m[x]; !ok || ex.s.Order() < v.Order() {
						m[x] = &expanded{v, off0 + off}
					}
				}
			}
		}
		var a []int64
		for k := range m {
			a = append(a, k)
		}
		sort.Slice(a, func(i, j int) bool { return a[i] < a[j] })
		for _, k := range a {
			v := m[k]
			if !c.isZeroInitializerSlice([]*cc.Initializer{v.s}) || !cc.IsArithmeticType(et) {
				if s := c.initializer(w, n, []*cc.Initializer{v.s}, et, v.off, true); !bytes.Equal(s.bytes(), zeroFuncPtr) {
					b.w("\n%d: %s, ", k, s)
				}
			}
		}
	default:
		for _, v := range s {
			v0 := v[0]
			off := v0.Offset() - off0
			off -= off % esz
			switch ln := v0.Len(); {
			case ln != 1:
				for i := int64(0); i < ln; i++ {
					b.w("\n%d: %s, ", off/esz+i, c.initializer(w, n, v, et, off0+off+i*esz, true))
				}
			default:
				off := v[0].Offset() - off0
				off -= off % esz
				if !c.isZeroInitializerSlice(v) || !cc.IsArithmeticType(et) {
					if s := c.initializer(w, n, v, et, off0+off, true); !bytes.Equal(s.bytes(), zeroFuncPtr) {
						b.w("\n%d: %s, ", off/esz, s)
					}
				}
			}
		}
	}
	b.w("\n}")
	return &b
}

func (c *ctx) initializerStruct(w writer, n cc.Node, a []*cc.Initializer, t *cc.StructType, off0 int64) (r *buf) {
	var b buf
	switch {
	case t.HasFlexibleArrayMember():
		b.w("%s{", c.initTyp(n, t))
	default:
		b.w("%s{", c.typ(n, t))
	}
	if c.isZeroInitializerSlice(a) {
		b.w("}")
		return &b
	}

	var flds []*cc.Field
	for i := 0; ; i++ {
		if f := t.FieldByIndex(i); f != nil {
			if f.Type().Size() <= 0 {
				switch x := f.Type().(type) {
				case *cc.StructType:
					if x.NumFields() != 0 {
						c.err(errorf("TODO %T", x))
						return nil
					}
				case *cc.UnionType:
					if x.NumFields() != 0 {
						c.err(errorf("TODO %T", x))
						return nil
					}
				case *cc.ArrayType:
					if x.Len() > 0 {
						c.err(errorf("TODO %T", x))
						return nil
					}
				default:
					c.err(errorf("TODO %T", x))
					return nil
				}
				continue
			}

			if f.IsBitfield() && f.ValueBits() == 0 {
				continue
			}

			flds = append(flds, f)
			continue
		}

		break
	}
	s := sortInitializers(a, func(off int64) int64 {
		off -= off0
		i := sort.Search(len(flds), func(i int) bool {
			return flds[i].OuterGroupOffset() >= off
		})
		if i < len(flds) && flds[i].OuterGroupOffset() == off {
			return off
		}

		return flds[i-1].OuterGroupOffset()
	})
	for _, v := range s {
		first := v[0]
		off := first.Offset() - off0
		for off > flds[0].Offset()+flds[0].Type().Size()-1 {
			flds = flds[1:]
			if len(flds) == 0 {
				panic(todo("", n.Position()))
			}
		}
		f := flds[0]
		if f.IsBitfield() {
			for len(flds) != 0 && flds[0].OuterGroupOffset() == f.OuterGroupOffset() {
				flds = flds[1:]
			}
			b.w("\n%s__ccgo%d: ", tag(field), f.OuterGroupOffset())
			sort.Slice(v, func(i, j int) bool {
				a, b := v[i].Field(), v[j].Field()
				return a.Offset()*8+int64(a.OffsetBits()) < b.Offset()*8+int64(b.OffsetBits())
			})
			ogo := f.OuterGroupOffset()
			gsz := 8 * (int64(f.GroupSize()) + f.Offset() - ogo)
			for i, in := range v {
				if i != 0 {
					b.w("|")
				}
				f = in.Field()
				sh := f.OffsetBits() + 8*int(f.Offset()-ogo)
				b.w("(((%s)&%#0x)<<%d)", c.expr(w, in.AssignmentExpression, c.unsignedInts[gsz/8], exprDefault), uint64(1)<<f.ValueBits()-1, sh)
			}
			b.w(", ")
			continue
		}

		for isEmpty(v[0].Type()) {
			v = v[1:]
		}
		flds = flds[1:]
		if !c.isZeroInitializerSlice(v) {
			if s := c.initializer(w, n, v, f.Type(), off0+f.Offset(), false); !bytes.Equal(s.bytes(), zeroFuncPtr) {
				b.w("\n%s%s: %s, ", tag(field), c.fieldName(t, f), s)
			}
		}
	}
	b.w("\n}")
	return &b
}

func (c *ctx) initializerUnion(w writer, n cc.Node, a []*cc.Initializer, t *cc.UnionType, off0 int64, arrayElem bool) (r *buf) {
	var b buf
	if c.isZeroInitializerSlice(a) {
		b.w("%s{}", c.typ(n, t))
		return &b
	}

	switch t.NumFields() {
	case 0:
		c.err(errorf("%v: cannot initialize empty union", n.Position()))
	case 1:
		b.w("%s{%s%s: %s}", c.typ(n, t), tag(field), c.fieldName(t, t.FieldByIndex(0)), c.initializer(w, n, a, t.FieldByIndex(0).Type(), off0, false))
		return &b
	}

	switch len(a) {
	case 1:
		b.w("(*(*%s)(%sunsafe.%sPointer(&struct{ ", c.typ(n, t), tag(importQualifier), tag(preserve))
		b.w("%s", c.initializerUnionOne(w, n, a, t, off0))
		b.w(")))")
	default:
		b.w("(*(*%s)(%sunsafe.%sPointer(&", c.typ(n, t), tag(importQualifier), tag(preserve))
		b.w("%s", c.initializerUnionMany(w, n, a, t, off0, arrayElem))
		b.w(")))")
	}
	return &b
}

func (c *ctx) initializerUnionMany(w writer, n cc.Node, a []*cc.Initializer, t *cc.UnionType, off0 int64, arrayElem bool) (r *buf) {
	var arrayElemOff int64
	if arrayElem {
		arrayElemOff = off0 - off0%t.Size()
	}
	var b buf
	var paths [][]*cc.Initializer
	for _, v := range a {
		var path []*cc.Initializer
		for p := v.Parent(); p != nil; p = p.Parent() {
			path = append(path, p)
		}
		paths = append(paths, path)
	}
	var lca *cc.Initializer
	for {
		var path *cc.Initializer
		for i, v := range paths {
			if len(v) == 0 {
				goto done
			}

			w := v[len(v)-1]
			if i == 0 {
				path = w
				continue
			}

			if w != path {
				goto done
			}
		}
		lca = path
		if lca.Type() == t {
			goto done
		}

		for i, v := range paths {
			paths[i] = v[:len(v)-1]
		}
	}
done:
	if lca == nil {
		w.w("panic(`TODO %v: (%v:)`);", pos(n), origin(1))
		b.w("(%s{})", c.typ(n, t))
		return &b
	}

	lcaType, lcaOff := c.fixLCA(t, lca, a, off0)
	if lcaType == nil {
		w.w("panic(`TODO %v: (%v:)`);", pos(n), origin(1))
		b.w("(%s{})", c.typ(n, t))
		return &b
	}

	if lcaType.Size() == t.Size() {
		return c.initializer(w, n, a, lcaType, off0, false)
	}

	pre := lcaOff - arrayElemOff
	post := t.Size() - lcaType.Size() - pre
	b.w("struct{")
	if lcaOff != 0 {
		b.w("%s_ [%d]byte;", tag(preserve), pre)
	}
	b.w("%sf ", tag(preserve))
	b.w("%s ", c.typ(n, lcaType))
	if post != 0 {
		b.w("; %s_ [%d]byte", tag(preserve), post)
	}
	b.w("}{%sf: ", tag(preserve))
	b.w("%s", c.initializer(w, n, a, lcaType, off0, false))
	b.w("}")
	return &b
}

func (c *ctx) fixLCA(t *cc.UnionType, lca *cc.Initializer, a []*cc.Initializer, off0 int64) (rt cc.Type, off int64) {
	rt = lca.Type()
	switch {
	case rt.Size() > t.Size():
		return rt, lca.Offset()
	case rt != t:
		return rt, lca.Offset()
	}

	okField, okName := true, true
	for _, v := range a {
		if v.Field() == nil {
			okField = false
			okName = false
			break
		}

		if v.Field().Name() == "" {
			okName = false
			break
		}
	}

	if okField && okName {
	nextUf:
		for i := 0; i < t.NumFields(); i++ {
			uf := t.FieldByIndex(i)
		ok:
			for _, v := range a {
				af := v.Field()
				fs := c.findFields(uf.Type(), af.Name(), 0)
				if len(fs) == 0 {
					continue nextUf
				}

				for _, f := range fs {
					if v.Offset()-off0 != f.off {
						continue
					}

					if v.Type().Size() != f.f.Type().Size() {
						continue
					}

					continue ok
				}

				continue nextUf
			}
			return uf.Type(), lca.Offset() + uf.Offset()
		}
	}

	f := t.FieldByIndex(0)
	return f.Type(), f.Offset()
}

type fld struct {
	f   *cc.Field
	off int64
}

func (c *ctx) findFields(t cc.Type, fn string, off int64) (r []fld) {
	x, ok := t.(interface {
		FieldByIndex(int) *cc.Field
		NumFields() int
	})
	if !ok {
		return nil
	}

	for i := 0; i < x.NumFields(); i++ {
		f := x.FieldByIndex(i)
		if f.Name() == fn {
			r = append(r, fld{f: f, off: off + f.Offset()})
		}

		r = append(r, c.findFields(f.Type(), fn, f.Offset())...)
	}
	return r
}

func (c *ctx) initializerUnionOne(w writer, n cc.Node, a []*cc.Initializer, t *cc.UnionType, off0 int64) (r *buf) {
	var b buf
	in := a[0]
	pre := in.Offset() - off0
	if pre != 0 {
		b.w("%s_ [%d]byte;", tag(preserve), pre)
	}
	b.w("%sf ", tag(preserve))
	f := in.Field()
	switch {
	case f != nil && f.IsBitfield():
		b.w("%suint%d", tag(preserve), f.AccessBytes()*8)
	default:
		b.w("%s ", c.typ(n, in.Type()))
	}
	if post := t.Size() - (pre + in.Type().Size()); post != 0 {
		b.w("; %s_ [%d]byte", tag(preserve), post)
	}
	b.w("}{%sf: ", tag(preserve))
	switch f := in.Field(); {
	case f != nil && f.IsBitfield():
		b.w("(((%s)&%#0x)<<%d)", c.expr(w, in.AssignmentExpression, c.unsignedInts[f.AccessBytes()], exprDefault), uint64(1)<<f.ValueBits()-1, f.OffsetBits())
	default:
		b.w("%s", c.expr(w, in.AssignmentExpression, in.Type(), exprDefault))
	}
	b.w("}")
	return &b
}
