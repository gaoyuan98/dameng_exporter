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

// 定义数据结构
type BufferPoolInfo struct {
	bufferName sql.NullString
	hitRate    sql.NullFloat64
}

// 定义收集器结构体
type DbBufferPoolInfoCollector struct {
	db                 *sql.DB
	bufferPoolInfoDesc *prometheus.Desc
}

func NewDbBufferPoolCollector(db *sql.DB) MetricCollector {
	return &DbBufferPoolInfoCollector{
		db: db,
		bufferPoolInfoDesc: prometheus.NewDesc(
			dmdbms_bufferpool_info,
			"Information about DM database bufferpool return hitRate",
			[]string{"host_name", "buffer_name"},
			nil,
		),
	}
}

func (c *DbBufferPoolInfoCollector) Describe(ch chan<- *prometheus.Desc) {
	ch <- c.bufferPoolInfoDesc
}

func (c *DbBufferPoolInfoCollector) Collect(ch chan<- prometheus.Metric) {

	if err := c.db.Ping(); err != nil {
		logger.Logger.Error("Database connection is not available: %v", zap.Error(err))
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(config.Global.GetQueryTimeout())*time.Second)
	defer cancel()

	rows, err := c.db.QueryContext(ctx, config.QueryBufferPoolHitRateInfoSql)
	if err != nil {
		handleDbQueryError(err)
		return
	}
	defer rows.Close()

	var bufferPoolInfos []BufferPoolInfo
	for rows.Next() {
		var info BufferPoolInfo
		if err := rows.Scan(&info.bufferName, &info.hitRate); err != nil {
			logger.Logger.Error("Error scanning row", zap.Error(err))
			continue
		}
		bufferPoolInfos = append(bufferPoolInfos, info)
	}

	if err := rows.Err(); err != nil {
		logger.Logger.Error("Error with rows", zap.Error(err))
	}

	hostname := config.GetHostName()
	// 发送数据到 Prometheus
	for _, info := range bufferPoolInfos {

		bufferName := NullStringToString(info.bufferName)
		hitRate := NullFloat64ToFloat64(info.hitRate)
		ch <- prometheus.MustNewConstMetric(
			c.bufferPoolInfoDesc,
			prometheus.GaugeValue,
			hitRate,
			hostname, bufferName,
		)
	}
}
