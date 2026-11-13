package collector

import (
	"context"
	"dameng_exporter/config"
	"dameng_exporter/db"
	"dameng_exporter/logger"
	"database/sql"
	"database/sql/driver"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest/observer"
)

func setupCollectionLogTest(t *testing.T) *observer.ObservedLogs {
	t.Helper()
	oldLog, oldManager, oldCfg := logger.Logger, db.GlobalPoolManager, config.Global.GetConfig()
	oldMultiCfg := config.GlobalMultiConfig
	core, logs := observer.New(zap.DebugLevel)
	logger.Logger = zap.New(core).Sugar()
	db.GlobalPoolManager = nil
	config.Global.Init(nil)
	config.GlobalMultiConfig = nil
	t.Cleanup(func() {
		logger.Logger = oldLog
		db.GlobalPoolManager = oldManager
		config.Global.Init(oldCfg)
		config.GlobalMultiConfig = oldMultiCfg
	})
	return logs
}

var logTestDesc = prometheus.NewDesc("collection_log_test_value", "Test collection", nil, nil)

type loggingTestCollector struct {
	collectorLog
	collect func(*loggingTestCollector, chan<- prometheus.Metric)
}

func (*loggingTestCollector) Describe(ch chan<- *prometheus.Desc)   { ch <- logTestDesc }
func (c *loggingTestCollector) Collect(ch chan<- prometheus.Metric) { c.collect(c, ch) }

type loggingTestAdapter struct {
	pool    *db.DataSourcePool
	factory func() MetricCollector
}

func (a *loggingTestAdapter) Describe(ch chan<- *prometheus.Desc) { a.factory().Describe(ch) }
func (a *loggingTestAdapter) Collect(ch chan<- prometheus.Metric) { a.collectWithRound(ch, nil) }
func (a *loggingTestAdapter) collectWithRound(ch chan<- prometheus.Metric, round *collectionRound) {
	collectWithLogging(ch, a.pool, a.factory(), round, false, time.Second)
}

func testLogPool(t *testing.T, source string, queryErr error) *db.DataSourcePool {
	t.Helper()
	conn := sql.OpenDB(sessionTestConnector{query: func(context.Context, string, []driver.NamedValue) (driver.Rows, error) {
		if queryErr != nil {
			return nil, queryErr
		}
		return &sessionTestRows{}, nil
	}})
	t.Cleanup(func() { conn.Close() })
	return &db.DataSourcePool{Name: source, DB: conn, Labels: map[string]string{"datasource": source}}
}

func TestLicenseDateParseFailureIsReported(t *testing.T) {
	logs := setupCollectionLogTest(t)
	conn := sql.OpenDB(sessionTestConnector{query: func(context.Context, string, []driver.NamedValue) (driver.Rows, error) {
		return &outcomeRows{columns: 1, values: [][]driver.Value{{"invalid-date"}}}, nil
	}})
	t.Cleanup(func() { conn.Close() })

	round := newCollectionRound()
	out := make(chan prometheus.Metric, 1)
	collectWithLogging(out, &db.DataSourcePool{Name: "license", DB: conn}, NewDbLicenseCollector(conn), round, false, 0)
	round.finish(nil)

	errors := logs.FilterMessage("collector_error").All()
	if len(errors) != 1 || errors[0].ContextMap()["reason"] != "license_date_parse" {
		t.Fatalf("missing license parse error: %v", errors)
	}
	if len(out) != 0 || logs.FilterMessage("scrape_finished").All()[0].ContextMap()["issue_runs"] != int64(1) {
		t.Fatal("invalid license date must omit the metric and mark the round as failed")
	}
}

