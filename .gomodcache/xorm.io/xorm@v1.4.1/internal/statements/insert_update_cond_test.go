// Copyright 2026 The Xorm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package statements

import (
	"reflect"
	"strings"
	"testing"
	"time"

	"xorm.io/builder"
	"xorm.io/xorm/caches"
	"xorm.io/xorm/dialects"
	"xorm.io/xorm/names"
	"xorm.io/xorm/schemas"
	"xorm.io/xorm/tags"
)

func newStatementWithTable(t *testing.T, dbType schemas.DBType) *Statement {
	t.Helper()
	dialect := dialects.QueryDialect(dbType)
	if dialect == nil {
		t.Fatalf("dialect not found for %s", dbType)
	}
	if err := dialect.Init(&dialects.URI{DBType: dbType, DBName: "test"}); err != nil {
		t.Fatalf("failed to init dialect: %v", err)
	}
	parser := tags.NewParser("xorm", dialect, names.SnakeMapper{}, names.SnakeMapper{}, caches.NewManager())
	statement := NewStatement(dialect, parser, time.Local)
	table := schemas.NewTable("users", nil)
	idCol := schemas.NewColumn("id", "", schemas.SQLType{Name: "INTEGER"}, 0, 0, true)
	idCol.IsAutoIncrement = true
	table.AddColumn(idCol)
	nameCol := schemas.NewColumn("name", "", schemas.SQLType{Name: "TEXT"}, 0, 0, true)
	table.AddColumn(nameCol)
	statement.RefTable = table
	statement.SetTableName("users")
	return statement
}

func TestGenInsertSQLDefaultValues(t *testing.T) {
	statement := newStatementWithTable(t, schemas.SQLITE)
	sqlText, args, err := statement.GenInsertSQL(nil, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if sqlText != "INSERT INTO `users` DEFAULT VALUES" {
		t.Fatalf("unexpected sql: %s", sqlText)
	}
	if len(args) != 0 {
		t.Fatalf("expected no args, got %#v", args)
	}
}

func TestGenInsertSQLPostgresReturning(t *testing.T) {
	statement := newStatementWithTable(t, schemas.POSTGRES)
	sqlText, _, err := statement.GenInsertSQL([]string{"name"}, []any{"bob"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(sqlText, "RETURNING \"id\"") {
		t.Fatalf("expected returning clause, got %s", sqlText)
	}
}

func TestGenInsertSQLOracleSequence(t *testing.T) {
	statement := newStatementWithTable(t, schemas.ORACLE)
	sqlText, _, err := statement.GenInsertSQL([]string{"name"}, []any{"bob"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(sqlText, "SEQ_USERS.nextval") {
		t.Fatalf("expected oracle sequence, got %s", sqlText)
	}
}

func TestGenInsertSQLWithConds(t *testing.T) {
	statement := newStatementWithTable(t, schemas.SQLITE)
	statement.Where("id>?", 1)
	sqlText, args, err := statement.GenInsertSQL([]string{"name"}, []any{"bob"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(sqlText, "SELECT ?") || !strings.Contains(sqlText, "FROM `users`") {
		t.Fatalf("unexpected sql: %s", sqlText)
	}
	if !strings.Contains(sqlText, "WHERE") || !strings.Contains(sqlText, "id>?") {
		t.Fatalf("unexpected where clause: %s", sqlText)
	}
	if len(args) != 2 || args[0] != "bob" || args[1] != 1 {
		t.Fatalf("unexpected args: %#v", args)
	}
}

func TestGenInsertMapSQL(t *testing.T) {
	statement := newStatementWithTable(t, schemas.SQLITE)
	sqlText, args, err := statement.GenInsertMapSQL([]string{"name"}, []any{"bob"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if sqlText != "INSERT INTO `users` (`name`) VALUES (?)" {
		t.Fatalf("unexpected sql: %s", sqlText)
	}
	if len(args) != 1 || args[0] != "bob" {
		t.Fatalf("unexpected args: %#v", args)
	}
}

func TestBuildUpdatesSkipBool(t *testing.T) {
	statement, err := createTestStatement()
	if err != nil {
		t.Fatalf("failed to create statement: %v", err)
	}
	value := TestType{ID: 1, IsDeleted: false, Caption: "cap"}
	colNames, args, err := statement.BuildUpdates(reflect.ValueOf(value), true, true, false, true, true)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	for _, col := range colNames {
		if strings.Contains(col, "IsDeleted") {
			t.Fatalf("expected IsDeleted to be skipped")
		}
	}
	if !containsCol(colNames, "Caption") || !containsArg(args, "cap") {
		t.Fatalf("expected caption update, cols=%v args=%v", colNames, args)
	}
}

func TestBuildUpdatesUseBool(t *testing.T) {
	statement, err := createTestStatement()
	if err != nil {
		t.Fatalf("failed to create statement: %v", err)
	}
	statement.UseBool()
	value := TestType{ID: 1, IsDeleted: false, Caption: "cap"}
	colNames, _, err := statement.BuildUpdates(reflect.ValueOf(value), true, true, false, true, true)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !containsCol(colNames, "IsDeleted") {
		t.Fatalf("expected IsDeleted to be included")
	}
}

func TestCondsAndOrIn(t *testing.T) {
	statement := newStatementWithTable(t, schemas.SQLITE)
	statement.And(map[string]any{"name": "bob"})
	statement.Or(builder.Expr("age>?", 18))
	statement.In("status", 1, 2)
	statement.NotIn("role", 3)
	if statement.LastError != nil {
		t.Fatalf("unexpected error: %v", statement.LastError)
	}
	writer := builder.NewWriter()
	if err := statement.writeWhere(writer); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	whereSQL := writer.String()
	if !strings.Contains(whereSQL, "`name`") || !strings.Contains(whereSQL, "IN") || !strings.Contains(whereSQL, "NOT IN") {
		t.Fatalf("unexpected where sql: %s", whereSQL)
	}

	statement.And(123)
	if statement.LastError != ErrConditionType {
		t.Fatalf("expected ErrConditionType, got %v", statement.LastError)
	}
}

func TestSetNoAutoCondition(t *testing.T) {
	statement := newStatementWithTable(t, schemas.SQLITE)
	statement.SetNoAutoCondition()
	if !statement.NoAutoCondition {
		t.Fatalf("expected NoAutoCondition true")
	}
	statement.SetNoAutoCondition(false)
	if statement.NoAutoCondition {
		t.Fatalf("expected NoAutoCondition false")
	}
}

func containsCol(cols []string, name string) bool {
	for _, col := range cols {
		if strings.Contains(col, name) {
			return true
		}
	}
	return false
}

func containsArg(args []any, target any) bool {
	for _, arg := range args {
		if arg == target {
			return true
		}
	}
	return false
}
