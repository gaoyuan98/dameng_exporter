package collector

import (
	"context"
	"dameng_exporter/config"
	"dameng_exporter/db"
	"dameng_exporter/logger"
	"database/sql"
	"database/sql/driver"
	"errors"
	"reflect"
	"sync"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"go.uber.org/zap/zaptest/observer"
)

type resultTestCollector struct {
	loggingTestCollector
	outcome                  CollectorOutcome
	legacyCalls, resultCalls int
}

func (c *resultTestCollector) Collect(ch chan<- prometheus.Metric) {
	c.legacyCalls++
	c.loggingTestCollector.Collect(ch)
}
func (c *resultTestCollector) CollectWithResult(ch chan<- prometheus.Metric) CollectorOutcome {
	c.resultCalls++
	c.loggingTestCollector.Collect(ch)
	return c.outcome
}
func testResultCollector(action func(*loggingTestCollector, chan<- prometheus.Metric)) *resultTestCollector {
	return &resultTestCollector{loggingTestCollector: loggingTestCollector{collect: action}, outcome: databaseOutcome(OutcomeSuccess, "")}
}
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

func TestResultDispatchAndFastCounters(t *testing.T) {
	setupCollectionLogTest(t)
	for _, readable := range []bool{false, true} {
		round := newCollectionRound()
		c := testResultCollector(func(_ *loggingTestCollector, ch chan<- prometheus.Metric) {
			for i := 0; i < 20; i++ {
				ch <- prometheus.MustNewConstMetric(logTestDesc, prometheus.GaugeValue, float64(i))
			}
		})
		out := make(chan prometheus.Metric)
		if readable {
			out = make(chan prometheus.Metric, 20)
		}
		collectWithLogging(out, testLogPool(t, "test", nil), c, round, true, time.Second)
		r := round.results[0]
		if c.legacyCalls != 0 || c.resultCalls != 1 || r.Outcome != OutcomeSuccess || r.Execution != "completed" || r.Samples+r.DroppedSamples != 20 {
			t.Fatalf("incorrect dispatch/counts: %+v", r)
		}
		if readable && (r.Samples != 20 || r.DroppedSamples != 0) || !readable && (r.Samples != 0 || r.DroppedSamples != 20 || !r.knownFailed()) {
			t.Fatal(r)
		}
	}
}

func TestLegacyOutcomesDoNotDependOnBusinessLogsOrSamples(t *testing.T) {
	setupCollectionLogTest(t)
	for _, action := range []func(*loggingTestCollector, chan<- prometheus.Metric){
		func(*loggingTestCollector, chan<- prometheus.Metric) {},
		func(_ *loggingTestCollector, ch chan<- prometheus.Metric) {
			ch <- prometheus.MustNewConstMetric(logTestDesc, prometheus.GaugeValue, 1)
		},
		func(c *loggingTestCollector, _ chan<- prometheus.Metric) { c.log().Error("business failed") },
	} {
		round := newCollectionRound()
		collectWithLogging(make(chan prometheus.Metric, 1), testLogPool(t, "legacy", nil), &loggingTestCollector{collect: action}, round, false, time.Second)
		r := round.results[0]
		if r.Outcome != OutcomeUnknown || r.Reason != "legacy_unclassified" || r.Execution != "completed" {
			t.Fatal(r)
		}
	}
}

func TestFrontendSnapshotIsFrozenAfterBackgroundExit(t *testing.T) {
	logs := setupCollectionLogTest(t)
	round := newCollectionRound()
	release, entered := make(chan struct{}), make(chan struct{})
	var once sync.Once
	t.Cleanup(func() { once.Do(func() { close(release) }) })
	c := testResultCollector(func(c *loggingTestCollector, ch chan<- prometheus.Metric) {
		close(entered)
		<-release
		c.log().Error("late_result_error")
		for i := 0; i < 1200; i++ {
			ch <- prometheus.MustNewConstMetric(logTestDesc, prometheus.GaugeValue, 1)
		}
	})
	out := make(chan prometheus.Metric, 2)
	done := make(chan struct{})
	go func() {
		collectWithLogging(out, testLogPool(t, "late", nil), c, round, true, 30*time.Millisecond)
		close(done)
	}()
	awaitDone(t, entered)
	awaitDone(t, done)
	frozen := append([]collectionResult(nil), round.results...)
	round.finish(nil)
	close(out)
	once.Do(func() { close(release) })
	awaitLog(t, logs, "collector_background_finished")
	if !reflect.DeepEqual(round.results, frozen) || len(frozen) != 1 || frozen[0].Execution != "timeout" || frozen[0].Outcome != OutcomeUnknown || frozen[0].Errors != 0 {
		t.Fatalf("frontend changed: %+v", round.results)
	}
	final := logs.FilterMessage("collector_background_finished").All()[0].ContextMap()
	finished := logs.FilterMessage("collector_finished").All()[0].ContextMap()
	if final["outcome"] != OutcomeSuccess || final["scrape_id"] != finished["scrape_id"] || final["mode"] != "fast" || final["timeout_ms"] != int64(30) || logs.FilterMessage("collector_finished").Len() != 1 || logs.FilterMessage("collector_timeout").Len() != 0 {
		t.Fatal(final)
	}
}