func TestStandaloneCollectorReportsCacheParseFailure(t *testing.T) {
	logs := setupCollectionLogTest(t)
	f := businessFixtures()[0]
	resetFixtureCache(t, f)
	config.SetCache(f.cacheKey+"_fixture", "{", time.Minute)
	conn, calls := fixtureDB(t, f, [][]driver.Value{f.row}, nil, nil, nil)
	reg := prometheus.NewRegistry()
	reg.MustRegister(fixtureCollector(f, conn))

	families, err := reg.Gather()
	if err != nil {
		t.Fatal(err)
	}
	if *calls != 1 || len(families) != 3 {
		t.Fatalf("database fallback changed: queries=%d, families=%d", *calls, len(families))
	}
	errors := logs.FilterMessage("collector_error").All()
	if len(errors) != 1 || errors[0].ContextMap()["reason"] != "business_validation" {
		t.Fatalf("standalone cache parse failure was not logged: %v", errors)
	}
}

func TestConcurrentScrapesKeepErrorsAndSummaryTogether(t *testing.T) {
	logs := setupCollectionLogTest(t)
	entered, release := make(chan struct{}, 2), make(chan struct{})
	var releaseOnce sync.Once
	t.Cleanup(func() { releaseOnce.Do(func() { close(release) }) })
	good := &loggingTestAdapter{pool: testLogPool(t, "healthy", nil), factory: func() MetricCollector {
		return &loggingTestCollector{collect: func(c *loggingTestCollector, ch chan<- prometheus.Metric) {
			entered <- struct{}{}
			<-release
			ch <- prometheus.MustNewConstMetric(logTestDesc, prometheus.GaugeValue, 42)
		}}
	}}
	badPool := testLogPool(t, "permission_denied", errors.New("permission denied on V$SESSIONS"))
	bad := &loggingTestAdapter{pool: badPool, factory: func() MetricCollector {
		c := NewDBSessionsStatusCollector(badPool.DB)
		SetDataSourceIfSupported(c, badPool.Name)
		return c
	}}
	reg := prometheus.NewRegistry()
	reg.MustRegister(&scrapeCollector{children: []prometheus.Collector{good, bad}})
	type response struct {
		families []*dto.MetricFamily
		err      error
	}
	responses := make(chan response, 2)
	for i := 0; i < 2; i++ {
		go func() { f, e := reg.Gather(); responses <- response{f, e} }()
	}
	for i := 0; i < 2; i++ {
		select {
		case <-entered:
		case <-time.After(2 * time.Second):
			t.Fatal("scrapes did not run concurrently")
		}
	}
	releaseOnce.Do(func() { close(release) })
	for i := 0; i < 2; i++ {
		r := <-responses
		if r.err != nil {
			t.Fatal(r.err)
		}
		if len(r.families) != 1 || len(r.families[0].Metric) != 1 {
			t.Fatalf("unexpected metrics: %v", r.families)
		}
		metric := r.families[0].Metric[0]
		if metric.GetGauge().GetValue() != 42 || len(metric.Label) != 1 || metric.Label[0].GetName() != "datasource" {
			t.Fatalf("diagnostics changed metrics: %v", metric)
		}
	}
	summaries := logs.FilterMessage("scrape_finished").All()
	if len(summaries) != 2 {
		t.Fatalf("want 2 round summaries, got %d", len(summaries))
	}
	ids := map[string]bool{}
	for _, entry := range summaries {
		fields := entry.ContextMap()
		id := fields["scrape_id"].(string)
		if ids[id] {
			t.Fatal("concurrent rounds share ID")
		}
		ids[id] = true
		if _, ok := fields["results"]; ok {
			t.Fatal("summary replays all results")
		}
		finished := logs.FilterMessage("collector_finished").FilterField(zap.String("scrape_id", id)).All()
		if len(finished) != 2 {
			t.Fatalf("round contains %d runs", len(finished))
		}
		for _, entry := range finished {
			f := entry.ContextMap()
			if f["datasource"] == "permission_denied" && (f["issue"] != true || f["samples"] != int64(0)) {
				t.Fatalf("query error lost: %v", f)
			}
		}
		if fields["samples"] != int64(1) {
			t.Fatalf("wrong metric count: %v", fields)
		}
	}
	for _, entry := range logs.FilterLevelExact(zap.ErrorLevel).All() {
		fields := entry.ContextMap()
		if !ids[fields["scrape_id"].(string)] || fields["datasource"] != "permission_denied" || fields["collector"] != "DBSessionsStatus" {
			t.Fatalf("uncorrelated error: %v", fields)
		}
	}
}

