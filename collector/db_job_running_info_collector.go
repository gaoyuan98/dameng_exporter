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

type DbJobRunningInfoCollector struct {
	db              *sql.DB
	jobErrorNumDesc *prometheus.Desc
}

// 定义存储查询结果的结构体
type ErrorCountInfo struct {
	ErrorNum sql.NullInt64
}

func NewDbJobRunningInfoCollector(db *sql.DB) MetricCollector {
	return &DbJobRunningInfoCollector{
		db: db,
		jobErrorNumDesc: prometheus.NewDesc(
			dmdbms_joblog_error_num,
			"dmdbms_joblog_error_num info information",
			[]string{"host_name"}, // 添加标签
			nil,
		),
	}
}

func (c *DbJobRunningInfoCollector) Describe(ch chan<- *prometheus.Desc) {
	ch <- c.jobErrorNumDesc
}

func (c *DbJobRunningInfoCollector) Collect(ch chan<- prometheus.Metric) {
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

	rows, err := c.db.QueryContext(ctx, config.QueryDbJobRunningInfoSqlStr)
	if err != nil {
		// 检查报错信息中是否包含 "v$dmmonitor" 字符串
		if strings.Contains(err.Error(), "SYSJOB") {
			logger.Logger.Warn("数据库未开启定时任务功能，无法检查错误任务异常数量。请执行sql语句call SP_INIT_JOB_SYS(1); 开启定时作业的功能。（该报错不影响其他指标采集数据,也可忽略）")
			return
		}
		handleDbQueryError(err)
		return
	}
	defer rows.Close()

	// 存储查询结果
	var errorCountInfo ErrorCountInfo
	if rows.Next() {
		if err := rows.Scan(&errorCountInfo.ErrorNum); err != nil {
			logger.Logger.Error("Error scanning row", zap.Error(err))
			return
		}
	}

	if err := rows.Err(); err != nil {
		logger.Logger.Error("Error with rows", zap.Error(err))
	}
	// 发送数据到 Prometheus

	ch <- prometheus.MustNewConstMetric(c.jobErrorNumDesc, prometheus.GaugeValue, NullInt64ToFloat64(errorCountInfo.ErrorNum), config.GetHostName())

}
