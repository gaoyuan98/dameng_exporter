package diagnostics

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"fmt"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"go.uber.org/zap/zaptest/observer"
	"io"
	"strings"
	"sync"
	"testing"
	"time"
)

type fakeClock struct {
	mu    sync.Mutex
	now   time.Time
	timer chan time.Time
}

func newClock() *fakeClock                                { return &fakeClock{now: time.Now(), timer: make(chan time.Time, 1)} }
func (c *fakeClock) Now() time.Time                       { c.mu.Lock(); defer c.mu.Unlock(); return c.now }
func (c *fakeClock) After(time.Duration) <-chan time.Time { return c.timer }
func (c *fakeClock) advance(d time.Duration)              { c.mu.Lock(); c.now = c.now.Add(d); c.mu.Unlock() }
func fixture(t *testing.T, threshold time.Duration) (*RoundTrace, *RunTrace, *fakeClock, *observer.ObservedLogs) {
	t.Helper()
	c := newClock()
	core, logs := observer.New(zap.DebugLevel)
	r := NewRound(Options{Clock: c, Threshold: threshold, Logger: zap.New(core).Sugar()})
	t.Cleanup(func() { r.Finish(nil) })
	return r, r.NewRun("database", "source", "fixture", "blocking", 0), c, logs
}

type connector struct {
	query func(context.Context, string, []driver.NamedValue) (driver.Rows, error)
}

func (c connector) Connect(context.Context) (driver.Conn, error) { return connection{c.query}, nil }
func (c connector) Driver() driver.Driver                        { return testDriver{} }

type testDriver struct{}

func (testDriver) Open(string) (driver.Conn, error) { return nil, errors.New("unused") }

type connection struct {
	query func(context.Context, string, []driver.NamedValue) (driver.Rows, error)
}

func (c connection) Prepare(string) (driver.Stmt, error) { return nil, errors.New("unused") }
func (c connection) Close() error                        { return nil }
func (c connection) Begin() (driver.Tx, error)           { return nil, errors.New("unused") }
func (c connection) QueryContext(ctx context.Context, q string, a []driver.NamedValue) (driver.Rows, error) {
	return c.query(ctx, q, a)
}

type driverRows struct {
	values      []driver.Value
	index       int
	readErr     error
	next, close func()
}

func (*driverRows) Columns() []string { return []string{"value"} }
func (r *driverRows) Close() error {
	if r.close != nil {
		r.close()
	}
	return nil
}
func (r *driverRows) Next(v []driver.Value) error {
	if r.next != nil {
		r.next()
	}
	if r.index == len(r.values) {
		if r.readErr != nil {
			return r.readErr
		}
		return io.EOF
	}
	v[0] = r.values[r.index]
	r.index++
	return nil
}
func openDB(t *testing.T, q func(context.Context, string, []driver.NamedValue) (driver.Rows, error)) *sql.DB {
	t.Helper()
	db := sql.OpenDB(connector{q})
	t.Cleanup(func() { db.Close() })
	return db
}
func event(t *testing.T, l *observer.ObservedLogs, name string) map[string]interface{} {
	t.Helper()
	all := l.FilterMessage(name).All()
	if len(all) == 0 {
		t.Fatalf("missing event %s", name)
	}
	return all[len(all)-1].ContextMap()
}
func wait(t *testing.T, c <-chan struct{}) {
	t.Helper()
	select {
	case <-c:
	case <-time.After(3 * time.Second):
		t.Fatal("blocked operation did not finish")
	}
}
func TestRunningQueryAndWallTime(t *testing.T) {
	r, u, c, logs := fixture(t, 30*time.Second)
	c.advance(time.Second)
	entered, release, done := make(chan struct{}), make(chan struct{}), make(chan struct{})
	db := openDB(t, func(context.Context, string, []driver.NamedValue) (driver.Rows, error) {
		close(entered)
		<-release
		return &driverRows{values: []driver.Value{int64(1)}}, nil
	})
	go func() {
		defer close(done)
		rows, err := Wrap(db, u).QueryContext(context.Background(), "select\n'quoted\\value'")
		if err != nil {
			return
		}
		for rows.Next() {
			var n int
			rows.Scan(&n)
		}
		rows.Err()
		rows.Close()
	}()
	wait(t, entered)
	c.advance(29 * time.Second)
	r.Trigger("duration_threshold")
	f := event(t, logs, "sql_top")
	if f["state"] != "running" || f["sql_phase"] != "query" || f["sql_duration_ms"] != int64(29000) {
		t.Fatal(f)
	}
	close(release)
	wait(t, done)
	u.Finish(1, 0, false)
	r.Finish(nil)
	if logs.FilterMessage("scrape_diagnostic").Len() != 1 || logs.FilterMessage("sql_top").Len() != 2 {
		t.Fatal("running to finished must refresh once")
	}
}
func TestAPIIntervalsExcludeProcessingAndRetainTopFive(t *testing.T) {
	r, u, c, logs := fixture(t, 30*time.Second)
	db := openDB(t, func(_ context.Context, q string, _ []driver.NamedValue) (driver.Rows, error) {
		var n int
		fmt.Sscan(q, &n)
		c.advance(time.Duration(n) * time.Second)
		return &driverRows{}, nil
	})
	for i := 1; i <= 8; i++ {
		rows, err := Wrap(db, u).QueryContext(context.Background(), fmt.Sprint(i))
		if err != nil {
			t.Fatal(err)
		}
		c.advance(10 * time.Second)
		rows.Next()
		rows.Err()
		rows.Close()
	}
	r.Trigger("manual")
	tops := logs.FilterMessage("sql_top").All()
	if len(tops) != 5 {
		t.Fatal(len(tops))
	}
	for i, e := range tops {
		f := e.ContextMap()
		if f["sql"] != fmt.Sprint(8-i) || f["sql_duration_ms"] != int64((8-i)*1000) || f["sql_wall_ms"] != int64((18-i)*1000) {
			t.Fatal(f)
		}
	}
	r.mu.Lock()
	if len(r.top) != 5 || len(r.active) != 0 {
		t.Fatal("unbounded history")
	}
	r.mu.Unlock()
	u.Finish(0, 0, false)
	r.Finish(nil)
	if logs.FilterMessage("sql_top").Len() != 5 {
		t.Fatal("unchanged final top was repeated")
	}
}

