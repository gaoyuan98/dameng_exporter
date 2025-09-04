package collector

import (
	"dameng_exporter/config"
	"dameng_exporter/db"
	"dameng_exporter/logger"
	"database/sql"
	"fmt"
	"reflect"
	"runtime/debug"
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
	collectorName   string // 采集器名称（延迟初始化）
	mu              sync.Mutex
	nameOnce        sync.Once // 确保名称只获取一次
}

// NewMultiSourceAdapter 创建多数据源适配器
func NewMultiSourceAdapter(poolManager *db.DBPoolManager, createFunc func(*sql.DB) MetricCollector) *MultiSourceAdapter {
	return &MultiSourceAdapter{
		poolManager:     poolManager,
		createCollector: createFunc,
		collectorName:   "UnknownCollector", // 默认名称，将在首次使用时更新
	}
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

	// 为每个数据源采集指标
	var wg sync.WaitGroup

	for _, pool := range pools {
		wg.Add(1)
		go func(p *db.DataSourcePool) {
			defer wg.Done()

			// 记录collector开始时间
			startTime := time.Now()

			// 创建采集器实例
			collector := a.createCollector(p.DB)

			// 延迟初始化采集器名称（只执行一次）
			a.nameOnce.Do(func() {
				a.collectorName = getCollectorName(collector)
			})

			// 如果采集器支持数据源感知，设置数据源名称
			SetDataSourceIfSupported(collector, p.Name)

			// 创建标签注入器
			labelInjector := NewLabelInjectorFromPool(p)

			// 根据配置选择采集模式
			if config.GlobalMultiConfig != nil && config.GlobalMultiConfig.IsFastMode() {
				// 快速模式：超时返回部分数据
				a.collectInFastMode(ch, p, collector, labelInjector, startTime)
			} else {
				// 默认阻塞模式：不丢失任何指标
				a.collectInBlockingMode(ch, p, collector, labelInjector, startTime)
			}
		}(pool)
	}

	// 等待所有goroutine完成
	wg.Wait()
}

// collectInBlockingMode 阻塞模式采集 - 不丢失任何指标
func (a *MultiSourceAdapter) collectInBlockingMode(ch chan<- prometheus.Metric, p *db.DataSourcePool, collector MetricCollector, labelInjector *LabelInjector, startTime time.Time) {
	var metricCount int32 = 0
	timeout := time.Duration(config.Global.GetGlobalTimeoutSeconds()) * time.Second

	// 小缓冲通道，仅用于解耦采集和标签注入
	safeChan := make(chan prometheus.Metric, 10)

	// 转发goroutine - 阻塞写入，不丢弃任何指标
	forwardDone := make(chan struct{})
	go func() {
		defer close(forwardDone)
		for metric := range safeChan {
			wrappedMetric := NewMetricWrapper(metric, labelInjector)
			ch <- wrappedMetric // 阻塞写入，确保所有指标都被接收
			atomic.AddInt32(&metricCount, 1)
		}
	}()

	// 采集goroutine
	collectDone := make(chan struct{})
	go func() {
		defer func() {
			if r := recover(); r != nil {
				logger.Logger.Errorf("[%s] Collector panic recovered: %v\nStack trace:\n%s",
					p.Name, r, debug.Stack())
			}
			close(safeChan)
			close(collectDone)
		}()
		collector.Collect(safeChan)
	}()

	// 超时仅用于日志记录，不中断采集
	slowCollector := false
	if timeout > 0 {
		select {
		case <-collectDone:
			slowCollector = false
		case <-time.After(timeout):
			slowCollector = true
			//logger.Logger.Warnf("[%s] %s SLOW (blocking mode) | Exceeded timeout %v, still collecting...",
			//	p.Name, a.collectorName, timeout)
		}
	}

	// 如果是慢采集器，继续等待它完成
	if slowCollector {
		<-collectDone
	}
	<-forwardDone

	// 记录结果
	collectorDuration := time.Since(startTime)
	finalCount := atomic.LoadInt32(&metricCount)

	if slowCollector {
		logger.Logger.Warnf("[%s] %s completed (slow, blocking mode) | Cost: %vms | Metrics: %d",
			p.Name, a.collectorName, collectorDuration.Milliseconds(), finalCount)
	} else {
		logger.Logger.Infof("[%s] %s completed (blocking mode) | Cost: %vms | Metrics: %d",
			p.Name, a.collectorName, collectorDuration.Milliseconds(), finalCount)
	}
}

// collectInFastMode 快速模式采集 - 超时返回部分数据
func (a *MultiSourceAdapter) collectInFastMode(ch chan<- prometheus.Metric, p *db.DataSourcePool, collector MetricCollector, labelInjector *LabelInjector, startTime time.Time) {
	var metricCount int32 = 0
	timeout := time.Duration(config.Global.GetGlobalTimeoutSeconds()) * time.Second

	// 大缓冲防止阻塞
	safeChan := make(chan prometheus.Metric, 500)
	stopForward := make(chan struct{})
	forwardDone := make(chan struct{})
	timedOut := false

	// 转发goroutine - 可以被安全终止
	go func() {
		defer close(forwardDone)
		for {
			select {
			case <-stopForward:
				// 排水阶段：继续读取但不转发
				for range safeChan {
					// drain and drop
				}
				return
			case metric, ok := <-safeChan:
				if !ok {
					return
				}
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

	// 采集goroutine
	collectDone := make(chan struct{})
	go func() {
		defer func() {
			if r := recover(); r != nil {
				logger.Logger.Errorf("[%s] Collector panic recovered: %v\nStack trace:\n%s",
					p.Name, r, debug.Stack())
			}
			close(safeChan)
			close(collectDone)
		}()
		collector.Collect(safeChan)
	}()

	// 超时控制 - 会真正中断数据转发
	if timeout <= 0 {
		<-collectDone
		<-forwardDone
		timedOut = false
	} else {
		select {
		case <-collectDone:
			<-forwardDone
			timedOut = false
		case <-time.After(timeout):
			timedOut = true
			close(stopForward) // 停止转发
			// 快速模式：不等待清理，让转发goroutine在后台自行完成
		}
	}

	// 记录结果
	collectorDuration := time.Since(startTime)
	finalCount := atomic.LoadInt32(&metricCount)

	if timedOut {
		logger.Logger.Warnf("[%s] %s TIMEOUT (fast mode) | Cost: %vms | Timeout: %v | Metrics: %d (partial)",
			p.Name, a.collectorName, collectorDuration.Milliseconds(), timeout, finalCount)
	} else {
		logger.Logger.Infof("[%s] %s completed (fast mode) | Cost: %vms | Metrics: %d",
			p.Name, a.collectorName, collectorDuration.Milliseconds(), finalCount)
	}
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
