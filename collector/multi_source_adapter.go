package collector

import (
	"dameng_exporter/db"
	"dameng_exporter/logger"
	"database/sql"
	"sync"

	"github.com/prometheus/client_golang/prometheus"
)

// MultiSourceAdapter 多数据源适配器，用于快速改造现有采集器
type MultiSourceAdapter struct {
	poolManager     *db.DBPoolManager
	createCollector func(*sql.DB) MetricCollector
	mu              sync.Mutex
}

// NewMultiSourceAdapter 创建多数据源适配器
func NewMultiSourceAdapter(poolManager *db.DBPoolManager, createFunc func(*sql.DB) MetricCollector) *MultiSourceAdapter {
	return &MultiSourceAdapter{
		poolManager:     poolManager,
		createCollector: createFunc,
	}
}

// Describe 实现Prometheus Collector接口
func (a *MultiSourceAdapter) Describe(ch chan<- *prometheus.Desc) {
	// 创建一个临时采集器来获取描述
	pools := a.poolManager.GetHealthyPools()
	if len(pools) > 0 {
		collector := a.createCollector(pools[0].DB)
		collector.Describe(ch)
	}
}

// Collect 实现Prometheus Collector接口
func (a *MultiSourceAdapter) Collect(ch chan<- prometheus.Metric) {
	// 获取所有健康的连接池
	pools := a.poolManager.GetHealthyPools()

	// 为每个数据源采集指标
	var wg sync.WaitGroup
	metricChan := make(chan prometheus.Metric, 1000)

	for _, pool := range pools {
		wg.Add(1)
		go func(p *db.DataSourcePool) {
			defer wg.Done()

			// 创建采集器实例
			collector := a.createCollector(p.DB)

			// 创建标签注入器
			labelInjector := NewLabelInjectorFromPool(p)

			// 创建临时channel收集指标
			tempChan := make(chan prometheus.Metric, 100)
			done := make(chan bool)

			// 启动goroutine转发指标并添加数据源标签
			go func() {
				for metric := range tempChan {
					// 使用MetricWrapper包装指标以注入数据源标签
					wrappedMetric := NewMetricWrapper(metric, labelInjector)
					metricChan <- wrappedMetric
				}
				done <- true
			}()

			// 执行采集
			collector.Collect(tempChan)

			// 关闭临时channel
			close(tempChan)
			<-done
		}(pool)
	}

	// 等待所有采集完成
	go func() {
		wg.Wait()
		close(metricChan)
	}()

	// 转发所有指标
	for metric := range metricChan {
		ch <- metric
	}
}

// AdaptCollector 适配单个采集器到多数据源
func AdaptCollector(poolManager *db.DBPoolManager, createFunc func(*sql.DB) MetricCollector) MetricCollector {
	// 如果poolManager为nil，返回兼容的单数据源采集器
	if poolManager == nil {
		// 使用全局DBPool
		if db.DBPool != nil {
			return createFunc(db.DBPool)
		}
		logger.Logger.Error("No database pool available")
		return nil
	}

	return NewMultiSourceAdapter(poolManager, createFunc)
}
