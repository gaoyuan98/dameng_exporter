package collector

import (
	"context"
	"dameng_exporter/config"
	"dameng_exporter/logger"
	"database/sql"
	"github.com/prometheus/client_golang/prometheus"
	"go.uber.org/zap"
	"strconv"
	"time"
)

type DBInstanceRunningInfoCollector struct {
	db                  *sql.DB
	startTimeDesc       *prometheus.Desc
	statusDesc          *prometheus.Desc
	modeDesc            *prometheus.Desc
	trxNumDesc          *prometheus.Desc
	deadlockDesc        *prometheus.Desc
	threadNumDesc       *prometheus.Desc
	statusOccursDesc    *prometheus.Desc
	switchingOccursDesc *prometheus.Desc
}

const (
	DB_INSTANCE_STATUS_MOUNT_2   float64 = 2
	DB_INSTANCE_STATUS_SUSPEND_3 float64 = 3
	AlarmStatus_Normal                   = 1
	AlarmStatus_Unusual                  = 0
	AlarmSwitchOccur                     = "InitiateAnAlarm_SwitchOccur"
	AlarmSwitchStr                       = "switchingOccurStr"
)

func NewDBInstanceRunningInfoCollector(db *sql.DB) MetricCollector {
	return &DBInstanceRunningInfoCollector{
		db: db,
		startTimeDesc: prometheus.NewDesc(
			dmdbms_start_time_info,
			"Database status time",
			[]string{"host_name"}, // 添加标签
			nil,
		),
		statusDesc: prometheus.NewDesc(
			dmdbms_status_info,
			"Database status",
			[]string{"host_name"}, // 添加标签
			nil,
		),
		modeDesc: prometheus.NewDesc(
			dmdbms_mode_info,
			"Database mode",
			[]string{"host_name"}, // 添加标签
			nil,
		),
		trxNumDesc: prometheus.NewDesc(
			dmdbms_trx_info,
			"Number of transactions",
			[]string{"host_name"}, // 添加标签
			nil,
		),
		deadlockDesc: prometheus.NewDesc(
			dmdbms_dead_lock_num_info,
			"Number of deadlocks",
			[]string{"host_name"}, // 添加标签
			nil,
		),
		threadNumDesc: prometheus.NewDesc(
			dmdbms_thread_num_info,
			"Number of threads",
			[]string{"host_name"}, // 添加标签
			nil,
		),
		statusOccursDesc: prometheus.NewDesc( //这个是数据库状态切换的标识  OPEN
			dmdbms_db_status_occurs,
			"status changes status, error is 0 , true is 1",
			[]string{"host_name"}, // 添加标签
			nil,
		),
		switchingOccursDesc: prometheus.NewDesc( //这个是集群切换的标识
			dmdbms_switching_occurs,
			"Database instance switching occurs， error is 0 , true is 1  ",
			[]string{"host_name"}, // 添加标签
			nil,
		),
	}
}

func (c *DBInstanceRunningInfoCollector) Describe(ch chan<- *prometheus.Desc) {
	ch <- c.startTimeDesc
	ch <- c.statusDesc
	ch <- c.modeDesc
	ch <- c.trxNumDesc
	ch <- c.deadlockDesc
	ch <- c.threadNumDesc
	ch <- c.statusOccursDesc
	ch <- c.switchingOccursDesc
}

func (c *DBInstanceRunningInfoCollector) Collect(ch chan<- prometheus.Metric) {

	if err := checkDBConnection(c.db); err != nil {
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(config.GlobalConfig.QueryTimeout)*time.Second)
	defer cancel()

	rows, err := c.db.QueryContext(ctx, config.QueryDBInstanceRunningInfoSqlStr)
	if err != nil {
		handleDbQueryError(err)
		return
	}
	defer rows.Close()

	var status, mode, trxNum, deadlockNum, threadNum float64
	var startTimeUnix int64
	if rows.Next() {
		var startTimeStr, statusStr, modeStr, trxNumStr, deadlockNumStr, threadNumStr string
		if err := rows.Scan(&startTimeStr, &statusStr, &modeStr, &trxNumStr, &deadlockNumStr, &threadNumStr); err != nil {
			logger.Logger.Error("Error scanning row", zap.Error(err))
			return
		}
		status, _ = strconv.ParseFloat(statusStr, 64)
		mode, _ = strconv.ParseFloat(modeStr, 64)
		trxNum, _ = strconv.ParseFloat(trxNumStr, 64)
		deadlockNum, _ = strconv.ParseFloat(deadlockNumStr, 64)
		threadNum, _ = strconv.ParseFloat(threadNumStr, 64)

		// 解析时间戳字符串为 time.Time 类型
		startTime, err := time.Parse("2006-01-02 15:04:05", startTimeStr)
		if err != nil {
			logger.Logger.Error("Error parsing start time", zap.Error(err))
			// 如果转换失败则赋予默认时间值
			var defaultTime = time.Date(2006, time.January, 1, 0, 0, 0, 0, time.UTC)
			startTime = defaultTime
		}
		// 获取秒级 Unix 时间戳
		startTimeUnix = startTime.Unix()

	}

	var statusOccurs = 0
	//对值进行二次封装处理

	//判断实例状态是否正常，异常时值为0 正常时为1
	//eg： 此处是为了兼容java版本的报错
	if status == DB_INSTANCE_STATUS_MOUNT_2 || status == DB_INSTANCE_STATUS_SUSPEND_3 {
		statusOccurs = 0
	} else {
		statusOccurs = 1
	}

	data := map[string]float64{
		"startTime":    float64(startTimeUnix),
		"status":       status,
		"mode":         mode,
		"trxNum":       trxNum,
		"deadlockNum":  deadlockNum,
		"threadNum":    threadNum,
		"statusOccurs": float64(statusOccurs),
	}
	// 处理数据库模式切换的逻辑（主备集群）
	c.handleDatabaseModeSwitch(ch, mode)

	//注册指标
	c.collectMetrics(ch, data)
	logger.Logger.Debugf("Collector DBInstanceRunningInfoCollector success,status: %v", status)

}

