package diagnostics

import (
	"bytes"
	"context"
	"dameng_exporter/logger"
	"database/sql/driver"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// Run explicitly to export log examples to DIAGNOSTICS_EVIDENCE_DIR.
func TestDiagnosticOutputEvidence(t *testing.T) {
	dir := os.Getenv("DIAGNOSTICS_EVIDENCE_DIR")
	if dir == "" {
		t.Skip("evidence output is opt-in")
	}
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatal(err)
	}
	for _, kind := range []string{"normal", "slow", "error", "debug", "fast_background"} {
		var out, console bytes.Buffer
		level := zap.InfoLevel
		if kind == "debug" {
			level = zap.DebugLevel
		}
		l := zap.New(logger.NewCore(zapcore.AddSync(&out), zapcore.AddSync(&console), level), zap.AddCaller()).Sugar()
		c := newClock()
		r := NewRound(Options{Clock: c, Threshold: 30 * time.Second, Logger: logger.DiagnosticLogger{SugaredLogger: l}})
		mode, budget := "blocking", time.Duration(0)
		if kind == "fast_background" {
			mode, budget = "fast", 5*time.Second
		}
		u := r.NewRun("database", "fixture", "CustomMetrics", mode, budget)
		if kind == "fast_background" {
			q := u.begin(context.Background(), "SELECT /* example */ COUNT(*) FROM SYSOBJECTS", false)
			c.advance(budget)
			u.Detach()
			u.Finish(0, 0, false)
			r.Finish(nil)
			c.advance(time.Second)
			q.leave()
			q.failure(os.ErrPermission, "query")
			q.finish()
			u.BackgroundFinished()
			saveEvidence(t, dir, kind, &out, &console)
			continue
		}

		call := 0
		db := openDB(t, func(context.Context, string, []driver.NamedValue) (driver.Rows, error) {
			call++
			if kind == "error" {
				return nil, os.ErrPermission
			}
			if kind == "slow" || kind == "debug" {
				if call == 1 {
					c.advance(29 * time.Second)
				} else {
					c.advance(time.Second)
					r.Trigger("duration_threshold")
					return nil, context.DeadlineExceeded
				}
			}
			return &driverRows{values: []driver.Value{int64(1)}}, nil
		})
		ctx, cancel := context.WithDeadline(context.Background(), c.Now().Add(30*time.Second))
		count := 1
		if kind == "slow" || kind == "debug" {
			count = 2
		}
		for i := 0; i < count; i++ {
			qctx := WithQueryOptions(ctx, QueryOptions{Purpose: "custom", Name: "count", Index: i + 1, AllowEmpty: true})
			rows, err := Wrap(db, u).QueryContext(qctx, "SELECT /* example */ COUNT(*) FROM SYSOBJECTS")
			if err == nil {
				for rows.Next() {
					var n int
					rows.Scan(&n)
				}
				rows.Err()
				rows.Close()
			}
		}
		cancel()
		u.Finish(0, 0, false)
		r.Finish(nil)
		saveEvidence(t, dir, kind, &out, &console)
	}
}
func saveEvidence(t *testing.T, dir, kind string, file, console *bytes.Buffer) {
	t.Helper()
	for name, data := range map[string][]byte{kind + ".file.jsonl": file.Bytes(), kind + ".console.txt": console.Bytes()} {
		if err := os.WriteFile(filepath.Join(dir, name), data, 0644); err != nil {
			t.Fatal(err)
		}
	}
}
