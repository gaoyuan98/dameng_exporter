package collector

import (
	"dameng_exporter/config"
	"dameng_exporter/db"
	"dameng_exporter/logger"
	"database/sql"
	"fmt"
	"reflect"
	"strings"
	"sync"
	"sync/atomic"
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

			// 记录collector开始时间
			startTime := time.Now()

			// 创建采集器实例
			collector := a.createCollector(p.DB)

			// 如果采集器支持数据源感知，设置数据源名称
			SetDataSourceIfSupported(collector, p.Name)

			// 创建标签注入器
			labelInjector := NewLabelInjectorFromPool(p)

			// 简化超时控制 - 安全的实现方式
			timeout := time.Duration(config.Global.GetGlobalTimeoutSeconds()) * time.Second

			// 最终版：防goroutine泄露的安全超时控制
			timedOut := false
			var metricCount int32 = 0 // 使用原子操作避免竞态

			// 创建安全的收集channel
			safeChan := make(chan prometheus.Metric, 500) // 大缓冲防止阻塞
			stopForward := make(chan struct{})
			forwardDone := make(chan struct{})

			// 转发goroutine - 可以被安全终止
			go func() {
				defer close(forwardDone)
				for {
					select {
					case <-stopForward:
						//不再向 Prometheus 输出，但继续读 safeChan 直到它被关闭 → 生产者永远写得进去 → 采集结束自行 close(safeChan) → 转发协程排水完毕自然退出
						for range safeChan {
							// drain and drop
						}
						return
					case metric, ok := <-safeChan:
						if !ok {
							// channel关闭，正常退出
							return
						}
						// 转发指标
						wrappedMetric := NewMetricWrapper(metric, labelInjector)
						select {
						case ch <- wrappedMetric:
							atomic.AddInt32(&metricCount, 1)
						default:
							// 主channel满了，丢弃
						}
					}
				}
			}()

			// 采集goroutine - 可以在后台继续运行
			collectDone := make(chan struct{})
			go func() {
				defer func() {
					if r := recover(); r != nil {
						logger.Logger.Debugf("[%s] Collector panic recovered: %v", p.Name, r)
					}
					close(safeChan) // 安全关闭channel
					close(collectDone)
				}()

				// 执行采集 - 即使超时也能继续完成
				collector.Collect(safeChan)
			}()

			// 超时控制逻辑
			if timeout <= 0 {
				// 无超时限制
				<-collectDone
				<-forwardDone
				timedOut = false
			} else {
				// 带超时控制
				select {
				case <-collectDone:
					// 采集正常完成
					<-forwardDone // 等待转发完成
					timedOut = false
				case <-time.After(timeout):
					// 超时 - 停止转发但让采集器继续
					timedOut = true
					logger.Logger.Warnf("[%s] FORCE TERMINATING %s after %v (timeout=%v)",
						p.Name, a.collectorName, time.Since(startTime), timeout)
					close(stopForward) // 发送停止信号
					<-forwardDone      // 等待转发goroutine退出
				}
			}

			// 记录结果
			collectorDuration := time.Since(startTime)

			if timedOut {
				logger.Logger.Warnf("[%s] %s TIMED OUT | Collector Cost: %vms | Metrics: %d",
					p.Name, a.collectorName, collectorDuration.Milliseconds(), metricCount)
			} else {
				logger.Logger.Infof("[%s] %s completed | Collector Cost: %vms | Metrics: %d",
					p.Name, a.collectorName, collectorDuration.Milliseconds(), metricCount)
			}
		}(pool)
	}

	// 等待所有goroutine完成
	// 注意：由于直接写入ch，快速的数据源会立即返回结果，不会被慢的数据源阻塞
	wg.Wait()
}

// AdaptCollector 适配单个采集器到多数据源
func AdaptCollector(poolManager *db.DBPoolManager, createFunc func(*sql.DB) MetricCollector) MetricCollector {
	// poolManager不能为nil
	if poolManager == nil {
		logger.Logger.Error("DBPoolManager is required")
		return nil
	}

	return NewMultiSourceAdapter(poolManager, createFunc)
}
