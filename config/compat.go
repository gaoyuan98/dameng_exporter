package config

import (
	"bufio"
	"os"
	"strings"
)

// DetectConfigFormat 检测配置文件格式
func DetectConfigFormat(configFile string) string {
	file, err := os.Open(configFile)
	if err != nil {
		return "unknown"
	}
	defer file.Close()

	// 读取文件内容来判断格式
	scanner := bufio.NewScanner(file)
	hasTomlSection := false
	hasEqualSign := false

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		// 检查是否包含TOML section标记
		if strings.HasPrefix(line, "[[") || strings.HasPrefix(line, "[") {
			hasTomlSection = true
		}

		// 检查是否包含等号
		if strings.Contains(line, "=") {
			hasEqualSign = true
		}
	}

	// 如果有TOML section标记，认为是TOML格式
	if hasTomlSection {
		return "toml"
	}

	// 如果只有等号没有section，认为是旧格式
	if hasEqualSign {
		return "legacy"
	}

	return "unknown"
}

// ConvertLegacyToMultiSource 将旧的全局配置转换为多数据源配置
func ConvertLegacyToMultiSource(oldConfig *Config) *MultiSourceConfig {
	if oldConfig == nil {
		oldConfig = &DefaultConfig
	}
	return ConvertLegacyConfig(oldConfig)
}

// ConvertLegacyConfig 将旧配置转换为多数据源配置
func ConvertLegacyConfig(oldConfig *Config) *MultiSourceConfig {
	msc := &MultiSourceConfig{
		// 全局配置映射
		ConfigFile:        oldConfig.ConfigFile,
		ListenAddress:     oldConfig.ListenAddress,
		MetricPath:        oldConfig.MetricPath,
		Version:           oldConfig.Version,
		LogMaxSize:        oldConfig.LogMaxSize,
		LogMaxBackups:     oldConfig.LogMaxBackups,
		LogMaxAge:         oldConfig.LogMaxAge,
		LogLevel:          oldConfig.LogLevel,
		EncodeConfigPwd:   oldConfig.EncodeConfigPwd,
		EnableBasicAuth:   oldConfig.EnableBasicAuth,
		BasicAuthUsername: oldConfig.BasicAuthUsername,
		BasicAuthPassword: oldConfig.BasicAuthPassword,

		// 采集策略（使用默认值）
		CollectStrategy:     StrategyHybrid,
		MaxConcurrentGroups: 3,
		GroupTimeout:        60,

		// 全局超时控制配置
		GlobalTimeoutSeconds: oldConfig.GlobalTimeoutSeconds,
		P99LatencyTarget:     oldConfig.P99LatencyTarget,
		EnablePartialReturn:  oldConfig.EnablePartialReturn,
		LatencyWindowSize:    oldConfig.LatencyWindowSize,
	}

	// 创建单个数据源
	ds := DataSourceConfig{
		Name:        "default",
		Description: "Default datasource (converted from legacy config)",
		Enabled:     true,

		// 数据库连接配置
		DbHost:          oldConfig.DbHost,
		DbUser:          oldConfig.DbUser,
		DbPwd:           oldConfig.DbPwd,
		QueryTimeout:    oldConfig.QueryTimeout,
		MaxOpenConns:    oldConfig.MaxOpenConns,
		MaxIdleConns:    oldConfig.MaxIdleConns,
		ConnMaxLifetime: oldConfig.ConnMaxLifetime,

		// 缓存配置
		BigKeyDataCacheTime: oldConfig.BigKeyDataCacheTime,
		AlarmKeyCacheTime:   oldConfig.AlarmKeyCacheTime,

		// 慢SQL配置
		CheckSlowSQL:   oldConfig.CheckSlowSQL,
		SlowSqlTime:    oldConfig.SlowSqlTime,
		SlowSqlMaxRows: oldConfig.SlowSqlMaxRows,

		// 指标注册配置
		RegisterHostMetrics:     oldConfig.RegisterHostMetrics,
		RegisterDatabaseMetrics: oldConfig.RegisterDatabaseMetrics,
		RegisterDmhsMetrics:     oldConfig.RegisterDmhsMetrics,
		RegisterCustomMetrics:   oldConfig.RegisterCustomMetrics,

		// 其他配置
		Priority:          2,  // 默认中等优先级
		Labels:            "", // 无额外标签
		CustomMetricsFile: oldConfig.CustomMetricsFile,
	}

	// 应用默认值
	ds.ApplyDefaults()

	// 添加数据源到列表
	msc.DataSources = append(msc.DataSources, ds)

	return msc
}

// ConvertMultiToGlobal 将多数据源配置转换为全局配置（用于兼容）
func ConvertMultiToGlobal(msc *MultiSourceConfig) Config {
	// 如果没有数据源，返回默认配置
	if len(msc.DataSources) == 0 {
		return DefaultConfig
	}

	// 使用第一个数据源的配置
	ds := msc.DataSources[0]

	return Config{
		// 全局配置
		ConfigFile:        msc.ConfigFile,
		ListenAddress:     msc.ListenAddress,
		MetricPath:        msc.MetricPath,
		Version:           msc.Version,
		LogMaxSize:        msc.LogMaxSize,
		LogMaxBackups:     msc.LogMaxBackups,
		LogMaxAge:         msc.LogMaxAge,
		LogLevel:          msc.LogLevel,
		EncodeConfigPwd:   msc.EncodeConfigPwd,
		EnableBasicAuth:   msc.EnableBasicAuth,
		BasicAuthUsername: msc.BasicAuthUsername,
		BasicAuthPassword: msc.BasicAuthPassword,

		// 使用第一个数据源的配置
		DbHost:                  ds.DbHost,
		DbUser:                  ds.DbUser,
		DbPwd:                   ds.DbPwd,
		QueryTimeout:            ds.QueryTimeout,
		MaxOpenConns:            ds.MaxOpenConns,
		MaxIdleConns:            ds.MaxIdleConns,
		ConnMaxLifetime:         ds.ConnMaxLifetime,
		BigKeyDataCacheTime:     ds.BigKeyDataCacheTime,
		AlarmKeyCacheTime:       ds.AlarmKeyCacheTime,
		CheckSlowSQL:            ds.CheckSlowSQL,
		SlowSqlTime:             ds.SlowSqlTime,
		SlowSqlMaxRows:          ds.SlowSqlMaxRows,
		RegisterHostMetrics:     ds.RegisterHostMetrics,
		RegisterDatabaseMetrics: ds.RegisterDatabaseMetrics,
		RegisterDmhsMetrics:     ds.RegisterDmhsMetrics,
		RegisterCustomMetrics:   ds.RegisterCustomMetrics,
		CustomMetricsFile:       ds.CustomMetricsFile,

		// 全局超时控制配置
		GlobalTimeoutSeconds: msc.GlobalTimeoutSeconds,
		P99LatencyTarget:     msc.P99LatencyTarget,
		EnablePartialReturn:  msc.EnablePartialReturn,
		LatencyWindowSize:    msc.LatencyWindowSize,
	}
}
