package collector

import (
	"context"
	"dameng_exporter/config"
	"dameng_exporter/logger"
	"database/sql"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"go.uber.org/zap"
)

// 定义数据结构
type RapplyTimeDiff struct {
	TimeDiff sql.NullFloat64
}

// 定义收集器结构体
type DbRapplyTimeDiffCollector struct {
	db           *sql.DB
	timeDiffDesc *prometheus.Desc
}

func NewDbRapplyTimeDiffCollector(db *sql.DB) MetricCollector {
	return &DbRapplyTimeDiffCollector{
		db: db,
		timeDiffDesc: prometheus.NewDesc(
			dmdbms_rapply_time_diff,
			"Time difference in seconds between APPLY_CMT_TIME and LAST_CMT_TIME from V$RAPPLY_STAT",
			[]string{"host_name"},
			nil,
		),
	}
}

func (c *DbRapplyTimeDiffCollector) Describe(ch chan<- *prometheus.Desc) {
	ch <- c.timeDiffDesc
}

func (c *DbRapplyTimeDiffCollector) Collect(ch chan<- prometheus.Metric) {

	if err := c.db.Ping(); err != nil {
		logger.Logger.Error("Database connection is not available", zap.Error(err))
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(config.Global.GetQueryTimeout())*time.Second)
	defer cancel()

	// 执行查询
	rows, err := c.db.QueryContext(ctx, config.QueryRapplyTimeDiffSql)
	if err != nil {
		handleDbQueryError(err)
		return
	}
	defer rows.Close()

	var rapplyTimeDiffs []RapplyTimeDiff
	for rows.Next() {
		var info RapplyTimeDiff
		if err := rows.Scan(&info.TimeDiff); err != nil {
			logger.Logger.Error("Error scanning row", zap.Error(err))
			continue
		}
		rapplyTimeDiffs = append(rapplyTimeDiffs, info)
	}
	if err := rows.Err(); err != nil {
		logger.Logger.Error("Error with rows", zap.Error(err))
		return
	}

	hostname := config.GetHostName()
	for _, info := range rapplyTimeDiffs {
		ch <- prometheus.MustNewConstMetric(
			c.timeDiffDesc,
			prometheus.GaugeValue,
			NullFloat64ToFloat64(info.TimeDiff),
			hostname,
		)
	}
}
