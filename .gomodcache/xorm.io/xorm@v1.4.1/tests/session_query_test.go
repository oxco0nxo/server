// Copyright 2017 The Xorm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package tests

import (
	"bytes"
	"strconv"
	"testing"
	"time"

	"xorm.io/builder"

	"xorm.io/xorm/schemas"

	"github.com/stretchr/testify/assert"
)

func TestQueryString(t *testing.T) {
	assert.NoError(t, PrepareEngine())

	type GetVar2 struct {
		Id      int64  `xorm:"autoincr pk"`
		Msg     string `xorm:"varchar(255)"`
		Age     int
		Money   float32
		Created time.Time `xorm:"created"`
	}

	assert.NoError(t, testEngine.Sync(new(GetVar2)))

	data := GetVar2{
		Msg:   "hi",
		Age:   28,
		Money: 1.5,
	}
	_, err := testEngine.Insert(data)
	assert.NoError(t, err)

	records, err := testEngine.QueryString("select * from " + testEngine.Quote(testEngine.TableName("get_var2", true)))
	assert.NoError(t, err)
	assert.Equal(t, 1, len(records))
	assert.Equal(t, 5, len(records[0]))
	assert.Equal(t, "1", records[0]["id"])
	assert.Equal(t, "hi", records[0]["msg"])
	assert.Equal(t, "28", records[0]["age"])
	assert.Equal(t, "1.5", records[0]["money"])
}

func TestQueryString2(t *testing.T) {
	assert.NoError(t, PrepareEngine())

	type GetVar3 struct {
		Id  int64 `xorm:"autoincr pk"`
		Msg bool
	}

	assert.NoError(t, testEngine.Sync(new(GetVar3)))

	data := GetVar3{
		Msg: false,
	}
	_, err := testEngine.Insert(data)
	assert.NoError(t, err)

	records, err := testEngine.QueryString("select * from " + testEngine.Quote(testEngine.TableName("get_var3", true)))
	assert.NoError(t, err)
	assert.Equal(t, 1, len(records))
	assert.Equal(t, 2, len(records[0]))
	assert.Equal(t, "1", records[0]["id"])
	assert.True(t, "0" == records[0]["msg"] || "false" == records[0]["msg"])
}

func toBool(i any) bool {
	switch t := i.(type) {
	case int32:
		return t > 0
	case bool:
		return t
	}
	return false
}

func TestQueryInterface(t *testing.T) {
	assert.NoError(t, PrepareEngine())

	type GetVarInterface struct {
		Id      int64  `xorm:"autoincr pk"`
		Msg     string `xorm:"varchar(255)"`
		Age     int
		Money   float32
		Created time.Time `xorm:"created"`
	}

	assert.NoError(t, testEngine.Sync(new(GetVarInterface)))

	data := GetVarInterface{
		Msg:   "hi",
		Age:   28,
		Money: 1.5,
	}
	_, err := testEngine.Insert(data)
	assert.NoError(t, err)

	records, err := testEngine.QueryInterface("select * from " + testEngine.Quote(testEngine.TableName("get_var_interface", true)))
	assert.NoError(t, err)
	assert.Equal(t, 1, len(records))
	assert.Equal(t, 5, len(records[0]))
	assert.EqualValues(t, int64(1), records[0]["id"])
	assert.Equal(t, "hi", records[0]["msg"])
	assert.EqualValues(t, 28, records[0]["age"])
	assert.EqualValues(t, 1.5, records[0]["money"])
	createdAt, ok := records[0]["created"].(time.Time)
	assert.True(t, ok)
	assert.False(t, createdAt.IsZero())
}

