package collector

import (
	"context"
	"dameng_exporter/config"
	"dameng_exporter/logger"
	"database/sql"
	"fmt"
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
	dataSource      string // 数据源名称

	// 每个实例独立的视图检查缓存
	viewCheckOnce sync.Once
	viewChecked   bool
}

// SetDataSource 实现DataSourceAware接口
func (c *MonitorInfoCollector) SetDataSource(name string) {
	c.dataSource = name
}

// checkDmMonitorExists 检查V$DMMONITOR视图是否存在
// 使用sync.Once确保每个数据源只检查一次
func (c *MonitorInfoCollector) checkDmMonitorExists(ctx context.Context) bool {
	c.viewCheckOnce.Do(func() {
		const query = "SELECT COUNT(1) FROM V$DYNAMIC_TABLES WHERE NAME = 'V$DMMONITOR'"
		var count int
		if err := c.db.QueryRowContext(ctx, query).Scan(&count); err != nil {
			logger.Logger.Warnf("[%s] Failed to check V$DMMONITOR existence: %v", c.dataSource, err)
			c.viewChecked = false
			return
		}
		c.viewChecked = count == 1
		logger.Logger.Debugf("[%s] V$DMMONITOR exists: %v", c.dataSource, c.viewChecked)
	})
	return c.viewChecked
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

	if err := c.db.Ping(); err != nil {
		logger.Logger.Error(fmt.Sprintf("[%s] Database connection is not available: %v", c.dataSource, err), zap.Error(err))
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(config.Global.GetQueryTimeout())*time.Second)
	defer cancel()

	// 检查视图是否存在
	if !c.checkDmMonitorExists(ctx) {
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
			logger.Logger.Error(fmt.Sprintf("[%s] Error scanning row", c.dataSource), zap.Error(err))
			continue
		}
		monitorInfos = append(monitorInfos, info)
	}

	if err := rows.Err(); err != nil {
		logger.Logger.Error(fmt.Sprintf("[%s] Error with rows", c.dataSource), zap.Error(err))
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
