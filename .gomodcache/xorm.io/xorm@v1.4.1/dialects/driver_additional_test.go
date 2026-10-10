// Copyright 2026 The Xorm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package dialects

import (
	"database/sql"
	"testing"

	"xorm.io/xorm/core"
)

type mockDriver struct{}

func (mockDriver) Parse(string, string) (*URI, error) {
	return &URI{DBType: "sqlite3"}, nil
}

func (mockDriver) Features() *DriverFeatures {
	return &DriverFeatures{}
}

func (mockDriver) GenScanResult(string) (any, error) {
	return nil, nil
}

func (mockDriver) Scan(*ScanContext, *core.Rows, []*sql.ColumnType, ...any) error {
	return nil
}

func TestRegisterDriverAndOpenDialect(t *testing.T) {
	startSize := RegisteredDriverSize()
	RegisterDriver("test-driver", mockDriver{})
	if got := RegisteredDriverSize(); got != startSize+1 {
		t.Fatalf("expected driver size %d, got %d", startSize+1, got)
	}
	if driver := QueryDriver("test-driver"); driver == nil {
		t.Fatalf("expected driver to be registered")
	}

	if _, err := OpenDialect("test-driver", ""); err != nil {
		t.Fatalf("unexpected error opening dialect: %v", err)
	}

	if _, err := OpenDialect("unknown-driver", ""); err == nil {
		t.Fatalf("expected error for unknown driver")
	}
}

func TestRegisterDriverPanics(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Fatalf("expected panic when registering nil driver")
		}
	}()
	RegisterDriver("nil-driver", nil)
}

func TestRegisterDriverDuplicatePanics(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Fatalf("expected panic when registering duplicate driver")
		}
	}()
	RegisterDriver("dup-driver", mockDriver{})
	RegisterDriver("dup-driver", mockDriver{})
}
