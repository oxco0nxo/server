// Copyright 2021 The Gc Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package gc // import "modernc.org/gc/v2"

import (
	"bytes"
	"encoding/hex"
	"flag"
	"fmt"
	goparser "go/parser"
	"go/scanner"
	"go/token"
	"io/ioutil"
	"math"
	"math/rand"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"runtime"
	"runtime/debug"
	"sort"
	"strings"
	"sync"
	"testing"
	"unicode"

	"github.com/dustin/go-humanize"
	"github.com/pmezard/go-difflib/difflib"
	"modernc.org/scannertest"
)

func caller(s string, va ...interface{}) {
	if s == "" {
		s = strings.Repeat("%v ", len(va))
	}
	_, fn, fl, _ := runtime.Caller(2)
	fmt.Fprintf(os.Stderr, "# caller: %s:%d: ", path.Base(fn), fl)
	fmt.Fprintf(os.Stderr, s, va...)
	fmt.Fprintln(os.Stderr)
	_, fn, fl, _ = runtime.Caller(1)
	fmt.Fprintf(os.Stderr, "# \tcallee: %s:%d: ", path.Base(fn), fl)
	fmt.Fprintln(os.Stderr)
	os.Stderr.Sync()
}

func dbg(s string, va ...interface{}) {
	if s == "" {
		s = strings.Repeat("%v ", len(va))
	}
	pc, fn, fl, _ := runtime.Caller(1)
	f := runtime.FuncForPC(pc)
	fmt.Fprintf(os.Stderr, "# dbg %s:%d:%s: ", path.Base(fn), fl, f.Name())
	fmt.Fprintf(os.Stderr, s, va...)
	fmt.Fprintln(os.Stderr)
	os.Stderr.Sync()
}

func stack() []byte { return debug.Stack() }

func use(...interface{}) {}

func init() {
	use(caller, dbg, stack, dumpExpr) //TODOOK
}

// ----------------------------------------------------------------------------

var (
	_ scannertest.Interface = (*testScanner)(nil)

	oFailNow   = flag.Bool("failnow", false, "")
	oRE        = flag.String("re", "", "")
	oTrc       = flag.Bool("trc", false, "")
	oTrcFail   = flag.Bool("trcfail", false, "")
	oTrcOK     = flag.Bool("trcok", false, "")
	oTypecheck = flag.Bool("typecheck", false, "")

	digits  = expand(unicode.Nd)
	letters = expand(unicode.L)
	re      *regexp.Regexp
	tempDir string
)

func expand(cat *unicode.RangeTable) (r []rune) {
	for _, v := range cat.R16 {
		for x := v.Lo; x <= v.Hi; x += v.Stride {
			r = append(r, rune(x))
		}
	}
	for _, v := range cat.R32 {
		for x := v.Lo; x <= v.Hi; x += v.Stride {
			r = append(r, rune(x))
		}
	}
	s := rand.NewSource(42)
	rn := rand.New(s)
	for i := range r {
		j := rn.Intn(len(r))
		r[i], r[j] = r[j], r[i]
	}
	return r
}

type golden struct {
	a  []string
	f  *os.File
	mu sync.Mutex
	t  *testing.T

	discard bool
}

func newGolden(t *testing.T, fn string) *golden {
	if re != nil || *oFailNow {
		return &golden{discard: true}
	}

	f, err := os.Create(filepath.FromSlash(fn))
	if err != nil { // Possibly R/O fs in a VM
		base := filepath.Base(filepath.FromSlash(fn))
		f, err = ioutil.TempFile("", base)
		if err != nil {
			t.Fatal(err)
		}

		t.Logf("writing results to %s\n", f.Name())
	}

	return &golden{t: t, f: f}
}

func (g *golden) w(s string, args ...interface{}) {
	if g.discard {
		return
	}

	g.mu.Lock()

	defer g.mu.Unlock()

	if s = strings.TrimRight(s, " \t\n\r"); !strings.HasSuffix(s, "\n") {
		s += "\n"
	}
	g.a = append(g.a, fmt.Sprintf(s, args...))
}

func (g *golden) close() {
	if g.discard || g.f == nil {
		return
	}

	defer func() { g.f = nil }()

	sort.Strings(g.a)
	if _, err := g.f.WriteString(strings.Join(g.a, "")); err != nil {
		g.t.Fatal(err)
	}

	if err := g.f.Sync(); err != nil {
		g.t.Fatal(err)
	}

	if err := g.f.Close(); err != nil {
		g.t.Fatal(err)
	}
}

