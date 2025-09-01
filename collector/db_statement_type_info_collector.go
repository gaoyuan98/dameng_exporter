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

// 定义数据结构
type DbSqlExecTypeInfo struct {
	Name    sql.NullString
	StatVal sql.NullFloat64
}

// 定义收集器结构体
type DbSqlExecTypeCollector struct {
	db                *sql.DB
	statementTypeDesc *prometheus.Desc
}

func NewDbSqlExecTypeCollector(db *sql.DB) MetricCollector {
	return &DbSqlExecTypeCollector{
		db: db,
		statementTypeDesc: prometheus.NewDesc(
			dmdbms_statement_type_info,
			"Information about different types of statements",
			[]string{"host_name", "statement_name"},
			nil,
		),
	}

}

func (c *DbSqlExecTypeCollector) Describe(ch chan<- *prometheus.Desc) {
	ch <- c.statementTypeDesc
}

func (c *DbSqlExecTypeCollector) Collect(ch chan<- prometheus.Metric) {

	if err := c.db.Ping(); err != nil {
		logger.Logger.Error("Database connection is not available: %v", zap.Error(err))
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(config.Global.GetQueryTimeout())*time.Second)
	defer cancel()

	rows, err := c.db.QueryContext(ctx, config.QuerySqlExecuteCountSqlStr)
	if err != nil {
		handleDbQueryError(err)
		return
	}
	defer rows.Close()

	var sysstatInfos []DbSqlExecTypeInfo
	for rows.Next() {
		var info DbSqlExecTypeInfo
		if err := rows.Scan(&info.Name, &info.StatVal); err != nil {
			logger.Logger.Error("Error scanning row", zap.Error(err))
			continue
		}
		sysstatInfos = append(sysstatInfos, info)
	}

	if err := rows.Err(); err != nil {
		logger.Logger.Error("Error with rows", zap.Error(err))
	}
	// 发送数据到 Prometheus
	hostname := config.GetHostName()
	for _, info := range sysstatInfos {
		statementName := NullStringToString(info.Name)

		ch <- prometheus.MustNewConstMetric(
			c.statementTypeDesc,
			prometheus.CounterValue,
			NullFloat64ToFloat64(info.StatVal),
			hostname, statementName,
		)
	}
}
