package collector

import (
	"context"
	"dameng_exporter/config"
	"dameng_exporter/db"
	"dameng_exporter/logger"
	"sync"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"go.uber.org/zap"
)

// CollectFunc 采集函数类型
type CollectFunc func(ctx context.Context, pool *db.DataSourcePool, ch chan<- prometheus.Metric) error

// BaseMultiSourceCollector 多数据源采集器基类
type BaseMultiSourceCollector struct {
	poolManager  *db.DBPoolManager      // 连接池管理器
	strategy     config.CollectStrategy // 采集策略
	timeout      time.Duration          // 超时时间
	logger       *zap.SugaredLogger     // 日志记录器
	collectFunc  CollectFunc            // 采集函数
	descriptions []*prometheus.Desc     // 指标描述
	name         string                 // 采集器名称
}

// NewBaseMultiSourceCollector 创建多数据源采集器基类
func NewBaseMultiSourceCollector(
	poolManager *db.DBPoolManager,
	name string,
	collectFunc CollectFunc,
	descriptions []*prometheus.Desc,
) *BaseMultiSourceCollector {
	return &BaseMultiSourceCollector{
		poolManager:  poolManager,
		strategy:     config.CollectStrategy(poolManager.GetStatus()["strategy"].(string)),
		timeout:      30 * time.Second,
		logger:       logger.Logger,
		collectFunc:  collectFunc,
		descriptions: descriptions,
		name:         name,
	}
}

// Describe 实现Prometheus Collector接口
func (c *BaseMultiSourceCollector) Describe(ch chan<- *prometheus.Desc) {
	for _, desc := range c.descriptions {
		ch <- desc
	}
}

// Collect 实现Prometheus Collector接口
func (c *BaseMultiSourceCollector) Collect(ch chan<- prometheus.Metric) {
	switch c.strategy {
	case config.StrategySequential:
		c.collectSequential(ch)
	case config.StrategyConcurrent:
		c.collectConcurrent(ch)
	case config.StrategyHybrid:
		c.collectHybrid(ch)
	default:
		c.collectSequential(ch) // 默认使用串行
	}
}

// collectSequential 串行采集
func (c *BaseMultiSourceCollector) collectSequential(ch chan<- prometheus.Metric) {
	pools := c.poolManager.GetHealthyPools()

	for _, pool := range pools {
		c.collectFromPool(pool, ch)
	}
}

// collectConcurrent 并发采集
func (c *BaseMultiSourceCollector) collectConcurrent(ch chan<- prometheus.Metric) {
	pools := c.poolManager.GetHealthyPools()

	var wg sync.WaitGroup
	// 使用带缓冲的channel避免阻塞
	metricChan := make(chan prometheus.Metric, 1000)

	// 启动goroutine收集指标
	for _, pool := range pools {
		wg.Add(1)
		go func(p *db.DataSourcePool) {
			defer wg.Done()
			c.collectFromPoolToChan(p, metricChan)
		}(pool)
	}

	// 等待所有采集完成后关闭channel
	go func() {
		wg.Wait()
		close(metricChan)
	}()

	// 将收集到的指标发送到Prometheus
	for metric := range metricChan {
		ch <- metric
	}
}

// collectHybrid 混合采集（按优先级）
func (c *BaseMultiSourceCollector) collectHybrid(ch chan<- prometheus.Metric) {
	// 按优先级分组处理
	for priority := 1; priority <= 3; priority++ {
		pools := c.poolManager.GetPoolsByPriority(priority)
		if len(pools) == 0 {
			continue
		}

		if priority == 1 {
			// 高优先级串行采集
			for _, pool := range pools {
				c.collectFromPool(pool, ch)
			}
		} else {
			// 中低优先级并发采集
			var wg sync.WaitGroup
			metricChan := make(chan prometheus.Metric, 1000)

			for _, pool := range pools {
				wg.Add(1)
				go func(p *db.DataSourcePool) {
					defer wg.Done()
					c.collectFromPoolToChan(p, metricChan)
				}(pool)
			}

			go func() {
				wg.Wait()
				close(metricChan)
			}()

			for metric := range metricChan {
				ch <- metric
			}
		}
	}
}

