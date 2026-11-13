package collector

import (
	"dameng_exporter/config"
	"dameng_exporter/db"
	"github.com/prometheus/client_golang/prometheus"
)

// DbNodeTypeInfoCollector 暴露数据库节点类型信息
type DbNodeTypeInfoCollector struct {
	poolManager *db.DBPoolManager
	desc        *prometheus.Desc
}

// NewDbNodeTypeInfoCollector 创建节点类型采集器
func NewDbNodeTypeInfoCollector(poolManager *db.DBPoolManager) *DbNodeTypeInfoCollector {
	return &DbNodeTypeInfoCollector{
		poolManager: poolManager,
		desc: prometheus.NewDesc(
			dmdbms_node_type_info,
			"Database node type information, value is always 1",
			[]string{"datasource", "node_type"},
			nil,
		),
	}
}

// Describe 实现 prometheus.Collector 接口
func (c *DbNodeTypeInfoCollector) Describe(ch chan<- *prometheus.Desc) {
	ch <- c.desc
}

// Collect 实现 prometheus.Collector 接口
func (c *DbNodeTypeInfoCollector) Collect(ch chan<- prometheus.Metric) {
	if c == nil || c.poolManager == nil {
		return
	}

	if config.GlobalMultiConfig != nil {
		for i := range config.GlobalMultiConfig.DataSources {
			ds := &config.GlobalMultiConfig.DataSources[i]
			if ds == nil || !ds.Enabled {
				continue
			}

			nodeType := string(db.NodeTypeDefault)
			if pool := c.poolManager.GetPool(ds.Name); pool != nil {
				nodeType = pool.GetNodeType()
			}
			if nodeType == "" {
				nodeType = string(db.NodeTypeDefault)
			}

			datasourceLabel, injector := buildLabelContextFromConfig(ds)

			metric := prometheus.MustNewConstMetric(
				c.desc,
				prometheus.GaugeValue,
				1,
				datasourceLabel,
				nodeType,
			)
			ch <- NewMetricWrapper(metric, injector)
		}
		return
	}

	for _, pool := range c.poolManager.GetHealthyPools() {
		if pool == nil {
			continue
		}
		datasourceLabel, injector := buildLabelContextFromPool(pool)

		nodeType := pool.GetNodeType()
		if nodeType == "" {
			nodeType = string(db.NodeTypeDefault)
		}

		metric := prometheus.MustNewConstMetric(
			c.desc,
			prometheus.GaugeValue,
			1,
			datasourceLabel,
			nodeType,
		)
		ch <- NewMetricWrapper(metric, injector)
	}
}
