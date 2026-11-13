package diagnostics

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"sync"
	"time"
)

type QueryOptions struct {
	Purpose, Name string
	Index         int
	ReadScope     string // first_row or first_result_set; never consumes extra rows
	AllowEmpty    bool
	ExpectedError func(error) bool
}
type queryOptionsKey struct{}

func WithQueryOptions(ctx context.Context, o QueryOptions) context.Context {
	return context.WithValue(ctx, queryOptionsKey{}, o)
}

type QueryTrace struct {
	run           *RunTrace
	ctx           context.Context
	options       QueryOptions
	data          querySnapshot
	apiStart      time.Time
	inAPI         bool
	seenErrors    map[string]int
	unexpected    bool
	previousStage string
}
type querySnapshot struct {
	Scrape, Run, Source, Collector, Phase, ID, SQL, Purpose, Name, State, Result, ReadState, ReadScope, SQLPhase, ErrorStage, ContextState string
	Index                                                                                                                                  int
	Sequence                                                                                                                               uint64
	Start, End                                                                                                                             time.Time
	Duration, Wall, Offset                                                                                                                 time.Duration
	Budget                                                                                                                                 *int64
	Rows                                                                                                                                   *int64
	API                                                                                                                                    [8]time.Duration
	RowAPI                                                                                                                                 bool
	Observed                                                                                                                               uint8
}

var apiNames = [8]string{"query", "next", "scan", "err", "close", "columns", "column_types", "row_scan"}

func apiIndex(s string) int {
	for i, n := range apiNames {
		if n == s {
			return i
		}
	}
	panic("invalid SQL phase: " + s)
}
func (q *QueryTrace) snapshotLocked(now time.Time) querySnapshot {
	s := q.data
	if q.inAPI {
		d := now.Sub(q.apiStart)
		s.Duration += d
		s.API[apiIndex(s.SQLPhase)] += d
	}
	end := now
	if !s.End.IsZero() {
		end = s.End
	}
	s.Wall = end.Sub(s.Start)
	if q.run.detached {
		s.Phase = "background"
	}
	return s
}