type blockingScanner struct{ entered, release chan struct{} }

func (s blockingScanner) Scan(interface{}) error { close(s.entered); <-s.release; return nil }
func TestRowsAPIStagesAndCloseLifetime(t *testing.T) {
	for _, stage := range []string{"next", "scan", "close", "row_scan"} {
		t.Run(stage, func(t *testing.T) {
			r, u, c, logs := fixture(t, 3*time.Second)
			entered, release, done := make(chan struct{}), make(chan struct{}), make(chan struct{})
			block := func() { close(entered); <-release }
			db := openDB(t, func(context.Context, string, []driver.NamedValue) (driver.Rows, error) {
				v := &driverRows{values: []driver.Value{int64(1)}}
				if stage == "next" {
					var once sync.Once
					v.next = func() { once.Do(block) }
				}
				if stage == "close" || stage == "row_scan" {
					v.close = block
				}
				return v, nil
			})
			go func() {
				defer close(done)
				if stage == "row_scan" {
					var n int
					Wrap(db, u).QueryRowContext(context.Background(), "select value").Scan(&n)
					return
				}
				rows, _ := Wrap(db, u).QueryContext(context.Background(), "select value")
				rows.Next()
				if stage == "scan" {
					rows.Scan(blockingScanner{entered, release})
				} else {
					var n int
					rows.Scan(&n)
				}
				rows.Close()
			}()
			wait(t, entered)
			c.advance(3 * time.Second)
			r.Trigger("manual")
			f := event(t, logs, "sql_top")
			if f["sql_phase"] != stage || f["state"] != "running" || logs.FilterMessage("sql_finished").Len() != 0 {
				t.Fatal(f)
			}
			close(release)
			wait(t, done)
			f = event(t, logs, "sql_finished")
			if f["sql_duration_ms"] != int64(3000) {
				t.Fatal(f)
			}
			if stage == "row_scan" && (f["scan_ms"] != nil || f["close_ms"] != nil || f["row_scan_ms"] != int64(3000)) {
				t.Fatal(f)
			}
		})
	}
}
func TestScanErrorsDeduplicateAndSurviveClose(t *testing.T) {
	r, u, _, logs := fixture(t, 0)
	db := openDB(t, func(context.Context, string, []driver.NamedValue) (driver.Rows, error) {
		return &driverRows{values: []driver.Value{"bad", "bad"}}, nil
	})
	rows, _ := Wrap(db, u).QueryContext(context.Background(), "select invalid")
	for rows.Next() {
		var n int
		if rows.Scan(&n) == nil {
			t.Fatal("scan error changed")
		}
	}
	rows.Err()
	rows.Close()
	rows.Close()
	u.Finish(0, 0, false)
	r.Finish(nil)
	f := event(t, logs, "scrape_finished")
	if f["sql_error_calls"] != int64(1) || f["issue_runs"] != int64(1) || logs.FilterMessage("sql_error").Len() != 1 || logs.FilterMessage("sql_finished").Len() != 1 {
		t.Fatal(f)
	}
	q := event(t, logs, "sql_finished")
	if q["read_state"] != "partial" || q["result"] != "error" || q["scan_error_repeats"] != int64(1) {
		t.Fatal(q)
	}
}
func TestReadErrorsKeepOriginalChain(t *testing.T) {
	for _, calls := range []int{0, 1, 2} {
		t.Run(fmt.Sprintf("explicit_err_calls_%d", calls), func(t *testing.T) {
			_, u, _, logs := fixture(t, 0)
			original := errors.New("read interrupted")
			db := openDB(t, func(context.Context, string, []driver.NamedValue) (driver.Rows, error) {
				return &driverRows{readErr: original}, nil
			})
			rows, _ := Wrap(db, u).QueryContext(context.Background(), "select value")
			if rows.Next() {
				t.Fatal("unexpected row")
			}
			for i := 0; i < calls; i++ {
				if !errors.Is(rows.Err(), original) {
					t.Fatal("error chain lost")
				}
			}
			rows.Close()
			q := event(t, logs, "sql_finished")
			if logs.FilterMessage("sql_error").Len() != 1 || q["result"] != "error" || q["err_error_repeats"] != int64(max(0, calls-1)) {
				t.Fatal(logs.All())
			}
		})
	}
}
func TestRowEmptyProbeAndPermission(t *testing.T) {
	for _, kind := range []string{"empty_allowed", "empty_required", "probe", "permission"} {
		t.Run(kind, func(t *testing.T) {
			r, u, _, logs := fixture(t, 0)
			missing := errors.New("known missing field")
			db := openDB(t, func(context.Context, string, []driver.NamedValue) (driver.Rows, error) {
				switch kind {
				case "probe":
					return nil, missing
				case "permission":
					return nil, errors.New("permission denied")
				}
				return &driverRows{}, nil
			})
			ctx := WithQueryOptions(context.Background(), QueryOptions{AllowEmpty: kind == "empty_allowed", Purpose: "probe", ExpectedError: func(e error) bool { return errors.Is(e, missing) }})
			var n int
			err := Wrap(db, u).QueryRowContext(ctx, "select value").Scan(&n)
			if err == nil {
				t.Fatal("driver error changed")
			}
			u.Finish(0, 0, false)
			r.Finish(nil)
			f := event(t, logs, "scrape_finished")
			want := int64(1)
			if kind == "probe" || kind == "empty_allowed" {
				want = 0
			}
			if f["sql_error_calls"] != want || int64(logs.FilterMessage("sql_error").Len()) != want {
				t.Fatal(f)
			}
			if kind == "empty_required" && event(t, logs, "sql_finished")["rows_read"] != nil {
				t.Fatal("unconfirmed Row.Scan fabricated row count")
			}
		})
	}
}
func TestDetachPerRunFreezesForeground(t *testing.T) {
	r, a, c, logs := fixture(t, 2*time.Second)
	b := r.NewRun("database", "other", "B", "blocking", 0)
	entered, release, done := make(chan struct{}), make(chan struct{}), make(chan struct{})
	db := openDB(t, func(_ context.Context, q string, _ []driver.NamedValue) (driver.Rows, error) {
		if q == "first" {
			close(entered)
			<-release
		}
		c.advance(time.Second)
		return nil, errors.New("background failure")
	})
	go func() {
		defer close(done)
		Wrap(db, a).QueryContext(context.Background(), "first")
		Wrap(db, a).QueryContext(context.Background(), "late")
		a.BackgroundFinished()
	}()
	wait(t, entered)
	c.advance(2 * time.Second)
	a.Detach()
	f := event(t, logs, "sql_top")
	if f["state"] != "detached" || f["sql_duration_ms"] != int64(2000) {
		t.Fatal(f)
	}
	close(release)
	wait(t, done)
	a.Finish(2, 0, false)
	b.Finish(0, 0, false)
	r.Finish(nil)
	f = event(t, logs, "scrape_finished")
	if f["sql_calls"] != int64(1) || f["sql_error_calls"] != int64(0) || f["sql_unfinished_at_cutoff"] != int64(1) || f["frontend_timeout_runs"] != int64(1) {
		t.Fatal(f)
	}
	for _, e := range logs.FilterMessage("sql_error").All() {
		if e.ContextMap()["phase"] != "background" {
			t.Fatal(e)
		}
	}
	if logs.FilterMessage("sql_top").Len() != 1 {
		t.Fatal("background completion changed frozen top")
	}
}
func TestContextBudgetAndAlreadyExpiredAttempts(t *testing.T) {
	r, u, c, logs := fixture(t, 0)
	deadline := c.Now().Add(30 * time.Second)
	ctx, cancel := context.WithDeadline(context.Background(), deadline)
	defer cancel()
	db := openDB(t, func(context.Context, string, []driver.NamedValue) (driver.Rows, error) {
		c.advance(29 * time.Second)
		return &driverRows{}, nil
	})
	rows, _ := Wrap(db, u).QueryContext(ctx, "slow success")
	rows.Next()
	rows.Err()
	rows.Close()
	c.advance(2 * time.Second)
	expired, stop := context.WithDeadline(context.Background(), time.Now().Add(-time.Second))
	defer stop()
	_, err := Wrap(db, u).QueryContext(expired, "expired attempt")
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatal(err)
	}
	u.Finish(0, 0, false)
	r.Finish(nil)
	f := event(t, logs, "sql_error")
	if f["budget_remaining_ms_at_start"] != int64(0) || f["context_state_at_start"] != "deadline_exceeded" {
		t.Fatal(f)
	}
	if event(t, logs, "scrape_finished")["sql_calls"] != int64(2) {
		t.Fatal("expired attempt not counted")
	}
	first := logs.FilterMessage("sql_finished").All()[0].ContextMap()
	if first["budget_remaining_ms_at_start"] != int64(30000) {
		t.Fatal(first)
	}
}
func TestSnapshotFinishRacesAndBoundedHistory(t *testing.T) {
	for i := 0; i < 50; i++ {
		r, u, c, logs := fixture(t, time.Second)
		q := u.begin(context.Background(), "select value", false)
		q.leave()
		q.read(func(s *querySnapshot) { s.ReadState = "complete" })
		c.advance(time.Second)
		var wg sync.WaitGroup
		for _, action := range []func(){func() { r.Trigger("duration_threshold") }, func() { c.timer <- c.Now() }, func() { q.finish(); u.Finish(0, 0, false); r.Finish(nil) }} {
			wg.Add(1)
			go func(f func()) { defer wg.Done(); f() }(action)
		}
		wg.Wait()
		finished := false
		diagnostic := 0
		for _, e := range logs.All() {
			if e.Message == "scrape_finished" {
				finished = true
			}
			if e.Message == "scrape_diagnostic" {
				diagnostic++
				if finished {
					t.Fatal("diagnostic after final summary")
				}
			}
		}
		if diagnostic > 1 {
			t.Fatal("duplicate group")
		}
		if tops := logs.FilterMessage("sql_top").Len(); tops < 1 || tops > 2 {
			t.Fatal("threshold/finish race lost or duplicated top", tops)
		}
	}
}
func TestThousandsOfCallsRetainFiveAndReleaseAtFinish(t *testing.T) {
	r, u, _, _ := fixture(t, 0)
	for i := 0; i < 1000; i++ {
		q := u.begin(context.Background(), "select value", false)
		q.leave()
		q.read(func(s *querySnapshot) { s.ReadState = "complete" })
		q.finish()
	}
	if len(r.top) != 5 || len(r.active) != 0 {
		t.Fatal("history not bounded")
	}
	u.Finish(0, 0, false)
	if len(r.runs) != 0 {
		t.Fatal("completed run retained")
	}
	r.Finish(nil)
	if r.top != nil || r.active != nil || r.runs != nil {
		t.Fatal("round retained snapshots")
	}
}

