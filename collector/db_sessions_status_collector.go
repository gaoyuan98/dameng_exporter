package collector

import (
	"context"
	"dameng_exporter/config"
	"dameng_exporter/utils"
	"database/sql"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"go.uber.org/zap"
)

// DBSessionsStatusCollector 结构体
type DBSessionsStatusCollector struct {
	collectorLog
	db              *sql.DB
	sessionTypeDesc *prometheus.Desc
	dataSource      string // 数据源名称
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
	}
}

// Describe 方法
func (c *DBSessionsStatusCollector) Describe(ch chan<- *prometheus.Desc) {
	ch <- c.sessionTypeDesc
}

func (c *DBSessionsStatusCollector) Collect(ch chan<- prometheus.Metric) {
	c.CollectWithResult(ch)
}

func (c *DBSessionsStatusCollector) CollectWithResult(ch chan<- prometheus.Metric) CollectorOutcome {

	// 完整读取成功后再输出，避免将查询失败或部分结果误报为零。
	var sessionsStatusInfos []DBSessionsStatusInfo

	if err := utils.CheckDBConnectionWithSource(c.db, c.dataSource, c.log()); err != nil {
		return databaseOutcome(OutcomeFailed, "datasource_unavailable")
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(config.Global.GetQueryTimeout())*time.Second)
	defer cancel()

	rows, err := c.db.QueryContext(ctx, config.QueryDBSessionsStatusSqlStr, config.ExporterAppName)
	if err != nil {
		outcome := queryFailure(ctx, err)
		utils.HandleDbQueryErrorWithSource(err, c.dataSource, c.log())
		return outcome
	}
	defer rows.Close()

	for rows.Next() {
		var info DBSessionsStatusInfo
		if err := rows.Scan(&info.stateType, &info.countVal); err != nil {
			c.log().Desugar().Error("Error scanning session row", zap.Error(err))
			return databaseOutcome(OutcomeFailed, "scan_error")
		}
		sessionsStatusInfos = append(sessionsStatusInfos, info)
	}
	if err := rows.Err(); err != nil {
		c.log().Desugar().Error("Error with rows", zap.Error(err))
		return databaseOutcome(OutcomeFailed, "rows_error")
	}

	// 过滤采集连接后可能没有 ACTIVE/IDLE；查询成功时明确输出零。
	seen := make(map[string]bool, len(sessionsStatusInfos))
	for _, info := range sessionsStatusInfos {
		state := utils.NullStringToString(info.stateType)
		seen[state] = true
		ch <- prometheus.MustNewConstMetric(c.sessionTypeDesc, prometheus.GaugeValue, utils.NullFloat64ToFloat64(info.countVal), state)
	}
	for _, state := range []string{"ACTIVE", "IDLE"} {
		if !seen[state] {
			ch <- prometheus.MustNewConstMetric(c.sessionTypeDesc, prometheus.GaugeValue, 0, state)
		}
	}

	return databaseOutcome(OutcomeSuccess, "")

}
