<h1 align="center">DAMENG_EXPORTER的介绍说明</h1>

# 介绍
1. DM数据库适配prometheus监控的采集器，目前已支持DM8数据库同时提供grafana 8.5.X 以上版本的监控面板（其他的grafana版本需要自己绘制表盘）。
2. 已支持的指标如下,具体的实现逻辑请查看doc目录下的xlsx表格
   <img src="./img/support_lable.png"  />

3. 如果有问题，欢迎提issue。如该项目对您有用请点亮右上角的starred
# 目录
- doc目录存放的是相关的配置文件（告警模板、配置模板、表盘）
- collector存放的是各个指标的采集逻辑
- build_all_versions.bat为window的一键编译脚本

# 参数
```
# 每个版本存在差异,以每个版本实际结果为准
[root@VM-24-16-centos opt]# ./dameng_exporter_v1.0.9_linux_amd64 --help
usage: dameng_exporter_v1.0.9_linux_amd64 [<flags>]


Flags:
  --[no-]help                   Show context-sensitive help (also try --help-long and --help-man).
  --configFile="./dameng_exporter.config"  
                                Path to configuration file
  --listenAddress=":9200"       Address to listen on
  --metricPath="/metrics"       Path for metrics
  --dbHost="127.0.0.1:5236"     Database Host
  --dbUser="SYSDBA"             Database user
  --dbPwd="SYSDBA"              Database password
  --queryTimeout=30             Timeout for queries (Second)
  --maxOpenConns=10             Maximum open connections (number)
  --maxIdleConns=1              Maximum idle connections (number)
  --connMaxLifetime=30          Connection maximum lifetime (Minute)
  --[no-]checkSlowSqL           Check slow SQL,default:false
  --slowSqLTime=10000           Slow SQL time (Millisecond)
  --slowSqlLimitRows=10         Slow SQL return limit row
  --[no-]registerHostMetrics    Register host metrics,default:true
  --[no-]registerDatabaseMetrics Register database metrics,default:true
  --[no-]registerDmhsMetrics    Register dmhs metrics,default:false
  --[no-]registerCustomMetrics  Register custom metrics,default:true
  --bigKeyDataCacheTime=60      Big key data cache time (Minute)
  --alarmKeyCacheTime=60        Alarm key cache time (Minute)
  --logMaxSize=10               Maximum log file size(MB)
  --logMaxBackups=3             Maximum log file backups (number)
  --logMaxAge=30                Maximum log file age (Day)
  --encryptPwd=""               Password to encrypt and exit
  --[no-]encodeConfigPwd        Encode the password in the config file,default:true
```
# 搭建效果图
<img src="./img/tubiao_01.png" width="1000" height="500" />
<br />
<img src="./img/tubiao_02.png" width="1000" height="500" />
<br />
<img src="./img/grafana_04.png" width="1000" height="500" />
<br />

# docker镜像拉取
```shell
## linux amd64版本
## 拉取镜像
docker pull registry.cn-hangzhou.aliyuncs.com/dameng_exporter/dameng_exporter:v1.0.9_amd64
## 更换别名
docker tag registry.cn-hangzhou.aliyuncs.com/dameng_exporter/dameng_exporter:v1.0.9_amd64 dameng_exporter:v1.0.9_amd64
## 启动
docker run -d --name dameng_exporter_amd64 -p 9200:9200 dameng_exporter:v1.0.9_amd64 --dbHost="ip地址:端口(192.168.121.001:5236)" --dbUser="SYSDBA" --dbPwd="数据库密码(SYSDBA)"


## linux arm64版本
## 拉取镜像
docker pull registry.cn-hangzhou.aliyuncs.com/dameng_exporter/dameng_exporter:v1.0.9_arm64
## 更换别名
docker tag registry.cn-hangzhou.aliyuncs.com/dameng_exporter/dameng_exporter:v1.0.9_arm64 dameng_exporter:v1.0.9_arm64
## 启动
docker run -d --name dameng_exporter_arm64 -p 9200:9200 dameng_exporter:v1.0.9_arm64 --dbHost="ip地址:端口(192.168.121.001:5236)" --dbUser="SYSDBA" --dbPwd="数据库密码(SYSDBA)"
```

# 微信公众号
扫码或微信公众号搜索“达梦课代表”分享DM数据库一线遇到的各类问题
<br />
<img src="./img/gzh01.png" />
<br />


# 搭建步骤
相关文章如下，如有问题提issue
1) dameng_exporter部署对接prometheus: https://mp.weixin.qq.com/s/l8LB0nZeevtyrtD_oa9oiQ
2) prometheus配置DM的全局的告警面板: https://blog.csdn.net/qq_35349982/article/details/144426840
3) dameng_exporter中如何开启监控慢sql功能: https://mp.weixin.qq.com/s/FMzbrVjwC-6UdAIopg65wA

