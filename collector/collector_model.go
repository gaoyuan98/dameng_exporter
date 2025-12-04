package collector

import (
	"github.com/prometheus/client_golang/prometheus"
	"sync"
)

var (
	collectors  []prometheus.Collector
	registerMux sync.Mutex
	//timeout     = 5 * time.Second
)

const (
	dameng_exporter_build_info        string = "dameng_exporter_build_info"
	dmdbms_tablespace_file_total_info string = "dmdbms_tablespace_file_total_info"
	dmdbms_tablespace_file_free_info  string = "dmdbms_tablespace_file_free_info"
	dmdbms_tablespace_size_total_info string = "dmdbms_tablespace_size_total_info"
	dmdbms_tablespace_size_free_info  string = "dmdbms_tablespace_size_free_info"
	dmdbms_start_time_info            string = "dmdbms_start_time_info"
	dmdbms_status_info                string = "dmdbms_status_info"
	dmdbms_mode_info                  string = "dmdbms_mode_info"
	dmdbms_trx_num_info               string = "dmdbms_trx_num_info"
	dmdbms_dead_lock_num_total        string = "dmdbms_dead_lock_num_total"
	dmdbms_thread_num_info            string = "dmdbms_thread_num_info"
	dmdbms_switching_occurs           string = "dmdbms_switching_occurs"

	dmdbms_memory_curr_pool_info  string = "dmdbms_memory_curr_pool_info"
	dmdbms_memory_total_pool_info string = "dmdbms_memory_total_pool_info"

	dmdbms_session_type_Info    string = "dmdbms_session_type_info"
	dmdbms_ckpttime_total       string = "dmdbms_ckpttime_total"
	dmdbms_rlog_lsn_total       string = "dmdbms_rlog_lsn_total"
	dmdbms_rlog_file_size_bytes string = "dmdbms_rlog_file_size_bytes"

	dmdbms_joblog_error_num string = "dmdbms_joblog_error_num"

	dmdbms_slow_sql_info                string = "dmdbms_slow_sql_info"
	dmdbms_monitor_info                 string = "dmdbms_monitor_info"
	dmdbms_statement_type_total         string = "dmdbms_statement_type_total"
	dmdbms_parameter_info               string = "dmdbms_parameter_info"
	dmdbms_user_list_info               string = "dmdbms_user_list_info"
	dmdbms_license_date                 string = "dmdbms_license_date"
	dmdbms_version                      string = "dmdbms_version"
	dmdbms_arch_status                  string = "dmdbms_arch_status"
	dmdbms_arch_switch_rate             string = "dmdbms_arch_switch_rate"
	dmdbms_arch_switch_rate_detail_info string = "dmdbms_arch_switch_rate_detail_info"
	dmdbms_arch_status_info             string = "dmdbms_arch_status_info"
	dmdbms_arch_send_detail_info        string = "dmdbms_arch_send_detail_info"
	dmdbms_arch_send_diff_value         string = "dmdbms_arch_send_diff_value"
	dmdbms_arch_queue_waiting_info      string = "dmdbms_arch_queue_waiting_info"
	dmdbms_start_day                    string = "dmdbms_start_day"
	dmdbms_redo_switch_last_time        string = "dmdbms_redo_switch_last_time_seconds"
	dmdbms_redo_switch_interval         string = "dmdbms_redo_switch_interval_seconds"
	dmdbms_rapply_sys_task_mem_used     string = "dmdbms_rapply_sys_task_mem_used"
	dmdbms_rapply_sys_task_num          string = "dmdbms_rapply_sys_task_num"
	dmdbms_rapply_time_diff             string = "dmdbms_rapply_time_diff"
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

	// 系统信息指标
	dmdbms_system_cpu_info    string = "dmdbms_system_cpu_info"
	dmdbms_system_memory_info string = "dmdbms_system_memory_info"
	dmdbms_system_base_info   string = "dmdbms_system_base_info"
	//数据库系统事件指标次数
	dmdbms_system_event_waits_total string = "dmdbms_system_event_waits_total"

	// 数据字典缓存指标
	dmdbms_dict_cache_total string = "dmdbms_dict_cache_total"

	dmdb_up string = "dmdb_up"
)

// MetricCollector 接口
type MetricCollector interface {
	Describe(ch chan<- *prometheus.Desc)
	Collect(ch chan<- prometheus.Metric)
}
