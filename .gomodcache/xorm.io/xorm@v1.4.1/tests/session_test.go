// Copyright 2017 The Xorm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package tests

import (
	"database/sql"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestClose(t *testing.T) {
	assert.NoError(t, PrepareEngine())

	sess1 := testEngine.NewSession()
	sess1.Close()
	assert.True(t, sess1.IsClosed())

	sess2 := testEngine.Where("`a` = ?", 1)
	sess2.Close()
	assert.True(t, sess2.IsClosed())
}

func TestNullFloatStruct(t *testing.T) {
	type MyNullFloat64 sql.NullFloat64

	type MyNullFloatStruct struct {
		Uuid   string
		Amount MyNullFloat64
	}

	assert.NoError(t, PrepareEngine())
	assert.NoError(t, testEngine.Sync(new(MyNullFloatStruct)))

	_, err := testEngine.Insert(&MyNullFloatStruct{
		Uuid: "111111",
		Amount: MyNullFloat64(sql.NullFloat64{
			Float64: 0.1111,
			Valid:   true,
		}),
	})
	assert.NoError(t, err)
}

func TestMustLogSQL(t *testing.T) {
	assert.NoError(t, PrepareEngine())
	testEngine.ShowSQL(false)
	defer testEngine.ShowSQL(true)

	assertSync(t, new(Userinfo))

	_, err := testEngine.Table("userinfo").MustLogSQL(true).Get(new(Userinfo))
	assert.NoError(t, err)
}

func TestEnableSessionId(t *testing.T) {
	assert.NoError(t, PrepareEngine())
	testEngine.EnableSessionID(true)
	assertSync(t, new(Userinfo))
	_, err := testEngine.Table("userinfo").MustLogSQL(true).Get(new(Userinfo))
	assert.NoError(t, err)
}

func TestIndexHint(t *testing.T) {
	assert.NoError(t, PrepareEngine())
	assertSync(t, new(Userinfo), new(Userdetail))
	if testEngine.Dialect().URI().DBType != "mysql" {
		return
	}

	_, err := testEngine.Table("userinfo").IndexHint("USE", "", "UQE_userinfo_username").Get(new(Userinfo))
	assert.NoError(t, err)

	_, err = testEngine.Table("userinfo").IndexHint("USE", "ORDER BY", "UQE_userinfo_username").Get(new(Userinfo))
	assert.NoError(t, err)

	_, err = testEngine.Table("userinfo").IndexHint("USE", "GROUP BY", "UQE_userinfo_username").Get(new(Userinfo))
	assert.NoError(t, err)

	// with join
	_, err = testEngine.Table("userinfo").
		Join("LEFT", "userdetail", "userinfo.id = userdetail.id").
		IndexHint("USE", "", "UQE_userinfo_username").
		Get(new(Userinfo))
	assert.NoError(t, err)
}
