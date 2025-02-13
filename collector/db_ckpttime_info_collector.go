package collector

import (
	"context"
	"dameng_exporter/config"
	"dameng_exporter/logger"
	"database/sql"
	"github.com/prometheus/client_golang/prometheus"
	"go.uber.org/zap"
	"strings"
	"time"
)

// 定义数据结构
type CkptInfo struct {
	CkptTotalCount   sql.NullFloat64
	CkptReserveCount sql.NullFloat64
	CkptFlushedPages sql.NullFloat64
	CkptTimeUsed     sql.NullFloat64
}

// 定义收集器结构体
type CkptCollector struct {
	db               *sql.DB
	ckptTimeInfoDesc *prometheus.Desc
	viewExists       bool
}

func NewCkptCollector(db *sql.DB) MetricCollector {
	return &CkptCollector{
		db: db,
		ckptTimeInfoDesc: prometheus.NewDesc(
			dmdbms_ckpttime_info,
			"Information about DM checkpoint times",
			[]string{"host_name" /*, "ckpt_total_count", "ckpt_reserve_count", "ckpt_flushed_pages"*/},
			nil,
		),
		viewExists: true,
	}
}

func (c *CkptCollector) Describe(ch chan<- *prometheus.Desc) {
	ch <- c.ckptTimeInfoDesc
}

func (c *CkptCollector) Collect(ch chan<- prometheus.Metric) {
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
	//不存在则直接返回
	if !c.viewExists {
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(config.GlobalConfig.QueryTimeout)*time.Second)
	defer cancel()

	rows, err := c.db.QueryContext(ctx, config.QueryCheckPointInfoSql)
	if err != nil {
		//if strings.EqualFold(err.Error(), "CKPT") { // 检查视图不存在的特定错误
		if strings.Contains(err.Error(), "v$CKPT") {
			logger.Logger.Warn("v$CKPT view does not exist, skipping future queries", zap.Error(err))
			c.viewExists = false
			return
		}
		handleDbQueryError(err)
		return
	}
	defer rows.Close()

	var ckptInfos []CkptInfo
	for rows.Next() {
		var info CkptInfo
		if err := rows.Scan(&info.CkptTotalCount, &info.CkptReserveCount, &info.CkptFlushedPages, &info.CkptTimeUsed); err != nil {
			logger.Logger.Error("Error scanning row", zap.Error(err))
			continue
		}
		ckptInfos = append(ckptInfos, info)
	}
	if err := rows.Err(); err != nil {
		logger.Logger.Error("Error with rows", zap.Error(err))
		return
	}

	hostname := config.GetHostName()
	// 发送数据到 Prometheus
	for _, info := range ckptInfos {
		//ckptTotalCount := NullFloat64ToString(info.CkptTotalCount)
		//ckptReserveCount := NullFloat64ToString(info.CkptReserveCount)
		//ckptFlushedPages := NullFloat64ToString(info.CkptFlushedPages)

		ch <- prometheus.MustNewConstMetric(
			c.ckptTimeInfoDesc,
			prometheus.GaugeValue,
			NullFloat64ToFloat64(info.CkptTimeUsed),
			hostname, /*, ckptTotalCount, ckptReserveCount, ckptFlushedPages*/
		)
	}
}
