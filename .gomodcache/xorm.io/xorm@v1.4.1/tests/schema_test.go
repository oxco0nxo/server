// Copyright 2017 The Xorm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package tests

import (
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"xorm.io/xorm"
	"xorm.io/xorm/schemas"
)

func TestStoreEngine(t *testing.T) {
	assert.NoError(t, PrepareEngine())

	assert.NoError(t, testEngine.DropTables("user_store_engine"))

	type UserinfoStoreEngine struct {
		Id   int64
		Name string
	}

	assert.NoError(t, testEngine.StoreEngine("InnoDB").Table("user_store_engine").CreateTable(&UserinfoStoreEngine{}))
}

func TestCreateTable(t *testing.T) {
	assert.NoError(t, PrepareEngine())

	assert.NoError(t, testEngine.DropTables("user_user"))

	type UserinfoCreateTable struct {
		Id   int64
		Name string
	}

	assert.NoError(t, testEngine.Table("user_user").CreateTable(&UserinfoCreateTable{}))
}

func TestCreateTable2(t *testing.T) {
	type BaseModelLogicalDel struct {
		Id        string     `xorm:"varchar(46) pk"`
		CreatedAt time.Time  `xorm:"created"`
		UpdatedAt time.Time  `xorm:"updated"`
		DeletedAt *time.Time `xorm:"deleted"`
	}
	type TestPerson struct {
		BaseModelLogicalDel `xorm:"extends"`
		UserId              string `xorm:"varchar(46) notnull"`
		PersonId            string `xorm:"varchar(46) notnull"`
		Star                bool
		SortNo              int
		DispName            string `xorm:"varchar(100)"`
		FirstName           string
		LastName            string
		FirstNameKana       string
		LastNameKana        string
		BirthYear           *int
		BirthMonth          *int
		BirthDay            *int
		ImageId             string `xorm:"varchar(46)"`
		ImageDefaultId      string `xorm:"varchar(46)"`
		UserText            string `xorm:"varchar(2000)"`
		GenderId            *int
		At1                 string `xorm:"varchar(10)"`
		At1Rate             int
		At2                 string `xorm:"varchar(10)"`
		At2Rate             int
		At3                 string `xorm:"varchar(10)"`
		At3Rate             int
		At4                 string `xorm:"varchar(10)"`
		At4Rate             int
		At5                 string `xorm:"varchar(10)"`
		At5Rate             int
		At6                 string `xorm:"varchar(10)"`
		At6Rate             int
	}

	assert.NoError(t, PrepareEngine())

	tb1, err := testEngine.TableInfo(TestPerson{})
	assert.NoError(t, err)
	tb2, err := testEngine.TableInfo(new(TestPerson))
	assert.NoError(t, err)
	cols1, cols2 := tb1.ColumnsSeq(), tb2.ColumnsSeq()
	assert.EqualValues(t, len(cols1), len(cols2))
	for i, col := range cols1 {
		assert.EqualValues(t, col, cols2[i])
	}

	result, err := testEngine.IsTableExist(new(TestPerson))
	assert.NoError(t, err)
	if result {
		assert.NoError(t, testEngine.DropTables(new(TestPerson)))
	}

	assert.NoError(t, testEngine.CreateTables(new(TestPerson)))
	tables1, err := testEngine.DBMetas()
	assert.NoError(t, err)
	assert.Len(t, tables1, 1)
	assert.EqualValues(t, len(cols1), len(tables1[0].Columns()))

	result, err = testEngine.IsTableExist(new(TestPerson))
	assert.NoError(t, err)
	if result {
		assert.NoError(t, testEngine.DropTables(new(TestPerson)))
	}

	assert.NoError(t, testEngine.CreateTables(TestPerson{}))
	tables2, err := testEngine.DBMetas()
	assert.NoError(t, err)
	assert.Len(t, tables2, 1)
	assert.EqualValues(t, len(cols1), len(tables2[0].Columns()))
}

