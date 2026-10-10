// Copyright 2021 The Gc Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package gc // import "modernc.org/gc/v2"

import (
	"bytes"
	"fmt"
	"go/token"
	"strings"
	"unicode"
	"unicode/utf8"

	mtoken "modernc.org/token"
)

var (
	_ Node = (*Token)(nil)
)

type errWithPosition struct {
	pos token.Position
	err error
}

type errList []errWithPosition

func (e errList) Err() error {
	if len(e) == 0 {
		return nil
	}

	w := 0
	prev := errWithPosition{pos: token.Position{Offset: -1}}
	for _, v := range e {
		if v.pos.Line == 0 || v.pos.Offset != prev.pos.Offset || v.err.Error() != prev.err.Error() {
			e[w] = v
			w++
			prev = v
		}
	}

	var a []string
	for _, v := range e {
		a = append(a, fmt.Sprintf("%v: %v", v.pos, v.err))
	}
	return fmt.Errorf("%s", strings.Join(a, "\n"))
}

func (e *errList) err(pos token.Position, msg string, args ...interface{}) {
	switch {
	case len(args) == 0:
		*e = append(*e, errWithPosition{pos, fmt.Errorf("%s", msg)})
	default:
		*e = append(*e, errWithPosition{pos, fmt.Errorf(msg, args...)})
	}
}

// Ch represents the lexical value of a Token. Valid values of type Ch are
// non-zero.
type Ch rune

func (c Ch) str() string {
	if c < beforeTokens || c > afterTokens { //TODO
		return fmt.Sprintf("%#U", c)
	}

	return c.String()
}

// token translates, if possible, c to the lexeme value defined in go/token or
// token.ILLEGAL otherwise.
func (c Ch) token() token.Token {
	if r, ok := xlat[c]; ok {
		return r
	}

	return token.ILLEGAL
}

// Node is an item of the CST tree.
type Node interface {
	Position() token.Position
	// Source returns the source form of n. Setting full to false will replace
	// every non-empty token separator by a single space character and drop the
	// first token separator entirely, if any.
	Source(full bool) []byte
	// Tokens returns the tokens a node consists of.
	Tokens() []Token
}

// Token is the product of Scanner.Scan and a terminal node of the complete
// syntax tree.
type Token struct { // 24 bytes on 64 bit arch
	source *source

	Ch
	next   int32
	off    int32
	sepOff int32
}

// Offset reports n's offset, in bytes, within its source file.
func (n *Token) Offset() int { return int(n.off) }

// Position implements Node.
func (n Token) Position() (r token.Position) {
	if n.IsValid() {
		s := n.source
		return token.Position(s.file.PositionFor(mtoken.Pos(s.base+n.off), true))
	}

	return r
}

// Source implements Node.
func (n Token) Source(full bool) []byte { return nodeSource(&bytes.Buffer{}, n, full).Bytes() }

// Tokens returns the tokens n consist of.
func (n Token) Tokens() []Token { return []Token{n} }

// String pretty formats n.
func (n Token) String() string {
	if n.Ch <= beforeTokens || n.Ch >= afterTokens { //TODO
		return fmt.Sprintf("%v: %q %#U, off %v", n.Position(), n.Src(), rune(n.Ch), n.Offset())
	}

	return fmt.Sprintf("%v: %q %s", n.Position(), n.Src(), n.Ch)
}

// IsValid reports the validity of n. Tokens not present in some nodes will
// report false.
func (n Token) IsValid() bool { return n.source != nil }

// Sep returns the whitespace preceding n, if any.
func (n Token) Sep() string { return string(n.sep()) }

// Src returns the textual form of n.
func (n Token) Src() string { return string(n.src()) }

func (n Token) sep() []byte {
	if !n.IsValid() {
		return nil
	}

	if p := n.source.patches[n.off]; p != nil {
		return p.b[:p.off]
	}

	return n.source.buf[n.sepOff:n.off]
}

func (n Token) src() []byte {
	if !n.IsValid() {
		return nil
	}

	if p := n.source.patches[n.off]; p != nil {
		return p.b[p.off:]
	}

	return n.source.buf[n.off:n.next]
}

// Set sets the result of n.Sep to be sep and n.Src() to be src.
// Set will allocate at least len(sep+src) bytes of additional memory.
func (n *Token) Set(sep, src string) { n.set([]byte(sep+src), len(sep)) }

// SetSep sets the result of n.Sep to be sep.  SetSep will allocate at least
// len(sep+n.Src()) bytes of additional memory.
func (n *Token) SetSep(sep string) { n.Set(sep, n.Src()) }

// SetSrc sets the result of n.Src to be src.  SetSrc will allocate at least
// len(n.Sep()+src()) bytes of additional memory.
func (n *Token) SetSrc(src string) { n.Set(n.Sep(), src) }

