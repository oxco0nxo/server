// Copyright 2026 The Xorm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package utils

import "testing"

func TestSliceEqAndIndexSlice(t *testing.T) {
	left := []string{"b", "a", "c"}
	right := []string{"c", "b", "a"}
	if !SliceEq(append([]string{}, left...), append([]string{}, right...)) {
		t.Fatalf("expected slices to be equal")
	}
	if SliceEq([]string{"a"}, []string{"a", "b"}) {
		t.Fatalf("expected slices with different lengths to be unequal")
	}

	values := []string{"x", "y", "z"}
	if idx := IndexSlice(values, "y"); idx != 1 {
		t.Fatalf("expected index 1, got %d", idx)
	}
	if idx := IndexSlice(values, "missing"); idx != -1 {
		t.Fatalf("expected -1 for missing value, got %d", idx)
	}
}