func TestAsyncCancellationCloseRemainsVisible(t *testing.T) {
	for _, phase := range []string{"close", "err"} {
		t.Run(phase, func(t *testing.T) {
			r, u, c, logs := fixture(t, 5*time.Second)
			closing, release, done := make(chan struct{}), make(chan struct{}), make(chan struct{})
			db := openDB(t, func(context.Context, string, []driver.NamedValue) (driver.Rows, error) {
				return &driverRows{values: []driver.Value{int64(1)}, close: func() { close(closing); <-release }}, nil
			})
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			rows, _ := Wrap(db, u).QueryContext(ctx, "select value")
			rows.Next()
			var n int
			rows.Scan(&n)
			if phase == "err" {
				go rows.raw.Close()
			} else {
				cancel()
			}
			wait(t, closing)
			go func() {
				defer close(done)
				if phase == "err" {
					rows.Err()
				} else {
					rows.Close()
				}
			}()
			deadline := time.Now().Add(time.Second)
			for {
				r.mu.Lock()
				current := rows.trace.data.SQLPhase
				r.mu.Unlock()
				if current == phase {
					break
				}
				if time.Now().After(deadline) {
					t.Fatal("wrapper never entered blocked API")
				}
			}
			c.advance(5 * time.Second)
			r.Trigger("manual")
			f := event(t, logs, "sql_top")
			if f["sql_phase"] != phase || logs.FilterMessage("sql_finished").Len() != 0 {
				t.Fatal(f)
			}
			close(release)
			wait(t, done)
			rows.Close()
			if logs.FilterMessage("sql_finished").Len() != 1 || (phase == "close" && event(t, logs, "sql_finished")["result"] != "error") {
				t.Fatal(logs.All())
			}
		})
	}
}
func TestQueryRowRegisteredBeforeQueryReturns(t *testing.T) {
	r, u, c, logs := fixture(t, 29*time.Second)
	entered, release, done := make(chan struct{}), make(chan struct{}), make(chan struct{})
	db := openDB(t, func(context.Context, string, []driver.NamedValue) (driver.Rows, error) {
		close(entered)
		<-release
		return &driverRows{values: []driver.Value{int64(1)}}, nil
	})
	go func() {
		defer close(done)
		var n int
		Wrap(db, u).QueryRowContext(context.Background(), "select value").Scan(&n)
	}()
	wait(t, entered)
	c.advance(29 * time.Second)
	r.Trigger("manual")
	if f := event(t, logs, "sql_top"); f["sql_phase"] != "query" || f["sql_duration_ms"] != int64(29000) {
		t.Fatal(f)
	}
	close(release)
	wait(t, done)
	if logs.FilterMessage("sql_finished").Len() != 1 {
		t.Fatal("QueryRow counted twice")
	}
}

