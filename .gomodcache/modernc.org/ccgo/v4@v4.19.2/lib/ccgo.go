// Copyright 2022 The CCGO Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package ccgo implements the ccgo command.
package ccgo // import "modernc.org/ccgo/v4/lib"

//TODO Tucontext_t - Tucontext_t5
//TODO acosh u does not need to be pinned, need better escape analysis above "address taken"
//TODO add inlining infinite recursion protection

//  [0]: http://www.open-std.org/jtc1/sc22/wg14/www/docs/n1256.pdf

// -export-X, -unexport-X flags

import (
	"flag"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"modernc.org/cc/v4"
	"modernc.org/gc/v2"
	"modernc.org/opt"
	"modernc.org/strutil"
)

var (
	oTraceL = flag.Bool("trcl", false, "Print produced object files.")
	oTraceG = flag.Bool("trcg", false, "Print produced Go files.")

	isTesting bool
)

// Task represents a compilation job.
type Task struct {
	D                     []string            // -D
	I                     []string            // -I
	L                     []string            // -L
	O                     string              // -O
	U                     []string            // -U
	archiveLinkFiles      map[string]struct{} // path:
	args                  []string            // command name in args[0]
	buildLines            string              // //go:build ... and/or // +build ...
	cfg                   *cc.Config
	cfgArgs               []string
	cleanupDirs           []string
	compiledfFiles        map[string]string // *.c -> *.o.go
	cpp                   string            // -cpp <string>
	defs                  string
	routes                string // -map <comma separated list>
	fs                    fs.FS
	goABI                 *gc.ABI
	goarch                string   // -goos <string>
	goos                  string   // -goarch <string>
	hidden                nameSet  // -hide <string>
	idirafter             []string // -idirafter
	ignoreFile            nameSet  // -ignore-file=<comma separated file list>
	imports               []string // -import=<comma separated import list>
	include               []string // -include
	inputArchives         []string
	inputFiles            []string
	iquote                []string // -iquote
	isystem               []string // -isystem
	l                     []string // -l
	libc                  string   // --libc=modernc.org/libc/v2
	linkFiles             []string
	o                     string   // -o
	packageName           string   // --package-name
	predef                []string // --predef
	prefixAnonType        string
	prefixAutomatic       string // --prefix-automatic <string>
	prefixCcgoAutomatic   string
	prefixDefine          string // --prefix-define <string>
	prefixEnumerator      string // --prefix-enumerator <string>
	prefixExternal        string // --prefix-external <string>
	prefixField           string // --prefix-field <string>
	prefixImportQualifier string // --prefix-import-qualifier <string>
	prefixMacro           string // --prefix-macro <string>
	prefixStaticInternal  string // --prefix-static-internal <string>
	prefixStaticNone      string // --prefix-static-none <string>
	prefixTaggedEnum      string // --prefix-tagfed-enum <string>
	prefixTaggedStruct    string // --prefix-tagged-struct <string>
	prefixTaggedUnion     string // --prefix-taged-union <string>
	prefixTypename        string // --prefix-typename <string>
	prefixUndefined       string // --prefix-undefined <string>
	// The simple form "tool1,tool2" asks to route commands "tool1" and "tool2" via
	// ccgo. For example
	//
	//	-map ar,cc
	//
	// The other form, like "tool=bin" asks to route command "tool" via ccgo using
	// "bin". For example
	//
	//	-map ar=x86_64-w64-mingw32-gcc-ar,cc=x86_64-w64-mingw32-gcc
	//
	// The tool must be one of ar,cc,clang,gcc,libtool,ln,mv,rm.
	std          string // -std
	stderr       io.Writer
	stdout       io.Writer
	target       string
	tlsQualifier string              // eg. "libc."
	winapi       map[string]struct{} // --winapi
	winapiTest   string              // --winapi-test
	winabi       string              // --winabi

	intSize int

	E                            bool // -E
	absolutePaths                bool // -absolute-paths
	ansi                         bool // -ansi
	c                            bool // -c
	debugLinkerSave              bool // -debug-linker-save, causes pre type checking save of the linker result.
	emitFuncAliases              bool // -emit-func-aliases
	evalAllMacros                bool // -eval-all-macros
	freeStanding                 bool // -ffreestanding
	fullPaths                    bool // -full-paths
	header                       bool // -header
	ignoreAsmErrors              bool // -ignore-asm-errors
	ignoreLinkErrors             bool // -ignore-link-errors
	ignoreUnsupportedAligment    bool // -ignore-unsupported-alignment
	ignoreUnsupportedAtomicSizes bool // -ignore-unsupported-atomic-sizes
	ignoreVectorFunctions        bool // -ignore-vector-functions
	isExeced                     bool // -exec ...
	keepObjectFiles              bool // -keep-object-files
	keepStrings                  bool // -keep-strings
	m32                          bool // -m32
	m64                          bool // -m64
	noBuiltin                    bool // -fno-builtin
	noObjFmt                     bool // -no-object-file-format
	nostdinc                     bool // -nostdinc
	nostdlib                     bool // -nostdlib
	opt0                         bool // -O0
	packageNameSet               bool
	pedantic                     bool // -pedantic
	pedanticErrros               bool // -pedantic-errors
	positions                    bool // -positions
	prefixDefineSet              bool // --prefix-define <string>
	pthread                      bool // -pthread
	strictISOMode                bool // -ansi or stc=c90
	unsignedEnums                bool // -unsigned-enums
	verifyTypes                  bool // -verify-types
	winapiNoErrno                bool // --winapi-no-errno
}

