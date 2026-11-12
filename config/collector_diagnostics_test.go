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

func TestCollectorSlowThresholdLifecycle(t *testing.T) {
	for _, tt := range []struct {
		field string
		want  int
	}{{"", 5000}, {"collectorSlowThresholdMs = 0", 0}, {"collectorSlowThresholdMs = 67", 67}} {
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
			if g.GetCollectorSlowThresholdMs() != 5000 {
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
				if cfg.CollectorSlowThresholdMs != tt.want || g.GetCollectorSlowThresholdMs() != tt.want || !strings.Contains(cfg.StringCategorized(), fmt.Sprintf("collectorSlowThresholdMs=%d", tt.want)) {
					t.Fatalf("threshold lost: %+v", cfg)
				}
			}
		})
	}
	cfg := MultiSourceConfig{}
	cfg.ApplyAllDefaults()
	if cfg.CollectorSlowThresholdMs != 0 {
		t.Fatal("direct zero must stay disabled")
	}
}

func TestCollectorSlowThresholdValidation(t *testing.T) {
	maxMS := int64(math.MaxInt64) / int64(time.Millisecond)
	values := []int64{-1, 0, 1, maxMS}
	if strconv.IntSize == 64 {
		values = append(values, maxMS+1)
	}
	for _, value := range values {
		cfg := DefaultMultiSourceConfig
		cfg.CollectorSlowThresholdMs = int(value)
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
