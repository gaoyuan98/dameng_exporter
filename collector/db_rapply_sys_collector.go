package collector

import (
	"context"
	"dameng_exporter/config"
	"dameng_exporter/logger"
	"dameng_exporter/utils"
	"database/sql"
	"fmt"
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
	dataSource      string // 数据源名称
}

// SetDataSource 实现DataSourceAware接口
func (c *DbRapplySysCollector) SetDataSource(name string) {
	c.dataSource = name
}

func NewDbRapplySysCollector(db *sql.DB) MetricCollector {
	return &DbRapplySysCollector{
		db: db,
		taskMemUsedDesc: prometheus.NewDesc(
			dmdbms_rapply_sys_task_mem_used,
			"Information about DM database apply system task memory used",
			[]string{},
			nil,
		),
		taskNumDesc: prometheus.NewDesc(
			dmdbms_rapply_sys_task_num,
			"Information about DM database apply system task number",
			[]string{},
			nil,
		),
	}
}

func (c *DbRapplySysCollector) Describe(ch chan<- *prometheus.Desc) {
	ch <- c.taskMemUsedDesc
	ch <- c.taskNumDesc
}

func (c *DbRapplySysCollector) Collect(ch chan<- prometheus.Metric) {

	if err := utils.CheckDBConnectionWithSource(c.db, c.dataSource); err != nil {
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(config.Global.GetQueryTimeout())*time.Second)
	defer cancel()

	// 执行查询
	rows, err := c.db.QueryContext(ctx, config.QueryStandbyInfoSql)
	if err != nil {
		utils.HandleDbQueryErrorWithSource(err, c.dataSource)
		return
	}
	defer rows.Close()

	var rapplySysInfos []RapplySysInfo
	for rows.Next() {
		var info RapplySysInfo
		if err := rows.Scan(&info.TaskMemUsed, &info.TaskNum); err != nil {
			logger.Logger.Error(fmt.Sprintf("[%s] Error scanning row", c.dataSource), zap.Error(err))
			continue
		}
		rapplySysInfos = append(rapplySysInfos, info)
	}
	if err := rows.Err(); err != nil {
		logger.Logger.Error(fmt.Sprintf("[%s] Error with rows", c.dataSource), zap.Error(err))
		return
	}

	for _, info := range rapplySysInfos {
		ch <- prometheus.MustNewConstMetric(
			c.taskMemUsedDesc,
			prometheus.GaugeValue,
			utils.NullFloat64ToFloat64(info.TaskMemUsed),
		)
		ch <- prometheus.MustNewConstMetric(
			c.taskNumDesc,
			prometheus.GaugeValue,
			utils.NullFloat64ToFloat64(info.TaskNum),
		)
	}
}
