package config

import (
	"sync"
)

// 全局变量
var (
	version string
)

// SetVersion 设置版本号
func SetVersion(v string) {
	version = v
}

// GetVersion 获取版本号
func GetVersion() string {
	return version
}

// GlobalSettings 全局配置访问接口
type GlobalSettings struct {
	mu     sync.RWMutex
	config *MultiSourceConfig
}

// Global 全局配置实例
var Global = &GlobalSettings{}

// Init 初始化全局配置
func (g *GlobalSettings) Init(config *MultiSourceConfig) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.config = config
}

// GetConfig 获取配置
func (g *GlobalSettings) GetConfig() *MultiSourceConfig {
	g.mu.RLock()
	defer g.mu.RUnlock()
	return g.config
}

// 以下是兼容性访问方法，提供类似旧Config的访问接口

// GetListenAddress 获取监听地址
func (g *GlobalSettings) GetListenAddress() string {
	g.mu.RLock()
	defer g.mu.RUnlock()
	if g.config == nil {
		return DefaultMultiSourceConfig.ListenAddress
	}
	return g.config.ListenAddress
}

// GetMetricPath 获取指标路径
func (g *GlobalSettings) GetMetricPath() string {
	g.mu.RLock()
	defer g.mu.RUnlock()
	if g.config == nil {
		return DefaultMultiSourceConfig.MetricPath
	}
	return g.config.MetricPath
}

// GetLogLevel 获取日志级别
func (g *GlobalSettings) GetLogLevel() string {
	g.mu.RLock()
	defer g.mu.RUnlock()
	if g.config == nil {
		return DefaultMultiSourceConfig.LogLevel
	}
	return g.config.LogLevel
}

// GetLogMaxSize 获取日志最大大小
func (g *GlobalSettings) GetLogMaxSize() int {
	g.mu.RLock()
	defer g.mu.RUnlock()
	if g.config == nil {
		return DefaultMultiSourceConfig.LogMaxSize
	}
	return g.config.LogMaxSize
}

// GetLogMaxBackups 获取日志最大备份数
func (g *GlobalSettings) GetLogMaxBackups() int {
	g.mu.RLock()
	defer g.mu.RUnlock()
	if g.config == nil {
		return DefaultMultiSourceConfig.LogMaxBackups
	}
	return g.config.LogMaxBackups
}

// GetLogMaxAge 获取日志最大保留天数
func (g *GlobalSettings) GetLogMaxAge() int {
	g.mu.RLock()
	defer g.mu.RUnlock()
	if g.config == nil {
		return DefaultMultiSourceConfig.LogMaxAge
	}
	return g.config.LogMaxAge
}

// GetEnableBasicAuth 获取是否启用基础认证
func (g *GlobalSettings) GetEnableBasicAuth() bool {
	g.mu.RLock()
	defer g.mu.RUnlock()
	if g.config == nil {
		return DefaultMultiSourceConfig.EnableBasicAuth
	}
	return g.config.EnableBasicAuth
}

// GetBasicAuthUsername 获取基础认证用户名
func (g *GlobalSettings) GetBasicAuthUsername() string {
	g.mu.RLock()
	defer g.mu.RUnlock()
	if g.config == nil {
		return DefaultMultiSourceConfig.BasicAuthUsername
	}
	return g.config.BasicAuthUsername
}

// GetBasicAuthPassword 获取基础认证密码
func (g *GlobalSettings) GetBasicAuthPassword() string {
	g.mu.RLock()
	defer g.mu.RUnlock()
	if g.config == nil {
		return DefaultMultiSourceConfig.BasicAuthPassword
	}
	return g.config.BasicAuthPassword
}

// GetGlobalTimeoutSeconds 获取全局超时时间
func (g *GlobalSettings) GetGlobalTimeoutSeconds() int {
	g.mu.RLock()
	defer g.mu.RUnlock()
	if g.config == nil {
		return DefaultMultiSourceConfig.GlobalTimeoutSeconds
	}
	return g.config.GlobalTimeoutSeconds
}

// GetEnableHealthPing 获取是否启用周期性健康检查
func (g *GlobalSettings) GetEnableHealthPing() bool {
	g.mu.RLock()
	defer g.mu.RUnlock()
	if g.config == nil {
		return DefaultMultiSourceConfig.EnableHealthPing
	}
	return g.config.IsHealthPingEnabled()
}

// GetDefaultDataSource 获取默认数据源配置（用于兼容旧代码）
func (g *GlobalSettings) GetDefaultDataSource() *DataSourceConfig {
	g.mu.RLock()
	defer g.mu.RUnlock()
	if g.config == nil || len(g.config.DataSources) == 0 {
		defaultDS := DefaultDataSourceConfig
		return &defaultDS
	}
	return &g.config.DataSources[0]
}

// GetQueryTimeout 获取查询超时（从第一个数据源）
func (g *GlobalSettings) GetQueryTimeout() int {
	ds := g.GetDefaultDataSource()
	return ds.QueryTimeout
}

// GetMaxOpenConns 获取最大连接数（从第一个数据源）
func (g *GlobalSettings) GetMaxOpenConns() int {
	ds := g.GetDefaultDataSource()
	return ds.MaxOpenConns
}

// GetMaxIdleConns 获取最大空闲连接数（从第一个数据源）
func (g *GlobalSettings) GetMaxIdleConns() int {
	ds := g.GetDefaultDataSource()
	return ds.MaxIdleConns
}

// GetConnMaxLifetime 获取连接最大生命周期（从第一个数据源）
func (g *GlobalSettings) GetConnMaxLifetime() int {
	ds := g.GetDefaultDataSource()
	return ds.ConnMaxLifetime
}

// GetRegisterHostMetrics 获取是否注册主机指标（从第一个数据源）
func (g *GlobalSettings) GetRegisterHostMetrics() bool {
	ds := g.GetDefaultDataSource()
	return ds.RegisterHostMetrics
}

// GetRegisterCustomMetrics 获取是否注册自定义指标（从第一个数据源）
func (g *GlobalSettings) GetRegisterCustomMetrics() bool {
	ds := g.GetDefaultDataSource()
	return ds.RegisterCustomMetrics
}

// GetCustomMetricsFile 获取自定义指标文件（从第一个数据源）
func (g *GlobalSettings) GetCustomMetricsFile() string {
	ds := g.GetDefaultDataSource()
	return ds.CustomMetricsFile
}

// GetCheckSlowSQL 获取是否检查慢SQL（从第一个数据源）
func (g *GlobalSettings) GetCheckSlowSQL() bool {
	ds := g.GetDefaultDataSource()
	return ds.CheckSlowSQL
}

// GetSlowSqlTime 获取慢SQL时间阈值（从第一个数据源）
func (g *GlobalSettings) GetSlowSqlTime() int {
	ds := g.GetDefaultDataSource()
	return ds.SlowSqlTime
}

// GetSlowSqlMaxRows 获取慢SQL最大行数（从第一个数据源）
func (g *GlobalSettings) GetSlowSqlMaxRows() int {
	ds := g.GetDefaultDataSource()
	return ds.SlowSqlMaxRows
}

// GetAlarmKeyCacheTime 获取告警缓存时间（从第一个数据源）
func (g *GlobalSettings) GetAlarmKeyCacheTime() int {
	ds := g.GetDefaultDataSource()
	return ds.AlarmKeyCacheTime
}

// GetBigKeyDataCacheTime 获取大key缓存时间（从第一个数据源）
func (g *GlobalSettings) GetBigKeyDataCacheTime() int {
	ds := g.GetDefaultDataSource()
	return ds.BigKeyDataCacheTime
}