func TestMain(m *testing.M) {
	ExtendedErrors = true
	flag.BoolVar(&trcError, "trcerror", false, "")
	flag.Parse()
	if s := *oRE; s != "" {
		re = regexp.MustCompile(s)
	}
	os.Exit(testMain(m))
}

func testMain(m *testing.M) int {
	var err error
	if tempDir, err = ioutil.TempDir("", "run-test-"); err != nil {
		panic(err) //TODOOK
	}

	defer os.RemoveAll(tempDir)

	return m.Run()
}

func testScan(p *parallel, t *testing.T, root, skip string) {
	if err := filepath.Walk(root, func(path0 string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		if info.IsDir() {
			return nil
		}

		switch {
		case re == nil:
			if strings.Contains(filepath.ToSlash(path0), "/errchk/") {
				return nil
			}

			if strings.Contains(filepath.ToSlash(path0), "/testdata/") {
				return nil
			}

			if skip != "" && strings.Contains(filepath.ToSlash(path0), skip) {
				return nil
			}
		default:
			if !re.MatchString(path0) {
				return nil
			}
		}

		if filepath.Ext(path0) != ".go" {
			return nil
		}

		path := path0
		p.file()
		p.exec(func() error {
			b, err := os.ReadFile(path)
			if err != nil {
				p.fail()
				return err
			}

			fs := token.NewFileSet()
			fi := fs.AddFile(path, -1, len(b))
			var s0 scanner.Scanner
			var err0 error
			s0.Init(fi, b, func(pos token.Position, msg string) {
				err0 = fmt.Errorf("%v: %s", pos, msg)
			}, 0)
			s, err := NewScanner(path, b)
			if err != nil {
				p.fail()
				return err
			}

			for {
				pos0, tok0, lit0 := s0.Scan()
				position0 := fi.Position(pos0)
				eof0 := tok0 == token.EOF
				eof := !s.Scan()
				err := s.Err()
				if g, e := s.Tok.token(), tok0; g != e {
					p.fail()
					return fmt.Errorf("%v: token, got %v, expected %v", position0, g, e)
				}

				if g, e := s.Tok.Src(), lit0; g != e {
					switch {
					case tok0 == token.SEMICOLON && lit0 != ";":
						// Ok, our result for injected semis is different.
					case noGoLit(s.Tok.Ch):
						// Ok, go/scanner does not return the literal string.
					default:
						p.fail()
						return fmt.Errorf("%v: source, got %q, expected %q", position0, g, e)
					}
				}

				if g, e := s.Tok.Position().String(), position0.String(); g != e {
					ok := false
					switch {
					case eof || eof0:
						if a, b := s.Tok.Position().Offset, position0.Offset; a-b == 1 || b-a == 1 {
							ok = true
						}
					case tok0 == token.SEMICOLON && lit0 == "\n":
						ok = s.Tok.Position().Filename == position0.Filename && s.Tok.Position().Line == position0.Line
					}
					if !ok {
						p.fail()
						return fmt.Errorf("%v: got %v", e, g)
					}
				}

				if g, e := err, err0; (g != nil) != (e != nil) {
					p.fail()
					return fmt.Errorf("%v: error, got %v, expected %v", position0, g, e)
				}

				if g, e := eof, eof0; g != e {
					p.fail()
					return fmt.Errorf("%v: EOF, got %v, expected %v", position0, g, e)
				}

				if eof {
					break
				}
			}
			p.ok()
			return nil
		})
		return nil
	}); err != nil {
		t.Error(err)
	}
}

func noGoLit(c Ch) bool {
	switch c {
	case
		'!',
		'%',
		'&',
		'(',
		')',
		'*',
		'+',
		',',
		'-',
		'.',
		'/',
		':',
		'<',
		'=',
		'>',
		'[',
		']',
		'^',
		'{',
		'|',
		'}',
		'~',
		ADD_ASSIGN,
		AND_ASSIGN,
		AND_NOT,
		AND_NOT_ASSIGN,
		ARROW,
		DEC,
		DEFINE,
		ELLIPSIS,
		EQ,
		GE,
		INC,
		LAND,
		LE,
		LOR,
		MUL_ASSIGN,
		NE,
		OR_ASSIGN,
		QUO_ASSIGN,
		REM_ASSIGN,
		SHL,
		SHL_ASSIGN,
		SHR,
		SHR_ASSIGN,
		SUB_ASSIGN,
		XOR_ASSIGN:

		return true
	}

	return false
}

