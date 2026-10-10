// Copyright 2026 The Xorm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package statements

import (
	"strings"
	"testing"

	"xorm.io/xorm/schemas"
)

func TestSelectAndColumnStr(t *testing.T) {
	statement := newTestStatement(t, schemas.SQLITE)
	statement.Select("id, name")
	if statement.SelectStr != "id, name" {
		t.Fatalf("unexpected SelectStr: %s", statement.SelectStr)
	}

	statement.Cols("`ID`", "name, code")
	if got := statement.ColumnStr(); got != "`ID`, `name`, `code`" {
		t.Fatalf("unexpected ColumnStr: %s", got)
	}
}

func TestAllColsMustColsUseBool(t *testing.T) {
	statement, err := createTestStatement()
	if err != nil {
		t.Fatalf("failed to create statement: %v", err)
	}
	statement.AllCols()
	if !statement.useAllCols {
		t.Fatalf("expected useAllCols to be true")
	}

	statement.MustCols("Name", "Code")
	if !statement.MustColumnMap["name"] || !statement.MustColumnMap["code"] {
		t.Fatalf("expected MustColumnMap to include columns")
	}

	statement.UseBool("Flag")
	if !statement.MustColumnMap["flag"] {
		t.Fatalf("expected UseBool to set MustColumnMap")
	}

	statement, err = createTestStatement()
	if err != nil {
		t.Fatalf("failed to create statement: %v", err)
	}
	statement.UseBool()
	if !statement.allUseBool {
		t.Fatalf("expected allUseBool to be true")
	}
}

func TestOmitAndDistinct(t *testing.T) {
	statement := newTestStatement(t, schemas.SQLITE)
	statement.Omit("Name", "Code")
	if !statement.OmitColumnMap.Contain("name") || !statement.OmitColumnMap.Contain("code") {
		t.Fatalf("expected omit columns to be recorded")
	}

	statement.Distinct("ID")
	if !statement.IsDistinct {
		t.Fatalf("expected IsDistinct to be true")
	}
	if !statement.ColumnMap.Contain("id") {
		t.Fatalf("expected Distinct to add columns")
	}
}

func TestColNameWithTableAndAlias(t *testing.T) {
	statement := newTestStatement(t, schemas.SQLITE)
	statement.joins = []join{{}}
	statement.TableAlias = "tt"
	col := &schemas.Column{Name: "ID"}
	if got := statement.colName(col, "ignored"); got != "`tt`.`ID`" {
		t.Fatalf("unexpected col name with alias: %s", got)
	}

	statement.TableAlias = ""
	statement.tableName = "test_table"
	if got := statement.colName(col, statement.TableName()); got != "`test_table`.`ID`" {
		t.Fatalf("unexpected col name with table: %s", got)
	}
}

func TestGenColumnStrWithJoin(t *testing.T) {
	statement, err := createTestStatement()
	if err != nil {
		t.Fatalf("failed to create statement: %v", err)
	}
	statement.joins = []join{{}}
	statement.TableAlias = "tt"
	colStr := statement.genColumnStr()
	if !strings.HasPrefix(colStr, "tt.") {
		t.Fatalf("expected columns to be prefixed with alias, got %s", colStr)
	}
}