// set sets the result of n.Sep to be s[:srcOff] and n.Src() to be s[srcOff:].
// set will allocate at least len(s) bytes of additional memory.
func (n *Token) set(s []byte, srcOff int) { n.source.patches[n.off] = &patch{int32(srcOff), s} }

type patch struct {
	off int32
	b   []byte
}

type source struct {
	buf     []byte
	file    *mtoken.File
	patches map[int32]*patch // Token.off: patch

	base int32
}

func newSource(name string, buf []byte) *source {
	file := mtoken.NewFile(name, len(buf))
	return &source{
		buf:     buf,
		file:    file,
		base:    int32(file.Base()),
		patches: map[int32]*patch{},
	}
}

// Scanner provides lexical analysis of its buffer.
type Scanner struct {
	*source
	// Tok is the current token. It is valid after first call to Scan. The value is
	// read only.
	Tok  Token
	errs errList

	// CommentHandler, if not nil, is invoked on line and general comments, passing
	// the offset and content of the comment. The content must not be modified.
	CommentHandler func(off int32, s []byte)

	cnt  int32
	last Ch
	off  int32 // Index into source.buf.

	c byte // Lookahead byte.

	eof      bool
	isClosed bool
}

// NewScanner returns a newly created scanner that will tokenize buf. Positions
// are reported as if buf is coming from a file named name. The buffer becomes
// owned by the scanner and must not be modified after calling NewScanner.
func NewScanner(name string, buf []byte) (*Scanner, error) {
	r := &Scanner{source: newSource(name, buf)}
	switch {
	case len(buf) == 0:
		r.eof = true
		r.Tok.Ch = EOF
	default:
		r.c = buf[0]
		if r.c == '\n' {
			r.file.AddLine(int(r.base + r.off))
		}
	}
	return r, nil
}

func (s *Scanner) position() token.Position {
	return token.Position(s.source.file.PositionFor(mtoken.Pos(s.base+s.off), true))
}

// Err reports any errors the scanner encountered during .Scan() invocations.
// For typical use please see the .Scan() documentation.
func (s *Scanner) Err() error { return s.errs.Err() }

func (s *Scanner) pos(off int32) token.Position {
	return token.Position(s.file.PositionFor(mtoken.Pos(s.base+off), true))
}

func (s *Scanner) err(off int32, msg string, args ...interface{}) {
	s.errs.err(s.pos(off), msg, args...)
}

func (s *Scanner) close() {
	if s.isClosed {
		return
	}

	s.Tok.source = s.source
	s.Tok.Ch = EOF
	s.eof = true
	s.isClosed = true
}

func isIDFirst(c byte) bool {
	return c >= 'a' && c <= 'z' ||
		c >= 'A' && c <= 'Z' ||
		c == '_'
}

func isDigit(c byte) bool      { return c >= '0' && c <= '9' }
func isHexDigit(c byte) bool   { return isDigit(c) || c >= 'a' && c <= 'f' || c >= 'A' && c <= 'F' }
func isIDNext(c byte) bool     { return isIDFirst(c) || isDigit(c) }
func isOctalDigit(c byte) bool { return c >= '0' && c <= '7' }

func (s *Scanner) next() {
	if s.eof {
		return
	}

	if int(s.off) >= len(s.buf)-1 {
		s.c = 0
		s.Tok.next = int32(len(s.buf))
		s.eof = true
		return
	}

	s.off++
	s.Tok.next = s.off
	s.c = s.buf[s.off]
	if s.c == '\n' {
		s.file.AddLine(int(s.base + s.off))
	}
}

func (s *Scanner) nextN(n int) {
	if int(s.off) == len(s.buf)-n {
		s.c = 0
		s.Tok.next = s.off + int32(n)
		s.eof = true
		return
	}

	s.off += int32(n)
	s.Tok.next = s.off
	s.c = s.buf[s.off]
	if s.c == '\n' {
		s.file.AddLine(int(s.base + s.off))
	}
}

// Scan moves to the next token and returns true if not at end of file. Usage
// example:
//
//	s, _ = NewScanner(buf, name, false)
//	for s.Scan() {
//		...
//	}
//	if err := s.Err() {
//		...
//	}
func (s *Scanner) Scan() (r bool) {
	if s.isClosed {
		return false
	}

	s.cnt++
	s.off = s.Tok.next
	s.last = s.Tok.Ch
	s.Tok.sepOff = s.off
	s.Tok.source = s.source
	s.Tok.Ch = -1
	for {
		if r = s.scan(); !r || s.Tok.Ch >= 0 {
			return r
		}
	}
}

