// Copyright 2023 The Xorm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package tests

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

type TestSync1 struct {
	Id      int64
	ClassId int64 `xorm:"index"`
}

func (TestSync1) TableName() string {
	return "test_sync"
}

type TestSync2 struct {
	Id      int64
	ClassId int64 `xorm:"unique"`
}

func (TestSync2) TableName() string {
	return "test_sync"
}

func TestSync(t *testing.T) {
	assert.NoError(t, testEngine.Sync(new(TestSync1)))
	assert.NoError(t, testEngine.Sync(new(TestSync2)))
}