## 1. 下载已经编译好的exporter包
https://github.com/gaoyuan98/dameng_exporter/releases
```
// 根据需要安装的平台下载已经编译好的包
dameng_exporter_v1.X_linux_amd64.tar.gz（linux_x86平台）
dameng_exporter_v1.X_linux_arm64.tar.gz（linux_arm平台）
dameng_exporter_v1.X_windows_amd64.tar.gz（window_x64平台）
```

## 2. 新建用户权限
```sql
## 新建用户
create tablespace "PROMETHEUS.DBF" datafile 'PROMETHEUS.DBF' size 512 CACHE = NORMAL;
create user "PROMETHEUS" identified by "PROMETHEUS";
alter user "PROMETHEUS" default tablespace "PROMETHEUS.DBF" default index tablespace "PROMETHEUS.DBF";
## 条件允许的话 最好赋予DBA权限(避免后期因升级exporter版本而导致查询权限不足)
grant "PUBLIC","RESOURCE","SOI","SVI","VTI" to "PROMETHEUS";
## 最小化权限
GRANT SELECT ON V$SYSSTAT TO PROMETHEUS;
GRANT SELECT ON V$SESSIONS TO PROMETHEUS;
GRANT SELECT ON V$LICENSE TO PROMETHEUS;
GRANT SELECT ON V$DATABASE TO PROMETHEUS;
GRANT SELECT ON V$DM_INI TO PROMETHEUS;
GRANT SELECT ON V$RLOGFILE TO PROMETHEUS;
GRANT SELECT ON V$TABLESPACE TO PROMETHEUS;
GRANT SELECT ON V$DATAFILE TO PROMETHEUS;
GRANT SELECT ON DBA_DATA_FILES TO PROMETHEUS;
GRANT SELECT ON DBA_FREE_SPACE TO PROMETHEUS;
GRANT SELECT ON V$TRXWAIT TO PROMETHEUS;
GRANT SELECT ON V$CKPT TO PROMETHEUS;
GRANT SELECT ON V$RAPPLY_SYS TO PROMETHEUS;
GRANT SELECT ON V$PROCESS TO PROMETHEUS;
GRANT SELECT ON V$LOCK TO PROMETHEUS;
GRANT SELECT ON V$THREADS TO PROMETHEUS;
```
## 3. 在数据库上运行
1. 解压压缩包
2. 修改dameng_exporter.config配置文件的数据库账号及密码
注意：程序运行后会自动对数据库密码部分进行密文处理，不用担心密码泄露问题
3. 启动exporter程序
```
# 启动程序

## 1） 第一种方式：直接启动
## 直接启动exporter程序后缀不跟参数,此时会自动使用同级目录下dameng_exporter.config配置文件的数据库账号及密码
[root@VM-24-17-centos dm_prometheus]#  nohup ./dameng_exporter_linux_amd64 > /dev/null 2>&1 &

## 2) 第二种方式：添加参数形式启动  ./dameng_exporter_arm64 --help可以查看参数
[root@VM-24-17-centos dm_prometheus]#  nohup ./dameng_exporter_linux_amd64 --listenAddress=":9200" --dbHost="ip地址:端口(192.168.121.001:5236)" --dbUser="SYSDBA" --dbPwd="数据库密码(SYSDBA)"

# 通过浏览器访问http://被监控端IP:9200/metrics
[root@server ~]# lsof -i:9200
```
## 4. 在prometheus上进行配置
修改prometheus的prometheus.yml配置文件
```
# 添加的是数据库监控的接口9200接口，如果是一套集群，则在targets标签后进行逗号拼接，如下图所示
# 注意 cluster_name标签名不能改，标签的值可以改，提供的模板用该标签做分类
# 每套集群的job_name和cluster_name的值需要保证全局唯一

# 单机示例
- job_name: "dm_db_single"
  static_configs:
   - targets: ["192.168.112.135:9200"]
     labels:
     cluster_name: '单机测试'
     
# 集群示例
- job_name: "dmdbms_bgoak_dw"
 static_configs:
   - targets: ["192.168.112.135:9200","192.168.112.136:9200"]
     labels:
       cluster_name: 'OA集群DW'     
```
<br />


## 5. 在grafana上导入提供的表盘
> 表盘模板在doc目录下
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