func (s *Scanner) scan() (r bool) {
	s.Tok.off = s.Tok.next
	switch s.c {
	case ' ', '\t', '\r', '\n':
		// White space, formed from spaces (U+0020), horizontal tabs (U+0009), carriage
		// returns (U+000D), and newlines (U+000A), is ignored except as it separates
		// tokens that would otherwise combine into a single token.
		if s.c == '\n' && s.injectSemi() {
			return true
		}

		s.next()
		return true
	case '/':
		off := s.off
		s.next()
		switch s.c {
		case '=':
			s.next()
			s.Tok.Ch = QUO_ASSIGN
		case '/':
			// Line comments start with the character sequence // and stop at the end of
			// the line.
			s.next()
			s.lineComment(off)
			return true
		case '*':
			// General comments start with the character sequence /* and stop with the
			// first subsequent character sequence */.
			s.next()
			s.generalComment(off)
			return true
		default:
			s.Tok.Ch = '/'
		}
	case '(', ')', '[', ']', '{', '}', ',', ';', '~':
		s.Tok.Ch = Ch(s.c)
		s.next()
	case '"':
		off := s.off
		s.next()
		s.stringLiteral(off)
	case '\'':
		off := s.off
		s.next()
		s.runeLiteral(off)
	case '`':
		s.next()
		for {
			switch {
			case s.c == '`':
				s.next()
				s.Tok.Ch = STRING_LIT
				return true
			case s.eof:
				s.err(s.off, "raw string literal not terminated")
				s.Tok.Ch = STRING_LIT
				return true
			case s.c == 0:
				panic(todo("%v: %#U", s.position(), s.c))
			default:
				s.next()
			}
		}
	case '.':
		s.next()
		off := s.off
		if isDigit(s.c) {
			s.dot(false, true)
			return true
		}

		if s.c != '.' {
			s.Tok.Ch = '.'
			return true
		}

		s.next()
		if s.c != '.' {
			s.off = off
			s.Tok.Ch = '.'
			return true
		}

		s.next()
		s.Tok.Ch = ELLIPSIS
		return true
	case '%':
		s.next()
		switch s.c {
		case '=':
			s.next()
			s.Tok.Ch = REM_ASSIGN
		default:
			s.Tok.Ch = '%'
		}
	case '*':
		s.next()
		switch s.c {
		case '=':
			s.next()
			s.Tok.Ch = MUL_ASSIGN
		default:
			s.Tok.Ch = '*'
		}
	case '^':
		s.next()
		switch s.c {
		case '=':
			s.next()
			s.Tok.Ch = XOR_ASSIGN
		default:
			s.Tok.Ch = '^'
		}
	case '+':
		s.next()
		switch s.c {
		case '+':
			s.next()
			s.Tok.Ch = INC
		case '=':
			s.next()
			s.Tok.Ch = ADD_ASSIGN
		default:
			s.Tok.Ch = '+'
		}
	case '-':
		s.next()
		switch s.c {
		case '-':
			s.next()
			s.Tok.Ch = DEC
		case '=':
			s.next()
			s.Tok.Ch = SUB_ASSIGN
		default:
			s.Tok.Ch = '-'
		}
	case ':':
		s.next()
		switch {
		case s.c == '=':
			s.next()
			s.Tok.Ch = DEFINE
		default:
			s.Tok.Ch = ':'
		}
	case '=':
		s.next()
		switch {
		case s.c == '=':
			s.next()
			s.Tok.Ch = EQ
		default:
			s.Tok.Ch = '='
		}
	case '!':
		s.next()
		switch {
		case s.c == '=':
			s.next()
			s.Tok.Ch = NE
		default:
			s.Tok.Ch = '!'
		}
	case '>':
		s.next()
		switch s.c {
		case '=':
			s.next()
			s.Tok.Ch = GE
		case '>':
			s.next()
			switch s.c {
			case '=':
				s.next()
				s.Tok.Ch = SHR_ASSIGN
			default:
				s.Tok.Ch = SHR
			}
		default:
			s.Tok.Ch = '>'
		}
	case '<':
		s.next()
		switch s.c {
		case '=':
			s.next()
			s.Tok.Ch = LE
		case '<':
			s.next()
			switch s.c {
			case '=':
				s.next()
				s.Tok.Ch = SHL_ASSIGN
			default:
				s.Tok.Ch = SHL
			}
		case '-':
			s.next()
			s.Tok.Ch = ARROW
		default:
			s.Tok.Ch = '<'
		}
	case '|':
		s.next()
		switch s.c {
		case '|':
			s.next()
			s.Tok.Ch = LOR
		case '=':
			s.next()
			s.Tok.Ch = OR_ASSIGN
		default:
			s.Tok.Ch = '|'
		}
	case '&':
		s.next()
		switch s.c {
		case '&':
			s.next()
			s.Tok.Ch = LAND
		case '^':
			s.next()
			switch s.c {
			case '=':
				s.next()
				s.Tok.Ch = AND_NOT_ASSIGN
			default:
				s.Tok.Ch = AND_NOT
			}
		case '=':
			s.next()
			s.Tok.Ch = AND_ASSIGN
		default:
			s.Tok.Ch = '&'
		}
	default:
		switch {
		case isIDFirst(s.c):
			s.next()
			s.identifierOrKeyword()
		case isDigit(s.c):
			s.numericLiteral()
		case s.c >= 0x80:
			off := s.off
			switch r := s.rune(); {
			case unicode.IsLetter(r):
				s.identifierOrKeyword()
			case r == 0xfeff:
				if off == 0 { // Ignore BOM, but only at buffer start.
					return true
				}

				s.err(off, "illegal byte order mark")
				s.Tok.Ch = 0
			default:
				s.err(s.off, "illegal character %#U", r)
				s.Tok.Ch = 0
			}
		case s.eof:
			if s.injectSemi() {
				return true
			}

			s.close()
			return false
		case s.c == 0:
			panic(todo("%v: %#U", s.position(), s.c))
		default:
			s.err(s.off, "illegal character %#U", s.c)
			s.next()
			s.Tok.Ch = 0
		}
	}
	return true
}

