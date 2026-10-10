// Copyright 2026 The Xorm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package core

import (
	"context"
	"database/sql"
	"reflect"
	"testing"

	"xorm.io/xorm/log"
)

type testLogger struct {
	showSQL bool
}

func (t *testLogger) BeforeSQL(log.LogContext) {}
func (t *testLogger) AfterSQL(log.LogContext)  {}
func (t *testLogger) Debugf(string, ...any)    {}
func (t *testLogger) Errorf(string, ...any)    {}
func (t *testLogger) Infof(string, ...any)     {}
func (t *testLogger) Warnf(string, ...any)     {}
func (t *testLogger) Level() log.LogLevel      { return log.LOG_INFO }
func (t *testLogger) SetLevel(log.LogLevel)    {}
func (t *testLogger) ShowSQL(show ...bool) {
	if len(show) == 0 {
		t.showSQL = true
		return
	}
	t.showSQL = show[0]
}
func (t *testLogger) IsShowSQL() bool { return t.showSQL }

func TestMapToSlice(t *testing.T) {
	query := "SELECT * FROM t WHERE id=?id AND name=?name"
	params := map[string]any{"id": 3, "name": "bob"}
	sqlText, args, err := MapToSlice(query, &params)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if sqlText != "SELECT * FROM t WHERE id=? AND name=?" {
		t.Fatalf("unexpected sql: %s", sqlText)
	}
	if len(args) != 2 {
		t.Fatalf("expected 2 args, got %d", len(args))
	}

	_, _, err = MapToSlice(query, params)
	if err != ErrNoMapPointer {
		t.Fatalf("expected ErrNoMapPointer, got %v", err)
	}

	params = map[string]any{"id": 3}
	_, _, err = MapToSlice(query, &params)
	if err == nil {
		t.Fatalf("expected error for missing key")
	}
}

func TestStructToSlice(t *testing.T) {
	type payload struct {
		ID   int
		Name sql.NullString
	}
	value := payload{ID: 9, Name: sql.NullString{String: "alice", Valid: true}}
	query := "SELECT * FROM t WHERE id=?ID AND name=?Name"
	sqlText, args, err := StructToSlice(query, &value)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if sqlText != "SELECT * FROM t WHERE id=? AND name=?" {
		t.Fatalf("unexpected sql: %s", sqlText)
	}
	if len(args) != 2 || args[0] != 9 || args[1] != "alice" {
		t.Fatalf("unexpected args: %#v", args)
	}

	_, _, err = StructToSlice(query, value)
	if err != ErrNoStructPointer {
		t.Fatalf("expected ErrNoStructPointer, got %v", err)
	}
}

func TestNeedLogSQL(t *testing.T) {
	logger := &testLogger{showSQL: false}
	db := &DB{Logger: logger}
	ctx := context.Background()
	if db.NeedLogSQL(ctx) {
		t.Fatalf("expected NeedLogSQL false when logger disabled")
	}

	logger.showSQL = true
	if !db.NeedLogSQL(ctx) {
		t.Fatalf("expected NeedLogSQL true when logger enabled")
	}

	ctx = context.WithValue(ctx, log.SessionShowSQLKey, false)
	if db.NeedLogSQL(ctx) {
		t.Fatalf("expected NeedLogSQL false when context disables")
	}
}

func TestReflectNew(t *testing.T) {
	db := &DB{reflectCache: make(map[reflect.Type]*cacheStruct)}
	val := db.reflectNew(reflect.TypeOf(int(0)))
	if val.Kind() != reflect.Ptr || val.Elem().Int() != 0 {
		t.Fatalf("unexpected reflect value: %#v", val)
	}
}