func TestCreateMultiTables(t *testing.T) {
	assert.NoError(t, PrepareEngine())

	session := testEngine.NewSession()
	defer session.Close()

	type UserinfoMultiTable struct {
		Id   int64
		Name string
	}

	user := &UserinfoMultiTable{}
	assert.NoError(t, session.Begin())

	for i := 0; i < 10; i++ {
		tableName := fmt.Sprintf("user_%v", i)

		assert.NoError(t, session.DropTable(tableName))

		assert.NoError(t, session.Table(tableName).CreateTable(user))
	}

	assert.NoError(t, session.Commit())
}

type SyncTable1 struct {
	Id   int64
	Name string
	Dev  int `xorm:"index"`
}

type SyncTable2 struct {
	Id     int64
	Name   string `xorm:"unique"`
	Number string `xorm:"index"`
	Dev    int
	Age    int
}

func (SyncTable2) TableName() string {
	return "sync_table1"
}

type SyncTable3 struct {
	Id     int64
	Name   string `xorm:"unique"`
	Number string `xorm:"index"`
	Dev    int
	Age    int
}

func (s *SyncTable3) TableName() string {
	return "sync_table1"
}

func TestSyncTable(t *testing.T) {
	assert.NoError(t, PrepareEngine())

	assert.NoError(t, testEngine.Sync(new(SyncTable1)))

	tables, err := testEngine.DBMetas()
	assert.NoError(t, err)
	assert.EqualValues(t, 1, len(tables))
	assert.EqualValues(t, "sync_table1", tables[0].Name)
	tableInfo, err := testEngine.TableInfo(new(SyncTable1))
	assert.NoError(t, err)
	assert.EqualValues(t, testEngine.Dialect().SQLType(tables[0].GetColumn("name")), testEngine.Dialect().SQLType(tableInfo.GetColumn("name")))

	assert.NoError(t, testEngine.Sync(new(SyncTable2)))

	tables, err = testEngine.DBMetas()
	assert.NoError(t, err)
	assert.EqualValues(t, 1, len(tables))
	assert.EqualValues(t, "sync_table1", tables[0].Name)
	tableInfo, err = testEngine.TableInfo(new(SyncTable2))
	assert.NoError(t, err)
	assert.EqualValues(t, testEngine.Dialect().SQLType(tables[0].GetColumn("name")), testEngine.Dialect().SQLType(tableInfo.GetColumn("name")))

	assert.NoError(t, testEngine.Sync(new(SyncTable3)))

	tables, err = testEngine.DBMetas()
	assert.NoError(t, err)
	assert.EqualValues(t, 1, len(tables))
	assert.EqualValues(t, "sync_table1", tables[0].Name)
	tableInfo, err = testEngine.TableInfo(new(SyncTable3))
	assert.NoError(t, err)
	assert.EqualValues(t, testEngine.Dialect().SQLType(tables[0].GetColumn("name")), testEngine.Dialect().SQLType(tableInfo.GetColumn("name")))
}

func TestSyncTable2(t *testing.T) {
	assert.NoError(t, PrepareEngine())

	assert.NoError(t, testEngine.Table("sync_tablex").Sync(new(SyncTable1)))

	tables, err := testEngine.DBMetas()
	assert.NoError(t, err)
	assert.EqualValues(t, 1, len(tables))
	assert.EqualValues(t, "sync_tablex", tables[0].Name)
	assert.EqualValues(t, 3, len(tables[0].Columns()))

	type SyncTable4 struct {
		SyncTable1 `xorm:"extends"`
		NewCol     string
	}

	assert.NoError(t, testEngine.Table("sync_tablex").Sync(new(SyncTable4)))
	tables, err = testEngine.DBMetas()
	assert.NoError(t, err)
	assert.EqualValues(t, 1, len(tables))
	assert.EqualValues(t, "sync_tablex", tables[0].Name)
	assert.EqualValues(t, 4, len(tables[0].Columns()))
	assert.EqualValues(t, colMapper.Obj2Table("NewCol"), tables[0].Columns()[3].Name)
}

