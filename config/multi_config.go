package config

import (
	"fmt"
	"strings"
)

// MultiSourceConfig 多数据源配置结构
type MultiSourceConfig struct {
	// 全局系统级配置（不可下沉）
	ConfigFile        string `toml:"-"` // 配置文件路径，不从配置文件读取
	ListenAddress     string `toml:"listenAddress"`
	MetricPath        string `toml:"metricPath"`
	Version           string `toml:"version"`
	LogMaxSize        int    `toml:"logMaxSize"`
	LogMaxBackups     int    `toml:"logMaxBackups"`
	LogMaxAge         int    `toml:"logMaxAge"`
	LogLevel          string `toml:"logLevel"`
	EncodeConfigPwd   bool   `toml:"encodeConfigPwd"`
	EnableBasicAuth   bool   `toml:"enableBasicAuth"`
	BasicAuthUsername string `toml:"basicAuthUsername"`
	BasicAuthPassword string `toml:"basicAuthPassword"`

	// 全局超时控制配置
	GlobalTimeoutSeconds int     `toml:"globalTimeoutSeconds"` // 全局超时时间（秒）
	P99LatencyTarget     float64 `toml:"p99LatencyTarget"`     // P99延迟目标（秒）
	EnablePartialReturn  bool    `toml:"enablePartialReturn"`  // 是否启用部分结果返回
	LatencyWindowSize    int     `toml:"latencyWindowSize"`    // P99延迟统计窗口大小（最近N次采集）

	// 数据源列表
	DataSources []DataSourceConfig `toml:"datasource"`
}

// DataSourceConfig 数据源配置
type DataSourceConfig struct {
	// 数据源基本信息
	Name        string `toml:"name"`
	Description string `toml:"description"`
	Enabled     bool   `toml:"enabled"`

	// 数据库连接配置（从全局下沉）
	DbHost          string `toml:"dbHost"`
	DbUser          string `toml:"dbUser"`
	DbPwd           string `toml:"dbPwd"` // 支持明文和ENC()加密格式
	QueryTimeout    int    `toml:"queryTimeout"`
	MaxOpenConns    int    `toml:"maxOpenConns"`
	MaxIdleConns    int    `toml:"maxIdleConns"`
	ConnMaxLifetime int    `toml:"connMaxLifetime"`

	// 缓存配置（从全局下沉）
	BigKeyDataCacheTime int `toml:"bigKeyDataCacheTime"`
	AlarmKeyCacheTime   int `toml:"alarmKeyCacheTime"`

	// 慢SQL配置（从全局下沉）
	CheckSlowSQL   bool `toml:"checkSlowSQL"`
	SlowSqlTime    int  `toml:"slowSqlTime"`
	SlowSqlMaxRows int  `toml:"slowSqlMaxRows"`

	// 指标注册配置（从全局下沉）
	RegisterHostMetrics     bool `toml:"registerHostMetrics"`
	RegisterDatabaseMetrics bool `toml:"registerDatabaseMetrics"`
	RegisterDmhsMetrics     bool `toml:"registerDmhsMetrics"`
	RegisterCustomMetrics   bool `toml:"registerCustomMetrics"`

	// 采集配置
	Priority          int    `toml:"priority"`          // 优先级: 1-高 2-中 3-低
	Labels            string `toml:"labels"`            // 标签字符串，格式: "key1=val1,key2=val2"
	CustomMetricsFile string `toml:"customMetricsFile"` // 数据源专用的自定义指标配置文件
}

// DefaultMultiSourceConfig 默认多数据源配置
var DefaultMultiSourceConfig = MultiSourceConfig{
	// 全局默认值
	ListenAddress:     ":9200",
	MetricPath:        "/metrics",
	LogMaxSize:        10,
	LogMaxBackups:     3,
	LogMaxAge:         30,
	LogLevel:          "info",
	EncodeConfigPwd:   false,
	EnableBasicAuth:   false,
	BasicAuthUsername: "",
	BasicAuthPassword: "",

	// 全局超时控制默认值
	GlobalTimeoutSeconds: 5,    // 默认5秒全局超时
	P99LatencyTarget:     2.0,  // 默认P99延迟目标2秒
	EnablePartialReturn:  true, // 默认启用部分结果返回
	LatencyWindowSize:    100,  // 默认统计最近100次采集
}

