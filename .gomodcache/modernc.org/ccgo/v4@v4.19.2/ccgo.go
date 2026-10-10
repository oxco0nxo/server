// Copyright 2022 The CCGO Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Command ccgo is a C compiler producing Go code.
package main // import "modernc.org/ccgo/v4"

import (
	"fmt"
	"os"
	"runtime"

	ccgo3 "modernc.org/ccgo/v3/lib"
	ccgo4 "modernc.org/ccgo/v4/lib"
)

func main() {
	var err error
	goarch := env("TARGET_GOARCH", env("GOARCH", runtime.GOARCH))
	goos := env("TARGET_GOOS", env("GOOS", runtime.GOOS))
	switch {
	case len(os.Args) > 1 && os.Args[1] == "-v3":
		err = ccgo3.NewTask(append([]string{os.Args[0]}, os.Args[2:]...), os.Stdout, os.Stderr).Main()
	case len(os.Args) > 1 && os.Args[1] == "-v4":
		err = ccgo4.NewTask(goos, goarch, append([]string{os.Args[0]}, os.Args[2:]...), os.Stdout, os.Stderr, nil).Main()
	default:
		err = ccgo4.NewTask(goos, goarch, os.Args, os.Stdout, os.Stderr, nil).Main()
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func env(name, deflt string) (r string) {
	r = deflt
	if s := os.Getenv(name); s != "" {
		r = s
	}
	return r
}