// collectFromPool 从单个连接池采集指标
func (c *BaseMultiSourceCollector) collectFromPool(pool *db.DataSourcePool, ch chan<- prometheus.Metric) {
	ctx, cancel := context.WithTimeout(context.Background(), c.timeout)
	defer cancel()

	if err := c.collectFunc(ctx, pool, ch); err != nil {
		c.logger.Error("Failed to collect metrics",
			zap.String("collector", c.name),
			zap.String("datasource", pool.Name),
			zap.Error(err))
	}
}

// collectFromPoolToChan 从单个连接池采集指标到channel
func (c *BaseMultiSourceCollector) collectFromPoolToChan(pool *db.DataSourcePool, metricChan chan<- prometheus.Metric) {
	// 创建临时channel收集指标
	tempChan := make(chan prometheus.Metric, 100)
	done := make(chan bool)

	// 启动goroutine将指标转发到主channel
	go func() {
		for metric := range tempChan {
			metricChan <- metric
		}
		done <- true
	}()

	// 执行采集
	ctx, cancel := context.WithTimeout(context.Background(), c.timeout)
	defer cancel()

	if err := c.collectFunc(ctx, pool, tempChan); err != nil {
		c.logger.Error("Failed to collect metrics",
			zap.String("collector", c.name),
			zap.String("datasource", pool.Name),
			zap.Error(err))
	}

	// 关闭临时channel并等待转发完成
	close(tempChan)
	<-done
}

// SetTimeout 设置超时时间
func (c *BaseMultiSourceCollector) SetTimeout(timeout time.Duration) {
	c.timeout = timeout
}

// SetStrategy 设置采集策略
func (c *BaseMultiSourceCollector) SetStrategy(strategy config.CollectStrategy) {
	c.strategy = strategy
}

// MultiSourceCollectorAdapter 多数据源采集器适配器（用于兼容旧采集器）
type MultiSourceCollectorAdapter struct {
	poolManager     *db.DBPoolManager
	legacyCollector MetricCollector // 旧的采集器接口
	logger          *zap.SugaredLogger
}

// NewMultiSourceCollectorAdapter 创建适配器
func NewMultiSourceCollectorAdapter(poolManager *db.DBPoolManager, legacyCollector MetricCollector) *MultiSourceCollectorAdapter {
	return &MultiSourceCollectorAdapter{
		poolManager:     poolManager,
		legacyCollector: legacyCollector,
		logger:          logger.Logger,
	}
}

// Describe 实现Prometheus Collector接口
func (a *MultiSourceCollectorAdapter) Describe(ch chan<- *prometheus.Desc) {
	a.legacyCollector.Describe(ch)
}

// Collect 实现Prometheus Collector接口
func (a *MultiSourceCollectorAdapter) Collect(ch chan<- prometheus.Metric) {
	// 获取所有健康的连接池
	pools := a.poolManager.GetHealthyPools()

	// 如果只有一个连接池，直接使用旧采集器
	if len(pools) == 1 {
		// 临时设置全局DBPool（用于兼容）
		oldPool := db.DBPool
		db.DBPool = pools[0].DB

		// 调用旧采集器
		a.legacyCollector.Collect(ch)

		// 恢复原始DBPool
		db.DBPool = oldPool
		return
	}

	// 多个连接池的情况，逐个采集
	for _, pool := range pools {
		// 临时设置全局DBPool（用于兼容）
		oldPool := db.DBPool
		db.DBPool = pool.DB

		// 调用旧采集器
		a.legacyCollector.Collect(ch)

		// 恢复原始DBPool
		db.DBPool = oldPool
	}
}
