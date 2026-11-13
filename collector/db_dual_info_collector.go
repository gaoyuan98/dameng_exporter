package collector

import (
	"context"
	"dameng_exporter/config"
	"dameng_exporter/diagnostics"
	"dameng_exporter/utils"
	"database/sql"
	"github.com/prometheus/client_golang/prometheus"
	"time"
)

const (
	DB_DUAL_FAILUR = 0
)

// 定义收集器结构体
type DbDualInfoCollector struct {
	collectorLog
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
			[]string{},
			nil,
		),
	}
}

func (c *DbDualInfoCollector) Describe(ch chan<- *prometheus.Desc) {
	ch <- c.dualInfoDesc
}

func (c *DbDualInfoCollector) Collect(ch chan<- prometheus.Metric) {

	if err := utils.CheckDBConnectionWithSource(c.db, c.dataSource, c.log()); err != nil {
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(config.Global.GetQueryTimeout())*time.Second)
	defer cancel()
	ctx = diagnostics.WithQueryOptions(ctx, diagnostics.QueryOptions{ReadScope: "first_row", AllowEmpty: true})

	dualValue := c.QueryDualInfo(ctx)

	// 发送数据到 Prometheus
	ch <- prometheus.MustNewConstMetric(
		c.dualInfoDesc,
		prometheus.GaugeValue,
		dualValue,
	)

}

func (c *DbDualInfoCollector) QueryDualInfo(ctx context.Context) float64 {
	var dualValue float64
	rows, err := c.database(c.db).QueryContext(ctx, config.QueryDualInfoSql)
	if err != nil {
		utils.HandleDbQueryErrorWithSource(err, c.dataSource, c.trace, c.log())
		return DB_DUAL_FAILUR
	}
	defer rows.Close()
	rows.Next()
	err = rows.Scan(&dualValue)
	if err != nil {
		return DB_DUAL_FAILUR
	}

	return dualValue
}