func TestSyncTable3(t *testing.T) {
	type SyncTable5 struct {
		Id         int64
		Name       string
		Text       string   `xorm:"TEXT"`
		Char       byte     `xorm:"CHAR(1)"`
		TenChar    [10]byte `xorm:"CHAR(10)"`
		TenVarChar string   `xorm:"VARCHAR(10)"`
	}

	assert.NoError(t, PrepareEngine())

	assert.NoError(t, testEngine.Sync(new(SyncTable5)))

	tables, err := testEngine.DBMetas()
	assert.NoError(t, err)
	tableInfo, err := testEngine.TableInfo(new(SyncTable5))
	assert.NoError(t, err)
	assert.EqualValues(t, testEngine.Dialect().SQLType(tableInfo.GetColumn("name")), testEngine.Dialect().SQLType(tables[0].GetColumn("name")))
	/* Engine.DBMetas() returns the size of the column from the database but Engine.TableInfo() might not be able to guess the column size.
	For example using MySQL/MariaDB: when utf-8 charset is used, "`xorm:"TEXT(21846)`" creates a MEDIUMTEXT column not a TEXT column. */
	assert.True(t, testEngine.Dialect().SQLType(tables[0].GetColumn("text")) == testEngine.Dialect().SQLType(tableInfo.GetColumn("text")) || strings.HasPrefix(testEngine.Dialect().SQLType(tables[0].GetColumn("text")), testEngine.Dialect().SQLType(tableInfo.GetColumn("text"))+"("))
	assert.EqualValues(t, testEngine.Dialect().SQLType(tableInfo.GetColumn("char")), testEngine.Dialect().SQLType(tables[0].GetColumn("char")))
	assert.EqualValues(t, testEngine.Dialect().SQLType(tableInfo.GetColumn("ten_char")), testEngine.Dialect().SQLType(tables[0].GetColumn("ten_char")))
	assert.EqualValues(t, testEngine.Dialect().SQLType(tableInfo.GetColumn("ten_var_char")), testEngine.Dialect().SQLType(tables[0].GetColumn("ten_var_char")))

	if *doNVarcharTest {
		var oldDefaultVarchar string
		var oldDefaultChar string
		oldDefaultVarchar, *defaultVarchar = *defaultVarchar, "nvarchar"
		oldDefaultChar, *defaultChar = *defaultChar, "nchar"
		testEngine.Dialect().SetParams(map[string]string{
			"DEFAULT_VARCHAR": *defaultVarchar,
			"DEFAULT_CHAR":    *defaultChar,
		})
		defer func() {
			*defaultVarchar = oldDefaultVarchar
			*defaultChar = oldDefaultChar
			testEngine.Dialect().SetParams(map[string]string{
				"DEFAULT_VARCHAR": *defaultVarchar,
				"DEFAULT_CHAR":    *defaultChar,
			})
		}()
		assert.NoError(t, PrepareEngine())

		assert.NoError(t, testEngine.Sync(new(SyncTable5)))

		tables, err := testEngine.DBMetas()
		assert.NoError(t, err)
		tableInfo, err := testEngine.TableInfo(new(SyncTable5))
		assert.NoError(t, err)
		assert.EqualValues(t, testEngine.Dialect().SQLType(tableInfo.GetColumn("name")), testEngine.Dialect().SQLType(tables[0].GetColumn("name")))
		assert.EqualValues(t, testEngine.Dialect().SQLType(tableInfo.GetColumn("text")), testEngine.Dialect().SQLType(tables[0].GetColumn("text")))
		assert.EqualValues(t, testEngine.Dialect().SQLType(tableInfo.GetColumn("char")), testEngine.Dialect().SQLType(tables[0].GetColumn("char")))
		assert.EqualValues(t, testEngine.Dialect().SQLType(tableInfo.GetColumn("ten_char")), testEngine.Dialect().SQLType(tables[0].GetColumn("ten_char")))
		assert.EqualValues(t, testEngine.Dialect().SQLType(tableInfo.GetColumn("ten_var_char")), testEngine.Dialect().SQLType(tables[0].GetColumn("ten_var_char")))
	}
}

