// Copyright 2021 The Gc Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

//go:generate stringer -output stringer.go -linecomment -type=Ch,Kind,guard

package gc // import "modernc.org/gc/v2"

// ParseSourceFileConfig configures ParseSourceFile.
type ParseSourceFileConfig struct {
	// Accept, if non nil, is called once the package clause and imports are
	// parsed. If Accept return a non-nil error the parsing stops and the error is
	// returned.  Passing nil Accept is the same as passing a function that always
	// returns nil
	Accept       func(*SourceFile) error
	packageScope *Scope

	AllErrors bool
}

// ParseSourceFile parses buf and returns a *SourceFile or an error, if any.
// Positions are reported as if buf is coming from a file named name. The
// buffer becomes owned by the *SourceFile and must not be modified after
// calling ParseSourceFile. The same cfg argument must be used for all source
// files within a package.  Distinct, new instances of the cfg arguments must
// be used for distinct packages.
func ParseSourceFile(cfg *ParseSourceFileConfig, name string, buf []byte) (r *SourceFile, err error) {
	if cfg.packageScope == nil {
		cfg.packageScope = &Scope{Parent: &universe}
	}
	s, err := NewScanner(name, buf)
	if err != nil {
		return nil, err
	}

	p := newParser(cfg, s)
	switch p.ch() {
	//       SourceFile
	case PACKAGE:
		r = &SourceFile{Scope: &Scope{}, packageScope: cfg.packageScope, PackageClause: &PackageClause{Package: p.must(PACKAGE), PackageName: p.must(IDENTIFIER), Semicolon: p.must(';')}, ImportDecls: p.importDecls()}
	default:
		p.err(errorf("TODO %v", p.s.Tok.Ch.str()))
		p.shift()
	}
	if err := p.s.errs.Err(); err != nil {
		return nil, err
	}

	if cfg.Accept != nil {
		if err = cfg.Accept(r); err != nil {
			return nil, err
		}
	}

	r.TopLevelDecls = p.topLevelDecls()
	if err := p.Err(); err != nil {
		return nil, err
	}

	switch p.ch() {
	//              eof
	case EOF:
		r.EOF = p.shift()
	default:
		p.err(errorf("TODO %v", p.s.Tok.Ch.str()))
		return nil, p.Err()
	}

	return r, nil
}

// PackageChecker provides the resolution API for (*Package).Check.
type PackageChecker interface {
	// PackageLoader returns a package by its import path or an error, if any. The
	// type checker never calls PackageLoader for  certain packages.
	PackageLoader(pkg *Package, src *SourceFile, importPath string) (*Package, error)
	// SymbolResolver returns the node bound to 'ident' within package 'pkg', using
	// currentScope and fileScope or an error, if any. The type checker never calls
	// SymbolResolver for certain identifiers of some packages.
	SymbolResolver(currentScope, fileScope *Scope, pkg *Package, ident Token) (Node, error)
	// CheckFunctions reports whether Check should type check function/method
	// bodies.
	CheckFunctions() bool
	// GOARCH reports the target architecture, it returns the same values as runtime.GOARCH.
	GOARCH() string
}
