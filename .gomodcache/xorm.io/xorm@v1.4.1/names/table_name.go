// Copyright 2020 The Xorm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package names

import (
	"reflect"
	"sync"
)

// TableName table name interface to define customerize table name
type TableName interface {
	TableName() string
}

type TableComment interface {
	TableComment() string
}

var (
	tpTableName    = reflect.TypeOf((*TableName)(nil)).Elem()
	tpTableComment = reflect.TypeOf((*TableComment)(nil)).Elem()
	tvCache        sync.Map
	tcCache        sync.Map
)

// GetTableName returns table name
func GetTableName(mapper Mapper, v reflect.Value) string {
	if v.Type().Implements(tpTableName) {
		return v.Interface().(TableName).TableName()
	}

	switch {
	case v.Kind() == reflect.Ptr:
		v = v.Elem()
		if v.Type().Implements(tpTableName) {
			return v.Interface().(TableName).TableName()
		}
	case v.CanAddr():
		v1 := v.Addr()
		if v1.Type().Implements(tpTableName) {
			return v1.Interface().(TableName).TableName()
		}
	default:
		name, ok := tvCache.Load(v.Type())
		if ok {
			if name.(string) != "" {
				return name.(string)
			}
		} else {
			v2 := reflect.New(v.Type())
			if v2.Type().Implements(tpTableName) {
				tableName := v2.Interface().(TableName).TableName()
				tvCache.Store(v.Type(), tableName)
				return tableName
			}

			tvCache.Store(v.Type(), "")
		}
	}

	return mapper.Obj2Table(v.Type().Name())
}

// GetTableComment returns table comment
func GetTableComment(v reflect.Value) string {
	if v.Type().Implements(tpTableComment) {
		return v.Interface().(TableComment).TableComment()
	}

	switch {
	case v.Kind() == reflect.Ptr:
		v = v.Elem()
		if v.Type().Implements(tpTableComment) {
			return v.Interface().(TableComment).TableComment()
		}
	case v.CanAddr():
		v1 := v.Addr()
		if v1.Type().Implements(tpTableComment) {
			return v1.Interface().(TableComment).TableComment()
		}
	default:
		comment, ok := tcCache.Load(v.Type())
		if ok {
			if comment.(string) != "" {
				return comment.(string)
			}
		} else {
			v2 := reflect.New(v.Type())
			if v2.Type().Implements(tpTableComment) {
				tableComment := v2.Interface().(TableComment).TableComment()
				tcCache.Store(v.Type(), tableComment)
				return tableComment
			}

			tcCache.Store(v.Type(), "")
		}
	}

	return ""
}
