package collector

import (
	"context"
	"dameng_exporter/config"
	"dameng_exporter/db"
	"dameng_exporter/logger"
	"database/sql"
	"database/sql/driver"
	"fmt"
	"github.com/prometheus/client_golang/prometheus"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"sync/atomic"
	"testing"
)

type benchmarkLogSink struct{ bytes, lines atomic.Int64 }

func (w *benchmarkLogSink) Write(p []byte) (int, error) {
	w.bytes.Add(int64(len(p)))
	w.lines.Add(1)
	return len(p), nil
}
func (*benchmarkLogSink) Sync() error { return nil }
func BenchmarkCollectionDiagnostics(b *testing.B) {
	for _, scenario := range []struct {
		name             string
		sources, queries int
		parallel         bool
	}{{"single", 1, 1, false}, {"multi", 4, 10, false}, {"thousand", 1, 1000, false}, {"concurrent", 4, 10, true}} {
		b.Run(scenario.name, func(b *testing.B) {
			oldLog, oldCfg, oldManager := logger.Logger, config.Global.GetConfig(), db.GlobalPoolManager
			defer func() { logger.Logger = oldLog; config.Global.Init(oldCfg); db.GlobalPoolManager = oldManager }()
			db.GlobalPoolManager = nil
			cfg := config.DefaultMultiSourceConfig
			config.Global.Init(&cfg)
			sink := &benchmarkLogSink{}
			enc := zap.NewProductionEncoderConfig()
			logger.Logger = zap.New(zapcore.NewCore(zapcore.NewJSONEncoder(enc), sink, zap.InfoLevel)).Sugar()
			var queries atomic.Int64
			conn := sql.OpenDB(sessionTestConnector{query: func(context.Context, string, []driver.NamedValue) (driver.Rows, error) {
				queries.Add(1)
				return &outcomeRows{columns: 1, values: [][]driver.Value{{int64(1)}}}, nil
			}})
			defer conn.Close()
			custom := config.CustomConfig{}
			for i := 0; i < scenario.queries; i++ {
				custom.Metrics = append(custom.Metrics, config.CustomMetric{Context: "bench", Request: "select value", MetricsDesc: map[string]string{"0": "benchmark"}, MetricsType: map[string]string{"0": "gauge"}})
			}
			run := func() {
				round := newCollectionRound()
				for i := 0; i < scenario.sources; i++ {
					p := &db.DataSourcePool{Name: fmt.Sprint(i), DB: conn}
					c := NewCustomMetrics(conn, custom)
					collectWithLogging(make(chan prometheus.Metric, 1), p, c, round, false, 0)
				}
				round.finish(nil)
			}
			b.ReportAllocs()
			b.ResetTimer()
			if scenario.parallel {
				b.RunParallel(func(pb *testing.PB) {
					for pb.Next() {
						run()
					}
				})
			} else {
				for i := 0; i < b.N; i++ {
					run()
				}
			}
			b.StopTimer()
			b.ReportMetric(float64(queries.Load())/float64(b.N), "sql/op")
			b.ReportMetric(float64(sink.lines.Load())/float64(b.N), "logs/op")
			b.ReportMetric(float64(sink.bytes.Load())/float64(b.N), "logB/op")
		})
	}
}
