// Copyright 2022 The CCGO Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package ccgo // import "modernc.org/ccgo/v4/lib"

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime/debug"
	"strings"
	"time"

	"modernc.org/opt"
	"modernc.org/strutil"
)

const (
	execEnvVar   = "CCGO_EXEC_ENV"
	cflagsEnvVar = "CCGO_EXEC_CFLAGS"
	commaSep     = ","
)

func (t *Task) exec(args []string) (err error) {
	// 	if dmesgs {
	// 		dmesg(
	// 			"==== task.exec t.goos=%s t.goarch=%s IsExecEnv()=%v CC=%s\nargs=%q\nt.args=%q",
	// 			t.goos, t.goarch, IsExecEnv(), os.Getenv("CC"), args, t.args,
	// 		)
	// 	}
	if len(args) == 0 {
		return errorf("-exec: missing command")
	}

	if s := os.Getenv(execEnvVar); s != "" {
		return errorf("-exec: %s already set: %q", execEnvVar, s)
	}

	cflags := t.args[1 : (len(t.args))-len(args)-1] // -1 for the final "-exec"
	setenv(cflagsEnvVar, strutil.JoinFields(cflags, commaSep))
	self, err := os.Executable()
	if err != nil {
		return err
	}

	dirTemp, err := os.MkdirTemp("", "")
	if err != nil {
		return err
	}

	defer os.RemoveAll(dirTemp)

	restorePath := os.Getenv("PATH")

	defer setenv("PATH", restorePath)

	setenv("PATH", fmt.Sprintf("%s%c%s", dirTemp, os.PathListSeparator, restorePath))
	var a []string
	// 	if dmesgs {
	// 		dmesg("t.routes=%s", t.routes)
	// 	}
	for _, v := range strings.Split(t.routes, commaSep) {
		pair := strings.SplitN(v, "=", 2)
		tool := pair[0]
		if hostBin, err := exec.LookPath(tool); err == nil {
			setenv(fmt.Sprintf("CCGO_%s", strings.ToUpper(tool)), hostBin)
		}
		bin := tool
		if len(pair) == 2 {
			bin = pair[1]
		}
		bin, err = exec.LookPath(bin)
		if err != nil {
			// 			if dmesgs {
			// 				dmesg("%s: %v", tool, err)
			// 			}
			continue
		}

		switch base := filepath.Base(bin); {
		case base != tool:
			symlink := filepath.Join(dirTemp, base)
			if err := os.Symlink(self, symlink); err != nil {
				return errorf("%v", err)
			}

			// 			if dmesgs {
			// 				dmesg("symlink %s -> %s", symlink, self)
			// 			}
		default:
			symlink := filepath.Join(dirTemp, tool)
			if err := os.Symlink(self, symlink); err != nil {
				return errorf("%v", err)
			}

			// 			if dmesgs {
			// 				dmesg("symlink %s -> %s", symlink, self)
			// 			}
		}
		a = append(a, fmt.Sprintf("%s=%s", tool, bin))
	}
	setenv(execEnvVar, strings.Join(a, commaSep))
	// 	if dmesgs {
	// 		dmesg("exec.Command(%q, %q)", args[0], args[1:])
	// 	}
	cmd := exec.Command(args[0], args[1:]...)
	cmd.Stdout = t.stdout
	cmd.Stderr = t.stderr
	err = cmd.Run()
	// 	if dmesgs {
	// 		dmesg("exec.Command->%v", err)
	// 	}
	return err
}

type strSlice []string

func (s *strSlice) add(v ...string) { *s = append(*s, v...) }

