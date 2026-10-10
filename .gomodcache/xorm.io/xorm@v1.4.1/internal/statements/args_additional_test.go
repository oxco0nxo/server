// Copyright 2026 The Xorm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package statements

import (
	"strings"
	"testing"

	"xorm.io/builder"
	"xorm.io/xorm/dialects"
	"xorm.io/xorm/schemas"
)

func newTestStatement(t *testing.T, dbType schemas.DBType) *Statement {
	t.Helper()
	dialect := dialects.QueryDialect(dbType)
	if dialect == nil {
		t.Fatalf("dialect not found for %s", dbType)
	}
	if err := dialect.Init(&dialects.URI{DBType: dbType, DBName: "test"}); err != nil {
		t.Fatalf("failed to init dialect: %v", err)
	}
	return &Statement{dialect: dialect}
}

func TestWriteArgBuilder(t *testing.T) {
	statement := newTestStatement(t, schemas.SQLITE)
	writer := builder.NewWriter()
	b := builder.SQLite().Select("id").From("users")
	if err := statement.WriteArg(writer, b); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	sqlText := writer.String()
	if !strings.HasPrefix(sqlText, "(") || !strings.HasSuffix(sqlText, ")") {
		t.Fatalf("expected parentheses around builder sql, got %s", sqlText)
	}
}

func TestWriteArgDateTime(t *testing.T) {
	statement := newTestStatement(t, schemas.ORACLE)
	writer := builder.NewWriter()
	arg := &DateTimeString{Layout: "2006-01-02", Str: "2024-01-01"}
	if err := statement.WriteArg(writer, arg); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(writer.String(), "TO_DATE") {
		t.Fatalf("expected TO_DATE for oracle, got %s", writer.String())
	}
	if len(writer.Args()) != 1 || writer.Args()[0] != arg {
		t.Fatalf("unexpected args: %#v", writer.Args())
	}

	sqliteStmt := newTestStatement(t, schemas.SQLITE)
	writer = builder.NewWriter()
	if err := sqliteStmt.WriteArg(writer, arg); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if writer.String() != "?" {
		t.Fatalf("expected placeholder for sqlite, got %s", writer.String())
	}
}

func TestWriteArgBoolMSSQL(t *testing.T) {
	statement := newTestStatement(t, schemas.MSSQL)
	writer := builder.NewWriter()
	if err := statement.WriteArg(writer, true); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if writer.String() != "?" {
		t.Fatalf("expected placeholder, got %s", writer.String())
	}
	if len(writer.Args()) != 1 || writer.Args()[0] != 1 {
		t.Fatalf("expected arg 1, got %#v", writer.Args())
	}
}

func TestWriteArgs(t *testing.T) {
	statement := newTestStatement(t, schemas.SQLITE)
	writer := builder.NewWriter()
	if err := statement.WriteArgs(writer, []any{1, "a"}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if writer.String() != "?,?" {
		t.Fatalf("expected placeholders, got %s", writer.String())
	}
	if len(writer.Args()) != 2 {
		t.Fatalf("expected 2 args, got %#v", writer.Args())
	}
}