// SQL event builders deliberately keep lifecycle snapshots separate from logs.
func (s querySnapshot) contextFields() []interface{} {
	f := []interface{}{"scrape_id", s.Scrape, "run_id", s.Run, "datasource", s.Source, "collector", s.Collector, "query_id", s.ID}
	if s.Phase == "background" {
		f = append(f, "phase", s.Phase)
	}
	if s.Purpose != "" {
		f = append(f, "purpose", s.Purpose)
	}
	if s.Name != "" {
		f = append(f, "query_name", s.Name)
	}
	if s.Index > 0 {
		f = append(f, "query_index", s.Index)
	}
	if s.ContextState != "active" {
		f = append(f, "context_state_at_start", s.ContextState)
	}
	return f
}
func (s querySnapshot) budgetFields(f []interface{}) []interface{} {
	var budget interface{}
	if s.Budget != nil {
		budget = *s.Budget
	}
	return append(f, "budget_remaining_ms_at_start", budget)
}
func (s querySnapshot) readFields(f []interface{}) []interface{} {
	var rows interface{}
	if s.Rows != nil {
		rows = *s.Rows
	}
	f = append(f, "read_scope", s.ReadScope, "rows_read", rows)
	if s.ReadState != "complete" {
		f = append(f, "read_state", s.ReadState)
	}
	return f
}
func (s querySnapshot) apiFields(f []interface{}) []interface{} {
	api := make(map[string]int64)
	for i, name := range apiNames {
		if s.Observed&(1<<i) != 0 {
			api[name] = s.API[i].Milliseconds()
		}
	}
	if len(api) > 0 {
		f = append(f, "api_ms", api)
	}
	return f
}
func (s querySnapshot) startedFields() []interface{} {
	return append(s.budgetFields(s.contextFields()), "sql", s.SQL)
}
func (s querySnapshot) finishedFields() []interface{} {
	f := append(s.contextFields(), "result", s.Result, "sql_duration_ms", s.Duration.Milliseconds())
	if s.Wall.Milliseconds() != s.Duration.Milliseconds() {
		f = append(f, "sql_wall_ms", s.Wall.Milliseconds())
	}
	if s.ErrorStage != "" {
		f = append(f, "error_stage", s.ErrorStage)
	}
	return s.apiFields(s.readFields(f))
}
func (s querySnapshot) errorFields(err error, stage string, expired bool) []interface{} {
	if stage == "query" {
		s.ReadState = "not_started"
	}
	f := append(s.budgetFields(s.contextFields()), "error", err, "error_stage", s.ErrorStage, "sql_duration_ms", s.Duration.Milliseconds(), "sql", s.SQL)
	if stage != s.ErrorStage {
		f = append(f, "error_stage_observed", stage)
	}
	if expired {
		f = append(f, "context_expired", true)
	}
	if errors.Is(err, context.DeadlineExceeded) {
		f = append(f, "timeout", true)
	}
	if expired || errors.Is(err, context.DeadlineExceeded) {
		f = append(f, "sql_wall_ms", s.Wall.Milliseconds(), "start_offset_ms", s.Offset.Milliseconds())
		f = s.apiFields(f)
	}
	return s.readFields(f)
}
func (s querySnapshot) topFields() []interface{} {
	f := append(s.budgetFields(s.contextFields()), "state", s.State, "result", s.Result, "sql_phase", s.SQLPhase, "sql_duration_ms", s.Duration.Milliseconds(), "sql_wall_ms", s.Wall.Milliseconds(), "start_offset_ms", s.Offset.Milliseconds(), "sql", s.SQL)
	if s.ErrorStage != "" {
		f = append(f, "error_stage", s.ErrorStage)
	}
	return s.apiFields(s.readFields(f))
}
func (u *RunTrace) begin(ctx context.Context, sqlText string, row bool) *QueryTrace {
	r := u.round
	r.mu.Lock()
	now := r.clock.Now()
	r.sequence++
	if u.origin == "none" {
		u.origin = "database"
	}
	o, _ := ctx.Value(queryOptionsKey{}).(QueryOptions)
	if o.ReadScope == "" {
		o.ReadScope = "first_result_set"
		if row {
			o.ReadScope = "first_row"
		}
	}
	var budget *int64
	if d, ok := ctx.Deadline(); ok {
		ms := max(int64(0), d.Sub(now).Milliseconds())
		budget = &ms
	}
	state := "active"
	if errors.Is(ctx.Err(), context.DeadlineExceeded) {
		state = "deadline_exceeded"
	} else if ctx.Err() != nil {
		state = "canceled"
	}
	n := int64(0)
	q := &QueryTrace{run: u, ctx: ctx, options: o, inAPI: false, apiStart: now, data: querySnapshot{Scrape: r.id, Run: u.id, Source: u.source, Collector: u.collector, Phase: "collection", ID: fmt.Sprintf("Q%d", r.sequence), SQL: sqlText, Purpose: o.Purpose, Name: o.Name, Index: o.Index, Sequence: r.sequence, Start: now, Offset: now.Sub(r.start), Budget: budget, Rows: &n, State: "running", Result: "pending", ReadState: "not_started", ReadScope: o.ReadScope, SQLPhase: "query", ContextState: state, RowAPI: row}}
	if !u.detached && !r.finished {
		r.active[q] = true
		r.counts.Calls++
		u.calls++
	} else {
		u.backgroundCalls++
	}
	var snapshot querySnapshot
	if r.debug {
		snapshot = q.snapshotLocked(now)
	}
	r.mu.Unlock()
	if r.debug {
		r.log.Debugw("sql_started", snapshot.startedFields()...)
	}
	q.enter("query")
	return q
}
func (q *QueryTrace) enter(phase string) {
	r := q.run.round
	r.mu.Lock()
	if q.data.State == "running" {
		q.data.SQLPhase = phase
		q.data.Observed |= 1 << apiIndex(phase)
		q.previousStage = q.run.stages["worker"]
		runStage := "read_rows"
		if phase == "query" {
			runStage = "query"
		}
		q.run.stages["worker"] = runStage
		q.apiStart = r.clock.Now()
		q.inAPI = true
	}
	r.mu.Unlock()
}
func (q *QueryTrace) leave() {
	r := q.run.round
	r.mu.Lock()
	if q.inAPI {
		d := r.clock.Now().Sub(q.apiStart)
		q.data.Duration += d
		q.data.API[apiIndex(q.data.SQLPhase)] += d
		q.inAPI = false
		q.run.stages["worker"] = q.previousStage
		q.data.SQLPhase = "between_calls"
	}
	r.mu.Unlock()
}
func (q *QueryTrace) failure(err error, stage string) {
	q.observeFailure(err, stage, true)
}
func (q *QueryTrace) observeFailure(err error, stage string, countRepeat bool) {
	if err == nil {
		return
	}
	r := q.run.round
	expected := (errors.Is(err, sql.ErrNoRows) && q.options.AllowEmpty) || (q.options.ExpectedError != nil && q.options.ExpectedError(err))
	r.mu.Lock()
	if q.data.State != "running" {
		r.mu.Unlock()
		return
	}
	if !countRepeat && q.seenErrors[stage] > 0 {
		r.mu.Unlock()
		return
	}
	if q.seenErrors == nil {
		q.seenErrors = make(map[string]int)
	}
	q.seenErrors[stage]++
	repeat := q.seenErrors[stage]
	if q.data.ErrorStage == "" {
		q.data.ErrorStage = stage
	}
	q.data.Result = "error"
	q.data.ReadState = "partial"
	if q.data.RowAPI {
		q.data.ReadState = "unconfirmed"
		q.data.Rows = nil
	}
	if errors.Is(err, sql.ErrNoRows) && q.options.AllowEmpty {
		q.data.Result = "empty"
		q.data.ReadState = "complete"
		n := int64(0)
		q.data.Rows = &n
	}
	if !expected && !q.unexpected {
		q.unexpected = true
		q.run.issueLocked()
		if !q.run.detached && !r.finished {
			r.counts.Errors++
			q.run.errors++
		} else {
			q.run.backgroundErrors++
		}
	}
	snapshot := q.snapshotLocked(r.clock.Now())
	expired := errors.Is(q.ctx.Err(), context.DeadlineExceeded)
	r.mu.Unlock()
	if repeat == 1 {
		f := snapshot.errorFields(err, stage, expired)
		if expected {
			r.log.Debugw("sql_probe_expected", f...)
		} else {
			r.log.Errorw("sql_error", f...)
		}
	}
}