// NewTask returns a newly created Task. args[0] is the command name.
func NewTask(goos, goarch string, args []string, stdout, stderr io.Writer, fs fs.FS) (r *Task) {
	return &Task{
		archiveLinkFiles: map[string]struct{}{},
		args:             args,
		compiledfFiles:   map[string]string{},
		routes:           "ar,cc,clang,gcc,libtool,ln,mv,rm",
		libc:             defaultLibcPackage,
		fs:               fs,
		goarch:           goarch,
		goos:             goos,
		prefixAnonType:   "_",
		stderr:           stderr,
		stdout:           stdout,
		target:           fmt.Sprintf("%s/%s", goos, goarch),
		tlsQualifier:     tag(importQualifier) + "libc.",
		winapi:           map[string]struct{}{},
	}
}

// Exec executes a task having the "-exec=foo" option.
func (t *Task) Exec() (err error) {
	// 	if dmesgs {
	// 		dmesg(
	// 			"==== task.Exec t.goos=%s t.goarch=%s IsExecEnv()=%v CC=%s\nt.args=%s",
	// 			t.goos, t.goarch, IsExecEnv(), os.Getenv("CC"), t.args,
	// 		)
	// 	}
	defer clearExecEnv()

	return t.Main()
}

// Main executes task.
func (t *Task) Main() (err error) {
	// 	if dmesgs {
	// 		dmesg(
	// 			"==== task.Main t.goos=%s t.goarch=%s IsExecEnv()=%v CC=%s\nt.args=%s",
	// 			t.goos, t.goarch, IsExecEnv(), os.Getenv("CC"), t.args,
	// 		)
	// 	}
	if ee := execEnv(); ee != "" {
		var flags []string
		if cflags := os.Getenv(cflagsEnvVar); cflags != "" {
			flags = strutil.SplitFields(cflags, commaSep)
		}
		return t.execed(ee, flags)
	}

	return t.main()
}

