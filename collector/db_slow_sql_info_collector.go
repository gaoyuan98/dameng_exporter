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

type SessionInfoCollector struct {
	db              *sql.DB
	slowSQLInfoDesc *prometheus.Desc
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
			[]string{"host_name", "sess_id", "curr_sch", "thrd_id", "last_recv_time", "conn_ip", "slow_sql"},
			nil,
		),
	}
}

func (c *SessionInfoCollector) Describe(ch chan<- *prometheus.Desc) {
	ch <- c.slowSQLInfoDesc
}

func (c *SessionInfoCollector) Collect(ch chan<- prometheus.Metric) {
	if !config.GlobalConfig.CheckSlowSQL {
		logger.Logger.Debug("CheckSlowSQL is false, skip collecting slow SQL info")
		return
	}
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

	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(config.GlobalConfig.QueryTimeout)*time.Second)
	defer cancel()

	rows, err := c.db.QueryContext(ctx, config.QueryDbSlowSqlInfoSqlStr, config.GlobalConfig.SlowSqlTime, config.GlobalConfig.SlowSqlMaxRows)
	if err != nil {
		handleDbQueryError(err)
		return
	}
	defer rows.Close()

	var sessionInfos []SessionInfo
	for rows.Next() {
		var info SessionInfo
		if err := rows.Scan(&info.ExecTime, &info.SlowSQL, &info.SessID, &info.CurrSch, &info.ThrdID, &info.LastRecvTime, &info.ConnIP); err != nil {
			logger.Logger.Error("Error scanning row", zap.Error(err))
			continue
		}
		sessionInfos = append(sessionInfos, info)
	}

	if err := rows.Err(); err != nil {
		logger.Logger.Error("Error with rows", zap.Error(err))
	}
	// 发送数据到 Prometheus
	for _, info := range sessionInfos {
		hostName := config.GetHostName()
		sessionID := NullStringToString(info.SessID)
		currentSchema := NullStringToString(info.CurrSch)
		threadID := NullStringToString(info.ThrdID)
		lastRecvTime := NullTimeToString(info.LastRecvTime)
		connIP := NullStringToString(info.ConnIP)
		slowSQL := NullStringToString(info.SlowSQL)

		ch <- prometheus.MustNewConstMetric(
			c.slowSQLInfoDesc,
			prometheus.GaugeValue,
			NullFloat64ToFloat64(info.ExecTime),
			hostName, sessionID, currentSchema, threadID, lastRecvTime, connIP, slowSQL,
		)
	}
}
