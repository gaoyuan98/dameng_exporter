package collector

import (
	"context"
	"dameng_exporter/config"
	"dameng_exporter/db"
	"dameng_exporter/logger"
	"database/sql"
	"database/sql/driver"
	"errors"
	"io"
	"reflect"
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	"go.uber.org/zap"
)

type sessionTestConnector struct {
	query func(context.Context, string, []driver.NamedValue) (driver.Rows, error)
}

func (c sessionTestConnector) Connect(context.Context) (driver.Conn, error) {
	return sessionTestConn{c}, nil
}
func (sessionTestConnector) Driver() driver.Driver { return sessionTestDriver{} }

type sessionTestDriver struct{}

func (sessionTestDriver) Open(string) (driver.Conn, error) {
	return nil, errors.New("use connector")
}

type sessionTestConn struct{ sessionTestConnector }

func (sessionTestConn) Prepare(string) (driver.Stmt, error) {
	return nil, errors.New("unexpected prepare")
}
func (sessionTestConn) Close() error { return nil }
func (sessionTestConn) Begin() (driver.Tx, error) {
	return nil, errors.New("unexpected transaction")
}
func (c sessionTestConn) QueryContext(ctx context.Context, query string, args []driver.NamedValue) (driver.Rows, error) {
	return c.query(ctx, query, args)
}

type sessionTestRows struct {
	values [][]driver.Value
	index  int
	err    error
}

func (sessionTestRows) Columns() []string { return []string{"STATE_TYPE", "COUNT_VAL"} }
func (sessionTestRows) Close() error      { return nil }
func (r *sessionTestRows) Next(dest []driver.Value) error {
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

func TestDBSessionsStatusCollectorFilteredResults(t *testing.T) {
	previousConfig, previousLogger, previousManager := config.Global.GetConfig(), logger.Logger, db.GlobalPoolManager
	config.Global.Init(nil)
	logger.Logger = zap.NewNop().Sugar()
	db.GlobalPoolManager = nil
	t.Cleanup(func() {
		config.Global.Init(previousConfig)
		logger.Logger = previousLogger
		db.GlobalPoolManager = previousManager
	})

	for _, tt := range []struct {
		name     string
		rows     [][]driver.Value
		queryErr error
		readErr  error
		want     map[string]float64
	}{
		{"all states", [][]driver.Value{{"ACTIVE", int64(3)}, {"IDLE", int64(20)}, {"UNKNOWN", int64(2)}, {"PENDING", int64(1)}}, nil, nil, map[string]float64{"ACTIVE": 3, "IDLE": 20, "UNKNOWN": 2, "PENDING": 1}},
		{"no active sessions", [][]driver.Value{{"IDLE", int64(20)}}, nil, nil, map[string]float64{"ACTIVE": 0, "IDLE": 20}},
		{"no idle sessions", [][]driver.Value{{"ACTIVE", int64(3)}}, nil, nil, map[string]float64{"ACTIVE": 3, "IDLE": 0}},
		{"only exporter sessions remain", nil, nil, nil, map[string]float64{"ACTIVE": 0, "IDLE": 0}},
		{"query failed", nil, errors.New("query failed"), nil, map[string]float64{}},
		{"partial read failed", [][]driver.Value{{"ACTIVE", int64(3)}}, nil, errors.New("read failed"), map[string]float64{}},
		{"partial scan failed", [][]driver.Value{{"ACTIVE", int64(3)}, {"IDLE", "invalid count"}}, nil, nil, map[string]float64{}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			conn := sql.OpenDB(sessionTestConnector{query: func(_ context.Context, query string, args []driver.NamedValue) (driver.Rows, error) {
				if query != config.QueryDBSessionsStatusSqlStr {
					t.Errorf("unexpected session query: %s", query)
				}
				if len(args) != 1 || args[0].Value != config.ExporterAppName {
					t.Errorf("query must filter the same application name used for connections, got %v", args)
				}
				if tt.queryErr != nil {
					return nil, tt.queryErr
				}
				return &sessionTestRows{values: tt.rows, err: tt.readErr}, nil
			}})
			t.Cleanup(func() { conn.Close() })
			reg := prometheus.NewRegistry()
			reg.MustRegister(NewDBSessionsStatusCollector(conn))
			families, err := reg.Gather()
			if err != nil {
				t.Fatal(err)
			}
			got := make(map[string]float64)
			for _, family := range families {
				if family.GetName() != "dmdbms_session_type_info" {
					t.Fatalf("unexpected new metric: %s", family.GetName())
				}
				for _, metric := range family.Metric {
					if len(metric.Label) != 1 || metric.Label[0].GetName() != "session_type" {
						t.Fatalf("unexpected labels: %v", metric.Label)
					}
					got[metric.Label[0].GetValue()] = metric.GetGauge().GetValue()
				}
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("session values = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestDBSessionsStatusOutcomes(t *testing.T) {
	setupCollectionLogTest(t)
	f := businessFixtures()[2]
	for _, name := range []string{"normal", "empty", "query", "timeout", "scan", "rows"} {
		t.Run(name, func(t *testing.T) {
			rows := [][]driver.Value{f.row}
			var queryErr, readErr error
			wantIssue := false
			wantSamples := 2
			switch name {
			case "empty":
				rows = nil
			case "query":
				queryErr = errors.New("denied")
				wantIssue = true
				wantSamples = 0
			case "timeout":
				queryErr = context.DeadlineExceeded
				wantIssue = true
				wantSamples = 0
			case "scan":
				rows = append(rows, []driver.Value{"IDLE", "bad"})
				wantIssue = true
				wantSamples = 0
			case "rows":
				readErr = errors.New("interrupted")
				wantIssue = true
				wantSamples = 0
			}
			conn, calls := fixtureDB(t, f, rows, queryErr, readErr, nil)
			facts, samples := collectTableFixture(t, f, conn)
			if facts.Issue != wantIssue || len(samples) != wantSamples || *calls != 1 {
				t.Fatalf("facts=%+v samples=%d calls=%d", facts, len(samples), *calls)
			}
		})
	}
}
