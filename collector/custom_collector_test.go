package collector

import (
	"context"
	"dameng_exporter/config"
	"dameng_exporter/db"
	"database/sql"
	"database/sql/driver"
	"errors"
	"strings"
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	"go.uber.org/zap"
)

func TestDynamicQueryRequiresCompleteRead(t *testing.T) {
	for _, scenario := range []struct {
		name     string
		values   [][]driver.Value
		queryErr error
		readErr  error
	}{
		{name: "complete", values: [][]driver.Value{{float64(7)}}},
		{name: "empty"},
		{name: "query error", queryErr: errors.New("permission denied")},
		{name: "before first row", readErr: errors.New("read interrupted")},
		{name: "after first row", values: [][]driver.Value{{float64(7)}}, readErr: errors.New("read interrupted")},
		{name: "deadline", values: [][]driver.Value{{float64(7)}}, readErr: context.DeadlineExceeded},
		{name: "canceled", values: [][]driver.Value{{float64(7)}}, readErr: context.Canceled},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			conn := sql.OpenDB(sessionTestConnector{query: func(context.Context, string, []driver.NamedValue) (driver.Rows, error) {
				if scenario.queryErr != nil {
					return nil, scenario.queryErr
				}
				return &outcomeRows{columns: 1, values: scenario.values, err: scenario.readErr}, nil
			}})
			t.Cleanup(func() { conn.Close() })
			rows, err := queryDynamicDatabase(context.Background(), conn, "select value")
			cause := scenario.queryErr
			if cause == nil {
				cause = scenario.readErr
			}
			if cause != nil {
				if !errors.Is(err, cause) || rows != nil {
					t.Fatalf("returned partial data or lost error: %v %v", rows, err)
				}
			} else if err != nil || len(rows) != len(scenario.values) || (len(rows) > 0 && rows[0]["0"] != float64(7)) {
				t.Fatalf("changed complete result: %v %v", rows, err)
			}
		})
	}
}

func TestCustomReadFailureKeepsOtherQueriesAndLogContext(t *testing.T) {
	logs := setupCollectionLogTest(t)
	var queries []string
	conn := sql.OpenDB(sessionTestConnector{query: func(_ context.Context, query string, _ []driver.NamedValue) (driver.Rows, error) {
		queries = append(queries, query)
		r := &outcomeRows{columns: 1, values: [][]driver.Value{{float64(7)}}}
		if query == "failed" {
			r.err = errors.New("read interrupted")
		}
		return r, nil
	}})
	t.Cleanup(func() { conn.Close() })
	cfg := &config.CustomConfig{}
	for _, name := range []string{"before", "failed", "after"} {
		cfg.Metrics = append(cfg.Metrics, config.CustomMetric{Context: name, Request: name,
			MetricsDesc: map[string]string{"0": "test " + name}, MetricsType: map[string]string{"0": "gauge"}})
	}
	out := make(chan prometheus.Metric, 10)
	round := newCollectionRound()
	NewCustomMetricsMultiSourceAdapter(nil).collectPool(out, &db.DataSourcePool{Name: "custom", DB: conn}, cfg, round)
	if len(queries) != 3 || queries[0] != "before" || queries[1] != "failed" || queries[2] != "after" || len(out) != 2 {
		t.Fatalf("failed query leaked data or interrupted other queries: %v samples=%d", queries, len(out))
	}
	for _, sample := range metricSnapshot(t, out) {
		if !strings.Contains(sample, "dmdbms_before_0") && !strings.Contains(sample, "dmdbms_after_0") {
			t.Fatalf("unexpected sample: %s", sample)
		}
	}
	errors := logs.FilterLevelExact(zap.ErrorLevel).All()
	if len(errors) != 1 {
		t.Fatalf("expected one read error, got %v", errors)
	}
	fields := errors[0].ContextMap()
	if fields["scrape_id"] != logs.FilterMessage("scrape_started").All()[0].ContextMap()["scrape_id"] || fields["datasource"] != "custom" || fields["collector"] != "CustomMetrics" {
		t.Fatalf("uncorrelated read error: %v", fields)
	}
	if len(round.results) != 1 || round.results[0].Outcome != OutcomeUnknown || round.results[0].Errors != 1 || round.results[0].Samples != 2 {
		t.Fatalf("wrong diagnostics: %+v", round.results)
	}
}