func (t *Task) execed(routes string, cflags []string) (err error) {
	if dmesgs {
		dmesg(
			"==== task.execed t.goos=%s t.goarch=%s IsExecEnv()=%v CC=%s routes=%s\nt.args=%s",
			t.goos, t.goarch, IsExecEnv(), os.Getenv("CC"), routes, t.args,
		)
	}

	defer func() {
		if e := recover(); e != nil && err == nil {
			err = errorf("PANIC: %v\n%s", e, debug.Stack())
		}
	}()

	if len(t.args) == 0 {
		return errorf("internal error: len(t.args) == 0")
	}

	pairs := strings.Split(routes, commaSep)
	cmd := t.noExe(t.args[0])
	cmdBase := filepath.Base(cmd)
	if t.cpp == "" {
		for _, v := range []string{"CCGO_CC", "CCGO_GCC", "CCGO_CLANG"} {
			if s := os.Getenv(v); s != "" {
				t.cpp = s
				break
			}
		}
	}
	if t.cpp != "" {
		setenv("CCGO_CPP", t.cpp)
	}
	for _, v := range pairs {
		pair := strings.SplitN(v, "=", 2)
		tool, bin := pair[0], pair[1]
		binBase := filepath.Base(bin)
		switch tool {
		case "ar":
			if cmd == tool || cmdBase == tool || cmdBase == binBase {
				return t.ar(bin, os.Getenv("CCGO_AR"))
			}
		case "cc":
			if cmd == tool || cmdBase == tool || cmdBase == binBase {
				return t.cc(bin, os.Getenv("CCGO_CC"), cflags)
			}
		case "gcc":
			if cmd == tool || cmdBase == tool || cmdBase == binBase {
				return t.cc(bin, os.Getenv("CCGO_GCC"), cflags)
			}
		case "clang":
			if cmd == tool || cmdBase == tool || cmdBase == binBase {
				return t.cc(bin, os.Getenv("CCGO_CLANG"), cflags)
			}
		case "libtool":
			if cmd == tool || cmdBase == tool || cmdBase == binBase {
				return t.libtool(bin, os.Getenv("CCGO_LIBTOOL"), os.Getenv("CCGO_AR"))
			}
		case "ln":
			if cmd == tool || cmdBase == tool || cmdBase == binBase {
				return t.ln(bin, os.Getenv("CCGO_LN"))
			}
		case "mv":
			if cmd == tool || cmdBase == tool || cmdBase == binBase {
				return t.mv(bin, os.Getenv("CCGO_MV"))
			}
		case "rm":
			if cmd == tool || cmdBase == tool || cmdBase == binBase {
				return t.rm(bin, os.Getenv("CCGO_RM"))
			}
		default:
			// 			if dmesgs {
			// 				dmesg("FAIL")
			// 			}
			return errorf("internal error: route %q", pair)
		}
	}
	// 	if dmesgs {
	// 		dmesg("FAIL cmd=%s", cmd)
	// 	}
	return errorf("internal error: %q", cmd)
}

func (t *Task) noExe(s string) string {
	const tag = ".exe"
	if t.goos != "windows" || !strings.HasSuffix(s, tag) {
		return s
	}

	return s[:len(s)-len(tag)]
}

func (t *Task) libtool(execLibtool, hostLibtool, hostAR string) error {
	cmd := exec.Command(execLibtool, t.args[1:]...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		_ = err
		// if dmesgs {
		// 	dmesg("NOTE: %s returns %v", execLibtool, err.(*exec.ExitError).ExitCode())
		// }
	}
	set := opt.NewSet()
	var args strSlice
	var outfn string
	set.Arg("o", true, func(arg, val string) error {
		if !strings.HasSuffix(val, ".a") {
			return errorf("unexpected -o argument: %s", val)
		}

		outfn = t.goFile(val)
		return nil
	})
	if err := set.Parse(t.args[1:], func(arg string) error {
		if strings.HasPrefix(arg, "-") {
			// 			if dmesgs {
			// 				dmesg("", errorf("unexpected/unsupported option: %q", arg))
			// 			}
			return errorf("unexpected/unsupported option: %s", arg)
		}

		args.add(t.goFile(arg))
		return nil
	}); err != nil {
		return err
	}
	args2 := strSlice{"-cr", outfn}
	args2 = append(args2, args...)
	// 	if dmesgs {
	// 		dmesg("", args2)
	// 	}
	cmd = exec.Command(hostAR, args2...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		// 		if dmesgs {
		// 			dmesg("SKIP2: %s returns %v", hostAR, err.(*exec.ExitError).ExitCode())
		// 		}
		return err
	}

	// 	if dmesgs {
	// 		dmesg("OK %v", args2)
	// 	}
	return nil
}