// Query and Row.Scan terminate before publishing a timeout snapshot. Rows keep
// their lifecycle open until explicit Close, including cleanup after errors.
func (q *QueryTrace) triggerTimeout(err error) {
	if !errors.Is(err, context.DeadlineExceeded) {
		return
	}
	r := q.run.round
	r.mu.Lock()
	trigger := q.unexpected && !q.run.detached && !r.finished
	r.mu.Unlock()
	if trigger {
		r.Trigger("sql_timeout")
	}
}
func (q *QueryTrace) finish() {
	r := q.run.round
	r.mu.Lock()
	if q.data.State != "running" {
		r.mu.Unlock()
		return
	}
	q.data.State = "finished"
	q.data.End = r.clock.Now()
	q.data.SQLPhase = "done"
	if q.data.Result == "pending" {
		q.data.Result = "ok"
		if q.data.ReadState != "complete" {
			q.data.ReadState = "partial"
			q.run.issueLocked()
		}
	}
	s := q.snapshotLocked(q.data.End)
	var f []interface{}
	if r.debug {
		for stage, n := range q.seenErrors {
			if n > 1 {
				f = append(f, stage+"_error_repeats", n-1)
			}
		}
	}
	if r.active[q] {
		delete(r.active, q)
		r.retainLocked(s)
	}
	r.mu.Unlock()
	if r.debug {
		r.log.Debugw("sql_finished", append(s.finishedFields(), f...)...)
	}
}
func (q *QueryTrace) read(update func(*querySnapshot)) {
	r := q.run.round
	r.mu.Lock()
	update(&q.data)
	r.mu.Unlock()
}

// DB preserves database/sql's calls, parameters, return values and explicit Close order.
type DB struct {
	raw *sql.DB
	run *RunTrace
}

