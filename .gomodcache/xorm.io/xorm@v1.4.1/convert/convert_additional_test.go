// Copyright 2026 The Xorm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package convert

import (
	"database/sql"
	"testing"
)

func TestAsBool(t *testing.T) {
	val := true
	nb := &sql.NullBool{Bool: true, Valid: true}
	ni32 := &sql.NullInt32{Int32: 1, Valid: true}
	ni64 := &sql.NullInt64{Int64: 2, Valid: true}

	cases := []struct {
		input    any
		expected bool
	}{
		{true, true},
		{&val, true},
		{nb, true},
		{int64(1), true},
		{int(0), false},
		{int8(1), true},
		{int16(0), false},
		{int32(2), true},
		{[]byte{}, false},
		{[]byte{0x00}, false},
		{[]byte{0x01}, true},
		{[]byte("true"), true},
		{"false", false},
		{ni32, true},
		{ni64, true},
	}

	for _, item := range cases {
		got, err := AsBool(item.input)
		if err != nil {
			t.Fatalf("unexpected error for %T: %v", item.input, err)
		}
		if got != item.expected {
			t.Fatalf("expected %v for %T, got %v", item.expected, item.input, got)
		}
	}

	if _, err := AsBool(struct{}{}); err == nil {
		t.Fatalf("expected error for unsupported type")
	}
}

func TestAsInt64AndUint64(t *testing.T) {
	var i64 int64 = 42
	var ui64 uint64 = 7
	var ui64Ptr *uint64
	ni32 := &sql.NullInt32{Int32: 12, Valid: true}
	ni64 := &sql.NullInt64{Int64: 24, Valid: true}
	ns := &sql.NullString{String: "33", Valid: true}

	if got, err := AsInt64(&i64); err != nil || got != 42 {
		t.Fatalf("expected 42, got %d, err=%v", got, err)
	}
	if got, err := AsInt64(ui64); err != nil || got != 7 {
		t.Fatalf("expected 7, got %d, err=%v", got, err)
	}
	if got, err := AsInt64(ui64Ptr); err != nil || got != 0 {
		t.Fatalf("expected 0 for nil pointer, got %d, err=%v", got, err)
	}
	if got, err := AsInt64([]byte("9")); err != nil || got != 9 {
		t.Fatalf("expected 9, got %d, err=%v", got, err)
	}
	if got, err := AsInt64(ns); err != nil || got != 33 {
		t.Fatalf("expected 33, got %d, err=%v", got, err)
	}
	if got, err := AsInt64(ni32); err != nil || got != 12 {
		t.Fatalf("expected 12, got %d, err=%v", got, err)
	}
	if got, err := AsInt64(ni64); err != nil || got != 24 {
		t.Fatalf("expected 24, got %d, err=%v", got, err)
	}

	type myInt int64
	if got, err := AsInt64(myInt(5)); err != nil || got != 5 {
		t.Fatalf("expected 5, got %d, err=%v", got, err)
	}
	if _, err := AsInt64(struct{}{}); err == nil {
		t.Fatalf("expected error for unsupported int64 type")
	}

	if got, err := AsUint64(int64(3)); err != nil || got != 3 {
		t.Fatalf("expected 3, got %d, err=%v", got, err)
	}
	if got, err := AsUint64([]byte("11")); err != nil || got != 11 {
		t.Fatalf("expected 11, got %d, err=%v", got, err)
	}
	if got, err := AsUint64(ns); err != nil || got != 33 {
		t.Fatalf("expected 33, got %d, err=%v", got, err)
	}
	if got, err := AsUint64(ni32); err != nil || got != 12 {
		t.Fatalf("expected 12, got %d, err=%v", got, err)
	}
	if got, err := AsUint64(ni64); err != nil || got != 24 {
		t.Fatalf("expected 24, got %d, err=%v", got, err)
	}

	type myUint uint64
	if got, err := AsUint64(myUint(6)); err != nil || got != 6 {
		t.Fatalf("expected 6, got %d, err=%v", got, err)
	}
	if _, err := AsUint64(struct{}{}); err == nil {
		t.Fatalf("expected error for unsupported uint64 type")
	}
}

func TestNullUint64AndUint32(t *testing.T) {
	var u64 NullUint64
	if err := u64.Scan(nil); err != nil {
		t.Fatalf("unexpected error scanning nil: %v", err)
	}
	if u64.Valid {
		t.Fatalf("expected invalid after nil scan")
	}
	if val, err := u64.Value(); err != nil || val != nil {
		t.Fatalf("expected nil value, got %v, err=%v", val, err)
	}

	if err := u64.Scan("18"); err != nil {
		t.Fatalf("unexpected error scanning string: %v", err)
	}
	if !u64.Valid || u64.Uint64 != 18 {
		t.Fatalf("expected valid 18, got %v", u64.Uint64)
	}
	if val, err := u64.Value(); err != nil || val != uint64(18) {
		t.Fatalf("expected 18 value, got %v, err=%v", val, err)
	}

	var u32 NullUint32
	if err := u32.Scan(nil); err != nil {
		t.Fatalf("unexpected error scanning nil: %v", err)
	}
	if u32.Valid {
		t.Fatalf("expected invalid after nil scan")
	}
	if val, err := u32.Value(); err != nil || val != nil {
		t.Fatalf("expected nil value, got %v, err=%v", val, err)
	}

	if err := u32.Scan(int64(5)); err != nil {
		t.Fatalf("unexpected error scanning int64: %v", err)
	}
	if !u32.Valid || u32.Uint32 != 5 {
		t.Fatalf("expected valid 5, got %v", u32.Uint32)
	}
	if val, err := u32.Value(); err != nil || val != int64(5) {
		t.Fatalf("expected 5 value, got %v, err=%v", val, err)
	}
}
