// Copyright 2026 The Xorm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package statements

import (
	"errors"
	"strings"
	"testing"

	"xorm.io/builder"
	"xorm.io/xorm/schemas"
)

func TestColumnMapAddAndContain(t *testing.T) {
	var cols columnMap
	if !cols.IsEmpty() {
		t.Fatalf("expected empty column map")
	}
	if !cols.Add("ID") {
		t.Fatalf("expected to add new column")
	}
	if cols.Add("id") {
		t.Fatalf("expected duplicate add to return false")
	}
	if cols.Len() != 1 {
		t.Fatalf("expected length 1, got %d", cols.Len())
	}
	if !cols.Contain("Id") {
		t.Fatalf("expected case-insensitive contain")
	}
	if cols.Contain("Name") {
		t.Fatalf("did not expect missing column")
	}
}

func TestGetFlagForColumn(t *testing.T) {
	flags := map[string]bool{"ID": true}
	val, ok := getFlagForColumn(flags, &schemas.Column{Name: "id"})
	if !ok || !val {
		t.Fatalf("expected true flag, got %v %v", val, ok)
	}

	flags = map[string]bool{"IDX": true}
	val, ok = getFlagForColumn(flags, &schemas.Column{Name: "ID"})
	if ok || val {
		t.Fatalf("expected no match with different length")
	}
}

func TestOrderByBehavior(t *testing.T) {
	statement := newTestStatement(t, schemas.SQLITE)
	statement.OrderBy("")
	if statement.LastError == nil {
		t.Fatalf("expected error for empty order by")
	}

	statement.ResetOrderBy()
	statement.Desc()
	if statement.LastError != ErrNoColumnName {
		t.Fatalf("expected ErrNoColumnName, got %v", statement.LastError)
	}

	statement.ResetOrderBy()
	statement.Desc("name")
	expr := builder.Expr("RAND(?)", 1).(*builder.Expression)
	statement.OrderBy(expr)
	writer := builder.NewWriter()
	if err := statement.writeOrderBys(writer); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if writer.String() != " ORDER BY `name` DESC, RAND(?)" {
		t.Fatalf("unexpected order by: %s", writer.String())
	}
	if len(writer.Args()) != 1 || writer.Args()[0] != 1 {
		t.Fatalf("unexpected args: %#v", writer.Args())
	}
}

func TestWritePaginationLimitOffset(t *testing.T) {
	statement := newTestStatement(t, schemas.SQLITE)
	limit := 10
	statement.Start = 5
	statement.LimitN = &limit
	writer := builder.NewWriter()
	if err := statement.writePagination(writer); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if writer.String() != " LIMIT 10 OFFSET 5" {
		t.Fatalf("unexpected pagination: %s", writer.String())
	}
}

func TestWriteOffsetFetch(t *testing.T) {
	statement := newTestStatement(t, schemas.MSSQL)
	limit := 10
	statement.LimitN = &limit
	writer := builder.NewWriter()
	if err := statement.writePagination(writer); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if writer.String() != " OFFSET 0 ROWS FETCH NEXT 10 ROWS ONLY" {
		t.Fatalf("unexpected pagination: %s", writer.String())
	}
}

func TestWriteMssqlPaginationCondNoTable(t *testing.T) {
	statement := newTestStatement(t, schemas.MSSQL)
	statement.Start = 1
	writer := builder.NewWriter()
	if err := statement.writeMssqlPaginationCond(writer); err == nil {
		t.Fatalf("expected error without reference table")
	}
}

func TestWriteOracleLimit(t *testing.T) {
	statement := newTestStatement(t, schemas.ORACLE)
	limit := 3
	statement.Start = 2
	statement.LimitN = &limit
	writer := builder.NewWriter()
	writer.WriteString("SELECT * FROM test")
	if err := statement.writeOracleLimit("*")(writer); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(writer.String(), "ROWNUM <= 5") || !strings.Contains(writer.String(), "RN > 2") {
		t.Fatalf("unexpected oracle limit sql: %s", writer.String())
	}
}

func TestTableNameAndAlias(t *testing.T) {
	oracleStmt := newTestStatement(t, schemas.ORACLE)
	oracleStmt.tableName = "users"
	oracleStmt.Alias("u")
	writer := builder.NewWriter()
	if err := oracleStmt.writeAlias(writer); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if writer.String() != " \"u\"" {
		t.Fatalf("unexpected oracle alias: %s", writer.String())
	}

	sqliteStmt := newTestStatement(t, schemas.SQLITE)
	sqliteStmt.tableName = "users"
	sqliteStmt.Alias("u")
	writer = builder.NewWriter()
	if err := sqliteStmt.writeAlias(writer); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if writer.String() != " AS `u`" {
		t.Fatalf("unexpected sqlite alias: %s", writer.String())
	}

	mssqlStmt := newTestStatement(t, schemas.MSSQL)
	mssqlStmt.tableName = "db..table"
	writer = builder.NewWriter()
	if err := mssqlStmt.writeTableName(writer); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if writer.String() != "db..table" {
		t.Fatalf("unexpected mssql table name: %s", writer.String())
	}
}

func TestWriterHelpers(t *testing.T) {
	statement := newTestStatement(t, schemas.SQLITE)
	writer := builder.NewWriter()
	if err := statement.writeStrings("a", "b")(writer); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if writer.String() != "ab" {
		t.Fatalf("unexpected writeStrings result: %s", writer.String())
	}

	writer.Reset()
	if err := statement.groupWriteFns(statement.writeStrings("a"), statement.writeStrings("b"))(writer); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if writer.String() != "ab" {
		t.Fatalf("unexpected groupWriteFns result: %s", writer.String())
	}

	writer.Reset()
	errTest := errors.New("write failed")
	if err := statement.writeMultiple(writer, statement.writeStrings("a"), func(*builder.BytesWriter) error {
		return errTest
	}); err != errTest {
		t.Fatalf("expected error %v, got %v", errTest, err)
	}
}
