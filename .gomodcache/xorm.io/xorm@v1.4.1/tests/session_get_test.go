// Copyright 2017 The Xorm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package tests

import (
	"database/sql"
	"errors"
	"fmt"
	"math/big"
	"testing"
	"time"

	"xorm.io/builder"
	"xorm.io/xorm"
	"xorm.io/xorm/contexts"
	"xorm.io/xorm/convert"
	"xorm.io/xorm/dialects"
	"xorm.io/xorm/schemas"

	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
)

func TestGetVar(t *testing.T) {
	assert.NoError(t, PrepareEngine())

	type GetVar struct {
		Id      int64  `xorm:"autoincr pk"`
		Msg     string `xorm:"varchar(255)"`
		Age     int
		Money   float32
		Created time.Time `xorm:"created"`
	}

	assert.NoError(t, testEngine.Sync(new(GetVar)))

	data := GetVar{
		Msg:   "hi",
		Age:   28,
		Money: 1.5,
	}
	_, err := testEngine.Insert(&data)
	assert.NoError(t, err)

	var msg string
	has, err := testEngine.Table("get_var").Cols("msg").Get(&msg)
	assert.NoError(t, err)
	assert.Equal(t, true, has)
	assert.Equal(t, "hi", msg)

	var age int
	has, err = testEngine.Table("get_var").Cols("age").Get(&age)
	assert.NoError(t, err)
	assert.Equal(t, true, has)
	assert.Equal(t, 28, age)

	var ageMax int
	has, err = testEngine.SQL("SELECT max(`age`) FROM "+testEngine.Quote(testEngine.TableName("get_var", true))+" WHERE `id` = ?", data.Id).Get(&ageMax)
	assert.NoError(t, err)
	assert.Equal(t, true, has)
	assert.Equal(t, 28, ageMax)

	var age2 int64
	has, err = testEngine.Table("get_var").Cols("age").
		Where("`age` > ?", 20).
		And("`age` < ?", 30).
		Get(&age2)
	assert.NoError(t, err)
	assert.Equal(t, true, has)
	assert.EqualValues(t, 28, age2)

	var age3 int8
	has, err = testEngine.Table("get_var").Cols("age").Get(&age3)
	assert.NoError(t, err)
	assert.Equal(t, true, has)
	assert.EqualValues(t, 28, age3)

	var age4 int16
	has, err = testEngine.Table("get_var").Cols("age").
		Where("`age` > ?", 20).
		And("`age` < ?", 30).
		Get(&age4)
	assert.NoError(t, err)
	assert.Equal(t, true, has)
	assert.EqualValues(t, 28, age4)

	var age5 int32
	has, err = testEngine.Table("get_var").Cols("age").
		Where("`age` > ?", 20).
		And("`age` < ?", 30).
		Get(&age5)
	assert.NoError(t, err)
	assert.Equal(t, true, has)
	assert.EqualValues(t, 28, age5)

	var age6 int
	has, err = testEngine.Table("get_var").Cols("age").Get(&age6)
	assert.NoError(t, err)
	assert.Equal(t, true, has)
	assert.EqualValues(t, 28, age6)

	var age7 int64
	has, err = testEngine.Table("get_var").Cols("age").
		Where("`age` > ?", 20).
		And("`age` < ?", 30).
		Get(&age7)
	assert.NoError(t, err)
	assert.Equal(t, true, has)
	assert.EqualValues(t, 28, age7)

	var age8 int8
	has, err = testEngine.Table("get_var").Cols("age").Get(&age8)
	assert.NoError(t, err)
	assert.Equal(t, true, has)
	assert.EqualValues(t, 28, age8)

	var age9 int16
	has, err = testEngine.Table("get_var").Cols("age").
		Where("`age` > ?", 20).
		And("`age` < ?", 30).
		Get(&age9)
	assert.NoError(t, err)
	assert.Equal(t, true, has)
	assert.EqualValues(t, 28, age9)

	var age10 int32
	has, err = testEngine.Table("get_var").Cols("age").
		Where("`age` > ?", 20).
		And("`age` < ?", 30).
		Get(&age10)
	assert.NoError(t, err)
	assert.Equal(t, true, has)
	assert.EqualValues(t, 28, age10)

	var id sql.NullInt64
	has, err = testEngine.Table("get_var").Cols("id").Get(&id)
	assert.NoError(t, err)
	assert.Equal(t, true, has)
	assert.Equal(t, true, id.Valid)
	assert.EqualValues(t, data.Id, id.Int64)

	var msgNull sql.NullString
	has, err = testEngine.Table("get_var").Cols("msg").Get(&msgNull)
	assert.NoError(t, err)
	assert.Equal(t, true, has)
	assert.Equal(t, true, msgNull.Valid)
	assert.EqualValues(t, data.Msg, msgNull.String)

	var nullMoney sql.NullFloat64
	has, err = testEngine.Table("get_var").Cols("money").Get(&nullMoney)
	assert.NoError(t, err)
	assert.Equal(t, true, has)
	assert.Equal(t, true, nullMoney.Valid)
	assert.EqualValues(t, data.Money, nullMoney.Float64)

	var money float64
	has, err = testEngine.Table("get_var").Cols("money").Get(&money)
	assert.NoError(t, err)
	assert.Equal(t, true, has)
	assert.Equal(t, "1.5", fmt.Sprintf("%.1f", money))

	var money2 float64
	if testEngine.Dialect().URI().DBType == schemas.MSSQL {
		has, err = testEngine.SQL("SELECT TOP 1 `money` FROM " + testEngine.Quote(testEngine.TableName("get_var", true))).Get(&money2)
	} else {
		has, err = testEngine.SQL("SELECT `money` FROM " + testEngine.Quote(testEngine.TableName("get_var", true)) + " LIMIT 1").Get(&money2)
	}
	assert.NoError(t, err)
	assert.Equal(t, true, has)
	assert.Equal(t, "1.5", fmt.Sprintf("%.1f", money2))

	var money3 float64
	has, err = testEngine.SQL("SELECT `money` FROM " + testEngine.Quote(testEngine.TableName("get_var", true)) + " WHERE `money` > 20").Get(&money3)
	assert.NoError(t, err)
	assert.Equal(t, false, has)

	valuesString := make(map[string]string)
	has, err = testEngine.Table("get_var").Get(&valuesString)
	assert.NoError(t, err)
	assert.Equal(t, true, has)
	assert.Equal(t, 5, len(valuesString))
	assert.Equal(t, "1", valuesString["id"])
	assert.Equal(t, "hi", valuesString["msg"])
	assert.Equal(t, "28", valuesString["age"])
	assert.Equal(t, "1.5", valuesString["money"])

	// for mymysql driver, any will be []byte, so ignore it currently
	if testEngine.DriverName() != "mymysql" {
		valuesInter := make(map[string]any)
		has, err = testEngine.Table("get_var").Where("`id` = ?", 1).Select("*").Get(&valuesInter)
		assert.NoError(t, err)
		assert.Equal(t, true, has)
		assert.Equal(t, 5, len(valuesInter))
		assert.EqualValues(t, 1, valuesInter["id"])
		assert.Equal(t, "hi", fmt.Sprintf("%s", valuesInter["msg"]))
		assert.EqualValues(t, 28, valuesInter["age"])
		assert.Equal(t, "1.5", fmt.Sprintf("%v", valuesInter["money"]))
	}

	valuesSliceString := make([]string, 5)
	has, err = testEngine.Table("get_var").Get(&valuesSliceString)
	assert.NoError(t, err)
	assert.Equal(t, true, has)
	assert.Equal(t, "1", valuesSliceString[0])
	assert.Equal(t, "hi", valuesSliceString[1])
	assert.Equal(t, "28", valuesSliceString[2])
	assert.Equal(t, "1.5", valuesSliceString[3])

	valuesSliceInter := make([]any, 5)
	has, err = testEngine.Table("get_var").Get(&valuesSliceInter)
	assert.NoError(t, err)
	assert.Equal(t, true, has)

	v1, err := convert.AsInt64(valuesSliceInter[0])
	assert.NoError(t, err)
	assert.EqualValues(t, 1, v1)

	assert.Equal(t, "hi", fmt.Sprintf("%s", valuesSliceInter[1]))

	v3, err := convert.AsInt64(valuesSliceInter[2])
	assert.NoError(t, err)
	assert.EqualValues(t, 28, v3)

	v4, err := convert.AsFloat64(valuesSliceInter[3])
	assert.NoError(t, err)
	assert.Equal(t, "1.5", fmt.Sprintf("%v", v4))
}

