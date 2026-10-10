// Copyright 2026 The Xorm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package xorm

import (
	"strings"
	"testing"

	"xorm.io/xorm/caches"
	"xorm.io/xorm/core"
	"xorm.io/xorm/dialects"
	"xorm.io/xorm/log"
	"xorm.io/xorm/names"
	"xorm.io/xorm/tags"
)

type engineTestLogger struct {
	showSQL bool
}

func (l *engineTestLogger) BeforeSQL(log.LogContext) {}
func (l *engineTestLogger) AfterSQL(log.LogContext)  {}
func (l *engineTestLogger) Debugf(string, ...any)    {}
func (l *engineTestLogger) Errorf(string, ...any)    {}
func (l *engineTestLogger) Infof(string, ...any)     {}
func (l *engineTestLogger) Warnf(string, ...any)     {}
func (l *engineTestLogger) Level() log.LogLevel      { return log.LOG_INFO }
func (l *engineTestLogger) SetLevel(log.LogLevel)    {}
func (l *engineTestLogger) ShowSQL(show ...bool) {
	if len(show) == 0 {
		l.showSQL = true
		return
	}
	l.showSQL = show[0]
}
func (l *engineTestLogger) IsShowSQL() bool { return l.showSQL }

func newTestEngine(t *testing.T) *Engine {
	t.Helper()
	dialect, err := dialects.OpenDialect("sqlite3", "file:test.db?mode=memory")
	if err != nil {
		t.Fatalf("failed to open dialect: %v", err)
	}
	mapper := names.NewCacheMapper(new(names.SnakeMapper))
	cacherMgr := caches.NewManager()
	parser := tags.NewParser("xorm", dialect, mapper, mapper, cacherMgr)
	return &Engine{
		dialect:        dialect,
		db:             &core.DB{},
		logger:         &engineTestLogger{},
		tagParser:      parser,
		cacherMgr:      cacherMgr,
		driverName:     "sqlite3",
		dataSourceName: "file:test.db?mode=memory",
	}
}

func TestEngineQuote(t *testing.T) {
	engine := newTestEngine(t)
	if got := engine.Quote(" user "); got != "`user`" {
		t.Fatalf("unexpected quote: %s", got)
	}
	var buf strings.Builder
	engine.QuoteTo(&buf, " user ")
	if buf.String() != "`user`" {
		t.Fatalf("unexpected QuoteTo result: %s", buf.String())
	}
	engine.QuoteTo(nil, "user")
}

func TestEngineShowSQLAndLogger(t *testing.T) {
	engine := newTestEngine(t)
	logger := engine.logger.(*engineTestLogger)
	engine.ShowSQL()
	if !logger.showSQL {
		t.Fatalf("expected ShowSQL to enable logging")
	}
	if engine.db.Logger != engine.logger {
		t.Fatalf("expected DB logger to be updated")
	}
	engine.ShowSQL(false)
	if logger.showSQL {
		t.Fatalf("expected ShowSQL to disable logging")
	}

	engine.SetLogger(log.DiscardLogger{})
	if engine.db.Logger == nil {
		t.Fatalf("expected DB logger to be set")
	}
}

func TestEngineSetLoggerPanics(t *testing.T) {
	engine := newTestEngine(t)
	defer func() {
		if recover() == nil {
			t.Fatalf("expected panic when logger is invalid")
		}
	}()
	engine.SetLogger(123)
}

func TestEngineIdentifiers(t *testing.T) {
	engine := newTestEngine(t)
	if engine.DriverName() != "sqlite3" {
		t.Fatalf("unexpected driver name: %s", engine.DriverName())
	}
	if engine.DataSourceName() == "" {
		t.Fatalf("expected non-empty data source name")
	}
}