type multiRows struct {
	driverRows
	nextSets int
}

func (r *multiRows) HasNextResultSet() bool { return true }
func (r *multiRows) NextResultSet() error   { r.nextSets++; return io.EOF }
func TestReadScopeDoesNotConsumeAdditionalResultSets(t *testing.T) {
	for _, first := range []bool{true, false} {
		t.Run(fmt.Sprint(first), func(t *testing.T) {
			_, u, _, logs := fixture(t, 0)
			raw := &multiRows{driverRows: driverRows{values: []driver.Value{int64(1), int64(2)}}}
			db := openDB(t, func(context.Context, string, []driver.NamedValue) (driver.Rows, error) { return raw, nil })
			scope := "first_result_set"
			if first {
				scope = "first_row"
			}
			ctx := WithQueryOptions(context.Background(), QueryOptions{ReadScope: scope})
			rows, _ := Wrap(db, u).QueryContext(ctx, "select value")
			for rows.Next() {
				var n int
				rows.Scan(&n)
				if first {
					break
				}
			}
			rows.Err()
			rows.Close()
			f := event(t, logs, "sql_finished")
			if f["read_scope"] != scope || f["read_state"] != "complete" || raw.nextSets != 0 {
				t.Fatal(f)
			}
			if first && raw.index != 1 {
				t.Fatal("extra row read")
			}
		})
	}
}
func TestBadConnectionRetryIsOneApplicationCall(t *testing.T) {
	r, u, _, logs := fixture(t, 0)
	attempts := 0
	db := openDB(t, func(context.Context, string, []driver.NamedValue) (driver.Rows, error) {
		attempts++
		if attempts < 3 {
			return nil, driver.ErrBadConn
		}
		return &driverRows{}, nil
	})
	rows, err := Wrap(db, u).QueryContext(context.Background(), "select retry")
	if err != nil {
		t.Fatal(err)
	}
	rows.Next()
	rows.Err()
	rows.Close()
	u.Finish(0, 0, false)
	r.Finish(nil)
	if attempts != 3 || event(t, logs, "scrape_finished")["sql_calls"] != int64(1) || logs.FilterMessage("sql_started").Len() != 1 {
		t.Fatal("internal retries leaked into application call count")
	}
}
func TestExpiredContextDoesNotReclassifyOrdinaryDriverError(t *testing.T) {
	r, u, _, logs := fixture(t, 0)
	permission := errors.New("permission denied")
	db := openDB(t, func(ctx context.Context, _ string, _ []driver.NamedValue) (driver.Rows, error) {
		<-ctx.Done()
		return nil, permission
	})
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()
	_, err := Wrap(db, u).QueryContext(ctx, "select denied")
	if !errors.Is(err, permission) {
		t.Fatal(err)
	}
	f := event(t, logs, "sql_error")
	if f["timeout"] != false || f["context_expired"] != true || logs.FilterMessage("scrape_diagnostic").Len() != 0 {
		t.Fatal(f)
	}
	u.Finish(0, 0, false)
	r.Finish(nil)
}
func TestCustomNamesAndActualContextBudgets(t *testing.T) {
	r, a, c, logs := fixture(t, 0)
	b := r.NewRun("database", "second", "custom", "blocking", 0)
	for index, u := range []*RunTrace{a, b} {
		ctx, cancel := context.WithDeadline(context.Background(), c.Now().Add(time.Duration(index+1)*10*time.Second))
		ctx = WithQueryOptions(ctx, QueryOptions{Purpose: "custom", Name: "", Index: index + 1})
		q := u.begin(ctx, "identical SQL", false)
		q.leave()
		q.read(func(s *querySnapshot) { s.ReadState = "complete" })
		q.finish()
		cancel()
	}
	events := logs.FilterMessage("sql_started").All()
	for i, e := range events {
		f := e.ContextMap()
		if f["query_name"] != "" || f["query_index"] != int64(i+1) || f["budget_remaining_ms_at_start"] != int64((i+1)*10000) {
			t.Fatal(f)
		}
	}
	if events[0].ContextMap()["run_id"] == events[1].ContextMap()["run_id"] || events[0].ContextMap()["query_id"] == events[1].ContextMap()["query_id"] {
		t.Fatal("identical SQL merged")
	}
}
func TestDurationTriggerAndZeroSQL(t *testing.T) {
	r, u, c, logs := fixture(t, time.Second)
	c.advance(time.Second)
	c.timer <- c.Now()
	wait(t, r.observed)
	u.Finish(0, 0, false)
	r.Finish(nil)
	if logs.FilterMessage("scrape_diagnostic").Len() != 1 || logs.FilterMessage("sql_top").Len() != 0 || event(t, logs, "scrape_finished")["top_count"] != int64(0) {
		t.Fatal(logs.All())
	}
}

