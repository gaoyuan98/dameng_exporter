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

// DbLogHistoryCollector 负责暴露 redo 日志切换时间相关指标
type DbLogHistoryCollector struct {
	db                     *sql.DB
	dataSource             string
	redoLastSwitchTimeDesc *prometheus.Desc
}

// NewDbLogHistoryCollector 返回 redo 日志切换指标采集器实例
func NewDbLogHistoryCollector(db *sql.DB) MetricCollector {
	return &DbLogHistoryCollector{
		db: db,
		redoLastSwitchTimeDesc: prometheus.NewDesc(
			dmdbms_redo_last_switch_time_seconds,
			"Unix timestamp of the last redo log switch; zero when unavailable",
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
	ch <- c.redoLastSwitchTimeDesc
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

	// 3. 查询最新一条 redo 切换记录
	var rectime sql.NullString
	err := c.db.QueryRowContext(ctx, config.QueryRedoLogHistorySql).Scan(&rectime)
	if err != nil {
		if err == sql.ErrNoRows {
			ch <- prometheus.MustNewConstMetric(c.redoLastSwitchTimeDesc, prometheus.GaugeValue, 0)
		} else {
			utils.HandleDbQueryErrorWithSource(err, c.dataSource)
		}
		return
	}

	lastSwitchTime, err := utils.NullStringTimeToUnixSeconds(rectime)
	if err != nil {
		logger.Logger.Warnf("[%s] Failed to parse redo log rectime %q: %v", c.dataSource, utils.NullStringToString(rectime), err)
		lastSwitchTime = 0
	}

	// 4. 暴露指标，值为最新切换时间（秒），用于在仪表盘端计算与当前时间的间隔
	ch <- prometheus.MustNewConstMetric(c.redoLastSwitchTimeDesc, prometheus.GaugeValue, lastSwitchTime)
}
