package config

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestConnectionValidation(t *testing.T) {
	for _, tt := range []struct {
		name, user, password, host string
		wantError                  bool
	}{
		{"complex password", "monitor", "test:@/?#&=+%!/", "127.0.0.1:5236", false},
		{"colon username", "monitor:ops", "test-password", "127.0.0.1:5236", true},
		{"128 bytes", "monitor", strings.Repeat("a", 128), "127.0.0.1:5236", false},
		{"129 bytes", "monitor", strings.Repeat("a", 129), "127.0.0.1:5236", true},
		{"unicode requires server encoding", "monitor", strings.Repeat("测", 60), "127.0.0.1:5236", false},
		{"invalid UTF-8", "monitor", string([]byte{0xff}), "127.0.0.1:5236", true},
		{"user override", "monitor", "test-password", "127.0.0.1:5236?USER=other", true},
		{"password override", "monitor", "test-password", "127.0.0.1:5236?PaSsWoRd=other", true},
		{"extra query separator", "monitor", "test-password", "127.0.0.1:5236?schema=a?b", true},
		{"raw parameters", "monitor", "test-password", "[::1]:5236?schema=业务&path=a+b%2Fc", false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			cfg := DataSourceConfig{DbUser: tt.user, DbPwd: tt.password, DbHost: tt.host}
			err := cfg.ValidateConnection()
			if (err != nil) != tt.wantError {
				t.Fatalf("unexpected validation error: %v", err)
			}
			if err != nil && tt.password != "" && strings.Contains(err.Error(), tt.password) {
				t.Fatal("validation error exposes the password")
			}
		})
	}
}

func TestLoadConfigDefersConnectionValidation(t *testing.T) {
	for _, enabled := range []bool{false, true} {
		name := "disabled source"
		if enabled {
			name = "enabled source"
		}
		t.Run(name, func(t *testing.T) {
			cfg := DefaultMultiSourceConfig
			ds := DefaultDataSourceConfig
			ds.Name, ds.DbHost, ds.DbUser, ds.DbPwd = "ordinary", "127.0.0.1:5236", "monitor", "test-password"
			cfg.DataSources = []DataSourceConfig{ds}
			ds.Name, ds.DbHost, ds.Enabled = "secondary", "127.0.0.2:5236", enabled
			ds.DbPwd = strings.Repeat("a", 129)
			cfg.DataSources = append(cfg.DataSources, ds)
			path := filepath.Join(t.TempDir(), "config.toml")
			if err := SaveMultiSourceConfig(&cfg, path); err != nil {
				t.Fatal(err)
			}
			loaded, err := LoadMultiSourceConfig(path)
			if err != nil {
				t.Fatalf("one source's connection constraint blocked loading all sources: %v", err)
			}
			if len(loaded.DataSources) != 2 || loaded.DataSources[1].Enabled != enabled {
				t.Fatal("configuration changed source selection")
			}
		})
	}
}