## 1. 简单语句
```
[[metric]]
context = "context_with_labels"
request = "SELECT 1 as value_1, 2 as value_2, 'First label' as label_1, 'Second label' as label_2 FROM DUAL"
metricsdesc = { value_1 = "Simple example returning always 1 as counter.", value_2 = "Same but returning always 2 as gauge." }
```
该文件在导出器中生成以下条目：
```
# HELP dmdbms_context_no_label_value_1 Simple example returning always 1.
# TYPE dmdbms_context_no_label_value_1 gauge
dmdbms_context_no_label_value_1{host_name="gy"} 1
# HELP dmdbms_context_no_label_value_2 Same but returning always 2.
# TYPE dmdbms_context_no_label_value_2 gauge
dmdbms_context_no_label_value_2{host_name="gy"} 2
```
## 2. 自定义指标的lable
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
# HELP dmdbms_context_with_labels_value_1 Simple example returning always 1 as counter.
# TYPE dmdbms_context_with_labels_value_1 counter
dmdbms_context_with_labels_value_1{host_name="gy",label_1="First label",label_2="Second label"} 1
# HELP dmdbms_context_with_labels_value_2 Same but returning always 2 as gauge.
# TYPE dmdbms_context_with_labels_value_2 gauge
dmdbms_context_with_labels_value_2{host_name="gy",label_1="First label",label_2="Second label"} 2
```
## 3.查询表空间数据文件的大小
```
[[metric]]
context = "test_table_metrics"
labels = [ "name"]
request = "SELECT name,TO_CHAR(TOTAL_SIZE*PAGE/1024/1024) AS total_size_mb FROM SYS.V$TABLESPACE"
metricsdesc = { total_size_mb = "Simple example"}
```
该文件在导出器中生成以下条目：
```
# HELP dmdbms_test_table_metrics_total_size_mb Simple example
# TYPE dmdbms_test_table_metrics_total_size_mb gauge
dmdbms_test_table_metrics_total_size_mb{host_name="gy",name="DMEAGLE"} 1024
dmdbms_test_table_metrics_total_size_mb{host_name="gy",name="DMEAGLE_DEV"} 1024
dmdbms_test_table_metrics_total_size_mb{host_name="gy",name="MAIN"} 2176
dmdbms_test_table_metrics_total_size_mb{host_name="gy",name="ROLL"} 128
dmdbms_test_table_metrics_total_size_mb{host_name="gy",name="SYSTEM"} 138
dmdbms_test_table_metrics_total_size_mb{host_name="gy",name="TEMP"} 74
```


# 更新记录
## v1.1.0
1. 新增对每个指标的逻辑介绍,将描述放到doc目录下的xlsx表格中
2. 新增归档切换频率的指标（dmdbms_arch_switch_rate）| 实现逻辑始终返回最新的这个归档的创建时间跟上个归档的创建时间 相减就是归档切换时间
3. 新增数据守护集群中watcher的信息指标（dmdbms_dw_watcher_info）| 返回DW_STATUS的状态，可用来判断守护进程的状态open = 1,mount = 2,suspend = 3 ,other = 4
4. 新增实例日志的错误信息指标（dmdbms_instance_log_error_info）|默认始终返回近5分钟内的错误实例日志的所有信息
5. 因存在docker以及部署的exporter可能不在数据库的机器上等场景，所以关闭掉查询OS的DM进程的相关指标
6. 修改打包的方式,打包的文件中不在添加版本号的信息统一名称，避免因为升级exporter而导致的大批量修改的工作

## v1.0.9
1. 将依赖的数据库由v1.3.162版本调整为v1.4.48版本，解决ipv6连接异常的问题
2. 修复开启慢SQL功能时，某些特殊场景下报错的问题，同时完善慢SQL的开启文档(https://mp.weixin.qq.com/s/FMzbrVjwC-6UdAIopg65wA)
3. 更新v1.0.9的docker镜像介质及地址
## v1.0.8
1. 修复在window环境下运行时报unknown time zone “Asia/Shanghai”的问题
2. 调整程序启动时的参数驼峰法命名，--help可查看
## 20241212
1. 新增全局报警的表盘以及对应的rules
## v1.0.7
1. 修复custom_metrics.toml不支持多个自定义指标的问题
## 20241119
1. docker介质新增amd64以及arm64的版本
2. 修正文档中的tps qps指标，实际使用的是dmdbms_statement_type_info指标
## 20241117
1. 新增docker镜像(阿里云+docker Hub)
   https://hub.docker.com/r/gaoyuan98/dameng_exporter
## v1.0.6
1. 修复指标dmdbms_start_time_info时间戳与实际时间相差14个小时问题
## v1.0.5
1. 修复表空间指标dmdbms_tablespace_size   total与free指标 赋值错误的问题
2. 优化查询指定版本时 因没有指定视图而提示的告警信息
## v1.0.4
1. 修复自定义SQL指标时，多条数据报lable重复的问题
2. 将依赖的go驱动调整为v1.3.162版本
3. 修复告警的rules中表空间告警规则不生效的问题
## v1.0.3
1. 修复自定义SQL指标时指标名称不包含context的问题
2. 优化logger的日志展示,日志级别带颜色输出
## v1.0.2
1. 新增自定义SQL指标的功能（在exporter的同级目录下创建一个custom_metrics.toml文件即可，写法与（oracledb_exporter相同）