func TestIssuesDoNotBypassDurationThreshold(t *testing.T) {
	for _, timing := range []struct {
		name               string
		threshold, elapsed time.Duration
	}{
		{"below_threshold", 30 * time.Second, 75 * time.Millisecond},
		{"disabled", 0, 31 * time.Second},
	} {
		for _, kind := range []string{"sql_error", "sql_timeout", "collector_error", "panic", "frontend_timeout", "dropped_samples", "partial_read"} {
			t.Run(timing.name+"/"+kind, func(t *testing.T) {
				r, u, c, logs := fixture(t, timing.threshold)
				db := openDB(t, func(context.Context, string, []driver.NamedValue) (driver.Rows, error) {
					c.advance(timing.elapsed)
					switch kind {
					case "sql_error":
						return nil, errors.New("permission denied")
					case "sql_timeout":
						return nil, context.DeadlineExceeded
					}
					return &driverRows{}, nil
				})
				rows, err := Wrap(db, u).QueryContext(context.Background(), "select value")
				if err == nil {
					if kind != "partial_read" {
						rows.Next()
						rows.Err()
					}
					rows.Close()
				}
				var dropped int64
				switch kind {
				case "collector_error":
					u.Error("parse_error", errors.New("invalid value"))
				case "panic":
					u.Panic("test panic", "test stack")
				case "frontend_timeout":
					u.Detach()
				case "dropped_samples":
					dropped = 1
				}
				if logs.FilterMessage("scrape_diagnostic").Len() != 0 || logs.FilterMessage("sql_top").Len() != 0 {
					t.Fatal("issue bypassed duration gate during collection", logs.All())
				}
				u.Finish(0, dropped, false)
				r.Finish(nil)
				f := event(t, logs, "scrape_finished")
				if f["issue_runs"] != int64(1) || !strings.Contains(f["diagnostic_reasons"].(string), "unexpected_issue") {
					t.Fatal("issue fact lost", f)
				}
				if logs.FilterMessage("scrape_diagnostic").Len() != 0 || logs.FilterMessage("sql_top").Len() != 0 || f["top_refreshed"] != false || f["last_top_snapshot_id"] != uint64(0) {
					t.Fatal("issue bypassed duration gate at finish", f)
				}
				errorEvent := kind
				if kind == "sql_timeout" {
					errorEvent = "sql_error"
				}
				if kind == "panic" {
					errorEvent = "collector_panic"
				}
				if kind == "sql_error" || kind == "sql_timeout" || kind == "collector_error" || kind == "panic" {
					if logs.FilterMessage(errorEvent).Len() != 1 {
						t.Fatal("independent error log lost", logs.All())
					}
				}
				if kind == "sql_timeout" || kind == "frontend_timeout" || kind == "panic" {
					if !strings.Contains(f["diagnostic_reasons"].(string), kind) {
						t.Fatal("early trigger reason lost", f)
					}
				}
			})
		}
	}
}

