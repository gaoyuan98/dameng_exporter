package collector

import (
	"context"
	"dameng_exporter/config"
	"dameng_exporter/logger"
	"dameng_exporter/utils"
	"database/sql"
	"fmt"
	"github.com/prometheus/client_golang/prometheus"
	"go.uber.org/zap"
	"time"
)

type SessionInfoCollector struct {
	db               *sql.DB
	slowSQLInfoDesc  *prometheus.Desc
	dataSource       string // 数据源名称
	dataSourceConfig *config.DataSourceConfig
}

// SetDataSource 实现DataSourceAware接口
func (c *SessionInfoCollector) SetDataSource(name string) {
	c.dataSource = name
}

// SetDataSourceConfig 设置当前采集器对应的数据源配置
func (c *SessionInfoCollector) SetDataSourceConfig(dataSourceConfig *config.DataSourceConfig) {
	c.dataSourceConfig = dataSourceConfig
}

// 定义数据结构
type SessionInfo struct {
	ExecTime     sql.NullFloat64
	SlowSQL      sql.NullString
	SessID       sql.NullString
	CurrSch      sql.NullString
	ThrdID       sql.NullString
	LastRecvTime sql.NullTime
	ConnIP       sql.NullString
}

func NewSlowSessionInfoCollector(db *sql.DB) MetricCollector {
	return &SessionInfoCollector{
		db: db,
		slowSQLInfoDesc: prometheus.NewDesc(
			dmdbms_slow_sql_info,
			"Information about slow SQL statements",
			[]string{"sess_id", "curr_sch", "thrd_id", "last_recv_time", "conn_ip", "slow_sql"},
			nil,
		),
	}
}

func (c *SessionInfoCollector) Describe(ch chan<- *prometheus.Desc) {
	ch <- c.slowSQLInfoDesc
}

func (c *SessionInfoCollector) Collect(ch chan<- prometheus.Metric) {
	dataSourceConfig := c.dataSourceConfig
	if dataSourceConfig == nil {
		dataSourceConfig = config.Global.GetDefaultDataSource()
	}

	if dataSourceConfig == nil || !dataSourceConfig.CheckSlowSQL {
		logger.Logger.Debugf("[%s] CheckSlowSQL is false, skip collecting slow SQL info", c.dataSource)
		return
	}

	if err := utils.CheckDBConnectionWithSource(c.db, c.dataSource); err != nil {
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(dataSourceConfig.QueryTimeout)*time.Second)
	defer cancel()

	rows, err := c.db.QueryContext(ctx, config.QueryDbSlowSqlInfoSqlStr, dataSourceConfig.SlowSqlTime, dataSourceConfig.SlowSqlMaxRows)
	if err != nil {
		utils.HandleDbQueryErrorWithSource(err, c.dataSource)
		return
	}
	defer rows.Close()

	var sessionInfos []SessionInfo
	for rows.Next() {
		var info SessionInfo
		if err := rows.Scan(&info.ExecTime, &info.SlowSQL, &info.SessID, &info.CurrSch, &info.ThrdID, &info.LastRecvTime, &info.ConnIP); err != nil {
			logger.Logger.Error(fmt.Sprintf("[%s] Error scanning row", c.dataSource), zap.Error(err))
			continue
		}
		sessionInfos = append(sessionInfos, info)
	}

	if err := rows.Err(); err != nil {
		logger.Logger.Error(fmt.Sprintf("[%s] Error with rows", c.dataSource), zap.Error(err))
	}
	// 发送数据到 Prometheus
	for _, info := range sessionInfos {
		sessionID := utils.NullStringToString(info.SessID)
		currentSchema := utils.NullStringToString(info.CurrSch)
		threadID := utils.NullStringToString(info.ThrdID)
		lastRecvTime := utils.NullTimeToString(info.LastRecvTime)
		connIP := utils.NullStringToString(info.ConnIP)
		slowSQL := utils.NullStringToString(info.SlowSQL)

		ch <- prometheus.MustNewConstMetric(
			c.slowSQLInfoDesc,
			prometheus.GaugeValue,
			utils.NullFloat64ToFloat64(info.ExecTime),
			sessionID, currentSchema, threadID, lastRecvTime, connIP, slowSQL,
		)
	}
}
