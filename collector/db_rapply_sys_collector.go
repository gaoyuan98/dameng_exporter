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
type RapplySysInfo struct {
	TaskMemUsed sql.NullFloat64
	TaskNum     sql.NullFloat64
}

// 定义收集器结构体
type DbRapplySysCollector struct {
	db              *sql.DB
	taskMemUsedDesc *prometheus.Desc
	taskNumDesc     *prometheus.Desc
}

func NewDbRapplySysCollector(db *sql.DB) MetricCollector {
	return &DbRapplySysCollector{
		db: db,
		taskMemUsedDesc: prometheus.NewDesc(
			dmdbms_rapply_sys_task_mem_used,
			"Information about DM database apply system task memory used",
			[]string{"host_name"},
			nil,
		),
		taskNumDesc: prometheus.NewDesc(
			dmdbms_rapply_sys_task_num,
			"Information about DM database apply system task number",
			[]string{"host_name"},
			nil,
		),
	}
}

func (c *DbRapplySysCollector) Describe(ch chan<- *prometheus.Desc) {
	ch <- c.taskMemUsedDesc
	ch <- c.taskNumDesc
}

func (c *DbRapplySysCollector) Collect(ch chan<- prometheus.Metric) {

	if err := c.db.Ping(); err != nil {
		logger.Logger.Error("Database connection is not available", zap.Error(err))
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(config.Global.GetQueryTimeout())*time.Second)
	defer cancel()

	// 执行查询
	rows, err := c.db.QueryContext(ctx, config.QueryStandbyInfoSql)
	if err != nil {
		handleDbQueryError(err)
		return
	}
	defer rows.Close()

	var rapplySysInfos []RapplySysInfo
	for rows.Next() {
		var info RapplySysInfo
		if err := rows.Scan(&info.TaskMemUsed, &info.TaskNum); err != nil {
			logger.Logger.Error("Error scanning row", zap.Error(err))
			continue
		}
		rapplySysInfos = append(rapplySysInfos, info)
	}
	if err := rows.Err(); err != nil {
		logger.Logger.Error("Error with rows", zap.Error(err))
		return
	}

	hostname := config.GetHostName()
	for _, info := range rapplySysInfos {
		ch <- prometheus.MustNewConstMetric(
			c.taskMemUsedDesc,
			prometheus.GaugeValue,
			NullFloat64ToFloat64(info.TaskMemUsed),
			hostname,
		)
		ch <- prometheus.MustNewConstMetric(
			c.taskNumDesc,
			prometheus.GaugeValue,
			NullFloat64ToFloat64(info.TaskNum),
			hostname,
		)
	}
}
