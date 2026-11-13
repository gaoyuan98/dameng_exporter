package logger

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

func TestConsoleSummaryAndSingleFileEvent(t *testing.T) {
	var console, file bytes.Buffer
	l := zap.New(zapcore.NewTee(NewConsoleCore(zapcore.AddSync(&console), zap.InfoLevel), zapcore.NewCore(zapcore.NewJSONEncoder(encoderConfig(false)), zapcore.AddSync(&file), zap.InfoLevel))).Sugar()
	l.Infow("scrape_finished", "scrape_id", "20261113T020649.558007000-1", "collection_duration_ms", int64(55), "runs", 36, "sql_calls", 55, "samples", 135)
	text := console.String()
	if !strings.Contains(text, "采集完成 | 耗时 55ms | 采集任务 36 | SQL 55 | 样本 135 | 关联 S1") || strings.Contains(text, "{") || len([]rune(strings.TrimSpace(text))) > 160 {
		t.Fatal(text)
	}
	if strings.Count(file.String(), "\n") != 1 || !strings.Contains(file.String(), `"msg":"scrape_finished"`) {
		t.Fatal(file.String())
	}
}
func TestErrorDetailsRemainVisibleAtEveryLevel(t *testing.T) {
	for _, level := range []zapcore.Level{zap.DebugLevel, zap.InfoLevel, zap.WarnLevel, zap.ErrorLevel} {
		t.Run(level.String(), func(t *testing.T) {
			var out bytes.Buffer
			l := zap.New(NewConsoleCore(zapcore.AddSync(&out), level)).Sugar()
			query := "select 'a|b' -- first\n\t/* \\" + strings.Repeat("x", 100000) + " */\x1b[31m\u2028"
			l.Errorw("sql_error", "scrape_id", "20261113T020649.558007000-1", "query_id", "Q6", "datasource", "prod|db\nforged", "collector", "Job", "error", errors.New("denied\nnext"), "sql", query)
			lines := strings.Split(strings.TrimSuffix(out.String(), "\n"), "\n")
			if len(lines) != 2 || !strings.Contains(lines[0], "SQL执行失败") || !strings.Contains(lines[1], "ERROR  SQL详情") {
				t.Fatal(out.String()[:min(out.Len(), 1000)])
			}
			encoded := strings.SplitN(lines[1], " | SQL ", 2)[1]
			decoded, err := strconv.Unquote(encoded)
			if err != nil || decoded != query || strings.Contains(out.String(), "\x1b") {
				t.Fatal("SQL escaping changed its contents", err)
			}
		})
	}
}

type countingObject struct{ calls *int }

func (v countingObject) MarshalLogObject(e zapcore.ObjectEncoder) error {
	*v.calls++
	e.AddString("value", "kept")
	return nil
}
func TestConsoleWithAndUnknownFields(t *testing.T) {
	var out bytes.Buffer
	calls := 0
	base := zap.New(NewConsoleCore(zapcore.AddSync(&out), zap.InfoLevel))
	a := base.With(zap.String("datasource", "first"), zap.Object("object", countingObject{&calls}))
	b := a.With(zap.String("datasource", "second"))
	b.Info("unknown\nmessage", zap.String("datasource", "event"), zap.String("future_field", "kept"), zap.String("phase", "future_phase"))
	a.Info("unknown")
	if calls != 1 || strings.Count(out.String(), "\n") != 2 || !strings.Contains(out.String(), "datasource event") || !strings.Contains(out.String(), "datasource first") || !strings.Contains(out.String(), "future_field kept") || !strings.Contains(out.String(), "phase future_phase") {
		t.Fatal(calls, out.String())
	}
	base.With(zap.Namespace("nested"), zap.String("a", "one")).Info("namespaced", zap.String("b", "two"))
	if !strings.Contains(out.String(), `\"a\":\"one\"`) || !strings.Contains(out.String(), `\"b\":\"two\"`) {
		t.Fatal(out.String())
	}
}
func TestConsoleConcurrentDetailsStayTogether(t *testing.T) {
	var out, file bytes.Buffer
	l := zap.New(NewCore(zapcore.AddSync(&file), zapcore.AddSync(&out), zap.InfoLevel))
	var wg sync.WaitGroup
	for i := 0; i < 80; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			l.With(zap.String("scrape_id", fmt.Sprintf("20261113T020649.558007000-%d", i))).Error("sql_error", zap.String("sql", fmt.Sprint(i)), zap.String("error", "denied"))
		}(i)
	}
	wg.Wait()
	fileLines := strings.Split(strings.TrimSpace(file.String()), "\n")
	if len(fileLines) != 80 {
		t.Fatal("wrong file event count", len(fileLines))
	}
	for _, line := range fileLines {
		if !json.Valid([]byte(line)) {
			t.Fatal("corrupt concurrent file event", line)
		}
	}
	lines := strings.Split(strings.TrimSuffix(out.String(), "\n"), "\n")
	if len(lines) != 160 {
		t.Fatal(len(lines))
	}
	for i := 0; i < len(lines); i += 2 {
		ref := strings.SplitN(lines[i], "关联 ", 2)[1]
		if !strings.Contains(lines[i+1], "SQL详情 | 关联 "+ref+" | SQL ") {
			t.Fatal(lines[i : i+2])
		}
	}
}