func TestSyncTable4(t *testing.T) {
	type SyncTable6 struct {
		Id  int64
		Qty float64 `xorm:"numeric(36,2)"`
	}

	assert.NoError(t, PrepareEngine())

	assert.NoError(t, testEngine.Sync(new(SyncTable6)))

	assert.NoError(t, testEngine.Sync(new(SyncTable6)))
}

func TestIsTableExist(t *testing.T) {
	assert.NoError(t, PrepareEngine())

	exist, err := testEngine.IsTableExist(new(CustomTableName))
	assert.NoError(t, err)
	assert.False(t, exist)

	assert.NoError(t, testEngine.CreateTables(new(CustomTableName)))

	exist, err = testEngine.IsTableExist(new(CustomTableName))
	assert.NoError(t, err)
	assert.True(t, exist)
}

func TestIsTableEmpty(t *testing.T) {
	assert.NoError(t, PrepareEngine())

	type NumericEmpty struct {
		Numeric float64 `xorm:"numeric(26,2)"`
	}

	type PictureEmpty struct {
		Id          int64
		Url         string `xorm:"unique"` // image's url
		Title       string
		Description string
		Created     time.Time `xorm:"created"`
		ILike       int
		PageView    int
		From_url    string
		Pre_url     string `xorm:"unique"` // pre view image's url
		Uid         int64
	}

	assert.NoError(t, testEngine.DropTables(&PictureEmpty{}, &NumericEmpty{}))

	assert.NoError(t, testEngine.Sync(new(PictureEmpty), new(NumericEmpty)))

	isEmpty, err := testEngine.IsTableEmpty(&PictureEmpty{})
	assert.NoError(t, err)
	assert.True(t, isEmpty)

	tbName := testEngine.GetTableMapper().Obj2Table("PictureEmpty")
	isEmpty, err = testEngine.IsTableEmpty(tbName)
	assert.NoError(t, err)
	assert.True(t, isEmpty)
}

type CustomTableName struct {
	Id   int64
	Name string
}

func (c *CustomTableName) TableName() string {
	return "customtablename"
}

func TestCustomTableName(t *testing.T) {
	assert.NoError(t, PrepareEngine())

	c := new(CustomTableName)
	assert.NoError(t, testEngine.DropTables(c))

	assert.NoError(t, testEngine.CreateTables(c))
}

type IndexOrUnique struct {
	Id        int64
	Index     int `xorm:"index"`
	Unique    int `xorm:"unique"`
	Group1    int `xorm:"index(ttt)"`
	Group2    int `xorm:"index(ttt)"`
	UniGroup1 int `xorm:"unique(lll)"`
	UniGroup2 int `xorm:"unique(lll)"`
}

func TestIndexAndUnique(t *testing.T) {
	assert.NoError(t, PrepareEngine())

	assert.NoError(t, testEngine.CreateTables(&IndexOrUnique{}))

	assert.NoError(t, testEngine.DropTables(&IndexOrUnique{}))

	assert.NoError(t, testEngine.CreateTables(&IndexOrUnique{}))

	assert.NoError(t, testEngine.CreateIndexes(&IndexOrUnique{}))

	assert.NoError(t, testEngine.CreateUniques(&IndexOrUnique{}))

	assert.NoError(t, testEngine.DropIndexes(&IndexOrUnique{}))
}

func TestMetaInfo(t *testing.T) {
	assert.NoError(t, PrepareEngine())
	assert.NoError(t, testEngine.Sync(new(CustomTableName), new(IndexOrUnique)))

	tables, err := testEngine.DBMetas()
	assert.NoError(t, err)
	assert.EqualValues(t, 2, len(tables))
	tableNames := []string{tables[0].Name, tables[1].Name}
	assert.Contains(t, tableNames, "customtablename")
	assert.Contains(t, tableNames, "index_or_unique")
}