// DefaultDataSourceConfig 默认数据源配置
var DefaultDataSourceConfig = DataSourceConfig{
	// 基本信息默认值
	Enabled: true,

	// 连接池默认值
	QueryTimeout:    30,
	MaxOpenConns:    10,
	MaxIdleConns:    2,
	ConnMaxLifetime: 30,

	// 缓存默认值
	BigKeyDataCacheTime: 60,
	AlarmKeyCacheTime:   5,

	// 慢SQL默认值
	CheckSlowSQL:   false,
	SlowSqlTime:    10000,
	SlowSqlMaxRows: 10,

	// 指标注册默认值
	RegisterHostMetrics:     false,
	RegisterDatabaseMetrics: true,
	RegisterDmhsMetrics:     false,
	RegisterCustomMetrics:   true,

	// 其他默认值
	Priority:          2,
	CustomMetricsFile: "./custom_metrics.toml",
}

// ApplyDefaults 应用默认值
func (ds *DataSourceConfig) ApplyDefaults() {
	if ds.QueryTimeout == 0 {
		ds.QueryTimeout = DefaultDataSourceConfig.QueryTimeout
	}
	if ds.MaxOpenConns == 0 {
		ds.MaxOpenConns = DefaultDataSourceConfig.MaxOpenConns
	}
	if ds.MaxIdleConns == 0 {
		ds.MaxIdleConns = DefaultDataSourceConfig.MaxIdleConns
	}
	if ds.ConnMaxLifetime == 0 {
		ds.ConnMaxLifetime = DefaultDataSourceConfig.ConnMaxLifetime
	}
	if ds.BigKeyDataCacheTime == 0 {
		ds.BigKeyDataCacheTime = DefaultDataSourceConfig.BigKeyDataCacheTime
	}
	if ds.AlarmKeyCacheTime == 0 {
		ds.AlarmKeyCacheTime = DefaultDataSourceConfig.AlarmKeyCacheTime
	}
	if ds.SlowSqlTime == 0 {
		ds.SlowSqlTime = DefaultDataSourceConfig.SlowSqlTime
	}
	if ds.SlowSqlMaxRows == 0 {
		ds.SlowSqlMaxRows = DefaultDataSourceConfig.SlowSqlMaxRows
	}
	if ds.Priority == 0 {
		ds.Priority = DefaultDataSourceConfig.Priority
	}
	if ds.CustomMetricsFile == "" {
		ds.CustomMetricsFile = DefaultDataSourceConfig.CustomMetricsFile
	}
	if ds.Description == "" {
		ds.Description = fmt.Sprintf("DataSource: %s", ds.Name)
	}

	// 布尔值默认处理（只有在创建新配置时才应用）
	// 注意：TOML解析会正确设置布尔值，这里主要用于程序内部创建的配置
}

// Validate 验证配置
func (ds *DataSourceConfig) Validate() error {
	// 必填字段检查
	if ds.Name == "" {
		return fmt.Errorf("datasource name is required")
	}
	if ds.DbHost == "" {
		return fmt.Errorf("dbHost is required for datasource: %s", ds.Name)
	}
	if ds.DbUser == "" {
		return fmt.Errorf("dbUser is required for datasource: %s", ds.Name)
	}

	// 数值范围验证
	if ds.QueryTimeout < 1 || ds.QueryTimeout > 300 {
		return fmt.Errorf("queryTimeout must be between 1-300 seconds for datasource: %s", ds.Name)
	}
	if ds.MaxOpenConns < 1 || ds.MaxOpenConns > 100 {
		return fmt.Errorf("maxOpenConns must be between 1-100 for datasource: %s", ds.Name)
	}
	if ds.Priority < 1 || ds.Priority > 3 {
		return fmt.Errorf("priority must be between 1-3 for datasource: %s", ds.Name)
	}

	return nil
}