func (t *Task) ln(execLN, hostLN string) error {
	cmd := exec.Command(execLN, t.args[1:]...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		_ = err
		// 		if dmesgs {
		// 			dmesg("NOTE: %s returns %v", execLN, err.(*exec.ExitError).ExitCode())
		// 		}
	}
	set := opt.NewSet()
	var args []string
	files := 0
	set.Opt("s", func(arg string) error { args = append(args, arg); return nil })
	set.Opt("sf", func(arg string) error { args = append(args, arg); return nil })
	set.Opt("fs", func(arg string) error { args = append(args, arg); return nil })
	if err := set.Parse(t.args[1:], func(arg string) error {
		if strings.HasPrefix(arg, "-") {
			// 			if dmesgs {
			// 				dmesg("", errorf("unexpected/unsupported option: %q", arg))
			// 			}
			return errorf("unexpected/unsupported option: %s", arg)
		}

		if arg := t.goFile(arg); arg != "" {
			args = append(args, t.goFile(arg))
			files++
		}
		return nil
	}); err != nil {
		return err
	}
	switch files {
	case 0:
		return nil
	case 2:
		if _, err := os.Stat(args[0]); err != nil {
			return nil
		}

		shell0(60*time.Second, true, hostLN, args...)
		return nil
	case 1:
		fallthrough
	default:
		return errorf("real LN=%q, faked args=%q args=%v files=%v", hostLN, t.args, args, files)
	}
}

func (t *Task) mv(execMV, hostMV string) error {
	cmd := exec.Command(execMV, t.args[1:]...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		// if dmesgs {
		// 	dmesg("SKIP: %s returns %v", execMV, err.(*exec.ExitError).ExitCode())
		// }
		return err
	}

	set := opt.NewSet()
	var args []string
	files := 0
	set.Opt("f", func(arg string) error { args = append(args, "-f"); return nil })
	if err := set.Parse(t.args[1:], func(arg string) error {
		if strings.HasPrefix(arg, "-") {
			// 			if dmesgs {
			// 				dmesg("", errorf("unexpected/unsupported option: %q", arg))
			// 			}
			return errorf("unexpected/unsupported option: %s", arg)
		}

		args = append(args, t.goFile(arg))
		files++
		return nil
	}); err != nil {
		return err
	}

	if files != 2 {
		return errorf("real MV=%q, faked args=%q", hostMV, t.args)
	}

	if _, err := os.Stat(args[0]); err != nil {
		return nil
	}

	shell0(60*time.Second, true, hostMV, args...)
	return nil
}

func (t *Task) rm(execRM, hostRM string) error {
	cmd := exec.Command(execRM, t.args[1:]...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		// 		if dmesgs {
		// 			dmesg("SKIP: %s returns %v", execRM, err.(*exec.ExitError).ExitCode())
		// 		}
		return err
	}

	rf := false
	set := opt.NewSet()
	set.Opt("r", func(arg string) error { return nil })
	set.Opt("f", func(arg string) error { return nil })
	set.Opt("rf", func(arg string) error { rf = true; return nil })
	set.Opt("fr", func(arg string) error { rf = true; return nil })
	return set.Parse(t.args[1:], func(arg string) error {
		if strings.HasPrefix(arg, "-") {
			// 			if dmesgs {
			// 				dmesg("", errorf("unexpected/unsupported option: %q", arg))
			// 			}
			return errorf("unexpected/unsupported option: %s", arg)
		}

		switch {
		case rf:
			// nop
		default:
			os.Remove(t.goFile(arg))
		}
		return nil
	})
}

