// Copyright 2019 The Xorm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package schemas

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestAlwaysQuoteTo(t *testing.T) {
	var (
		quoter = Quoter{'[', ']', AlwaysReserve}
		kases  = []struct {
			expected string
			value    string
		}{
			{"[mytable]", "mytable"},
			{"[mytable]", "`mytable`"},
			{"[mytable]", `[mytable]`},
			{`["mytable"]`, `"mytable"`},
			{`[mytable].*`, `[mytable].*`},
			{"[myschema].[mytable]", "myschema.mytable"},
			{"[myschema].[mytable]", "`myschema`.mytable"},
			{"[myschema].[mytable]", "myschema.`mytable`"},
			{"[myschema].[mytable]", "`myschema`.`mytable`"},
			{"[myschema].[mytable]", `[myschema].mytable`},
			{"[myschema].[mytable]", `myschema.[mytable]`},
			{"[myschema].[mytable]", `[myschema].[mytable]`},
			{`["myschema].[mytable"]`, `"myschema.mytable"`},
			{"[message_user] AS [sender]", "`message_user` AS `sender`"},
			{"[myschema].[mytable] AS [table]", "myschema.mytable AS table"},
			{" [mytable]", " mytable"},
			{"  [mytable]", "  mytable"},
			{"[mytable] ", "mytable "},
			{"[mytable]  ", "mytable  "},
			{" [mytable] ", " mytable "},
			{"  [mytable]  ", "  mytable  "},
		}
	)

	for _, v := range kases {
		t.Run(v.value, func(t *testing.T) {
			buf := &strings.Builder{}
			err := quoter.QuoteTo(buf, v.value)
			assert.NoError(t, err)
			assert.EqualValues(t, v.expected, buf.String())
		})
	}
}

func TestReversedQuoteTo(t *testing.T) {
	var (
		quoter = Quoter{'[', ']', func(s string) bool {
			return s == "mytable"
		}}
		kases = []struct {
			expected string
			value    string
		}{
			{"[mytable]", "mytable"},
			{"[mytable]", "`mytable`"},
			{"[mytable]", `[mytable]`},
			{"[mytable].*", `[mytable].*`},
			{`"mytable"`, `"mytable"`},
			{"myschema.[mytable]", "myschema.mytable"},
			{"myschema.[mytable]", "`myschema`.mytable"},
			{"myschema.[mytable]", "myschema.`mytable`"},
			{"myschema.[mytable]", "`myschema`.`mytable`"},
			{"myschema.[mytable]", `[myschema].mytable`},
			{"myschema.[mytable]", `myschema.[mytable]`},
			{"myschema.[mytable]", `[myschema].[mytable]`},
			{`"myschema.mytable"`, `"myschema.mytable"`},
			{"message_user AS sender", "`message_user` AS `sender`"},
			{"myschema.[mytable] AS table", "myschema.mytable AS table"},
		}
	)

	for _, v := range kases {
		t.Run(v.value, func(t *testing.T) {
			buf := &strings.Builder{}
			quoter.QuoteTo(buf, v.value)
			assert.EqualValues(t, v.expected, buf.String())
		})
	}
}

func TestNoQuoteTo(t *testing.T) {
	var (
		quoter = Quoter{'[', ']', AlwaysNoReserve}
		kases  = []struct {
			expected string
			value    string
		}{
			{"mytable", "mytable"},
			{"mytable", "`mytable`"},
			{"mytable", `[mytable]`},
			{"mytable.*", `[mytable].*`},
			{`"mytable"`, `"mytable"`},
			{"myschema.mytable", "myschema.mytable"},
			{"myschema.mytable", "`myschema`.mytable"},
			{"myschema.mytable", "myschema.`mytable`"},
			{"myschema.mytable", "`myschema`.`mytable`"},
			{"myschema.mytable", `[myschema].mytable`},
			{"myschema.mytable", `myschema.[mytable]`},
			{"myschema.mytable", `[myschema].[mytable]`},
			{`"myschema.mytable"`, `"myschema.mytable"`},
			{"message_user AS sender", "`message_user` AS `sender`"},
			{"myschema.mytable AS table", "myschema.mytable AS table"},
		}
	)

	for _, v := range kases {
		t.Run(v.value, func(t *testing.T) {
			buf := &strings.Builder{}
			err := quoter.QuoteTo(buf, v.value)
			assert.NoError(t, err)
			assert.EqualValues(t, v.expected, buf.String())
		})
	}
}