func (t *Task) main() (err error) {
	if dmesgs {
		dmesg(
			"==== task.main t.goos=%s t.goarch=%s IsExecEnv()=%v CC=%s\nt.args=%q",
			t.goos, t.goarch, IsExecEnv(), os.Getenv("CC"), t.args,
		)
	}
	switch len(t.args) {
	case 0:
		return errorf("invalid arguments")
	case 1:
		return errorf("no input files")
	}

	defer func() {
		for _, v := range t.cleanupDirs {
			os.RemoveAll(v)
		}
	}()

	// Defaults
	t.prefixField = "F"

	set := opt.NewSet()
	set.Arg("-cpp", true, func(arg, val string) error { t.cpp = strings.TrimSpace(val); return nil })
	set.Arg("-goarch", true, func(arg, val string) error { t.goarch = val; return nil })
	set.Arg("-goos", true, func(arg, val string) error { t.goos = val; return nil })
	set.Arg("-libc", false, func(arg, val string) error { t.libc = val; return nil })
	set.Arg("-package-name", false, func(arg, val string) error { t.packageName = val; t.packageNameSet = true; return nil })
	set.Arg("-predef", false, func(arg, val string) error { t.predef = append(t.predef, val); return nil })
	set.Arg("-prefix-automatic", false, func(arg, val string) error { t.prefixAutomatic = val; return nil })
	set.Arg("-prefix-define", false, func(arg, val string) error { t.prefixDefine = val; t.prefixDefineSet = true; return nil })
	set.Arg("-prefix-enumerator", false, func(arg, val string) error { t.prefixEnumerator = val; return nil })
	set.Arg("-prefix-external", false, func(arg, val string) error { t.prefixExternal = val; return nil })
	set.Arg("-prefix-field", false, func(arg, val string) error { t.prefixField = val; return nil })
	set.Arg("-prefix-import-qualifier", false, func(arg, val string) error { t.prefixImportQualifier = val; return nil })
	set.Arg("-prefix-macro", false, func(arg, val string) error { t.prefixMacro = val; return nil })
	set.Arg("-prefix-static-internal", false, func(arg, val string) error { t.prefixStaticInternal = val; return nil })
	set.Arg("-prefix-static-none", false, func(arg, val string) error { t.prefixStaticNone = val; return nil })
	set.Arg("-prefix-tagged-enum", false, func(arg, val string) error { t.prefixTaggedEnum = val; return nil })
	set.Arg("-prefix-tagged-struct", false, func(arg, val string) error { t.prefixTaggedStruct = val; return nil })
	set.Arg("-prefix-tagged-union", false, func(arg, val string) error { t.prefixTaggedUnion = val; return nil })
	set.Arg("-prefix-typename", false, func(arg, val string) error { t.prefixTypename = val; return nil })
	set.Arg("-prefix-undefined", false, func(arg, val string) error { t.prefixUndefined = val; return nil })
	set.Arg("D", true, func(arg, val string) error { t.D = append(t.D, fmt.Sprintf("%s%s", arg, val)); return nil })
	set.Arg("I", true, func(arg, val string) error { t.I = append(t.I, val); return nil })
	set.Arg("L", true, func(arg, val string) error { t.L = append(t.L, val); return nil })
	set.Arg("O", true, func(arg, val string) error { t.O = fmt.Sprintf("%s%s", arg, val); t.opt0 = val == "0"; return nil })
	set.Arg("U", true, func(arg, val string) error { t.U = append(t.U, fmt.Sprintf("%s%s", arg, val)); return nil })
	set.Arg("build-lines", false, func(arg, val string) error { t.buildLines = val; return nil })
	set.Arg("hide", false, func(arg, val string) error {
		for _, v := range strings.Split(val, ",") {
			t.hidden.add(v)
		}
		return nil
	})
	set.Arg("idirafter", true, func(arg, val string) error { t.idirafter = append(t.idirafter, val); return nil })
	set.Arg("include", true, func(arg, val string) error { t.include = append(t.include, val); return nil })
	set.Arg("ignore-file", false, func(arg, val string) error {
		for _, v := range strings.Split(val, ",") {
			t.ignoreFile.add(v)
		}
		return nil
	})
	set.Arg("import", false, func(arg, val string) error {
		t.imports = append(t.imports, strings.Split(val, ",")...)
		return nil
	})
	set.Arg("iquote", true, func(arg, val string) error { t.iquote = append(t.iquote, val); return nil })
	set.Arg("isystem", true, func(arg, val string) error { t.isystem = append(t.isystem, val); return nil })

	set.Arg("l", true, func(arg, val string) error {
		lib := "lib" + val + ".ago"
		for _, prefix := range t.L {
			fn := filepath.Join(prefix, lib)
			if _, err := os.Stat(fn); err != nil {
				continue
			}

			list, err := t.arExtract(fn)
			if err != nil {
				continue
			}
			t.linkFiles = append(t.linkFiles, list...)
			for _, v := range list {
				t.archiveLinkFiles[v] = struct{}{}
			}
			return nil
		}

		t.l = append(t.l, val)
		t.linkFiles = append(t.linkFiles, arg+"="+val)
		return nil
	})

	set.Arg("map", true, func(arg, val string) error { t.routes = val; return nil })
	set.Arg("o", true, func(arg, val string) error { t.o = val; return nil })
	set.Arg("std", true, func(arg, val string) error {
		t.std = fmt.Sprintf("%s=%s", arg, val)
		if val == "c90" {
			t.strictISOMode = true
		}
		return nil
	})
	set.Arg("-winabi", false, func(arg, val string) error { t.winabi = val; return nil })
	set.Arg("-winapi-test", false, func(arg, val string) error { t.winapiTest = val; return nil })
	set.Arg("-winapi", false, func(arg, val string) error {
		for _, v := range strings.Split(val, ",") {
			t.winapi[v] = struct{}{}
		}
		return nil
	})

	set.Opt("E", func(arg string) error { t.E = true; return nil })
	set.Opt("absolute-paths", func(arg string) error { t.absolutePaths = true; return nil })
	set.Opt("ansi", func(arg string) error { t.ansi = true; t.strictISOMode = true; return nil })
	set.Opt("c", func(arg string) error { t.c = true; return nil })
	set.Opt("debug-linker-save", func(arg string) error { t.debugLinkerSave = true; return nil })
	set.Opt("emit-func-aliases", func(arg string) error { t.emitFuncAliases = true; return nil })
	set.Opt("eval-all-macros", func(arg string) error { t.evalAllMacros = true; return nil })
	set.Opt("exec", func(arg string) error { return opt.Skip(nil) })
	set.Opt("extended-errors", func(arg string) error { extendedErrors = true; gc.ExtendedErrors = true; return nil })
	set.Opt("ffreestanding", func(arg string) error {
		t.freeStanding = true
		t.cfgArgs = append(t.cfgArgs, arg)
		return nil
	})
	set.Opt("fno-builtin", func(arg string) error { t.noBuiltin = true; t.cfgArgs = append(t.cfgArgs, arg); return nil })
	set.Opt("full-paths", func(arg string) error { t.fullPaths = true; return nil })
	set.Opt("header", func(arg string) error { t.header = true; return nil })
	set.Opt("ignore-asm-errors", func(arg string) error { t.ignoreAsmErrors = true; return nil })
	set.Opt("ignore-link-errors", func(arg string) error { t.ignoreLinkErrors = true; return nil })
	set.Opt("ignore-unsupported-alignment", func(arg string) error { t.ignoreUnsupportedAligment = true; return nil })
	set.Opt("ignore-unsupported-atomic-sizes", func(arg string) error { t.ignoreUnsupportedAtomicSizes = true; return nil })
	set.Opt("ignore-vector-functions", func(arg string) error { t.ignoreVectorFunctions = true; return nil })
	set.Opt("keep-object-files", func(arg string) error { t.keepObjectFiles = true; return nil })
	set.Opt("keep-strings", func(arg string) error { t.keepStrings = true; return nil })
	set.Opt("m32", func(arg string) error { t.m32 = true; return nil })
	set.Opt("m64", func(arg string) error { t.m64 = true; return nil })
	set.Opt("mlong-double-64", func(arg string) error { t.cfgArgs = append(t.cfgArgs, arg); return nil })
	set.Opt("no-object-file-format", func(arg string) error { t.noObjFmt = true; return nil })
	set.Opt("nostdinc", func(arg string) error { t.nostdinc = true; t.cfgArgs = append(t.cfgArgs, arg); return nil })
	set.Opt("nostdlib", func(arg string) error { t.nostdlib = true; return nil })
	set.Opt("pedantic", func(arg string) error { t.pedantic = true; return nil })
	set.Opt("pedantic-errors", func(arg string) error { t.pedanticErrros = true; return nil })
	set.Opt("positions", func(arg string) error { t.positions = true; return nil })
	set.Opt("pthread", func(arg string) error { t.pthread = true; t.cfgArgs = append(t.cfgArgs, arg); return nil })
	set.Opt("unsigned-enums", func(arg string) error { t.unsignedEnums = true; return nil })
	set.Opt("verify-types", func(arg string) error { t.verifyTypes = true; return nil })
	set.Opt("verify-types", func(arg string) error { t.verifyTypes = true; return nil })
	set.Opt("-winapi-no-errno", func(arg string) error { t.winapiNoErrno = true; return nil })

	// Ignored
	set.Arg("MF", true, func(arg, val string) error { return nil })
	set.Arg("MQ", true, func(arg, val string) error { return nil })
	set.Arg("MT", true, func(arg, val string) error { return nil })
	set.Arg("arch", true, func(arg, val string) error { return nil })
	set.Arg("gz", true, func(arg, val string) error { return nil })
	set.Arg("march", true, func(arg, val string) error { return nil })
	set.Arg("mtune", true, func(arg, val string) error { return nil })
	set.Opt("-version", func(arg string) error { return nil })
	set.Opt("M", func(arg string) error { return nil })
	set.Opt("MD", func(arg string) error { return nil })
	set.Opt("MM", func(arg string) error { return nil })
	set.Opt("MMD", func(arg string) error { return nil })
	set.Opt("MP", func(arg string) error { return nil })
	set.Opt("Qunused-arguments", func(arg string) error { return nil })
	set.Opt("Qunused-arguments", func(arg string) error { return nil })
	set.Opt("S", func(arg string) error { return nil })
	set.Opt("dumpmachine", func(arg string) error { return nil })
	set.Opt("dynamiclib", func(arg string) error { return nil })
	set.Opt("headerpad_max_install_names", func(arg string) error { return nil })
	set.Opt("herror_on_warning", func(arg string) error { return nil })
	set.Opt("mconsole", func(arg string) error { return nil })
	set.Opt("municode", func(arg string) error { return nil })
	set.Opt("mwindows", func(arg string) error { return nil })
	set.Opt("pipe", func(arg string) error { return nil })
	set.Opt("rdynamic", func(arg string) error { return nil })
	set.Opt("s", func(arg string) error { return nil })
	set.Opt("shared", func(arg string) error { return nil })
	set.Opt("static", func(arg string) error { return nil })
	set.Opt("static-libgcc", func(arg string) error { return nil })
	set.Opt("v", func(arg string) error { return nil })
	set.Opt("w", func(arg string) error { return nil })

	if err := set.Parse(t.args[1:], func(arg string) error {
		if strings.HasPrefix(arg, "-") {
			// 			if dmesgs {
			// 				dmesg("", errorf("unexpected/unsupported option: %q", arg))
			// 			}
			return errorf("unexpected/unsupported option: %s", arg)
		}

		if t.ignoreFile.has(arg) {
			return nil
		}

		nm := arg
		switch {
		case strings.HasSuffix(arg, ".c") || strings.HasSuffix(arg, ".h"):
			t.inputFiles = append(t.inputFiles, arg)
			t.linkFiles = append(t.linkFiles, arg)
			return nil
		case strings.HasSuffix(arg, ".go"):
			t.linkFiles = append(t.linkFiles, arg)
			return nil
		case strings.HasSuffix(arg, ".o"):
			t.linkFiles = append(t.linkFiles, t.goFile(arg))
			return nil
		case strings.HasSuffix(arg, ".a"):
			nm += "go" // foo.a -> foo.ago
			fallthrough
		case strings.HasSuffix(arg, ".ago"):
			t.inputArchives = append(t.inputArchives, nm)
			list, err := t.arExtract(nm)
			if err != nil {
				return err
			}

			t.linkFiles = append(t.linkFiles, list...)
			for _, v := range list {
				t.archiveLinkFiles[v] = struct{}{}
			}
			return nil
		case strings.HasSuffix(arg, ".def"):
			return nil
		}

		return errorf("unexpected argument %s", arg)
	}); err != nil {
		switch x := err.(type) {
		case opt.Skip:
			return t.exec([]string(x))
		default:
			return errorf("parsing %v: %v", t.args[1:], err)
		}
	}

	if t.goABI, err = gc.NewABI(t.goos, t.goarch); err != nil {
		return errorf("%v", err)
	}

	if t.m32 && t.goABI.Types[gc.Pointer].Size != 4 {
		return errorf("-m32 not supported on %s/%s", t.goos, t.goarch)
	}

	if t.m64 && t.goABI.Types[gc.Pointer].Size != 8 {
		return errorf("-m64 not supported on %s/%s", t.goos, t.goarch)
	}

	if t.buildLines == "" {
		t.buildLines = fmt.Sprintf("//go:build %[1]s && %[2]s", t.goos, t.goarch)
	}
	switch {
	case len(t.isystem) == 0 && !t.freeStanding && !t.nostdlib && t.libc == libcV2:
		isystem, err := isystem(t.goos, t.goarch, t.libc)
		if err != nil {
			return err
		}

		if isystem != "" {
			t.isystem = []string{isystem}
			t.D = append(t.D, "-D_GNU_SOURCE")
		}
	case len(t.isystem) == 0 && !t.freeStanding && !t.nostdlib && t.libc == libcV1:
		isystem, err := isystem(t.goos, t.goarch, t.libc)
		if err == nil && isystem != "" {
			t.isystem = []string{isystem}
			t.D = append(t.D, "-D_GNU_SOURCE")
		}
	}

	switch t.goarch {
	case "arm", "386":
		// modernc.org/libc@v1/sys/types/Off_t is 64 bit
		t.D = append(t.D, "-D_FILE_OFFSET_BITS=64")
	}
	switch t.goarch {
	case "arm", "386":
		t.intSize = 4
	default:
		t.intSize = 8
	}

	t.D = append(t.D, "-D__CCGO__")
	t.cfgArgs = append(t.cfgArgs, t.D...)
	t.cfgArgs = append(t.cfgArgs, t.U...)
	t.cfgArgs = append(t.cfgArgs,
		t.O,
		t.std,
	)
	ldflag := cc.LongDouble64Flag(t.goos, t.goarch)
	if ldflag != "" {
		t.cfgArgs = append(t.cfgArgs, ldflag)
	}

	if t.goos == "windows" && (t.goarch == "386" || t.goarch == "amd64") {
		t.cfgArgs = append(t.cfgArgs,
			"-mno-3dnow",
			"-mno-abm",
			"-mno-aes",
			"-mno-avx",
			"-mno-avx2",
			"-mno-avx512cd",
			"-mno-avx512er",
			"-mno-avx512f",
			"-mno-avx512pf",
			"-mno-bmi",
			"-mno-bmi2",
			"-mno-f16c",
			"-mno-fma",
			"-mno-fma4",
			"-mno-fsgsbase",
			"-mno-lwp",
			"-mno-lzcnt",
			"-mno-mmx",
			"-mno-pclmul",
			"-mno-popcnt",
			"-mno-prefetchwt1",
			"-mno-rdrnd",
			"-mno-sha",
			"-mno-sse",
			"-mno-sse2",
			"-mno-sse3",
			"-mno-sse4",
			"-mno-sse4.1",
			"-mno-sse4.2",
			"-mno-sse4a",
			"-mno-ssse3",
			"-mno-tbm",
			"-mno-xop",
		)
	}

	svCC := os.Getenv("CC")
	switch cpp := os.Getenv("CCGO_CPP"); {
	case t.cpp != "":
		setenv("CC", t.cpp)
	case cpp != "":
		setenv("CC", cpp)
	}
	// 	if dmesgs {
	// 		dmesg("cc.NewConfig(%q, %q, %q) CC=%q", t.goos, t.goarch, t.cfgArgs, os.Getenv("CC"))
	// 	}
	cfg, err := cc.NewConfig(t.goos, t.goarch, t.cfgArgs...)
	setenv("CC", svCC)
	if err != nil {
		return err
	}

	// if dmesgs {
	// 	dmesg("cfg.Predefined=%s", cfg.Predefined)
	// }
	cfg.UnsignedEnums = t.unsignedEnums
	cfg.EvalAllMacros = t.evalAllMacros
	if ldflag == "" {
		if err = cfg.AdjustLongDouble(); err != nil {
			return err
		}
	}

	if t.header {
		cfg.Header = true
	}

	if t.nostdinc {
		cfg.HostIncludePaths = nil
		cfg.HostSysIncludePaths = nil
	}

	// --------------------------------------------------------------------
	// https://gcc.gnu.org/onlinedocs/gcc/Directory-Options.html
	//
	// Directories specified with -iquote apply only to the quote form of the
	// directive, #include "file". Directories specified with -I, -isystem, or
	// -idirafter apply to lookup for both the #include "file" and #include <file>
	// directives.
	//
	// You can specify any number or combination of these options on the command
	// line to search for header files in several directories. The lookup order is
	// as follows:

	cfg.IncludePaths = nil
	cfg.SysIncludePaths = nil

	// 1 For the quote form of the include directive, the directory of the current
	//   file is searched first.
	cfg.IncludePaths = append(cfg.IncludePaths, "")

	// 2 For the quote form of the include directive, the directories specified by
	//   -iquote options are searched in left-to-right order, as they appear on the
	//   command line.
	cfg.IncludePaths = append(cfg.IncludePaths, t.iquote...)

	// 3 Directories specified with -I options are scanned in left-to-right order.
	cfg.IncludePaths = append(cfg.IncludePaths, t.I...)
	cfg.SysIncludePaths = append(cfg.SysIncludePaths, t.I...)

	// 4 Directories specified with -isystem options are scanned in left-to-right
	//   order.
	//
	// More info from https://gcc.gnu.org/onlinedocs/gcc/Directory-Options.html
	//
	// -isystem dir
	//
	// Search dir for header files, after all directories specified by -I but
	// before the standard system directories. Mark it as a system directory, so
	// that it gets the same special treatment as is applied to the standard system
	// directories. If dir begins with =, then the = will be replaced by the
	// sysroot prefix; see --sysroot and -isysroot.
	cfg.IncludePaths = append(cfg.IncludePaths, t.isystem...)
	// ... but before the standard directories.
	cfg.SysIncludePaths = append(append([]string(nil), t.isystem...), cfg.SysIncludePaths...)

	// 5 Standard system directories are scanned.
	cfg.IncludePaths = append(cfg.IncludePaths, cfg.HostIncludePaths...)
	cfg.IncludePaths = append(cfg.IncludePaths, cfg.HostSysIncludePaths...)
	cfg.SysIncludePaths = append(cfg.SysIncludePaths, cfg.HostIncludePaths...)
	cfg.SysIncludePaths = append(cfg.SysIncludePaths, cfg.HostSysIncludePaths...)

	// 6 Directories specified with -idirafter options are scanned in left-to-right
	//   order.
	cfg.IncludePaths = append(cfg.IncludePaths, t.idirafter...)
	cfg.SysIncludePaths = append(cfg.SysIncludePaths, t.idirafter...)
	// --------------------------------------------------------------------
	// trc("IncludePaths=%v", cfg.IncludePaths)
	// trc("SysIncludePaths=%v", cfg.SysIncludePaths)

	t.defs = buildDefs(t.D, t.U)
	cfg.FS = t.fs
	t.cfg = cfg
	if t.E {
		for _, ifn := range t.inputFiles {
			sources, err := sourcesFor(cfg, ifn, t)
			if err != nil {
				return err
			}

			if err := cc.Preprocess(cfg, sources, t.stdout); err != nil {
				return err
			}
		}
		return nil
	}

	if t.nostdlib || t.freeStanding {
		t.tlsQualifier = ""
	}
	if t.c {
		return t.compile(t.o)
	}

	if !t.nostdlib {
		t.linkFiles = append(t.linkFiles, "-l=c")
	}
	t.L = append(t.L, defaultLibs)
	return t.link()
}

