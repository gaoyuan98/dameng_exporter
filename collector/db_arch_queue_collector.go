package collector

import (
	"context"
	"dameng_exporter/config"
	"dameng_exporter/logger"
	"dameng_exporter/utils"
	"database/sql"
	"fmt"
	"sync"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"go.uber.org/zap"
)

// DbArchQueueInfo 归档队列等待信息
type DbArchQueueInfo struct {
	archType sql.NullString
	waiting  sql.NullFloat64
}

// DbArchQueueCollector 归档队列等待指标采集器
type DbArchQueueCollector struct {
	db                    *sql.DB
	archQueueWaitingDesc  *prometheus.Desc
	dataSource            string
	waitingFieldCheckOnce sync.Once
	waitingFieldExists    bool
}

// SetDataSource 实现DataSourceAware接口
func (c *DbArchQueueCollector) SetDataSource(name string) {
	c.dataSource = name
}

// NewDbArchQueueCollector 初始化归档队列等待指标采集器
func NewDbArchQueueCollector(db *sql.DB) MetricCollector {
	return &DbArchQueueCollector{
		db: db,
		archQueueWaitingDesc: prometheus.NewDesc(
			dmdbms_arch_queue_waiting_info,
			"DM database archive queue waiting information",
			[]string{"arch_type"},
			nil,
		),
	}
}

func (c *DbArchQueueCollector) Describe(ch chan<- *prometheus.Desc) {
	ch <- c.archQueueWaitingDesc
}

func (c *DbArchQueueCollector) Collect(ch chan<- prometheus.Metric) {
	if err := utils.CheckDBConnectionWithSource(c.db, c.dataSource); err != nil {
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(config.Global.GetQueryTimeout())*time.Second)
	defer cancel()

	if !c.checkWaitingFieldExists(ctx) {
		return
	}

	rows, err := c.db.QueryContext(ctx, config.QueryArchQueueWaitingSql)
	if err != nil {
		utils.HandleDbQueryErrorWithSource(err, c.dataSource)
		return
	}
	defer rows.Close()

	for rows.Next() {
		var info DbArchQueueInfo
		if err := rows.Scan(&info.archType, &info.waiting); err != nil {
			logger.Logger.Error(fmt.Sprintf("[%s] Error scanning V$ARCH_QUEUE row", c.dataSource), zap.Error(err))
			continue
		}

		if !info.waiting.Valid {
			continue
		}

		ch <- prometheus.MustNewConstMetric(
			c.archQueueWaitingDesc,
			prometheus.GaugeValue,
			info.waiting.Float64,
			utils.NullStringToString(info.archType),
		)
	}

	if err := rows.Err(); err != nil {
		logger.Logger.Error(fmt.Sprintf("[%s] Error iterating V$ARCH_QUEUE rows", c.dataSource), zap.Error(err))
	}
}

func (c *DbArchQueueCollector) checkWaitingFieldExists(ctx context.Context) bool {
	c.waitingFieldCheckOnce.Do(func() {
		var count int
		if err := c.db.QueryRowContext(ctx, config.QueryArchQueueWaitingFieldExists).Scan(&count); err != nil {
			logger.Logger.Warnf("[%s] Failed to check V$ARCH_QUEUE WAITING field: %v", c.dataSource, err)
			c.waitingFieldExists = false
			return
		}

		c.waitingFieldExists = count > 0
		if c.waitingFieldExists {
			logger.Logger.Debugf("[%s] V$ARCH_QUEUE WAITING field exists", c.dataSource)
		} else {
			logger.Logger.Infof("[%s] V$ARCH_QUEUE WAITING field not found, skip waiting metric", c.dataSource)
		}
	})

	return c.waitingFieldExists
}