func TestScanner(t *testing.T) {
	p := newParallel(*oFailNow)
	t.Run("states", func(t *testing.T) { testScanStates(t) })
	t.Run(".", func(t *testing.T) { testScan(p, t, ".", "") })
	t.Run("GOROOT", func(t *testing.T) { testScan(p, t, runtime.GOROOT(), "/test/") })
	t.Run("errors", func(t *testing.T) { testScanErrors(t) })
	t.Run("numbers", func(t *testing.T) { testNumbers(t) })
	if err := p.wait(); err != nil {
		t.Error(err)
	}
	t.Logf("TOTAL files %v, ok %v, fails %v", p.files, p.oks, p.fails)
}

type testScanner struct {
	inits int
	mod   int
	s     *Scanner
}

func newTestScanner() *testScanner {
	return &testScanner{}
}

func (s *testScanner) Init(name string, src []byte) (err error) {
	s.s, err = NewScanner(name, []byte(src))
	s.inits++
	return err
}

func (s *testScanner) Rune(c byte) (r rune, ok bool) {
	switch c {
	case 0:
		return -1, false
	case 0x80: // unicodeDigit
		r = digits[s.mod%len(digits)]
		s.mod++
		return r, true
	case 0x81: // unicodeLetter
		r = letters[s.mod%len(letters)]
		s.mod++
		return r, true
	}

	if c < 128 {
		return rune(c), true
	}

	return -1, false
}

func (s *testScanner) Scan() error {
	s.s.Tok.source = s.s.source
	s.s.scan()
	return s.s.Err()
}

func testScanStates(t *testing.T) {
	if testing.Short() {
		t.Skip("-short")
	}

	b, err := os.ReadFile(filepath.FromSlash("testdata/scanner/scanner.l"))
	if err != nil {
		t.Fatal(err)
	}

	s := newTestScanner()
	if err := scannertest.TestStates("scanner.l", bytes.NewReader(b), s); err != nil {
		t.Fatal(err)
	}

	t.Logf("%v test cases", s.inits)
}

func BenchmarkScanner(b *testing.B) {
	root := runtime.GOROOT()
	skip := filepath.ToSlash(root + "/test/")
	var sz int64
	files := 0
	debug.FreeOSMemory()
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		err := filepath.Walk(runtime.GOROOT(), func(path string, info os.FileInfo, err error) error {
			if err != nil {
				return err
			}

			if info.IsDir() {
				return nil
			}

			if strings.HasPrefix(filepath.ToSlash(path), skip) {
				return nil
			}

			buf, err := ioutil.ReadFile(path)
			if err != nil {
				return err
			}

			if i == 0 {
				sz += int64(len(buf))
				files++
			}
			switch filepath.Ext(path) {
			case ".go":
				s, err := NewScanner(path, buf)
				if err != nil {
					return err
				}

				for s.Scan() {
				}
				if err := s.Err(); err != nil {
					b.Fatalf("%s: %v", path, err)
				}
			}
			return nil
		})
		if err != nil {
			b.Fatal(err)
		}
	}
	b.SetBytes(sz)
}

func BenchmarkGoScanner(b *testing.B) {
	root := runtime.GOROOT()
	skip := filepath.ToSlash(root + "/test/")
	var sz int64
	files := 0
	debug.FreeOSMemory()
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		err := filepath.Walk(runtime.GOROOT(), func(path string, info os.FileInfo, err error) error {
			if err != nil {
				return err
			}

			if info.IsDir() {
				return nil
			}

			if strings.HasPrefix(filepath.ToSlash(path), skip) {
				return nil
			}

			buf, err := ioutil.ReadFile(path)
			if err != nil {
				return err
			}

			if i == 0 {
				sz += int64(len(buf))
				files++
			}
			switch filepath.Ext(path) {
			case ".go":
				fs := token.NewFileSet()
				fi := fs.AddFile(path, -1, len(buf))
				var s scanner.Scanner
				s.Init(fi, buf, nil, scanner.ScanComments)
				for {
					_, tok, _ := s.Scan()
					if tok == token.EOF {
						break
					}
				}
			}
			return nil
		})
		if err != nil {
			b.Fatal(err)
		}
	}
	b.SetBytes(sz)
}

