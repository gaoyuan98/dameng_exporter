package collector

import (
	"context"
	"dameng_exporter/config"
	"dameng_exporter/logger"
	"database/sql"
	"github.com/prometheus/client_golang/prometheus"
	"go.uber.org/zap"
	"time"
)

// SELECT /*+DMDB_CHECK_FLAG*/ WATCHER.DW_MODE,WATCHER.DW_STATUS,WATCHER.AUTO_RESTART,CASE WATCHER.DW_STATUS WHEN 'OPEN' THEN '1' WHEN 'MOUNT' THEN '2' WHEN 'SUSPEND' THEN '3' ELSE '4' END AS DW_STATUS_V
// 定义数据结构
type DbDwWatcherInfo struct {
	DwMode        sql.NullString
	DwStatus      sql.NullString
	AutoRestart   sql.NullString
	DwStatusToNum sql.NullFloat64
}

// 定义收集器结构体
type DbDwWatcherInfoCollector struct {
	db                *sql.DB
	dwWatcherInfoDesc *prometheus.Desc
}

func NewDbDwWatcherInfoCollector(db *sql.DB) MetricCollector {
	return &DbDwWatcherInfoCollector{
		db: db,
		dwWatcherInfoDesc: prometheus.NewDesc(
			dmdbms_dw_watcher_info,
			"Information about DM database Instance Watcher info, dw_status value info:  open = 1,mount = 2,suspend = 3 ,other = 4",
			[]string{"host_name", "dw_mode", "dw_status", "auto_restart"},
			nil,
		),
	}
}

func (c *DbDwWatcherInfoCollector) Describe(ch chan<- *prometheus.Desc) {
	ch <- c.dwWatcherInfoDesc
}

func (c *DbDwWatcherInfoCollector) Collect(ch chan<- prometheus.Metric) {

	if err := c.db.Ping(); err != nil {
		logger.Logger.Error("Database connection is not available: %v", zap.Error(err))
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(config.GlobalConfig.QueryTimeout)*time.Second)
	defer cancel()

	rows, err := c.db.QueryContext(ctx, config.QueryDwWatcherInfoSql)
	if err != nil {
		handleDbQueryError(err)
		return
	}
	defer rows.Close()

	var dbDwWatcherInfos []DbDwWatcherInfo
	for rows.Next() {
		var info DbDwWatcherInfo
		if err := rows.Scan(&info.DwMode, &info.DwStatus, &info.AutoRestart, &info.DwStatusToNum); err != nil {
			logger.Logger.Error("Error scanning row", zap.Error(err))
			continue
		}
		dbDwWatcherInfos = append(dbDwWatcherInfos, info)
	}

	if err := rows.Err(); err != nil {
		logger.Logger.Error("Error with rows", zap.Error(err))
	}

	hostname := config.GetHostName()
	// 发送数据到 Prometheus
	for _, info := range dbDwWatcherInfos {
		//[]string{"host_name", "pid", "level", "log_time", "txt"}

		dwMode := NullStringToString(info.DwMode)
		dwStatus := NullStringToString(info.DwStatus)
		autoRestart := NullStringToString(info.AutoRestart)
		dwStatusToNum := NullFloat64ToFloat64(info.DwStatusToNum)

		ch <- prometheus.MustNewConstMetric(
			c.dwWatcherInfoDesc,
			prometheus.GaugeValue,
			dwStatusToNum,
			hostname, dwMode, dwStatus, autoRestart,
		)
	}
}