func TestSlowThresholdIsIndependentOfFastBudget(t *testing.T) {
	for _, slowFirst := range []bool{true, false} {
		t.Run(map[bool]string{true: "slow first", false: "timeout first"}[slowFirst], func(t *testing.T) {
			logs := setupCollectionLogTest(t)
			cfg := config.DefaultMultiSourceConfig
			cfg.CollectorSlowThresholdMs = 15
			budget := time.Second
			if !slowFirst {
				cfg.CollectorSlowThresholdMs = 1000
				budget = 25 * time.Millisecond
			}
			config.Global.Init(&cfg)
			release, done := make(chan struct{}), make(chan struct{})
			var once sync.Once
			t.Cleanup(func() { once.Do(func() { close(release) }) })
			round := newCollectionRound()
			c := testResultCollector(func(_ *loggingTestCollector, _ chan<- prometheus.Metric) { <-release })
			go func() {
				collectWithLogging(make(chan prometheus.Metric), testLogPool(t, "slow", nil), c, round, true, budget)
				close(done)
			}()
			if slowFirst {
				awaitLog(t, logs, "collector_slow")
				select {
				case <-done:
					t.Fatal("slow warning cancelled worker")
				default:
				}
				once.Do(func() { close(release) })
			}
			awaitDone(t, done)
			if !slowFirst {
				once.Do(func() { close(release) })
				awaitLog(t, logs, "collector_background_finished")
			}
			expected := 0
			if slowFirst {
				expected = 1
			}
			if logs.FilterMessage("collector_slow").Len() != expected || round.results[0].Warnings != 0 || round.results[0].Slow != slowFirst {
				t.Fatalf("observer affected result: %+v", round.results)
			}
		})
	}
}

func TestSlowPreflightRetainsModeAndNeverStartsBusiness(t *testing.T) {
	logs := setupCollectionLogTest(t)
	cfg := config.DefaultMultiSourceConfig
	cfg.CollectorSlowThresholdMs = 15
	config.Global.Init(&cfg)
	release := make(chan struct{})
	var once sync.Once
	t.Cleanup(func() { once.Do(func() { close(release) }) })
	// Hold the real nil-connection error at its log hook, inside preflight.
	logger.Logger = logger.Logger.Desugar().WithOptions(zap.Hooks(func(e zapcore.Entry) error {
		if e.Level == zap.ErrorLevel {
			<-release
		}
		return nil
	})).Sugar()
	round := newCollectionRound()
	c := testResultCollector(func(*loggingTestCollector, chan<- prometheus.Metric) {
		t.Error("business started after failed preflight")
	})
	done := make(chan struct{})
	go func() {
		collectWithLogging(make(chan prometheus.Metric), &db.DataSourcePool{Name: "offline"}, c, round, true, 100*time.Millisecond)
		close(done)
	}()
	awaitLog(t, logs, "collector_slow")
	once.Do(func() { close(release) })
	awaitDone(t, done)
	r := round.results[0]
	if r.Execution != "skipped" || r.Outcome != OutcomeUnknown || r.Mode != "fast" || r.TimeoutMS != 100 || r.TimeoutScope != "worker" || !r.Slow || c.resultCalls != 0 {
		t.Fatal(r)
	}
}

