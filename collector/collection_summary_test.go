package collector

import (
	"dameng_exporter/config"
	"dameng_exporter/logger"
	"encoding/json"
	"fmt"
	"testing"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"go.uber.org/zap/zaptest/observer"
)

func TestZeroSampleSummaryIsBoundedAndDetailsRecoverable(t *testing.T) {
	setupCollectionLogTest(t)
	cfg := config.DefaultMultiSourceConfig
	cfg.CollectorSlowThresholdMs = 0
	config.Global.Init(&cfg)
	for _, level := range []zapcore.Level{zap.InfoLevel, zap.DebugLevel} {
		for _, count := range []int{0, 1, 20, 21, 1000} {
			for _, failed := range []bool{false, true} {
				if failed && count <= 20 {
					continue
				}
				t.Run(fmt.Sprintf("%s/%d/failed=%t", level, count, failed), func(t *testing.T) {
					core, logs := observer.New(level)
					logger.Logger = zap.New(core).Sugar()
					round := newCollectionRound()
					for i := count - 1; i >= 0; i-- {
						run := newCollectionRun(round, "database", "source", fmt.Sprintf("C%04d", i), "blocking", 0, "")
						outcome := unknownOutcome()
						if failed && i == count-1 {
							outcome = databaseOutcome(OutcomeFailed, "rows_error")
						}
						run.finish(round, 0, 0, workerResult{execution: "completed", outcome: outcome})
					}
					round.finish(nil)
					summary := logs.FilterMessage("scrape_finished").All()[0]
					fields := summary.ContextMap()
					zeros := fields["zero_sample_collectors"].([]zeroSampleCollector)
					shown := min(count, 20)
					if len(zeros) != shown || fields["zero_sample_runs"] != int64(count) || fields["zero_sample_collectors_omitted"] != int64(count-shown) || fields["recorded_runs"] != int64(count) {
						t.Fatalf("incorrect bounded summary: %v", fields)
					}
					for i, zero := range zeros {
						if zero.Collector != fmt.Sprintf("C%04d", i) {
							t.Fatalf("unstable order: %v", zeros)
						}
					}
					if len(fields["slowest"].([]slowestCollector)) != min(count, 3) {
						t.Fatal("truncation changed slowest entries")
					}
					failures := int64(0)
					if failed {
						failures = 1
						if logs.FilterMessage("collector_finished").FilterLevelExact(zap.WarnLevel).Len() != 1 {
							t.Fatal("omitted failure lost its WARN detail")
						}
					}
					if fields["known_failed_runs"] != failures || fields["unknown_runs"] != int64(count)-failures || (summary.Level == zap.WarnLevel) != failed {
						t.Fatalf("truncation changed severity/counts: %v", fields)
					}
					if level == zap.DebugLevel && logs.FilterMessage("collector_finished").Len() != count {
						t.Fatal("DEBUG cannot recover every zero-sample run")
					}
					if level == zap.InfoLevel && count == 1000 && !failed {
						bounded, err := json.Marshal(fields)
						if err != nil {
							t.Fatal(err)
						}
						full := make([]zeroSampleCollector, 0, count)
						for _, r := range round.results {
							full = append(full, zeroSampleCollector{r.Scope, r.Datasource, r.Collector, r.Execution, r.Outcome, r.Reason})
						}
						fields["zero_sample_collectors"] = full
						unbounded, err := json.Marshal(fields)
						if err != nil {
							t.Fatal(err)
						}
						t.Logf("1000 synthetic zero-sample runs: summary JSON fields %d bytes bounded, %d bytes with full list", len(bounded), len(unbounded))
					}
				})
			}
		}
	}
}
