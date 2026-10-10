// Copyright 2021 The Xorm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package tests

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func BenchmarkGetVars(b *testing.B) {
	b.StopTimer()

	assert.NoError(b, PrepareEngine())
	testEngine.ShowSQL(false)

	type BenchmarkGetVars struct {
		Id   int64
		Name string
	}

	assert.NoError(b, testEngine.Sync(new(BenchmarkGetVars)))

	v := BenchmarkGetVars{
		Name: "myname",
	}
	_, err := testEngine.Insert(&v)
	assert.NoError(b, err)

	b.StartTimer()
	var myname string
	for i := 0; i < b.N; i++ {
		has, err := testEngine.Cols("name").Table("benchmark_get_vars").Where("`id`=?", v.Id).Get(&myname)
		b.StopTimer()
		myname = ""
		assert.True(b, has)
		assert.NoError(b, err)
		b.StartTimer()
	}
}

func BenchmarkGetStruct(b *testing.B) {
	b.StopTimer()

	assert.NoError(b, PrepareEngine())
	testEngine.ShowSQL(false)

	type BenchmarkGetStruct struct {
		Id   int64
		Name string
	}

	assert.NoError(b, testEngine.Sync(new(BenchmarkGetStruct)))

	v := BenchmarkGetStruct{
		Name: "myname",
	}
	_, err := testEngine.Insert(&v)
	assert.NoError(b, err)

	b.StartTimer()
	var myname BenchmarkGetStruct
	for i := 0; i < b.N; i++ {
		has, err := testEngine.ID(v.Id).Get(&myname)
		b.StopTimer()
		myname.Id = 0
		myname.Name = ""
		assert.True(b, has)
		assert.NoError(b, err)
		b.StartTimer()
	}
}

func BenchmarkFindStruct(b *testing.B) {
	b.StopTimer()

	assert.NoError(b, PrepareEngine())
	testEngine.ShowSQL(false)

	type BenchmarkFindStruct struct {
		Id   int64
		Name string
	}

	assert.NoError(b, testEngine.Sync(new(BenchmarkFindStruct)))

	v := BenchmarkFindStruct{
		Name: "myname",
	}
	_, err := testEngine.Insert(&v)
	assert.NoError(b, err)

	mynames := make([]BenchmarkFindStruct, 0, 1)
	b.StartTimer()
	for i := 0; i < b.N; i++ {
		err := testEngine.Find(&mynames)
		b.StopTimer()
		mynames = make([]BenchmarkFindStruct, 0, 1)
		assert.NoError(b, err)
		b.StartTimer()
	}
}
