package collector

import (
	"context"
	"dameng_exporter/config"
	"dameng_exporter/db"
	"database/sql"
	"database/sql/driver"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/prometheus/client_golang/prometheus"
	"os"
	"reflect"
	"strings"
	"testing"
)

// This fixture is also run against HEAD before the migration. The committed
// baseline checks actual SQL text/order/binds and all produced metric payloads
// across every collector, including compatibility branches. It is synthetic,
// so live DM permissions, cancellation and query plans remain separate checks.
func TestAllCollectorQueryCompatibility(t *testing.T) {
	setupCollectionLogTest(t)
	factories := []func(*sql.DB) MetricCollector{
		NewDbArchQueueCollector, NewDbArchSendCollector, NewDbArchStatusCollector, NewDbArchSwitchCollector, NewAsmGroupInfoCollector, NewDbBufferPoolCollector, NewCkptCollector, NewDbDictCacheCollector, NewDbDualCollector, NewDbDwWatcherInfoCollector, NewDbInstanceLogErrorCollector, NewDBInstanceRunningInfoCollector, NewDbJobRunningInfoCollector, NewDbLicenseCollector, NewDbLogHistoryCollector, NewDbMemoryPoolInfoCollector, NewMonitorInfoCollector, NewIniParameterCollector, NewPurgeCollector, NewDbRapplySysCollector, NewDbRapplyTimeDiffCollector, NewDbRlogFileCollector, NewDbRedoLogLsnCollector, NewDBSessionsStatusCollector, NewSlowSessionInfoCollector, NewDbSqlExecTypeCollector, NewDbSystemEventWaitCollector, NewDBSystemInfoCollector, NewTableSpaceDateFileInfoCollector, NewTableSpaceInfoCollector, NewDbUserCollector, NewDbVersionCollector, func(d *sql.DB) MetricCollector {
			c := NewDmapProcessCollector(d)
			c.lastPID = "42"
			c.localInstallBinPath = "/dm/bin"
			c.processOutput = func(string) ([]byte, error) { return []byte("1"), nil }
			return c
		}, func(d *sql.DB) MetricCollector {
			return NewCustomMetrics(d, config.CustomConfig{Metrics: []config.CustomMetric{{Context: "coverage", Request: "select coverage", MetricsDesc: map[string]string{"0": "fixture"}}}})
		},
	}
	type record struct {
		Queries []string
		Samples []string
	}
	records := map[string]record{}
	for i, factory := range factories {
		for _, scenario := range []string{"modern", "legacy", "dsc", "empty", "query_error", "probe_error", "partial_fields", "missing_columns"} {
			t.Run(fmt.Sprintf("%d/%s", i, scenario), func(t *testing.T) {
				var queries []string
				conn := sql.OpenDB(sessionTestConnector{query: func(_ context.Context, q string, args []driver.NamedValue) (driver.Rows, error) {
					encoded, _ := json.Marshal(args)
					queries = append(queries, q+" args="+string(encoded))
					if scenario == "query_error" {
						return nil, errors.New("permission denied")
					}
					upper := strings.ToUpper(q)
					row := func(v ...driver.Value) (driver.Rows, error) {
						return &outcomeRows{columns: len(v), values: [][]driver.Value{v}}, nil
					}
					if strings.Contains(upper, "COUNT") && strings.Contains(upper, "V$DYNAMIC") {
						if scenario == "probe_error" {
							return nil, errors.New("permission denied")
						}
						count := int64(1)
						if strings.Contains(upper, "('EVENT','TOTAL_WAITS')") || strings.Contains(upper, "('LAST_SEND_CODE','LAST_SEND_DESC')") {
							count = 2
						}
						if strings.Contains(upper, "'CKPT_LSN','FILE_LSN'") {
							count = 4
						}
						if scenario == "empty" {
							count = 0
						}
						if scenario == "missing_columns" && strings.Contains(upper, "V$DYNAMIC_TABLE_COLUMNS") {
							count = 0
						}
						if scenario == "partial_fields" && strings.Contains(upper, "V$DB_CACHE") && !strings.Contains(upper, "LRU_DISCARD") {
							count = 0
						}

						if scenario == "legacy" && (strings.Contains(upper, "V$ARCH_APPLY_INFO") || strings.Contains(upper, "LAST_SEND_CODE")) {
							count = 0
						}
						return row(count)
					}
					if strings.Contains(upper, "CONFIG_PATH") {
						return row("/dm/dm.ini", "42")
					}
					if strings.Contains(upper, "PARA_NAME='ARCH_INI'") {
						return row("1")
					}
					if strings.Contains(upper, "CASE ARCH_STATUS") {
						if strings.Contains(upper, "THEN 'VALID'") {
							return row("VALID")
						}
						return row("1")
					}
					if q == config.QueryVersionInfoSqlStr {
						if scenario == "legacy" || scenario == "dsc" {
							return nil, errors.New("missing version column")
						}
						return row("8", "release", "1")
					}
					if strings.Contains(upper, "TABLEDEF") {
						if scenario == "legacy" {
							return row(int64(0))
						}
						return row(int64(1))
					}
					if strings.Contains(upper, "BUILD_VERSION VERSION") || strings.Contains(upper, "TOP 1 BANNER") {
						return row("DM Database Server 8")
					}
					return &outcomeRows{columns: 1}, nil
				}})
				defer conn.Close()
				source := fmt.Sprintf("coverage_%d_%s", i, scenario)
				c := factory(conn)
				SetDataSourceIfSupported(c, source)
				ds := config.DefaultDataSourceConfig
				ds.CheckSlowSQL = true
				ds.QueryTimeout = 30
				SetDataSourceConfigIfSupported(c, &ds)
				if scenario == "dsc" {
					SetNodeTypeIfSupported(c, string(db.NodeTypeDSC))
				}
				out := make(chan prometheus.Metric, 100)
				c.Collect(out)
				records[source] = record{queries, metricSnapshot(t, out)}
			})
		}
	}
	data, err := json.MarshalIndent(records, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if path := os.Getenv("QUERY_COMPATIBILITY_BASELINE_OUTPUT"); path != "" {
		if err := os.WriteFile(path, data, 0600); err != nil {
			t.Fatal(err)
		}
		return
	}
	raw, err := os.ReadFile("testdata/collection_query_baseline.json")
	if err != nil {
		t.Fatal(err)
	}
	var want map[string]record
	if err := json.Unmarshal(raw, &want); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(records, want) {
		t.Fatal("collector SQL, binds, order or metrics differ from pre-migration fixture")
	}
}