func TestNoMetricsIsNotAnErrorAndPanicIsReported(t *testing.T) {
	logs := setupCollectionLogTest(t)
	pool := testLogPool(t, "test", nil)
	round := newCollectionRound()
	for _, action := range []func(*loggingTestCollector, chan<- prometheus.Metric){
		func(*loggingTestCollector, chan<- prometheus.Metric) {},
		func(*loggingTestCollector, chan<- prometheus.Metric) { panic("test panic") },
	} {
		collectWithLogging(make(chan prometheus.Metric, 1), pool, &loggingTestCollector{collect: action}, round, false, time.Second)
	}
	round.finish(nil)
	if logs.FilterMessage("collector_panic").Len() != 1 || logs.FilterMessage("scrape_finished").All()[0].ContextMap()["issue_runs"] != int64(1) {
		t.Fatal("panic fact lost")
	}

}

func TestFastTimeoutStopsForwardingAndRetainsLateErrorID(t *testing.T) {
	logs := setupCollectionLogTest(t)
	pool := testLogPool(t, "slow", nil)
	round := newCollectionRound()
	release, exited := make(chan struct{}), make(chan struct{})
	var releaseOnce sync.Once
	t.Cleanup(func() { releaseOnce.Do(func() { close(release) }) })
	collector := &loggingTestCollector{collect: func(c *loggingTestCollector, ch chan<- prometheus.Metric) {
		defer close(exited)
		ch <- prometheus.MustNewConstMetric(logTestDesc, prometheus.GaugeValue, 1)
		<-release
		c.businessError("late_query_error", errors.New("late"))
		ch <- prometheus.MustNewConstMetric(logTestDesc, prometheus.GaugeValue, 2)
	}}
	out := make(chan prometheus.Metric, 5)
	collectWithLogging(out, pool, collector, round, true, 20*time.Millisecond)

	round.finish(nil)
	// Like Registry.Gather, close the output after Collect returns. Late sends would panic.
	close(out)
	releaseOnce.Do(func() { close(release) })
	select {
	case <-exited:
	case <-time.After(time.Second):
		t.Fatal("background collector was not drained")
	}
	late := logs.FilterMessage("collector_error").All()
	if len(late) != 1 || late[0].ContextMap()["scrape_id"] != logs.FilterMessage("scrape_finished").All()[0].ContextMap()["scrape_id"] {
		t.Fatal("late error lost original round")
	}
	// Wait until the deferred worker log completes before restoring the shared test logger.
	deadline := time.Now().Add(time.Second)
	for logs.FilterMessage("collector_background_finished").Len() == 0 && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if logs.FilterMessage("collector_background_finished").Len() != 1 {
		t.Fatal("missing background completion")
	}
}

func TestFastDropsAreReported(t *testing.T) {
	logs := setupCollectionLogTest(t)
	round := newCollectionRound()
	collector := &loggingTestCollector{collect: func(c *loggingTestCollector, ch chan<- prometheus.Metric) {
		ch <- prometheus.MustNewConstMetric(logTestDesc, prometheus.GaugeValue, 1)
	}}
	// No receiver: existing fast mode drops instead of blocking.
	collectWithLogging(make(chan prometheus.Metric), testLogPool(t, "full", nil), collector, round, true, time.Second)
	round.finish(nil)
	f := logs.FilterMessage("scrape_finished").All()[0].ContextMap()
	if f["issue_runs"] != int64(1) || f["dropped_samples"] != int64(1) {
		t.Fatal(f)
	}

}