func TestGetOrError(t *testing.T) {
	assert.NoError(t, PrepareEngine())

	type GetOrErrorTable struct {
		Id   int64  `xorm:"autoincr pk"`
		Name string `xorm:"varchar(20)"`
	}

	assert.NoError(t, testEngine.Sync(new(GetOrErrorTable)))

	data := GetOrErrorTable{
		Name: "exist",
	}
	_, err := testEngine.Insert(&data)
	assert.NoError(t, err)

	var got GetOrErrorTable
	err = testEngine.ID(data.Id).GetOrError(&got)
	assert.NoError(t, err)
	assert.Equal(t, data.Id, got.Id)
	assert.Equal(t, data.Name, got.Name)

	var missing GetOrErrorTable
	err = testEngine.ID(data.Id + 1).GetOrError(&missing)
	assert.ErrorIs(t, err, xorm.ErrNotExist)
}

func TestGetStruct(t *testing.T) {
	assert.NoError(t, PrepareEngine())

	type UserinfoGet struct {
		Uid   int `xorm:"pk autoincr"`
		IsMan bool
	}

	assert.NoError(t, testEngine.Sync(new(UserinfoGet)))

	session := testEngine.NewSession()
	defer session.Close()

	var err error
	if testEngine.Dialect().URI().DBType == schemas.MSSQL {
		err = session.Begin()
		assert.NoError(t, err)
		_, err = session.Exec("SET IDENTITY_INSERT `userinfo_get` ON")
		assert.NoError(t, err)
	}
	cnt, err := session.Insert(&UserinfoGet{Uid: 2})
	assert.NoError(t, err)
	assert.EqualValues(t, 1, cnt)
	if testEngine.Dialect().URI().DBType == schemas.MSSQL {
		err = session.Commit()
		assert.NoError(t, err)
	}

	user := UserinfoGet{Uid: 2}
	has, err := testEngine.Get(&user)
	assert.NoError(t, err)
	assert.True(t, has)

	type NoIdUser struct {
		User   string `xorm:"unique"`
		Remain int64
		Total  int64
	}

	assert.NoError(t, testEngine.Sync(&NoIdUser{}))

	userCol := testEngine.GetColumnMapper().Obj2Table("User")
	_, err = testEngine.Where("`"+userCol+"` = ?", "xlw").Delete(&NoIdUser{})
	assert.NoError(t, err)

	cnt, err = testEngine.Insert(&NoIdUser{"xlw", 20, 100})
	assert.NoError(t, err)
	assert.EqualValues(t, 1, cnt)

	noIdUser := new(NoIdUser)
	has, err = testEngine.Where("`"+userCol+"` = ?", "xlw").Get(noIdUser)
	assert.NoError(t, err)
	assert.True(t, has)
}