// When the input is broken into tokens, a semicolon is automatically inserted
// into the token stream immediately after a line's final token if that token
// is
//
//   - an identifier
//   - an integer, floating-point, imaginary, rune, or string literal
//   - one of the keywords break, continue, fallthrough, or return
//   - one of the operators and punctuation ++, --, ), ], or }
func (s *Scanner) injectSemi() bool {
	switch s.last {
	case
		IDENTIFIER, INT_LIT, FLOAT_LIT, IMAG_LIT, RUNE_LIT, STRING_LIT,
		BREAK, CONTINUE, FALLTHROUGH, RETURN,
		INC, DEC, ')', ']', '}':

		s.Tok.Ch = ';'
		s.last = 0
		if s.c == '\n' {
			s.next()
		}
		return true
	}

	s.last = 0
	return false
}

func (s *Scanner) numericLiteral() {
	// Leading decimal digit not consumed.
	var hasHexMantissa, needFrac bool
more:
	switch s.c {
	case '0':
		s.next()
		switch s.c {
		case '.':
			// nop
		case 'b', 'B':
			s.next()
			s.binaryLiteral()
			return
		case 'e', 'E':
			s.exponent()
			s.Tok.Ch = FLOAT_LIT
			return
		case 'p', 'P':
			s.err(s.off, "'%c' exponent requires hexadecimal mantissa", s.c)
			s.exponent()
			s.Tok.Ch = FLOAT_LIT
			return
		case 'o', 'O':
			s.next()
			s.octalLiteral()
			return
		case 'x', 'X':
			hasHexMantissa = true
			needFrac = true
			s.Tok.Ch = INT_LIT
			s.next()
			if s.c == '.' {
				s.next()
				s.dot(hasHexMantissa, needFrac)
				return
			}

			if s.hexadecimals() == 0 {
				s.err(s.base+s.off, "hexadecimal literal has no digits")
				return
			}

			needFrac = false
		case 'i':
			s.next()
			s.Tok.Ch = IMAG_LIT
			return
		default:
			invalidOff := int32(-1)
			var invalidDigit byte
			for {
				if s.c == '_' {
					for n := 0; s.c == '_'; n++ {
						if n == 1 {
							s.err(s.off, "'_' must separate successive digits")
						}
						s.next()
					}
					if !isDigit(s.c) {
						s.err(s.off-1, "'_' must separate successive digits")
					}
				}
				if isOctalDigit(s.c) {
					s.next()
					continue
				}

				if isDigit(s.c) {
					if invalidOff < 0 {
						invalidOff = s.off
						invalidDigit = s.c
					}
					s.next()
					continue
				}

				break
			}
			switch s.c {
			case '.', 'e', 'E', 'i':
				break more
			}
			if isDigit(s.c) {
				break more
			}
			if invalidOff > 0 {
				s.err(invalidOff, "invalid digit '%c' in octal literal", invalidDigit)
			}
			s.Tok.Ch = INT_LIT
			return
		}
	default:
		s.decimals()
	}
	switch s.c {
	case '.':
		s.next()
		s.dot(hasHexMantissa, needFrac)
	case 'e', 'E', 'p', 'P':
		s.exponent()
		if s.c == 'i' {
			s.next()
			s.Tok.Ch = IMAG_LIT
			return
		}

		s.Tok.Ch = FLOAT_LIT
	case 'i':
		s.next()
		s.Tok.Ch = IMAG_LIT
	default:
		s.Tok.Ch = INT_LIT
	}
}