func TestJoin(t *testing.T) {
	cols := []string{"f1", "f2", "f3"}
	quoter := Quoter{'[', ']', AlwaysReserve}

	assert.EqualValues(t, "[a],[b]", quoter.Join([]string{"a", " b"}, ","))

	assert.EqualValues(t, "[a].*,[b].[c]", quoter.Join([]string{"a.*", " b.c"}, ","))

	assert.EqualValues(t, "[b] [a]", quoter.Join([]string{"b a"}, ","))

	assert.EqualValues(t, "[f1], [f2], [f3]", quoter.Join(cols, ", "))

	quoter.IsReserved = AlwaysNoReserve
	assert.EqualValues(t, "f1, f2, f3", quoter.Join(cols, ", "))
}

func TestStrings(t *testing.T) {
	cols := []string{"f1", "f2", "t3.f3", "t4.*"}
	quoter := Quoter{'[', ']', AlwaysReserve}

	quotedCols := quoter.Strings(cols)
	assert.EqualValues(t, []string{"[f1]", "[f2]", "[t3].[f3]", "[t4].*"}, quotedCols)
}

func TestTrim(t *testing.T) {
	kases := map[string]string{
		"[table_name]":          "table_name",
		"[schema].[table_name]": "schema.table_name",
	}

	for src, dst := range kases {
		assert.EqualValues(t, src, CommonQuoter.Trim(src))
		assert.EqualValues(t, dst, Quoter{'[', ']', AlwaysReserve}.Trim(src))
	}
}

func TestReplace(t *testing.T) {
	q := Quoter{'[', ']', AlwaysReserve}
	kases := []struct {
		source   string
		expected string
	}{
		{
			"SELECT `COLUMN_NAME` FROM `INFORMATION_SCHEMA`.`COLUMNS` WHERE `TABLE_SCHEMA` = ? AND `TABLE_NAME` = ? AND `COLUMN_NAME` = ?",
			"SELECT [COLUMN_NAME] FROM [INFORMATION_SCHEMA].[COLUMNS] WHERE [TABLE_SCHEMA] = ? AND [TABLE_NAME] = ? AND [COLUMN_NAME] = ?",
		},
		{
			"SELECT 'abc```test```''', `a` FROM b",
			"SELECT 'abc```test```''', [a] FROM b",
		},
		{
			"SELECT * FROM `a` INNER JOIN `b` `c` WHERE `a`.`id` = `c`.`a_id`",
			"SELECT * FROM [a] INNER JOIN [b] [c] WHERE [a].[id] = [c].[a_id]",
		},
		{
			"UPDATE table SET `a` = ~ `a`, `b`='abc`'",
			"UPDATE table SET [a] = ~ [a], [b]='abc`'",
		},
		{
			"INSERT INTO `insert_where` (`height`,`name`,`repo_id`,`width`,`index`) SELECT $1,$2,$3,$4,coalesce(MAX(`index`),0)+1 FROM `insert_where` WHERE (`repo_id`=$5)",
			"INSERT INTO [insert_where] ([height],[name],[repo_id],[width],[index]) SELECT $1,$2,$3,$4,coalesce(MAX([index]),0)+1 FROM [insert_where] WHERE ([repo_id]=$5)",
		},
	}

	for _, kase := range kases {
		t.Run(kase.source, func(t *testing.T) {
			assert.EqualValues(t, kase.expected, q.Replace(kase.source))
		})
	}
}