func TestGetSlice(t *testing.T) {
	assert.NoError(t, PrepareEngine())

	type UserinfoSlice struct {
		Uid   int `xorm:"pk autoincr"`
		IsMan bool
	}

	assertSync(t, new(UserinfoSlice))

	var users []UserinfoSlice
	has, err := testEngine.Get(&users)
	assert.False(t, has)
	assert.Error(t, err)
}

func TestGetMap(t *testing.T) {
	assert.NoError(t, PrepareEngine())

	if testEngine.Dialect().Features().AutoincrMode == dialects.SequenceAutoincrMode {
		t.SkipNow()
		return
	}

	type UserinfoMap struct {
		Uid   int `xorm:"pk autoincr"`
		IsMan bool
	}

	assertSync(t, new(UserinfoMap))

	tableName := testEngine.Quote(testEngine.TableName("userinfo_map", true))
	_, err := testEngine.Exec(fmt.Sprintf("INSERT INTO %s (`is_man`) VALUES (NULL)", tableName))
	assert.NoError(t, err)

	valuesString := make(map[string]string)
	has, err := testEngine.Table("userinfo_map").Get(&valuesString)
	assert.NoError(t, err)
	assert.Equal(t, true, has)
	assert.Equal(t, 2, len(valuesString))
	assert.Equal(t, "1", valuesString["uid"])
	assert.Equal(t, "", valuesString["is_man"])
}

func TestGetError(t *testing.T) {
	assert.NoError(t, PrepareEngine())

	type GetError struct {
		Uid   int `xorm:"pk autoincr"`
		IsMan bool
	}

	assertSync(t, new(GetError))

	info := new(GetError)
	has, err := testEngine.Get(&info)
	assert.False(t, has)
	assert.Error(t, err)

	has, err = testEngine.Get(info)
	assert.False(t, has)
	assert.NoError(t, err)
}

