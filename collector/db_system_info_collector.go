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

// DBSystemInfoCollector 结构体
type DBSystemInfoCollector struct {
	db         *sql.DB
	cpuDesc    *prometheus.Desc
	memoryDesc *prometheus.Desc
}

// DBSystemInfo 结构体
type DBSystemInfo struct {
	cpuCount   sql.NullFloat64
	memorySize sql.NullFloat64
}

// NewDBSystemInfoCollector 函数
func NewDBSystemInfoCollector(db *sql.DB) MetricCollector {
	return &DBSystemInfoCollector{
		db: db,
		cpuDesc: prometheus.NewDesc(
			dmdbms_system_cpu_info,
			"Number of CPU cores",
			[]string{"host_name"},
			nil,
		),
		memoryDesc: prometheus.NewDesc(
			dmdbms_system_memory_info,
			"Total physical memory size in bytes",
			[]string{"host_name"},
			nil,
		),
	}
}

// Describe 方法
func (c *DBSystemInfoCollector) Describe(ch chan<- *prometheus.Desc) {
	ch <- c.cpuDesc
	ch <- c.memoryDesc
}

// Collect 方法
func (c *DBSystemInfoCollector) Collect(ch chan<- prometheus.Metric) {
	funcStart := time.Now()
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

	rows, err := c.db.QueryContext(ctx, config.QuerySystemInfoSqlStr)
	if err != nil {
		handleDbQueryError(err)
		return
	}
	defer rows.Close()

	var info DBSystemInfo
	if rows.Next() {
		if err := rows.Scan(&info.cpuCount, &info.memorySize); err != nil {
			logger.Logger.Error("Error scanning row", zap.Error(err))
			return
		}
	}

	if err := rows.Err(); err != nil {
		logger.Logger.Error("Error with rows", zap.Error(err))
		return
	}

	// 发送CPU信息到Prometheus
	ch <- prometheus.MustNewConstMetric(
		c.cpuDesc,
		prometheus.GaugeValue,
		NullFloat64ToFloat64(info.cpuCount),
		config.GetHostName(),
	)

	// 发送内存信息到Prometheus
	ch <- prometheus.MustNewConstMetric(
		c.memoryDesc,
		prometheus.GaugeValue,
		NullFloat64ToFloat64(info.memorySize),
		config.GetHostName(),
	)
}
