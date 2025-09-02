package collector

import (
	"context"
	"dameng_exporter/config"
	"dameng_exporter/logger"
	"database/sql"
	"time"

	"github.com/prometheus/client_golang/prometheus"
)

type PurgeCollector struct {
	dbPool       *sql.DB
	purgeObjects *prometheus.Desc
	dataSource   string // 数据源名称
}

// SetDataSource 实现DataSourceAware接口
func (c *PurgeCollector) SetDataSource(name string) {
	c.dataSource = name
}

// PurgeInfo 存储回滚段信息
type PurgeInfo struct {
	ObjNum     int64
	IsRunning  string
	PurgeForTs string
}

func NewPurgeCollector(dbPool *sql.DB) MetricCollector {
	return &PurgeCollector{
		dbPool: dbPool,
		purgeObjects: prometheus.NewDesc(
			dmdbms_purge_objects_info,
			"Number of purge objects",
			[]string{"is_running", "purge_for_ts", "data_source"},
			nil,
		),
	}
}

func (c *PurgeCollector) Describe(ch chan<- *prometheus.Desc) {
	ch <- c.purgeObjects
}

func (c *PurgeCollector) Collect(ch chan<- prometheus.Metric) {

	if err := checkDBConnectionWithSource(c.dbPool, c.dataSource); err != nil {
		return
	}

	// 获取回滚段数据
	purgeInfos, err := c.getPurgeInfos()
	if err != nil {
		return
	}
	// 创建指标
	for _, info := range purgeInfos {
		ch <- prometheus.MustNewConstMetric(
			c.purgeObjects,
			prometheus.GaugeValue,
			float64(info.ObjNum),
			info.IsRunning,
			info.PurgeForTs,
			c.dataSource,
		)
	}
}

// getPurgeInfos 获取回滚段信息
func (c *PurgeCollector) getPurgeInfos() ([]PurgeInfo, error) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(config.Global.GetQueryTimeout())*time.Second)
	defer cancel()

	rows, err := c.dbPool.QueryContext(ctx, config.QueryPurgeInfoSqlStr)
	if err != nil {
		handleDbQueryErrorWithSource(err, c.dataSource)
		return nil, err
	}
	defer rows.Close()

	var purgeInfos []PurgeInfo
	for rows.Next() {
		var info PurgeInfo
		err := rows.Scan(&info.ObjNum, &info.IsRunning, &info.PurgeForTs)
		if err != nil {
			logger.Logger.Errorf("[%s] Error scanning purge row: %v", c.dataSource, err)
			continue
		}
		purgeInfos = append(purgeInfos, info)
	}

	if err = rows.Err(); err != nil {
		logger.Logger.Errorf("[%s] Error iterating purge rows: %v", c.dataSource, err)
		return nil, err
	}

	return purgeInfos, nil
}