func TestQueryNoParams(t *testing.T) {
	assert.NoError(t, PrepareEngine())

	type QueryNoParams struct {
		Id      int64  `xorm:"autoincr pk"`
		Msg     string `xorm:"varchar(255)"`
		Age     int
		Money   float32
		Created time.Time `xorm:"created"`
	}

	testEngine.ShowSQL(true)

	assert.NoError(t, testEngine.Sync(new(QueryNoParams)))

	q := QueryNoParams{
		Msg:   "message",
		Age:   20,
		Money: 3000,
	}
	cnt, err := testEngine.Insert(&q)
	assert.NoError(t, err)
	assert.EqualValues(t, 1, cnt)

	assertResult := func(t *testing.T, results []map[string][]byte) {
		assert.EqualValues(t, 1, len(results))
		id, err := strconv.ParseInt(string(results[0]["id"]), 10, 64)
		assert.NoError(t, err)
		assert.EqualValues(t, 1, id)
		assert.Equal(t, "message", string(results[0]["msg"]))

		age, err := strconv.Atoi(string(results[0]["age"]))
		assert.NoError(t, err)
		assert.EqualValues(t, 20, age)

		money, err := strconv.ParseFloat(string(results[0]["money"]), 32)
		assert.NoError(t, err)
		assert.EqualValues(t, 3000, money)
	}

	results, err := testEngine.Table("query_no_params").Limit(10).Query()
	assert.NoError(t, err)
	assertResult(t, results)

	results, err = testEngine.SQL("select * from " + testEngine.Quote(testEngine.TableName("query_no_params", true))).Query()
	assert.NoError(t, err)
	assertResult(t, results)
}

func TestQueryStringNoParam(t *testing.T) {
	assert.NoError(t, PrepareEngine())

	type GetVar4 struct {
		Id  int64 `xorm:"autoincr pk"`
		Msg bool
	}

	assert.NoError(t, testEngine.Sync(new(GetVar4)))

	data := GetVar4{
		Msg: false,
	}
	_, err := testEngine.Insert(data)
	assert.NoError(t, err)

	records, err := testEngine.Table("get_var4").Limit(1).QueryString()
	assert.NoError(t, err)
	assert.EqualValues(t, 1, len(records))
	assert.EqualValues(t, "1", records[0]["id"])
	if testEngine.Dialect().URI().DBType == schemas.POSTGRES || testEngine.Dialect().URI().DBType == schemas.MSSQL {
		assert.EqualValues(t, "false", records[0]["msg"])
	} else {
		assert.EqualValues(t, "0", records[0]["msg"])
	}

	records, err = testEngine.Table("get_var4").Where(builder.Eq{"`id`": 1}).QueryString()
	assert.NoError(t, err)
	assert.EqualValues(t, 1, len(records))
	assert.EqualValues(t, "1", records[0]["id"])
	if testEngine.Dialect().URI().DBType == schemas.POSTGRES || testEngine.Dialect().URI().DBType == schemas.MSSQL {
		assert.EqualValues(t, "false", records[0]["msg"])
	} else {
		assert.EqualValues(t, "0", records[0]["msg"])
	}
}

func TestQuerySliceStringNoParam(t *testing.T) {
	assert.NoError(t, PrepareEngine())

	type GetVar6 struct {
		Id  int64 `xorm:"autoincr pk"`
		Msg bool
	}

	assert.NoError(t, testEngine.Sync(new(GetVar6)))

	data := GetVar6{
		Msg: false,
	}
	_, err := testEngine.Insert(data)
	assert.NoError(t, err)

	records, err := testEngine.Table("get_var6").Limit(1).QuerySliceString()
	assert.NoError(t, err)
	assert.EqualValues(t, 1, len(records))
	assert.EqualValues(t, "1", records[0][0])
	if testEngine.Dialect().URI().DBType == schemas.POSTGRES || testEngine.Dialect().URI().DBType == schemas.MSSQL {
		assert.EqualValues(t, "false", records[0][1])
	} else {
		assert.EqualValues(t, "0", records[0][1])
	}

	records, err = testEngine.Table("get_var6").Where(builder.Eq{"`id`": 1}).QuerySliceString()
	assert.NoError(t, err)
	assert.EqualValues(t, 1, len(records))
	assert.EqualValues(t, "1", records[0][0])
	if testEngine.Dialect().URI().DBType == schemas.POSTGRES || testEngine.Dialect().URI().DBType == schemas.MSSQL {
		assert.EqualValues(t, "false", records[0][1])
	} else {
		assert.EqualValues(t, "0", records[0][1])
	}
}

