// Copyright 2026 The Xorm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package utils

import "testing"

func TestIndexNameAndSeqName(t *testing.T) {
	if name := IndexName("user", "email"); name != "IDX_user_email" {
		t.Fatalf("unexpected index name: %s", name)
	}
	if name := SeqName("user"); name != "SEQ_USER" {
		t.Fatalf("unexpected sequence name: %s", name)
	}
}
