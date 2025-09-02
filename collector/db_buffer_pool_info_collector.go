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
type BufferPoolInfo struct {
	bufferName sql.NullString
	hitRate    sql.NullFloat64
}

// 定义收集器结构体
type DbBufferPoolInfoCollector struct {
	db                 *sql.DB
	bufferPoolInfoDesc *prometheus.Desc
	dataSource         string // 数据源名称
}

// SetDataSource 实现DataSourceAware接口
func (c *DbBufferPoolInfoCollector) SetDataSource(name string) {
	c.dataSource = name
}

func NewDbBufferPoolCollector(db *sql.DB) MetricCollector {
	return &DbBufferPoolInfoCollector{
		db: db,
		bufferPoolInfoDesc: prometheus.NewDesc(
			dmdbms_bufferpool_info,
			"Information about DM database bufferpool return hitRate",
			[]string{"buffer_name"},
			nil,
		),
	}
}

func (c *DbBufferPoolInfoCollector) Describe(ch chan<- *prometheus.Desc) {
	ch <- c.bufferPoolInfoDesc
}

func (c *DbBufferPoolInfoCollector) Collect(ch chan<- prometheus.Metric) {

	if err := utils.CheckDBConnectionWithSource(c.db, c.dataSource); err != nil {
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(config.Global.GetQueryTimeout())*time.Second)
	defer cancel()

	rows, err := c.db.QueryContext(ctx, config.QueryBufferPoolHitRateInfoSql)
	if err != nil {
		utils.HandleDbQueryErrorWithSource(err, c.dataSource)
		return
	}
	defer rows.Close()

	var bufferPoolInfos []BufferPoolInfo
	for rows.Next() {
		var info BufferPoolInfo
		if err := rows.Scan(&info.bufferName, &info.hitRate); err != nil {
			logger.Logger.Error(fmt.Sprintf("[%s] Error scanning row", c.dataSource), zap.Error(err))
			continue
		}
		bufferPoolInfos = append(bufferPoolInfos, info)
	}

	if err := rows.Err(); err != nil {
		logger.Logger.Error(fmt.Sprintf("[%s] Error with rows", c.dataSource), zap.Error(err))
	}

	// 发送数据到 Prometheus
	for _, info := range bufferPoolInfos {

		bufferName := utils.NullStringToString(info.bufferName)
		hitRate := utils.NullFloat64ToFloat64(info.hitRate)
		ch <- prometheus.MustNewConstMetric(
			c.bufferPoolInfoDesc,
			prometheus.GaugeValue,
			hitRate,
			bufferName,
		)
	}
}