func TestEarlyIssueStillAllowsDurationTrigger(t *testing.T) {
	for _, kind := range []string{"sql_timeout", "frontend_timeout", "panic"} {
		t.Run(kind, func(t *testing.T) {
			r, u, c, logs := fixture(t, 30*time.Second)
			q := u.begin(context.Background(), "select value", false)
			c.advance(75 * time.Millisecond)
			q.leave()
			q.read(func(s *querySnapshot) { s.ReadState = "complete" })
			if kind == "sql_timeout" {
				q.failure(context.DeadlineExceeded, "query")
			}
			q.finish()
			switch kind {
			case "sql_timeout":
				q.triggerTimeout(context.DeadlineExceeded)
			case "frontend_timeout":
				u.Detach()
			case "panic":
				u.Panic("test panic", "test stack")
			}
			c.advance(30*time.Second - 75*time.Millisecond - time.Nanosecond)
			r.Trigger(kind)
			if logs.FilterMessage("scrape_diagnostic").Len() != 0 || logs.FilterMessage("sql_top").Len() != 0 {
				t.Fatal("snapshot before threshold", logs.All())
			}
			c.advance(time.Nanosecond)
			c.timer <- c.Now()
			wait(t, r.observed)
			if logs.FilterMessage("scrape_diagnostic").Len() != 1 || logs.FilterMessage("sql_top").Len() != 1 {
				t.Fatal("early issue consumed duration trigger", logs.All())
			}
			u.Finish(0, 0, false)
			r.Finish(nil)
			f := event(t, logs, "scrape_finished")
			reasons := f["diagnostic_reasons"].(string)
			if !strings.Contains(reasons, kind) || !strings.Contains(reasons, "duration_threshold") || f["top_refreshed"] != false || f["last_top_snapshot_id"] != uint64(1) {
				t.Fatal("duration snapshot or reasons lost", f)
			}
		})
	}
}

