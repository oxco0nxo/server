// Copyright 2026 The Xorm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package statements

import (
	"errors"
	"reflect"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"xorm.io/xorm/caches"
	"xorm.io/xorm/dialects"
	"xorm.io/xorm/names"
	"xorm.io/xorm/schemas"
	"xorm.io/xorm/tags"
)

func TestMssqlLegacySelectDistinctTopOrder(t *testing.T) {
	statement, err := createMssqlLegacyStatement()
	assert.NoError(t, err)

	statement.Cols("ID")
	statement.IsDistinct = true
	statement.Limit(10)

	sql, _, err := statement.GenQuerySQL()
	assert.NoError(t, err)
	assert.Equal(t, "SELECT DISTINCT TOP 10 [ID] FROM [TestTable]", sql)
}

func createMssqlLegacyStatement() (*Statement, error) {
	dialect := dialects.QueryDialect(schemas.MSSQL)
	if dialect == nil {
		return nil, errors.New("mssql dialect not registered")
	}

	uri := &dialects.URI{DBType: schemas.MSSQL}
	if err := dialect.Init(uri); err != nil {
		return nil, err
	}
	// Enable legacy limit/offset behavior for SQL Server 2008-style pagination.
	dialect.SetParams(map[string]string{"USE_LEGACY_LIMIT_OFFSET": "true"})

	tagParser := tags.NewParser("xorm", dialect, names.SnakeMapper{}, names.SnakeMapper{}, caches.NewManager())
	statement := NewStatement(dialect, tagParser, time.Local)
	if err := statement.SetRefValue(reflect.ValueOf(TestType{})); err != nil {
		return nil, err
	}
	return statement, nil
}