func TestBlockingSlowCollectorStillWaitsForMetrics(t *testing.T) {
	logs := setupCollectionLogTest(t)
	cfg := config.DefaultMultiSourceConfig
	cfg.ScrapeSlowThresholdMs = 15
	config.Global.Init(&cfg)
	pool := testLogPool(t, "slow", nil)
	round := newCollectionRound()
	release, done := make(chan struct{}), make(chan struct{})
	var releaseOnce sync.Once
	t.Cleanup(func() { releaseOnce.Do(func() { close(release) }) })
	c := &loggingTestCollector{collect: func(c *loggingTestCollector, ch chan<- prometheus.Metric) {
		<-release
		ch <- prometheus.MustNewConstMetric(logTestDesc, prometheus.GaugeValue, 1)
	}}
	out := make(chan prometheus.Metric, 1)
	go func() {
		collectWithLogging(out, pool, c, round, false, 10*time.Millisecond)
		close(done)
	}()
	deadline := time.Now().Add(time.Second)
	for logs.FilterMessage("scrape_diagnostic").Len() == 0 && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if logs.FilterMessage("scrape_diagnostic").Len() != 1 {
		t.Fatal("missing in-progress slow collector warning")
	}
	select {
	case <-done:
		t.Fatal("blocking collection returned before SQL finished")
	default:
	}
	releaseOnce.Do(func() { close(release) })
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("blocking collection did not finish")
	}
	round.finish(nil)
	if len(out) != 1 || logs.FilterMessage("scrape_finished").All()[0].ContextMap()["frontend_timeout_runs"] != nil {
		t.Fatal("blocking behavior changed")
	}

}

func TestUnavailableDatasourceAppearsWithoutRunningCollectors(t *testing.T) {
	logs := setupCollectionLogTest(t)
	cfg := config.DefaultMultiSourceConfig
	cfg.DataSources = []config.DataSourceConfig{{Name: "offline", Enabled: true, RegisterDatabaseMetrics: true}, {Name: "disabled", Enabled: false}}
	config.Global.Init(&cfg)
	config.GlobalMultiConfig = &cfg
	manager := db.NewDBPoolManager(&cfg)
	reg := prometheus.NewRegistry()
	RegisterMultiSourceCollectors(reg, manager)
	families, err := reg.Gather()
	if err != nil {
		t.Fatal(err)
	}
	foundHealth := false
	for _, family := range families {
		if family.GetName() == dmdb_up {
			foundHealth = true
			if len(family.Metric) != 1 || family.Metric[0].GetGauge().GetValue() != 0 {
				t.Fatalf("unavailable health metric changed: %v", family)
			}
		}
	}
	if !foundHealth {
		t.Fatal("missing health metric after aggregate registration")
	}
	entries := logs.FilterMessage("scrape_finished").All()
	if entries[0].ContextMap()["unavailable_datasource_count"] != int64(1) {
		t.Fatal(entries)
	}

}

func TestIndependentRegistriesRegisterAndGatherConcurrently(t *testing.T) {
	setupCollectionLogTest(t)
	cfg := config.DefaultMultiSourceConfig
	cfg.DataSources = []config.DataSourceConfig{{Name: "offline", Enabled: true, RegisterDatabaseMetrics: true}}
	config.Global.Init(&cfg)
	config.GlobalMultiConfig = &cfg
	manager := db.NewDBPoolManager(&cfg)
	t.Cleanup(manager.Close)
	registries := []*prometheus.Registry{prometheus.NewRegistry(), prometheus.NewRegistry()}
	type response struct {
		families []*dto.MetricFamily
		err      error
	}
	responses := make(chan response, len(registries))
	for _, reg := range registries {
		go func(reg *prometheus.Registry) {
			RegisterMultiSourceCollectors(reg, manager)
			families, err := reg.Gather()
			responses <- response{families, err}
		}(reg)
	}
	for range registries {
		r := <-responses
		if r.err != nil {
			t.Fatal(r.err)
		}
		want := map[string]bool{dameng_exporter_build_info: true, dmdb_up: true, dmdbms_node_type_info: true}
		for _, family := range r.families {
			if !want[family.GetName()] || len(family.Metric) != 1 {
				t.Fatalf("registry mixed or duplicated collectors: %v", family)
			}
			delete(want, family.GetName())
		}
		if len(want) != 0 {
			t.Fatalf("registry omitted collectors: %v", want)
		}
	}
	defer func() {
		if _, ok := recover().(prometheus.AlreadyRegisteredError); !ok {
			t.Fatal("duplicate registration no longer reports AlreadyRegisteredError")
		}
	}()
	RegisterMultiSourceCollectors(registries[0], manager)
}
