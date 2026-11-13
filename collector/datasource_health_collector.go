package collector

import (
	"dameng_exporter/config"
	"dameng_exporter/db"
	"github.com/prometheus/client_golang/prometheus"
)

// DatasourceHealthCollector 用于暴露每个数据源的健康状态
type DatasourceHealthCollector struct {
	poolManager *db.DBPoolManager
	desc        *prometheus.Desc
}

// NewDatasourceHealthCollector 创建新的数据源状态采集器
func NewDatasourceHealthCollector(poolManager *db.DBPoolManager) *DatasourceHealthCollector {
	return &DatasourceHealthCollector{
		poolManager: poolManager,
		desc: prometheus.NewDesc(
			dmdb_up,
			"Current health status of the data source, 1 indicates normal, 0 indicates unavailable",
			[]string{"datasource"},
			nil,
		),
	}
}

// Describe 实现 prometheus.Collector 接口
func (c *DatasourceHealthCollector) Describe(ch chan<- *prometheus.Desc) {
	ch <- c.desc
}

// Collect 实现 prometheus.Collector 接口
func (c *DatasourceHealthCollector) Collect(ch chan<- prometheus.Metric) {
	if c == nil || c.poolManager == nil {
		return
	}

	if config.GlobalMultiConfig != nil {
		for i := range config.GlobalMultiConfig.DataSources {
			ds := &config.GlobalMultiConfig.DataSources[i]
			if ds == nil || !ds.Enabled {
				continue
			}

			value := 0.0
			status := c.poolManager.GetDatasourceHealthStatus(ds.Name)
			if status.Healthy {
				value = 1.0
			}

			datasourceLabel, injector := buildLabelContextFromConfig(ds)

			metric := prometheus.MustNewConstMetric(
				c.desc,
				prometheus.GaugeValue,
				value,
				datasourceLabel,
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
		metric := prometheus.MustNewConstMetric(
			c.desc,
			prometheus.GaugeValue,
			1,
			datasourceLabel,
		)
		ch <- NewMetricWrapper(metric, injector)
	}
}