func TestQueryInterfaceNoParam(t *testing.T) {
	assert.NoError(t, PrepareEngine())

	type GetVar5 struct {
		Id  int64 `xorm:"autoincr pk"`
		Msg bool
	}

	assert.NoError(t, testEngine.Sync(new(GetVar5)))

	data := GetVar5{
		Msg: false,
	}
	_, err := testEngine.Insert(data)
	assert.NoError(t, err)

	records, err := testEngine.Table("get_var5").Limit(1).QueryInterface()
	assert.NoError(t, err)
	assert.EqualValues(t, 1, len(records))
	assert.EqualValues(t, 1, records[0]["id"])
	assert.False(t, toBool(records[0]["msg"]))

	records, err = testEngine.Table("get_var5").Where(builder.Eq{"`id`": 1}).QueryInterface()
	assert.NoError(t, err)
	assert.EqualValues(t, 1, len(records))
	assert.EqualValues(t, 1, records[0]["id"])
	assert.False(t, toBool(records[0]["msg"]))
}

func TestQueryWithBuilder(t *testing.T) {
	assert.NoError(t, PrepareEngine())

	type QueryWithBuilder struct {
		Id      int64  `xorm:"autoincr pk"`
		Msg     string `xorm:"varchar(255)"`
		Age     int
		Money   float32
		Created time.Time `xorm:"created"`
	}

	testEngine.ShowSQL(true)

	assert.NoError(t, testEngine.Sync(new(QueryWithBuilder)))

	q := QueryWithBuilder{
		Msg:   "message",
		Age:   20,
		Money: 3000,
	}
	cnt, err := testEngine.Insert(&q)
	assert.NoError(t, err)
	assert.EqualValues(t, 1, cnt)

	assertResult := func(t *testing.T, results []map[string][]byte) {
		assert.EqualValues(t, 1, len(results))
		id, err := strconv.ParseInt(string(results[0]["id"]), 10, 64)
		assert.NoError(t, err)
		assert.EqualValues(t, 1, id)
		assert.Equal(t, "message", string(results[0]["msg"]))

		age, err := strconv.Atoi(string(results[0]["age"]))
		assert.NoError(t, err)
		assert.EqualValues(t, 20, age)

		money, err := strconv.ParseFloat(string(results[0]["money"]), 32)
		assert.NoError(t, err)
		assert.EqualValues(t, 3000, money)
	}

	results, err := testEngine.Query(builder.Select("*").From(testEngine.Quote(testEngine.TableName("query_with_builder", true))))
	assert.NoError(t, err)
	assertResult(t, results)
}

func TestJoinWithSubQuery(t *testing.T) {
	assert.NoError(t, PrepareEngine())

	type JoinWithSubQuery1 struct {
		Id       int64  `xorm:"autoincr pk"`
		Msg      string `xorm:"varchar(255)"`
		DepartId int64
		Money    float32
	}

	type JoinWithSubQueryDepart struct {
		Id   int64 `xorm:"autoincr pk"`
		Name string
	}

	testEngine.ShowSQL(true)

	assert.NoError(t, testEngine.Sync(new(JoinWithSubQuery1), new(JoinWithSubQueryDepart)))

	depart := JoinWithSubQueryDepart{
		Name: "depart1",
	}
	cnt, err := testEngine.Insert(&depart)
	assert.NoError(t, err)
	assert.EqualValues(t, 1, cnt)

	q := JoinWithSubQuery1{
		Msg:      "message",
		DepartId: depart.Id,
		Money:    3000,
	}

	cnt, err = testEngine.Insert(&q)
	assert.NoError(t, err)
	assert.EqualValues(t, 1, cnt)

	tbName := testEngine.Quote(testEngine.TableName("join_with_sub_query_depart", true))
	var querys []JoinWithSubQuery1
	err = testEngine.Join("INNER", builder.Select("`id`").From(tbName),
		"`join_with_sub_query_depart`.`id` = `join_with_sub_query1`.`depart_id`").Find(&querys)
	assert.NoError(t, err)
	assert.EqualValues(t, 1, len(querys))
	assert.EqualValues(t, q, querys[0])

	querys = make([]JoinWithSubQuery1, 0, 1)
	err = testEngine.Join("INNER", "(SELECT `id` FROM "+tbName+") `a`", "`a`.`id` = `join_with_sub_query1`.`depart_id`").
		Find(&querys)
	assert.NoError(t, err)
	assert.EqualValues(t, 1, len(querys))
	assert.EqualValues(t, q, querys[0])
}

