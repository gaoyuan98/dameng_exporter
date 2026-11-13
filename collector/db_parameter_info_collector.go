package collector

import (
	"context"
	"dameng_exporter/config"
	"dameng_exporter/utils"
	"database/sql"
	"github.com/prometheus/client_golang/prometheus"
	"time"
)

// 定义数据结构
type IniParameterInfo struct {
	ParaName  sql.NullString
	ParaValue sql.NullFloat64
}

// 定义收集器结构体
type IniParameterCollector struct {
	collectorLog
	db                *sql.DB
	parameterInfoDesc *prometheus.Desc
	dataSource        string // 数据源名称
}

// SetDataSource 实现DataSourceAware接口
func (c *IniParameterCollector) SetDataSource(name string) {
	c.dataSource = name
}

func NewIniParameterCollector(db *sql.DB) MetricCollector {
	return &IniParameterCollector{
		db: db,
		parameterInfoDesc: prometheus.NewDesc(
			dmdbms_parameter_info,
			"Information about DM database parameters",
			[]string{"param_name"},
			nil,
		),
	}

}

func (c *IniParameterCollector) Describe(ch chan<- *prometheus.Desc) {
	ch <- c.parameterInfoDesc
}

func (c *IniParameterCollector) Collect(ch chan<- prometheus.Metric) {

	if err := utils.CheckDBConnectionWithSource(c.db, c.dataSource, c.log()); err != nil {
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(config.Global.GetQueryTimeout())*time.Second)
	defer cancel()

	rows, err := c.database(c.db).QueryContext(ctx, config.QueryParameterInfoSql)
	if err != nil {
		utils.HandleDbQueryErrorWithSource(err, c.dataSource, c.trace, c.log())
		return
	}
	defer rows.Close()

	var iniParameterInfos []IniParameterInfo
	for rows.Next() {
		var info IniParameterInfo
		if err := rows.Scan(&info.ParaName, &info.ParaValue); err != nil {

			continue
		}
		iniParameterInfos = append(iniParameterInfos, info)
	}
	if err := rows.Err(); err != nil {
		return
	}

	// 发送数据到 Prometheus
	for _, info := range iniParameterInfos {
		paramName := utils.NullStringToString(info.ParaName)
		ch <- prometheus.MustNewConstMetric(
			c.parameterInfoDesc,
			prometheus.GaugeValue,
			utils.NullFloat64ToFloat64(info.ParaValue),
			paramName,
		)
	}
}
