// Copyright 2022 The Gc Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package gc // import "modernc.org/gc/v2"

import (
	"bytes"
	"fmt"
	"go/token"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
)

var (
	_ = todo //TODOOK
	_ = trc  //TODOOK

	// ExtendedErrors optionally amends errors with a stack mini trace. Intended
	// for debugging only.
	ExtendedErrors bool
)

func origin(skip int) string {
	pc, fn, fl, _ := runtime.Caller(skip)
	fn = filepath.Base(fn)
	f := runtime.FuncForPC(pc)
	var fns string
	if f != nil {
		fns = f.Name()
		if x := strings.LastIndex(fns, "."); x > 0 {
			fns = fns[x+1:]
		}
	}
	return fmt.Sprintf("%s:%d:%s", fn, fl, fns)
}

func todo(s string, args ...interface{}) string {
	switch {
	case s == "":
		s = fmt.Sprintf(strings.Repeat("%v ", len(args)), args...)
	default:
		s = fmt.Sprintf(s, args...)
	}
	pc, fn, fl, _ := runtime.Caller(1)
	f := runtime.FuncForPC(pc)
	var fns string
	if f != nil {
		fns = f.Name()
		if x := strings.LastIndex(fns, "."); x > 0 {
			fns = fns[x+1:]
		}
	}
	r := fmt.Sprintf("%s:%d:%s: TODOTODO %s", fn, fl, fns, s) //TODOOK
	fmt.Fprintf(os.Stdout, "%s\n", r)
	os.Stdout.Sync()
	return r
}

func trc(s string, args ...interface{}) string {
	switch {
	case s == "":
		s = fmt.Sprintf(strings.Repeat("%v ", len(args)), args...)
	default:
		s = fmt.Sprintf(s, args...)
	}
	_, fn, fl, _ := runtime.Caller(1)
	r := fmt.Sprintf("%s:%d: TRC %s", fn, fl, s)
	fmt.Fprintf(os.Stdout, "%s\n", r)
	os.Stdout.Sync()
	return r
}

// errorf constructs and error message. If ExtendedErrors is true, the error will
// contain a mini stack trace.
func errorf(s string, args ...interface{}) string {
	switch {
	case s == "":
		s = fmt.Sprintf(strings.Repeat("%v ", len(args)), args...)
	default:
		s = fmt.Sprintf(s, args...)
	}
	switch {
	case ExtendedErrors:
		return fmt.Sprintf("%s (%v: %v: %v:)", s, origin(4), origin(3), origin(2))
	default:
		return s
	}
}

type parallel struct {
	errors []error
	limit  chan struct{}
	sync.Mutex
	wg sync.WaitGroup

	fails int32
	files int32
	oks   int32
	skips int32

	failFast bool
}

func newParallel(failFast bool) *parallel {
	return &parallel{
		failFast: failFast,
		limit:    make(chan struct{}, runtime.GOMAXPROCS(0)),
	}
}

func (p *parallel) fail() { atomic.AddInt32(&p.fails, 1) }
func (p *parallel) file() { atomic.AddInt32(&p.files, 1) }
func (p *parallel) ok()   { atomic.AddInt32(&p.oks, 1) }
func (p *parallel) skip() { atomic.AddInt32(&p.skips, 1) }

func (p *parallel) err(err error) {
	if err == nil {
		return
	}

	if p.failFast {
		panic(err)
	}

	s := err.Error()
	for _, v := range strings.Split(s, "\n") {
		if x := strings.Index(v, "TODO"); x >= 0 {
			fmt.Println(v[x:])
		}
	}
	p.Lock()
	p.errors = append(p.errors, err)
	p.Unlock()
}

func (p *parallel) exec(run func() error) {
	p.limit <- struct{}{}
	p.wg.Add(1)

	go func() {
		defer func() {
			p.wg.Done()
			<-p.limit
		}()

		p.err(run())
	}()
}

func (p *parallel) wait() error {
	p.wg.Wait()
	if len(p.errors) == 0 {
		return nil
	}

	var a []string
	for _, v := range p.errors {
		a = append(a, v.Error())
	}
	return fmt.Errorf("%s", strings.Join(a, "\n"))
}

func nodeTokens(n interface{}) (r []Token) {
	if n == nil {
		return nil
	}

	if x, ok := n.(Token); ok && x.IsValid() {
		return []Token{x}
	}

	t := reflect.TypeOf(n)
	v := reflect.ValueOf(n)
	var zero reflect.Value
	if t.Kind() == reflect.Pointer {
		t = t.Elem()
		v = v.Elem()
		if v == zero {
			return nil
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

			r = append(r, nodeTokens(v.Field(i).Interface())...)
		}
	case reflect.Slice:
		ne := v.Len()
		for i := 0; i < ne; i++ {
			r = append(r, nodeTokens(v.Index(i).Interface())...)
		}
	}
	return r
}

func nodeSource(b *bytes.Buffer, n interface{}, full bool) *bytes.Buffer {
	if n == nil {
		return b
	}

	if x, ok := n.(Token); ok && x.IsValid() {
		switch s := x.sep(); {
		case full:
			b.Write(s)
		default:
			if b.Len() != 0 && len(s) != 0 {
				b.WriteByte(' ')
			}
		}
		b.Write(x.src())
		return b
	}

	t := reflect.TypeOf(n)
	v := reflect.ValueOf(n)
	var zero reflect.Value
	if t.Kind() == reflect.Pointer {
		t = t.Elem()
		v = v.Elem()
		if v == zero {
			return b
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

			nodeSource(b, v.Field(i).Interface(), full)
		}
	case reflect.Slice:
		ne := v.Len()
		for i := 0; i < ne; i++ {
			nodeSource(b, v.Index(i).Interface(), full)
		}
	}
	return b
}

func position(n Node) (r token.Position) {
	if n != nil {
		r = n.Position()
	}
	return r
}

func isIntegerType(t Type) bool {
	switch t.Kind() {
	case Int, Int8, Int16, Int32, Int64, Uint, Uint8, Uint16, Uint32, Uint64, Uintptr:
		return true
	default:
		return false
	}
}

func isAnyIntegerType(t Type) bool { return isIntegerType(t) || t.Kind() == UntypedInt }

func isFloatType(t Type) bool {
	switch t.Kind() {
	case Float32, Float64:
		return true
	default:
		return false
	}
}

func isAnyFloatType(t Type) bool { return isFloatType(t) || t.Kind() == UntypedFloat }

func isComplexType(t Type) bool {
	switch t.Kind() {
	case Complex64, Complex128:
		return true
	default:
		return false
	}
}

func isAnyComplexType(t Type) bool { return isComplexType(t) || t.Kind() == UntypedComplex }

func isArithmeticType(t Type) bool {
	return isIntegerType(t) || isFloatType(t) || isComplexType(t)
}

func isUntypedArithmeticType(t Type) bool {
	switch t.Kind() {
	case UntypedInt, UntypedFloat, UntypedComplex:
		return true
	default:
		return false
	}
}

func isAnyArithmeticType(t Type) bool { return isArithmeticType(t) || isUntypedArithmeticType(t) }

func isAnyStringType(t Type) bool {
	switch t.Kind() {
	case String, UntypedString:
		return true
	default:
		return false
	}
}

func isAnyBoolType(t Type) bool {
	switch t.Kind() {
	case Bool, UntypedBool:
		return true
	default:
		return false
	}
}
