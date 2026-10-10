// Copyright 2022 The Xorm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package builder

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestExists(t *testing.T) {
	cond1 := Exists(Select("*").From("table").Where(Eq{"col1": 1}))
	sql, err := ToBoundSQL(cond1)
	assert.NoError(t, err)
	assert.EqualValues(t, "EXISTS (SELECT * FROM table WHERE col1=1)", sql)
}

func TestNotExists1(t *testing.T) {
	cond1 := Exists(Select("*").From("table").Where(Eq{"col1": 1}))
	sql, err := ToBoundSQL(Not{cond1})
	assert.NoError(t, err)
	assert.EqualValues(t, "NOT EXISTS (SELECT * FROM table WHERE col1=1)", sql)
}

func TestNotExists2(t *testing.T) {
	cond1 := NotExists(Select("*").From("table").Where(Eq{"col1": 1}))
	sql, err := ToBoundSQL(cond1)
	assert.NoError(t, err)
	assert.EqualValues(t, "NOT EXISTS (SELECT * FROM table WHERE col1=1)", sql)
}
