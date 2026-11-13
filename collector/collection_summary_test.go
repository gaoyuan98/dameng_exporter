package collector

import (
	"dameng_exporter/config"
	"dameng_exporter/logger"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest/observer"
	"testing"
)

func TestSummaryCountsIndependentOfLogLevel(t *testing.T) {
	setupCollectionLogTest(t)
	cfg := config.DefaultMultiSourceConfig
	cfg.ScrapeSlowThresholdMs = 0
	config.Global.Init(&cfg)
	for _, level := range []zap.AtomicLevel{zap.NewAtomicLevelAt(zap.InfoLevel), zap.NewAtomicLevelAt(zap.DebugLevel), zap.NewAtomicLevelAt(zap.ErrorLevel)} {
		core, logs := observer.New(level)
		logger.Logger = zap.New(core).Sugar()
		round := newCollectionRound()
		for i := 0; i < 1000; i++ {
			run := newCollectionRun(round, "database", "test", "empty", "blocking", 0, "")
			run.finish(round, 0, 0, workerResult{execution: "completed"})
		}
		level.SetLevel(zap.InfoLevel)
		round.finish(nil)
		summary := logs.FilterMessage("scrape_finished").All()[0].ContextMap()
		if summary["runs"] != int64(1000) || summary["issue_runs"] != int64(0) {
			t.Fatal(summary)
		}
		for _, key := range []string{"slowest", "zero_sample_collectors", "unknown_runs", "observed_error_logs"} {
			if _, exists := summary[key]; exists {
				t.Fatal("obsolete field", key)
			}
		}
		if logs.FilterMessage("scrape_started").Len() == 0 && logs.Len() != 1 {
			t.Fatal("normal INFO must contain one summary")
		}
	}
}
