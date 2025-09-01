package config

import (
	"fmt"
)

var hostName string
var version string

func SetHostName(hn string) {
	hostName = hn
}

func GetHostName() string {
	return hostName
}

func SetVersion(v string) {
	version = v
}

func GetVersion() string {
	return version
}

var GlobalConfig *Config

// GlobalMultiConfig 全局多数据源配置实例（用于多数据源模式）
var GlobalMultiConfig *MultiSourceConfig

type Config struct {
	ConfigFile        string
	CustomMetricsFile string
	ListenAddress     string
	MetricPath        string
	Version           string

	//QueryTimeout  time.Duration
	QueryTimeout int
	MaxIdleConns int
	MaxOpenConns int
	//ConnMaxLifetime time.Duration
	ConnMaxLifetime int
	LogMaxSize      int
	LogMaxBackups   int
	LogMaxAge       int
	LogLevel        string
	DbHost          string
	DbUser          string
	DbPwd           string

	//大key的保留时间
	BigKeyDataCacheTime int
	AlarmKeyCacheTime   int

	//指标是否注册选项
	RegisterHostMetrics     bool
	RegisterDatabaseMetrics bool
	RegisterDmhsMetrics     bool
	RegisterCustomMetrics   bool
	EncodeConfigPwd         bool
	CheckSlowSQL            bool
	SlowSqlTime             int
	SlowSqlMaxRows          int

	// Basic Auth配置
	EnableBasicAuth   bool
	BasicAuthUsername string
	BasicAuthPassword string

	// 全局超时控制配置
	GlobalTimeoutSeconds int     // 全局超时时间（秒）
	P99LatencyTarget     float64 // P99延迟目标（秒）
	EnablePartialReturn  bool    // 是否启用部分结果返回
	LatencyWindowSize    int     // P99延迟统计窗口大小（最近N次采集）
}

var DefaultConfig = Config{
	ConfigFile:        "./dameng_exporter.toml",
	CustomMetricsFile: "./custom_metrics.toml",
	ListenAddress:     ":9200",
	MetricPath:        "/metrics",
	Version:           "v1.1.6",
	//QueryTimeout:    30 * time.Second,
	QueryTimeout:            30, //秒
	MaxIdleConns:            1,  //个数
	MaxOpenConns:            10, //个数
	ConnMaxLifetime:         30, //分钟
	LogMaxSize:              10, //MB
	LogMaxBackups:           3,  //个数
	LogMaxAge:               30, //天
	LogLevel:                "debug",
	BigKeyDataCacheTime:     60, //分
	AlarmKeyCacheTime:       5,  //分
	RegisterHostMetrics:     false,
	RegisterDatabaseMetrics: true,
	RegisterDmhsMetrics:     false,
	RegisterCustomMetrics:   true,
	EncodeConfigPwd:         true,
	DbUser:                  "SYSDBA",
	DbPwd:                   "SYSDBA",
	DbHost:                  "127.0.0.1:5236",
	CheckSlowSQL:            false,
	SlowSqlTime:             10000,
	SlowSqlMaxRows:          10,
	EnableBasicAuth:         false,
	BasicAuthUsername:       "",
	BasicAuthPassword:       "",
	GlobalTimeoutSeconds:    5,    // 默认5秒全局超时
	P99LatencyTarget:        2.0,  // 默认P99延迟目标2秒
	EnablePartialReturn:     true, // 默认启用部分结果返回
	LatencyWindowSize:       100,  // 默认统计最近100次采集
}

// StringCategorized 按类别输出，每个类别一行
func (c *Config) StringCategorized() string {
	maskedPwd := "***"
	if c.DbPwd == "" {
		maskedPwd = "(empty)"
	}
	maskedBasicAuthPwd := ""
	if c.BasicAuthPassword != "" {
		maskedBasicAuthPwd = "***"
	} else {
		maskedBasicAuthPwd = "(empty)"
	}

	return fmt.Sprintf(`Configuration loaded:
  [Database] dbHost=%s, dbUser=%s, dbPwd=%s
  [Server] listenAddress=%s, metricPath=%s, enableBasicAuth=%v(basicAuthUsername=%s,basicAuthPassword=%s)
  [Connection] maxOpenConns=%d, maxIdleConns=%d, queryTimeout=%ds, connMaxLifetime=%dm
  [Logging] logLevel=%s, logMaxSize=%dMB, logMaxBackups=%d, logMaxAge=%dd
  [Cache] bigKeyDataCacheTime=%dm, alarmKeyCacheTime=%dm
  [Features] registerHostMetrics=%v, registerDatabaseMetrics=%v, registerDmhsMetrics=%v, registerCustomMetrics=%v
  [SlowSQL] checkSlowSQL=%v, slowSqlTime=%dms, slowSqlLimitRows=%d
  [TimeoutControl] globalTimeoutSeconds=%ds, p99LatencyTarget=%.1fs, enablePartialReturn=%v, latencyWindowSize=%d
  [Security] encodeConfigPwd=%v`,
		c.DbHost, c.DbUser, maskedPwd,
		c.ListenAddress, c.MetricPath, c.EnableBasicAuth, c.BasicAuthUsername, maskedBasicAuthPwd,
		c.MaxOpenConns, c.MaxIdleConns, c.QueryTimeout, c.ConnMaxLifetime,
		c.LogLevel, c.LogMaxSize, c.LogMaxBackups, c.LogMaxAge,
		c.BigKeyDataCacheTime, c.AlarmKeyCacheTime,
		c.RegisterHostMetrics, c.RegisterDatabaseMetrics, c.RegisterDmhsMetrics, c.RegisterCustomMetrics,
		c.CheckSlowSQL, c.SlowSqlTime, c.SlowSqlMaxRows,
		c.GlobalTimeoutSeconds, c.P99LatencyTarget, c.EnablePartialReturn, c.LatencyWindowSize,
		c.EncodeConfigPwd)
}
