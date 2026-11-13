package collector

import (
	"context"
	"dameng_exporter/config"
	"dameng_exporter/db"
	"dameng_exporter/logger"
	"database/sql"
	"database/sql/driver"
	"github.com/prometheus/client_golang/prometheus"
	"go.uber.org/zap"
	"os"
	"runtime"
	"testing"
)

func TestCompletedRoundRetainedMemory(t *testing.T) {
	if os.Getenv("DIAGNOSTICS_RETAINED_REPORT") == "" {
		t.Skip("retained-memory measurement is opt-in")
	}
	setupCollectionLogTest(t)
	logger.Logger = zap.NewNop().Sugar()
	cfg := config.DefaultMultiSourceConfig
	config.Global.Init(&cfg)
	conn := sql.OpenDB(sessionTestConnector{query: func(context.Context, string, []driver.NamedValue) (driver.Rows, error) {
		return &outcomeRows{columns: 1, values: [][]driver.Value{{int64(1)}}}, nil
	}})
	defer conn.Close()
	for _, n := range []int{1, 1000} {
		custom := config.CustomConfig{}
		for i := 0; i < n; i++ {
			custom.Metrics = append(custom.Metrics, config.CustomMetric{Context: "memory", Request: "select value", MetricsDesc: map[string]string{"0": "fixture"}})
		}
		rounds := make([]*collectionRound, 0, 100)
		runtime.GC()
		var before, after runtime.MemStats
		runtime.ReadMemStats(&before)
		for i := 0; i < 100; i++ {
			round := newCollectionRound()
			collectWithLogging(make(chan prometheus.Metric, 1), &db.DataSourcePool{Name: "memory", DB: conn}, NewCustomMetrics(conn, custom), round, false, 0)
			round.finish(nil)
			rounds = append(rounds, round)
		}
		runtime.GC()
		runtime.ReadMemStats(&after)
		runtime.KeepAlive(rounds)
		t.Logf("100 completed rounds, %d SQL/round: retained heap delta %d bytes (%d bytes/round); includes Go runtime and pool noise", n, int64(after.HeapAlloc)-int64(before.HeapAlloc), (int64(after.HeapAlloc)-int64(before.HeapAlloc))/100)
	}
}
