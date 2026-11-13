package collector

import (
	"dameng_exporter/config"
	"dameng_exporter/db"
	"dameng_exporter/logger"
	"github.com/duke-git/lancet/v2/fileutil"
	"github.com/prometheus/client_golang/prometheus"
	"go.uber.org/zap"
	"strings"
	"sync"
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
	// 配置已在注册时预加载，这里只需要从缓存读取
	a.cacheMutex.RLock()
	cfg, exists := a.configCache[dsName]
	a.cacheMutex.RUnlock()

	if !exists {
		// 配置应该在注册时已加载，如果没有找到说明该数据源没有配置或加载失败
		logger.Logger.Debugf("No custom metrics config cached for datasource [%s] ", dsName)
	}

	return cfg
}

// Describe 实现Prometheus Collector接口
func (a *CustomMetricsMultiSourceAdapter) Describe(ch chan<- *prometheus.Desc) {
	// 自定义指标是动态的，不预先描述
}

func (a *CustomMetricsMultiSourceAdapter) Collect(ch chan<- prometheus.Metric) {
	round := newCollectionRound()
	a.collectWithRound(ch, round)
	round.finish(nil)
}

func (a *CustomMetricsMultiSourceAdapter) collectWithRound(ch chan<- prometheus.Metric, round *collectionRound) {
	var wg sync.WaitGroup
	for _, pool := range a.poolManager.GetHealthyPools() {
		cfg := a.loadConfigForDataSource(pool.Name)
		if cfg == nil || len(cfg.Metrics) == 0 {
			continue
		}
		wg.Add(1)
		go func(p *db.DataSourcePool, cfg *config.CustomConfig) {
			defer wg.Done()
			a.collectPool(ch, p, cfg, round)
		}(pool, cfg)
	}
	wg.Wait()
}

// Custom metrics retain blocking forwarding, independently of the global mode.
func (a *CustomMetricsMultiSourceAdapter) collectPool(ch chan<- prometheus.Metric, p *db.DataSourcePool, cfg *config.CustomConfig, round *collectionRound) {
	collector := NewCustomMetrics(p.DB, *cfg)
	SetDataSourceIfSupported(collector, p.Name)
	collectWithLogging(ch, p, collector, round, false, 0)
}

// RegisterCustomMetricsForMultiSource 注册支持多数据源独立配置的自定义指标采集器
func RegisterCustomMetricsForMultiSource(reg *prometheus.Registry, poolManager *db.DBPoolManager) {
	if adapter := buildCustomMetricsAdapter(poolManager); adapter != nil {
		reg.MustRegister(adapter)
	}
}

func buildCustomMetricsAdapter(poolManager *db.DBPoolManager) *CustomMetricsMultiSourceAdapter {
	// 检查是否有任何数据源需要自定义指标
	needCustomMetrics := false
	totalMetricsCount := 0
	loadedDataSources := []string{}

	// 创建适配器实例
	adapter := NewCustomMetricsMultiSourceAdapter(poolManager)

	for _, ds := range config.GlobalMultiConfig.DataSources {
		if ds.Enabled && ds.RegisterCustomMetrics {
			needCustomMetrics = true

			// 如果没有配置自定义文件，跳过
			if ds.CustomMetricsFile == "" {
				logger.Logger.Debugf("DataSource [%s] has no custom metrics file configured, skipping",
					ds.Name)
				continue
			}

			// 预加载并验证配置文件
			if fileutil.IsExist(ds.CustomMetricsFile) {
				customConfig, err := config.ParseCustomConfig(ds.CustomMetricsFile)
				if err != nil {
					logger.Logger.Desugar().Error("Failed to parse custom metrics config",
						zap.String("datasource", ds.Name),
						zap.String("file", ds.CustomMetricsFile),
						zap.Error(err))
					continue
				}

				// 缓存配置到适配器中，避免运行时重复加载
				adapter.cacheMutex.Lock()
				adapter.configCache[ds.Name] = &customConfig
				adapter.cacheMutex.Unlock()

				metricsCount := len(customConfig.Metrics)
				totalMetricsCount += metricsCount
				loadedDataSources = append(loadedDataSources, ds.Name)

				// 输出每个数据源的详细加载信息
				logger.Logger.Infof("DataSource [%s] loaded %d custom metric(s) from %s",
					ds.Name, metricsCount, ds.CustomMetricsFile)

				// 输出每个指标的详细信息
				for _, metric := range customConfig.Metrics {
					fieldsCount := len(metric.MetricsDesc)
					logger.Logger.Debugf("  - Context: %s, Labels: %v, Fields: %d",
						metric.Context, metric.Labels, fieldsCount)
				}
			} else {
				logger.Logger.Warnf("Custom metrics file not found for datasource [%s]: %s",
					ds.Name, ds.CustomMetricsFile)
			}
		}
	}

	if !needCustomMetrics {
		logger.Logger.Info("No datasource requires custom metrics")
		return nil
	}

	// 输出汇总信息
	if len(loadedDataSources) > 0 {
		logger.Logger.Infof("Successfully registered custom metrics adapter: Total %d metric(s) from %d datasource(s) [%s]",
			totalMetricsCount, len(loadedDataSources), strings.Join(loadedDataSources, ", "))
	} else {
		logger.Logger.Warn("Custom metrics adapter registered but no metrics were loaded successfully")
	}
	return adapter
}
