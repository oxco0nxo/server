// Copyright 2026 The Xorm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package utils

import (
	"reflect"
	"testing"
)

func TestNewCreatesValues(t *testing.T) {
	sliceType := reflect.TypeOf([]string{})
	sliceValue := New(sliceType, 2, 3)
	if sliceValue.Kind() != reflect.Ptr || sliceValue.Elem().Len() != 2 {
		t.Fatalf("unexpected slice value: %#v", sliceValue)
	}
	if sliceValue.Elem().Cap() != 3 {
		t.Fatalf("expected slice cap 3, got %d", sliceValue.Elem().Cap())
	}

	mapType := reflect.TypeOf(map[string]int{})
	mapValue := New(mapType, 0, 4)
	if mapValue.Kind() != reflect.Ptr || mapValue.Elem().Len() != 0 {
		t.Fatalf("unexpected map value: %#v", mapValue)
	}

	intType := reflect.TypeOf(0)
	intValue := New(intType, 0, 0)
	if intValue.Kind() != reflect.Ptr || intValue.Elem().Int() != 0 {
		t.Fatalf("unexpected int value: %#v", intValue)
	}
}
