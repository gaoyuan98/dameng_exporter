package collector

import (
	"context"
	"dameng_exporter/config"
	"dameng_exporter/db"
	"database/sql"
	"database/sql/driver"
	"errors"
	dm "github.com/gaoyuan98/dm"
	"github.com/prometheus/client_golang/prometheus"
	"strings"
	"testing"
)

func TestVersionProbeOnlyDowngradesDeclaredDriverErrors(t *testing.T) {
	for _, expected := range []bool{true, false} {
		t.Run(map[bool]string{true: "missing_column", false: "permission"}[expected], func(t *testing.T) {
			logs := setupCollectionLogTest(t)
			conn := sql.OpenDB(sessionTestConnector{query: func(_ context.Context, q string, _ []driver.NamedValue) (driver.Rows, error) {
				if q == config.QueryVersionInfoSqlStr {
					if expected {
						return nil, &dm.DmError{ErrText: "无效的列名"}
					}
					return nil, errors.New("permission denied")
				}
				value := driver.Value("version")
				if strings.Contains(q, "TABLEDEF") {
					value = int64(1)
				}
				return &outcomeRows{columns: 1, values: [][]driver.Value{{value}}}, nil
			}})
			defer conn.Close()
			round := newCollectionRound()
			c := NewDbVersionCollector(conn)
			SetDataSourceIfSupported(c, "probe_policy")
			config.DeleteFromCache("db_version_info_probe_policy")
			defer config.DeleteFromCache("db_version_info_probe_policy")
			collectWithLogging(make(chan prometheus.Metric, 2), &db.DataSourcePool{Name: "probe_policy", DB: conn}, c, round, false, 0)
			round.finish(nil)
			want := 1
			if expected {
				want = 0
			}
			if logs.FilterMessage("sql_error").Len() != want {
				t.Fatal(logs.All())
			}
			if expected && logs.FilterMessage("sql_probe_expected").Len() != 1 {
				t.Fatal("expected compatibility failure not DEBUG")
			}
		})
	}
}
