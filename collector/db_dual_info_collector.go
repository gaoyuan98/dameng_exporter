package collector

import (
	"context"
	"dameng_exporter/config"
	"dameng_exporter/logger"
	"database/sql"
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
	funcStart := time.Now()
	// 时间间隔的计算发生在 defer 语句执行时，确保能够获取到正确的函数执行时间。
	defer func() {
		duration := time.Since(funcStart)
		logger.Logger.Debugf("func exec time：%vms", duration.Milliseconds())
	}()

	if err := c.db.Ping(); err != nil {
		logger.Logger.Error("Database connection is not available: %v", zap.Error(err))
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(config.GlobalConfig.QueryTimeout)*time.Second)
	defer cancel()

	dualValue := QueryDualInfo(ctx, c.db)

	hostname := config.GetHostName()
	// 发送数据到 Prometheus
	ch <- prometheus.MustNewConstMetric(
		c.dualInfoDesc,
		prometheus.GaugeValue,
		dualValue,
		hostname,
	)

}

func QueryDualInfo(ctx context.Context, db *sql.DB) float64 {
	var dualValue float64
	rows, err := db.QueryContext(ctx, config.QueryDualInfoSql)
	if err != nil {
		handleDbQueryError(err)
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
