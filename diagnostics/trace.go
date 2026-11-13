// Package diagnostics records application SQL calls without changing database policy.
package diagnostics

import (
	"fmt"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

type Logger interface {
	Debugw(string, ...interface{})
	Infow(string, ...interface{})
	Warnw(string, ...interface{})
	Errorw(string, ...interface{})
}
type discard struct{}

func (discard) Debugw(string, ...interface{}) {}
func (discard) Infow(string, ...interface{})  {}
func (discard) Warnw(string, ...interface{})  {}
func (discard) Errorw(string, ...interface{}) {}

// Clock allows snapshots and timers to be tested without wall-clock sleeps.
type Clock interface {
	Now() time.Time
	After(time.Duration) <-chan time.Time
}
type realClock struct{}

func (realClock) Now() time.Time                         { return time.Now() }
func (realClock) After(d time.Duration) <-chan time.Time { return time.After(d) }

type Options struct {
	Threshold time.Duration
	Logger    Logger
	Clock     Clock
}
type RoundTrace struct {
	debug                             bool
	mu                                sync.Mutex
	output                            sync.Mutex // serializes groups, never held by database operations
	id                                string
	start                             time.Time
	clock                             Clock
	log                               Logger
	threshold                         time.Duration
	stop, observed                    chan struct{}
	finished, triggered               bool
	sequence, runSequence, snapshotID uint64
	active                            map[*QueryTrace]bool
	runs                              map[*RunTrace]bool
	top                               []querySnapshot
	lastTop                           string
	counts                            counts
}
type counts struct{ Runs, Skipped, Calls, Unfinished, Errors, Samples, Dropped, Issues, Timeouts int64 }
type RunTrace struct {
	backgroundCalls, backgroundErrors  int64
	cutoffAt                           time.Time
	calls, errors                      int64
	round                              *RoundTrace
	id, scope, source, collector, mode string
	start                              time.Time
	budget                             time.Duration
	detached, finished, issue          bool
	stages                             map[string]string
	origin                             string
	fallback                           bool
}

var sequence atomic.Uint64
var prefix = time.Now().UTC().Format("20060102T150405.000000000")

func NewRound(o Options) *RoundTrace {
	if o.Clock == nil {
		o.Clock = realClock{}
	}
	if o.Logger == nil {
		o.Logger = discard{}
	}
	r := &RoundTrace{id: fmt.Sprintf("%s-%d", prefix, sequence.Add(1)), clock: o.Clock, log: o.Logger, threshold: o.Threshold, stop: make(chan struct{}), observed: make(chan struct{}), top: make([]querySnapshot, 0, 5), active: map[*QueryTrace]bool{}, runs: map[*RunTrace]bool{}}
	r.debug = true
	if enabled, ok := o.Logger.(interface{ DebugEnabled() bool }); ok {
		r.debug = enabled.DebugEnabled()
	}
	if _, ok := o.Logger.(discard); ok {
		r.debug = false
	}
	r.start = r.clock.Now()
	r.log.Debugw("scrape_started", "scrape_id", r.id)
	if r.threshold > 0 {
		var timer <-chan time.Time
		stopTimer := func() {}
		if _, ok := r.clock.(realClock); ok {
			t := time.NewTimer(r.threshold)
			timer = t.C
			stopTimer = func() { t.Stop() }
		} else {
			timer = r.clock.After(r.threshold)
		}
		go func() {
			defer close(r.observed)
			defer stopTimer()
			select {
			case <-r.stop:
			case <-timer:
				r.Trigger("duration_threshold")
			}
		}()
	} else {
		close(r.observed)
	}
	return r
}
func (r *RoundTrace) ID() string { return r.id }
func (r *RoundTrace) NewRun(scope, source, name, mode string, budget time.Duration) *RunTrace {
	r.mu.Lock()
	r.runSequence++
	u := &RunTrace{round: r, id: fmt.Sprintf("R%d", r.runSequence), scope: scope, source: source, collector: name, mode: mode, budget: budget, start: r.clock.Now(), origin: "none", stages: map[string]string{"worker": "preflight"}}
	r.runs[u] = true
	r.counts.Runs++
	f := u.fieldsLocked()
	if mode == "fast" {
		f = append(f, "mode", mode, "timeout_ms", budget.Milliseconds(), "timeout_scope", "worker")
	}
	r.mu.Unlock()
	r.log.Debugw("collector_started", f...)
	return u
}
func (u *RunTrace) fieldsLocked() []interface{} {
	f := []interface{}{"scrape_id", u.round.id, "run_id", u.id, "collector", u.collector}
	if u.source != "" {
		f = append(f, "datasource", u.source)
	}
	if u.scope == "exporter" {
		f = append(f, "scope", u.scope)
	}
	if u.detached {
		f = append(f, "phase", "background")
	}
	return f
}
func (u *RunTrace) Fields() []interface{} {
	r := u.round
	r.mu.Lock()
	defer r.mu.Unlock()
	return u.fieldsLocked()
}

// Stage is scoped to a flow; concurrent forwarding never overwrites worker state.
func (u *RunTrace) Stage(flow, stage string) func() {
	if u == nil {
		return func() {}
	}
	r := u.round
	r.mu.Lock()
	old, had := u.stages[flow]
	u.stages[flow] = stage
	f := u.fieldsLocked()
	r.mu.Unlock()
	r.log.Debugw("collector_stage_started", append(f, "flow", flow, "stage", stage)...)
	return func() {
		r.mu.Lock()
		if had {
			u.stages[flow] = old
		} else {
			delete(u.stages, flow)
		}
		f := u.fieldsLocked()
		r.mu.Unlock()
		r.log.Debugw("collector_stage_finished", append(f, "flow", flow, "stage", stage)...)
	}
}
func (u *RunTrace) issueLocked() {
	if !u.issue {
		u.issue = true
		if !u.detached && !u.round.finished {
			u.round.counts.Issues++
		}
	}
}
func (u *RunTrace) Error(reason string, err error) {
	if u == nil {
		return
	}
	r := u.round
	r.mu.Lock()
	u.issueLocked()
	f := u.fieldsLocked()
	r.mu.Unlock()
	r.log.Errorw("collector_error", append(f, "reason", reason, "error", err)...)
}
func (u *RunTrace) Cache(origin string, fallback bool) {
	if u == nil {
		return
	}
	r := u.round
	r.mu.Lock()
	u.origin = origin
	u.fallback = u.fallback || fallback
	r.mu.Unlock()
}
func (u *RunTrace) Panic(value interface{}, stack string) {
	r := u.round
	r.mu.Lock()
	u.issueLocked()
	f := u.fieldsLocked()
	bg := u.detached
	r.mu.Unlock()
	r.log.Errorw("collector_panic", append(f, "panic", fmt.Sprint(value), "stack", stack)...)
	if !bg {
		r.Trigger("panic")
	}
}
func (u *RunTrace) Detach() {
	r := u.round
	r.mu.Lock()
	if u.detached || u.finished {
		r.mu.Unlock()
		return
	}
	u.issueLocked()
	now := r.clock.Now()
	r.counts.Timeouts++
	u.cutoffAt = now
	for q := range r.active {
		if q.run == u {
			s := q.snapshotLocked(now)
			s.State = "detached"
			r.retainLocked(s)
			delete(r.active, q)
			r.counts.Unfinished++
		}
	}
	f := u.fieldsLocked()
	u.detached = true
	delete(r.runs, u)
	f = append(f, "mode", u.mode, "timeout_ms", u.budget.Milliseconds(), "timeout_scope", "worker", "foreground_cutoff_at", now.Format(time.RFC3339Nano))
	r.mu.Unlock()
	r.log.Warnw("collector_timeout", f...)
	r.Trigger("frontend_timeout")
}
func (u *RunTrace) Finish(samples, dropped int64, skipped bool) {
	u.finish(samples, dropped, skipped, "")
}
func (u *RunTrace) Skip(reason string) {
	u.finish(0, 0, true, reason)
}
func (u *RunTrace) finish(samples, dropped int64, skipped bool, skipReason string) {
	r := u.round
	r.mu.Lock()
	if u.finished {
		r.mu.Unlock()
		return
	}
	u.finished = true
	delete(r.runs, u)
	r.counts.Samples += samples
	r.counts.Dropped += dropped
	if dropped > 0 { // samples freeze only after the forwarder exits
		if !u.issue {
			u.issue = true
			r.counts.Issues++
		}
	}
	if skipped {
		r.counts.Skipped++
	}
	// This is the foreground completion even when its worker was detached.
	f := []interface{}{"scrape_id", r.id, "run_id", u.id, "collector", u.collector}
	if u.source != "" {
		f = append(f, "datasource", u.source)
	}
	if u.scope == "exporter" {
		f = append(f, "scope", u.scope)
	}
	f = append(f, "sql_calls", u.calls, "duration_ms", r.clock.Now().Sub(u.start).Milliseconds(), "samples", samples)
	if u.errors > 0 {
		f = append(f, "sql_error_calls", u.errors)
	}
	if dropped > 0 {
		f = append(f, "dropped_samples", dropped)
	}
	if u.issue {
		f = append(f, "issue", true)
	}
	if u.origin != "none" {
		f = append(f, "data_origin", u.origin)
	}
	if u.fallback {
		f = append(f, "cache_fallback", true)
	}
	if u.detached {
		f = append(f, "run_state", "detached", "foreground_cutoff_at", u.cutoffAt.Format(time.RFC3339Nano), "forwarding_finished_at", r.clock.Now().Format(time.RFC3339Nano))
	}
	if skipped {
		f = append(f, "run_state", "skipped", "skip_reason", skipReason)
	}
	r.mu.Unlock()
	r.log.Debugw("collector_finished", f...)
}
func (u *RunTrace) BackgroundFinished() {
	r := u.round
	r.mu.Lock()
	f := u.fieldsLocked()
	f = append(f, "background_duration_ms", r.clock.Now().Sub(u.start).Milliseconds(), "background_sql_calls", u.backgroundCalls)
	if u.backgroundErrors > 0 {
		f = append(f, "background_sql_error_calls", u.backgroundErrors)
	}
	r.mu.Unlock()
	r.log.Infow("collector_background_finished", f...)
}

// Insertion into five terminal candidates is bounded; sorting snapshots and I/O
// happen after releasing the state lock.
func (r *RoundTrace) retainLocked(s querySnapshot) {
	index := 0
	for index < len(r.top) && !queryBefore(s, r.top[index]) {
		index++
	}
	if index >= 5 {
		return
	}
	if len(r.top) < 5 {
		r.top = append(r.top, querySnapshot{})
	}
	copy(r.top[index+1:], r.top[index:len(r.top)-1])
	r.top[index] = s
}
func queryBefore(a, b querySnapshot) bool {
	if a.Duration != b.Duration {
		return a.Duration > b.Duration
	}
	return a.Sequence < b.Sequence
}

func sortTop(s []querySnapshot) {
	sort.Slice(s, func(i, j int) bool {
		if s[i].Duration != s[j].Duration {
			return s[i].Duration > s[j].Duration
		}
		return s[i].Sequence < s[j].Sequence
	})
}

type snapshot struct {
	top                   []querySnapshot
	counts                counts
	activeRuns, activeSQL int
	stages                string
	at                    time.Time
}

func (r *RoundTrace) snapshotLocked() snapshot {
	now := r.clock.Now()
	s := snapshot{at: now, top: append([]querySnapshot(nil), r.top...)}
	for q := range r.active {
		s.top = append(s.top, q.snapshotLocked(now))
	}
	s.counts = r.counts
	s.activeRuns, s.activeSQL = len(r.runs), len(r.active)
	stages := map[string]int{}
	for u := range r.runs {
		for _, v := range u.stages {
			stages[v]++
		}
	}
	parts := make([]string, 0, len(stages))
	for k, v := range stages {
		parts = append(parts, fmt.Sprintf("%s:%d", k, v))
	}
	sort.Strings(parts)
	s.stages = strings.Join(parts, ",")
	return s
}
func (s snapshot) summaryFields(id string, start time.Time) []interface{} {
	c := s.counts
	f := []interface{}{"scrape_id", id, "collection_duration_ms", s.at.Sub(start).Milliseconds(), "runs", c.Runs, "sql_calls", c.Calls, "samples", c.Samples}
	for _, item := range []struct {
		key   string
		value int64
	}{
		{"skipped_runs", c.Skipped}, {"sql_unfinished_at_cutoff", c.Unfinished}, {"sql_error_calls", c.Errors}, {"dropped_samples", c.Dropped}, {"issue_runs", c.Issues}, {"frontend_timeout_runs", c.Timeouts},
	} {
		if item.value > 0 {
			f = append(f, item.key, item.value)
		}
	}
	return f
}
func prepare(s *snapshot) {
	sortTop(s.top)
	if len(s.top) > 5 {
		s.top = s.top[:5]
	}
}
func topKey(s []querySnapshot) string {
	var b strings.Builder
	for _, q := range s {
		fmt.Fprintf(&b, "%d/%s/%s/%s;", q.Sequence, q.State, q.Result, q.ErrorStage)
	}
	return b.String()
}
func (r *RoundTrace) Trigger(reason string) {
	r.output.Lock()
	defer r.output.Unlock()
	r.mu.Lock()
	if r.finished {
		r.mu.Unlock()
		return
	}
	// Errors remain diagnostic facts, but only round duration permits a snapshot.
	if r.triggered || r.threshold <= 0 || r.clock.Now().Sub(r.start) < r.threshold {
		r.mu.Unlock()
		return
	}
	r.triggered = true
	r.snapshotID++
	id := r.snapshotID
	s := r.snapshotLocked()
	r.mu.Unlock()
	prepare(&s)
	r.lastTop = topKey(s.top)
	f := s.summaryFields(r.id, r.start)
	f = append(f, "snapshot_id", id, "snapshot_at", s.at.Format(time.RFC3339Nano), "trigger", reason, "threshold_ms", r.threshold.Milliseconds(), "runs_active", s.activeRuns, "sql_active", s.activeSQL)
	if s.stages != "" {
		f = append(f, "active_stages", s.stages)
	}
	if len(s.top) > 0 {
		f = append(f, "top_emitted_count", len(s.top))
	}
	r.log.Infow("scrape_diagnostic", f...)
	r.emitTop(s.top, id)
}
func (r *RoundTrace) emitTop(top []querySnapshot, id uint64) {
	for i, q := range top {
		ordered := append(q.topFields(), "snapshot_id", id, "rank", i+1)
		r.log.Infow("sql_top", ordered...)
	}
}
func (r *RoundTrace) Finish(unavailable []string) {
	r.output.Lock()
	r.mu.Lock()
	if r.finished {
		r.mu.Unlock()
		r.output.Unlock()
		return
	}
	s := r.snapshotLocked()
	r.finished = true
	close(r.stop)
	triggered := r.triggered
	r.mu.Unlock()
	prepare(&s)
	slow := r.threshold > 0 && s.at.Sub(r.start) >= r.threshold
	refresh := slow && (!triggered || r.lastTop != topKey(s.top))
	if refresh {
		r.snapshotID++
	}
	unique := map[string]bool{}
	for _, v := range unavailable {
		unique[v] = true
	}
	f := s.summaryFields(r.id, r.start)
	if len(unique) > 0 {
		f = append(f, "unavailable_datasource_count", len(unique))
	}
	if r.snapshotID > 0 {
		f = append(f, "snapshot_id", r.snapshotID)
	}
	if refresh && len(s.top) > 0 {
		f = append(f, "top_emitted_count", len(s.top))
	}
	r.log.Infow("scrape_finished", f...)
	if refresh {
		r.emitTop(s.top, r.snapshotID)
	}
	r.mu.Lock()
	r.top = nil
	r.active = nil
	r.runs = nil
	r.lastTop = ""
	r.mu.Unlock()
	r.output.Unlock()
	<-r.observed
}