func (s *Scanner) octalLiteral() {
	// Leading 0o consumed.
	ok := false
	invalidOff := int32(-1)
	var invalidDigit byte
	s.Tok.Ch = INT_LIT
	for {
		for n := 0; s.c == '_'; n++ {
			if n == 1 {
				s.err(s.off, "'_' must separate successive digits")
			}
			s.next()
		}
		switch s.c {
		case '0', '1', '2', '3', '4', '5', '6', '7':
			s.next()
			ok = true
		case '8', '9':
			if invalidOff < 0 {
				invalidOff = s.off
				invalidDigit = s.c
			}
			s.next()
		case '.':
			s.Tok.Ch = FLOAT_LIT
			s.err(s.off, "invalid radix point in octal literal")
			s.next()
		case 'e', 'E':
			s.Tok.Ch = FLOAT_LIT
			s.err(s.off, "'%c' exponent requires decimal mantissa", s.c)
			s.exponent()
		case 'p', 'P':
			s.Tok.Ch = FLOAT_LIT
			s.err(s.off, "'%c' exponent requires hexadecimal mantissa", s.c)
			s.exponent()
		default:
			switch {
			case !ok:
				s.err(s.base+s.off, "octal literal has no digits")
			case invalidOff > 0:
				s.err(invalidOff, "invalid digit '%c' in octal literal", invalidDigit)
			}
			if s.c == 'i' {
				s.next()
				s.Tok.Ch = IMAG_LIT
			}
			return
		}
	}
}

func (s *Scanner) dot(hasHexMantissa, needFrac bool) {
	// '.' already consumed
	switch {
	case hasHexMantissa:
		if s.hexadecimals() == 0 && needFrac {
			s.err(s.off, "hexadecimal literal has no digits")
		}
		switch s.c {
		case 'p', 'P':
			// ok
		default:
			s.err(s.off, "hexadecimal mantissa requires a 'p' exponent")
		}
	default:
		if s.decimals() == 0 && needFrac {
			panic(todo("%v: %#U", s.position(), s.c))
		}
	}
	switch s.c {
	case 'p', 'P':
		if !hasHexMantissa {
			s.err(s.off, "'%c' exponent requires hexadecimal mantissa", s.c)
		}
		fallthrough
	case 'e', 'E':
		s.exponent()
		if s.c == 'i' {
			s.next()
			s.Tok.Ch = IMAG_LIT
			return
		}

		s.Tok.Ch = FLOAT_LIT
	case 'i':
		s.next()
		s.Tok.Ch = IMAG_LIT
	default:
		s.Tok.Ch = FLOAT_LIT
	}
}

func (s *Scanner) decimals() (r int) {
	first := true
	for {
		switch {
		case isDigit(s.c):
			first = false
			s.next()
			r++
		case s.c == '_':
			for n := 0; s.c == '_'; n++ {
				if first || n == 1 {
					s.err(s.off, "'_' must separate successive digits")
				}
				s.next()
			}
			if !isDigit(s.c) {
				s.err(s.off-1, "'_' must separate successive digits")
			}
		default:
			return r
		}
	}
}

func (s *Scanner) hexadecimals() (r int) {
	for {
		switch {
		case isHexDigit(s.c):
			s.next()
			r++
		case s.c == '_':
			for n := 0; s.c == '_'; n++ {
				if n == 1 {
					s.err(s.off, "'_' must separate successive digits")
				}
				s.next()
			}
			if !isHexDigit(s.c) {
				s.err(s.off-1, "'_' must separate successive digits")
			}
		default:
			return r
		}
	}
}

func (s *Scanner) binaryLiteral() {
	// Leading 0b consumed.
	ok := false
	invalidOff := int32(-1)
	var invalidDigit byte
	s.Tok.Ch = INT_LIT
	for {
		for n := 0; s.c == '_'; n++ {
			if n == 1 {
				s.err(s.off, "'_' must separate successive digits")
			}
			s.next()
		}
		switch s.c {
		case '0', '1':
			s.next()
			ok = true
		case '.':
			s.Tok.Ch = FLOAT_LIT
			s.err(s.off, "invalid radix point in binary literal")
			s.next()
		case 'e', 'E':
			s.Tok.Ch = FLOAT_LIT
			s.err(s.off, "'%c' exponent requires decimal mantissa", s.c)
			s.exponent()
		case 'p', 'P':
			s.Tok.Ch = FLOAT_LIT
			s.err(s.off, "'%c' exponent requires hexadecimal mantissa", s.c)
			s.exponent()
		default:
			if isDigit(s.c) {
				if invalidOff < 0 {
					invalidOff = s.off
					invalidDigit = s.c
				}
				s.next()
				continue
			}

			switch {
			case !ok:
				s.err(s.base+s.off, "binary literal has no digits")
			case invalidOff > 0:
				s.err(invalidOff, "invalid digit '%c' in binary literal", invalidDigit)
			}
			if s.c == 'i' {
				s.next()
				s.Tok.Ch = IMAG_LIT
			}
			return
		}
	}
}

