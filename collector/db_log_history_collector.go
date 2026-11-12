package collector

import (
	"context"
	"dameng_exporter/config"
	"dameng_exporter/utils"
	"database/sql"
	"sync"
	"time"

	"github.com/prometheus/client_golang/prometheus"
)

// DbLogHistoryCollector 负责暴露 redo 日志切换时间相关指标。
type DbLogHistoryCollector struct {
	collectorLog
	db                     *sql.DB
	dataSource             string
	redoLastSwitchTimeDesc *prometheus.Desc
	viewCheckOnce          sync.Once
	viewExists             bool
}

// NewDbLogHistoryCollector 返回 redo 日志切换指标采集器实例。
func NewDbLogHistoryCollector(db *sql.DB) MetricCollector {
	return &DbLogHistoryCollector{
		db: db,
		redoLastSwitchTimeDesc: prometheus.NewDesc(
			dmdbms_redo_last_switch_time_seconds,
			"Unix timestamp of the last redo log switch; zero when unavailable",
			[]string{},
			nil,
		),
		viewExists: true,
	}
}

func (c *DbLogHistoryCollector) SetDataSource(name string) {
	c.dataSource = name
}

// Describe 实现 Prometheus Collector 接口，输出指标描述。
func (c *DbLogHistoryCollector) Describe(ch chan<- *prometheus.Desc) {
	ch <- c.redoLastSwitchTimeDesc
}

// Collect 从 V$LOG_HISTORY 采样并计算指标值。
func (c *DbLogHistoryCollector) Collect(ch chan<- prometheus.Metric) {
	if err := utils.CheckDBConnectionWithSource(c.db, c.dataSource, c.log()); err != nil {
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(config.Global.GetQueryTimeout())*time.Second)
	defer cancel()

	if !c.checkLogHistoryViewExists(ctx) {
		return
	}

	var rectime sql.NullString
	err := c.db.QueryRowContext(ctx, config.QueryRedoLogHistorySql).Scan(&rectime)
	if err != nil {
		if err == sql.ErrNoRows {
			ch <- prometheus.MustNewConstMetric(c.redoLastSwitchTimeDesc, prometheus.GaugeValue, 0)
		} else {
			utils.HandleDbQueryErrorWithSource(err, c.dataSource, c.log())
		}
		return
	}

	lastSwitchTime, err := utils.NullStringTimeToUnixSeconds(rectime)
	if err != nil {
		c.log().Warnf("[%s] Failed to parse redo log rectime %q: %v", c.dataSource, utils.NullStringToString(rectime), err)
		lastSwitchTime = 0
	}

	ch <- prometheus.MustNewConstMetric(c.redoLastSwitchTimeDesc, prometheus.GaugeValue, lastSwitchTime)
}

// checkLogHistoryViewExists 检查 V$LOG_HISTORY 视图是否存在，缺失时跳过 redo 日志历史指标采集。
func (c *DbLogHistoryCollector) checkLogHistoryViewExists(ctx context.Context) bool {
	c.viewCheckOnce.Do(func() {
		var count int
		if err := c.db.QueryRowContext(ctx, config.QueryRedoLogHistoryViewExistsSql).Scan(&count); err != nil {
			c.log().Warnf("[%s] Failed to check V$LOG_HISTORY view existence: %v", c.dataSource, err)
			c.viewExists = false
			return
		}
		c.viewExists = count > 0
		if !c.viewExists {
			c.log().Debugf("[%s] V$LOG_HISTORY view not found, skip redo log history metrics", c.dataSource)
		}
	})
	return c.viewExists
}
