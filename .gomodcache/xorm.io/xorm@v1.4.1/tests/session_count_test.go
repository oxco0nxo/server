// Copyright 2021 The Xorm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package tests

import (
	"testing"

	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"xorm.io/builder"
	"xorm.io/xorm/schemas"
)

func TestCount(t *testing.T) {
	assert.NoError(t, PrepareEngine())

	type UserinfoCount struct {
		Departname string
	}
	assert.NoError(t, testEngine.Sync(new(UserinfoCount)))

	colName := testEngine.GetColumnMapper().Obj2Table("Departname")
	var cond builder.Cond = builder.Eq{
		"`" + colName + "`": "dev",
	}

	total, err := testEngine.Where(cond).Count(new(UserinfoCount))
	assert.NoError(t, err)
	assert.EqualValues(t, 0, total)

	cnt, err := testEngine.Insert(&UserinfoCount{
		Departname: "dev",
	})
	assert.NoError(t, err)
	assert.EqualValues(t, 1, cnt)

	total, err = testEngine.Where(cond).Count(new(UserinfoCount))
	assert.NoError(t, err)
	assert.EqualValues(t, 1, total)

	total, err = testEngine.Where(cond).Table("userinfo_count").Count()
	assert.NoError(t, err)
	assert.EqualValues(t, 1, total)

	total, err = testEngine.Table("userinfo_count").Count()
	assert.NoError(t, err)
	assert.EqualValues(t, 1, total)
}

func TestSQLCount(t *testing.T) {
	assert.NoError(t, PrepareEngine())

	type UserinfoCount2 struct {
		Id         int64
		Departname string
	}

	type UserinfoBooks struct {
		Id     int64
		Pid    int64
		IsOpen bool
	}

	assertSync(t, new(UserinfoCount2), new(UserinfoBooks))

	total, err := testEngine.SQL("SELECT count(`id`) FROM " + testEngine.Quote(testEngine.TableName("userinfo_count2", true))).
		Count()
	assert.NoError(t, err)
	assert.EqualValues(t, 0, total)
}

func TestCountWithOthers(t *testing.T) {
	assert.NoError(t, PrepareEngine())

	type CountWithOthers struct {
		Id   int64
		Name string
	}

	assertSync(t, new(CountWithOthers))

	_, err := testEngine.Insert(&CountWithOthers{
		Name: "orderby",
	})
	assert.NoError(t, err)

	_, err = testEngine.Insert(&CountWithOthers{
		Name: "limit",
	})
	assert.NoError(t, err)

	total, err := testEngine.OrderBy("count(`id`) desc").Limit(1).Count(new(CountWithOthers))
	assert.NoError(t, err)
	assert.EqualValues(t, 2, total)
}

type CountWithTableName struct {
	Id   int64
	Name string
}

func (CountWithTableName) TableName() string {
	return "count_with_table_name1"
}

func TestWithTableName(t *testing.T) {
	assert.NoError(t, PrepareEngine())

	assertSync(t, new(CountWithTableName))

	_, err := testEngine.Insert(&CountWithTableName{
		Name: "orderby",
	})
	assert.NoError(t, err)

	_, err = testEngine.Insert(CountWithTableName{
		Name: "limit",
	})
	assert.NoError(t, err)

	total, err := testEngine.Count(new(CountWithTableName))
	assert.NoError(t, err)
	assert.EqualValues(t, 2, total)

	total, err = testEngine.Count(CountWithTableName{})
	assert.NoError(t, err)
	assert.EqualValues(t, 2, total)
}

func TestCountWithSelectCols(t *testing.T) {
	assert.NoError(t, PrepareEngine())

	assertSync(t, new(CountWithTableName))

	_, err := testEngine.Insert(&CountWithTableName{
		Name: "orderby",
	})
	assert.NoError(t, err)

	_, err = testEngine.Insert(CountWithTableName{
		Name: "limit",
	})
	assert.NoError(t, err)

	total, err := testEngine.Cols("id").Count(new(CountWithTableName))
	assert.NoError(t, err)
	assert.EqualValues(t, 2, total)

	total, err = testEngine.Select("count(`id`)").Count(CountWithTableName{})
	assert.NoError(t, err)
	assert.EqualValues(t, 2, total)
}

