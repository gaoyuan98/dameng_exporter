package collector

import (
	"context"
	"dameng_exporter/config"
	"database/sql/driver"
	"errors"
	"math"
	"reflect"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
)

func TestTableOutcomes(t *testing.T) {
	setupCollectionLogTest(t)
	for _, f := range businessFixtures()[:2] {
		for _, name := range []string{"multi", "empty", "query", "timeout", "scan partial", "scan failed", "rows partial", "rows failed", "rows priority", "cache", "empty cache", "cache recovery", "marshal"} {
			t.Run(f.name+"/"+name, func(t *testing.T) {
				resetFixtureCache(t, f)
				row := append([]driver.Value(nil), f.row...)
				bad := append([]driver.Value(nil), row...)
				bad[1] = "not numeric"
				rows := [][]driver.Value{row}
				var queryErr, readErr error
				want := databaseOutcome(OutcomeSuccess, "")
				wantSamples := 3
				if f.name == "datafile" {
					wantSamples = 2
				}
				perRow := wantSamples
				cached := ""
				wantCalls := 1
				switch name {
				case "multi":
					rows = append(rows, row)
					wantSamples *= 2
				case "empty":
					rows = nil
					want.State = OutcomeEmpty
					wantSamples = 0
				case "query":
					queryErr = errors.New("permission denied")
					want.State, want.Reason = OutcomeFailed, "query_error"
					wantSamples = 0
				case "timeout":
					queryErr = errors.Join(errors.New("wrapped"), context.DeadlineExceeded)
					want.State, want.Reason = OutcomeFailed, "query_timeout"
					wantSamples = 0
				case "scan partial":
					rows = append(rows, bad)
					want.State, want.Reason = OutcomePartial, "scan_error"
				case "scan failed":
					rows = [][]driver.Value{bad}
					want.State, want.Reason = OutcomeFailed, "scan_error"
					wantSamples = 0
				case "rows partial":
					readErr = errors.New("interrupted")
					want.State, want.Reason = OutcomePartial, "rows_error"
				case "rows failed":
					rows = nil
					readErr = errors.New("interrupted")
					want.State, want.Reason = OutcomeFailed, "rows_error"
					wantSamples = 0
				case "rows priority":
					rows = append(rows, bad)
					readErr = errors.New("interrupted")
					want.State, want.Reason = OutcomePartial, "rows_error"
				case "cache":
					cached = `[{"TotalSize":100,"FreeSize":25}]`
					want = CollectorOutcome{State: OutcomeUnknown, Reason: "cache_unverified", DataOrigin: "cache"}
					wantCalls = 0
				case "empty cache":
					cached = `[]`
					want = CollectorOutcome{State: OutcomeUnknown, Reason: "cache_unverified", DataOrigin: "cache"}
					wantSamples = 0
					wantCalls = 0
				case "cache recovery":
					cached = `[{"TotalSize":100},{"TotalSize":"invalid"}]`
					want.State, want.Reason, want.CacheFallback = OutcomeUnknown, "cache_recovery_unverified", true
					wantSamples += 2 * perRow
				case "marshal":
					row[2] = math.NaN()
					want.State = OutcomeSuccess
				}
				if cached != "" {
					config.SetCache(f.cacheKey+"_fixture", cached, time.Minute)
				}
				conn, calls := fixtureDB(t, f, rows, queryErr, readErr, nil)
				c := fixtureCollector(f, conn).(ResultCollector)
				ch := make(chan prometheus.Metric, 100)
				got := c.CollectWithResult(ch)
				if got != want || len(ch) != wantSamples || *calls != wantCalls {
					t.Fatalf("outcome %+v samples %d queries %d; want %+v/%d/%d", got, len(ch), *calls, want, wantSamples, wantCalls)
				}
			})
		}
	}
}

func TestSystemInfoOutcomes(t *testing.T) {
	setupCollectionLogTest(t)
	f := businessFixtures()[3]
	for _, name := range []string{"normal", "null", "empty", "query", "timeout", "scan", "rows", "first row only"} {
		t.Run(name, func(t *testing.T) {
			rows := [][]driver.Value{append([]driver.Value(nil), f.row...)}
			var queryErr, readErr error
			want := databaseOutcome(OutcomeSuccess, "")
			wantSamples := 3
			switch name {
			case "null":
				rows = [][]driver.Value{{nil, nil, nil, nil}}
				wantSamples = 1
			case "empty":
				rows = nil
				want.State, want.Reason = OutcomeUnknown, "no_row_placeholder"
				wantSamples = 1
			case "query":
				queryErr = errors.New("permission denied")
				want.State, want.Reason = OutcomeFailed, "query_error"
				wantSamples = 0
			case "timeout":
				queryErr = context.DeadlineExceeded
				want.State, want.Reason = OutcomeFailed, "query_timeout"
				wantSamples = 0
			case "scan":
				rows[0][0] = "invalid"
				want.State, want.Reason = OutcomeFailed, "scan_error"
				wantSamples = 0
			case "rows":
				rows = nil
				readErr = errors.New("interrupted")
				want.State, want.Reason = OutcomeFailed, "rows_error"
				wantSamples = 0
			case "first row only":
				readErr = errors.New("unobserved trailing failure")
				rows = append(rows, []driver.Value{"invalid", nil, nil, nil})
			}
			conn, calls := fixtureDB(t, f, rows, queryErr, readErr, nil)
			ch := make(chan prometheus.Metric, 100)
			got := fixtureCollector(f, conn).(ResultCollector).CollectWithResult(ch)
			if got != want || len(ch) != wantSamples || *calls != 1 {
				t.Fatalf("got %+v/%d/%d want %+v/%d/1", got, len(ch), *calls, want, wantSamples)
			}
			// Compare legacy Collect to the new entry point, including labels and values.
			other := make(chan prometheus.Metric, 100)
			fixtureCollector(f, conn).Collect(other)
			if !reflect.DeepEqual(metricSnapshot(t, ch), metricSnapshot(t, other)) || *calls != 2 {
				t.Fatal("Collect changed business metrics or executed twice")
			}
		})
	}
}

func TestBusinessConnectionFailureOutcomes(t *testing.T) {
	setupCollectionLogTest(t)
	for _, f := range businessFixtures() {
		resetFixtureCache(t, f)
		ch := make(chan prometheus.Metric, 10)
		got := fixtureCollector(f, nil).(ResultCollector).CollectWithResult(ch)
		if got != databaseOutcome(OutcomeFailed, "datasource_unavailable") || len(ch) != 0 {
			t.Fatalf("%s: %+v", f.name, got)
		}
	}
}

func TestQueryTimeoutUsesTypeAndContext(t *testing.T) {
	ctx, cancel := context.WithDeadline(context.Background(), time.Now().Add(-time.Second))
	defer cancel()
	if got := queryFailure(ctx, errors.New("driver error")); got.Reason != "query_timeout" {
		t.Fatal(got)
	}
	if got := queryFailure(context.Background(), errors.New("DM 123 timeout denied")); got.Reason != "query_error" {
		t.Fatal("guessed an unverified DM code", got)
	}
}
