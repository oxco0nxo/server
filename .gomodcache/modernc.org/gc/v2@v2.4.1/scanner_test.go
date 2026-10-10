// Copyright 2009 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the GO-LICENSE file.

// The original file is at:
//
//	https://github.com/golang/go/blob/master/src/go/scanner/scanner_test.go

// Modification Copyright 2021 The Gc Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package gc // import "modernc.org/gc/v2"

import (
	"fmt"
	"strings"
	"testing"
)

func testScanErrors(t *testing.T) {
	for itest, test := range []struct {
		src string
		ch  Ch
		off int
		_   string // not used
		err string
	}{
		{"'\n   ", RUNE_LIT, 1, "'", "non-printable character: U+000A"},
		{"'\n", RUNE_LIT, 1, "'", "non-printable character: U+000A"},
		{"'\ufeff" + `'`, RUNE_LIT, 1, "'\ufeff" + `'`, "illegal byte order mark"}, // only first BOM is ignored
		{"'n   ", RUNE_LIT, 2, "'", "rune literal not terminated"},
		{"'n", RUNE_LIT, 2, "'", "rune literal not terminated"},
		{"..", '.', 0, "", ""}, // two periods, not invalid token (issue #28112)
		{"/*", 0, 0, "/*", "comment not terminated"},
		{"/**/", EOF, 0, "/**/", ""},
		{"//\ufeff", EOF, 2, "//\ufeff", "illegal byte order mark"}, // only first BOM is ignored
		{"07090000008", INT_LIT, 3, "07090000008", "invalid digit '9' in octal literal"},
		{"077", INT_LIT, 0, "077", ""},
		{"078", INT_LIT, 2, "078", "invalid digit '8' in octal literal"},
		{"078.", FLOAT_LIT, 0, "078.", ""},
		{"07801234567.", FLOAT_LIT, 0, "07801234567.", ""},
		{"078e0", FLOAT_LIT, 0, "078e0", ""},
		{"0E", FLOAT_LIT, 2, "0E", "exponent has no digits"}, // issue 17621
		{"0x", INT_LIT, 2, "0x", "hexadecimal literal has no digits"},
		{"\"abc\n   ", STRING_LIT, 4, `"abc`, "non-printable character: U+000A"},
		{"\"abc\n", STRING_LIT, 4, `"abc`, "non-printable character: U+000A"},
		{"\"abc\x00def\"", STRING_LIT, 4, "\"abc\x00def\"", "illegal character NUL"},
		{"\"abc\x80def\"", STRING_LIT, 4, "\"abc\x80def\"", "illegal UTF-8 encoding"},
		{"\"abcn   ", STRING_LIT, 0, `"abc`, "string literal not terminated"},
		{"\"abcn", STRING_LIT, 0, `"abc`, "string literal not terminated"},
		{"\a", 0, 0, "", "illegal character U+0007"},
		{"\ufeff\ufeff", 0, 3, "\ufeff\ufeff", "illegal byte order mark"}, // only first BOM is ignored
		{"`", STRING_LIT, 0, "`", "raw string literal not terminated"},
		{"``", STRING_LIT, 0, "``", ""},
		{`""`, STRING_LIT, 0, `""`, ""},
		{`"` + "abc\ufeffdef" + `"`, STRING_LIT, 4, `"` + "abc\ufeffdef" + `"`, "illegal byte order mark"}, // only first BOM is ignored
		{`"abc`, STRING_LIT, 0, `"abc`, "string literal not terminated"},
		{`#`, 0, 0, "", "illegal character U+0023 '#'"},
		{`' '`, RUNE_LIT, 0, `' '`, ""},
		{`''`, RUNE_LIT, 0, `''`, "illegal rune literal"},
		{`'12'`, RUNE_LIT, 0, `'12'`, "illegal rune literal"},
		{`'123'`, RUNE_LIT, 0, `'123'`, "illegal rune literal"},
		{`'\0'`, RUNE_LIT, 3, `'\0'`, "illegal character U+0027 ''' in escape sequence"},
		{`'\07'`, RUNE_LIT, 4, `'\07'`, "illegal character U+0027 ''' in escape sequence"},
		{`'\08'`, RUNE_LIT, 3, `'\08'`, "illegal character U+0038 '8' in escape sequence"},
		{`'\8'`, RUNE_LIT, 2, `'\8'`, "unknown escape sequence"},
		{`'\U'`, RUNE_LIT, 3, `'\U'`, "illegal character U+0027 ''' in escape sequence"},
		{`'\U0'`, RUNE_LIT, 4, `'\U0'`, "illegal character U+0027 ''' in escape sequence"},
		{`'\U00'`, RUNE_LIT, 5, `'\U00'`, "illegal character U+0027 ''' in escape sequence"},
		{`'\U000'`, RUNE_LIT, 6, `'\U000'`, "illegal character U+0027 ''' in escape sequence"},
		{`'\U0000'`, RUNE_LIT, 7, `'\U0000'`, "illegal character U+0027 ''' in escape sequence"},
		{`'\U00000'`, RUNE_LIT, 8, `'\U00000'`, "illegal character U+0027 ''' in escape sequence"},
		{`'\U000000'`, RUNE_LIT, 9, `'\U000000'`, "illegal character U+0027 ''' in escape sequence"},
		{`'\U0000000'`, RUNE_LIT, 10, `'\U0000000'`, "illegal character U+0027 ''' in escape sequence"},
		{`'\U00000000'`, RUNE_LIT, 0, `'\U00000000'`, ""},
		{`'\U0000000`, RUNE_LIT, 10, `'\U0000000`, "escape sequence not terminated"},
		{`'\Uffffffff'`, RUNE_LIT, 3, `'\Uffffffff'`, "escape sequence is invalid Unicode code point"},
		{`'\`, RUNE_LIT, 2, `'\`, "escape sequence not terminated"},
		{`'\u'`, RUNE_LIT, 3, `'\u'`, "illegal character U+0027 ''' in escape sequence"},
		{`'\u0'`, RUNE_LIT, 4, `'\u0'`, "illegal character U+0027 ''' in escape sequence"},
		{`'\u00'`, RUNE_LIT, 5, `'\u00'`, "illegal character U+0027 ''' in escape sequence"},
		{`'\u000'`, RUNE_LIT, 6, `'\u000'`, "illegal character U+0027 ''' in escape sequence"},
		{`'\u0000'`, RUNE_LIT, 0, `'\u0000'`, ""},
		{`'\u000`, RUNE_LIT, 6, `'\u000`, "escape sequence not terminated"},
		{`'\x'`, RUNE_LIT, 3, `'\x'`, "illegal character U+0027 ''' in escape sequence"},
		{`'\x0'`, RUNE_LIT, 4, `'\x0'`, "illegal character U+0027 ''' in escape sequence"},
		{`'\x00'`, RUNE_LIT, 0, "", ""},
		{`'\x0g'`, RUNE_LIT, 4, `'\x0g'`, "illegal character U+0067 'g' in escape sequence"},
		{`'`, RUNE_LIT, 0, `'`, "rune literal not terminated"},
		{`…`, 0, 0, "", "illegal character U+2026 '…'"},
	} {
		s, err := NewScanner(fmt.Sprintf("%d.go", itest), []byte(test.src))
		if err != nil {
			t.Fatalf("%v: %v", itest, err)
		}

		if !s.Scan() && s.Tok.Ch != EOF {
			t.Fatalf("%v: unexpected EOF", itest)
		}

		if g, e := s.Tok.Ch, test.ch; g != e {
			t.Errorf("%v: %q: got %s, expected %s", itest, test.src, g, e)
		}
		switch {
		case len(s.errs) != 0:
			if g, e := s.errs[0].err.Error(), test.err; g != e {
				t.Errorf("%v: %q: got err %q, expected %q", itest, test.src, g, test.err)
			}
			if g, e := s.errs[0].pos.Offset, test.off; g != e {
				t.Errorf("%v: %q: got error offset %d, expected %d", itest, test.src, g, e)
			}
		default:
			if test.err != "" {
				t.Errorf("%q: missing error %q", test.src, test.err)
			}
		}
	}
}

func testNumbers(t *testing.T) {
	for itest, test := range []struct {
		tok              Ch
		src, tokens, err string
	}{
		// binaries
		{INT_LIT, "0b0", "0b0", ""},
		{INT_LIT, "0b1010", "0b1010", ""},
		{INT_LIT, "0B1110", "0B1110", ""},

		{INT_LIT, "0b", "0b", "binary literal has no digits"},
		{INT_LIT, "0b0190", "0b0190", "invalid digit '9' in binary literal"},
		{INT_LIT, "0b01a0", "0b01 a0", ""}, // only accept 0-9

		{FLOAT_LIT, "0b.", "0b.", "invalid radix point in binary literal"},

		// We do not support the error handling of this original
		// go/scanner test case as `1b` is not a valid number prefix.
		// {FLOAT_LIT, "1b.1", "0b.1", "invalid radix point in binary literal"},

		{FLOAT_LIT, "0b1.0", "0b1.0", "invalid radix point in binary literal"},
		{FLOAT_LIT, "0b1e10", "0b1e10", "'e' exponent requires decimal mantissa"},
		{FLOAT_LIT, "0b1P-1", "0b1P-1", "'P' exponent requires hexadecimal mantissa"},

		{IMAG_LIT, "0b10i", "0b10i", ""},
		{IMAG_LIT, "0b10.0i", "0b10.0i", "invalid radix point in binary literal"},

		// // octals
		{INT_LIT, "0o0", "0o0", ""},
		{INT_LIT, "0o1234", "0o1234", ""},
		{INT_LIT, "0O1234", "0O1234", ""},

		{INT_LIT, "0o", "0o", "octal literal has no digits"},
		{INT_LIT, "0o8123", "0o8123", "invalid digit '8' in octal literal"},
		{INT_LIT, "0o1293", "0o1293", "invalid digit '9' in octal literal"},
		{INT_LIT, "0o12a3", "0o12 a3", ""}, // only accept 0-9

		{FLOAT_LIT, "0o.", "0o.", "invalid radix point in octal literal"},
		{FLOAT_LIT, "0o.2", "0o.2", "invalid radix point in octal literal"},
		{FLOAT_LIT, "0o1.2", "0o1.2", "invalid radix point in octal literal"},
		{FLOAT_LIT, "0o1E+2", "0o1E+2", "'E' exponent requires decimal mantissa"},
		{FLOAT_LIT, "0o1p10", "0o1p10", "'p' exponent requires hexadecimal mantissa"},

		{IMAG_LIT, "0o10i", "0o10i", ""},
		{IMAG_LIT, "0o10e0i", "0o10e0i", "'e' exponent requires decimal mantissa"},

		// // 0-octals
		{INT_LIT, "0", "0", ""},
		{INT_LIT, "0123", "0123", ""},

		{INT_LIT, "08123", "08123", "invalid digit '8' in octal literal"},
		{INT_LIT, "01293", "01293", "invalid digit '9' in octal literal"},
		{INT_LIT, "0F.", "0 F .", ""}, // only accept 0-9
		{INT_LIT, "0123F.", "0123 F .", ""},
		{INT_LIT, "0123456x", "0123456 x", ""},

		// // decimals
		{INT_LIT, "1", "1", ""},
		{INT_LIT, "1234", "1234", ""},

		{INT_LIT, "1f", "1 f", ""}, // only accept 0-9

		{IMAG_LIT, "0i", "0i", ""},
		{IMAG_LIT, "0678i", "0678i", ""},

		// // decimal floats
		{FLOAT_LIT, "0.", "0.", ""},
		{FLOAT_LIT, "123.", "123.", ""},
		{FLOAT_LIT, "0123.", "0123.", ""},

		{FLOAT_LIT, ".0", ".0", ""},
		{FLOAT_LIT, ".123", ".123", ""},
		{FLOAT_LIT, ".0123", ".0123", ""},

		{FLOAT_LIT, "0.0", "0.0", ""},
		{FLOAT_LIT, "123.123", "123.123", ""},
		{FLOAT_LIT, "0123.0123", "0123.0123", ""},

		{FLOAT_LIT, "0e0", "0e0", ""},
		{FLOAT_LIT, "123e+0", "123e+0", ""},
		{FLOAT_LIT, "0123E-1", "0123E-1", ""},

		{FLOAT_LIT, "0.e+1", "0.e+1", ""},
		{FLOAT_LIT, "123.E-10", "123.E-10", ""},
		{FLOAT_LIT, "0123.e123", "0123.e123", ""},

		{FLOAT_LIT, ".0e-1", ".0e-1", ""},
		{FLOAT_LIT, ".123E+10", ".123E+10", ""},
		{FLOAT_LIT, ".0123E123", ".0123E123", ""},

		{FLOAT_LIT, "0.0e1", "0.0e1", ""},
		{FLOAT_LIT, "123.123E-10", "123.123E-10", ""},
		{FLOAT_LIT, "0123.0123e+456", "0123.0123e+456", ""},

		{FLOAT_LIT, "0e", "0e", "exponent has no digits"},
		{FLOAT_LIT, "0E+", "0E+", "exponent has no digits"},
		{FLOAT_LIT, "1e+f", "1e+ f", "exponent has no digits"},
		{FLOAT_LIT, "0p0", "0p0", "'p' exponent requires hexadecimal mantissa"},
		{FLOAT_LIT, "1.0P-1", "1.0P-1", "'P' exponent requires hexadecimal mantissa"},

		{IMAG_LIT, "0.i", "0.i", ""},
		{IMAG_LIT, ".123i", ".123i", ""},
		{IMAG_LIT, "123.123i", "123.123i", ""},
		{IMAG_LIT, "123e+0i", "123e+0i", ""},
		{IMAG_LIT, "123.E-10i", "123.E-10i", ""},
		{IMAG_LIT, ".123E+10i", ".123E+10i", ""},

		// // hexadecimals
		{INT_LIT, "0x0", "0x0", ""},
		{INT_LIT, "0x1234", "0x1234", ""},
		{INT_LIT, "0xcafef00d", "0xcafef00d", ""},
		{INT_LIT, "0XCAFEF00D", "0XCAFEF00D", ""},

		// We do not support the error handling of this original
		// go/scanner test case as `1x` is not a valid number prefix.
		// {INT_LIT, "1x", "0x", "hexadecimal literal has no digits"},

		{INT_LIT, "0x1g", "0x1 g", ""},

		{IMAG_LIT, "0xf00i", "0xf00i", ""},

		// // hexadecimal floats
		{FLOAT_LIT, "0x0p0", "0x0p0", ""},
		{FLOAT_LIT, "0x12efp-123", "0x12efp-123", ""},
		{FLOAT_LIT, "0xABCD.p+0", "0xABCD.p+0", ""},
		{FLOAT_LIT, "0x.0189P-0", "0x.0189P-0", ""},
		{FLOAT_LIT, "0x1.ffffp+1023", "0x1.ffffp+1023", ""},

		{FLOAT_LIT, "0x.", "0x.", "hexadecimal literal has no digits"},
		{FLOAT_LIT, "0x0.", "0x0.", "hexadecimal mantissa requires a 'p' exponent"},
		{FLOAT_LIT, "0x.0", "0x.0", "hexadecimal mantissa requires a 'p' exponent"},
		{FLOAT_LIT, "0x1.1", "0x1.1", "hexadecimal mantissa requires a 'p' exponent"},
		{FLOAT_LIT, "0x1.1e0", "0x1.1e0", "hexadecimal mantissa requires a 'p' exponent"},
		{FLOAT_LIT, "0x1.2gp1a", "0x1.2 gp1a", "hexadecimal mantissa requires a 'p' exponent"},
		{FLOAT_LIT, "0x0p", "0x0p", "exponent has no digits"},
		{FLOAT_LIT, "0xeP-", "0xeP-", "exponent has no digits"},
		{FLOAT_LIT, "0x1234PAB", "0x1234P AB", "exponent has no digits"},
		{FLOAT_LIT, "0x1.2p1a", "0x1.2p1 a", ""},

		{IMAG_LIT, "0xf00.bap+12i", "0xf00.bap+12i", ""},

		// // separators
		{INT_LIT, "0b_1000_0001", "0b_1000_0001", ""},
		{INT_LIT, "0o_600", "0o_600", ""},
		{INT_LIT, "0_466", "0_466", ""},
		{INT_LIT, "1_000", "1_000", ""},
		{FLOAT_LIT, "1_000.000_1", "1_000.000_1", ""},
		{IMAG_LIT, "10e+1_2_3i", "10e+1_2_3i", ""},
		{INT_LIT, "0x_f00d", "0x_f00d", ""},
		{FLOAT_LIT, "0x_f00d.0p1_2", "0x_f00d.0p1_2", ""},

		{INT_LIT, "0b__1000", "0b__1000", "'_' must separate successive digits"},
		{INT_LIT, "0o60___0", "0o60___0", "'_' must separate successive digits"},
		{INT_LIT, "0466_", "0466_", "'_' must separate successive digits"},
		{FLOAT_LIT, "1_.", "1_.", "'_' must separate successive digits"},
		{FLOAT_LIT, "0._1", "0._1", "'_' must separate successive digits"},
		{FLOAT_LIT, "2.7_e0", "2.7_e0", "'_' must separate successive digits"},
		{IMAG_LIT, "10e+12_i", "10e+12_i", "'_' must separate successive digits"},
		{INT_LIT, "0x___0", "0x___0", "'_' must separate successive digits"},
		{FLOAT_LIT, "0x1.0_p0", "0x1.0_p0", "'_' must separate successive digits"},
	} {
		s, err := NewScanner(fmt.Sprintf("%d.go", itest), []byte(test.src))
		if err != nil {
			t.Fatalf("%v: %v", itest, err)
		}

		for i, want := range strings.Split(test.tokens, " ") {
			if !s.Scan() {
				t.Errorf("%v: unecpected EOF", itest)
				continue
			}

			if i == 0 {
				if g, e := s.Tok.Ch, test.tok; g != e {
					t.Errorf("%q: got token %s; want %s", test.src, g, e)
				}

				switch {
				case len(s.errs) != 0:
					if g, e := s.errs[0].err.Error(), test.err; g != e {
						t.Errorf("%v: %q: got err %q, expected %q", i, test.src, g, test.err)
					}
				default:
					if test.err != "" {
						t.Errorf("%q: missing error %q", test.src, test.err)
					}
				}
			}

			if g, e := s.Tok.Src(), want; g != e {
				t.Errorf("%q: got literal %q (%s); want %s", test.src, g, s.Tok.Ch, e)
			}
		}

		// make sure we read all
		s.Scan()
		if s.Tok.Ch == ';' {
			s.Scan()
		}
		if s.Tok.Ch != EOF {
			t.Errorf("%q: got %s; want EOF", test.src, s.Tok.Ch)
		}
	}
}
