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

type DbMemoryPoolInfoCollector struct {
	db            *sql.DB
	totalPoolDesc *prometheus.Desc
	currPoolDesc  *prometheus.Desc
}

type MemoryPoolInfo struct {
	ZoneType sql.NullString
	CurrVal  sql.NullFloat64
	ResVal   sql.NullFloat64
	TotalVal sql.NullFloat64
}

func NewDbMemoryPoolInfoCollector(db *sql.DB) MetricCollector {
	return &DbMemoryPoolInfoCollector{
		db: db,
		totalPoolDesc: prometheus.NewDesc(
			dmdbms_memory_total_pool_info,
			"mem total pool info information",
			[]string{"host_name", "pool_type"}, // 添加标签
			nil,
		),
		currPoolDesc: prometheus.NewDesc(
			dmdbms_memory_curr_pool_info,
			"mem curr pool info information",
			[]string{"host_name", "pool_type"}, // 添加标签
			nil,
		),
	}
}

func (c *DbMemoryPoolInfoCollector) Describe(ch chan<- *prometheus.Desc) {
	ch <- c.totalPoolDesc
	ch <- c.currPoolDesc
}

func (c *DbMemoryPoolInfoCollector) Collect(ch chan<- prometheus.Metric) {
	funcStart := time.Now()
	// 时间间隔的计算发生在 defer 语句执行时，确保能够获取到正确的函数执行时间。
	defer func() {
		duration := time.Since(funcStart)
		logger.Logger.Debugf("func exec time：%vms", duration.Milliseconds())
	}()

	//保存全局结果对象
	var memoryPoolInfos []MemoryPoolInfo

	if err := c.db.Ping(); err != nil {
		logger.Logger.Error("Database connection is not available: %v", zap.Error(err))
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(config.GlobalConfig.QueryTimeout)*time.Second)
	defer cancel()

	rows, err := c.db.QueryContext(ctx, config.QueryMemoryPoolInfoSqlStr)
	if err != nil {
		handleDbQueryError(err)
		return
	}
	defer rows.Close()

	for rows.Next() {
		var info MemoryPoolInfo
		if err := rows.Scan(&info.ZoneType, &info.CurrVal, &info.ResVal, &info.TotalVal); err != nil {
			logger.Logger.Error("Error scanning row", zap.Error(err))
			continue
		}
		memoryPoolInfos = append(memoryPoolInfos, info)
	}
	if err := rows.Err(); err != nil {
		logger.Logger.Error("Error with rows", zap.Error(err))
	}
	// 发送数据到 Prometheus
	for _, info := range memoryPoolInfos {
		ch <- prometheus.MustNewConstMetric(c.totalPoolDesc, prometheus.GaugeValue, NullFloat64ToFloat64(info.TotalVal), config.GetHostName(), NullStringToString(info.ZoneType))
		ch <- prometheus.MustNewConstMetric(c.currPoolDesc, prometheus.GaugeValue, NullFloat64ToFloat64(info.CurrVal), config.GetHostName(), NullStringToString(info.ZoneType))
	}

	logger.Logger.Infof("MemoryPoolInfo exec finish")

}
