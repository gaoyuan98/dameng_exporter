package collector

import (
	"dameng_exporter/db"
	"dameng_exporter/logger"
	"database/sql"
	"fmt"
	"reflect"
	"strings"
	"sync"
	"time"

	"github.com/prometheus/client_golang/prometheus"
)

// DataSourceAware 接口，用于标识支持数据源感知的采集器
type DataSourceAware interface {
	SetDataSource(name string)
}

// SetDataSourceIfSupported 用于设置采集器的数据源名称
func SetDataSourceIfSupported(collector MetricCollector, dataSource string) {
	if dsa, ok := collector.(DataSourceAware); ok {
		dsa.SetDataSource(dataSource)
	}
}

// MultiSourceAdapter 多数据源适配器，用于快速改造现有采集器
type MultiSourceAdapter struct {
	poolManager     *db.DBPoolManager
	createCollector func(*sql.DB) MetricCollector
	collectorName   string // 采集器名称
	mu              sync.Mutex
}

// NewMultiSourceAdapter 创建多数据源适配器
func NewMultiSourceAdapter(poolManager *db.DBPoolManager, createFunc func(*sql.DB) MetricCollector) *MultiSourceAdapter {
	adapter := &MultiSourceAdapter{
		poolManager:     poolManager,
		createCollector: createFunc,
	}

	// 尝试获取采集器名称
	if poolManager != nil {
		pools := poolManager.GetHealthyPools()
		if len(pools) > 0 {
			// 创建一个临时采集器来获取类型名称
			tempCollector := createFunc(pools[0].DB)
			adapter.collectorName = getCollectorName(tempCollector)
		}
	}

	return adapter
}

// getCollectorName 获取采集器的名称
func getCollectorName(collector MetricCollector) string {
	if collector == nil {
		return "UnknownCollector"
	}

	// 获取类型名称
	t := reflect.TypeOf(collector)
	if t.Kind() == reflect.Ptr {
		t = t.Elem()
	}

	// 获取类型名称
	name := t.Name()
	if name == "" {
		name = fmt.Sprintf("%v", t)
	}

	// 移除Collector后缀，使名称更简洁
	name = strings.TrimSuffix(name, "Collector")

	return name
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

	// 为每个数据源采集指标 - 使用流式处理，不等待所有数据源完成
	var wg sync.WaitGroup

	for _, pool := range pools {
		wg.Add(1)
		go func(p *db.DataSourcePool) {
			defer wg.Done()

			// 记录开始时间
			startTime := time.Now()

			// 创建采集器实例
			collector := a.createCollector(p.DB)

			// 如果采集器支持数据源感知，设置数据源名称
			SetDataSourceIfSupported(collector, p.Name)

			// 创建标签注入器
			labelInjector := NewLabelInjectorFromPool(p)

			// 创建临时channel收集指标
			tempChan := make(chan prometheus.Metric, 100)

			// 启动goroutine实时转发指标并添加数据源标签
			// 关键改进：直接写入最终的ch，实现真正的流式处理
			var innerWg sync.WaitGroup
			innerWg.Add(1)
			go func() {
				defer innerWg.Done()
				for metric := range tempChan {
					// 使用MetricWrapper包装指标以注入数据源标签
					wrappedMetric := NewMetricWrapper(metric, labelInjector)
					// 直接发送到最终的channel，立即返回给Prometheus
					// 这样快速的数据源可以立即返回结果
					select {
					case ch <- wrappedMetric:
						// 成功发送
					case <-time.After(10 * time.Second):
						// 添加超时保护，避免永久阻塞
						logger.Logger.Warnf("[%s] Timeout sending metric for %s", p.Name, a.collectorName)
					}
				}
			}()

			// 执行采集
			collector.Collect(tempChan)

			// 关闭临时channel
			close(tempChan)

			// 等待内部转发完成
			innerWg.Wait()

			// 记录执行时间，包含数据源和采集器名称
			duration := time.Since(startTime)
			if a.collectorName != "" {
				logger.Logger.Infof("[%s] %s completed in: %vms", p.Name, a.collectorName, duration.Milliseconds())
			} else {
				logger.Logger.Infof("[%s] collector completed in: %vms", p.Name, duration.Milliseconds())
			}
		}(pool)
	}

	// 等待所有goroutine完成
	// 注意：由于直接写入ch，快速的数据源会立即返回结果，不会被慢的数据源阻塞
	wg.Wait()
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