func TestTokenSet(t *testing.T) {
	for itest, test := range []string{
		"",
		"a",
		"a b",
		"a b c",
		"a1",
		"a1 b2",
		"a1 b2 c2",
	} {
		ntoks := len(strings.Split(test, " "))
		for itok := 0; itok < ntoks; itok++ {
			s, err := NewScanner(fmt.Sprintf("%v.go", itest), []byte(test))
			if err != nil {
				t.Fatal(itest, err)
			}

			var toks []Token
			var seps, srcs []string
			for s.Scan() || len(toks) == 0 {
				toks = append(toks, s.Tok)
				seps = append(seps, s.Tok.Sep())
				srcs = append(srcs, s.Tok.Src())
			}

			for i, v := range toks {
				max := int32(len(v.source.buf))
				if v.sepOff > max || v.sepOff > v.off ||
					v.off < v.sepOff || v.off > max || v.off > v.next ||
					v.next < v.off || v.next > max {
					t.Errorf("token has invalid data: sepOff %d, off %d, next %d, len(source) %d", v.sepOff, v.off, v.next, max)
				}
				if i != 0 {
					prev := toks[i-1]
					if v.sepOff < prev.next {
						t.Errorf("token has invalid data: sepOff %d, off %d, next %d, len(source) %d; prev: sepOff %d off %d next %d",
							v.sepOff, v.off, v.next, max, prev.sepOff, prev.off, prev.next,
						)
					}
				}
			}

			for j, v := range []struct{ sep, src string }{
				{"", ""},
				{"x", ""},
				{"x", "y"},
				{"", "y"},
				{"xx", ""},
				{"xx", "y"},
				{"", "y"},
				{"xx", ""},
				{"xx", "yy"},
				{"", "yy"},
				{"x", ""},
				{"x", "yy"},
				{"", "yy"},
			} {
				toks[itok].Set(v.sep, v.src)
				var sep, src string
				for i, tok := range toks {
					switch {
					case i == itok:
						sep = v.sep
						src = v.src
					default:
						sep = seps[i]
						src = srcs[i]
					}
					if g, e := tok.Sep(), sep; g != e {
						t.Errorf("test %v, tok %v, j %v, got separator %q, expected %q", itest, itok, j, g, e)
					}
					if g, e := tok.Src(), src; g != e {
						t.Errorf("test %v, tok %v, j %v, got source %q, expected %q", itest, itok, j, g, e)
					}
				}
			}
		}
	}
}

func TestParser(t *testing.T) {
	g := newGolden(t, "testdata/test_parse.golden")

	defer g.close()

	p := newParallel(*oFailNow)
	t.Run("cd", func(t *testing.T) { testParser(p, t, g, ".") })
	t.Run("goroot", func(t *testing.T) { testParser(p, t, g, runtime.GOROOT()) })
	if err := p.wait(); err != nil {
		switch s := err.Error(); {
		case strings.ContainsRune(s, '\n'):
			t.Errorf("\n%s", s)
		default:
			t.Error(s)
		}
	}
	t.Logf("TOTAL files %v, skip %v, ok %v, fails %v", h(p.files), h(p.skips), h(p.oks), h(p.fails))
}

func h(v interface{}) string {
	switch x := v.(type) {
	case int:
		return humanize.Comma(int64(x))
	case int32:
		return humanize.Comma(int64(x))
	case int64:
		return humanize.Comma(x)
	case uint32:
		return humanize.Comma(int64(x))
	case uint64:
		if x <= math.MaxInt64 {
			return humanize.Comma(int64(x))
		}
	}
	return fmt.Sprint(v)
}