func TestFinishDurationThresholdBoundary(t *testing.T) {
	for _, tt := range []struct {
		name    string
		elapsed time.Duration
		wantTop bool
	}{
		{"before", 30*time.Second - time.Nanosecond, false},
		{"exact", 30 * time.Second, true},
		{"after", 30*time.Second + time.Nanosecond, true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			r, u, c, logs := fixture(t, 30*time.Second)
			q := u.begin(context.Background(), "select value", false)
			c.advance(tt.elapsed)
			q.leave()
			q.read(func(s *querySnapshot) { s.ReadState = "complete" })
			q.finish()
			u.Finish(0, 0, false)
			// The timer is deliberately not delivered: Finish must cover late scheduling.
			r.Finish(nil)
			f := event(t, logs, "scrape_finished")
			wantCount := 0
			if tt.wantTop {
				wantCount = 1
			}
			if logs.FilterMessage("scrape_diagnostic").Len() != 0 || logs.FilterMessage("sql_top").Len() != wantCount || f["top_refreshed"] != tt.wantTop {
				t.Fatal("incorrect final duration gate", f)
			}
		})
	}
}

func TestConcurrentFlowStagesRestoreNestedScopes(t *testing.T) {
	r, u, c, logs := fixture(t, time.Second)
	process := u.Stage("worker", "process")
	forward := u.Stage("forwarder", "metric_forward")
	handling := u.Stage("worker", "error_handling")
	health := u.Stage("worker", "health_check")
	c.advance(time.Second)
	r.Trigger("manual")
	if f := event(t, logs, "scrape_diagnostic"); f["active_stages"] != "health_check:1,metric_forward:1" {
		t.Fatal(f)
	}
	health()
	r.mu.Lock()
	if u.stages["worker"] != "error_handling" || u.stages["forwarder"] != "metric_forward" {
		t.Fatal(u.stages)
	}
	r.mu.Unlock()
	handling()
	forward()
	process()
	u.Finish(0, 0, false)
	r.Finish(nil)
}
func TestFirstScanFailureSurvivesFollowingSuccessfulRows(t *testing.T) {
	_, u, _, logs := fixture(t, 0)
	db := openDB(t, func(context.Context, string, []driver.NamedValue) (driver.Rows, error) {
		return &driverRows{values: []driver.Value{"bad", int64(1)}}, nil
	})
	rows, _ := Wrap(db, u).QueryContext(context.Background(), "select value")
	for rows.Next() {
		var n int
		rows.Scan(&n)
	}
	rows.Err()
	rows.Close()
	if f := event(t, logs, "sql_finished"); f["read_state"] != "partial" || f["result"] != "error" {
		t.Fatal(f)
	}
}

type blockingLog struct {
	Logger
	entered, release chan struct{}
	once             sync.Once
}