func TestConsoleWithCapturesMutableValues(t *testing.T) {
	var out bytes.Buffer
	metadata := map[string]interface{}{"nested": []interface{}{"original", uint64(9007199254740993)}}
	l := zap.New(NewConsoleCore(zapcore.AddSync(&out), zap.InfoLevel)).With(zap.Any("metadata", metadata))
	metadata["nested"].([]interface{})[0] = "changed"
	l.Info("unknown")
	if !strings.Contains(out.String(), "original") || strings.Contains(out.String(), "changed") || !strings.Contains(out.String(), "9007199254740993") {
		t.Fatal(out.String())
	}
}

type syncCountingSink struct {
	bytes.Buffer
	syncs int
}

func (s *syncCountingSink) Sync() error { s.syncs++; return nil }

func TestConsolePreservesEntryMetadataAndFlushesFatal(t *testing.T) {
	sink := &syncCountingSink{}
	c := NewConsoleCore(sink, zap.DebugLevel)
	e := zapcore.Entry{Level: zap.FatalLevel, Message: "failed", LoggerName: "service", Stack: "frame1\nframe2"}
	if err := c.Write(e, nil); err != nil {
		t.Fatal(err)
	}
	if sink.syncs != 1 || !strings.Contains(sink.String(), "日志 service") || !strings.Contains(sink.String(), `stack "frame1\nframe2"`) || strings.Count(sink.String(), "\n") != 2 {
		t.Fatal(sink.syncs, sink.String())
	}
}

type failingSink struct{ short bool }

func (s failingSink) Write(p []byte) (int, error) {
	if s.short {
		return 1, nil
	}
	return 0, io.ErrClosedPipe
}
func (s failingSink) Sync() error { return io.ErrClosedPipe }
func TestConsolePropagatesWriteAndSyncErrors(t *testing.T) {
	e := zapcore.Entry{Message: "scrape_started", Level: zap.InfoLevel}
	for _, short := range []bool{false, true} {
		c := NewConsoleCore(failingSink{short}, zap.InfoLevel)
		err := c.Write(e, nil)
		want := io.ErrClosedPipe
		if short {
			want = io.ErrShortWrite
		}
		if !errors.Is(err, want) || !errors.Is(c.Sync(), io.ErrClosedPipe) {
			t.Fatal(err)
		}
	}
}

func TestTeeContinuesWhenOneOutputFails(t *testing.T) {
	e := zapcore.Entry{Message: "scrape_finished", Level: zap.InfoLevel}
	var console, file bytes.Buffer
	c := NewCore(failingSink{}, zapcore.AddSync(&console), zap.InfoLevel)
	if err := c.Write(e, nil); !errors.Is(err, io.ErrClosedPipe) || !strings.Contains(console.String(), "采集完成") {
		t.Fatal("file failure suppressed console", err, console.String())
	}
	c = NewCore(zapcore.AddSync(&file), failingSink{}, zap.InfoLevel)
	if err := c.Write(e, nil); !errors.Is(err, io.ErrClosedPipe) || !json.Valid(bytes.TrimSpace(file.Bytes())) {
		t.Fatal("console failure suppressed file", err, file.String())
	}
}