func Wrap(raw *sql.DB, run *RunTrace) DB { return DB{raw, run} }
func (d DB) QueryContext(ctx context.Context, query string, args ...interface{}) (*Rows, error) {
	var q *QueryTrace
	if d.run != nil {
		q = d.run.begin(ctx, query, false)
	}
	rows, err := d.raw.QueryContext(ctx, query, args...)
	if q != nil {
		q.leave()
		q.failure(err, "query")
		if err != nil {
			q.read(func(s *querySnapshot) { s.ReadState = "not_started" })
			q.finish()
			q.triggerTimeout(err)
		}
	}
	if err != nil {
		return nil, err
	}
	return &Rows{raw: rows, trace: q}, nil
}
func (d DB) QueryRowContext(ctx context.Context, query string, args ...interface{}) *Row {
	var q *QueryTrace
	if d.run != nil {
		q = d.run.begin(ctx, query, true)
	}
	row := d.raw.QueryRowContext(ctx, query, args...)
	if q != nil {
		q.leave()
	}
	return &Row{raw: row, trace: q}
}

type Rows struct {
	raw       *sql.Rows
	trace     *QueryTrace
	closeOnce sync.Once
	closeErr  error
	eof       bool
}

func (s *Rows) enter(p string) {
	if s.trace != nil {
		s.trace.enter(p)
	}
}
func (s *Rows) leave(err error, p string) {
	if s.trace != nil {
		s.trace.leave()
		s.trace.failure(err, p)
		s.trace.triggerTimeout(err)
	}
}
func (s *Rows) Next() bool {
	s.enter("next")
	ok := s.raw.Next()
	s.leave(nil, "next")
	s.eof = !ok
	if ok && s.trace != nil {
		s.trace.read(func(q *querySnapshot) {
			n := *q.Rows + 1
			q.Rows = &n
			if q.Result != "error" {
				q.ReadState = "reading"
			}
		})
	}
	return ok
}
func (s *Rows) Scan(dest ...interface{}) error {
	s.enter("scan")
	err := s.raw.Scan(dest...)
	s.leave(err, "scan")
	if err == nil && s.trace != nil {
		s.trace.read(func(q *querySnapshot) {
			if q.ReadScope == "first_row" && q.Result != "error" {
				q.ReadState = "complete"
			}
		})
	}
	return err
}
func (s *Rows) Err() error {
	return s.err(true)
}
func (s *Rows) err(countRepeat bool) error {
	s.enter("err")
	err := s.raw.Err()
	if s.trace != nil {
		s.trace.leave()
		s.trace.observeFailure(err, "err", countRepeat)
		s.trace.triggerTimeout(err)
	}
	if s.trace != nil && err == nil && s.eof {
		s.trace.read(func(q *querySnapshot) {
			if q.Result != "error" {
				q.ReadState = "complete"
				if *q.Rows == 0 && s.trace.options.AllowEmpty {
					q.Result = "empty"
				}
			}
		})
	}
	return err
}
func (s *Rows) Columns() ([]string, error) {
	s.enter("columns")
	v, e := s.raw.Columns()
	s.leave(e, "columns")
	return v, e
}
func (s *Rows) ColumnTypes() ([]*sql.ColumnType, error) {
	s.enter("column_types")
	v, e := s.raw.ColumnTypes()
	s.leave(e, "column_types")
	return v, e
}
func (s *Rows) Close() error {
	s.closeOnce.Do(func() {
		s.enter("close")
		s.closeErr = s.raw.Close()
		s.leave(s.closeErr, "close")
		if s.trace != nil { // Err may wait for database/sql's asynchronous close; time it too.
			// This internal observation must not inflate the business call's repeat count.
			s.err(false)
			s.trace.finish()
		}
	})
	return s.closeErr
}

type Row struct {
	raw   *sql.Row
	trace *QueryTrace
}

func (s *Row) Scan(dest ...interface{}) error {
	if s.trace != nil {
		s.trace.enter("row_scan")
	}
	err := s.raw.Scan(dest...)
	if s.trace != nil {
		s.trace.leave()
		stage := "row_scan"
		if s.raw.Err() != nil {
			stage = "query"
		}
		s.trace.failure(err, stage)
		if err == nil {
			s.trace.read(func(q *querySnapshot) { n := int64(1); q.Rows = &n; q.ReadState = "complete" })
		}
		s.trace.finish()
		s.trace.triggerTimeout(err)
	}
	return err
}
