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

// DBSessionsStatusCollector 结构体
type DBSessionsStatusCollector struct {
	db                    *sql.DB
	sessionTypeDesc       *prometheus.Desc
	sessionPercentageDesc *prometheus.Desc
}

// DBSessionsStatusInfo 结构体
type DBSessionsStatusInfo struct {
	stateType sql.NullString
	countVal  sql.NullFloat64
}

// NewDBSessionsStatusCollector 函数
func NewDBSessionsStatusCollector(db *sql.DB) MetricCollector {
	return &DBSessionsStatusCollector{
		db: db,
		sessionTypeDesc: prometheus.NewDesc(
			dmdbms_session_type_Info,
			"Number of database sessions type status",
			[]string{"host_name", "session_type"},
			nil,
		),
		sessionPercentageDesc: prometheus.NewDesc(
			dmdbms_session_percentage,
			"Number of database sessions type percentage,method: total/max_session * 100%",
			[]string{"host_name"},
			nil,
		),
	}
}

// Describe 方法
func (c *DBSessionsStatusCollector) Describe(ch chan<- *prometheus.Desc) {
	ch <- c.sessionTypeDesc
	ch <- c.sessionPercentageDesc
}

func (c *DBSessionsStatusCollector) Collect(ch chan<- prometheus.Metric) {

	//保存全局结果对象
	var sessionsStatusInfos []DBSessionsStatusInfo

	if err := c.db.Ping(); err != nil {
		logger.Logger.Error("Database connection is not available: %v", zap.Error(err))
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(config.Global.GetQueryTimeout())*time.Second)
	defer cancel()

	rows, err := c.db.QueryContext(ctx, config.QueryDBSessionsStatusSqlStr)
	if err != nil {
		handleDbQueryError(err)
		return
	}
	defer rows.Close()

	for rows.Next() {
		var info DBSessionsStatusInfo
		if err := rows.Scan(&info.stateType, &info.countVal); err != nil {
			logger.Logger.Error("Error scanning row", zap.Error(err))
			continue
		}
		sessionsStatusInfos = append(sessionsStatusInfos, info)
	}
	if err := rows.Err(); err != nil {
		logger.Logger.Error("Error with rows", zap.Error(err))
	}

	var maxSession float64 = 0
	var totalSession float64 = 0
	// 发送数据到 Prometheus
	for _, info := range sessionsStatusInfos {
		if info.stateType.Valid && info.stateType.String == "MAX_SESSION" {
			maxSession = NullFloat64ToFloat64(info.countVal)
		} else if info.stateType.Valid && info.stateType.String == "TOTAL" {
			totalSession = NullFloat64ToFloat64(info.countVal)
		}
		ch <- prometheus.MustNewConstMetric(c.sessionTypeDesc, prometheus.GaugeValue, NullFloat64ToFloat64(info.countVal), config.GetHostName(), NullStringToString(info.stateType))
	}

	div := float64(0)
	if maxSession != 0 {
		div = totalSession / float64(maxSession)
	}
	if maxSession == 0 || div == 0 {
		div = 0
	}
	//eg：计算百分比，此处没有计算百分比
	ch <- prometheus.MustNewConstMetric(c.sessionPercentageDesc, prometheus.GaugeValue, div, "")

}