func TestCharst(t *testing.T) {
	assert.NoError(t, PrepareEngine())

	err := testEngine.DropTables("user_charset")
	assert.NoError(t, err)

	err = testEngine.Charset("utf8").Table("user_charset").CreateTable(&Userinfo{})
	assert.NoError(t, err)
}

func TestSync2_1(t *testing.T) {
	type WxTest struct {
		Id                 int   `xorm:"not null pk autoincr INT(64)"`
		Passport_user_type int16 `xorm:"null int"`
		Id_delete          int8  `xorm:"null int default 1"`
	}

	assert.NoError(t, PrepareEngine())

	assert.NoError(t, testEngine.DropTables("wx_test"))
	assert.NoError(t, testEngine.Sync(new(WxTest)))
	assert.NoError(t, testEngine.Sync(new(WxTest)))
}

func TestUnique_1(t *testing.T) {
	type UserUnique struct {
		Id        int64
		UserName  string    `xorm:"unique varchar(25) not null"`
		Password  string    `xorm:"varchar(255) not null"`
		Admin     bool      `xorm:"not null"`
		CreatedAt time.Time `xorm:"created"`
		UpdatedAt time.Time `xorm:"updated"`
	}

	assert.NoError(t, PrepareEngine())

	assert.NoError(t, testEngine.DropTables("user_unique"))
	assert.NoError(t, testEngine.Sync(new(UserUnique)))

	assert.NoError(t, testEngine.DropTables("user_unique"))
	assert.NoError(t, testEngine.CreateTables(new(UserUnique)))
	assert.NoError(t, testEngine.CreateUniques(new(UserUnique)))
}

func TestSync2_2(t *testing.T) {
	type TestSync2Index struct {
		Id     int64
		UserId int64 `xorm:"index"`
	}

	assert.NoError(t, PrepareEngine())

	tableNames := make(map[string]bool)
	for i := 0; i < 10; i++ {
		tableName := fmt.Sprintf("test_sync2_index_%d", i)
		tableNames[tableName] = true
		assert.NoError(t, testEngine.Table(tableName).Sync(new(TestSync2Index)))

		exist, err := testEngine.IsTableExist(tableName)
		assert.NoError(t, err)
		assert.True(t, exist)
	}

	tables, err := testEngine.DBMetas()
	assert.NoError(t, err)

	for _, table := range tables {
		assert.True(t, tableNames[table.Name])
	}
}

func TestSync2_Default(t *testing.T) {
	type TestSync2Default struct {
		Id       int64
		UserId   int64  `xorm:"default(1)"`
		IsMember bool   `xorm:"default(true)"`
		Name     string `xorm:"default('my_name')"`
	}

	assert.NoError(t, PrepareEngine())
	assertSync(t, new(TestSync2Default))
	assert.NoError(t, testEngine.Sync(new(TestSync2Default)))
}

func TestSync2_Default2(t *testing.T) {
	type TestSync2Default2 struct {
		Id       int64
		UserId   int64  `xorm:"default(1)"`
		IsMember bool   `xorm:"default(true)"`
		Name     string `xorm:"default('')"`
	}

	assert.NoError(t, PrepareEngine())
	assertSync(t, new(TestSync2Default2))
	assert.NoError(t, testEngine.Sync(new(TestSync2Default2)))
	assert.NoError(t, testEngine.Sync(new(TestSync2Default2)))
	assert.NoError(t, testEngine.Sync(new(TestSync2Default2)))

	assert.NoError(t, testEngine.Sync(new(TestSync2Default2)))
	assert.NoError(t, testEngine.Sync(new(TestSync2Default2)))
	assert.NoError(t, testEngine.Sync(new(TestSync2Default2)))
}

