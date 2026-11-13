package diagnostics

import (
	"bytes"
	"context"
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
	for _, kind := range []string{"normal", "slow", "error", "debug"} {
		var out bytes.Buffer
		level := zap.InfoLevel
		if kind == "debug" {
			level = zap.DebugLevel
		}
		cfg := zap.NewProductionEncoderConfig()
		cfg.EncodeLevel = zapcore.CapitalLevelEncoder
		cfg.EncodeTime = zapcore.ISO8601TimeEncoder
		l := zap.New(zapcore.NewCore(zapcore.NewJSONEncoder(cfg), zapcore.AddSync(&out), level)).Sugar()
		c := newClock()
		r := NewRound(Options{Clock: c, Threshold: 30 * time.Second, Logger: l})
		u := r.NewRun("database", "fixture", "CustomMetrics", "blocking", 0)
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
		if err := os.WriteFile(filepath.Join(dir, kind+".jsonl"), out.Bytes(), 0644); err != nil {
			t.Fatal(err)
		}
	}
}
