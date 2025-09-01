package collector

import (
	"dameng_exporter/config"
	"dameng_exporter/db"
	"dameng_exporter/logger"
	"sync"

	"github.com/duke-git/lancet/v2/fileutil"
	"github.com/prometheus/client_golang/prometheus"
	"go.uber.org/zap"
)

// CustomMetricsMultiSourceAdapter 专门处理自定义指标的多数据源适配器
// 每个数据源可以有自己独立的自定义指标配置文件
type CustomMetricsMultiSourceAdapter struct {
	poolManager *db.DBPoolManager
	configCache map[string]*config.CustomConfig // 缓存每个数据源的配置
	cacheMutex  sync.RWMutex
}

// NewCustomMetricsMultiSourceAdapter 创建自定义指标的多数据源适配器
func NewCustomMetricsMultiSourceAdapter(poolManager *db.DBPoolManager) *CustomMetricsMultiSourceAdapter {
	return &CustomMetricsMultiSourceAdapter{
		poolManager: poolManager,
		configCache: make(map[string]*config.CustomConfig),
	}
}

// loadConfigForDataSource 为指定数据源加载自定义指标配置
func (a *CustomMetricsMultiSourceAdapter) loadConfigForDataSource(dsName string) *config.CustomConfig {
	// 先检查缓存
	a.cacheMutex.RLock()
	if cfg, exists := a.configCache[dsName]; exists {
		a.cacheMutex.RUnlock()
		return cfg
	}
	a.cacheMutex.RUnlock()

	// 查找数据源配置
	var customMetricsFile string
	for _, ds := range config.GlobalMultiConfig.DataSources {
		if ds.Name == dsName && ds.Enabled && ds.RegisterCustomMetrics {
			customMetricsFile = ds.CustomMetricsFile
			break
		}
	}

	if customMetricsFile == "" {
		return nil
	}

	// 检查文件是否存在
	if !fileutil.IsExist(customMetricsFile) {
		logger.Logger.Warnf("Custom metrics file not found for datasource %s: %s", dsName, customMetricsFile)
		return nil
	}

	// 解析配置文件
	customConfig, err := config.ParseCustomConfig(customMetricsFile)
	if err != nil {
		logger.Logger.Error("Failed to parse custom metrics config",
			zap.String("datasource", dsName),
			zap.String("file", customMetricsFile),
			zap.Error(err))
		return nil
	}

	// 缓存配置
	a.cacheMutex.Lock()
	a.configCache[dsName] = &customConfig
	a.cacheMutex.Unlock()

	logger.Logger.Infof("Loaded %d custom metrics for datasource %s from %s",
		len(customConfig.Metrics), dsName, customMetricsFile)

	return &customConfig
}

// Describe 实现Prometheus Collector接口
func (a *CustomMetricsMultiSourceAdapter) Describe(ch chan<- *prometheus.Desc) {
	// 自定义指标是动态的，不预先描述
}

// Collect 实现Prometheus Collector接口
func (a *CustomMetricsMultiSourceAdapter) Collect(ch chan<- prometheus.Metric) {
	pools := a.poolManager.GetHealthyPools()

	var wg sync.WaitGroup
	for _, pool := range pools {
		// 为每个数据源加载其独立的配置
		customConfig := a.loadConfigForDataSource(pool.Name)
		if customConfig == nil || len(customConfig.Metrics) == 0 {
			continue
		}

		wg.Add(1)
		go func(p *db.DataSourcePool, cfg *config.CustomConfig) {
			defer wg.Done()

			// 为该数据源创建自定义指标采集器
			collector := NewCustomMetrics(p.DB, *cfg)

			// 设置数据源名称
			SetDataSourceIfSupported(collector, p.Name)

			// 创建标签注入器
			labelInjector := NewLabelInjectorFromPool(p)

			// 创建临时channel收集指标
			tempCh := make(chan prometheus.Metric, 500)
			done := make(chan struct{})

			// 异步收集指标
			go func() {
				defer close(done)
				collector.Collect(tempCh)
				close(tempCh)
			}()

			// 转发指标并注入标签
			for metric := range tempCh {
				// 使用 MetricWrapper 注入标签
				ch <- NewMetricWrapper(metric, labelInjector)
			}
			<-done
		}(pool, customConfig)
	}

	wg.Wait()
}

// RegisterCustomMetricsForMultiSource 注册支持多数据源独立配置的自定义指标采集器
func RegisterCustomMetricsForMultiSource(reg *prometheus.Registry, poolManager *db.DBPoolManager) {
	// 检查是否有任何数据源需要自定义指标
	needCustomMetrics := false
	for _, ds := range config.GlobalMultiConfig.DataSources {
		if ds.Enabled && ds.RegisterCustomMetrics {
			needCustomMetrics = true
			logger.Logger.Debugf("DataSource %s requires custom metrics from %s",
				ds.Name, ds.CustomMetricsFile)
		}
	}

	if !needCustomMetrics {
		logger.Logger.Debug("No datasource requires custom metrics")
		return
	}

	// 创建并注册自定义指标适配器
	adapter := NewCustomMetricsMultiSourceAdapter(poolManager)
	reg.MustRegister(adapter)
	logger.Logger.Info("Registered custom metrics adapter for multi-source with independent configs")
}