func TestJSONString(t *testing.T) {
	assert.NoError(t, PrepareEngine())

	type JsonString struct {
		Id      int64
		Content string `xorm:"json"`
	}
	type JsonJson struct {
		Id      int64
		Content []string `xorm:"json"`
	}

	assertSync(t, new(JsonJson))

	_, err := testEngine.Insert(&JsonJson{
		Content: []string{"1", "2"},
	})
	assert.NoError(t, err)

	var js JsonString
	has, err := testEngine.Table("json_json").Get(&js)
	assert.NoError(t, err)
	assert.True(t, has)
	assert.EqualValues(t, 1, js.Id)
	assert.True(t, `["1","2"]` == js.Content || `["1", "2"]` == js.Content)

	var jss []JsonString
	err = testEngine.Table("json_json").Find(&jss)
	assert.NoError(t, err)
	assert.EqualValues(t, 1, len(jss))
	assert.True(t, `["1","2"]` == jss[0].Content || `["1", "2"]` == jss[0].Content)

	type JsonAnonymousStruct struct {
		Id         int64
		JsonString `xorm:"'json_string' JSON LONGTEXT"`
	}

	assertSync(t, new(JsonAnonymousStruct))

	_, err = testEngine.Insert(&JsonAnonymousStruct{
		JsonString: JsonString{
			Content: "1",
		},
	})
	assert.NoError(t, err)

	var jas JsonAnonymousStruct
	has, err = testEngine.Get(&jas)
	assert.NoError(t, err)
	assert.True(t, has)
	assert.EqualValues(t, 1, jas.Id)
	assert.EqualValues(t, "1", jas.Content)

	var jass []JsonAnonymousStruct
	err = testEngine.Find(&jass)
	assert.NoError(t, err)
	assert.EqualValues(t, 1, len(jass))
	assert.EqualValues(t, "1", jass[0].Content)

	type JsonStruct struct {
		Id   int64
		JSON JsonString `xorm:"'json_string' JSON LONGTEXT"`
	}

	assertSync(t, new(JsonStruct))

	_, err = testEngine.Insert(&JsonStruct{
		JSON: JsonString{
			Content: "2",
		},
	})
	assert.NoError(t, err)

	var jst JsonStruct
	has, err = testEngine.Get(&jst)
	assert.NoError(t, err)
	assert.True(t, has)
	assert.EqualValues(t, 1, jst.Id)
	assert.EqualValues(t, "2", jst.JSON.Content)

	var jsts []JsonStruct
	err = testEngine.Find(&jsts)
	assert.NoError(t, err)
	assert.EqualValues(t, 1, len(jsts))
	assert.EqualValues(t, "2", jsts[0].JSON.Content)
}

func TestGetActionMapping(t *testing.T) {
	assert.NoError(t, PrepareEngine())

	type ActionMapping struct {
		ActionId    string `xorm:"pk"`
		ActionName  string `xorm:"index"`
		ScriptId    string `xorm:"unique"`
		RollbackId  string `xorm:"unique"`
		Env         string
		Tags        string
		Description string
		UpdateTime  time.Time `xorm:"updated"`
		DeleteTime  time.Time `xorm:"deleted"`
	}

	assertSync(t, new(ActionMapping))

	_, err := testEngine.Insert(&ActionMapping{
		ActionId: "1",
		ScriptId: "2",
	})
	assert.NoError(t, err)

	valuesSlice := make([]string, 2)
	has, err := testEngine.Table(new(ActionMapping)).
		Cols("script_id", "rollback_id").
		ID("1").Get(&valuesSlice)
	assert.NoError(t, err)
	assert.True(t, has)
	assert.EqualValues(t, "2", valuesSlice[0])
	assert.EqualValues(t, "", valuesSlice[1])
}

func TestGetDeletedColumnForNonStruct(t *testing.T) {
	assert.NoError(t, PrepareEngine())

	type DeletedGetStruct struct {
		Id        int64 `xorm:"pk autoincr"`
		Name      string
		DeletedAt time.Time `xorm:"deleted"`
	}

	assertSync(t, new(DeletedGetStruct))

	bean := &DeletedGetStruct{Name: "first"}
	_, err := testEngine.Insert(bean)
	assert.NoError(t, err)

	_, err = testEngine.ID(bean.Id).Delete(new(DeletedGetStruct))
	assert.NoError(t, err)

	var id int64
	has, err := testEngine.Table(new(DeletedGetStruct)).Select("id").Get(&id)
	assert.NoError(t, err)
	assert.False(t, has)
	assert.EqualValues(t, 0, id)

	var ids []int64
	err = testEngine.Table(new(DeletedGetStruct)).Select("id").Limit(10).Find(&ids)
	assert.NoError(t, err)
	assert.Empty(t, ids)
}

func TestGetStructId(t *testing.T) {
	type TestGetStruct struct {
		Id int64
	}

	assert.NoError(t, PrepareEngine())
	assertSync(t, new(TestGetStruct))

	_, err := testEngine.Insert(&TestGetStruct{})
	assert.NoError(t, err)
	_, err = testEngine.Insert(&TestGetStruct{})
	assert.NoError(t, err)

	type maxidst struct {
		Id int64
	}

	// var id int64
	var maxid maxidst
	sql := "select max(`id`) as id from " + testEngine.Quote(testEngine.TableName(&TestGetStruct{}, true))
	has, err := testEngine.SQL(sql).Get(&maxid)
	assert.NoError(t, err)
	assert.True(t, has)
	assert.EqualValues(t, 2, maxid.Id)
}

