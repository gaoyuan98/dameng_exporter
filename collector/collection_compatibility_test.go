package collector

import (
	"context"
	"dameng_exporter/config"
	"database/sql"
	"database/sql/driver"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"os"
	"reflect"
	"sort"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"
	"google.golang.org/protobuf/encoding/protojson"
)

// Each query shape has its own column count; malformed values exercise real Scan.
type outcomeRows struct {
	columns int
	values  [][]driver.Value
	index   int
	err     error
}

func (r *outcomeRows) Columns() []string {
	names := make([]string, r.columns)
	for i := range names {
		names[i] = fmt.Sprint(i)
	}
	return names
}
func (*outcomeRows) Close() error { return nil }
func (r *outcomeRows) Next(dest []driver.Value) error {
	if r.index == len(r.values) {
		if r.err != nil {
			return r.err
		}
		return io.EOF
	}
	copy(dest, r.values[r.index])
	r.index++
	return nil
}

type businessFixture struct {
	name     string
	factory  func(*sql.DB) MetricCollector
	query    string
	columns  int
	row      []driver.Value
	cacheKey string
}

func businessFixtures() []businessFixture {
	return []businessFixture{
		{"tablespace", NewTableSpaceInfoCollector, config.QueryTablespaceInfoSqlStr, 8, []driver.Value{"MAIN", int64(1), float64(100), float64(25), float64(75), float64(10), float64(25), float64(75)}, dmdbms_tablespace_size_total_info},
		{"datafile", NewTableSpaceDateFileInfoCollector, config.QueryTablespaceFileSqlStr, 6, []driver.Value{"/dm/main.dbf", float64(100), float64(25), "ON", "10", "1000"}, dmdbms_tablespace_file_total_info},
		{"sessions", NewDBSessionsStatusCollector, config.QueryDBSessionsStatusSqlStr, 2, []driver.Value{"ACTIVE", int64(3)}, ""},
		{"system", NewDBSystemInfoCollector, config.QuerySystemInfoSqlStr, 4, []driver.Value{float64(8), float64(4096), float64(8192), float64(10000)}, ""},
	}
}
func fixtureDB(t *testing.T, f businessFixture, rows [][]driver.Value, queryErr, readErr error, wait func()) (*sql.DB, *int) {
	t.Helper()
	calls := new(int)
	conn := sql.OpenDB(sessionTestConnector{query: func(_ context.Context, query string, args []driver.NamedValue) (driver.Rows, error) {
		*calls++
		if query != f.query {
			t.Errorf("extra or changed SQL: %s", query)
		}
		if f.name == "sessions" {
			if len(args) != 1 || args[0].Value != config.ExporterAppName {
				t.Errorf("changed bind values: %v", args)
			}
		} else if len(args) != 0 {
			t.Errorf("extra bind values: %v", args)
		}
		if wait != nil {
			wait()
		}
		if queryErr != nil {
			return nil, queryErr
		}
		return &outcomeRows{columns: f.columns, values: rows, err: readErr}, nil
	}})
	t.Cleanup(func() { conn.Close() })
	return conn, calls
}
func fixtureCollector(f businessFixture, conn *sql.DB) MetricCollector {
	c := f.factory(conn)
	SetDataSourceIfSupported(c, "fixture")
	return c
}
func resetFixtureCache(t *testing.T, f businessFixture) {
	t.Helper()
	if f.cacheKey != "" {
		key := f.cacheKey + "_fixture"
		config.DeleteFromCache(key)
		t.Cleanup(func() { config.DeleteFromCache(key) })
	}
}
func metricSnapshot(t *testing.T, ch chan prometheus.Metric) []string {
	t.Helper()
	result := make([]string, 0, len(ch))
	close(ch)
	for m := range ch {
		value := &dto.Metric{}
		if err := m.Write(value); err != nil {
			t.Fatal(err)
		}
		encoded, err := protojson.Marshal(value)
		if err != nil {
			t.Fatal(err)
		}
		var canonical any
		if err = json.Unmarshal(encoded, &canonical); err != nil {
			t.Fatal(err)
		}
		encoded, _ = json.Marshal(canonical)
		result = append(result, m.Desc().String()+" "+string(encoded))
	}
	sort.Strings(result)
	return result
}

// The committed sample set is captured from the pre-change source snapshot.
// It compares names, types, labels, values, SQL count and all business samples.
func TestBusinessCollectionCompatibility(t *testing.T) {
	setupCollectionLogTest(t)
	type snapshot struct {
		Samples []string
		Queries int
	}
	snapshots := map[string]snapshot{}
	for _, f := range businessFixtures() {
		scenarios := []string{"normal", "slow", "error", "timeout", "empty", "scan", "rows", "scan failed", "rows failed"}
		if f.cacheKey != "" {
			scenarios = append(scenarios, "cache", "empty cache", "cache recovery", "marshal")
		}
		if f.name == "system" {
			scenarios = append(scenarios, "null")
		}
		for _, scenario := range scenarios {
			resetFixtureCache(t, f)
			rows := [][]driver.Value{append([]driver.Value(nil), f.row...)}
			var queryErr, readErr error
			var wait func()
			switch scenario {
			case "slow":
				wait = func() { time.Sleep(25 * time.Millisecond) }
			case "error":
				queryErr = errors.New("permission denied")
			case "timeout":
				queryErr = context.DeadlineExceeded
			case "cache":
				config.SetCache(f.cacheKey+"_fixture", `[{"TotalSize":100,"FreeSize":25}]`, time.Minute)
			case "empty cache":
				config.SetCache(f.cacheKey+"_fixture", `[]`, time.Minute)
			case "cache recovery":
				config.SetCache(f.cacheKey+"_fixture", `[{"TotalSize":100},{"TotalSize":"invalid"}]`, time.Minute)
			case "marshal":
				rows[0][2] = math.NaN()
			case "null":
				rows = [][]driver.Value{{nil, nil, nil, nil}}
			case "scan failed":
				rows[0][1] = "invalid number"
			case "rows failed":
				rows = nil
				readErr = errors.New("read interrupted")
			case "empty":
				rows = nil
			case "scan":
				bad := append([]driver.Value(nil), f.row...)
				bad[1] = "invalid number"
				rows = append(rows, bad)
			case "rows":
				readErr = errors.New("read interrupted")
			}
			conn, calls := fixtureDB(t, f, rows, queryErr, readErr, wait)
			ch := make(chan prometheus.Metric, 100)
			fixtureCollector(f, conn).Collect(ch)
			snapshots[f.name+"/"+scenario] = snapshot{metricSnapshot(t, ch), *calls}
		}
	}
	data, err := json.MarshalIndent(snapshots, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if path := os.Getenv("COLLECTION_BASELINE_OUTPUT"); path != "" {
		if err := os.WriteFile(path, data, 0600); err != nil {
			t.Fatal(err)
		}
		return
	}
	baseline, err := os.ReadFile("testdata/collection_business_baseline.json")
	if err != nil {
		t.Fatal(err)
	}
	var want map[string]snapshot
	if err := json.Unmarshal(baseline, &want); err != nil {
		t.Fatal(err)
	}
	// These two repaired paths must now match the historical fresh-query samples,
	// without the partially decoded cache rows. Keep the original JSON as evidence.
	want["tablespace/cache recovery"] = want["tablespace/normal"]
	want["datafile/cache recovery"] = want["datafile/normal"]
	if !reflect.DeepEqual(snapshots, want) {
		t.Fatalf("business samples or query counts changed\ngot %s\nwant %s", data, baseline)
	}
}
