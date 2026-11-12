package collector

import (
	"dameng_exporter/config"
	"dameng_exporter/db"
	"dameng_exporter/logger"
	"fmt"
	"runtime/debug"
	"sort"
	"sync"
	"sync/atomic"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

// Inject only into a fresh business collector, never a shared public instance.
type collectorLog struct{ collectionLogger *zap.SugaredLogger }

func (c *collectorLog) SetCollectionLogger(log *zap.SugaredLogger) { c.collectionLogger = log }
func (c *collectorLog) log() *zap.SugaredLogger {
	if c.collectionLogger != nil {
		return c.collectionLogger
	}
	return logger.Logger
}

type collectionResult struct {
	Scope          string       `json:"scope"`
	Datasource     string       `json:"datasource"`
	Collector      string       `json:"collector"`
	Mode           string       `json:"mode"`
	TimeoutMS      int64        `json:"timeout_ms"`
	TimeoutScope   string       `json:"timeout_scope"`
	DurationMS     int64        `json:"duration_ms"`
	Samples        int64        `json:"samples"`
	DroppedSamples int64        `json:"dropped_samples"`
	Execution      string       `json:"execution"`
	Outcome        OutcomeState `json:"outcome"`
	Reason         string       `json:"reason"`
	DataOrigin     string       `json:"data_origin"`
	CacheFallback  bool         `json:"cache_fallback"`
	Slow           bool         `json:"slow"`
	Warnings       int64        `json:"observed_warning_logs"`
	Errors         int64        `json:"observed_error_logs"`
}

func (r collectionResult) knownFailed() bool {
	return r.Outcome == OutcomeFailed || r.Outcome == OutcomePartial || r.Execution == "timeout" || r.Execution == "panic" || r.DroppedSamples > 0
}
func (r collectionResult) abnormal() bool {
	return r.knownFailed() || r.Execution == "skipped" || r.Warnings > 0 || r.Errors > 0 || r.Slow
}
func (r collectionResult) fields() []interface{} {
	return []interface{}{"execution", r.Execution, "outcome", r.Outcome, "reason", r.Reason,
		"data_origin", r.DataOrigin, "cache_fallback", r.CacheFallback, "duration_ms", r.DurationMS,
		"samples", r.Samples, "dropped_samples", r.DroppedSamples, "slow", r.Slow,
		"observed_warning_logs", r.Warnings, "observed_error_logs", r.Errors,
		"background_may_continue", r.Execution == "timeout"}
}

type collectionRun struct {
	log, diagnostic  *zap.SugaredLogger
	start            time.Time
	threshold        time.Duration
	stopSlow         chan struct{}
	slowDone         chan struct{}
	warnings, errors atomic.Int64
	result           collectionResult // immutable run metadata; worker never writes here
}

func newCollectionRun(round *collectionRound, scope, source, name, mode string, timeout time.Duration, sourceLabel string) *collectionRun {
	r := &collectionRun{start: time.Now(), threshold: time.Duration(config.Global.GetCollectorSlowThresholdMs()) * time.Millisecond,
		result: collectionResult{Scope: scope, Datasource: source, Collector: name, Mode: mode, TimeoutScope: "none"}}
	if mode == "fast" && timeout > 0 {
		r.result.TimeoutMS, r.result.TimeoutScope = timeout.Milliseconds(), "worker"
	}
	base := logger.Logger
	if round != nil {
		base = round.log
	}
	r.diagnostic = base.With("scope", scope, "datasource", source, "collector", name,
		"mode", mode, "timeout_ms", r.result.TimeoutMS, "timeout_scope", r.result.TimeoutScope)
	if sourceLabel != "" && sourceLabel != source {
		r.diagnostic = r.diagnostic.With("datasource_label", sourceLabel)
	}
	// Hooks observe emitted business logs only; diagnostic warnings must not feed back.
	r.log = r.diagnostic.Desugar().WithOptions(zap.Hooks(func(e zapcore.Entry) error {
		if e.Level >= zapcore.ErrorLevel {
			r.errors.Add(1)
		} else if e.Level == zapcore.WarnLevel {
			r.warnings.Add(1)
		}
		return nil
	})).Sugar()
	r.diagnostic.Debug("collector_started")
	if r.threshold > 0 {
		r.stopSlow, r.slowDone = make(chan struct{}), make(chan struct{})
		go func() {
			defer close(r.slowDone)
			timer := time.NewTimer(max(0, r.threshold-time.Since(r.start)))
			defer timer.Stop()
			select {
			case <-r.stopSlow:
				return
			case <-timer.C:
				select {
				case <-r.stopSlow:
					return
				default:
				}
				r.diagnostic.Warnw("collector_slow", "duration_ms", time.Since(r.start).Milliseconds(), "threshold_ms", r.threshold.Milliseconds(), "still_collecting", true)
			}
		}()
	}
	return r
}

// Only the foreground executor calls finish, once, after forwarding has stopped.
func (r *collectionRun) finish(round *collectionRound, samples, dropped int64, worker workerResult) {
	if r.stopSlow != nil {
		close(r.stopSlow)
		<-r.slowDone
	}
	duration := time.Since(r.start)
	result := r.result
	result.DurationMS, result.Samples, result.DroppedSamples = duration.Milliseconds(), samples, dropped
	result.Execution, result.Outcome = worker.execution, worker.outcome.State
	result.Reason, result.DataOrigin, result.CacheFallback = worker.outcome.Reason, worker.outcome.DataOrigin, worker.outcome.CacheFallback
	result.Slow = r.threshold > 0 && duration >= r.threshold
	result.Warnings, result.Errors = r.warnings.Load(), r.errors.Load()
	if result.abnormal() {
		r.diagnostic.Warnw("collector_finished", result.fields()...)
	} else {
		r.diagnostic.Debugw("collector_finished", result.fields()...)
	}
	if round != nil {
		round.mu.Lock()
		round.results = append(round.results, result)
		round.mu.Unlock()
	}
}

type workerResult struct {
	execution  string
	outcome    CollectorOutcome
	finishedAt time.Time
}

// The worker owns its output and reports exactly once through a buffered channel.
func executeCollector(child prometheus.Collector, out chan prometheus.Metric, done chan<- workerResult, run *collectionRun) {
	result := workerResult{execution: "completed", outcome: unknownOutcome()}
	defer func() {
		if p := recover(); p != nil {
			result = workerResult{execution: "panic", outcome: CollectorOutcome{State: OutcomeFailed, Reason: "panic", DataOrigin: "unknown"}}
			run.log.Errorw("collector_panic", "panic", fmt.Sprint(p), "stack", string(debug.Stack()))
		}
		close(out)
		result.finishedAt = time.Now()
		done <- result
	}()
	if aware, ok := child.(ResultCollector); ok {
		result.outcome = aware.CollectWithResult(out)
	} else {
		child.Collect(out)
	}
}

type collectionRound struct {
	log     *zap.SugaredLogger
	start   time.Time
	mu      sync.Mutex
	results []collectionResult
}

var roundSequence atomic.Uint64
var roundPrefix = time.Now().UTC().Format("20060102T150405.000000000")

func newCollectionRound() *collectionRound {
	id := fmt.Sprintf("%s-%d", roundPrefix, roundSequence.Add(1))
	r := &collectionRound{log: logger.Logger.With("scrape_id", id), start: time.Now()}
	r.log.Infow("scrape_started", "phase", "collection")
	return r
}

type zeroSampleCollector struct {
	Scope      string       `json:"scope"`
	Datasource string       `json:"datasource"`
	Collector  string       `json:"collector"`
	Execution  string       `json:"execution"`
	Outcome    OutcomeState `json:"outcome"`
	Reason     string       `json:"reason"`
}

const zeroSampleCollectorLimit = 20

type slowestCollector struct {
	Scope        string       `json:"scope"`
	Datasource   string       `json:"datasource"`
	Collector    string       `json:"collector"`
	Mode         string       `json:"mode"`
	TimeoutMS    int64        `json:"timeout_ms"`
	TimeoutScope string       `json:"timeout_scope"`
	DurationMS   int64        `json:"duration_ms"`
	Samples      int64        `json:"samples"`
	Execution    string       `json:"execution"`
	Outcome      OutcomeState `json:"outcome"`
}

func (r *collectionRound) finish(unavailable []string) {
	r.mu.Lock()
	results := append([]collectionResult(nil), r.results...)
	r.mu.Unlock()
	sort.Slice(results, func(i, j int) bool {
		a, b := results[i], results[j]
		if a.DurationMS != b.DurationMS {
			return a.DurationMS > b.DurationMS
		}
		if a.Datasource != b.Datasource {
			return a.Datasource < b.Datasource
		}
		return a.Collector < b.Collector
	})
	zero := make([]zeroSampleCollector, 0)
	slowest := make([]slowestCollector, 0, 3)
	var samples int64
	var failed, unknown, errorRuns, skipped, slow int
	issues := len(unavailable) > 0
	for _, v := range results {
		samples += v.Samples
		if v.knownFailed() {
			failed++
		}
		if v.Outcome == OutcomeUnknown {
			unknown++
		}
		if v.Errors > 0 {
			errorRuns++
		}
		if v.Execution == "skipped" {
			skipped++
		}
		if v.Slow {
			slow++
		}
		issues = issues || v.abnormal()
		if v.Samples == 0 {
			zero = append(zero, zeroSampleCollector{v.Scope, v.Datasource, v.Collector, v.Execution, v.Outcome, v.Reason})
		}
		if len(slowest) < 3 {
			slowest = append(slowest, slowestCollector{v.Scope, v.Datasource, v.Collector, v.Mode, v.TimeoutMS, v.TimeoutScope, v.DurationMS, v.Samples, v.Execution, v.Outcome})
		}
	}
	sort.Slice(zero, func(i, j int) bool {
		a, b := zero[i], zero[j]
		if a.Scope != b.Scope {
			return a.Scope < b.Scope
		}
		if a.Datasource != b.Datasource {
			return a.Datasource < b.Datasource
		}
		return a.Collector < b.Collector
	})
	unique := make(map[string]bool)
	for _, name := range unavailable {
		unique[name] = true
	}
	unavailable = make([]string, 0, len(unique))
	for name := range unique {
		unavailable = append(unavailable, name)
	}
	sort.Strings(unavailable)
	zeroCount := len(zero)
	if len(zero) > zeroSampleCollectorLimit {
		zero = zero[:zeroSampleCollectorLimit]
	}
	fields := []interface{}{"phase", "collection", "collection_duration_ms", time.Since(r.start).Milliseconds(),
		"recorded_runs", len(results), "samples", samples, "known_failed_runs", failed, "unknown_runs", unknown,
		"observed_error_runs", errorRuns, "skipped_runs", skipped, "slow_runs", slow, "zero_sample_runs", zeroCount,
		"zero_sample_collectors", zero, "zero_sample_collectors_omitted", zeroCount - len(zero),
		"unavailable_datasources", unavailable, "slowest", slowest}
	if issues {
		r.log.Warnw("scrape_finished", fields...)
	} else {
		r.log.Infow("scrape_finished", fields...)
	}
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
			for metric := range out {
				ch <- metric
				samples++
			}
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
