package main

import (
	"dameng_exporter/config"
	"dameng_exporter/logger"
	"fmt"
	"strings"
	"testing"

	"go.uber.org/zap"
	"go.uber.org/zap/zaptest/observer"
)

func TestStartupConfigurationExcludesCredentials(t *testing.T) {
	old := logger.Logger
	t.Cleanup(func() { logger.Logger = old })
	core, logs := observer.New(zap.DebugLevel)
	logger.Logger = zap.New(core).Sugar()
	cfg := config.DefaultMultiSourceConfig
	cfg.BasicAuthPassword = "private-basic-auth-secret"
	cfg.DataSources = []config.DataSourceConfig{{Name: "test", Enabled: true, DbPwd: "private-db-secret"}}
	logStartupConfig(&cfg)
	if logs.FilterMessage("config_loaded").Len() != 1 || logs.FilterMessage("config_loaded").All()[0].ContextMap()["enabled_datasources"] != int64(1) {
		t.Fatal(logs.All())
	}
	for _, e := range logs.All() {
		text := fmt.Sprint(e.ContextMap())
		if strings.Contains(text, cfg.BasicAuthPassword) || strings.Contains(text, cfg.DataSources[0].DbPwd) {
			t.Fatal("startup logs contain credentials")
		}
	}
}
