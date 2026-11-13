package diagnostics

import (
	"context"
	"database/sql/driver"
	"testing"
	"time"
)

func TestEventFieldsKeepEvidenceWithoutDefaultSnapshots(t *testing.T) {
	r, u, c, logs := fixture(t, 0)
	db := openDB(t, func(context.Context, string, []driver.NamedValue) (driver.Rows, error) {
		c.advance(500 * time.Microsecond)
		return &driverRows{values: []driver.Value{int64(1)}}, nil
	})
	rows, err := Wrap(db, u).QueryContext(context.Background(), "select value")
	if err != nil {
		t.Fatal(err)
	}
	for rows.Next() {
		var n int
		rows.Scan(&n)
	}
	rows.Err()
	rows.Close()
	u.Finish(1, 0, false)
	r.Finish(nil)
	start := event(t, logs, "sql_started")
	end := event(t, logs, "sql_finished")
	summary := event(t, logs, "scrape_finished")
	for _, key := range []string{"state", "result", "read_state", "rows_read", "sql_duration_ms", "query_sequence", "api_ms"} {
		if _, ok := start[key]; ok {
			t.Fatal("start contains unobserved/redundant field", key)
		}
	}
	if start["sql"] != "select value" || start["budget_remaining_ms_at_start"] != nil {
		t.Fatal(start)
	}
	if _, ok := end["sql"]; ok {
		t.Fatal("completion copied SQL")
	}
	if end["sql_duration_ms"] != int64(0) || end["rows_read"] != int64(1) || end["read_scope"] != "first_result_set" {
		t.Fatal(end)
	}
	api := end["api_ms"].(map[string]int64)
	if _, observed := api["query"]; !observed {
		t.Fatal("submillisecond observation lost")
	}
	if _, observed := api["row_scan"]; observed {
		t.Fatal("invented API observation")
	}
	if len(summary) != 5 || summary["samples"] != int64(1) {
		t.Fatal(summary)
	}
}
func TestTimeoutWarningIsForegroundAndEmittedOnce(t *testing.T) {
	r, u, _, logs := fixture(t, 0)
	u.Detach()
	u.Detach()
	u.Finish(0, 0, false)
	r.Finish(nil)
	events := logs.FilterMessage("collector_timeout").All()
	if len(events) != 1 || events[0].ContextMap()["phase"] != nil || events[0].ContextMap()["foreground_cutoff_at"] == nil {
		t.Fatal(events)
	}
	if event(t, logs, "scrape_finished")["frontend_timeout_runs"] != int64(1) {
		t.Fatal(logs.All())
	}
}
