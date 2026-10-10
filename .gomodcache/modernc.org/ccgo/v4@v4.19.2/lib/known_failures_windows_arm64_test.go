// Copyright 2022 The CCGO Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package ccgo // import "modernc.org/ccgo/v4/lib"

var testExecKnownFails = map[string]struct{}{
	// ==== EXEC FAIL - compiles and builds but fails when executed.

	// // Won't fix: setjmp/longjmp
	// `assets/github.com/vnmakarov/mir/c-benchmarks/except.c`: {},
	// `assets/github.com/vnmakarov/mir/c-tests/new/setjmp.c`:  {},
	// `assets/github.com/vnmakarov/mir/c-tests/new/setjmp2.c`: {},

	// // Won't fix: sigfpe
	// `assets/gcc-9.1.0/gcc/testsuite/gcc.c-torture/execute/20101011-1.c`:                    {},
	// `assets/gcc-9.1.0/gcc/testsuite/gcc.c-torture/execute/ieee/fp-cmp-1.c`:                 {},
	// `assets/gcc-9.1.0/gcc/testsuite/gcc.c-torture/execute/ieee/fp-cmp-2.c`:                 {},
	// `assets/gcc-9.1.0/gcc/testsuite/gcc.c-torture/execute/ieee/fp-cmp-3.c`:                 {},
	// `assets/github.com/gcc-mirror/gcc/gcc/testsuite/gcc.c-torture/execute/20101011-1.c`:    {},
	// `assets/github.com/gcc-mirror/gcc/gcc/testsuite/gcc.c-torture/execute/ieee/fp-cmp-1.c`: {},
	// `assets/github.com/gcc-mirror/gcc/gcc/testsuite/gcc.c-torture/execute/ieee/fp-cmp-2.c`: {},
	// `assets/github.com/gcc-mirror/gcc/gcc/testsuite/gcc.c-torture/execute/ieee/fp-cmp-3.c`: {},

	// // Won't fix: Architecture specific conversion overflow.
	// `assets/gcc-9.1.0/gcc/testsuite/gcc.c-torture/execute/20031003-1.c`:                 {},
	// `assets/github.com/gcc-mirror/gcc/gcc/testsuite/gcc.c-torture/execute/20031003-1.c`: {},

	// // Won't fix: implementation defined bit fields
	// `assets/github.com/vnmakarov/mir/c-tests/lacc/bitfield-basic.c`:         {},
	// `assets/github.com/vnmakarov/mir/c-tests/lacc/bitfield-pack-next.c`:     {},
	// `assets/github.com/vnmakarov/mir/c-tests/lacc/bitfield-trailing-zero.c`: {},
	// `assets/github.com/vnmakarov/mir/c-tests/lacc/bitfield-types-init.c`:    {},

	// // Won't fix: long double
	// `assets/github.com/vnmakarov/mir/c-tests/lacc/long-double-load.c`: {},

	// // Won't fix: return_addr
	// `assets/github.com/gcc-mirror/gcc/gcc/testsuite/gcc.c-torture/execute/return-addr.c`: {},

	// // Won't fix, puts particular return value isn't specified
	// `assets/github.com/vnmakarov/mir/c-tests/lacc/macro-paste.c`: {},
	// `assets/github.com/vnmakarov/mir/c-tests/lacc/whitespace.c`:  {},

	//TODO
	`assets/CompCert-3.6/test/c/mandelbrot.c`: {}, // EXEC FAIL

	// ==== BUILD FAIL - compiles but does not build.

	// ==== COMPILE FAIL - does not compile.

}