func (t *Task) goFile(s string) string {
	switch filepath.Ext(s) {
	case ".go":
		return s
	case ".lo", ".o":
		return s + ".go"
	case ".a":
		return s + "go"
	default:
		return ""
	}
}

func (t *Task) cc(execCC, hostCC string, cflags []string) error {
	// 	if dmesgs {
	// 		dmesg("cc(%q, %q, %q)", execCC, hostCC, cflags)
	// 		dmesg("%s %v", execCC, t.args[1:])
	// 	}
	cmd := exec.Command(execCC, t.args[1:]...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		_ = err
		// if dmesgs {
		// 	dmesg("NOTE: %s returns %v", execCC, err.(*exec.ExitError).ExitCode())
		// }
	}

	optE := false
	args := append(strSlice{t.args[0]}, cflags...)
	set := opt.NewSet()
	ignore := 0
	set.Arg("-cpp", true, func(arg, val string) error { args.add(fmt.Sprintf("%s=%s", arg, val)); return nil })
	set.Arg("-goarch", true, func(arg, val string) error { args.add(fmt.Sprintf("%s=%s", arg, val)); return nil })
	set.Arg("-goos", true, func(arg, val string) error { args.add(fmt.Sprintf("%s=%s", arg, val)); return nil })
	set.Arg("-libc", false, func(arg, val string) error { args.add(fmt.Sprintf("%s=%s", arg, val)); return nil })
	set.Arg("D", true, func(arg, val string) error { args.add(arg + val); return nil })
	set.Arg("I", true, func(arg, val string) error { args.add(arg + val); return nil })
	set.Arg("L", true, func(arg, val string) error { args.add(arg + val); return nil })
	set.Arg("MD", true, func(arg, val string) error { return nil })
	set.Arg("MF", true, func(arg, val string) error { return nil })
	set.Arg("MT", true, func(arg, val string) error { return nil })
	set.Arg("O", true, func(arg, val string) error { args.add(arg + val); return nil })
	set.Arg("U", true, func(arg, val string) error { args.add(arg + val); return nil })
	set.Arg("arch", true, func(arg, val string) error { args.add(fmt.Sprintf("%s=%s", arg, val)); return nil })
	set.Arg("build-lines", true, func(arg, val string) error { args.add(fmt.Sprintf("%s=%s", arg, val)); return nil })
	set.Arg("compatibility_version", true, func(arg, val string) error { return nil })
	set.Arg("current_version", false, func(arg, val string) error { return nil })
	set.Arg("gz", true, func(arg, val string) error { args.add(fmt.Sprintf("%s=%s", arg, val)); return nil })
	set.Arg("idirafter", true, func(arg, val string) error { args.add(fmt.Sprintf("%s=%s", arg, val)); return nil })
	set.Arg("include", true, func(arg, val string) error { args.add(fmt.Sprintf("%s=%s", arg, val)); return nil })
	set.Arg("install_name", true, func(arg, val string) error { return nil })
	set.Arg("iquote", true, func(arg, val string) error { args.add(fmt.Sprintf("%s=%s", arg, val)); return nil })
	set.Arg("isystem", true, func(arg, val string) error { args.add(fmt.Sprintf("%s=%s", arg, val)); return nil })
	set.Arg("l", true, func(arg, val string) error { args.add(arg + val); return nil })
	set.Arg("march", true, func(arg, val string) error { args.add(fmt.Sprintf("%s=%s", arg, val)); return nil })
	set.Arg("mtune", true, func(arg, val string) error { args.add(fmt.Sprintf("%s=%s", arg, val)); return nil })
	set.Arg("o", true, func(arg, val string) error { args.add(arg, val+".go"); return nil })
	set.Arg("sectcreate", false, func(arg, val string) error { ignore = 2; return nil })
	set.Arg("std", true, func(arg, val string) error { args.add(fmt.Sprintf("%s=%s", arg, val)); return nil })
	set.Opt("-version", func(arg string) error { args.add(arg); return nil })
	set.Opt("E", func(arg string) error { optE = true; return nil })
	set.Opt("MMD", func(arg string) error { return nil })
	set.Opt("Qunused-arguments", func(arg string) error { args.add(arg); return nil })
	set.Opt("ansi", func(arg string) error { args.add(arg); return nil })
	set.Opt("c", func(arg string) error { args.add(arg); return nil })
	set.Opt("dumpmachine", func(arg string) error { args.add(arg); return nil })
	set.Opt("dynamiclib", func(arg string) error { return nil })
	set.Opt("emit-func-aliases", func(arg string) error { args.add(arg); return nil })
	set.Opt("eval-all-macros", func(arg string) error { args.add(arg); return nil })
	set.Opt("ffreestanding", func(arg string) error { args.add(arg); return nil })
	set.Opt("fno-builtin", func(arg string) error { args.add(arg); return nil })
	set.Opt("g", func(arg string) error { return nil })
	set.Opt("headerpad_max_install_names", func(arg string) error { args.add(arg); return nil })
	set.Opt("ignore-link-errors", func(arg string) error { args.add(arg); return nil })
	set.Opt("m32", func(arg string) error { args.add(arg); return nil })
	set.Opt("m64", func(arg string) error { args.add(arg); return nil })
	set.Opt("mconsole", func(arg string) error { args.add(arg); return nil })
	set.Opt("mdynamic-no-pic", func(arg string) error { return nil })
	set.Opt("mlong-double-64", func(arg string) error { args.add(arg); return nil })
	set.Opt("mno-3dnow", func(arg string) error { args.add(arg); return nil })
	set.Opt("mno-abm", func(arg string) error { args.add(arg); return nil })
	set.Opt("mno-aes", func(arg string) error { args.add(arg); return nil })
	set.Opt("mno-avx", func(arg string) error { args.add(arg); return nil })
	set.Opt("mno-avx2", func(arg string) error { args.add(arg); return nil })
	set.Opt("mno-avx512cd", func(arg string) error { args.add(arg); return nil })
	set.Opt("mno-avx512er", func(arg string) error { args.add(arg); return nil })
	set.Opt("mno-avx512f", func(arg string) error { args.add(arg); return nil })
	set.Opt("mno-avx512pf", func(arg string) error { args.add(arg); return nil })
	set.Opt("mno-bmi", func(arg string) error { args.add(arg); return nil })
	set.Opt("mno-bmi2", func(arg string) error { args.add(arg); return nil })
	set.Opt("mno-f16c", func(arg string) error { args.add(arg); return nil })
	set.Opt("mno-fma", func(arg string) error { args.add(arg); return nil })
	set.Opt("mno-fma4", func(arg string) error { args.add(arg); return nil })
	set.Opt("mno-fsgsbase", func(arg string) error { args.add(arg); return nil })
	set.Opt("mno-lwp", func(arg string) error { args.add(arg); return nil })
	set.Opt("mno-lzcnt", func(arg string) error { args.add(arg); return nil })
	set.Opt("mno-mmx", func(arg string) error { args.add(arg); return nil })
	set.Opt("mno-pclmul", func(arg string) error { args.add(arg); return nil })
	set.Opt("mno-popcnt", func(arg string) error { args.add(arg); return nil })
	set.Opt("mno-prefetchwt1", func(arg string) error { args.add(arg); return nil })
	set.Opt("mno-rdrnd", func(arg string) error { args.add(arg); return nil })
	set.Opt("mno-sha", func(arg string) error { args.add(arg); return nil })
	set.Opt("mno-sse", func(arg string) error { args.add(arg); return nil })
	set.Opt("mno-sse2", func(arg string) error { args.add(arg); return nil })
	set.Opt("mno-sse3", func(arg string) error { args.add(arg); return nil })
	set.Opt("mno-sse4", func(arg string) error { args.add(arg); return nil })
	set.Opt("mno-sse4.1", func(arg string) error { args.add(arg); return nil })
	set.Opt("mno-sse4.2", func(arg string) error { args.add(arg); return nil })
	set.Opt("mno-sse4a", func(arg string) error { args.add(arg); return nil })
	set.Opt("mno-ssse3", func(arg string) error { args.add(arg); return nil })
	set.Opt("mno-tbm", func(arg string) error { args.add(arg); return nil })
	set.Opt("mno-xop", func(arg string) error { args.add(arg); return nil })
	set.Opt("municode", func(arg string) error { args.add(arg); return nil })
	set.Opt("mwindows", func(arg string) error { args.add(arg); return nil })
	set.Opt("nostdinc", func(arg string) error { args.add(arg); return nil })
	set.Opt("nostdlib", func(arg string) error { args.add(arg); return nil })
	set.Opt("pedantic", func(arg string) error { args.add(arg); return nil })
	set.Opt("pedantic-errors", func(arg string) error { args.add(arg); return nil })
	set.Opt("pipe", func(arg string) error { return nil })
	set.Opt("pthread", func(arg string) error { args.add(arg); return nil })
	set.Opt("rdynamic", func(arg string) error { args.add(arg); return nil })
	set.Opt("s", func(arg string) error { args.add(arg); return nil })
	set.Opt("shared", func(arg string) error { args.add(arg); return nil })
	set.Opt("static", func(arg string) error { args.add(arg); return nil })
	set.Opt("static-libgcc", func(arg string) error { args.add(arg); return nil })
	set.Opt("v", func(arg string) error { args.add(arg); return nil })
	set.Opt("w", func(arg string) error { args.add(arg); return nil })
	files := 0
	var postfix strSlice
	if err := set.Parse(t.args[1:], func(arg string) error {
		if ignore > 0 {
			ignore--
			return nil
		}

		if optE {
			return nil
		}

		if strings.HasPrefix(arg, "-f") {
			return nil
		}

		if strings.HasPrefix(arg, "-W") { // eg. -Wa,--noexecstack
			return nil
		}

		if strings.HasPrefix(arg, "-") {
			// 			if dmesgs {
			// 				dmesg("", errorf("unexpected/unsupported option: %q", arg))
			// 			}
			return errorf("unexpected/unsupported option: %s", arg)
		}

		switch filepath.Ext(arg) {
		case ".c", ".h":
			args.add(arg)
			files++
			return nil
		case ".s", ".S":
			return nil
		case ".o", ".lo":
			nm := arg + ".go"
			nm2 := ""
			if strings.HasSuffix(arg, ".lo") {
				nm2 = arg[:len(arg)-len(".lo")] + ".o.go"
			}
			switch {
			case t.fs != nil:
				if _, err := t.fs.Open(nm); err != nil {
					nm = nm2
					if _, err := t.fs.Open(nm); err != nil {
						return nil
					}
				}
			default:
				if _, err := os.Stat(nm); err != nil {
					nm = nm2
					if _, err := os.Stat(nm); err != nil {
						return nil
					}
				}
			}
			args.add(nm)
			files++
			return nil
		case ".a", ".def":
			args.add(arg)
			files++
			return nil
		case ".so":
			bn := filepath.Base(arg)
			bn = bn[:len(bn)-len(".so")]
			if !strings.HasPrefix(bn, "lib") {
				break
			}

			postfix.add(fmt.Sprintf("-l%s", bn[len("lib"):]))
			return nil
		case ".dylib":
			bn := filepath.Base(arg)
			bn = bn[:len(bn)-len(".dylib")]
			if !strings.HasPrefix(bn, "lib") {
				break
			}

			postfix.add(fmt.Sprintf("-l%s", bn[len("lib"):]))
			return nil
		}

		return errorf("unexpected/unsupported argument: %s", arg)
	}); err != nil {
		return err
	}
	args = append(args, postfix...)

	if files == 0 || optE {
		return nil
	}

	// 	if dmesgs {
	// 		dmesg("DBG args=%v", args)
	// 	}
	t = NewTask(t.goos, t.goarch, args, t.stdout, t.stderr, t.fs)
	t.isExeced = true
	return t.main()
}

