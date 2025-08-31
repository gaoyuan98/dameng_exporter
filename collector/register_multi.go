package collector

import (
	"dameng_exporter/config"
	"dameng_exporter/db"
	"dameng_exporter/logger"
	"database/sql"
	"strings"

	"github.com/duke-git/lancet/v2/fileutil"
	"github.com/prometheus/client_golang/prometheus"
	"go.uber.org/zap"
)

// RegisterMultiSourceCollectors 注册多数据源收集器
func RegisterMultiSourceCollectors(reg *prometheus.Registry, poolManager *db.DBPoolManager) {
	registerMux.Lock()
	defer registerMux.Unlock()

	logger.Logger.Debugf("Registering multi-source collectors, OS: %v", GetOS())

	// 清空现有收集器
	collectors = []prometheus.Collector{}

	// 系统级收集器（不依赖数据库）
	collectors = append(collectors, NewSystemInfoCollector())
	collectors = append(collectors, NewBuildInfoCollector())

	// 如果poolManager为nil，报错
	if poolManager == nil {
		logger.Logger.Error("PoolManager is nil, cannot register collectors")
		return
	}

	// 主机指标（如果启用）
	if config.GlobalConfig.RegisterHostMetrics && strings.Compare(GetOS(), OS_LINUX) == 0 {
		collectors = append(collectors, AdaptCollector(poolManager, func(db *sql.DB) MetricCollector {
			return NewDmapProcessCollector(db)
		}))
	}

	// 数据库指标（如果启用）
	if config.GlobalConfig.RegisterDatabaseMetrics {
		// 使用适配器包装所有采集器
		collectors = append(collectors, AdaptCollector(poolManager, NewTableSpaceDateFileInfoCollector))
		collectors = append(collectors, AdaptCollector(poolManager, NewTableSpaceInfoCollector))
		// 继续使用适配器包装其他采集器
		collectors = append(collectors, AdaptCollector(poolManager, NewDBInstanceRunningInfoCollector))
		collectors = append(collectors, AdaptCollector(poolManager, NewDbMemoryPoolInfoCollector))
		collectors = append(collectors, AdaptCollector(poolManager, NewDBSessionsStatusCollector))
		collectors = append(collectors, AdaptCollector(poolManager, NewDbJobRunningInfoCollector))
		collectors = append(collectors, AdaptCollector(poolManager, NewSlowSessionInfoCollector))
		collectors = append(collectors, AdaptCollector(poolManager, NewMonitorInfoCollector))
		collectors = append(collectors, AdaptCollector(poolManager, NewDbSqlExecTypeCollector))
		collectors = append(collectors, AdaptCollector(poolManager, NewIniParameterCollector))
		collectors = append(collectors, AdaptCollector(poolManager, NewDbUserCollector))
		collectors = append(collectors, AdaptCollector(poolManager, NewDbLicenseCollector))
		collectors = append(collectors, AdaptCollector(poolManager, NewDbVersionCollector))
		collectors = append(collectors, AdaptCollector(poolManager, NewDbArchStatusCollector))
		collectors = append(collectors, AdaptCollector(poolManager, NewDbRapplySysCollector))
		collectors = append(collectors, AdaptCollector(poolManager, NewDbRapplyTimeDiffCollector))
		collectors = append(collectors, AdaptCollector(poolManager, func(db *sql.DB) MetricCollector {
			return NewPurgeCollector(db)
		}))
		collectors = append(collectors, AdaptCollector(poolManager, NewCkptCollector))
		collectors = append(collectors, AdaptCollector(poolManager, NewDbBufferPoolCollector))
		collectors = append(collectors, AdaptCollector(poolManager, NewDbDualCollector))
		collectors = append(collectors, AdaptCollector(poolManager, NewDbDwWatcherInfoCollector))
	}

	// DMHS指标
	if config.GlobalConfig.RegisterDmhsMetrics {
		// TODO: 添加DMHS相关采集器
	}

	// 自定义指标
	if config.GlobalConfig.RegisterCustomMetrics && fileutil.IsExist(config.GlobalConfig.CustomMetricsFile) {
		customConfig, customErr := config.ParseCustomConfig(config.GlobalConfig.CustomMetricsFile)
		if customErr != nil {
			logger.Logger.Error("解析自定义metrics指标配置文件失败", zap.Error(customErr))
		} else {
			if len(customConfig.Metrics) > 0 {
				// 为每个数据源创建自定义指标采集器
				collectors = append(collectors, AdaptCollector(poolManager, func(db *sql.DB) MetricCollector {
					return NewCustomMetrics(db, customConfig)
				}))
			}
		}
	}

	// 注册所有收集器
	for _, collector := range collectors {
		if collector != nil {
			reg.MustRegister(collector)
		}
	}

	logger.Logger.Infof("Registered %d collectors in multi-source mode", len(collectors))
}

// RegisterCollectorsWithPoolManager 使用连接池管理器注册收集器
func RegisterCollectorsWithPoolManager(reg *prometheus.Registry, poolManager *db.DBPoolManager) {
	// 统一使用多数据源架构
	RegisterMultiSourceCollectors(reg, poolManager)
}
