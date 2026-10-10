// Copyright 2026 The Xorm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package utils

import "testing"

func TestIndexAndSplitNoCase(t *testing.T) {
	if idx := IndexNoCase("HelloWorld", "world"); idx != 5 {
		t.Fatalf("expected index 5, got %d", idx)
	}
	if idx := IndexNoCase("HelloWorld", "missing"); idx != -1 {
		t.Fatalf("expected index -1, got %d", idx)
	}

	parts := SplitNoCase("Hello-World", "-world")
	if len(parts) != 2 || parts[0] != "Hello" || parts[1] != "" {
		t.Fatalf("unexpected split result: %#v", parts)
	}

	parts = SplitNoCase("NoMatchHere", "-")
	if len(parts) != 1 || parts[0] != "NoMatchHere" {
		t.Fatalf("unexpected split result: %#v", parts)
	}
}

func TestSplitNNoCase(t *testing.T) {
	parts := SplitNNoCase("A-B-C", "-b", 2)
	if len(parts) != 2 || parts[0] != "A" || parts[1] != "-C" {
		t.Fatalf("unexpected splitN result: %#v", parts)
	}

	parts = SplitNNoCase("Single", "-", 2)
	if len(parts) != 1 || parts[0] != "Single" {
		t.Fatalf("unexpected splitN result: %#v", parts)
	}
}
