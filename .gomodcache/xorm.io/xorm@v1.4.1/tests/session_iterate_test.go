// Copyright 2017 The Xorm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package tests

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestIterate(t *testing.T) {
	assert.NoError(t, PrepareEngine())

	type UserIterate struct {
		Id    int64
		IsMan bool
	}

	assert.NoError(t, testEngine.Sync(new(UserIterate)))

	cnt, err := testEngine.Insert(&UserIterate{
		IsMan: true,
	})
	assert.NoError(t, err)
	assert.EqualValues(t, 1, cnt)

	cnt, err = testEngine.Insert(&UserIterate{
		IsMan: false,
	})
	assert.NoError(t, err)
	assert.EqualValues(t, 1, cnt)

	cnt = 0
	err = testEngine.Iterate(new(UserIterate), func(i int, bean any) error {
		user := bean.(*UserIterate)
		if cnt == 0 {
			assert.EqualValues(t, 1, user.Id)
			assert.EqualValues(t, true, user.IsMan)
		} else {
			assert.EqualValues(t, 2, user.Id)
			assert.EqualValues(t, false, user.IsMan)
		}
		cnt++
		return nil
	})
	assert.NoError(t, err)
	assert.EqualValues(t, 2, cnt)
}

func TestBufferIterate(t *testing.T) {
	assert.NoError(t, PrepareEngine())

	type UserBufferIterate struct {
		Id    int64
		IsMan bool
	}

	assert.NoError(t, testEngine.Sync(new(UserBufferIterate)))

	size := 20
	for i := 0; i < size; i++ {
		cnt, err := testEngine.Insert(&UserBufferIterate{
			IsMan: true,
		})
		assert.NoError(t, err)
		assert.EqualValues(t, 1, cnt)
	}

	cnt := 0
	err := testEngine.BufferSize(9).Iterate(new(UserBufferIterate), func(i int, bean any) error {
		user := bean.(*UserBufferIterate)
		assert.EqualValues(t, cnt+1, user.Id)
		assert.EqualValues(t, true, user.IsMan)
		cnt++
		return nil
	})
	assert.NoError(t, err)
	assert.EqualValues(t, size, cnt)

	cnt = 0
	err = testEngine.Limit(20).BufferSize(9).Iterate(new(UserBufferIterate), func(i int, bean any) error {
		user := bean.(*UserBufferIterate)
		assert.EqualValues(t, cnt+1, user.Id)
		assert.EqualValues(t, true, user.IsMan)
		cnt++
		return nil
	})
	assert.NoError(t, err)
	assert.EqualValues(t, size, cnt)

	cnt = 0
	err = testEngine.Limit(7).BufferSize(9).Iterate(new(UserBufferIterate), func(i int, bean any) error {
		user := bean.(*UserBufferIterate)
		assert.EqualValues(t, cnt+1, user.Id)
		assert.EqualValues(t, true, user.IsMan)
		cnt++
		return nil
	})
	assert.NoError(t, err)
	assert.EqualValues(t, 7, cnt)

	cnt = 0
	err = testEngine.Where("`id` <= 10").BufferSize(2).Iterate(new(UserBufferIterate), func(i int, bean any) error {
		user := bean.(*UserBufferIterate)
		assert.EqualValues(t, cnt+1, user.Id)
		assert.EqualValues(t, true, user.IsMan)
		cnt++
		return nil
	})
	assert.NoError(t, err)
	assert.EqualValues(t, 10, cnt)

	// test reset statement
	cnt = 0
	sess := testEngine.NewSession()
	defer sess.Close()
	// generate: SELECT `id`, `is_man` FROM `user_buffer_iterate` WHERE (`id` <= 10) LIMIT 2 OFFSET 10 []
	err = sess.Where("`id` <= 10").BufferSize(2).Iterate(new(UserBufferIterate), func(i int, bean any) error {
		user := bean.(*UserBufferIterate)
		assert.EqualValues(t, cnt+1, user.Id)
		assert.EqualValues(t, true, user.IsMan)
		cnt++
		return nil
	})
	assert.NoError(t, err)
	assert.EqualValues(t, 10, cnt)

	beans := make([]*UserBufferIterate, 0, 10)
	// generate: SELECT `id`, `is_man` FROM `user_buffer_iterate` WHERE (`id` <= 10) AND (`id` >= 10) LIMIT 2 OFFSET 10 []
	err = sess.Where("`id` >= 10").Limit(2).Find(&beans)
	assert.NoError(t, err)
	assert.Len(t, beans, 2)
}
