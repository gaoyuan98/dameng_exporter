package collector

import (
	"context"
	"dameng_exporter/config"
	"dameng_exporter/db"
	"database/sql"
	"database/sql/driver"
	"github.com/prometheus/client_golang/prometheus"
	"testing"
)

func TestBudgetsReflectExistingGlobalAndInjectedContextPaths(t *testing.T) {
	logs := setupCollectionLogTest(t)
	cfg := config.DefaultMultiSourceConfig
	first, second := config.DefaultDataSourceConfig, config.DefaultDataSourceConfig
	first.Name, first.QueryTimeout = "first", 15
	second.Name, second.QueryTimeout, second.CheckSlowSQL = "second", 7, true
	cfg.DataSources = []config.DataSourceConfig{first, second}
	config.Global.Init(&cfg)
	round := newCollectionRound()
	for _, slow := range []bool{false, true} {
		conn := sql.OpenDB(sessionTestConnector{query: func(context.Context, string, []driver.NamedValue) (driver.Rows, error) {
			return &outcomeRows{columns: 1}, nil
		}})
		defer conn.Close()
		var c MetricCollector
		if slow {
			c = NewSlowSessionInfoCollector(conn)
		} else {
			c = NewDBSessionsStatusCollector(conn)
		}
		SetDataSourceIfSupported(c, "second")
		SetDataSourceConfigIfSupported(c, &second)
		collectWithLogging(make(chan prometheus.Metric, 10), &db.DataSourcePool{Name: "second", DB: conn, Config: &second}, c, round, false, 0)
	}
	round.finish(nil)
	events := logs.FilterMessage("sql_started").All()
	if len(events) != 2 {
		t.Fatal(events)
	}
	for i, e := range events {
		budget := e.ContextMap()["budget_remaining_ms_at_start"].(int64)
		want := int64(15000)
		if i == 1 {
			want = 7000
		}
		if budget > want || budget < want-1000 {
			t.Fatalf("actual context budget %d, expected %d; datasource config must not be substituted", budget, want)
		}
	}
}
