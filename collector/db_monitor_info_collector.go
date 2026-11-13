package collector

import (
	"context"
	"dameng_exporter/config"
	"dameng_exporter/utils"
	"database/sql"
	"github.com/prometheus/client_golang/prometheus"
	"sync"
	"time"
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
	collectorLog
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
		if err := c.database(c.db).QueryRowContext(probeContext(ctx), query).Scan(&count); err != nil {

			c.viewChecked = false
			return
		}
		c.viewChecked = count == 1
		c.log().Debugf("[%s] V$DMMONITOR exists: %v", c.dataSource, c.viewChecked)
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
			[]string{"dw_conn_time", "mon_confirm", "mon_id", "mon_ip", "mon_version"},
			nil,
		),
		viewExists: true,
	}
}

func (c *MonitorInfoCollector) Describe(ch chan<- *prometheus.Desc) {
	ch <- c.monitorInfoDesc
}

func (c *MonitorInfoCollector) Collect(ch chan<- prometheus.Metric) {

	if err := utils.CheckDBConnectionWithSource(c.db, c.dataSource, c.log()); err != nil {
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(config.Global.GetQueryTimeout())*time.Second)
	defer cancel()

	// 检查视图是否存在
	if !c.checkDmMonitorExists(ctx) {
		return
	}

	rows, err := c.database(c.db).QueryContext(ctx, config.QueryMonitorInfoSqlStr)
	if err != nil {
		utils.HandleDbQueryErrorWithSource(err, c.dataSource, c.trace, c.log())
		return
	}
	defer rows.Close()

	var monitorInfos []MonitorInfo
	for rows.Next() {
		var info MonitorInfo
		if err := rows.Scan(&info.DwConnTime, &info.MonConfirm, &info.MonId, &info.MonIp, &info.MonVersion, &info.Mid); err != nil {

			continue
		}
		monitorInfos = append(monitorInfos, info)
	}

	_ = rows.Err()
	// 发送数据到 Prometheus
	for _, info := range monitorInfos {
		dwConnTime := utils.NullStringToString(info.DwConnTime)
		monConfirm := utils.NullStringToString(info.MonConfirm)
		monId := utils.NullStringToString(info.MonId)
		monIp := utils.NullStringToString(info.MonIp)
		monVersion := utils.NullStringToString(info.MonVersion)

		ch <- prometheus.MustNewConstMetric(
			c.monitorInfoDesc,
			prometheus.GaugeValue,
			utils.NullFloat64ToFloat64(info.Mid),
			dwConnTime, monConfirm, monId, monIp, monVersion,
		)
	}
}
