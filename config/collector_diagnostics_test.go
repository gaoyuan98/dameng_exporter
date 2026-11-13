package config

import (
	"fmt"
	"math"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestScrapeSlowThresholdLifecycle(t *testing.T) {
	for _, tt := range []struct {
		field string
		want  int
	}{{"", 30000}, {"scrapeSlowThresholdMs = 0", 0}, {"scrapeSlowThresholdMs = 67", 67}} {
		t.Run(fmt.Sprint(tt.want), func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "exporter.toml")
			content := tt.field + `\n[[datasource]]
name = "test"
dbHost = "127.0.0.1:5236"
dbUser = "monitor"
dbPwd = "test-password"
`
			content = strings.ReplaceAll(content, `\n`, "\n")
			if err := os.WriteFile(path, []byte(content), 0600); err != nil {
				t.Fatal(err)
			}
			cfg, err := LoadMultiSourceConfig(path)
			if err != nil {
				t.Fatal(err)
			}
			g := &GlobalSettings{}
			if g.GetScrapeSlowThresholdMs() != 30000 {
				t.Fatal("missing global default")
			}
			for _, commandLine := range []bool{false, true} {
				args := &CmdArgs{}
				if commandLine {
					args = passwordTestCmdArgs()
					*args.DbHost = "127.0.0.1:5236"
					*args.DbUser = "monitor"
					*args.DbPwd = "test-password"
				}
				if err := MergeMultiSourceConfigFromCmdArgs(cfg, args); err != nil {
					t.Fatal(err)
				}
				cfg.EncodeConfigPwd = true
				if err := SaveMultiSourceConfig(cfg, path); err != nil {
					t.Fatal(err)
				}
				cfg, err = CheckAndEncryptConfigPasswords(cfg, path)
				if err != nil {
					t.Fatal(err)
				}
				cfg, err = LoadMultiSourceConfig(path)
				if err != nil {
					t.Fatal(err)
				}
				cfg.ApplyAllDefaults()
				g.Init(cfg)
				if cfg.ScrapeSlowThresholdMs != tt.want || g.GetScrapeSlowThresholdMs() != tt.want || !strings.Contains(cfg.StringCategorized(), fmt.Sprintf("scrapeSlowThresholdMs=%d", tt.want)) {
					t.Fatalf("threshold lost: %+v", cfg)
				}
			}
		})
	}
	cfg := MultiSourceConfig{}
	cfg.ApplyAllDefaults()
	if cfg.ScrapeSlowThresholdMs != 0 {
		t.Fatal("direct zero must stay disabled")
	}
}

func TestScrapeSlowThresholdValidation(t *testing.T) {
	maxMS := int64(math.MaxInt64) / int64(time.Millisecond)
	values := []int64{-1, 0, 1, maxMS}
	if strconv.IntSize == 64 {
		values = append(values, maxMS+1)
	}
	for _, value := range values {
		cfg := DefaultMultiSourceConfig
		cfg.ScrapeSlowThresholdMs = int(value)
		ds := DefaultDataSourceConfig
		ds.Name, ds.DbHost, ds.DbUser, ds.DbPwd = "test", "127.0.0.1:5236", "monitor", "test-password"
		cfg.DataSources = []DataSourceConfig{ds}
		err := cfg.ValidateAll()
		if (err != nil) != (value < 0 || value > maxMS) {
			t.Fatalf("value %d: %v", value, err)
		}
		path := filepath.Join(t.TempDir(), "config.toml")
		if err := SaveMultiSourceConfig(&cfg, path); err != nil {
			t.Fatal(err)
		}
		_, err = LoadMultiSourceConfig(path)
		if (err != nil) != (value < 0 || value > maxMS) {
			t.Fatalf("loaded value %d: %v", value, err)
		}
	}
}

func TestRemovedDiagnosticSettingRejected(t *testing.T) {
	for _, content := range []string{"collectorSlowThresholdMs=0\n", "[[datasource]]\ncollectorSlowThresholdMs=1\n"} {
		path := filepath.Join(t.TempDir(), "old.toml")
		if err := os.WriteFile(path, []byte(content), 0600); err != nil {
			t.Fatal(err)
		}
		_, err := LoadMultiSourceConfig(path)
		if err == nil || !strings.Contains(err.Error(), "已移除") || !strings.Contains(err.Error(), "scrapeSlowThresholdMs") {
			t.Fatalf("old setting silently ignored: %v", err)
		}
	}
}
