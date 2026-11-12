package collector

import (
	"context"
	"dameng_exporter/config"
	"database/sql"
	"database/sql/driver"
	"errors"
	"reflect"
	"sync"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
)

func collectTableFixture(t *testing.T, f businessFixture, conn *sql.DB) (CollectorOutcome, []string) {
	t.Helper()
	out := make(chan prometheus.Metric, 20)
	result := fixtureCollector(f, conn).(ResultCollector).CollectWithResult(out)
	return result, metricSnapshot(t, out)
}

func TestTableCacheRecoversAfterIncompleteRead(t *testing.T) {
	setupCollectionLogTest(t)
	for _, f := range businessFixtures()[:2] {
		for _, scenario := range []string{"rows partial", "rows failed", "scan partial", "scan failed", "corrupt cache query failed", "corrupt cache rows partial", "corrupt cache empty"} {
			t.Run(f.name+"/"+scenario, func(t *testing.T) {
				resetFixtureCache(t, f)
				key := f.cacheKey + "_fixture"
				corrupt := scenario == "corrupt cache query failed" || scenario == "corrupt cache rows partial" || scenario == "corrupt cache empty"
				badCache := `[{"TablespaceName":"GHOST","Path":"GHOST","TotalSize":100},{"TotalSize":"invalid"}]`
				if corrupt {
					config.SetCache(key, badCache, time.Minute)
				}
				calls := 0
				conn := sql.OpenDB(sessionTestConnector{query: func(_ context.Context, query string, _ []driver.NamedValue) (driver.Rows, error) {
					calls++
					if query != f.query {
						t.Errorf("unexpected SQL: %s", query)
					}
					if calls > 1 {
						other := append([]driver.Value(nil), f.row...)
						other[0] = "SECOND"
						return &outcomeRows{columns: f.columns, values: [][]driver.Value{f.row, other}}, nil
					}
					bad := append([]driver.Value(nil), f.row...)
					bad[1] = "invalid number"
					r := &outcomeRows{columns: f.columns, values: [][]driver.Value{f.row}}
					switch scenario {
					case "rows partial", "corrupt cache rows partial":
						r.err = errors.New("read interrupted")
					case "rows failed":
						r.values, r.err = nil, errors.New("read interrupted")
					case "scan partial":
						r.values = append(r.values, bad)
					case "scan failed":
						r.values = [][]driver.Value{bad}
					case "corrupt cache query failed":
						return nil, errors.New("permission denied")
					case "corrupt cache empty":
						r.values = nil
					}
					return r, nil
				}})
				t.Cleanup(func() { conn.Close() })
				first, samples := collectTableFixture(t, f, conn)
				if first.CacheFallback != corrupt {
					t.Fatalf("lost cache fallback: %+v", first)
				}
				if scenario == "corrupt cache empty" {
					if first.State != OutcomeEmpty || len(samples) != 0 {
						t.Fatalf("bad cache leaked into empty query: %+v %v", first, samples)
					}
					second, cached := collectTableFixture(t, f, conn)
					if calls != 1 || second.DataOrigin != "cache" || len(cached) != 0 {
						t.Fatal("complete empty result was not cached")
					}
					return
				}
				wantState := OutcomePartial
				if scenario == "rows failed" || scenario == "scan failed" || scenario == "corrupt cache query failed" {
					wantState = OutcomeFailed
				}
				if first.State != wantState {
					t.Fatalf("unexpected first outcome: %+v", first)
				}
				perRow := 3
				if f.name == "datafile" {
					perRow = 2
				}
				wantSamples := perRow
				if wantState == OutcomeFailed {
					wantSamples = 0
				}
				if len(samples) != wantSamples {
					t.Fatalf("incomplete read leaked cached rows: %v", samples)
				}
				cached, found := config.GetFromCache(key)
				if found != corrupt || (found && cached != badCache) {
					t.Fatal("failed read wrote or deleted shared cache")
				}
				second, complete := collectTableFixture(t, f, conn)
				if calls != 2 || second.State != OutcomeSuccess || second.CacheFallback != corrupt || len(complete) != 2*perRow {
					t.Fatalf("recovery did not query complete data: %+v samples=%v calls=%d", second, complete, calls)
				}
				third, reused := collectTableFixture(t, f, conn)
				if calls != 2 || third.DataOrigin != "cache" || !reflect.DeepEqual(complete, reused) {
					t.Fatal("third scrape did not reuse complete cache")
				}
			})
		}
	}
}

func TestFailedTableReadCannotOverwriteConcurrentCompleteCache(t *testing.T) {
	setupCollectionLogTest(t)
	for _, f := range businessFixtures()[:2] {
		t.Run(f.name, func(t *testing.T) {
			resetFixtureCache(t, f)
			key := f.cacheKey + "_fixture"
			config.SetCache(key, `[{"TotalSize":"invalid"}]`, time.Minute)
			entered, release, done := make(chan struct{}), make(chan struct{}), make(chan struct{})
			var once sync.Once
			failed, _ := fixtureDB(t, f, [][]driver.Value{f.row}, nil, errors.New("late read failure"), func() {
				close(entered)
				<-release
			})
			t.Cleanup(func() {
				once.Do(func() { close(release) })
				awaitDone(t, done)
			})
			go func() {
				defer close(done)
				fixtureCollector(f, failed).Collect(make(chan prometheus.Metric, 20))
			}()
			awaitDone(t, entered)
			good, _ := fixtureDB(t, f, [][]driver.Value{f.row}, nil, nil, nil)
			result, samples := collectTableFixture(t, f, good)
			if result.State != OutcomeSuccess || !result.CacheFallback {
				t.Fatalf("complete cache recovery failed: %+v", result)
			}
			before, found := config.GetFromCache(key)
			if !found {
				t.Fatal("complete result was not cached")
			}
			once.Do(func() { close(release) })
			awaitDone(t, done)
			after, found := config.GetFromCache(key)
			if !found || before != after {
				t.Fatal("late failure modified a complete concurrent cache")
			}
			reused, cached := collectTableFixture(t, f, good)
			if reused.DataOrigin != "cache" || !reflect.DeepEqual(samples, cached) {
				t.Fatal("complete concurrent cache was not reusable")
			}
		})
	}
}