func (t *Task) ar(execAR, hostAR string) (err error) {
	// if dmesgs {
	// 	dmesg("execAR=%s hostAR=%s t.args=%v", execAR, hostAR, t.args)
	// 	defer func() {
	// 		if err != nil {
	// 			dmesg("t.ar() -> err=%v", err)
	// 		}
	// 	}()
	// }
	cmd := exec.Command(execAR, t.args[1:]...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	// if dmesgs {
	// 	wd, _ := os.Getwd()
	// 	wd, _ = filepath.Abs(wd)
	// 	dmesg("WD=%s", wd)
	// 	for _, v := range t.args[3:] {
	// 		switch fi, err := os.Stat(v); {
	// 		case err != nil:
	// 			dmesg("%v NOT FOUND (A) err=%v", v, err)
	// 		default:
	// 			dmesg("%v FOUND (A) sz=%v", v, fi.Size())
	// 		}
	// 	}
	// }
	if err := cmd.Run(); err != nil {
		if dmesgs {
			dmesg("SKIP: %s returns %v", execAR, err.(*exec.ExitError).ExitCode())
		}
		return err
	}

	set := opt.NewSet()
	var argN, members int
	basenames := map[string]string{} // base: path
	args := strSlice{t.args[0]}
	var out string
	if err := set.Parse(t.args[1:], func(arg string) error {
		if strings.HasPrefix(arg, "-") {
			// if dmesgs {
			// 	dmesg("", errorf("unexpected/unsupported option: %q", arg))
			// }
			return errorf("unexpected/unsupported option: %s", arg)
		}

		argN++
		switch argN {
		case 1: // keyletters
			for _, c := range arg {
				switch sc := string(c); sc {
				case
					"c", // create the archive
					"q", // quick append
					"r", // insert member
					"u", // update
					"x": // extract

					out += sc
				case
					"s", // add index
					"t": // display content

					// nop
				default:
					return errorf("TODO #%d: %q: faked args=%q", argN, arg, t.args)
				}
			}
			args.add(out)
			return nil
		case 2: // archive name
			if !strings.HasSuffix(arg, ".a") {
				return errorf("TODO #%d: %q: faked args=%q", argN, arg, t.args)
			}

			args.add(arg + "go") // archive.ago
			return nil
		default:
			switch filepath.Ext(arg) {
			case ".lo", ".o":
				nm := arg + ".go"
				if _, err := os.Stat(nm); err == nil {
					bn := filepath.Base(nm)
					if ex, ok := basenames[bn]; ok {
						return errorf("duplicate basename %s: %s", ex, nm)
					}

					basenames[bn] = arg
					members++
					args.add(nm)
				} // else if dmesgs {
				// dmesg("arg=%v .go version NOT FOUND: %v", arg, nm)
				// }
				return nil
			case ".def":
				return nil
			default:
				return errorf("TODO #%d: %q: faked args=%q", argN, arg, t.args)
			}
		}

		return errorf("unexpected/unsupported argument: %s", arg)
	}); err != nil {
		return err
	}

	if out == "" {
		return nil
	}

	// if dmesgs {
	// 	dmesg("hostAR=%s args[1:]=%v", hostAR, args[1:])
	// 	wd, _ := os.Getwd()
	// 	wd, _ = filepath.Abs(wd)
	// 	dmesg("WD=%s", wd)
	// 	for _, v := range args[3:] {
	// 		switch fi, err := os.Stat(v); {
	// 		case err != nil:
	// 			dmesg("%v NOT FOUND (B) err=%v", v, err)
	// 		default:
	// 			dmesg("%v FOUND (B) sz=%v", v, fi.Size())
	// 		}
	// 	}
	// }
	cmd = exec.Command(hostAR, []string(args[1:])...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		// if dmesgs {
		// 	dmesg("SKIP2: %s returns %v", hostAR, err.(*exec.ExitError).ExitCode())
		// }
		return err
	}

	return nil
}
