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
type IniParameterInfo struct {
	ParaName  sql.NullString
	ParaValue sql.NullFloat64
}

// 定义收集器结构体
type IniParameterCollector struct {
	db                *sql.DB
	parameterInfoDesc *prometheus.Desc
}

func NewIniParameterCollector(db *sql.DB) MetricCollector {
	return &IniParameterCollector{
		db: db,
		parameterInfoDesc: prometheus.NewDesc(
			dmdbms_parameter_info,
			"Information about DM database parameters",
			[]string{"host_name", "param_name"},
			nil,
		),
	}

}

func (c *IniParameterCollector) Describe(ch chan<- *prometheus.Desc) {
	ch <- c.parameterInfoDesc
}

func (c *IniParameterCollector) Collect(ch chan<- prometheus.Metric) {

	if err := c.db.Ping(); err != nil {
		logger.Logger.Error("Database connection is not available: %v", zap.Error(err))
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(config.GlobalConfig.QueryTimeout)*time.Second)
	defer cancel()

	rows, err := c.db.QueryContext(ctx, config.QueryParameterInfoSql)
	if err != nil {
		handleDbQueryError(err)
		return
	}
	defer rows.Close()

	var iniParameterInfos []IniParameterInfo
	for rows.Next() {
		var info IniParameterInfo
		if err := rows.Scan(&info.ParaName, &info.ParaValue); err != nil {
			logger.Logger.Error("Error scanning row", zap.Error(err))
			continue
		}
		iniParameterInfos = append(iniParameterInfos, info)
	}
	if err := rows.Err(); err != nil {
		logger.Logger.Error("Error with rows", zap.Error(err))
		return
	}

	// 发送数据到 Prometheus
	hostname := config.GetHostName()
	for _, info := range iniParameterInfos {
		paramName := NullStringToString(info.ParaName)
		ch <- prometheus.MustNewConstMetric(
			c.parameterInfoDesc,
			prometheus.GaugeValue,
			NullFloat64ToFloat64(info.ParaValue),
			hostname, paramName,
		)
	}
}
