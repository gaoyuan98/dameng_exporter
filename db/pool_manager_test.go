package db

import (
	"dameng_exporter/config"
	"strings"
	"testing"
)

func TestBuildDSNMarksExporterAndPreservesParameters(t *testing.T) {
	for _, tt := range []struct {
		name    string
		host    string
		timeout int
		want    string
	}{
		{"defaults", "localhost:5236", 12, "localhost:5236?autoCommit=true&socketTimeout=12&connectTimeout=12000&appName=dameng_exporter"},
		{"fallback timeout", "localhost:5236", 0, "localhost:5236?autoCommit=true&socketTimeout=5&connectTimeout=5000&appName=dameng_exporter"},
		{"raw values", "localhost:5236?schema=业务&path=a+b%2Fc&autoCommit=false&socketTimeout=9&connectTimeout=7000", 12, "localhost:5236?schema=业务&path=a+b%2Fc&autoCommit=false&socketTimeout=9&connectTimeout=7000&appName=dameng_exporter"},
		{"duplicate application names", "localhost:5236?APPNAME=old&appName=another&ApPnAmE=&socketTimeout=8", 12, "localhost:5236?socketTimeout=8&appName=dameng_exporter"},
		{"only application name", "localhost:5236?appName=old", 12, "localhost:5236?appName=dameng_exporter"},
		{"empty query", "localhost:5236?", 12, "localhost:5236?appName=dameng_exporter"},
		{"ipv6", "[::1]:5236?autoCommit=true", 12, "[::1]:5236?autoCommit=true&appName=dameng_exporter"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			cfg := config.DataSourceConfig{DbHost: tt.host, DbUser: "monitor", DbPwd: "test-password", QueryTimeout: tt.timeout}
			got := (&DBPoolManager{}).buildDSN(&cfg)
			if want := "dm://monitor:test-password@" + tt.want; got != want {
				t.Fatalf("DSN = %q, want %q", got, want)
			}
			// Match the driver's case-insensitive, raw parameter parsing.
			_, query, _ := strings.Cut(got, "?")
			appNames := 0
			for _, param := range strings.Split(query, "&") {
				key, value, _ := strings.Cut(param, "=")
				if strings.EqualFold(key, "appName") {
					appNames++
					if value != config.ExporterAppName {
						t.Fatalf("connection marker %q differs from session filter %q", value, config.ExporterAppName)
					}
				}
			}
			if appNames != 1 {
				t.Fatalf("expected exactly one appName parameter, got %d", appNames)
			}
		})
	}
}