func (s *Scanner) exponent() {
	// Leanding e or E not consumed.
	s.next()
	switch s.c {
	case '+', '-':
		s.next()
	}
	if !isDigit(s.c) {
		s.err(s.base+s.off, "exponent has no digits")
		return
	}

	s.decimals()
}

func (s *Scanner) runeLiteral(off int32) {
	// Leading ' consumed.
	ok := 0
	s.Tok.Ch = RUNE_LIT
	expOff := int32(-1)
	if s.eof {
		s.err(off, "rune literal not terminated")
		return
	}

	for {
		switch s.c {
		case '\\':
			ok++
			s.next()
			switch s.c {
			case '\'', '\\', 'a', 'b', 'f', 'n', 'r', 't', 'v':
				s.next()
			case 'x', 'X':
				s.next()
				for i := 0; i < 2; i++ {
					if s.c == '\'' {
						if i != 2 {
							s.err(s.off, "illegal character %#U in escape sequence", s.c)
						}
						s.next()
						return
					}

					if !isHexDigit(s.c) {
						s.err(s.off, "illegal character %#U in escape sequence", s.c)
						break
					}
					s.next()
				}
			case 'u':
				s.u(4)
			case 'U':
				s.u(8)
			default:
				switch {
				case s.eof:
					s.err(s.base+s.off, "escape sequence not terminated")
					return
				case isOctalDigit(s.c):
					for i := 0; i < 3; i++ {
						s.next()
						if s.c == '\'' {
							if i != 2 {
								s.err(s.off, "illegal character %#U in escape sequence", s.c)
							}
							s.next()
							return
						}

						if !isOctalDigit(s.c) {
							s.err(s.off, "illegal character %#U in escape sequence", s.c)
							break
						}
					}
				default:
					s.err(s.off, "unknown escape sequence")
				}
			}
		case '\'':
			s.next()
			if ok != 1 {
				s.err(off, "illegal rune literal")
			}
			return
		case '\t':
			s.next()
			ok++
		default:
			switch {
			case s.eof:
				switch {
				case ok != 0:
					s.err(expOff, "rune literal not terminated")
				default:
					s.err(s.base+s.off, "rune literal not terminated")
				}
				return
			case s.c == 0:
				panic(todo("%v: %#U", s.position(), s.c))
			case s.c < ' ':
				ok++
				s.err(s.off, "non-printable character: %#U", s.c)
				s.next()
			case s.c >= 0x80:
				ok++
				off := s.off
				if c := s.rune(); c == 0xfeff {
					s.err(off, "illegal byte order mark")
				}
			default:
				ok++
				s.next()
			}
		}
		if ok != 0 && expOff < 0 {
			expOff = s.off
			if s.eof {
				expOff++
			}
		}
	}
}

func (s *Scanner) rune() rune {
	switch r, sz := utf8.DecodeRune(s.buf[s.off:]); {
	case r == utf8.RuneError && sz == 0:
		panic(todo("%v: %#U", s.position(), s.c))
	case r == utf8.RuneError && sz == 1:
		s.err(s.off, "illegal UTF-8 encoding")
		s.next()
		return r
	default:
		s.nextN(sz)
		return r
	}
}

func (s *Scanner) stringLiteral(off int32) {
	// Leadind " consumed.
	s.Tok.Ch = STRING_LIT
	for {
		switch {
		case s.c == '"':
			s.next()
			return
		case s.c == '\\':
			s.next()
			switch s.c {
			case '"', '\\', 'a', 'b', 'f', 'n', 'r', 't', 'v':
				s.next()
				continue
			case 'x', 'X':
				s.next()
				if !isHexDigit(s.c) {
					panic(todo("%v: %#U", s.position(), s.c))
				}

				s.next()
				if !isHexDigit(s.c) {
					panic(todo("%v: %#U", s.position(), s.c))
				}

				s.next()
				continue
			case 'u':
				s.u(4)
				continue
			case 'U':
				s.u(8)
				continue
			case '\'':
				s.err(off, "unknown escape")
				return
			default:
				switch {
				case isOctalDigit(s.c):
					s.next()
					if isOctalDigit(s.c) {
						s.next()
					}
					if isOctalDigit(s.c) {
						s.next()
					}
					continue
				default:
					panic(todo("%v: %#U", s.position(), s.c))
				}
			}
		case s.eof:
			s.err(off, "string literal not terminated")
			return
		case s.c == 0:
			s.err(s.off, "illegal character NUL")
		}

		switch {
		case s.c == '\t':
			// ok
		case s.c < ' ':
			s.err(s.off, "non-printable character: %#U", s.c)
			s.next()
		case s.c >= 0x80:
			off := s.off
			if s.rune() == 0xfeff {
				s.err(off, "illegal byte order mark")
			}
			continue
		}

		s.next()
	}
}