// ParseLabels 解析标签字符串
func (ds *DataSourceConfig) ParseLabels() map[string]string {
	labels := make(map[string]string)
	if ds.Labels == "" {
		return labels
	}

	pairs := strings.Split(ds.Labels, ",")
	for _, pair := range pairs {
		parts := strings.SplitN(strings.TrimSpace(pair), "=", 2)
		if len(parts) == 2 {
			key := strings.TrimSpace(parts[0])
			value := strings.TrimSpace(parts[1])
			if key != "" && value != "" {
				labels[key] = value
			}
		}
	}
	return labels
}

// GetDataSourceByName 根据名称获取数据源配置
func (msc *MultiSourceConfig) GetDataSourceByName(name string) *DataSourceConfig {
	for i := range msc.DataSources {
		if msc.DataSources[i].Name == name {
			return &msc.DataSources[i]
		}
	}
	return nil
}

// GetDataSourcesByPriority 根据优先级获取数据源配置
func (msc *MultiSourceConfig) GetDataSourcesByPriority(priority int) []DataSourceConfig {
	var result []DataSourceConfig
	for _, ds := range msc.DataSources {
		if ds.Priority == priority && ds.Enabled {
			result = append(result, ds)
		}
	}
	return result
}

// ValidateAll 验证所有配置
func (msc *MultiSourceConfig) ValidateAll() error {
	// 验证全局配置
	if msc.ListenAddress == "" {
		return fmt.Errorf("listenAddress is required")
	}
	if msc.MetricPath == "" {
		return fmt.Errorf("metricPath is required")
	}

	// 验证数据源配置
	if len(msc.DataSources) == 0 {
		return fmt.Errorf("at least one datasource is required")
	}

	// 检查数据源名称唯一性
	nameMap := make(map[string]bool)
	for _, ds := range msc.DataSources {
		if nameMap[ds.Name] {
			return fmt.Errorf("duplicate datasource name: %s", ds.Name)
		}
		nameMap[ds.Name] = true

		// 验证每个数据源
		if err := ds.Validate(); err != nil {
			return err
		}
	}

	return nil
}

// ApplyAllDefaults 为所有数据源应用默认值
func (msc *MultiSourceConfig) ApplyAllDefaults() {
	// 应用全局默认值
	if msc.ListenAddress == "" {
		msc.ListenAddress = DefaultMultiSourceConfig.ListenAddress
	}
	if msc.MetricPath == "" {
		msc.MetricPath = DefaultMultiSourceConfig.MetricPath
	}
	if msc.LogMaxSize == 0 {
		msc.LogMaxSize = DefaultMultiSourceConfig.LogMaxSize
	}
	if msc.LogMaxBackups == 0 {
		msc.LogMaxBackups = DefaultMultiSourceConfig.LogMaxBackups
	}
	if msc.LogMaxAge == 0 {
		msc.LogMaxAge = DefaultMultiSourceConfig.LogMaxAge
	}
	if msc.LogLevel == "" {
		msc.LogLevel = DefaultMultiSourceConfig.LogLevel
	}

	// 应用全局超时控制默认值
	if msc.GlobalTimeoutSeconds == 0 {
		msc.GlobalTimeoutSeconds = DefaultMultiSourceConfig.GlobalTimeoutSeconds
	}
	if msc.P99LatencyTarget == 0 {
		msc.P99LatencyTarget = DefaultMultiSourceConfig.P99LatencyTarget
	}
	if msc.LatencyWindowSize == 0 {
		msc.LatencyWindowSize = DefaultMultiSourceConfig.LatencyWindowSize
	}
	// EnablePartialReturn 是布尔类型，TOML解析会正确处理

	// 为每个数据源应用默认值
	for i := range msc.DataSources {
		msc.DataSources[i].ApplyDefaults()
	}
}
