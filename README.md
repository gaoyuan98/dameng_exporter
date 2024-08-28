<h1 align="center">DAMENG_EXPORTER的介绍说明</h1>

# 介绍
1. DM数据库适配prometheus监控的采集器，目前已支持DM8数据库同时提供grafana 8.5.X 以上版本的监控面板（其他的grafana版本需要自己绘制表盘）。
2. 已支持的指标如下
```
   数据库线程数	dmdbms_thread_num_info
   数据库事务等待数	dmdbms_trx_info
   数据库死锁数	dmdbms_dead_lock_num_info
   数据库的状态	dmdbms_status_info
   数据库启动时间	dmdbms_start_time_info
   数据库QPS数量	dmdbms_qps_count
   数据库TPS数量	dmdbms_tps_count
   主备集群同步延迟	dmdbms_rapply_stat
   表空间总大小	dmdbms_tablespace_size_total_info
   表空间空闲大小	dmdbms_tablespace_size_free_info
   表空间数据文件总大小	dmdbms_tablespace_file_total_info
   表空间数据文件空闲大小	dmdbms_tablespace_file_free_info
   数据库会话数状态	dmdbms_session_type_info
   数据库实例的错误事件	dmdbms_instance_log_error_info
   查询内存池的当前使用状态	dmdbms_memory_curr_pool_info
   查询内存池的配置上限	dmdbms_memory_total_pool_info
   数据库活动会话执行延迟监控	dmdbms_waiting_session
   数据库的最大连接数	dmdbms_connect_session
   数据库授权查询	dmdbms_license_date
   数据库定时任务错误	dmdbms_joblog_error_num
   监控监视器进程	dmdbms_monitor_info
   监控慢SQL语句	dmdbms_slow_sql_info
   备库重演运行线程数	dmdbms_rapply_sys_task_num
   备库重演内存堆积信息	dmdbms_rapply_sys_task_mem_used
   数据库语句类型数量展示逻辑	dmdbms_statement_type_info
   检查点更新	dmdbms_ckpttime_info
   检查用户信息	dmdbms_user_list_info
   数据库版本	dmdbms_version
   数据库启动天数	dmdbms_start_day
   数据库归档状态	dmdbms_arch_status
   dmap进程探活	dmdbms_dmap_process_is_exit
   dmserver进程探活	dmdbms_dmserver_process_is_exit
   dmwatcher进程探活	dmdbms_dmwatcher_process_is_exit
   dmmonitor进程探活	dmdbms_dmmonitor_process_is_exit
   dmagent进程探活	dmdbms_dmagent_process_is_exit
```
3. 源码解析地址：https://blog.csdn.net/qq_35349982/article/details/140698149
# 目录
- doc目录存放的是相关的配置文件（告警模板、配置模板、表盘）
- collector存放的是各个指标的采集逻辑
- build_all_versions.bat为window的一键编译脚本

# 搭建效果图
<img src="./img/tubiao_01.png" width="1000" height="500" />
<br />
<img src="./img/tubiao_02.png" width="1000" height="500" />
<br />

# 搭建步骤
可查看这个：https://blog.csdn.net/qq_35349982/article/details/140700625
## 1. 下载编译的exporter包
https://github.com/gaoyuan98/dameng_exporter/releases
```
dameng_exporter_v1.0.0_linux_amd64.tar.gz（linux_x86平台）
dameng_exporter_v1.0.0_linux_arm64.tar.gz（linux_arm平台）
dameng_exporter_v1.0.0_windows_amd64.tar.gz（window_x64平台）
```