func TestContextGet(t *testing.T) {
	type ContextGetStruct struct {
		Id   int64
		Name string
	}

	assert.NoError(t, PrepareEngine())
	assertSync(t, new(ContextGetStruct))

	_, err := testEngine.Insert(&ContextGetStruct{Name: "1"})
	assert.NoError(t, err)

	sess := testEngine.NewSession()
	defer sess.Close()

	context := contexts.NewMemoryContextCache()

	var c2 ContextGetStruct
	has, err := sess.ID(1).NoCache().ContextCache(context).Get(&c2)
	assert.NoError(t, err)
	assert.True(t, has)
	assert.EqualValues(t, 1, c2.Id)
	assert.EqualValues(t, "1", c2.Name)
	sql, args := sess.LastSQL()
	assert.True(t, len(sql) > 0)
	assert.True(t, len(args) > 0)

	var c3 ContextGetStruct
	has, err = sess.ID(1).NoCache().ContextCache(context).Get(&c3)
	assert.NoError(t, err)
	assert.True(t, has)
	assert.EqualValues(t, 1, c3.Id)
	assert.EqualValues(t, "1", c3.Name)
	sql, args = sess.LastSQL()
	assert.True(t, len(sql) == 0)
	assert.True(t, len(args) == 0)
}

func TestContextGet2(t *testing.T) {
	type ContextGetStruct2 struct {
		Id   int64
		Name string
	}

	assert.NoError(t, PrepareEngine())
	assertSync(t, new(ContextGetStruct2))

	_, err := testEngine.Insert(&ContextGetStruct2{Name: "1"})
	assert.NoError(t, err)

	context := contexts.NewMemoryContextCache()

	var c2 ContextGetStruct2
	has, err := testEngine.ID(1).NoCache().ContextCache(context).Get(&c2)
	assert.NoError(t, err)
	assert.True(t, has)
	assert.EqualValues(t, 1, c2.Id)
	assert.EqualValues(t, "1", c2.Name)

	var c3 ContextGetStruct2
	has, err = testEngine.ID(1).NoCache().ContextCache(context).Get(&c3)
	assert.NoError(t, err)
	assert.True(t, has)
	assert.EqualValues(t, 1, c3.Id)
	assert.EqualValues(t, "1", c3.Name)
}

type GetCustomTableInterface interface {
	TableName() string
}

type MyGetCustomTableImpletation struct {
	Id   int64  `json:"id"`
	Name string `json:"name"`
}

const getCustomTableName = "GetCustomTableInterface"

func (MyGetCustomTableImpletation) TableName() string {
	return getCustomTableName
}

func TestGetCustomTableInterface(t *testing.T) {
	assert.NoError(t, PrepareEngine())
	assert.NoError(t, testEngine.Table(getCustomTableName).Sync(new(MyGetCustomTableImpletation)))

	exist, err := testEngine.IsTableExist(getCustomTableName)
	assert.NoError(t, err)
	assert.True(t, exist)

	_, err = testEngine.Insert(&MyGetCustomTableImpletation{
		Name: "xlw",
	})
	assert.NoError(t, err)

	var c GetCustomTableInterface = new(MyGetCustomTableImpletation)
	has, err := testEngine.Get(c)
	assert.NoError(t, err)
	assert.True(t, has)
}

func TestGetNullVar(t *testing.T) {
	type TestGetNullVarStruct struct {
		Id   int64
		Name string
		Age  int
	}

	assert.NoError(t, PrepareEngine())
	assertSync(t, new(TestGetNullVarStruct))

	if testEngine.Dialect().Features().AutoincrMode == dialects.SequenceAutoincrMode {
		t.SkipNow()
		return
	}

	affected, err := testEngine.Exec("insert into " + testEngine.Quote(testEngine.TableName(new(TestGetNullVarStruct), true)) + " (`name`,`age`) values (null,null)")
	assert.NoError(t, err)
	a, _ := affected.RowsAffected()
	assert.EqualValues(t, 1, a)

	var name string
	has, err := testEngine.Table(new(TestGetNullVarStruct)).Where("`id` = ?", 1).Cols("name").Get(&name)
	assert.NoError(t, err)
	assert.True(t, has)
	assert.EqualValues(t, "", name)

	var age int
	has, err = testEngine.Table(new(TestGetNullVarStruct)).Where("`id` = ?", 1).Cols("age").Get(&age)
	assert.NoError(t, err)
	assert.True(t, has)
	assert.EqualValues(t, 0, age)

	var age2 int8
	has, err = testEngine.Table(new(TestGetNullVarStruct)).Where("`id` = ?", 1).Cols("age").Get(&age2)
	assert.NoError(t, err)
	assert.True(t, has)
	assert.EqualValues(t, 0, age2)

	var age3 int16
	has, err = testEngine.Table(new(TestGetNullVarStruct)).Where("`id` = ?", 1).Cols("age").Get(&age3)
	assert.NoError(t, err)
	assert.True(t, has)
	assert.EqualValues(t, 0, age3)

	var age4 int32
	has, err = testEngine.Table(new(TestGetNullVarStruct)).Where("`id` = ?", 1).Cols("age").Get(&age4)
	assert.NoError(t, err)
	assert.True(t, has)
	assert.EqualValues(t, 0, age4)

	var age5 int64
	has, err = testEngine.Table(new(TestGetNullVarStruct)).Where("`id` = ?", 1).Cols("age").Get(&age5)
	assert.NoError(t, err)
	assert.True(t, has)
	assert.EqualValues(t, 0, age5)

	var age6 uint
	has, err = testEngine.Table(new(TestGetNullVarStruct)).Where("`id` = ?", 1).Cols("age").Get(&age6)
	assert.NoError(t, err)
	assert.True(t, has)
	assert.EqualValues(t, 0, age6)

	var age7 uint8
	has, err = testEngine.Table(new(TestGetNullVarStruct)).Where("`id` = ?", 1).Cols("age").Get(&age7)
	assert.NoError(t, err)
	assert.True(t, has)
	assert.EqualValues(t, 0, age7)

	var age8 int16
	has, err = testEngine.Table(new(TestGetNullVarStruct)).Where("`id` = ?", 1).Cols("age").Get(&age8)
	assert.NoError(t, err)
	assert.True(t, has)
	assert.EqualValues(t, 0, age8)

	var age9 int32
	has, err = testEngine.Table(new(TestGetNullVarStruct)).Where("`id` = ?", 1).Cols("age").Get(&age9)
	assert.NoError(t, err)
	assert.True(t, has)
	assert.EqualValues(t, 0, age9)

	var age10 int64
	has, err = testEngine.Table(new(TestGetNullVarStruct)).Where("`id` = ?", 1).Cols("age").Get(&age10)
	assert.NoError(t, err)
	assert.True(t, has)
	assert.EqualValues(t, 0, age10)
}

