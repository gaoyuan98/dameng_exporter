package config

import (
	"bufio"
	"fmt"
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

// LoadCompatibleConfig 加载兼容配置（自动识别格式）
func LoadCompatibleConfig(configFile string) (*MultiSourceConfig, error) {
	// 检测配置格式
	format := DetectConfigFormat(configFile)

	switch format {
	case "toml":
		// 尝试加载为多数据源配置
		return LoadMultiSourceConfig(configFile)

	case "legacy":
		// 加载旧格式配置并转换
		oldConfig, err := LoadConfig(configFile)
		if err != nil {
			return nil, fmt.Errorf("failed to load legacy config: %w", err)
		}

		// 转换为多数据源配置
		msc := ConvertLegacyConfig(&oldConfig)
		msc.ConfigFile = configFile

		// 应用默认值
		msc.ApplyAllDefaults()

		// 解密密码
		if err := msc.DecryptPasswords(); err != nil {
			return nil, fmt.Errorf("failed to decrypt passwords: %w", err)
		}

		// 验证配置
		if err := msc.ValidateAll(); err != nil {
			return nil, fmt.Errorf("config validation failed: %w", err)
		}

		return msc, nil

	default:
		return nil, fmt.Errorf("unknown config format")
	}
}

// ConvertConfigToTOML 将旧格式配置文件转换为TOML格式
func ConvertConfigToTOML(oldConfigFile, newConfigFile string) error {
	// 加载旧配置
	oldConfig, err := LoadConfig(oldConfigFile)
	if err != nil {
		return fmt.Errorf("failed to load old config: %w", err)
	}

	// 转换为多数据源配置
	msc := ConvertLegacyConfig(&oldConfig)

	// 保存为TOML格式
	return SaveMultiSourceConfig(msc, newConfigFile)
}

// MigrateGlobalConfig 将全局Config迁移到MultiSourceConfig
func MigrateGlobalConfig() (*MultiSourceConfig, error) {
	// 如果GlobalConfig为空，返回错误
	if GlobalConfig == nil {
		return nil, fmt.Errorf("global config is not initialized")
	}

	// 转换配置
	msc := ConvertLegacyConfig(GlobalConfig)

	// 应用默认值
	msc.ApplyAllDefaults()

	return msc, nil
}

// CreateCompatibleGlobalConfig 创建兼容的全局配置（用于向后兼容）
func CreateCompatibleGlobalConfig(msc *MultiSourceConfig) *Config {
	// 如果没有数据源，返回默认配置
	if len(msc.DataSources) == 0 {
		return &DefaultConfig
	}

	// 使用第一个数据源的配置
	ds := msc.DataSources[0]

	return &Config{
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
	}
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
	}
}

// CreateSampleTOMLConfig 创建示例TOML配置文件
func CreateSampleTOMLConfig(filename string) error {
	sampleConfig := `# 达梦数据库监控采集器多数据源配置示例
# 全局配置
listenAddress = ":9200"
metricPath = "/metrics"
logLevel = "info"
logMaxSize = 10
logMaxBackups = 3
logMaxAge = 30
encodeConfigPwd = true

# Basic Auth配置（可选）
enableBasicAuth = false
basicAuthUsername = "admin"
basicAuthPassword = ""

# 采集策略配置
collectStrategy = "hybrid"  # sequential/concurrent/hybrid
maxConcurrentGroups = 3
groupTimeout = 60

# 数据源配置示例1：生产环境主库
[[datasource]]
name = "prod-master"
description = "生产环境主库"
enabled = true
dbHost = "192.168.1.10:5236"
dbUser = "SYSDBA"
dbPwd = "password1"  # 将自动加密为ENC(xxx)格式
queryTimeout = 30
maxOpenConns = 10
maxIdleConns = 2
connMaxLifetime = 30
bigKeyDataCacheTime = 60
alarmKeyCacheTime = 5
checkSlowSQL = true
slowSqlTime = 10000
slowSqlMaxRows = 10
registerHostMetrics = false
registerDatabaseMetrics = true
registerDmhsMetrics = false
registerCustomMetrics = true
priority = 1  # 高优先级
labels = "cluster=prod,role=master,zone=beijing"
customMetricsFile = "./custom_metrics_prod_master.toml"

# 数据源配置示例2：生产环境从库
[[datasource]]
name = "prod-slave1"
description = "生产环境从库1"
enabled = true
dbHost = "192.168.1.11:5236"
dbUser = "SYSDBA"
dbPwd = "password2"
queryTimeout = 20
maxOpenConns = 5
maxIdleConns = 1
connMaxLifetime = 20
bigKeyDataCacheTime = 30
alarmKeyCacheTime = 3
checkSlowSQL = false
registerDatabaseMetrics = true
priority = 2  # 中等优先级
labels = "cluster=prod,role=slave,zone=beijing"

# 数据源配置示例3：测试环境（最简配置）
[[datasource]]
name = "test-db"
dbHost = "192.168.1.100:5236"
dbUser = "SYSDBA"
dbPwd = "password3"
# 其他配置项将使用默认值
`

	return os.WriteFile(filename, []byte(sampleConfig), 0644)
}
