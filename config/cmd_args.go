package config

// CmdArgs 命令行参数结构体
type CmdArgs struct {
	ConfigFile              *string
	ListenAddr              *string
	MetricPath              *string
	DbHost                  *string
	DbUser                  *string
	DbPwd                   *string
	QueryTimeout            *int
	MaxOpenConns            *int
	MaxIdleConns            *int
	ConnMaxLife             *int
	CheckSlowSQL            *bool
	SlowSqlTime             *int
	SlowSqlMaxRows          *int
	RegisterHostMetrics     *bool
	RegisterDatabaseMetrics *bool
	RegisterDmhsMetrics     *bool
	RegisterCustomMetrics   *bool
	BigKeyDataCacheTime     *int
	AlarmKeyCacheTime       *int
	LogMaxSize              *int
	LogMaxBackups           *int
	LogMaxAge               *int
	LogLevel                *string
	EncryptPwd              *string
	EncodeConfigPwd         *bool
	EnableBasicAuth         *bool
	BasicAuthUsername       *string
	BasicAuthPassword       *string
	EncryptBasicAuthPwd     *string

	// 全局超时控制参数
	GlobalTimeoutSeconds *int
	P99LatencyTarget     *float64
	EnablePartialReturn  *bool
	LatencyWindowSize    *int
}

// MergeConfig 合并命令行参数到配置
func MergeConfig(config *Config, args *CmdArgs) {
	// 使用辅助函数简化重复的条件判断
	applyStringFlag := func(flagValue *string, defaultValue string, configField *string) {
		if flagValue != nil && *flagValue != defaultValue {
			*configField = *flagValue
		}
	}

	applyIntFlag := func(flagValue *int, defaultValue int, configField *int) {
		if flagValue != nil && *flagValue != defaultValue {
			*configField = *flagValue
		}
	}

	applyBoolFlag := func(flagValue *bool, defaultValue bool, configField *bool) {
		if flagValue != nil && *flagValue != defaultValue {
			*configField = *flagValue
		}
	}

	applyFloat64Flag := func(flagValue *float64, defaultValue float64, configField *float64) {
		if flagValue != nil && *flagValue != defaultValue {
			*configField = *flagValue
		}
	}

	// 应用字符串类型的配置
	applyStringFlag(args.ListenAddr, DefaultConfig.ListenAddress, &config.ListenAddress)
	applyStringFlag(args.MetricPath, DefaultConfig.MetricPath, &config.MetricPath)
	applyStringFlag(args.DbUser, DefaultConfig.DbUser, &config.DbUser)
	applyStringFlag(args.DbPwd, DefaultConfig.DbPwd, &config.DbPwd)
	applyStringFlag(args.DbHost, DefaultConfig.DbHost, &config.DbHost)
	applyStringFlag(args.LogLevel, DefaultConfig.LogLevel, &config.LogLevel)
	applyStringFlag(args.BasicAuthUsername, DefaultConfig.BasicAuthUsername, &config.BasicAuthUsername)
	applyStringFlag(args.BasicAuthPassword, DefaultConfig.BasicAuthPassword, &config.BasicAuthPassword)

	// 应用整数类型的配置
	applyIntFlag(args.QueryTimeout, DefaultConfig.QueryTimeout, &config.QueryTimeout)
	applyIntFlag(args.MaxIdleConns, DefaultConfig.MaxIdleConns, &config.MaxIdleConns)
	applyIntFlag(args.MaxOpenConns, DefaultConfig.MaxOpenConns, &config.MaxOpenConns)
	applyIntFlag(args.ConnMaxLife, DefaultConfig.ConnMaxLifetime, &config.ConnMaxLifetime)
	applyIntFlag(args.LogMaxSize, DefaultConfig.LogMaxSize, &config.LogMaxSize)
	applyIntFlag(args.LogMaxBackups, DefaultConfig.LogMaxBackups, &config.LogMaxBackups)
	applyIntFlag(args.LogMaxAge, DefaultConfig.LogMaxAge, &config.LogMaxAge)
	applyIntFlag(args.BigKeyDataCacheTime, DefaultConfig.BigKeyDataCacheTime, &config.BigKeyDataCacheTime)
	applyIntFlag(args.AlarmKeyCacheTime, DefaultConfig.AlarmKeyCacheTime, &config.AlarmKeyCacheTime)
	applyIntFlag(args.SlowSqlTime, DefaultConfig.SlowSqlTime, &config.SlowSqlTime)
	applyIntFlag(args.SlowSqlMaxRows, DefaultConfig.SlowSqlMaxRows, &config.SlowSqlMaxRows)

	// 应用布尔类型的配置
	applyBoolFlag(args.RegisterHostMetrics, DefaultConfig.RegisterHostMetrics, &config.RegisterHostMetrics)
	applyBoolFlag(args.RegisterDatabaseMetrics, DefaultConfig.RegisterDatabaseMetrics, &config.RegisterDatabaseMetrics)
	applyBoolFlag(args.RegisterDmhsMetrics, DefaultConfig.RegisterDmhsMetrics, &config.RegisterDmhsMetrics)
	applyBoolFlag(args.RegisterCustomMetrics, DefaultConfig.RegisterCustomMetrics, &config.RegisterCustomMetrics)
	applyBoolFlag(args.EncodeConfigPwd, DefaultConfig.EncodeConfigPwd, &config.EncodeConfigPwd)
	applyBoolFlag(args.CheckSlowSQL, DefaultConfig.CheckSlowSQL, &config.CheckSlowSQL)
	applyBoolFlag(args.EnableBasicAuth, DefaultConfig.EnableBasicAuth, &config.EnableBasicAuth)

	// 应用全局超时控制配置
	applyIntFlag(args.GlobalTimeoutSeconds, DefaultConfig.GlobalTimeoutSeconds, &config.GlobalTimeoutSeconds)
	applyFloat64Flag(args.P99LatencyTarget, DefaultConfig.P99LatencyTarget, &config.P99LatencyTarget)
	applyBoolFlag(args.EnablePartialReturn, DefaultConfig.EnablePartialReturn, &config.EnablePartialReturn)
	applyIntFlag(args.LatencyWindowSize, DefaultConfig.LatencyWindowSize, &config.LatencyWindowSize)
}