func (c *DBInstanceRunningInfoCollector) collectMetrics(ch chan<- prometheus.Metric, data map[string]float64) {
	ch <- prometheus.MustNewConstMetric(c.startTimeDesc, prometheus.GaugeValue, data["startTime"], config.GetHostName())
	ch <- prometheus.MustNewConstMetric(c.statusDesc, prometheus.GaugeValue, data["status"], config.GetHostName())
	ch <- prometheus.MustNewConstMetric(c.modeDesc, prometheus.GaugeValue, data["mode"], config.GetHostName())
	ch <- prometheus.MustNewConstMetric(c.trxNumDesc, prometheus.GaugeValue, data["trxNum"], config.GetHostName())
	ch <- prometheus.MustNewConstMetric(c.deadlockDesc, prometheus.GaugeValue, data["deadlockNum"], config.GetHostName())
	ch <- prometheus.MustNewConstMetric(c.threadNumDesc, prometheus.GaugeValue, data["threadNum"], config.GetHostName())
	ch <- prometheus.MustNewConstMetric(c.statusOccursDesc, prometheus.GaugeValue, data["status"], config.GetHostName())
}

/*
*
Case 1 (switchOccurExists)：如果 AlarmSwitchOccur 缓存键存在，表示之前发生过切换，设置 switchingOccursDesc 为 AlarmStatus_Unusual。
Case 2 (modeExists && cachedMode == modeStr)：如果 AlarmSwitchStr 缓存键存在且模式没有变化，设置 switchingOccursDesc 为 AlarmStatus_Normal。
Case 3 (modeExists)：如果 AlarmSwitchStr 缓存键存在但模式发生变化，设置 switchingOccursDesc 为 AlarmStatus_Unusual，并更新缓存。
Default Case：如果 AlarmSwitchStr 缓存键不存在，设置 switchingOccursDesc 为 AlarmStatus_Normal 并更新缓存。
*/
func (c *DBInstanceRunningInfoCollector) handleDatabaseModeSwitch(ch chan<- prometheus.Metric, mode float64) {
	modeStr := strconv.FormatFloat(mode, 'f', -1, 64)

	cachedModeValue, modeExists := config.GetFromCache(AlarmSwitchStr) //这个key存储的是 mode值
	switchOccurExists := config.GetKeyExists(AlarmSwitchOccur)         //这个key表示已经发生切换了，保留的时间

	switch {
	case switchOccurExists:
		ch <- prometheus.MustNewConstMetric(c.switchingOccursDesc, prometheus.GaugeValue, AlarmStatus_Unusual, config.GetHostName())
	case modeExists && cachedModeValue == modeStr:
		ch <- prometheus.MustNewConstMetric(c.switchingOccursDesc, prometheus.GaugeValue, AlarmStatus_Normal, config.GetHostName())
	case modeExists:
		ch <- prometheus.MustNewConstMetric(c.switchingOccursDesc, prometheus.GaugeValue, AlarmStatus_Unusual, config.GetHostName())
		config.DeleteFromCache(AlarmSwitchStr)
		config.SetCache(AlarmSwitchOccur, strconv.Itoa(AlarmStatus_Unusual), 30*time.Minute)
	default:
		config.SetCache(AlarmSwitchStr, modeStr, 30*time.Minute)
		ch <- prometheus.MustNewConstMetric(c.switchingOccursDesc, prometheus.GaugeValue, AlarmStatus_Normal, config.GetHostName())
	}
}

/*
func (c *DBInstanceRunningInfoCollector) handleDatabaseModeSwitch(ch chan<- prometheus.Metric, mode float64) {
	//，'f'表示以小数形式输出，-1表示将所有小数位都输出，64表示mode的类型是float64。
	modeStr := strconv.FormatFloat(mode, 'f', -1, 64)

	if config.GetKeyExists(AlarmSwitchOccur) {
		// 如果key存在表名发生过切换
		ch <- prometheus.MustNewConstMetric(c.switchingOccursDesc, prometheus.GaugeValue, AlarmStatus_Unusual, config.GetHostName())
	} else {
		// 判断是否发生切换
		if config.GetKeyExists(AlarmSwitchStr) {
			// 判断模式是否发生切换
			if cachedMode, found := config.GetFromCache(AlarmSwitchStr); found && cachedMode == modeStr {
				// 模式未发生变化
				ch <- prometheus.MustNewConstMetric(c.switchingOccursDesc, prometheus.GaugeValue, AlarmStatus_Normal, config.GetHostName())
			} else {
				// 模式发生变化
				ch <- prometheus.MustNewConstMetric(c.switchingOccursDesc, prometheus.GaugeValue, AlarmStatus_Unusual, config.GetHostName())
				config.DeleteFromCache(AlarmSwitchStr)
				config.SetCache(AlarmSwitchOccur, strconv.Itoa(AlarmStatus_Unusual), 30*time.Minute)
			}
		} else {
			// 第一次出现，更新缓存
			config.SetCache(AlarmSwitchStr, modeStr, 30*time.Minute)
			ch <- prometheus.MustNewConstMetric(c.switchingOccursDesc, prometheus.GaugeValue, AlarmStatus_Normal, config.GetHostName())
		}
	}
}
*/