func parserFails(fn string, src []byte) bool {
	//trc("", fn)
	switch s := filepath.ToSlash(fn); { // Files go/parser parses w/o error but contain syntax errors
	case
		strings.HasSuffix(s, "src/cmd/compile/internal/types2/testdata/check/const0.go"),
		strings.HasSuffix(s, "src/cmd/compile/internal/types2/testdata/check/expr0.go"),
		strings.HasSuffix(s, "src/cmd/compile/internal/types2/testdata/fixedbugs/issue20583.go"),
		strings.HasSuffix(s, "src/cmd/compile/internal/types2/testdata/fixedbugs/issue42695.go"),
		strings.HasSuffix(s, "src/go/types/testdata/check/const0.go"),
		strings.HasSuffix(s, "src/go/types/testdata/check/expr0.go"),
		strings.HasSuffix(s, "src/go/types/testdata/fixedbugs/issue20583.go"),
		strings.HasSuffix(s, "src/go/types/testdata/fixedbugs/issue42695.go"),
		strings.HasSuffix(s, "test/fixedbugs/bug299.go"),
		strings.HasSuffix(s, "test/fixedbugs/issue15055.go"),
		strings.HasSuffix(s, "test/fixedbugs/issue20232.go"): // :9:11: invalid literal: 6e5518446744

		return true
	}

	_, err := goparser.ParseFile(token.NewFileSet(), fn, src, 0)
	return err != nil
}