func TestPublicCollectorPanicAndSharedInstances(t *testing.T) {
	logs := setupCollectionLogTest(t)
	cfg := config.DefaultMultiSourceConfig
	cfg.DataSources = []config.DataSourceConfig{{Name: "one", Enabled: true, DbHost: "127.0.0.1:5236"}, {Name: "two", Enabled: true, DbHost: "127.0.0.2:5236"}}
	config.Global.Init(&cfg)
	config.GlobalMultiConfig = &cfg
	manager := db.NewDBPoolManager(&cfg)
	panicCollector := &loggingTestCollector{collect: func(*loggingTestCollector, chan<- prometheus.Metric) { panic("public panic") }}
	reg := prometheus.NewRegistry()
	reg.MustRegister(&scrapeCollector{children: []prometheus.Collector{NewBuildInfoCollector(), NewDatasourceHealthCollector(manager), NewDbNodeTypeInfoCollector(manager), panicCollector}, poolManager: manager})
	var wg sync.WaitGroup
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			families, err := reg.Gather()
			if err != nil {
				t.Error(err)
			}
			for _, f := range families {
				if f.GetName() == dmdb_up || f.GetName() == dmdbms_node_type_info {
					if len(f.Metric) != 2 {
						t.Errorf("changed public samples: %v", f)
					}
				}
			}
		}()
	}
	wg.Wait()
	if panicCollector.collectionLogger != nil {
		t.Fatal("shared public instance received mutable context")
	}
	for _, summary := range logs.FilterMessage("scrape_finished").All() {
		f := summary.ContextMap()
		if f["recorded_runs"] != int64(4) || f["known_failed_runs"] != int64(1) {
			t.Fatal(f)
		}
	}
	for _, event := range logs.FilterMessage("collector_finished").All() {
		f := event.ContextMap()
		if f["scope"] != "exporter" || f["datasource"] != "" || f["mode"] != "direct" || f["timeout_ms"] != int64(0) {
			t.Fatal(f)
		}
	}
}

func TestHostProcessErrorsStayWithDatasource(t *testing.T) {
	logs := setupCollectionLogTest(t)
	round := newCollectionRound()
	entered := make(chan struct{}, 2)
	release := make(chan struct{})
	var wg sync.WaitGroup
	for _, source := range []string{"one", "two"} {
		conn := sql.OpenDB(sessionTestConnector{query: func(context.Context, string, []driver.NamedValue) (driver.Rows, error) {
			return &sessionTestRows{values: [][]driver.Value{{"/dm", "42"}}}, nil
		}})
		t.Cleanup(func() { conn.Close() })
		p := &db.DataSourcePool{Name: source, DB: conn, Labels: map[string]string{"datasource": source + "-label"}}
		c := NewDmapProcessCollector(conn)
		c.dataSource, c.lastPID, c.localInstallBinPath = source, "42", "/dm/bin"
		first := true
		c.processOutput = func(string) ([]byte, error) {
			if first {
				first = false
				entered <- struct{}{}
				<-release
			}
			return nil, errors.New("command failed")
		}
		wg.Add(1)
		go func() {
			defer wg.Done()
			out := make(chan prometheus.Metric, 5)
			collectWithLogging(out, p, c, round, false, 0)
			if len(out) != 5 {
				t.Error("process failure changed zero samples")
			}
			for len(out) > 0 {
				m := <-out
				value := &dto.Metric{}
				m.Write(value)
				if value.GetGauge().GetValue() != 0 {
					t.Error("changed process failure value")
				}
			}
		}()
	}
	for i := 0; i < 2; i++ {
		select {
		case <-entered:
		case <-time.After(3 * time.Second):
			t.Fatal("process commands did not overlap")
		}
	}
	close(release)
	wg.Wait()
	for _, r := range round.results {
		if r.Scope != "database" || r.Datasource == "" || r.Outcome != OutcomeUnknown || r.Errors != 5 {
			t.Fatal(r)
		}
	}
	for _, e := range logs.FilterMessage("Error checking process").All() {
		f := e.ContextMap()
		if f["scrape_id"] == nil || f["datasource_label"] != f["datasource"].(string)+"-label" || f["collector"] != "DmapProcess" || f["process_name"] == nil {
			t.Fatal(f)
		}
	}
}

