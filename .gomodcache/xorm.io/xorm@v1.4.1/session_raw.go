// Copyright 2016 The Xorm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package xorm

import (
	"database/sql"
	"strings"

	"xorm.io/xorm/core"
)

func (session *Session) queryPreprocess(sqlStr *string, paramStr ...any) {
	for _, filter := range session.engine.dialect.Filters() {
		*sqlStr = filter.Do(session.ctx, *sqlStr)
	}

	session.lastSQL = *sqlStr
	session.lastSQLArgs = paramStr
}

func (session *Session) queryRows(sqlStr string, args ...any) (*core.Rows, error) {
	defer session.resetStatement()
	if session.statement.LastError != nil {
		return nil, session.statement.LastError
	}

	session.queryPreprocess(&sqlStr, args...)

	session.lastSQL = sqlStr
	session.lastSQLArgs = args

	if session.isAutoCommit {
		var db *core.DB
		if session.sessionType == groupSession && strings.EqualFold(strings.TrimSpace(sqlStr)[:6], "select") && !session.statement.IsForUpdate {
			db = session.engine.engineGroup.Slave().DB()
		} else {
			db = session.DB()
		}

		if session.prepareStmt {
			// don't clear stmt since session will cache them
			stmt, err := session.doPrepare(db, sqlStr)
			if err != nil {
				return nil, err
			}

			return stmt.QueryContext(session.ctx, args...)
		}

		return db.QueryContext(session.ctx, sqlStr, args...)
	}

	if session.prepareStmt {
		stmt, err := session.doPrepareTx(sqlStr)
		if err != nil {
			return nil, err
		}

		return stmt.QueryContext(session.ctx, args...)
	}

	return session.tx.QueryContext(session.ctx, sqlStr, args...)
}

func (session *Session) queryRow(sqlStr string, args ...any) *core.Row {
	return core.NewRow(session.queryRows(sqlStr, args...))
}

// Query runs a raw sql and return records as []map[string][]byte
func (session *Session) Query(sqlOrArgs ...any) ([]map[string][]byte, error) {
	if session.isAutoClose {
		defer session.Close()
	}

	sqlStr, args, err := session.statement.GenQuerySQL(sqlOrArgs...)
	if err != nil {
		return nil, err
	}

	rows, err := session.queryRows(sqlStr, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	return session.engine.scanByteMaps(rows)
}

// QueryString runs a raw sql and return records as []map[string]string
func (session *Session) QueryString(sqlOrArgs ...any) ([]map[string]string, error) {
	if session.isAutoClose {
		defer session.Close()
	}

	sqlStr, args, err := session.statement.GenQuerySQL(sqlOrArgs...)
	if err != nil {
		return nil, err
	}

	rows, err := session.queryRows(sqlStr, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	return session.engine.ScanStringMaps(rows)
}

// QuerySliceString runs a raw sql and return records as [][]string
func (session *Session) QuerySliceString(sqlOrArgs ...any) ([][]string, error) {
	if session.isAutoClose {
		defer session.Close()
	}

	sqlStr, args, err := session.statement.GenQuerySQL(sqlOrArgs...)
	if err != nil {
		return nil, err
	}

	rows, err := session.queryRows(sqlStr, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	return session.engine.ScanStringSlices(rows)
}

// QueryInterface runs a raw sql and return records as []map[string]any
func (session *Session) QueryInterface(sqlOrArgs ...any) ([]map[string]any, error) {
	if session.isAutoClose {
		defer session.Close()
	}

	sqlStr, args, err := session.statement.GenQuerySQL(sqlOrArgs...)
	if err != nil {
		return nil, err
	}

	rows, err := session.queryRows(sqlStr, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	return session.engine.ScanInterfaceMaps(rows)
}

func (session *Session) exec(sqlStr string, args ...any) (sql.Result, error) {
	defer session.resetStatement()

	session.queryPreprocess(&sqlStr, args...)

	session.lastSQL = sqlStr
	session.lastSQLArgs = args

	var (
		res  sql.Result
		err  error
		stmt *core.Stmt
	)
	switch {
	case !session.isAutoCommit && session.prepareStmt:
		stmt, err = session.doPrepareTx(sqlStr)
		if err == nil {
			res, err = stmt.ExecContext(session.ctx, args...)
		}
	case !session.isAutoCommit:
		res, err = session.tx.ExecContext(session.ctx, sqlStr, args...)
	case session.prepareStmt:
		stmt, err = session.doPrepare(session.DB(), sqlStr)
		if err == nil {
			res, err = stmt.ExecContext(session.ctx, args...)
		}
	default:
		res, err = session.DB().ExecContext(session.ctx, sqlStr, args...)
	}
	if err != nil {
		cleanupProcessorsClosures(&session.afterClosures)
	}
	return res, err
}

// Exec raw sql
func (session *Session) Exec(sqlOrArgs ...any) (sql.Result, error) {
	if session.isAutoClose {
		defer session.Close()
	}

	if len(sqlOrArgs) == 0 {
		return nil, ErrUnSupportedType
	}

	sqlStr, args, err := session.statement.ConvertSQLOrArgs(sqlOrArgs...)
	if err != nil {
		return nil, err
	}

	return session.exec(sqlStr, args...)
}