func (t *Task) arExtract(fn string) (r []string, err error) {
	ar := "ar"
	if t.isExeced {
		ar = os.Getenv("CCGO_AR")
	}
	tmp, err := os.MkdirTemp("", "ccgo-tmp-ar-")
	if err != nil {
		return nil, errorf("%v", err)
	}

	out, err := exec.Command(ar, "t", fn).CombinedOutput()
	// 	if dmesgs {
	// 		dmesg("fn=%s out=%s err=%v", fn, out, err)
	// 	}
	if err != nil {
		return nil, errorf("%s: %s\nFAIL: %v", ar, out, err)
	}

	m := map[string]struct{}{}
	for _, v := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		w := filepath.Join(tmp, strings.TrimSpace(v))
		if _, ok := m[w]; !ok {
			r = append(r, w)
		}
		m[w] = struct{}{}
	}
	if !t.keepObjectFiles {
		t.cleanupDirs = append(t.cleanupDirs, tmp)
	}
	switch runtime.GOOS {
	case "freebsd", "darwin", "openbsd", "windows":
		fn, err := filepath.Abs(fn)
		if err != nil {
			return nil, errorf("%v", err)
		}

		cmd := exec.Command(ar, "x", fn)
		cmd.Dir = tmp
		out, err = cmd.CombinedOutput()
		// 		if dmesgs {
		// 			dmesg("fn=%s out=%s err=%v", fn, out, err)
		// 		}
		if err != nil {
			return nil, errorf("%s: %s\nFAIL: %v", ar, out, err)
		}
	default:
		out, err = exec.Command(ar, "x", "--output", tmp, fn).CombinedOutput()
		// 		if dmesgs {
		// 			dmesg("fn=%s out=%s err=%v", fn, out, err)
		// 		}
		if err != nil {
			return nil, errorf("%s: %s\nFAIL: %v", ar, out, err)
		}
	}

	// 	if dmesgs {
	// 		for _, v := range r {
	// 			dmesg("ar extracted %s", v)
	// 		}
	// 	}
	return r, nil
}