func TestShortSummaryKeepsZeroSamplesWithinLimitAndStableTopThree(t *testing.T) {
	logs := setupCollectionLogTest(t)
	round := newCollectionRound()
	// The fourth run is fast, silent, unclassified and absent from top three.
	round.results = []collectionResult{
		{Scope: "database", Datasource: "b", Collector: "C", DurationMS: 100, Outcome: OutcomeSuccess, Execution: "completed", Mode: "blocking", TimeoutScope: "none", Samples: 1},
		{Scope: "database", Datasource: "a", Collector: "B", DurationMS: 100, Outcome: OutcomeSuccess, Execution: "completed", Mode: "fast", TimeoutMS: 50, TimeoutScope: "worker", Samples: 1},
		{Scope: "exporter", Collector: "A", DurationMS: 100, Outcome: OutcomeUnknown, Execution: "completed", Mode: "direct", TimeoutScope: "none", Samples: 1},
		{Scope: "database", Datasource: "a", Collector: "DbDual", DurationMS: 1, Outcome: OutcomeUnknown, Reason: "legacy_unclassified", Execution: "completed"},
		{Scope: "database", Datasource: "a", Collector: "Empty", Outcome: OutcomeEmpty, Execution: "completed"},
	}
	round.finish([]string{})
	summary := logs.FilterMessage("scrape_finished").All()[0]
	f := summary.ContextMap()
	if summary.Level != zap.InfoLevel || f["recorded_runs"] != int64(5) || f["unknown_runs"] != int64(2) || f["zero_sample_runs"] != int64(2) || f["known_failed_runs"] != int64(0) {
		t.Fatal(f)
	}
	if _, ok := f["results"]; ok {
		t.Fatal("normal results replayed")
	}
	zeros := f["zero_sample_collectors"].([]zeroSampleCollector)
	if len(zeros) != 2 || zeros[0].Collector != "DbDual" || zeros[0].Outcome != OutcomeUnknown || zeros[1].Outcome != OutcomeEmpty {
		t.Fatal(zeros)
	}
	top := f["slowest"].([]slowestCollector)
	if len(top) != 3 || top[0].Collector != "A" || top[1].Collector != "B" || top[2].Collector != "C" || top[1].TimeoutMS != 50 || top[1].Mode != "fast" {
		t.Fatal(top)
	}
	// Failed/partial/timeout/drops overlap: count each run once; logs stay independent.
	round.results = []collectionResult{
		{Outcome: OutcomeFailed, Execution: "panic", DroppedSamples: 1, Errors: 2},
		{Outcome: OutcomePartial, Execution: "completed"},
		{Outcome: OutcomeUnknown, Execution: "timeout"},
		{Outcome: OutcomeUnknown, Execution: "skipped"},
		{Outcome: OutcomeSuccess, Execution: "completed", DroppedSamples: 2},
	}
	round.finish([]string{"z", "a", "z"})
	f = logs.FilterMessage("scrape_finished").All()[1].ContextMap()
	if f["known_failed_runs"] != int64(4) || f["observed_error_runs"] != int64(1) || f["unknown_runs"] != int64(2) || f["skipped_runs"] != int64(1) || !reflect.DeepEqual(f["unavailable_datasources"], []interface{}{"a", "z"}) {
		t.Fatal(f)
	}
}

func TestNormalInfoHasOnlyRoundEventsAndSlowZeroIsDisabled(t *testing.T) {
	setupCollectionLogTest(t)
	core, logs := observer.New(zap.InfoLevel)
	logger.Logger = zap.New(core).Sugar()
	cfg := config.DefaultMultiSourceConfig
	cfg.CollectorSlowThresholdMs = 0
	config.Global.Init(&cfg)
	c := &loggingTestAdapter{pool: testLogPool(t, "normal", nil), factory: func() MetricCollector {
		return testResultCollector(func(_ *loggingTestCollector, ch chan<- prometheus.Metric) {
			time.Sleep(25 * time.Millisecond)
			ch <- prometheus.MustNewConstMetric(logTestDesc, prometheus.GaugeValue, 42)
		})
	}}
	reg := prometheus.NewRegistry()
	reg.MustRegister(&scrapeCollector{children: []prometheus.Collector{c}})
	if _, err := reg.Gather(); err != nil {
		t.Fatal(err)
	}
	if logs.Len() != 2 || logs.All()[0].Message != "scrape_started" || logs.All()[1].Message != "scrape_finished" || logs.All()[1].Level != zap.InfoLevel || logs.All()[1].ContextMap()["slow_runs"] != int64(0) {
		t.Fatal(logs.All())
	}
}

func TestBusinessOutcomesAndSamplesDoNotDependOnLogLevel(t *testing.T) {
	setupCollectionLogTest(t)
	for _, f := range businessFixtures() {
		for _, fail := range []bool{false, true} {
			var expected []string
			var expectedOutcome OutcomeState
			for i, level := range []zapcore.Level{zap.InfoLevel, zap.ErrorLevel, zap.DebugLevel} {
				core, _ := observer.New(level)
				logger.Logger = zap.New(core).Sugar()
				resetFixtureCache(t, f)
				var queryErr error
				if fail {
					queryErr = errors.New("permission denied")
				}
				conn, calls := fixtureDB(t, f, [][]driver.Value{f.row}, queryErr, nil, nil)
				round := newCollectionRound()
				out := make(chan prometheus.Metric, 100)
				collectWithLogging(out, &db.DataSourcePool{Name: "fixture", DB: conn}, fixtureCollector(f, conn), round, false, time.Millisecond)
				actual := metricSnapshot(t, out)
				r := round.results[0]
				if *calls != 1 {
					t.Fatal("extra query")
				}
				if i == 0 {
					expected = actual
					expectedOutcome = r.Outcome
				} else if !reflect.DeepEqual(actual, expected) || r.Outcome != expectedOutcome {
					t.Fatalf("level changed %s: %+v", f.name, r)
				}
			}
		}
	}
}