func TestCustomTypes(t *testing.T) {
	type MyInt int
	type MyString string

	type TestCustomizeStruct struct {
		Id   int64
		Name MyString
		Age  MyInt
	}

	assert.NoError(t, PrepareEngine())
	assertSync(t, new(TestCustomizeStruct))

	s := TestCustomizeStruct{
		Name: "test",
		Age:  32,
	}
	_, err := testEngine.Insert(&s)
	assert.NoError(t, err)
	assert.EqualValues(t, 1, s.Id)

	var name MyString
	has, err := testEngine.Table(new(TestCustomizeStruct)).ID(s.Id).Cols("name").Get(&name)
	assert.NoError(t, err)
	assert.True(t, has)
	assert.EqualValues(t, "test", name)

	var age MyInt
	has, err = testEngine.Table(new(TestCustomizeStruct)).ID(s.Id).Select("`age`").Get(&age)
	assert.NoError(t, err)
	assert.True(t, has)
	assert.EqualValues(t, 32, age)
}

func TestGetViaMapCond(t *testing.T) {
	type GetViaMapCond struct {
		Id       int64
		Platform int
		Index    int
	}

	assert.NoError(t, PrepareEngine())
	assertSync(t, new(GetViaMapCond))

	var (
		r           GetViaMapCond
		platformStr = colMapper.Obj2Table("Platform")
		indexStr    = colMapper.Obj2Table("Index")
		query       = map[string]any{
			platformStr: 1,
			indexStr:    1,
		}
	)

	has, err := testEngine.Where(query).Get(&r)
	assert.NoError(t, err)
	assert.False(t, has)
}

func TestGetNil(t *testing.T) {
	type GetNil struct {
		Id int64
	}

	assert.NoError(t, PrepareEngine())
	assertSync(t, new(GetNil))

	var gn *GetNil
	has, err := testEngine.Get(gn)
	assert.True(t, errors.Is(err, xorm.ErrObjectIsNil))
	assert.False(t, has)
}

func TestGetBigFloat(t *testing.T) {
	type GetBigFloat struct {
		Id    int64
		Money *big.Float `xorm:"numeric(22,2)"`
	}

	assert.NoError(t, PrepareEngine())
	assertSync(t, new(GetBigFloat))

	{
		gf := GetBigFloat{
			Money: big.NewFloat(999999.99),
		}
		_, err := testEngine.Insert(&gf)
		assert.NoError(t, err)

		var m big.Float
		has, err := testEngine.Table("get_big_float").Cols("money").Where("`id`=?", gf.Id).Get(&m)
		assert.NoError(t, err)
		assert.True(t, has)
		assert.True(t, m.String() == gf.Money.String(), "%v != %v", m.String(), gf.Money.String())
		// fmt.Println(m.Cmp(gf.Money))
		// assert.True(t, m.Cmp(gf.Money) == 0, "%v != %v", m.String(), gf.Money.String())
	}

	type GetBigFloat2 struct {
		Id     int64
		Money  *big.Float `xorm:"decimal(22,2)"`
		Money2 big.Float  `xorm:"decimal(22,2)"`
	}

	assert.NoError(t, PrepareEngine())
	assertSync(t, new(GetBigFloat2))

	{
		gf2 := GetBigFloat2{
			Money:  big.NewFloat(9999999.99),
			Money2: *big.NewFloat(99.99),
		}
		_, err := testEngine.Insert(&gf2)
		assert.NoError(t, err)

		var m2 big.Float
		has, err := testEngine.Table("get_big_float2").Cols("money").Where("`id`=?", gf2.Id).Get(&m2)
		assert.NoError(t, err)
		assert.True(t, has)
		assert.True(t, m2.String() == gf2.Money.String(), "%v != %v", m2.String(), gf2.Money.String())
		// fmt.Println(m.Cmp(gf.Money))
		// assert.True(t, m.Cmp(gf.Money) == 0, "%v != %v", m.String(), gf.Money.String())

		var gf3 GetBigFloat2
		has, err = testEngine.ID(gf2.Id).Get(&gf3)
		assert.NoError(t, err)
		assert.True(t, has)
		assert.True(t, gf3.Money.String() == gf2.Money.String(), "%v != %v", gf3.Money.String(), gf2.Money.String())
		assert.True(t, gf3.Money2.String() == gf2.Money2.String(), "%v != %v", gf3.Money2.String(), gf2.Money2.String())

		var gfs []GetBigFloat2
		err = testEngine.Find(&gfs)
		assert.NoError(t, err)
		assert.EqualValues(t, 1, len(gfs))
		assert.True(t, gfs[0].Money.String() == gf2.Money.String(), "%v != %v", gfs[0].Money.String(), gf2.Money.String())
		assert.True(t, gfs[0].Money2.String() == gf2.Money2.String(), "%v != %v", gfs[0].Money2.String(), gf2.Money2.String())
	}
}

