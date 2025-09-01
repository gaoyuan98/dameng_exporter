package collector

import (
	"context"
	"dameng_exporter/config"
	"dameng_exporter/logger"
	"database/sql"
	"fmt"
	"github.com/prometheus/client_golang/prometheus"
	"go.uber.org/zap"
	"time"
)

type DbMemoryPoolInfoCollector struct {
	db            *sql.DB
	totalPoolDesc *prometheus.Desc
	currPoolDesc  *prometheus.Desc
	dataSource    string // 数据源名称
}

type MemoryPoolInfo struct {
	ZoneType sql.NullString
	CurrVal  sql.NullFloat64
	ResVal   sql.NullFloat64
	TotalVal sql.NullFloat64
}

// SetDataSource 实现DataSourceAware接口
func (c *DbMemoryPoolInfoCollector) SetDataSource(name string) {
	c.dataSource = name
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

	//保存全局结果对象
	var memoryPoolInfos []MemoryPoolInfo

	if err := checkDBConnectionWithSource(c.db, c.dataSource); err != nil {
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(config.Global.GetQueryTimeout())*time.Second)
	defer cancel()

	rows, err := c.db.QueryContext(ctx, config.QueryMemoryPoolInfoSqlStr)
	if err != nil {
		handleDbQueryErrorWithSource(err, c.dataSource)
		return
	}
	defer rows.Close()

	for rows.Next() {
		var info MemoryPoolInfo
		if err := rows.Scan(&info.ZoneType, &info.CurrVal, &info.ResVal, &info.TotalVal); err != nil {
			logger.Logger.Error(fmt.Sprintf("[%s] Error scanning row", c.dataSource), zap.Error(err))
			continue
		}
		memoryPoolInfos = append(memoryPoolInfos, info)
	}
	if err := rows.Err(); err != nil {
		logger.Logger.Error(fmt.Sprintf("[%s] Error with rows", c.dataSource), zap.Error(err))
	}
	// 发送数据到 Prometheus
	for _, info := range memoryPoolInfos {
		ch <- prometheus.MustNewConstMetric(c.totalPoolDesc, prometheus.GaugeValue, NullFloat64ToFloat64(info.TotalVal), config.GetHostName(), NullStringToString(info.ZoneType))
		ch <- prometheus.MustNewConstMetric(c.currPoolDesc, prometheus.GaugeValue, NullFloat64ToFloat64(info.CurrVal), config.GetHostName(), NullStringToString(info.ZoneType))
	}

	//	logger.Logger.Infof("MemoryPoolInfo exec finish")

}
