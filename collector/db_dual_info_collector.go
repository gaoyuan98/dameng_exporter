package collector

import (
	"context"
	"dameng_exporter/config"
	"dameng_exporter/logger"
	"database/sql"
	"fmt"
	"github.com/prometheus/client_golang/prometheus"
	"go.uber.org/zap"
	"time"
)

const (
	DB_DUAL_FAILUR = 0
)

// 定义收集器结构体
type DbDualInfoCollector struct {
	db           *sql.DB
	dualInfoDesc *prometheus.Desc
	dataSource   string // 数据源名称
}

// SetDataSource 实现DataSourceAware接口
func (c *DbDualInfoCollector) SetDataSource(name string) {
	c.dataSource = name
}

func NewDbDualCollector(db *sql.DB) MetricCollector {
	return &DbDualInfoCollector{
		db: db,
		dualInfoDesc: prometheus.NewDesc(
			dmdbms_dual_info,
			"Information about DM database query dual table info,return false is 0, true is 1",
			[]string{"host_name"},
			nil,
		),
	}
}

func (c *DbDualInfoCollector) Describe(ch chan<- *prometheus.Desc) {
	ch <- c.dualInfoDesc
}

func (c *DbDualInfoCollector) Collect(ch chan<- prometheus.Metric) {

	if err := checkDBConnectionWithSource(c.db, c.dataSource); err != nil {
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(config.Global.GetQueryTimeout())*time.Second)
	defer cancel()

	dualValue := c.QueryDualInfo(ctx)

	hostname := config.GetHostName()
	// 发送数据到 Prometheus
	ch <- prometheus.MustNewConstMetric(
		c.dualInfoDesc,
		prometheus.GaugeValue,
		dualValue,
		hostname,
	)

}

func (c *DbDualInfoCollector) QueryDualInfo(ctx context.Context) float64 {
	var dualValue float64
	rows, err := c.db.QueryContext(ctx, config.QueryDualInfoSql)
	if err != nil {
		handleDbQueryErrorWithSource(err, c.dataSource)
		return DB_DUAL_FAILUR
	}
	defer rows.Close()
	rows.Next()
	err = rows.Scan(&dualValue)
	if err != nil {
		logger.Logger.Error(fmt.Sprintf("[%s] Error scanning dual value", c.dataSource), zap.Error(err))
		return DB_DUAL_FAILUR
	}

	return dualValue
}
