package collector

import (
	"context"
	"dameng_exporter/config"
	"dameng_exporter/logger"
	"database/sql"
	"sync"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"go.uber.org/zap"
)

// 定义数据结构
type MonitorInfo struct {
	DwConnTime sql.NullString
	MonConfirm sql.NullString
	MonId      sql.NullString
	MonIp      sql.NullString
	MonVersion sql.NullString
	Mid        sql.NullFloat64
}

// 定义收集器结构体
type MonitorInfoCollector struct {
	db              *sql.DB
	monitorInfoDesc *prometheus.Desc
	viewExists      bool
}

var (
	viewDmMonitorCheckOnce sync.Once
	viewDmMonitorExists    bool
)

// ViewDmMonitorExists 检查V$DMMONITOR视图是否存在
// 使用sync.Once确保检查只执行一次，结果会被缓存供后续使用
// 通过查询V$DYNAMIC_TABLES系统表来判断视图是否存在
// 参数:
//   - ctx: 上下文，用于控制查询超时
//   - db: 数据库连接
//
// 返回值:
//   - bool: 视图存在返回true，否则返回false
func ViewDmMonitorExists(ctx context.Context, db *sql.DB) bool {
	viewDmMonitorCheckOnce.Do(func() {
		const query = "SELECT COUNT(1) FROM V$DYNAMIC_TABLES WHERE NAME = 'V$DMMONITOR'"
		var count int
		if err := db.QueryRowContext(ctx, query).Scan(&count); err != nil {
			logger.Logger.Warn("Failed to check V$DMMONITOR existence", zap.Error(err))
			viewDmMonitorExists = false
			return
		}
		viewDmMonitorExists = count == 1
		logger.Logger.Debugf("V$DMMONITOR exists: %v", viewDmMonitorExists)
	})
	return viewDmMonitorExists
}

// NewMonitorInfoCollector 创建一个新的监控信息收集器
// 初始化收集器并设置监控指标的描述信息
// 参数:
//   - db: 数据库连接
//
// 返回值:
//   - MetricCollector: 实现了MetricCollector接口的收集器实例
func NewMonitorInfoCollector(db *sql.DB) MetricCollector {
	return &MonitorInfoCollector{
		db: db,
		monitorInfoDesc: prometheus.NewDesc(
			dmdbms_monitor_info,
			"Information about DM monitor",
			[]string{"host_name", "dw_conn_time", "mon_confirm", "mon_id", "mon_ip", "mon_version"},
			nil,
		),
		viewExists: true,
	}
}

func (c *MonitorInfoCollector) Describe(ch chan<- *prometheus.Desc) {
	ch <- c.monitorInfoDesc
}

func (c *MonitorInfoCollector) Collect(ch chan<- prometheus.Metric) {
	funcStart := time.Now()
	// 时间间隔的计算发生在 defer 语句执行时，确保能够获取到正确的函数执行时间。
	defer func() {
		duration := time.Since(funcStart)
		logger.Logger.Debugf("func exec time：%vms", duration.Milliseconds())
	}()

	if err := c.db.Ping(); err != nil {
		logger.Logger.Error("Database connection is not available: %v", zap.Error(err))
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(config.GlobalConfig.QueryTimeout)*time.Second)
	defer cancel()

	// 检查视图是否存在
	if !ViewDmMonitorExists(ctx, c.db) {
		return
	}

	rows, err := c.db.QueryContext(ctx, config.QueryMonitorInfoSqlStr)
	if err != nil {
		handleDbQueryError(err)
		return
	}
	defer rows.Close()

	var monitorInfos []MonitorInfo
	for rows.Next() {
		var info MonitorInfo
		if err := rows.Scan(&info.DwConnTime, &info.MonConfirm, &info.MonId, &info.MonIp, &info.MonVersion, &info.Mid); err != nil {
			logger.Logger.Error("Error scanning row", zap.Error(err))
			continue
		}
		monitorInfos = append(monitorInfos, info)
	}

	if err := rows.Err(); err != nil {
		logger.Logger.Error("Error with rows", zap.Error(err))
	}
	// 发送数据到 Prometheus
	for _, info := range monitorInfos {
		hostName := config.GetHostName()
		dwConnTime := NullStringToString(info.DwConnTime)
		monConfirm := NullStringToString(info.MonConfirm)
		monId := NullStringToString(info.MonId)
		monIp := NullStringToString(info.MonIp)
		monVersion := NullStringToString(info.MonVersion)

		ch <- prometheus.MustNewConstMetric(
			c.monitorInfoDesc,
			prometheus.GaugeValue,
			NullFloat64ToFloat64(info.Mid),
			hostName, dwConnTime, monConfirm, monId, monIp, monVersion,
		)
	}
}
