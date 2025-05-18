package collector

import (
	"context"
	"dameng_exporter/config"
	"dameng_exporter/db"
	"dameng_exporter/logger"
	"database/sql"
	"errors"
	"strings"
	"sync"

	"github.com/duke-git/lancet/v2/fileutil"
	"github.com/prometheus/client_golang/prometheus"
	"go.uber.org/zap"
)

var (
	collectors  []prometheus.Collector
	registerMux sync.Mutex
	//timeout     = 5 * time.Second
)

const (
	dmdbms_node_uname_info            string = "dmdbms_node_uname_info"
	dmdbms_tablespace_file_total_info string = "dmdbms_tablespace_file_total_info"
	dmdbms_tablespace_file_free_info  string = "dmdbms_tablespace_file_free_info"
	dmdbms_tablespace_size_total_info string = "dmdbms_tablespace_size_total_info"
	dmdbms_tablespace_size_free_info  string = "dmdbms_tablespace_size_free_info"
	dmdbms_start_time_info            string = "dmdbms_start_time_info"
	dmdbms_status_info                string = "dmdbms_status_info"
	dmdbms_mode_info                  string = "dmdbms_mode_info"
	dmdbms_trx_info                   string = "dmdbms_trx_info"
	dmdbms_dead_lock_num_info         string = "dmdbms_dead_lock_num_info"
	dmdbms_thread_num_info            string = "dmdbms_thread_num_info"
	dmdbms_switching_occurs           string = "dmdbms_switching_occurs"
	dmdbms_db_status_occurs           string = "dmdbms_db_status_occurs"

	dmdbms_memory_curr_pool_info  string = "dmdbms_memory_curr_pool_info"
	dmdbms_memory_total_pool_info string = "dmdbms_memory_total_pool_info"

	dmdbms_session_percentage string = "dmdbms_session_percentage"
	dmdbms_session_type_Info  string = "dmdbms_session_type_info"
	dmdbms_ckpttime_info      string = "dmdbms_ckpttime_info"

	dmdbms_joblog_error_num string = "dmdbms_joblog_error_num"

	dmdbms_slow_sql_info            string = "dmdbms_slow_sql_info"
	dmdbms_monitor_info             string = "dmdbms_monitor_info"
	dmdbms_statement_type_info      string = "dmdbms_statement_type_info"
	dmdbms_parameter_info           string = "dmdbms_parameter_info"
	dmdbms_user_list_info           string = "dmdbms_user_list_info"
	dmdbms_license_date             string = "dmdbms_license_date"
	dmdbms_version                  string = "dmdbms_version"
	dmdbms_arch_status              string = "dmdbms_arch_status"
	dmdbms_arch_switch_rate         string = "dmdbms_arch_switch_rate"
	dmdbms_arch_status_info         string = "dmdbms_arch_status_info"
	dmdbms_arch_send_detail_info    string = "dmdbms_arch_send_detail_info"
	dmdbms_start_day                string = "dmdbms_start_day"
	dmdbms_rapply_sys_task_mem_used string = "dmdbms_rapply_sys_task_mem_used"
	dmdbms_rapply_sys_task_num      string = "dmdbms_rapply_sys_task_num"
	//dmdbms_instance_log_error_info  string = "dmdbms_instance_log_error_info"
	dmdbms_dmap_process_is_exit      string = "dmdbms_dmap_process_is_exit"
	dmdbms_dmserver_process_is_exit  string = "dmdbms_dmserver_process_is_exit"
	dmdbms_dmwatcher_process_is_exit string = "dmdbms_dmwatcher_process_is_exit"
	dmdbms_dmmonitor_process_is_exit string = "dmdbms_dmmonitor_process_is_exit"
	dmdbms_dmagent_process_is_exit   string = "dmdbms_dmagent_process_is_exit"

	dmdbms_instance_log_error_info string = "dmdbms_instance_log_error_info"
	//DW守护进程的状态
	dmdbms_dw_watcher_info string = "dmdbms_dw_watcher_info"
	//DM缓冲池的命中率
	dmdbms_bufferpool_info string = "dmdbms_bufferpool_info"
	//DM的dual
	dmdbms_dual_info string = "dmdbms_dual_info"
	//回滚段信息
	dmdbms_purge_objects_info string = "dmdbms_purge_objects_info"
)

// MetricCollector 接口
type MetricCollector interface {
	Describe(ch chan<- *prometheus.Desc)
	Collect(ch chan<- prometheus.Metric)
}