func TestQueryStringWithLimit(t *testing.T) {
	assert.NoError(t, PrepareEngine())

	type QueryWithLimit struct {
		Id       int64  `xorm:"autoincr pk"`
		Msg      string `xorm:"varchar(255)"`
		DepartId int64
		Money    float32
	}

	assert.NoError(t, testEngine.Sync(new(QueryWithLimit)))

	data, err := testEngine.Table("query_with_limit").Limit(20, 20).QueryString()
	assert.NoError(t, err)
	assert.EqualValues(t, 0, len(data))
}

func TestQueryBLOBInMySQL(t *testing.T) {
	assert.NoError(t, PrepareEngine())

	var err error
	type Avatar struct {
		Id     int64  `xorm:"autoincr pk"`
		Avatar []byte `xorm:"BLOB"`
	}

	assert.NoError(t, testEngine.Sync(new(Avatar)))
	testEngine.Delete(Avatar{})

	repeatBytes := func(n int, b byte) []byte {
		return bytes.Repeat([]byte{b}, n)
	}

	const N = 10
	data := []Avatar{}
	for i := 0; i < N; i++ {
		// allocate a []byte that is as twice big as the last one
		// so that the underlying buffer will need to reallocate when querying
		bs := repeatBytes(1<<(i+2), 'A'+byte(i))
		data = append(data, Avatar{
			Avatar: bs,
		})
	}
	_, err = testEngine.Insert(data)
	assert.NoError(t, err)
	defer func() {
		testEngine.Delete(Avatar{})
	}()

	{
		records, err := testEngine.QueryInterface("select avatar from " + testEngine.Quote(testEngine.TableName("avatar", true)))
		assert.NoError(t, err)
		for i, record := range records {
			bs := record["avatar"].([]byte)
			assert.EqualValues(t, repeatBytes(1<<(i+2), 'A'+byte(i))[:3], bs[:3])
			t.Logf("%d => %p => %02x %02x %02x", i, bs, bs[0], bs[1], bs[2])
		}
	}

	{
		arr := make([][]any, 0)
		err = testEngine.Table(testEngine.Quote(testEngine.TableName("avatar", true))).Cols("avatar").Find(&arr)
		assert.NoError(t, err)
		for i, record := range arr {
			bs := record[0].([]byte)
			assert.EqualValues(t, repeatBytes(1<<(i+2), 'A'+byte(i))[:3], bs[:3])
			t.Logf("%d => %p => %02x %02x %02x", i, bs, bs[0], bs[1], bs[2])
		}
	}

	{
		arr := make([]map[string]any, 0)
		err = testEngine.Table(testEngine.Quote(testEngine.TableName("avatar", true))).Cols("avatar").Find(&arr)
		assert.NoError(t, err)
		for i, record := range arr {
			bs := record["avatar"].([]byte)
			assert.EqualValues(t, repeatBytes(1<<(i+2), 'A'+byte(i))[:3], bs[:3])
			t.Logf("%d => %p => %02x %02x %02x", i, bs, bs[0], bs[1], bs[2])
		}
	}
}

func TestRowsReset(t *testing.T) {
	assert.NoError(t, PrepareEngine())

	type RowsReset1 struct {
		Id   int64
		Name string
	}

	type RowsReset2 struct {
		Id   int64
		Name string
	}

	assert.NoError(t, testEngine.Sync(new(RowsReset1), new(RowsReset2)))

	data := []RowsReset1{
		{0, "1"},
		{0, "2"},
		{0, "3"},
	}
	_, err := testEngine.Insert(data)
	assert.NoError(t, err)

	data2 := []RowsReset2{
		{0, "4"},
		{0, "5"},
		{0, "6"},
	}
	_, err = testEngine.Insert(data2)
	assert.NoError(t, err)

	sess := testEngine.NewSession()
	defer sess.Close()

	rows, err := sess.Rows(new(RowsReset1))
	assert.NoError(t, err)
	for rows.Next() {
		var data1 RowsReset1
		assert.NoError(t, rows.Scan(&data1))
	}
	rows.Close()

	var rrs []RowsReset2
	assert.NoError(t, sess.Find(&rrs))

	assert.Len(t, rrs, 3)
	assert.EqualValues(t, "4", rrs[0].Name)
	assert.EqualValues(t, "5", rrs[1].Name)
	assert.EqualValues(t, "6", rrs[2].Name)
}
