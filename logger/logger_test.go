package logger

import (
	"bytes"
	"dameng_exporter/diagnostics"
	"encoding/json"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"strings"
	"testing"
	"time"
)

func TestFileEncodingIsSingleLineAndReversible(t *testing.T) {
	var out bytes.Buffer
	l := zap.New(zapcore.NewCore(zapcore.NewJSONEncoder(encoderConfig(false)), zapcore.AddSync(&out), zap.DebugLevel))
	query := "select 'quote\\value\"' -- comment\n\t/* multiline\n SQL */ " + strings.Repeat("x", 100000)
	l.Sugar().Errorw("sql_error", "sql", query)
	if strings.Count(out.String(), "\n") != 1 || strings.Contains(out.String(), "\x1b[") || strings.Contains(out.String(), "stacktrace") {
		t.Fatal("multiline, color or implicit stack in output")
	}
	var data map[string]interface{}
	if err := json.Unmarshal(out.Bytes(), &data); err != nil {
		t.Fatal(err)
	}
	if data["sql"] != query {
		t.Fatal("SQL changed or truncated")
	}
}
func TestTimestampHasMilliseconds(t *testing.T) {
	var out bytes.Buffer
	cfg := zap.NewProductionEncoderConfig()
	cfg.EncodeTime = customTimeEncoder
	enc := zapcore.NewJSONEncoder(cfg)
	entry := zapcore.Entry{Time: time.Date(2026, 11, 12, 10, 20, 30, 123000000, time.FixedZone("CST", 8*3600))}
	buffer, err := enc.EncodeEntry(entry, nil)
	if err != nil {
		t.Fatal(err)
	}
	out.Write(buffer.Bytes())
	buffer.Free()
	if !strings.Contains(out.String(), "2026-11-12T10:20:30.123+08:00") {
		t.Fatal(out.String())
	}
}
func TestPanicCarriesExplicitStack(t *testing.T) {
	var out bytes.Buffer
	cfg := zap.NewProductionEncoderConfig()
	l := zap.New(zapcore.NewCore(zapcore.NewJSONEncoder(cfg), zapcore.AddSync(&out), zap.DebugLevel)).Sugar()
	r := diagnostics.NewRound(diagnostics.Options{Logger: l})
	run := r.NewRun("exporter", "", "panic", "direct", 0)
	run.Panic("failure", "explicit stack")
	run.Finish(0, 0, false)
	r.Finish(nil)
	if !strings.Contains(out.String(), `"stack":"explicit stack"`) {
		t.Fatal("panic stack missing")
	}
}
