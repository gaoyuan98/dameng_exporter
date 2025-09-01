package collector

import (
	"dameng_exporter/config"
	"dameng_exporter/db"
	"dameng_exporter/logger"
	"database/sql"
	"strings"

	"github.com/prometheus/client_golang/prometheus"
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

	// 注意：理想情况下，RegisterHostMetrics、RegisterDatabaseMetrics等应该是全局配置
	// 而不是每个数据源单独配置。但为了保持向后兼容，我们检查是否有任何数据源需要这些指标

	// 检查是否有任何数据源需要各类指标
	// 这里使用 OR 逻辑：只要有任何一个数据源需要，就注册相应的采集器
	needHostMetrics := false
	needDatabaseMetrics := false
	needDmhsMetrics := false
	needCustomMetrics := false

	for _, ds := range config.GlobalMultiConfig.DataSources {
		if ds.Enabled {
			if ds.RegisterHostMetrics {
				needHostMetrics = true
			}
			if ds.RegisterDatabaseMetrics {
				needDatabaseMetrics = true
			}
			if ds.RegisterDmhsMetrics {
				needDmhsMetrics = true
			}
			if ds.RegisterCustomMetrics {
				needCustomMetrics = true
				// 不再需要收集 customMetricsFile，每个数据源独立处理
			}
		}
	}

	// 主机指标（如果任何数据源需要，且在Linux系统上）
	if needHostMetrics && strings.Compare(GetOS(), OS_LINUX) == 0 {
		collectors = append(collectors, AdaptCollector(poolManager, func(db *sql.DB) MetricCollector {
			return NewDmapProcessCollector(db)
		}))
	}

	// 数据库指标（如果任何数据源需要）
	if needDatabaseMetrics {
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

	// DMHS指标（如果任何数据源需要）
	if needDmhsMetrics {
		// TODO: 添加DMHS相关采集器
		logger.Logger.Debug("DMHS metrics requested but not yet implemented")
	}

	// 注册所有收集器
	for _, collector := range collectors {
		if collector != nil {
			reg.MustRegister(collector)
		}
	}

	// 自定义指标处理 - 使用新的多数据源独立配置方式
	// 每个数据源可以有自己独立的自定义指标配置文件
	if needCustomMetrics {
		// 使用专门的自定义指标适配器，支持每个数据源独立配置
		RegisterCustomMetricsForMultiSource(reg, poolManager)
	}

	logger.Logger.Infof("Registered %d collectors in multi-source mode", len(collectors))
}

// RegisterCollectorsWithPoolManager 使用连接池管理器注册收集器
func RegisterCollectorsWithPoolManager(reg *prometheus.Registry, poolManager *db.DBPoolManager) {
	// 统一使用多数据源架构
	RegisterMultiSourceCollectors(reg, poolManager)
}