func TestGetDecimal(t *testing.T) {
	type GetDecimal struct {
		Id    int64
		Money decimal.Decimal `xorm:"decimal(22,2)"`
	}

	assert.NoError(t, PrepareEngine())
	assertSync(t, new(GetDecimal))

	{
		gf := GetDecimal{
			Money: decimal.NewFromFloat(999999.99),
		}
		_, err := testEngine.Insert(&gf)
		assert.NoError(t, err)

		var m decimal.Decimal
		has, err := testEngine.Table("get_decimal").Cols("money").Where("`id`=?", gf.Id).Get(&m)
		assert.NoError(t, err)
		assert.True(t, has)
		assert.True(t, m.String() == gf.Money.String(), "%v != %v", m.String(), gf.Money.String())
		// fmt.Println(m.Cmp(gf.Money))
		// assert.True(t, m.Cmp(gf.Money) == 0, "%v != %v", m.String(), gf.Money.String())
	}

	type GetDecimal2 struct {
		Id    int64
		Money *decimal.Decimal `xorm:"decimal(22,2)"`
	}

	assert.NoError(t, PrepareEngine())
	assertSync(t, new(GetDecimal2))

	{
		v := decimal.NewFromFloat(999999.99)
		gf := GetDecimal2{
			Money: &v,
		}
		_, err := testEngine.Insert(&gf)
		assert.NoError(t, err)

		var m decimal.Decimal
		has, err := testEngine.Table("get_decimal2").Cols("money").Where("`id`=?", gf.Id).Get(&m)
		assert.NoError(t, err)
		assert.True(t, has)
		assert.True(t, m.String() == gf.Money.String(), "%v != %v", m.String(), gf.Money.String())
		// fmt.Println(m.Cmp(gf.Money))
		// assert.True(t, m.Cmp(gf.Money) == 0, "%v != %v", m.String(), gf.Money.String())
	}
}

func TestGetTime(t *testing.T) {
	type GetTimeStruct struct {
		Id         int64
		CreateTime time.Time
	}

	assert.NoError(t, PrepareEngine())
	assertSync(t, new(GetTimeStruct))

	gts := GetTimeStruct{
		CreateTime: time.Now().In(testEngine.GetTZLocation()),
	}
	_, err := testEngine.Insert(&gts)
	assert.NoError(t, err)

	var gn time.Time
	has, err := testEngine.Table("get_time_struct").Cols(colMapper.Obj2Table("CreateTime")).Get(&gn)
	assert.NoError(t, err)
	assert.True(t, has)
	assert.EqualValues(t, gts.CreateTime.Format(time.RFC3339), gn.Format(time.RFC3339))
}

func TestGetVars(t *testing.T) {
	type GetVars struct {
		Id   int64
		Name string
		Age  int
	}

	assert.NoError(t, PrepareEngine())
	assertSync(t, new(GetVars))

	_, err := testEngine.Insert(&GetVars{
		Name: "xlw",
		Age:  42,
	})
	assert.NoError(t, err)

	var name string
	var age int
	has, err := testEngine.Table(new(GetVars)).Cols("name", "age").Get(&name, &age)
	assert.NoError(t, err)
	assert.True(t, has)
	assert.EqualValues(t, "xlw", name)
	assert.EqualValues(t, 42, age)
}