func TestCountWithGroupBy(t *testing.T) {
	assert.NoError(t, PrepareEngine())

	assertSync(t, new(CountWithTableName))

	_, err := testEngine.Insert(&CountWithTableName{
		Name: "1",
	})
	assert.NoError(t, err)

	_, err = testEngine.Insert(CountWithTableName{
		Name: "2",
	})
	assert.NoError(t, err)

	cnt, err := testEngine.GroupBy("`name`").Count(new(CountWithTableName))
	assert.NoError(t, err)
	assert.EqualValues(t, 2, cnt)
}

func TestCountWithLimit(t *testing.T) {
	assert.NoError(t, PrepareEngine())

	assertSync(t, new(CountWithTableName))

	_, err := testEngine.Insert(&CountWithTableName{
		Name: "1",
	})
	assert.NoError(t, err)

	_, err = testEngine.Insert(CountWithTableName{
		Name: "2",
	})
	assert.NoError(t, err)

	cnt, err := testEngine.Limit(100).Count(new(CountWithTableName))
	assert.NoError(t, err)
	assert.EqualValues(t, 2, cnt)
}

func TestDistinctFindAndCount(t *testing.T) {
	assert.NoError(t, PrepareEngine())

	type TestDistinctFindAndCount struct {
		Id   int64
		Name string `xorm:"index"`
		Age2 int
	}

	assertSync(t, new(TestDistinctFindAndCount))

	objects := make([]*TestDistinctFindAndCount, 0, 10)
	total, err := testEngine.Distinct(testEngine.TableName(new(TestDistinctFindAndCount)) + ".*").FindAndCount(&objects)
	assert.NoError(t, err)
	assert.EqualValues(t, 0, total)
}

func TestDistinctFindAndCountWithLimit(t *testing.T) {
	assert.NoError(t, PrepareEngine())

	type DistinctFindAndCountWithLimit struct {
		Name string
		Age  int
	}

	assertSync(t, new(DistinctFindAndCountWithLimit))

	_, err := testEngine.Insert([]DistinctFindAndCountWithLimit{
		{
			Name: "dup",
			Age:  10,
		},
		{
			Name: "dup",
			Age:  10,
		},
	})
	assert.NoError(t, err)

	objects := make([]*DistinctFindAndCountWithLimit, 0, 10)
	total, err := testEngine.Distinct(testEngine.TableName(new(DistinctFindAndCountWithLimit)) + ".*").Limit(1).
		FindAndCount(&objects)
	assert.NoError(t, err)
	assert.EqualValues(t, 1, total)
}

func TestCountStructWithDecimal(t *testing.T) {
	assert.NoError(t, PrepareEngine())
	type ProductCount struct {
		Uid   int             `xorm:"pk autoincr"`
		Price decimal.Decimal `xorm:"decimal(35,30)"`
	}
	assert.NoError(t, testEngine.Sync(new(ProductCount)))

	session := testEngine.NewSession()
	defer session.Close()
	var err error
	if testEngine.Dialect().URI().DBType == schemas.MSSQL {
		err = session.Begin()
		assert.NoError(t, err)
		_, err = session.Exec("SET IDENTITY_INSERT `product_count` ON")
		assert.NoError(t, err)
	}
	cnt, err := session.Insert(&ProductCount{Uid: 2, Price: decimal.NewFromFloat(0.8)})
	assert.NoError(t, err)
	assert.EqualValues(t, 1, cnt)
	cnt, err = session.Insert(&ProductCount{Uid: 3, Price: decimal.NewFromFloat(1.5)})
	assert.NoError(t, err)
	assert.EqualValues(t, 1, cnt)
	if testEngine.Dialect().URI().DBType == schemas.MSSQL {
		err = session.Commit()
		assert.NoError(t, err)
	}

	// Expected SQL: SELECT count(*) FROM `product_count` WHERE price>?
	// Wrong SQL: SELECT count(*) FROM `product_count` WHERE price>? AND `price`=?
	total, err := testEngine.Where(builder.Gt{"price": decimal.NewFromInt(1)}).Count(new(ProductCount))
	assert.NoError(t, err)
	assert.Equal(t, int64(1), total)
}