func TestConsoleFilteringAndMissingValues(t *testing.T) {
	var out bytes.Buffer
	l := zap.New(NewConsoleCore(zapcore.AddSync(&out), zap.InfoLevel)).Sugar()
	l.Debugw("sql_started", "sql", "hidden")
	if out.Len() != 0 {
		t.Fatal(out.String())
	}
	l.Infow("sql_top", "rows_read", nil, "budget_remaining_ms_at_start", nil, "read_state", "unconfirmed")
	l.Infow("sql_top", "rows_read", int64(0), "budget_remaining_ms_at_start", int64(0))
	if !strings.Contains(out.String(), "启动时剩余预算 无限制") || !strings.Contains(out.String(), "启动时剩余预算 0ms") || !strings.Contains(out.String(), "读取行数 未知") || !strings.Contains(out.String(), "读取行数 0") {
		t.Fatal(out.String())
	}
}
func TestConsolePanicStackAppearsOnce(t *testing.T) {
	var out bytes.Buffer
	l := zap.New(NewConsoleCore(zapcore.AddSync(&out), zap.InfoLevel)).Sugar()
	l.Errorw("collector_panic", "panic", "broken", "stack", "frame1\nframe2")
	if strings.Count(out.String(), "frame1") != 1 || strings.Count(out.String(), "\n") != 2 {
		t.Fatal(out.String())
	}
}

type benchmarkConsoleSink struct{ bytes, lines int64 }

func (s *benchmarkConsoleSink) Write(p []byte) (int, error) {
	s.bytes += int64(len(p))
	s.lines += int64(bytes.Count(p, []byte{'\n'}))
	return len(p), nil
}
func (*benchmarkConsoleSink) Sync() error { return nil }

func BenchmarkConsole(b *testing.B) {
	for _, kind := range []string{"normal", "error", "slow", "debug_started", "debug_finished"} {
		fields := []zapcore.Field{zap.String("scrape_id", "20261113T020649.558007000-1"), zap.Int64("collection_duration_ms", 55), zap.Int("runs", 36), zap.Int("sql_calls", 55), zap.Int("samples", 135)}
		message := "scrape_finished"
		if kind == "error" || kind == "slow" {
			message = "sql_error"
			if kind == "slow" {
				message = "sql_top"
			}
			fields = []zapcore.Field{zap.String("scrape_id", "20261113T020649.558007000-1"), zap.String("datasource", "prod"), zap.String("collector", "Custom"), zap.String("sql", strings.Repeat("select value;", 50)), zap.Int64("sql_duration_ms", 30000)}
		}
		level := zap.InfoLevel
		if strings.HasPrefix(kind, "debug_") {
			level = zap.DebugLevel
			fields = []zapcore.Field{zap.String("scrape_id", "20261113T020649.558007000-1"), zap.String("run_id", "R1"), zap.String("query_id", "Q1"), zap.String("datasource", "prod"), zap.String("collector", "Custom")}
			if kind == "debug_started" {
				message = "sql_started"
				fields = append(fields, zap.Int64("budget_remaining_ms_at_start", 4999), zap.String("sql", strings.Repeat("select value;", 50)))
			} else {
				message = "sql_finished"
				fields = append(fields, zap.String("result", "ok"), zap.Int64("sql_duration_ms", 1), zap.Int64("rows_read", 1), zap.String("read_scope", "first_result_set"), zap.Any("api_ms", map[string]int64{"query": 1, "next": 0}))
			}
		}
		for _, renderer := range []string{"previous", "current"} {
			b.Run(kind+"/"+renderer, func(b *testing.B) {
				sink := &benchmarkConsoleSink{}
				var core zapcore.Core
				if renderer == "previous" {
					core = zapcore.NewCore(zapcore.NewConsoleEncoder(encoderConfig(true)), sink, level)
				} else {
					core = NewConsoleCore(sink, level)
				}
				e := zapcore.Entry{Time: time.Now(), Message: message, Level: level}
				if level == zap.DebugLevel {
					e.Caller = zapcore.NewEntryCaller(0, "/app/diagnostics/query.go", 200, true)
				}
				b.ReportAllocs()
				b.ResetTimer()
				for i := 0; i < b.N; i++ {
					if err := core.Write(e, fields); err != nil {
						b.Fatal(err)
					}
				}
				b.StopTimer()
				b.ReportMetric(float64(sink.bytes)/float64(b.N), "logB/op")
				b.ReportMetric(float64(sink.lines)/float64(b.N), "lines/op")
			})
		}
	}
}
