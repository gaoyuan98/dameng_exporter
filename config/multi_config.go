package config

import (
	"fmt"
	"strings"
)

// GlobalMultiConfig 全局多数据源配置实例
var GlobalMultiConfig *MultiSourceConfig

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
	GlobalTimeoutSeconds int `toml:"globalTimeoutSeconds"` // 全局超时时间（秒）

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

	// 缓存配置
	BigKeyDataCacheTime int `toml:"bigKeyDataCacheTime"`
	AlarmKeyCacheTime   int `toml:"alarmKeyCacheTime"`

	// 慢SQL配置
	CheckSlowSQL   bool `toml:"checkSlowSQL"`
	SlowSqlTime    int  `toml:"slowSqlTime"`
	SlowSqlMaxRows int  `toml:"slowSqlMaxRows"`

	// 指标注册配置
	RegisterHostMetrics     bool `toml:"registerHostMetrics"`
	RegisterDatabaseMetrics bool `toml:"registerDatabaseMetrics"`
	RegisterDmhsMetrics     bool `toml:"registerDmhsMetrics"`
	RegisterCustomMetrics   bool `toml:"registerCustomMetrics"`

	// 采集配置
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
	GlobalTimeoutSeconds: 5, // 默认5秒全局超时
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
	CustomMetricsFile: "./custom_queries.metrics",
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
		return fmt.Errorf("数据源名称不能为空")
	}
	if ds.DbHost == "" {
		return fmt.Errorf("数据源 %s: 数据库地址不能为空 (dbHost)", ds.Name)
	}
	if ds.DbUser == "" {
		return fmt.Errorf("数据源 %s: 数据库用户名不能为空 (dbUser)", ds.Name)
	}

	// 数值范围验证
	if ds.QueryTimeout < 1 || ds.QueryTimeout > 300 {
		return fmt.Errorf("数据源 %s: 查询超时时间必须在 1-300 秒之间 (queryTimeout)", ds.Name)
	}
	if ds.MaxOpenConns < 1 || ds.MaxOpenConns > 100 {
		return fmt.Errorf("数据源 %s: 最大打开连接数必须在 1-100 之间 (maxOpenConns)", ds.Name)
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

// ValidateAll 验证所有配置
func (msc *MultiSourceConfig) ValidateAll() error {
	// 验证全局配置
	if msc.ListenAddress == "" {
		return fmt.Errorf("监听地址不能为空 (listenAddress)")
	}
	if msc.MetricPath == "" {
		return fmt.Errorf("指标路径不能为空 (metricPath)")
	}

	// 验证数据源配置
	if len(msc.DataSources) == 0 {
		return fmt.Errorf("至少需要配置一个数据源")
	}

	// 检查数据源名称唯一性
	nameMap := make(map[string]bool)
	// 检查数据源地址唯一性
	hostMap := make(map[string]string) // host -> name mapping

	for _, ds := range msc.DataSources {
		// 检查名称重复
		if nameMap[ds.Name] {
			return fmt.Errorf("数据源名称重复: %s", ds.Name)
		}
		nameMap[ds.Name] = true

		// 检查地址重复（仅对启用的数据源进行检查）
		if ds.Enabled {
			// 标准化主机地址（去除可能的查询参数）
			hostAddr := ds.DbHost
			if idx := strings.Index(hostAddr, "?"); idx != -1 {
				hostAddr = hostAddr[:idx]
			}

			// 检查是否已存在相同的主机地址
			if existingName, exists := hostMap[hostAddr]; exists {
				return fmt.Errorf("数据源地址重复: %s (被 '%s' 和 '%s' 同时使用)",
					hostAddr, existingName, ds.Name)
			}
			hostMap[hostAddr] = ds.Name
		}

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

	// 为每个数据源应用默认值
	for i := range msc.DataSources {
		msc.DataSources[i].ApplyDefaults()
	}
}

// StringCategorized 返回分类格式的配置信息字符串（简洁版）
func (msc *MultiSourceConfig) StringCategorized() string {
	var sb strings.Builder

	sb.WriteString("\n========== Configuration Summary ==========\n")

	// 服务配置 - 一行
	sb.WriteString(fmt.Sprintf("[Service] listenAddress=%s, metricPath=%s, version=%s\n",
		msc.ListenAddress, msc.MetricPath, msc.Version))

	// 日志配置 - 一行
	sb.WriteString(fmt.Sprintf("[Logging] level=%s, maxSize=%dMB, maxBackups=%d, maxAge=%d days\n",
		msc.LogLevel, msc.LogMaxSize, msc.LogMaxBackups, msc.LogMaxAge))

	// 安全配置 - 一行
	authInfo := fmt.Sprintf("basicAuth=%v", msc.EnableBasicAuth)
	if msc.EnableBasicAuth {
		authInfo += fmt.Sprintf(", user=%s", msc.BasicAuthUsername)
	}
	sb.WriteString(fmt.Sprintf("[Security] %s, encodeConfigPwd=%v\n",
		authInfo, msc.EncodeConfigPwd))

	// 性能配置 - 一行
	sb.WriteString(fmt.Sprintf("[Performance] globalTimeout=%ds\n",
		msc.GlobalTimeoutSeconds))

	// 数据源摘要 - 一行
	enabledCount := 0
	var dsNames []string
	for _, ds := range msc.DataSources {
		if ds.Enabled {
			enabledCount++
			dsNames = append(dsNames, ds.Name)
		}
	}
	sb.WriteString(fmt.Sprintf("[DataSources] total=%d, enabled=%d (%s)\n",
		len(msc.DataSources), enabledCount, strings.Join(dsNames, ", ")))

	// 调试级别时输出每个数据源的详细配置
	if strings.ToLower(msc.LogLevel) == "debug" {
		sb.WriteString("\n---------- Debug: DataSource Details ----------\n")
		for i, ds := range msc.DataSources {
			sb.WriteString(fmt.Sprintf("[DS-%d] %s:\n", i+1, ds.Name))
			// 基本信息
			sb.WriteString(fmt.Sprintf("  Host: %s | User: %s | Enabled: %v\n",
				ds.DbHost, ds.DbUser, ds.Enabled))

			// 连接池配置
			sb.WriteString(fmt.Sprintf("  Connection: maxOpen=%d, maxIdle=%d, lifetime=%dmin, queryTimeout=%ds\n",
				ds.MaxOpenConns, ds.MaxIdleConns, ds.ConnMaxLifetime, ds.QueryTimeout))

			// 缓存配置
			sb.WriteString(fmt.Sprintf("  Cache: bigKeyData=%dmin, alarmKey=%dmin\n",
				ds.BigKeyDataCacheTime, ds.AlarmKeyCacheTime))

			// 指标功能开关
			sb.WriteString(fmt.Sprintf("  Metrics: host=%v, database=%v, dmhs=%v, custom=%v\n",
				ds.RegisterHostMetrics, ds.RegisterDatabaseMetrics, ds.RegisterDmhsMetrics, ds.RegisterCustomMetrics))

			// 慢SQL配置
			if ds.CheckSlowSQL {
				sb.WriteString(fmt.Sprintf("  SlowSQL: enabled=%v, threshold=%dms, maxRows=%d\n",
					ds.CheckSlowSQL, ds.SlowSqlTime, ds.SlowSqlMaxRows))
			} else {
				sb.WriteString(fmt.Sprintf("  SlowSQL: enabled=%v\n", ds.CheckSlowSQL))
			}

			// 显示标签信息（如果有）
			if ds.Labels != "" {
				sb.WriteString(fmt.Sprintf("  Labels: %s\n", ds.Labels))
			}

			// 显示自定义指标文件（如果有）
			if ds.CustomMetricsFile != "" && ds.CustomMetricsFile != "./custom_queries.metrics" {
				sb.WriteString(fmt.Sprintf("  CustomFile: %s\n", ds.CustomMetricsFile))
			}
		}
		sb.WriteString("----------------------------------------------\n")
	}

	sb.WriteString("============================================")
	return sb.String()
}
