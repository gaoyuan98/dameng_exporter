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

	// 采集模式参数
	CollectionMode *string
}

// MergeConfig 函数已移除，使用 MergeMultiSourceConfig 代替