func TestCustomCollectionKeepsBlockingInGlobalFastMode(t *testing.T) {
	logs := setupCollectionLogTest(t)
	cfg := config.DefaultMultiSourceConfig
	cfg.CollectionMode = "fast"
	cfg.CollectorSlowThresholdMs = 15
	config.Global.Init(&cfg)
	config.GlobalMultiConfig = &cfg
	entered, release, done := make(chan struct{}), make(chan struct{}), make(chan struct{})
	var once sync.Once
	t.Cleanup(func() { once.Do(func() { close(release) }) })
	conn := sql.OpenDB(sessionTestConnector{query: func(context.Context, string, []driver.NamedValue) (driver.Rows, error) {
		close(entered)
		<-release
		return &outcomeRows{columns: 1, values: [][]driver.Value{{float64(7)}}}, nil
	}})
	t.Cleanup(func() { conn.Close() })
	custom := &config.CustomConfig{Metrics: []config.CustomMetric{{Context: "test_custom", Request: "select value", MetricsDesc: map[string]string{"0": "test"}, MetricsType: map[string]string{"0": "gauge"}}}}
	adapter := NewCustomMetricsMultiSourceAdapter(nil)
	round := newCollectionRound()
	out := make(chan prometheus.Metric, 1)
	go func() {
		adapter.collectPool(out, &db.DataSourcePool{Name: "custom", DB: conn}, custom, round)
		close(done)
	}()
	awaitDone(t, entered)
	awaitLog(t, logs, "collector_slow")
	select {
	case <-done:
		t.Fatal("custom returned before query finished")
	default:
	}
	once.Do(func() { close(release) })
	awaitDone(t, done)
	r := round.results[0]
	if r.Mode != "blocking" || r.TimeoutMS != 0 || r.TimeoutScope != "none" || r.Execution != "completed" || len(out) != 1 || r.Samples != 1 || r.Outcome != OutcomeUnknown {
		t.Fatal(r)
	}
}

func TestWorkerCompletionWinsReadyDeadline(t *testing.T) {
	for i := 0; i < 50; i++ {
		done := make(chan workerResult, 1)
		deadline := make(chan time.Time, 1)
		done <- workerResult{execution: "completed", outcome: databaseOutcome(OutcomeSuccess, "")}
		deadline <- time.Now()
		result, timedOut := waitForWorker(done, deadline)
		if timedOut || result.outcome.State != OutcomeSuccess {
			t.Fatal("available result lost to deadline")
		}
	}
	deadline := make(chan time.Time, 1)
	deadline <- time.Now()
	if _, timedOut := waitForWorker(make(chan workerResult, 1), deadline); !timedOut {
		t.Fatal("missing timeout")
	}
}

func TestFastPolicyAtEveryLogLevel(t *testing.T) {
	setupCollectionLogTest(t)
	for _, level := range []zapcore.Level{zap.InfoLevel, zap.ErrorLevel, zap.DebugLevel} {
		core, _ := observer.New(level)
		logger.Logger = zap.New(core).Sugar()
		for _, buffer := range []int{0, 20} {
			c := testResultCollector(func(_ *loggingTestCollector, ch chan<- prometheus.Metric) {
				for i := 0; i < 20; i++ {
					ch <- prometheus.MustNewConstMetric(logTestDesc, prometheus.GaugeValue, 1)
				}
			})
			round := newCollectionRound()
			collectWithLogging(make(chan prometheus.Metric, buffer), testLogPool(t, "fast", nil), c, round, true, 0)
			r := round.results[0]
			if r.Execution != "completed" || r.Outcome != OutcomeSuccess || r.Samples != int64(buffer) || r.DroppedSamples != int64(20-buffer) || r.TimeoutMS != 0 || r.TimeoutScope != "none" {
				t.Fatalf("%s: %+v", level, r)
			}
		}
	}
}