func TestGetWithPrepare(t *testing.T) {
	type GetVarsWithPrepare struct {
		Id   int64
		Name string
		Age  int
	}

	assert.NoError(t, PrepareEngine())
	assertSync(t, new(GetVarsWithPrepare))

	_, err := testEngine.Insert(&GetVarsWithPrepare{
		Name: "xlw",
		Age:  42,
	})
	assert.NoError(t, err)

	var v1 GetVarsWithPrepare
	has, err := testEngine.Prepare().ID(1).Get(&v1)
	assert.NoError(t, err)
	assert.True(t, has)
	assert.EqualValues(t, "xlw", v1.Name)
	assert.EqualValues(t, 42, v1.Age)

	sess := testEngine.NewSession()
	defer sess.Close()

	var v2 GetVarsWithPrepare
	has, err = sess.Prepare().ID(1).Get(&v2)
	assert.NoError(t, err)
	assert.True(t, has)
	assert.EqualValues(t, "xlw", v2.Name)
	assert.EqualValues(t, 42, v2.Age)

	var v3 GetVarsWithPrepare
	has, err = sess.Prepare().ID(1).Get(&v3)
	assert.NoError(t, err)
	assert.True(t, has)
	assert.EqualValues(t, "xlw", v3.Name)
	assert.EqualValues(t, 42, v3.Age)

	err = sess.Begin()
	assert.NoError(t, err)

	cnt, err := sess.Prepare().Insert(&GetVarsWithPrepare{
		Name: "xlw2",
		Age:  12,
	})
	assert.NoError(t, err)
	assert.EqualValues(t, 1, cnt)

	cnt, err = sess.Prepare().Insert(&GetVarsWithPrepare{
		Name: "xlw3",
		Age:  13,
	})
	assert.NoError(t, err)
	assert.EqualValues(t, 1, cnt)

	err = sess.Commit()
	assert.NoError(t, err)
}

func TestGetBytesVars(t *testing.T) {
	type GetBytesVars struct {
		Id     int64
		Bytes1 []byte
		Bytes2 []byte
	}

	assert.NoError(t, PrepareEngine())
	assertSync(t, new(GetBytesVars))

	_, err := testEngine.Insert([]GetBytesVars{
		{
			Bytes1: []byte("bytes1"),
			Bytes2: []byte("bytes2"),
		},
		{
			Bytes1: []byte("bytes1-1"),
			Bytes2: []byte("bytes2-2"),
		},
	})
	assert.NoError(t, err)

	var gbv GetBytesVars
	has, err := testEngine.Asc("id").Get(&gbv)
	assert.NoError(t, err)
	assert.True(t, has)
	assert.EqualValues(t, []byte("bytes1"), gbv.Bytes1)
	assert.EqualValues(t, []byte("bytes2"), gbv.Bytes2)

	has, err = testEngine.Desc("id").NoAutoCondition().Get(&gbv)
	assert.NoError(t, err)
	assert.True(t, has)
	assert.EqualValues(t, []byte("bytes1-1"), gbv.Bytes1)
	assert.EqualValues(t, []byte("bytes2-2"), gbv.Bytes2)

	type MyID int64
	var myID MyID

	has, err = testEngine.Table("get_bytes_vars").Select("id").Desc("id").Get(&myID)
	assert.NoError(t, err)
	assert.True(t, has)
	assert.EqualValues(t, gbv.Id, myID)
}

func TestGetStructWithDecimal(t *testing.T) {
	assert.NoError(t, PrepareEngine())
	type ProductGet struct {
		Uid   int             `xorm:"pk autoincr"`
		Price decimal.Decimal `xorm:"decimal(35,30)"`
	}
	assert.NoError(t, testEngine.Sync(new(ProductGet)))

	session := testEngine.NewSession()
	defer session.Close()
	var err error
	if testEngine.Dialect().URI().DBType == schemas.MSSQL {
		err = session.Begin()
		assert.NoError(t, err)
		_, err = session.Exec("SET IDENTITY_INSERT `product_get` ON")
		assert.NoError(t, err)
	}
	cnt, err := session.Insert(&ProductGet{Uid: 2, Price: decimal.NewFromFloat(0.8)})
	assert.NoError(t, err)
	assert.EqualValues(t, 1, cnt)
	cnt, err = session.Insert(&ProductGet{Uid: 3, Price: decimal.NewFromFloat(1.5)})
	assert.NoError(t, err)
	assert.EqualValues(t, 1, cnt)
	if testEngine.Dialect().URI().DBType == schemas.MSSQL {
		err = session.Commit()
		assert.NoError(t, err)
	}

	// Expected SQL: SELECT `uid`, `price` FROM `product_get` WHERE price>? LIMIT 1
	// Wrong SQL: SELECT `uid`, `price` FROM `product_get` WHERE price>? AND `price`=? LIMIT 1
	product := new(ProductGet)
	exist, err := testEngine.Where(builder.Gt{"price": decimal.NewFromInt(1)}).Get(product)
	assert.NoError(t, err)
	assert.True(t, exist)
}
