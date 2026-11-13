package collector

import (
	"dameng_exporter/config"
	"dameng_exporter/db"
	"github.com/prometheus/client_golang/prometheus"
	"go.uber.org/zap/zaptest/observer"
	"testing"
	"time"
)

func awaitLog(t *testing.T, logs *observer.ObservedLogs, event string) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for logs.FilterMessage(event).Len() == 0 && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if logs.FilterMessage(event).Len() == 0 {
		t.Fatalf("missing %s", event)
	}
}
func awaitDone(t *testing.T, done <-chan struct{}) {
	t.Helper()
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("collector did not finish")
	}
}
func TestWorkerCompletionWinsReadyDeadline(t *testing.T) {
	for i := 0; i < 100; i++ {
		done, deadline := make(chan workerResult, 1), make(chan time.Time, 1)
		done <- workerResult{execution: "completed"}
		deadline <- time.Now()
		r, timeout := waitForWorker(done, deadline)
		if timeout || r.execution != "completed" {
			t.Fatal("ready completion lost")
		}
	}
}
func TestPublicCollectorPanicAndSharedInstances(t *testing.T) {
	logs := setupCollectionLogTest(t)
	c := &loggingTestCollector{collect: func(*loggingTestCollector, chan<- prometheus.Metric) { panic("public panic") }}
	reg := prometheus.NewRegistry()
	reg.MustRegister(&scrapeCollector{children: []prometheus.Collector{NewBuildInfoCollector(), c}})
	for i := 0; i < 2; i++ {
		if _, err := reg.Gather(); err != nil {
			t.Fatal(err)
		}
	}
	if c.trace != nil || c.collectionLogger != nil {
		t.Fatal("shared instance mutated")
	}
	for _, e := range logs.FilterMessage("scrape_finished").All() {
		if e.ContextMap()["issue_runs"] != int64(1) || e.ContextMap()["runs"] != int64(2) {
			t.Fatal(e)
		}
	}
}
func TestFastCountsAndPreflight(t *testing.T) {
	logs := setupCollectionLogTest(t)
	for _, buffer := range []int{0, 20} {
		round := newCollectionRound()
		c := &loggingTestCollector{collect: func(_ *loggingTestCollector, ch chan<- prometheus.Metric) {
			for i := 0; i < 20; i++ {
				ch <- prometheus.MustNewConstMetric(logTestDesc, prometheus.GaugeValue, 1)
			}
		}}
		collectWithLogging(make(chan prometheus.Metric, buffer), testLogPool(t, "fast", nil), c, round, true, 0)
		round.finish(nil)
		f := logs.FilterMessage("scrape_finished").All()[buffer/20].ContextMap()
		if f["samples"] != int64(buffer) || f["dropped_samples"] != int64(20-buffer) {
			t.Fatal(f)
		}
	}
	round := newCollectionRound()
	collectWithLogging(make(chan prometheus.Metric), &db.DataSourcePool{Name: "offline"}, &loggingTestCollector{collect: func(*loggingTestCollector, chan<- prometheus.Metric) { t.Error("failed preflight ran business") }}, round, true, time.Second)
	round.finish(nil)
	if logs.FilterMessage("scrape_finished").All()[2].ContextMap()["skipped_runs"] != int64(1) {
		t.Fatal("missing skip")
	}
	f := logs.FilterMessage("collector_finished").All()[2].ContextMap()
	if f["run_state"] != "skipped" || f["skip_reason"] != "数据库连接未初始化" {
		t.Fatal("missing preflight skip reason", f)
	}
}
func TestCustomCollectionKeepsBlockingInGlobalFastMode(t *testing.T) {
	logs := setupCollectionLogTest(t)
	cfg := config.DefaultMultiSourceConfig
	cfg.CollectionMode = "fast"
	config.Global.Init(&cfg)
	config.GlobalMultiConfig = &cfg
	pool := testLogPool(t, "custom", nil)
	adapter := NewCustomMetricsMultiSourceAdapter(nil)
	round := newCollectionRound()
	adapter.collectPool(make(chan prometheus.Metric, 1), pool, &config.CustomConfig{}, round)
	round.finish(nil)
	f := logs.FilterMessage("collector_finished").All()[0].ContextMap()
	if f["mode"] != "blocking" || f["timeout_ms"] != int64(0) {
		t.Fatal(f)
	}
}