func (l *blockingLog) Infow(message string, args ...interface{}) {
	if message == "scrape_diagnostic" {
		l.once.Do(func() { close(l.entered); <-l.release })
	}
	l.Logger.Infow(message, args...)
}
func TestLogIOIsOutsideStateLockAndGroupsStayOrdered(t *testing.T) {
	c := newClock()
	core, logs := observer.New(zap.DebugLevel)
	sink := &blockingLog{Logger: zap.New(core).Sugar(), entered: make(chan struct{}), release: make(chan struct{})}
	r := NewRound(Options{Clock: c, Threshold: time.Second, Logger: sink})
	u := r.NewRun("database", "source", "fixture", "blocking", 0)
	q := u.begin(context.Background(), "select value", false)
	triggered := make(chan struct{})
	c.advance(time.Second)
	go func() { defer close(triggered); r.Trigger("manual") }()
	wait(t, sink.entered)
	updated := make(chan struct{})
	go func() {
		defer close(updated)
		q.leave()
		q.read(func(s *querySnapshot) { s.ReadState = "complete" })
		q.finish()
		u.Finish(0, 0, false)
	}()
	wait(t, updated)
	finished := make(chan struct{})
	go func() { defer close(finished); r.Finish(nil) }()
	close(sink.release)
	wait(t, triggered)
	wait(t, finished)
	finalSeen := false
	for _, e := range logs.All() {
		if e.Message == "scrape_finished" {
			finalSeen = true
		}
		if finalSeen && e.Message == "scrape_diagnostic" {
			t.Fatal("old diagnostic overtook final summary")
		}
	}
}

func TestSQLFactsIndependentOfLogFiltering(t *testing.T) {
	for _, level := range []zapcore.Level{zap.InfoLevel, zap.DebugLevel, zap.ErrorLevel} {
		t.Run(level.String(), func(t *testing.T) {
			clock := newClock()
			filter := zap.NewAtomicLevelAt(level)
			core, logs := observer.New(filter)
			r := NewRound(Options{Clock: clock, Logger: zap.New(core).Sugar()})
			u := r.NewRun("database", "source", "fixture", "blocking", 0)
			db := openDB(t, func(_ context.Context, q string, _ []driver.NamedValue) (driver.Rows, error) {
				if q == "failure" {
					return nil, errors.New("denied")
				}
				return &driverRows{}, nil
			})
			rows, _ := Wrap(db, u).QueryContext(context.Background(), "success")
			rows.Next()
			rows.Err()
			rows.Close()
			Wrap(db, u).QueryContext(context.Background(), "failure")
			u.Finish(2, 0, false)
			filter.SetLevel(zap.InfoLevel)
			r.Finish(nil)
			f := event(t, logs, "scrape_finished")
			if f["sql_calls"] != int64(2) || f["sql_error_calls"] != int64(1) || f["samples"] != int64(2) || f["issue_runs"] != int64(1) {
				t.Fatal(f)
			}
		})
	}
}

func TestSharedBudgetKeepsSlowSuccessAndSmallFinalBudget(t *testing.T) {
	r, u, c, logs := fixture(t, 30*time.Second)
	ctx, cancel := context.WithDeadline(context.Background(), c.Now().Add(30*time.Second))
	defer cancel()
	db := openDB(t, func(_ context.Context, q string, _ []driver.NamedValue) (driver.Rows, error) {
		if q == "slow success" {
			c.advance(29 * time.Second)
			return &driverRows{}, nil
		}
		c.advance(time.Second)
		return nil, context.DeadlineExceeded
	})
	rows, _ := Wrap(db, u).QueryContext(ctx, "slow success")
	rows.Next()
	rows.Err()
	rows.Close()
	Wrap(db, u).QueryContext(ctx, "last timeout")
	f := event(t, logs, "sql_error")
	if f["budget_remaining_ms_at_start"] != int64(1000) || f["sql_duration_ms"] != int64(1000) {
		t.Fatal(f)
	}
	top := logs.FilterMessage("sql_top").All()
	if len(top) != 2 || top[0].ContextMap()["sql"] != "slow success" || top[0].ContextMap()["sql_duration_ms"] != int64(29000) || top[1].ContextMap()["state"] != "finished" {
		t.Fatal(top)
	}
	u.Finish(0, 0, false)
	r.Finish(nil)
}
func TestQueryStartCompletionAndCutoffRaceHasOneAttribution(t *testing.T) {
	for i := 0; i < 100; i++ {
		r, u, _, logs := fixture(t, 0)
		var wg sync.WaitGroup
		wg.Add(2)
		go func() {
			defer wg.Done()
			q := u.begin(context.Background(), "select racing", false)
			q.leave()
			q.read(func(s *querySnapshot) { s.ReadState = "complete" })
			q.finish()
		}()
		go func() { defer wg.Done(); u.Detach() }()
		wg.Wait()
		u.Finish(0, 0, false)
		r.Finish(nil)
		f := event(t, logs, "scrape_finished")
		calls := f["sql_calls"].(int64)
		unfinished := f["sql_unfinished_at_cutoff"].(int64)
		if calls > 1 || unfinished > calls || f["frontend_timeout_runs"] != int64(1) || f["sql_error_calls"] != int64(0) {
			t.Fatal(f)
		}
	}
}
