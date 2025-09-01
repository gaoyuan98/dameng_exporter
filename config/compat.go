package config

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
