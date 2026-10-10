// Copyright 2018 The Xorm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package builder

import (
	"github.com/stretchr/testify/assert"
	"testing"
)

func TestCond_NotIn(t *testing.T) {
	t.Run("normal", func(t *testing.T) {
		cond := NotIn("test", 1, 2, 2, 3, 4, 4, 4)
		buf := NewWriter()
		err := cond.WriteTo(buf)
		assert.NoError(t, err)
		assert.Equal(t, "test NOT IN (?,?,?,?)", buf.String())
		assert.Len(t, buf.args, 4)
		assert.Equal(t, []interface{}{1, 2, 3, 4}, buf.args)
	})
	t.Run("slice", func(t *testing.T) {
		cond := NotIn("test", []int{1, 2, 2, 3, 4, 4, 4})
		buf := NewWriter()
		err := cond.WriteTo(buf)
		assert.NoError(t, err)
		assert.Equal(t, "test NOT IN (?,?,?,?)", buf.String())
		assert.Len(t, buf.args, 4)
		assert.Equal(t, []interface{}{1, 2, 3, 4}, buf.args)
	})
	t.Run("blank", func(t *testing.T) {
		cond := NotIn("test")
		buf := NewWriter()
		err := cond.WriteTo(buf)
		// The "error" is written to the bytes buffer
		assert.NoError(t, err)
		assert.Equal(t, "0=0", buf.String())
	})
}

func TestCond_In(t *testing.T) {
	t.Run("normal", func(t *testing.T) {
		cond := In("test", 1, 2, 2, 3, 4, 4, 4)
		buf := NewWriter()
		err := cond.WriteTo(buf)
		assert.NoError(t, err)
		assert.Equal(t, "test IN (?,?,?,?)", buf.String())
		assert.Len(t, buf.args, 4)
		assert.Equal(t, []interface{}{1, 2, 3, 4}, buf.args)
	})
	t.Run("slice", func(t *testing.T) {
		cond := In("test", []int{1, 2, 2, 3, 4, 4, 4})
		buf := NewWriter()
		err := cond.WriteTo(buf)
		assert.NoError(t, err)
		assert.Equal(t, "test IN (?,?,?,?)", buf.String())
		assert.Len(t, buf.args, 4)
		assert.Equal(t, []interface{}{1, 2, 3, 4}, buf.args)
	})
	t.Run("blank", func(t *testing.T) {
		cond := In("test")
		buf := NewWriter()
		err := cond.WriteTo(buf)
		// The "error" is written to the bytes buffer
		assert.NoError(t, err)
		assert.Equal(t, "0=1", buf.String())
	})
}
