// Copyright 2017 The Xorm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package tests

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"xorm.io/xorm/internal/utils"
	"xorm.io/xorm/names"
)

func TestTransaction(t *testing.T) {
	assert.NoError(t, PrepareEngine())
	assertSync(t, new(Userinfo))

	counter := func(t *testing.T) {
		_, err := testEngine.Count(&Userinfo{})
		assert.NoError(t, err)
	}

	counter(t)
	// defer counter()

	session := testEngine.NewSession()
	defer session.Close()

	err := session.Begin()
	assert.NoError(t, err)

	user1 := Userinfo{Username: "xiaoxiao", Departname: "dev", Alias: "lunny", Created: time.Now()}
	_, err = session.Insert(&user1)
	assert.NoError(t, err)

	user2 := Userinfo{Username: "yyy"}
	_, err = session.Where("`id` = ?", 0).Update(&user2)
	assert.NoError(t, err)

	_, err = session.Delete(&user2)
	assert.NoError(t, err)

	err = session.Commit()
	assert.NoError(t, err)
}

func TestCombineTransaction(t *testing.T) {
	assert.NoError(t, PrepareEngine())
	assertSync(t, new(Userinfo))

	counter := func() {
		total, err := testEngine.Count(&Userinfo{})
		assert.NoError(t, err)
		t.Logf("----now total %v records", total)
	}

	counter()
	// defer counter()
	session := testEngine.NewSession()
	defer session.Close()

	err := session.Begin()
	assert.NoError(t, err)

	user1 := Userinfo{Username: "xiaoxiao2", Departname: "dev", Alias: "lunny", Created: time.Now()}
	_, err = session.Insert(&user1)
	assert.NoError(t, err)

	user2 := Userinfo{Username: "zzz"}
	_, err = session.Where("`id` = ?", 0).Update(&user2)
	assert.NoError(t, err)

	_, err = session.Exec("delete from "+testEngine.Quote(testEngine.TableName("userinfo", true))+" where `username` = ?", user2.Username)
	assert.NoError(t, err)

	err = session.Commit()
	assert.NoError(t, err)
}

func TestCombineTransactionSameMapper(t *testing.T) {
	assert.NoError(t, PrepareEngine())

	oldMapper := testEngine.GetColumnMapper()
	testEngine.UnMapType(utils.ReflectValue(new(Userinfo)).Type())
	testEngine.SetMapper(names.SameMapper{})
	defer func() {
		testEngine.UnMapType(utils.ReflectValue(new(Userinfo)).Type())
		testEngine.SetMapper(oldMapper)
	}()

	assertSync(t, new(Userinfo))

	counter := func() {
		total, err := testEngine.Count(&Userinfo{})
		assert.NoError(t, err)
		t.Logf("----now total %v records", total)
	}

	counter()
	defer counter()

	session := testEngine.NewSession()
	defer session.Close()

	err := session.Begin()
	assert.NoError(t, err)

	user1 := Userinfo{Username: "xiaoxiao2", Departname: "dev", Alias: "lunny", Created: time.Now()}
	_, err = session.Insert(&user1)
	assert.NoError(t, err)

	user2 := Userinfo{Username: "zzz"}
	_, err = session.Where("`id` = ?", 0).Update(&user2)
	assert.NoError(t, err)

	_, err = session.Exec("delete from  "+testEngine.Quote(testEngine.TableName("Userinfo", true))+" where `Username` = ?", user2.Username)
	assert.NoError(t, err)

	err = session.Commit()
	assert.NoError(t, err)
}

func TestMultipleTransaction(t *testing.T) {
	assert.NoError(t, PrepareEngine())

	type MultipleTransaction struct {
		Id   int64
		Name string
	}

	assertSync(t, new(MultipleTransaction))

	session := testEngine.NewSession()
	defer session.Close()

	err := session.Begin()
	assert.NoError(t, err)

	m1 := MultipleTransaction{Name: "xiaoxiao2"}
	_, err = session.Insert(&m1)
	assert.NoError(t, err)

	user2 := MultipleTransaction{Name: "zzz"}
	_, err = session.Where("`id` = ?", 0).Update(&user2)
	assert.NoError(t, err)

	err = session.Commit()
	assert.NoError(t, err)

	var ms []MultipleTransaction
	err = session.Find(&ms)
	assert.NoError(t, err)
	assert.EqualValues(t, 1, len(ms))

	err = session.Begin()
	assert.NoError(t, err)

	_, err = session.Where("`id`=?", m1.Id).Delete(new(MultipleTransaction))
	assert.NoError(t, err)

	err = session.Commit()
	assert.NoError(t, err)

	ms = make([]MultipleTransaction, 0)
	err = session.Find(&ms)
	assert.NoError(t, err)
	assert.EqualValues(t, 0, len(ms))

	err = session.Begin()
	assert.NoError(t, err)

	_, err = session.Insert(&MultipleTransaction{
		Name: "ssss",
	})
	assert.NoError(t, err)

	err = session.Rollback()
	assert.NoError(t, err)

	ms = make([]MultipleTransaction, 0)
	err = session.Find(&ms)
	assert.NoError(t, err)
	assert.EqualValues(t, 0, len(ms))
}

func TestInsertMulti2InterfaceTransaction(t *testing.T) {
	type Multi2InterfaceTransaction struct {
		ID         uint64 `xorm:"id pk autoincr"`
		Name       string
		Alias      string
		CreateTime time.Time `xorm:"created"`
		UpdateTime time.Time `xorm:"updated"`
	}
	assert.NoError(t, PrepareEngine())
	assertSync(t, new(Multi2InterfaceTransaction))
	session := testEngine.NewSession()
	defer session.Close()
	err := session.Begin()
	assert.NoError(t, err)

	users := []any{
		&Multi2InterfaceTransaction{Name: "a", Alias: "A"},
		&Multi2InterfaceTransaction{Name: "b", Alias: "B"},
		&Multi2InterfaceTransaction{Name: "c", Alias: "C"},
		&Multi2InterfaceTransaction{Name: "d", Alias: "D"},
	}
	cnt, err := session.Insert(&users)

	assert.NoError(t, err)
	assert.EqualValues(t, len(users), cnt)

	assert.NotPanics(t, func() {
		err = session.Commit()
		assert.NoError(t, err)
	})
}

func TestTransactionInsertAutoTime(t *testing.T) {
	assert.NoError(t, PrepareEngine())

	type TransactionAutoTime struct {
		ID      int64 `xorm:"pk autoincr"`
		Name    string
		Created time.Time `xorm:"created"`
		Updated time.Time `xorm:"updated"`
	}

	assertSync(t, new(TransactionAutoTime))

	session := testEngine.NewSession()
	defer session.Close()

	err := session.Begin()
	assert.NoError(t, err)

	item := &TransactionAutoTime{Name: "tx"}
	_, err = session.Insert(item)
	assert.NoError(t, err)
	assert.False(t, item.Created.IsZero())
	assert.False(t, item.Updated.IsZero())

	err = session.Rollback()
	assert.NoError(t, err)
}