func testParser(p *parallel, t *testing.T, g *golden, root string) {
	cfg := &ParseSourceFileConfig{}
	err := filepath.Walk(root, func(path0 string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		if info.IsDir() {
			return nil
		}

		switch {
		case re == nil:
			// ok
		default:
			if !re.MatchString(path0) {
				return nil
			}
		}

		if filepath.Ext(path0) != ".go" {
			return nil
		}

		if *oTrc {
			fmt.Fprintf(os.Stderr, "TEST: %v\n", path0)
		}
		path := path0
		p.file()
		p.exec(func() (err error) {
			// Bugs: Files we don't handle correctly yet.
			switch s := filepath.ToSlash(path); {
			case
				strings.HasSuffix(s, "src/cmd/compile/internal/types2/testdata/fixedbugs/issue49439.go"), //TODO generics
				strings.HasSuffix(s, "src/cmd/compile/internal/types2/testdata/fixedbugs/issue49482.go"), //TODO generics
				strings.HasSuffix(s, "src/crypto/elliptic/nistec.go"),                                    //TODO generics
				strings.HasSuffix(s, "src/go/types/testdata/fixedbugs/issue49439.go"),                    //TODO generics
				strings.HasSuffix(s, "src/go/types/testdata/fixedbugs/issue49482.go"):                    //TODO generics

				p.skip()
				return nil
			}

			defer func() {
				if err != nil {
					p.fail()
					if *oTrcFail {
						fmt.Fprintf(os.Stderr, "FAIL: %v\n", path)
					}
				}
			}()

			b, err := os.ReadFile(path)
			if err != nil {
				return err
			}

			ast, err := ParseSourceFile(cfg, path, b)
			if err != nil {
				if parserFails(path, b) {
					p.skip()
					return nil
				}

				return err
			}

			var checker testPackageChecker
			pkg, err := NewPackage("", []*SourceFile{ast})
			if err != nil {
				return err
			}

			if *oTypecheck {
				if err := pkg.Check(&checker); !checker.loadCalled && err != nil {
					if pth := filepath.ToSlash(path); !strings.Contains(pth, "/test/") && !strings.Contains(pth, "/testdata/") {
						return err
					}
				}
			}

			b2 := ast.Source(true)
			got := string(b2)
			exp := string(b)
			if got != exp {
				diff := difflib.UnifiedDiff{
					A:        difflib.SplitLines(exp),
					B:        difflib.SplitLines(got),
					FromFile: path,
					ToFile:   "<nodesource>",
					Context:  0,
				}
				s, err := difflib.GetUnifiedDiffString(diff)
				if err != nil {
					return fmt.Errorf("%v: %v", path, err)
				}

				return fmt.Errorf("%v\ngot\n%s\nexp\n%s\ngot\n%s\nexp\n%s", s, got, exp, hex.Dump(b2), hex.Dump(b))
			}

			p.ok()
			g.w("%s\n", path)
			if *oTrcOK {
				fmt.Fprintf(os.Stderr, "OK: %v\n", path)
			}
			return nil
		})
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

type testPackageChecker struct {
	loadCalled bool
}

func (c *testPackageChecker) PackageLoader(pkg *Package, src *SourceFile, importPath string) (*Package, error) {
	c.loadCalled = true
	return nil, fmt.Errorf("%s", errorf("TODO no loader"))
}

func (c *testPackageChecker) SymbolResolver(currentScope, fileScope *Scope, pkg *Package, ident Token) (Node, error) {
	panic(todo(""))
}

func (c *testPackageChecker) CheckFunctions() bool {
	panic(todo(""))
}

func (c *testPackageChecker) CheckUnexported() bool {
	panic(todo(""))
}

func (c *testPackageChecker) GOARCH() string {
	panic(todo(""))
}

func dumpExpr(n Node) string {
	var b strings.Builder
	dumpExpr0(&b, n)
	return b.String()
}

func dumpExpr0(b *strings.Builder, n Node) {
	switch x := n.(type) {
	case *BinaryExpr:
		b.WriteByte('(')
		dumpExpr0(b, x.A)
		b.WriteString(x.Op.Src())
		dumpExpr0(b, x.B)
		b.WriteByte(')')
	default:
		b.WriteByte('(')
		b.Write(x.Source(false))
		b.WriteByte(')')
	}
}

func BenchmarkParser(b *testing.B) {
	var (
		names []string
		files = map[string][]byte{}
		bytes int64
	)
	if err := filepath.Walk(runtime.GOROOT(), func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		if info.IsDir() {
			return nil
		}

		if filepath.Ext(path) != ".go" {
			return nil
		}

		b, err := os.ReadFile(path)
		if err != nil {
			return err
		}

		names = append(names, path)
		files[path] = b
		bytes += info.Size()
		return nil
	}); err != nil {
		b.Fatal(err)
	}
	b.Run("gc/v2/serial", func(b *testing.B) { benchmarkParserSerial(b, names, files, bytes) })
	b.Run("go/parser/serial", func(b *testing.B) { benchmarkGoParserSerial(b, names, files, bytes) })
	b.Run("gc/v2/paralel", func(b *testing.B) { benchmarkParserParalel(b, names, files, bytes) })
	b.Run("go/parser/paralel", func(b *testing.B) { benchmarkGoParserParalel(b, names, files, bytes) })
}

var sink []interface{}

func benchmarkParserSerial(b *testing.B, names []string, files map[string][]byte, bytes int64) {
	for i := 0; i < b.N; i++ {
		sink = make([]interface{}, 0, len(names))
		cfg := &ParseSourceFileConfig{}
		b.ReportAllocs()
		debug.FreeOSMemory()
		b.ResetTimer()
		for _, nm := range names {
			ast, _ := ParseSourceFile(cfg, nm, files[nm])
			sink = append(sink, ast)
		}
	}
	b.SetBytes(bytes)
}

func benchmarkGoParserSerial(b *testing.B, names []string, files map[string][]byte, bytes int64) {
	for i := 0; i < b.N; i++ {
		sink = make([]interface{}, 0, len(names))
		b.ReportAllocs()
		fs := token.NewFileSet()
		debug.FreeOSMemory()
		b.ResetTimer()
		for _, nm := range names {
			ast, _ := goparser.ParseFile(fs, nm, files[nm], goparser.ParseComments|goparser.SkipObjectResolution)
			sink = append(sink, ast)
		}
	}
	b.SetBytes(bytes)
}

func benchmarkParserParalel(b *testing.B, names []string, files map[string][]byte, bytes int64) {
	p := newParallel(*oFailNow)
	sink = make([]interface{}, 0, len(names)*b.N)
	cfg := &ParseSourceFileConfig{}
	b.ReportAllocs()
	debug.FreeOSMemory()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		for _, nm := range names {
			p.exec(func() error {
				ast, _ := ParseSourceFile(cfg, nm, files[nm])
				sink = append(sink, ast)
				return nil
			})
		}
	}
	p.wait()
	b.SetBytes(bytes)
}

func benchmarkGoParserParalel(b *testing.B, names []string, files map[string][]byte, bytes int64) {
	p := newParallel(*oFailNow)
	sink = make([]interface{}, 0, len(names)*b.N)
	b.ReportAllocs()
	fs := token.NewFileSet()
	debug.FreeOSMemory()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		for _, nm := range names {
			p.exec(func() error {
				ast, _ := goparser.ParseFile(fs, nm, files[nm], goparser.ParseComments|goparser.SkipObjectResolution)
				sink = append(sink, ast)
				return nil
			})
		}
	}
	p.wait()
	b.SetBytes(bytes)
}
