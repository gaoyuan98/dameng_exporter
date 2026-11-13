package collector

import (
	"dameng_exporter/config"
	"dameng_exporter/db"
	"dameng_exporter/diagnostics"
	"dameng_exporter/logger"
	"dameng_exporter/utils"
	"database/sql"
	"fmt"
	"github.com/prometheus/client_golang/prometheus"
	"go.uber.org/zap"
	"reflect"
	"strings"
	"sync"
	"time"
)

// DataSourceAware 接口，用于标识支持数据源感知的采集器
type DataSourceAware interface {
	SetDataSource(name string)
}

// DataSourceConfigAware 接口，用于向采集器注入当前数据源配置
type DataSourceConfigAware interface {
	SetDataSourceConfig(dataSourceConfig *config.DataSourceConfig)
}

// NodeTypeAware 接口，用于标识支持数据库节点类型感知的采集器
type NodeTypeAware interface {
	SetNodeType(nodeType string)
}

// SetDataSourceIfSupported 用于设置采集器的数据源名称
func SetDataSourceIfSupported(collector MetricCollector, dataSource string) {
	if dsa, ok := collector.(DataSourceAware); ok {
		dsa.SetDataSource(dataSource)
	}
}

// SetDataSourceConfigIfSupported 用于设置采集器的数据源配置
func SetDataSourceConfigIfSupported(collector MetricCollector, dataSourceConfig *config.DataSourceConfig) {
	if dsca, ok := collector.(DataSourceConfigAware); ok {
		dsca.SetDataSourceConfig(dataSourceConfig)
	}
}

// SetNodeTypeIfSupported 用于设置采集器的数据库节点类型
func SetNodeTypeIfSupported(collector MetricCollector, nodeType string) {
	if nta, ok := collector.(NodeTypeAware); ok {
		nta.SetNodeType(nodeType)
	}
}

// MultiSourceAdapter 多数据源适配器，用于快速改造现有采集器
type MultiSourceAdapter struct {
	poolManager     *db.DBPoolManager
	createCollector func(*sql.DB) MetricCollector
}

// NewMultiSourceAdapter 创建多数据源适配器
func NewMultiSourceAdapter(poolManager *db.DBPoolManager, createFunc func(*sql.DB) MetricCollector) *MultiSourceAdapter {
	return &MultiSourceAdapter{
		poolManager:     poolManager,
		createCollector: createFunc,
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

// Collect supports standalone registration with its own round summary.
func (a *MultiSourceAdapter) Collect(ch chan<- prometheus.Metric) {
	round := newCollectionRound()
	a.collectWithRound(ch, round)
	round.finish(nil)
}

func (a *MultiSourceAdapter) collectWithRound(ch chan<- prometheus.Metric, round *collectionRound) {
	pools := a.poolManager.GetHealthyPools()
	var wg sync.WaitGroup
	for _, pool := range pools {
		wg.Add(1)
		go func(p *db.DataSourcePool) {
			defer wg.Done()
			collector := a.createCollector(p.DB)
			SetDataSourceIfSupported(collector, p.Name)
			SetDataSourceConfigIfSupported(collector, p.Config)
			SetNodeTypeIfSupported(collector, p.GetNodeType())
			fast := config.GlobalMultiConfig != nil && config.GlobalMultiConfig.IsFastMode()
			collectWithLogging(ch, p, collector, round, fast, time.Duration(config.Global.GetGlobalTimeoutSeconds())*time.Second)
		}(pool)
	}
	wg.Wait()
}

// collectWithLogging measures the whole collector, including connection waits,
// SQL, row processing and forwarding. It does not claim to measure SQL alone.
func collectWithLogging(ch chan<- prometheus.Metric, p *db.DataSourcePool, collector MetricCollector, round *collectionRound, fast bool, timeout time.Duration) {
	if round == nil {
		round = newCollectionRound()
		defer round.finish(nil)
	}
	mode := "blocking"
	if fast {
		mode = "fast"
	}
	if !fast {
		timeout = 0
	}
	sourceLabel, injector := buildLabelContextFromPool(p)
	run := newCollectionRun(round, "database", p.Name, getCollectorName(collector), mode, timeout, sourceLabel)
	if aware, ok := collector.(interface{ SetCollectionLogger(*zap.SugaredLogger) }); ok {
		aware.SetCollectionLogger(run.base)
	}
	if aware, ok := collector.(interface{ SetRunTrace(*diagnostics.RunTrace) }); ok {
		aware.SetRunTrace(run.trace)
	}
	if err := utils.CheckDBConnectionWithSource(p.DB, p.Name, run.log); err != nil {
		run.trace.Skip(err.Error())
		return
	}
	buffer := 10
	if fast {
		buffer = 500
	}
	safeChan := make(chan prometheus.Metric, buffer)
	stopForward, forwardDone := make(chan struct{}), make(chan struct{})
	// Only the forwarder writes these counters. Reading after forwardDone is safe.
	var samples, dropped int64
	go func() {
		restore := run.trace.Stage("forwarder", "metric_forward")
		defer restore()
		for {
			select {
			case <-stopForward:
				close(forwardDone)
				for range safeChan {
				} // allow late workers to finish without touching Registry
				return
			case metric, ok := <-safeChan:
				if !ok {
					close(forwardDone)
					return
				}
				// Prefer an already-requested stop, even with a continuously ready worker.
				select {
				case <-stopForward:
					close(forwardDone)
					for range safeChan {
					}
					return
				default:
				}
				wrapped := NewMetricWrapper(metric, injector)
				if fast {
					select {
					case ch <- wrapped:
						samples++
					default:
						dropped++
					}
				} else {
					ch <- wrapped
					samples++
				}
			}
		}
	}()
	done := make(chan workerResult, 1)
	// Keep the existing worker-stage budget boundary (preflight is excluded).
	var timer *time.Timer
	if fast && timeout > 0 {
		timer = time.NewTimer(timeout)
		defer timer.Stop()
	}
	go executeCollector(collector, safeChan, done, run)
	var deadline <-chan time.Time
	if timer != nil {
		deadline = timer.C
	}
	result, timedOut := waitForWorker(done, deadline)
	if timedOut {
		run.trace.Detach()
		close(stopForward)
		result = workerResult{execution: "timeout"}
	}
	<-forwardDone
	run.finish(round, samples, dropped, result)
	if timedOut {
		go func() {
			<-done
			run.trace.BackgroundFinished()
		}()
	}
}

// A nil deadline preserves blocking behavior. A ready completion takes priority
// when the budget and the result are observed together.
func waitForWorker(done <-chan workerResult, deadline <-chan time.Time) (workerResult, bool) {
	select {
	case result := <-done:
		return result, false
	case <-deadline:
		select {
		case result := <-done:
			return result, false
		default:
			return workerResult{}, true
		}
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
