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

type PurgeCollector struct {
	dbPool       *sql.DB
	purgeObjects *prometheus.Desc
}

// PurgeInfo 存储回滚段信息
type PurgeInfo struct {
	ObjNum     int64
	IsRunning  string
	PurgeForTs string
}

func NewPurgeCollector(dbPool *sql.DB) *PurgeCollector {
	return &PurgeCollector{
		dbPool: dbPool,
		purgeObjects: prometheus.NewDesc(
			dmdbms_purge_objects_info,
			"Number of purge objects",
			[]string{"host_name", "is_running", "purge_for_ts"},
			nil,
		),
	}
}

func (c *PurgeCollector) Describe(ch chan<- *prometheus.Desc) {
	ch <- c.purgeObjects
}

func (c *PurgeCollector) Collect(ch chan<- prometheus.Metric) {

	if err := c.dbPool.Ping(); err != nil {
		logger.Logger.Error("Database connection is not available: %v", zap.Error(err))
		return
	}

	// 获取回滚段数据
	purgeInfos, err := c.getPurgeInfos()
	if err != nil {
		return
	}
	hostname := config.GetHostName()
	// 创建指标
	for _, info := range purgeInfos {
		ch <- prometheus.MustNewConstMetric(
			c.purgeObjects,
			prometheus.GaugeValue,
			float64(info.ObjNum),
			hostname,
			info.IsRunning,
			info.PurgeForTs,
		)
	}
}

// getPurgeInfos 获取回滚段信息
func (c *PurgeCollector) getPurgeInfos() ([]PurgeInfo, error) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(config.Global.GetQueryTimeout())*time.Second)
	defer cancel()

	rows, err := c.dbPool.QueryContext(ctx, config.QueryPurgeInfoSqlStr)
	if err != nil {
		handleDbQueryError(err)
		return nil, err
	}
	defer rows.Close()

	var purgeInfos []PurgeInfo
	for rows.Next() {
		var info PurgeInfo
		err := rows.Scan(&info.ObjNum, &info.IsRunning, &info.PurgeForTs)
		if err != nil {
			logger.Logger.Error("Error scanning purge row", zap.Error(err))
			continue
		}
		purgeInfos = append(purgeInfos, info)
	}

	if err = rows.Err(); err != nil {
		logger.Logger.Error("Error iterating purge rows", zap.Error(err))
		return nil, err
	}

	return purgeInfos, nil
}