func (s *Scanner) u(n int) (r rune) {
	// Leading u/U not consumed.
	s.next()
	off := s.off
	for i := 0; i < n; i++ {
		switch {
		case isHexDigit(s.c):
			var n rune
			switch {
			case s.c >= '0' && s.c <= '9':
				n = rune(s.c) - '0'
			case s.c >= 'a' && s.c <= 'f':
				n = rune(s.c) - 'a'
			case s.c >= 'A' && s.c <= 'F':
				n = rune(s.c) - 'A'
			}
			r = 16*r + n
		default:
			switch {
			case s.eof:
				s.err(s.base+s.off, "escape sequence not terminated")
			default:
				s.err(s.off, "illegal character %#U in escape sequence", s.c)
			}
			return r
		}

		s.next()
	}
	if r > unicode.MaxRune {
		s.err(off, "escape sequence is invalid Unicode code point")
	}
	return r
}

func (s *Scanner) identifierOrKeyword() {
out:
	for {
		switch {
		case isIDNext(s.c):
			s.next()
		case s.c >= 0x80:
			switch r := s.rune(); {
			case unicode.IsLetter(r) || unicode.IsDigit(r):
				// already consumed
			default:
				s.err(s.off, "invalid character %#U in identifier", r)
				break out
			}
		case s.eof:
			break out
		case s.c == 0:
			break out
		default:
			break out
		}
	}
	if s.Tok.Ch = Keywords[string(s.Tok.src())]; s.Tok.Ch == 0 {
		s.Tok.Ch = IDENTIFIER
	}
}

func (s *Scanner) generalComment(off int32) (injectSemi bool) {
	// Leading /* consumed
	if s.CommentHandler != nil {
		defer func() {
			s.CommentHandler(off, s.source.buf[off:s.off])
		}()
	}
	var nl bool
	for {
		switch {
		case s.c == '*':
			s.next()
			switch s.c {
			case '/':
				s.next()
				if nl {
					return s.injectSemi()
				}

				return false
			}
		case s.c == '\n':
			nl = true
			s.next()
		case s.eof:
			s.Tok.Ch = 0
			s.err(off, "comment not terminated")
			return true
		case s.c == 0:
			panic(todo("%v: %#U", s.position(), s.c))
		default:
			s.next()
		}
	}
}

func (s *Scanner) lineComment(off int32) (injectSemi bool) {
	// Leading // consumed
	if s.CommentHandler != nil {
		defer func() {
			s.CommentHandler(off, s.source.buf[off:s.off])
		}()
	}
	for {
		switch {
		case s.c == '\n':
			if s.injectSemi() {
				return true
			}

			s.next()
			return false
		case s.c >= 0x80:
			if c := s.rune(); c == 0xfeff {
				s.err(off+2, "illegal byte order mark")
			}
		case s.eof:
			s.off++
			if s.injectSemi() {
				return true
			}

			return false
		case s.c == 0:
			return false
		default:
			s.next()
		}
	}
}

// Named values of Ch.
const (
	beforeTokens Ch = iota + 0xe000

	ADD_ASSIGN     // +=
	AND_ASSIGN     // &=
	AND_NOT        // &^
	AND_NOT_ASSIGN // &^=
	ARROW          // <-
	BREAK          // break
	CASE           // case
	CHAN           // chan
	CONST          // const
	CONTINUE       // continue
	DEC            // --
	DEFAULT        // default
	DEFER          // defer
	DEFINE         // :=
	ELLIPSIS       // ...
	ELSE           // else
	EOF            // end of file
	EQ             // ==
	FALLTHROUGH    // fallthrough
	FLOAT_LIT      // floating point literal
	FOR            // for
	FUNC           // func
	GE             // >=
	GO             // go
	GOTO           // goto
	IDENTIFIER     // identifier
	IF             // if
	IMAG_LIT       // imaginary literal
	IMPORT         // import
	INC            // ++
	INTERFACE      // interface
	INT_LIT        // integer literal
	LAND           // &&
	LE             // <=
	LOR            // ||
	MAP            // map
	MUL_ASSIGN     // *=
	NE             // !=
	OR_ASSIGN      // |=
	PACKAGE        // package
	QUO_ASSIGN     // /=
	RANGE          // range
	REM_ASSIGN     // %=
	RETURN         // return
	RUNE_LIT       // rune literal
	SELECT         // select
	SHL            // <<
	SHL_ASSIGN     // <<=
	SHR            // >>
	SHR_ASSIGN     // >>=
	STRING_LIT     // string literal
	STRUCT         // struct
	SUB_ASSIGN     // -=
	SWITCH         // switch
	TILDE          // ~
	TYPE           // type
	VAR            // var
	XOR_ASSIGN     // ^=

	body // body

	afterTokens
)