// 注册所有的收集器
func RegisterCollectors(reg *prometheus.Registry) {
	registerMux.Lock()
	defer registerMux.Unlock()
	logger.Logger.Debugf("exporter running system is %v", GetOS())

	collectors = append(collectors, NewSystemInfoCollector())
	//
	if config.GlobalConfig.RegisterHostMetrics && strings.Compare(GetOS(), OS_LINUX) == 0 {
		collectors = append(collectors, NewDmapProcessCollector(db.DBPool))
		//collectors = append(collectors, NewExampleCounterCollector())
	}
	if config.GlobalConfig.RegisterDatabaseMetrics {
		collectors = append(collectors, NewTableSpaceDateFileInfoCollector(db.DBPool))
		collectors = append(collectors, NewTableSpaceInfoCollector(db.DBPool))
		collectors = append(collectors, NewDBInstanceRunningInfoCollector(db.DBPool))
		collectors = append(collectors, NewDbMemoryPoolInfoCollector(db.DBPool))
		collectors = append(collectors, NewDBSessionsStatusCollector(db.DBPool))
		collectors = append(collectors, NewDbJobRunningInfoCollector(db.DBPool))
		collectors = append(collectors, NewSlowSessionInfoCollector(db.DBPool))
		collectors = append(collectors, NewMonitorInfoCollector(db.DBPool))
		collectors = append(collectors, NewDbSqlExecTypeCollector(db.DBPool))
		collectors = append(collectors, NewIniParameterCollector(db.DBPool))
		collectors = append(collectors, NewDbUserCollector(db.DBPool))
		collectors = append(collectors, NewDbLicenseCollector(db.DBPool))
		collectors = append(collectors, NewDbVersionCollector(db.DBPool))
		collectors = append(collectors, NewDbArchStatusCollector(db.DBPool))
		collectors = append(collectors, NewDbRapplySysCollector(db.DBPool))
		//回滚段信息
		collectors = append(collectors, NewPurgeCollector(db.DBPool))
		//与DM数据库的归档状态
		//collectors = append(collectors, NewInstanceLogErrorCollector(db.DBPool))
		collectors = append(collectors, NewCkptCollector(db.DBPool))
		//查询实例的异常日志(近5分钟内)
		collectors = append(collectors, NewDbInstanceLogErrorCollector(db.DBPool))
		//Dw集群进程信息
		collectors = append(collectors, NewDbDwWatcherInfoCollector(db.DBPool))
		//数据库缓冲池的命中率
		collectors = append(collectors, NewDbBufferPoolCollector(db.DBPool))
		//dual
		collectors = append(collectors, NewDbDualCollector(db.DBPool))

	}
	if config.GlobalConfig.RegisterDmhsMetrics {
		// 添加中间件指标收集器
		// collectors = append(collectors, NewMiddlewareCollector())
	}
	// 添加自定义指标收集器
	if config.GlobalConfig.RegisterCustomMetrics && fileutil.IsExist(config.GlobalConfig.CustomMetricsFile) {

		customConfig, customErr := config.ParseCustomConfig(config.GlobalConfig.CustomMetricsFile)
		if customErr != nil {
			logger.Logger.Error("解析自定义metrics指标配置文件失败", zap.Error(customErr))
		} else {
			if len(customConfig.Metrics) > 0 {
				// 创建 CustomMetrics 实例并注册
				customMetrics := NewCustomMetrics(db.DBPool, customConfig)
				reg.MustRegister(customMetrics)
			}
		}

		// collectors = append(collectors, NewCustomCollector())
	}

	for _, collector := range collectors {
		reg.MustRegister(collector)
	}
}

// 卸载所有的收集器
func UnregisterCollectors(reg *prometheus.Registry) {
	registerMux.Lock()
	defer registerMux.Unlock()

	for _, collector := range collectors {
		reg.Unregister(collector)
	}
	collectors = nil
}

// 封装数据库连接检查逻辑
func checkDBConnection(db *sql.DB) error {
	if err := db.Ping(); err != nil {
		logger.Logger.Error("Database connection is not available", zap.Error(err))
		return err
	}
	return nil
}

// 封装通用的错误处理逻辑
func handleDbQueryError(err error) {
	if errors.Is(err, context.DeadlineExceeded) {
		logger.Logger.Error("Query timed out", zap.Error(err))
	} else {
		logger.Logger.Error("Error querying database", zap.Error(err))
	}
}