func TestModifyColum(t *testing.T) {
	// Since SQLITE don't support modify column SQL, currrently just ignore
	if testEngine.Dialect().URI().DBType == schemas.SQLITE {
		return
	}
	type TestModifyColumn struct {
		Id       int64
		UserId   int64  `xorm:"default(1)"`
		IsMember bool   `xorm:"default(true)"`
		Name     string `xorm:"char(10)"`
	}

	assert.NoError(t, PrepareEngine())
	assertSync(t, new(TestModifyColumn))

	alterSQL := testEngine.Dialect().ModifyColumnSQL("test_modify_column", &schemas.Column{
		Name: "name",
		SQLType: schemas.SQLType{
			Name: "VARCHAR",
		},
		Length:         16,
		Nullable:       false,
		DefaultIsEmpty: true,
	})
	_, err := testEngine.Exec(alterSQL)
	assert.NoError(t, err)
}

type TestCollateColumn struct {
	Id     int64
	UserId int64  `xorm:"unique(s)"`
	Name   string `xorm:"varchar(20) unique(s)"`
	dbtype string `xorm:"-"`
}

func (t TestCollateColumn) TableCollations() []*schemas.Collation {
	if t.dbtype == string(schemas.MYSQL) {
		return []*schemas.Collation{
			{
				Name:   "utf8mb4_general_ci",
				Column: "name",
			},
		}
	} else if t.dbtype == string(schemas.MSSQL) {
		return []*schemas.Collation{
			{
				Name:   "Latin1_General_CI_AS",
				Column: "name",
			},
		}
	}
	return nil
}

func TestCollate(t *testing.T) {
	assert.NoError(t, PrepareEngine())
	assertSync(t, &TestCollateColumn{
		dbtype: string(testEngine.Dialect().URI().DBType),
	})

	_, err := testEngine.Insert(&TestCollateColumn{
		UserId: 1,
		Name:   "test",
	})
	assert.NoError(t, err)
	_, err = testEngine.Insert(&TestCollateColumn{
		UserId: 1,
		Name:   "Test",
	})
	switch testEngine.Dialect().URI().DBType {
	case schemas.MYSQL:
		ver, err1 := testEngine.DBVersion()
		assert.NoError(t, err1)

		tables, err1 := testEngine.DBMetas()
		assert.NoError(t, err1)
		for _, table := range tables {
			if table.Name == "test_collate_column" {
				col := table.GetColumn("name")
				if col == nil {
					assert.Error(t, errors.New("not found column"))
					return
				}
				// tidb doesn't follow utf8mb4_general_ci
				if col.Collation == "utf8mb4_general_ci" && ver.Edition != "TiDB" {
					assert.Error(t, err)
				} else {
					assert.NoError(t, err)
				}
				break
			}
		}
	case schemas.MSSQL:
		assert.Error(t, err)
	default:
		assert.NoError(t, err)
	}

	// Since SQLITE don't support modify column SQL, currrently just ignore
	if testEngine.Dialect().URI().DBType != schemas.MYSQL && testEngine.Dialect().URI().DBType != schemas.MSSQL {
		return
	}

	var newCollation string
	switch testEngine.Dialect().URI().DBType {
	case schemas.MYSQL:
		newCollation = "utf8mb4_bin"
	case schemas.MSSQL:
		return
	default:
		newCollation = "Latin1_General_CS_AS"
	}

	alterSQL := testEngine.Dialect().ModifyColumnSQL("test_collate_column", &schemas.Column{
		Name: "name",
		SQLType: schemas.SQLType{
			Name: "VARCHAR",
		},
		Length:         20,
		Nullable:       true,
		DefaultIsEmpty: true,
		Collation:      newCollation,
	})
	_, err = testEngine.Exec(alterSQL)
	assert.NoError(t, err)

	_, err = testEngine.Insert(&TestCollateColumn{
		UserId: 1,
		Name:   "test1",
	})
	assert.NoError(t, err)
	_, err = testEngine.Insert(&TestCollateColumn{
		UserId: 1,
		Name:   "Test1",
	})
	assert.NoError(t, err)
}

