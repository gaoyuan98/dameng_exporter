package collector

import (
	"context"
	"dameng_exporter/config"
	"dameng_exporter/logger"
	"database/sql"
	"github.com/prometheus/client_golang/prometheus"
	"go.uber.org/zap"
	"strings"
	"time"
)

// 定义数据结构
type MonitorInfo struct {
	DwConnTime sql.NullTime
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
	//不存在则直接返回
	if !c.viewExists {
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(config.GlobalConfig.QueryTimeout)*time.Second)
	defer cancel()

	rows, err := c.db.QueryContext(ctx, config.QueryMonitorInfoSqlStr)
	if err != nil {
		if strings.EqualFold(err.Error(), "v$dmmonitor") { // 检查视图不存在的特定错误
			logger.Logger.Warn("v$dmmonitor view does not exist, skipping future queries", zap.Error(err))
			c.viewExists = false
			return
		}
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
		dwConnTime := NullTimeToString(info.DwConnTime)
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
