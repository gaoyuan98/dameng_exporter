package collector

import (
	"context"
	"dameng_exporter/config"
	"dameng_exporter/logger"
	"dameng_exporter/utils"
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
	dataSource            string // 数据源名称
}

// DBSessionsStatusInfo 结构体
type DBSessionsStatusInfo struct {
	stateType sql.NullString
	countVal  sql.NullFloat64
}

// SetDataSource 实现DataSourceAware接口
func (c *DBSessionsStatusCollector) SetDataSource(name string) {
	c.dataSource = name
}

// NewDBSessionsStatusCollector 函数
func NewDBSessionsStatusCollector(db *sql.DB) MetricCollector {
	return &DBSessionsStatusCollector{
		db: db,
		sessionTypeDesc: prometheus.NewDesc(
			dmdbms_session_type_Info,
			"Number of database sessions type status",
			[]string{"session_type"},
			nil,
		),
		sessionPercentageDesc: prometheus.NewDesc(
			dmdbms_session_percentage,
			"Number of database sessions type percentage,method: total/max_session * 100%",
			[]string{},
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

	if err := utils.CheckDBConnectionWithSource(c.db, c.dataSource); err != nil {
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(config.Global.GetQueryTimeout())*time.Second)
	defer cancel()

	rows, err := c.db.QueryContext(ctx, config.QueryDBSessionsStatusSqlStr)
	if err != nil {
		utils.HandleDbQueryErrorWithSource(err, c.dataSource)
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
			maxSession = utils.NullFloat64ToFloat64(info.countVal)
		} else if info.stateType.Valid && info.stateType.String == "TOTAL" {
			totalSession = utils.NullFloat64ToFloat64(info.countVal)
		}
		ch <- prometheus.MustNewConstMetric(c.sessionTypeDesc, prometheus.GaugeValue, utils.NullFloat64ToFloat64(info.countVal), utils.NullStringToString(info.stateType))
	}

	div := float64(0)
	if maxSession != 0 {
		div = totalSession / float64(maxSession)
	}
	if maxSession == 0 || div == 0 {
		div = 0
	}
	//eg：计算百分比，此处没有计算百分比
	ch <- prometheus.MustNewConstMetric(c.sessionPercentageDesc, prometheus.GaugeValue, div)

}