type SyncWithOpts1 struct {
	Id        int64
	Index     int `xorm:"index"`
	Unique    int `xorm:"unique"`
	Group1    int `xorm:"index(ttt)"`
	Group2    int `xorm:"index(ttt)"`
	UniGroup1 int `xorm:"unique(lll)"`
	UniGroup2 int `xorm:"unique(lll)"`
}

func (*SyncWithOpts1) TableName() string {
	return "sync_with_opts"
}

type SyncWithOpts2 struct {
	Id        int64
	Index     int `xorm:"index"`
	Unique    int `xorm:""`
	Group1    int `xorm:"index(ttt)"`
	Group2    int `xorm:"index(ttt)"`
	UniGroup1 int `xorm:""`
	UniGroup2 int `xorm:"unique(lll)"`
}

func (*SyncWithOpts2) TableName() string {
	return "sync_with_opts"
}

type SyncWithOpts3 struct {
	Id        int64
	Index     int `xorm:""`
	Unique    int `xorm:"unique"`
	Group1    int `xorm:""`
	Group2    int `xorm:"index(ttt)"`
	UniGroup1 int `xorm:"unique(lll)"`
	UniGroup2 int `xorm:"unique(lll)"`
}

func (*SyncWithOpts3) TableName() string {
	return "sync_with_opts"
}

func TestSyncWithOptions(t *testing.T) {
	assert.NoError(t, PrepareEngine())

	// ignore indices and constrains
	result, err := testEngine.SyncWithOptions(xorm.SyncOptions{IgnoreIndices: true, IgnoreConstrains: true}, &SyncWithOpts1{})
	assert.NoError(t, err)
	assert.NotNil(t, result)
	assert.Len(t, getIndicesOfBeanFromDB(t, &SyncWithOpts1{}), 0)

	// only ignore constrains
	result, err = testEngine.SyncWithOptions(xorm.SyncOptions{IgnoreConstrains: true}, &SyncWithOpts2{})
	assert.NoError(t, err)
	assert.NotNil(t, result)
	indices := getIndicesOfBeanFromDB(t, &SyncWithOpts1{})
	assert.Len(t, indices, 2)
	assert.ElementsMatch(t, []string{"ttt", "index"}, getKeysFromMap(indices))

	// only ignore indices
	result, err = testEngine.SyncWithOptions(xorm.SyncOptions{IgnoreIndices: true}, &SyncWithOpts3{})
	assert.NoError(t, err)
	assert.NotNil(t, result)
	indices = getIndicesOfBeanFromDB(t, &SyncWithOpts1{})
	assert.Len(t, indices, 4)
	assert.ElementsMatch(t, []string{"ttt", "index", "unique", "lll"}, getKeysFromMap(indices))

	// only ignore drop indices
	result, err = testEngine.SyncWithOptions(xorm.SyncOptions{IgnoreDropIndices: true}, &SyncWithOpts3{})
	assert.NoError(t, err)
	assert.NotNil(t, result)
	indices = getIndicesOfBeanFromDB(t, &SyncWithOpts1{})
	assert.Len(t, indices, 4)
	assert.ElementsMatch(t, []string{"ttt", "index", "unique", "lll"}, getKeysFromMap(indices))

	tableInfoFromStruct, _ := testEngine.TableInfo(&SyncWithOpts1{})
	assert.ElementsMatch(t, getKeysFromMap(tableInfoFromStruct.Indexes), getKeysFromMap(getIndicesOfBeanFromDB(t, &SyncWithOpts1{})))
}