func sourcesFor(cfg *cc.Config, fn string, t *Task) (r []cc.Source, err error) {
	predef := cfg.Predefined
	if len(t.predef) != 0 {
		predef += "\n" + strings.Join(t.predef, "\n")
	}
	r = []cc.Source{
		{Name: "<predefined>", Value: predef},
		{Name: "<builtin>", Value: cc.Builtin},
	}
	if t.defs != "" {
		r = append(r, cc.Source{Name: "<command-line>", Value: t.defs})
	}
	// -include file
	//
	// Process file as if #include "file" appeared as the first line of the primary
	// source file. However, the first directory searched for file is the
	// preprocessor’s working directory instead of the directory containing the
	// main source file. If not found there, it is searched for in the remainder of
	// the #include "…" search chain as normal.
	//
	// If multiple -include options are given, the files are included in the order
	// they appear on the command line.
	paths := append([]string{"."}, t.include...)
next:
	for _, v := range t.include {
		for _, w := range paths {
			path := filepath.Join(w, v)
			fi, err := os.Stat(path)
			if err != nil || !fi.Mode().IsRegular() {
				continue
			}

			r = append(r, cc.Source{Name: path, FS: cfg.FS})
			continue next
		}

		return nil, errorf("-include %s: not found", v)
	}
	return append(r, cc.Source{Name: fn, FS: cfg.FS}), nil
}

// -c
func (t *Task) compile(optO string) error {
	switch len(t.inputFiles) {
	case 0:
		return errorf("no input files")
	case 1:
		// ok
	default:
		if t.o != "" && t.c {
			return errorf("cannot specify '-o' with '-c' with multiple files")
		}
	}

	p := newParallel("")
	for _, ifn := range t.inputFiles {
		ifn := ifn
		ofn := optO
		if ofn == "" {
			switch filepath.Ext(ifn) {
			case ".c":
				ofn = filepath.Base(ifn)
				ofn = ofn[:len(ofn)-len(".c")] + ".o.go"
			default:
				ofn = filepath.Base(ifn) + ".go"
			}
		}
		t.compiledfFiles[ifn] = ofn
		p.exec(func() error { return newCtx(t, p.eh).compile(ifn, ofn) })
	}
	return p.wait()
}

func setenv(nm, val string) {
	os.Setenv(nm, val)
	//	if dmesgs {
	//		dmesg("os.Setenv(%q, %q)", nm, val)
	//	}
}
