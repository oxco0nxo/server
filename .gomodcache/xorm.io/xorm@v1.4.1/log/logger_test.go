// Copyright 2026 The Xorm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package log

import (
	"bytes"
	"context"
	"fmt"
	"strings"
	"testing"
	"time"
)

type captureLogger struct {
	DiscardLogger
	infofCalls []string
	level      LogLevel
	showSQL    bool
}

func (c *captureLogger) Infof(format string, v ...any) {
	c.infofCalls = append(c.infofCalls, fmt.Sprintf(format, v...))
}

func (c *captureLogger) Level() LogLevel {
	return c.level
}

func (c *captureLogger) SetLevel(l LogLevel) {
	c.level = l
}

func (c *captureLogger) ShowSQL(show ...bool) {
	if len(show) == 0 {
		c.showSQL = true
		return
	}
	c.showSQL = show[0]
}

func (c *captureLogger) IsShowSQL() bool {
	return c.showSQL
}

func TestSimpleLoggerLevelsAndShowSQL(t *testing.T) {
	var buf bytes.Buffer
	logger := NewSimpleLogger3(&buf, "[xorm]", 0, LOG_INFO)

	logger.Debug("skip")
	if buf.Len() != 0 {
		t.Fatalf("expected no debug output, got %q", buf.String())
	}

	logger.Info("hello")
	output := buf.String()
	if !strings.Contains(output, "[xorm] [info]") || !strings.Contains(output, "hello") {
		t.Fatalf("unexpected info output: %q", output)
	}

	buf.Reset()
	logger.SetLevel(LOG_DEBUG)
	logger.Debugf("d=%d", 1)
	output = buf.String()
	if !strings.Contains(output, "[xorm] [debug]") || !strings.Contains(output, "d=1") {
		t.Fatalf("unexpected debug output: %q", output)
	}

	if logger.IsShowSQL() {
		t.Fatalf("expected ShowSQL to default to false")
	}
	logger.ShowSQL()
	if !logger.IsShowSQL() {
		t.Fatalf("expected ShowSQL to be enabled")
	}
	logger.ShowSQL(false)
	if logger.IsShowSQL() {
		t.Fatalf("expected ShowSQL to be disabled")
	}
}

func TestLoggerAdapterAfterSQL(t *testing.T) {
	base := &captureLogger{}
	adapter := NewLoggerAdapter(base)

	adapter.ShowSQL()
	if !base.showSQL {
		t.Fatalf("expected ShowSQL to be forwarded")
	}
	adapter.ShowSQL(false)
	if base.showSQL {
		t.Fatalf("expected ShowSQL=false to be forwarded")
	}

	ctx := LogContext{
		Ctx:         context.WithValue(context.Background(), SessionIDKey, "session-1"),
		SQL:         "SELECT * FROM users",
		Args:        []any{1},
		ExecuteTime: time.Millisecond,
	}
	adapter.AfterSQL(ctx)
	if len(base.infofCalls) != 1 {
		t.Fatalf("expected 1 log call, got %d", len(base.infofCalls))
	}
	logLine := base.infofCalls[0]
	if !strings.Contains(logLine, "[SQL] [session-1]") || !strings.Contains(logLine, "SELECT * FROM users") {
		t.Fatalf("unexpected SQL log: %q", logLine)
	}
	if !strings.Contains(logLine, "[1]") || !strings.Contains(logLine, " - ") {
		t.Fatalf("expected args and duration in SQL log: %q", logLine)
	}

	base.infofCalls = nil
	ctx = LogContext{
		Ctx:  context.Background(),
		SQL:  "UPDATE users SET name=?",
		Args: []any{"alice"},
	}
	adapter.AfterSQL(ctx)
	if len(base.infofCalls) != 1 {
		t.Fatalf("expected 1 log call, got %d", len(base.infofCalls))
	}
	logLine = base.infofCalls[0]
	if strings.Contains(logLine, " - ") {
		t.Fatalf("expected SQL log without duration: %q", logLine)
	}
	if !strings.Contains(logLine, "UPDATE users SET name=?") || !strings.Contains(logLine, "[alice]") {
		t.Fatalf("unexpected SQL log content: %q", logLine)
	}
}