func getIndicesOfBeanFromDB(t *testing.T, bean any) map[string]*schemas.Index {
	dbm, err := testEngine.DBMetas()
	assert.NoError(t, err)

	tName := testEngine.TableName(bean)
	var tSchema *schemas.Table
	for _, t := range dbm {
		if t.Name == tName {
			tSchema = t
			break
		}
	}
	if !assert.NotNil(t, tSchema) {
		return nil
	}
	return tSchema.Indexes
}

func getKeysFromMap(m map[string]*schemas.Index) []string {
	var ss []string
	for k := range m {
		ss = append(ss, k)
	}
	return ss
}

type SyncTestUser struct {
	Id   int64  `xorm:"pk autoincr 'id' comment('primary key 1')"`
	Name string `xorm:"'name' notnull comment('nickname')" json:"name"`
}

func (m *SyncTestUser) TableName() string {
	return "sync_test_user"
}

type SyncTestUser2 struct {
	Id   int64  `xorm:"pk autoincr 'id' comment('primary key 2')"`
	Name string `xorm:"'name' notnull comment('nickname')" json:"name"`
}

func (m *SyncTestUser2) TableName() string {
	return "sync_test_user"
}

func TestSync2_3(t *testing.T) {
	if testEngine.Dialect().URI().DBType == schemas.MYSQL {
		assert.NoError(t, PrepareEngine())
		assertSync(t, new(SyncTestUser))

		assert.NoError(t, testEngine.Sync2(new(SyncTestUser2)))
		tables, err := testEngine.DBMetas()
		assert.NoError(t, err)
		tableInfo, err := testEngine.TableInfo(new(SyncTestUser2))
		assert.NoError(t, err)

		assert.EqualValues(t, tables[0].GetColumn("id").IsAutoIncrement, tableInfo.GetColumn("id").IsAutoIncrement)
		assert.EqualValues(t, tables[0].GetColumn("id").Name, tableInfo.GetColumn("id").Name)
		assert.EqualValues(t, tables[0].GetColumn("id").SQLType.Name, tableInfo.GetColumn("id").SQLType.Name)
		assert.EqualValues(t, tables[0].GetColumn("id").Nullable, tableInfo.GetColumn("id").Nullable)
		assert.EqualValues(t, tables[0].GetColumn("id").Comment, tableInfo.GetColumn("id").Comment)
	}
}

func TestSyncJSON(t *testing.T) {
	type SyncTestJSON struct {
		Id    int64
		Value string `xorm:"LONGTEXT JSON 'value' comment('json value')"`
	}

	assert.NoError(t, PrepareEngine())
	assertSync(t, new(SyncTestJSON))

	assert.NoError(t, testEngine.Sync(new(SyncTestJSON)))
	tables, err := testEngine.DBMetas()
	assert.NoError(t, err)

	tableInfo, err := testEngine.TableInfo(new(SyncTestJSON))
	assert.NoError(t, err)

	assert.EqualValues(t, tables[0].GetColumn("id").IsAutoIncrement, tableInfo.GetColumn("id").IsAutoIncrement)
	assert.EqualValues(t, tables[0].GetColumn("id").Name, tableInfo.GetColumn("id").Name)
	if testEngine.Dialect().URI().DBType == schemas.MYSQL {
		assert.EqualValues(t, tables[0].GetColumn("id").SQLType.Name, tableInfo.GetColumn("id").SQLType.Name)
	}
	assert.EqualValues(t, tables[0].GetColumn("id").Nullable, tableInfo.GetColumn("id").Nullable)

	assert.EqualValues(t, tables[0].GetColumn("value").IsAutoIncrement, tableInfo.GetColumn("value").IsAutoIncrement)
	assert.EqualValues(t, tables[0].GetColumn("value").Name, tableInfo.GetColumn("value").Name)
	assert.EqualValues(t, tables[0].GetColumn("value").Nullable, tableInfo.GetColumn("value").Nullable)

	if testEngine.Dialect().URI().DBType == schemas.MYSQL {
		assert.EqualValues(t, tables[0].GetColumn("value").SQLType.Name, tableInfo.GetColumn("value").SQLType.Name)
	}
}