## 2. 新建用户权限
```sql
## 最小化权限
## 条件允许的话 最好赋予DBA权限
create tablespace "PROMETHEUS.DBF" datafile 'PROMETHEUS.DBF' size 512 CACHE = NORMAL;
create user "PROMETHEUS" identified by "PROMETHEUS";
alter user "PROMETHEUS" default tablespace "PROMETHEUS.DBF" default index tablespace "PROMETHEUS.DBF";
grant "PUBLIC","RESOURCE","SOI","SVI","VTI" to "PROMETHEUS";
grant select on DBA_FREE_SPACE to PROMETHEUS;
grant select on DBA_DATA_FILES to PROMETHEUS;
grant select on DBA_USERS to PROMETHEUS;
grant select on V$SESSIONS to PROMETHEUS;
```
## 3. 在数据库上运行
1. 解压压缩包
2. 修改dameng_exporter.config配置文件的数据库账号及密码
注意：程序运行后会自动对数据库密码部分进行密文处理，不用担心密码泄露问题
3. 启动exporter程序
```
## 启动服务
[root@VM-24-17-centos dm_prometheus]#  nohup  ./dameng_exporter_v1.0.0_linux_amd64 > /dev/null 2>&1 &
## 2. 访问接口
##  通过浏览器访问http://被监控端IP:9200/metrics
[root@server ~]# lsof -i:9200
```
## 4. 在prometheus上进行配置
修改prometheus的prometheus.yml配置文件
```
# 添加的是数据库监控的接口9200接口，如果是一套集群，则在targets标签后进行逗号拼接，如下图所示
# 注意 cluster_name标签不能改，提供的模板用该标签做分类
- job_name: "dm_db_single"
  static_configs:
   - targets: ["192.168.112.135:9200"]
     labels:
     cluster_name: '单机测试'
```
<br />


## 5. 在grafana上导入提供的表盘
1. 登录grafana登录，导入模板
   <br />
   <img src="./img/grafana_01.png" width="1000" height="500" />
2. 所使用的模板在表盘中
   <br />
   <img src="./img/grafana_02.png" width="1000" height="500" />

3. 效果图
   <br />
   <img src="./img/grafana_03.png" width="1000" height="500" />


# 6. 自定义指标
在exporter的同级目录下创建一个custom_metrics.toml文件，注意文件权限,编写SQL即可。写法与(oracledb_exporter)类似
这是一个简单的例子：
```
[[metric]]
context = "context_with_labels"
request = "SELECT 1 as value_1, 2 as value_2, 'First label' as label_1, 'Second label' as label_2 FROM DUAL"
metricsdesc = { value_1 = "Simple example returning always 1 as counter.", value_2 = "Same but returning always 2 as gauge." }
```
该文件在导出器中生成以下条目：
```
# HELP dmdbms_value_1 Simple example returning always 1 as counter.
# TYPE dmdbms_value_1 gauge
dmdbms_value_1{host_name="gy"} 1
# HELP dmdbms_value_2 Same but returning always 2 as gauge.
# TYPE dmdbms_value_2 gauge
dmdbms_value_2{host_name="gy"} 2
```

自定义标签的例子:
```
[[metric]]
context = "context_with_labels"
labels = [ "label_1", "label_2" ]
request = "SELECT 1 as value_1, 2 as value_2, 'First label' as label_1, 'Second label' as label_2 FROM DUAL"
metricsdesc = { value_1 = "Simple example returning always 1 as counter.", value_2 = "Same but returning always 2 as gauge." }
# Can be counter or gauge (default)
metricstype = { value_1 = "counter" }
```
该文件在导出器中生成以下条目：
```
# HELP dmdbms_value_1 Simple example returning always 1 as counter.
# TYPE dmdbms_value_1 counter
dmdbms_value_1{host_name="gy",label_1="First label",label_2="Second label"} 1
# HELP dmdbms_value_2 Same but returning always 2 as gauge.
# TYPE dmdbms_value_2 gauge
dmdbms_value_2{host_name="gy",label_1="First label",label_2="Second label"} 2
```

# 更新记录
## v1.0.2
1. 新增自定义SQL指标的功能（在exporter的同级目录下创建一个custom_metrics.toml文件即可，写法与（oracledb_exporter相同）
