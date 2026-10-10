// Copyright 2026 The Xorm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package schemas

import "testing"

func TestUnsignedDecimalIsNumeric(t *testing.T) {
	st := SQLType{Name: UnsignedDecimal}
	if !st.IsNumeric() {
		t.Fatalf("expected %s to be numeric", UnsignedDecimal)
	}
}
