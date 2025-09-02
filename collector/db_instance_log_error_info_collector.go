package collector

import (
	"context"
	"dameng_exporter/config"
	"dameng_exporter/logger"
	"dameng_exporter/utils"
	"database/sql"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"go.uber.org/zap"
)

// 定义数据结构
type InstanceLogInfo struct {
	Txt     sql.NullString
	Level   sql.NullString
	Pid     sql.NullString
	LogTime sql.NullString
}

// 定义收集器结构体
type DbInstanceLogInfoCollector struct {
	db                  *sql.DB
	instanceLogInfoDesc *prometheus.Desc
	dataSource          string // 数据源名称
}

// SetDataSource 实现DataSourceAware接口
func (c *DbInstanceLogInfoCollector) SetDataSource(name string) {
	c.dataSource = name
}

func NewDbInstanceLogErrorCollector(db *sql.DB) MetricCollector {
	return &DbInstanceLogInfoCollector{
		db: db,
		instanceLogInfoDesc: prometheus.NewDesc(
			dmdbms_instance_log_error_info,
			"Information about DM database Instance error log info",
			[]string{"pid", "level", "log_time", "txt"},
			nil,
		),
	}
}

func (c *DbInstanceLogInfoCollector) Describe(ch chan<- *prometheus.Desc) {
	ch <- c.instanceLogInfoDesc
}

func (c *DbInstanceLogInfoCollector) Collect(ch chan<- prometheus.Metric) {

	if err := utils.CheckDBConnectionWithSource(c.db, c.dataSource); err != nil {
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(config.Global.GetQueryTimeout())*time.Second)
	defer cancel()

	rows, err := c.db.QueryContext(ctx, config.QueryInstanceErrorLogSql)
	if err != nil {
		utils.HandleDbQueryErrorWithSource(err, c.dataSource)
		return
	}
	defer rows.Close()

	var instanceLogInfos []InstanceLogInfo
	for rows.Next() {
		var info InstanceLogInfo
		//LOG_TIME,PID,LEVEL$,TXT
		if err := rows.Scan(&info.LogTime, &info.Pid, &info.Level, &info.Txt); err != nil {
			logger.Logger.Error("Error scanning row", zap.Error(err))
			continue
		}
		instanceLogInfos = append(instanceLogInfos, info)
	}

	if err := rows.Err(); err != nil {
		logger.Logger.Error("Error with rows", zap.Error(err))
	}

	// 对instanceLogInfos进行去重处理
	instanceLogInfos = removeDuplicateLogInfos(instanceLogInfos)

	// 发送数据到 Prometheus
	for _, info := range instanceLogInfos {
		//[]string{"pid", "level", "log_time", "txt"}

		pid := utils.NullStringToString(info.Pid)
		level := utils.NullStringToString(info.Level)
		logTime := utils.NullStringToString(info.LogTime)
		txt := utils.NullStringToString(info.Txt)

		//ps: log日志本身就是异常的,所以统一设置为1
		logStatusValue := 1

		ch <- prometheus.MustNewConstMetric(
			c.instanceLogInfoDesc,
			prometheus.GaugeValue,
			float64(logStatusValue),
			pid, level, logTime, txt,
		)
	}
}

// 移除重复的日志记录（保留原始顺序）
func removeDuplicateLogInfos(logs []InstanceLogInfo) []InstanceLogInfo {
	// 使用map来跟踪已经看到的日志
	seen := make(map[string]bool)
	result := []InstanceLogInfo{} // 保留原始顺序的结果集

	// 按原始顺序遍历，只保留第一次出现的元素
	for _, info := range logs {
		// 为每条日志创建一个唯一标识
		pid := utils.NullStringToString(info.Pid)
		level := utils.NullStringToString(info.Level)
		logTime := utils.NullStringToString(info.LogTime)
		txt := utils.NullStringToString(info.Txt)

		key := pid + "|" + level + "|" + logTime + "|" + txt

		// 如果这个日志已经见过，则跳过
		if seen[key] {
			continue
		}

		// 标记为已见，并添加到结果中
		seen[key] = true
		result = append(result, info)
	}

	return result
}