var xlat = map[Ch]token.Token{
	'!':            token.NOT,
	'%':            token.REM,
	'&':            token.AND,
	'(':            token.LPAREN,
	')':            token.RPAREN,
	'*':            token.MUL,
	'+':            token.ADD,
	',':            token.COMMA,
	'-':            token.SUB,
	'.':            token.PERIOD,
	'/':            token.QUO,
	':':            token.COLON,
	';':            token.SEMICOLON,
	'<':            token.LSS,
	'=':            token.ASSIGN,
	'>':            token.GTR,
	'[':            token.LBRACK,
	']':            token.RBRACK,
	'^':            token.XOR,
	'{':            token.LBRACE,
	'|':            token.OR,
	'}':            token.RBRACE,
	'~':            token.TILDE,
	ADD_ASSIGN:     token.ADD_ASSIGN,
	AND_ASSIGN:     token.AND_ASSIGN,
	AND_NOT:        token.AND_NOT,
	AND_NOT_ASSIGN: token.AND_NOT_ASSIGN,
	ARROW:          token.ARROW,
	BREAK:          token.BREAK,
	CASE:           token.CASE,
	CHAN:           token.CHAN,
	CONST:          token.CONST,
	CONTINUE:       token.CONTINUE,
	DEC:            token.DEC,
	DEFAULT:        token.DEFAULT,
	DEFER:          token.DEFER,
	DEFINE:         token.DEFINE,
	ELLIPSIS:       token.ELLIPSIS,
	ELSE:           token.ELSE,
	EOF:            token.EOF,
	EQ:             token.EQL,
	FALLTHROUGH:    token.FALLTHROUGH,
	FLOAT_LIT:      token.FLOAT,
	FOR:            token.FOR,
	FUNC:           token.FUNC,
	GE:             token.GEQ,
	GO:             token.GO,
	GOTO:           token.GOTO,
	IDENTIFIER:     token.IDENT,
	IF:             token.IF,
	IMAG_LIT:       token.IMAG,
	IMPORT:         token.IMPORT,
	INC:            token.INC,
	INTERFACE:      token.INTERFACE,
	INT_LIT:        token.INT,
	LAND:           token.LAND,
	LE:             token.LEQ,
	LOR:            token.LOR,
	MAP:            token.MAP,
	MUL_ASSIGN:     token.MUL_ASSIGN,
	NE:             token.NEQ,
	OR_ASSIGN:      token.OR_ASSIGN,
	PACKAGE:        token.PACKAGE,
	QUO_ASSIGN:     token.QUO_ASSIGN,
	RANGE:          token.RANGE,
	REM_ASSIGN:     token.REM_ASSIGN,
	RETURN:         token.RETURN,
	RUNE_LIT:       token.CHAR,
	SELECT:         token.SELECT,
	SHL:            token.SHL,
	SHL_ASSIGN:     token.SHL_ASSIGN,
	SHR:            token.SHR,
	SHR_ASSIGN:     token.SHR_ASSIGN,
	STRING_LIT:     token.STRING,
	STRUCT:         token.STRUCT,
	SUB_ASSIGN:     token.SUB_ASSIGN,
	SWITCH:         token.SWITCH,
	TYPE:           token.TYPE,
	VAR:            token.VAR,
	XOR_ASSIGN:     token.XOR_ASSIGN,
}

// Keywords represents the mapping of identifiers to Go reserved names.
var Keywords = map[string]Ch{
	"break":       BREAK,
	"case":        CASE,
	"chan":        CHAN,
	"const":       CONST,
	"continue":    CONTINUE,
	"default":     DEFAULT,
	"defer":       DEFER,
	"else":        ELSE,
	"fallthrough": FALLTHROUGH,
	"for":         FOR,
	"func":        FUNC,
	"go":          GO,
	"goto":        GOTO,
	"if":          IF,
	"import":      IMPORT,
	"interface":   INTERFACE,
	"map":         MAP,
	"package":     PACKAGE,
	"range":       RANGE,
	"return":      RETURN,
	"select":      SELECT,
	"struct":      STRUCT,
	"switch":      SWITCH,
	"type":        TYPE,
	"var":         VAR,
}
