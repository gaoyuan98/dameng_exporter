package collector

import (
	"context"
	"dameng_exporter/config"
	"dameng_exporter/logger"
	"database/sql"
	"github.com/prometheus/client_golang/prometheus"
)

type DBSessionsCollector struct {
	db         *sql.DB
	metricDesc *prometheus.Desc
}

func NewDBSessionsCollector(db *sql.DB) MetricCollector {
	return &DBSessionsCollector{
		db: db,
		metricDesc: prometheus.NewDesc(
			"db_sessions",
			"Number of database sessions",
			[]string{"host_name"}, // 添加标签
			nil,
		),
	}
}

func (c *DBSessionsCollector) Describe(ch chan<- *prometheus.Desc) {
	ch <- c.metricDesc
}

func (c *DBSessionsCollector) Collect(ch chan<- prometheus.Metric) {
	// ping 一下判断连接是否有问题
	if err := checkDBConnection(c.db); err != nil {
		return
	}
	//设置超时时间的ctx对象
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	var value float64
	err := c.db.QueryRowContext(ctx, config.QueryDBSessionsSqlStr).Scan(&value)
	if err != nil {
		handleDbQueryError(err)
		return
	}

	logger.Logger.Debugf("DBSessionsCollector: %v", value)
	ch <- prometheus.MustNewConstMetric(c.metricDesc, prometheus.GaugeValue, value, config.GetHostName())
}
