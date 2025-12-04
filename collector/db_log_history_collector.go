package collector

import (
	"context"
	"dameng_exporter/config"
	"dameng_exporter/logger"
	"dameng_exporter/utils"
	"database/sql"
	"time"

	"github.com/prometheus/client_golang/prometheus"
)

// logHistoryFetchLimit 控制单次查询 V$LOG_HISTORY 的记录数量
const logHistoryFetchLimit = 5

type logHistoryRow struct {
	rectime sql.NullTime
}

// DbLogHistoryCollector 负责暴露 redo 日志切换时间相关指标
type DbLogHistoryCollector struct {
	db           *sql.DB
	dataSource   string
	lastTimeDesc *prometheus.Desc
	intervalDesc *prometheus.Desc
}

// NewDbLogHistoryCollector 返回 redo 日志切换指标采集器实例
func NewDbLogHistoryCollector(db *sql.DB) MetricCollector {
	return &DbLogHistoryCollector{
		db: db,
		lastTimeDesc: prometheus.NewDesc(
			dmdbms_redo_switch_last_time_seconds,
			"Unix timestamp of the most recent redo log switch",
			[]string{},
			nil,
		),
		intervalDesc: prometheus.NewDesc(
			dmdbms_redo_switch_interval_seconds,
			"Seconds between the two most recent redo log switches",
			[]string{},
			nil,
		),
	}
}

func (c *DbLogHistoryCollector) SetDataSource(name string) {
	c.dataSource = name
}

// Describe 实现 Prometheus Collector 接口，输出指标描述
func (c *DbLogHistoryCollector) Describe(ch chan<- *prometheus.Desc) {
	ch <- c.lastTimeDesc
	ch <- c.intervalDesc
}

// Collect 从 V$LOG_HISTORY 采样并计算指标值
func (c *DbLogHistoryCollector) Collect(ch chan<- prometheus.Metric) {
	// 1. 快速校验数据库连接是否可用，避免因连接异常导致采集阻塞
	if err := utils.CheckDBConnectionWithSource(c.db, c.dataSource); err != nil {
		return
	}

	// 2. 根据查询超时配置创建上下文，限制对 V$LOG_HISTORY 的访问时间
	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(config.Global.GetQueryTimeout())*time.Second)
	defer cancel()

	// 3. 按 RECID 倒序拉取最近若干条切换记录
	rows, err := c.db.QueryContext(ctx, config.QueryRedoLogHistorySql, logHistoryFetchLimit)
	if err != nil {
		utils.HandleDbQueryErrorWithSource(err, c.dataSource)
		return
	}
	defer rows.Close()

	var history []logHistoryRow
	// 4. 遍历结果集，收集用于计算的字段
	for rows.Next() {
		var item logHistoryRow
		if err := rows.Scan(&item.rectime); err != nil {
			logger.Logger.Warnf("[%s] Failed to scan redo log history: %v", c.dataSource, err)
			return
		}
		history = append(history, item)
	}

	if err := rows.Err(); err != nil {
		logger.Logger.Warnf("[%s] Redo log history iteration error: %v", c.dataSource, err)
		return
	}

	var validTimes []time.Time
	for _, item := range history {
		if item.rectime.Valid {
			validTimes = append(validTimes, item.rectime.Time)
			if len(validTimes) == 2 {
				break
			}
		}
	}

	var lastSwitchTime float64
	var lastInterval float64

	// 5. 计算最新一次切换时间
	if len(validTimes) > 0 {
		lastSwitchTime = float64(validTimes[0].Unix())
	}

	// 6. 若存在上一条记录，计算最近一次切换间隔
	if len(validTimes) > 1 {
		delta := validTimes[0].Sub(validTimes[1]).Seconds()
		if delta >= 0 {
			lastInterval = delta
		}
	}

	// 7. 发送两条指标：最近切换时间与最新间隔
	ch <- prometheus.MustNewConstMetric(c.lastTimeDesc, prometheus.GaugeValue, lastSwitchTime)
	ch <- prometheus.MustNewConstMetric(c.intervalDesc, prometheus.GaugeValue, lastInterval)
}
