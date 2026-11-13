package collector

import (
	"context"
	"dameng_exporter/config"
	"dameng_exporter/db"
	"dameng_exporter/diagnostics"
	"dameng_exporter/logger"
	"database/sql"
	"errors"
	dm "github.com/gaoyuan98/dm"
	"github.com/prometheus/client_golang/prometheus"
	"go.uber.org/zap"
	"runtime/debug"
	"strings"
	"sync"
	"time"
)

// Only fresh business collectors receive a trace. Shared public collectors use
// a local execution wrapper and are never mutated by concurrent scrapes.
type collectorLog struct {
	collectionLogger *zap.SugaredLogger
	trace            *diagnostics.RunTrace
}

func (c *collectorLog) SetCollectionLogger(l *zap.SugaredLogger) { c.collectionLogger = l }
func (c *collectorLog) SetRunTrace(r *diagnostics.RunTrace)      { c.trace = r }
func (c *collectorLog) log() *zap.SugaredLogger {
	if c.collectionLogger != nil {
		if c.trace != nil {
			return c.collectionLogger.With(c.trace.Fields()...)
		}
		return c.collectionLogger
	}
	if logger.Logger != nil {
		return logger.Logger
	}
	return zap.NewNop().Sugar()
}
func (c *collectorLog) database(d *sql.DB) diagnostics.DB {
	run := c.trace
	if run == nil {
		r := diagnostics.NewRound(diagnostics.Options{Logger: c.log()})
		run = r.NewRun("database", "", "standalone", "direct", 0)
	}
	return diagnostics.Wrap(d, run)
}
func (c *collectorLog) cache(origin string, fallback bool) { c.trace.Cache(origin, fallback) }
func (c *collectorLog) businessError(reason string, err error) {
	if c.trace != nil {
		c.trace.Error(reason, err)
		return
	}
	c.log().Errorw("collector_error", "reason", reason, "error", err)
}

type collectionRound struct{ trace *diagnostics.RoundTrace }

func newCollectionRound() *collectionRound {
	return &collectionRound{diagnostics.NewRound(diagnostics.Options{Threshold: time.Duration(config.Global.GetScrapeSlowThresholdMs()) * time.Millisecond, Logger: logger.Diagnostics()})}
}
func (r *collectionRound) finish(unavailable []string) { r.trace.Finish(unavailable) }

type collectionRun struct {
	trace *diagnostics.RunTrace
	log   *zap.SugaredLogger
	base  *zap.SugaredLogger
}

func newCollectionRun(round *collectionRound, scope, source, name, mode string, timeout time.Duration, sourceLabel string) *collectionRun {
	trace := round.trace.NewRun(scope, source, name, mode, timeout)
	base := logger.Logger
	if base == nil {
		base = zap.NewNop().Sugar()
	}
	f := trace.Fields()
	if sourceLabel != "" && sourceLabel != source {
		base = base.With("datasource_label", sourceLabel)
	}
	return &collectionRun{trace: trace, log: base.With(f...), base: base}
}
func (r *collectionRun) finish(_ *collectionRound, samples, dropped int64, w workerResult) {
	r.trace.Finish(samples, dropped, w.execution == "skipped")
}

type workerResult struct {
	execution string
}

func executeCollector(child prometheus.Collector, out chan prometheus.Metric, done chan<- workerResult, run *collectionRun) {
	result := workerResult{execution: "completed"}
	restore := run.trace.Stage("worker", "process")
	defer func() {
		if p := recover(); p != nil {
			result.execution = "panic"
			run.trace.Panic(p, string(debug.Stack()))
		}
		restore()
		close(out)
		done <- result
	}()
	child.Collect(out)
}

type scrapeCollector struct {
	children    []prometheus.Collector
	poolManager *db.DBPoolManager
}

func (c *scrapeCollector) Describe(ch chan<- *prometheus.Desc) {
	for _, child := range c.children {
		child.Describe(ch)
	}
}
func (c *scrapeCollector) Collect(ch chan<- prometheus.Metric) {
	round := newCollectionRound()
	var wg sync.WaitGroup
	for _, child := range c.children {
		wg.Add(1)
		go func(child prometheus.Collector) {
			defer wg.Done()
			if aware, ok := child.(interface {
				collectWithRound(chan<- prometheus.Metric, *collectionRound)
			}); ok {
				aware.collectWithRound(ch, round)
				return
			}
			run := newCollectionRun(round, "exporter", "", getCollectorName(child), "direct", 0, "")
			out, done := make(chan prometheus.Metric), make(chan workerResult, 1)
			go executeCollector(child, out, done, run)
			var samples int64
			restore := run.trace.Stage("forwarder", "metric_forward")
			for m := range out {
				ch <- m
				samples++
			}
			restore()
			run.finish(round, samples, 0, <-done)
		}(child)
	}
	wg.Wait()
	unavailable := []string{}
	if cfg := config.Global.GetConfig(); c.poolManager != nil && cfg != nil {
		for _, ds := range cfg.DataSources {
			if ds.Enabled && !c.poolManager.GetDatasourceHealthStatus(ds.Name).Healthy {
				unavailable = append(unavailable, ds.Name)
			}
		}
	}
	round.finish(unavailable)
}

// Only a driver-confirmed missing field/view can be an expected compatibility
// probe failure. Permission, network, deadline and unknown errors stay ERROR.
func probeContext(ctx context.Context) context.Context {
	return diagnostics.WithQueryOptions(ctx, diagnostics.QueryOptions{Purpose: "probe", ExpectedError: func(err error) bool {
		var de *dm.DmError
		if !errors.As(err, &de) {
			return false
		}
		text := strings.ToLower(de.Error())
		return strings.Contains(text, "无效的列名") || strings.Contains(text, "无效的表或视图名") || strings.Contains(text, "invalid column name") || strings.Contains(text, "invalid table or view name")
	}})
}
