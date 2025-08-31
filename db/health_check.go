package db

import (
	"context"
	"dameng_exporter/logger"
	"database/sql"
	"fmt"
	"sync"
	"time"

	"go.uber.org/zap"
)

// HealthChecker 健康检查器
type HealthChecker struct {
	poolManager   *DBPoolManager
	checkInterval time.Duration
	stopChan      chan struct{}
	wg            sync.WaitGroup
	logger        *zap.SugaredLogger
}

// NewHealthChecker 创建健康检查器
func NewHealthChecker(poolManager *DBPoolManager, checkInterval time.Duration) *HealthChecker {
	if checkInterval <= 0 {
		checkInterval = 30 * time.Second // 默认30秒检查一次
	}

	return &HealthChecker{
		poolManager:   poolManager,
		checkInterval: checkInterval,
		stopChan:      make(chan struct{}),
		logger:        logger.Logger,
	}
}

// Start 启动健康检查
func (hc *HealthChecker) Start() {
	hc.wg.Add(1)
	go hc.run()
	hc.logger.Info("Health checker started",
		zap.Duration("interval", hc.checkInterval))
}

// Stop 停止健康检查
func (hc *HealthChecker) Stop() {
	close(hc.stopChan)
	hc.wg.Wait()
	hc.logger.Info("Health checker stopped")
}

// run 运行健康检查循环
func (hc *HealthChecker) run() {
	defer hc.wg.Done()

	// 立即执行一次检查
	hc.checkAllDataSources()

	ticker := time.NewTicker(hc.checkInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			hc.checkAllDataSources()
		case <-hc.stopChan:
			return
		}
	}
}

// checkAllDataSources 检查所有数据源
func (hc *HealthChecker) checkAllDataSources() {
	pools := hc.poolManager.GetPools()

	var wg sync.WaitGroup
	for _, pool := range pools {
		// 跳过已禁用的数据源
		if !pool.Config.Enabled {
			continue
		}

		wg.Add(1)
		go func(p *DataSourcePool) {
			defer wg.Done()
			hc.checkDataSource(p)
		}(pool)
	}

	wg.Wait()
}

// checkDataSource 检查单个数据源
func (hc *HealthChecker) checkDataSource(pool *DataSourcePool) {
	// 创建超时上下文
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// 执行ping检查
	err := pool.DB.PingContext(ctx)

	pool.mu.Lock()
	pool.LastCheck = time.Now()
	pool.LastError = err

	oldStatus := pool.Health
	if err != nil {
		pool.Health = HealthStatusUnhealthy
		if oldStatus != HealthStatusUnhealthy {
			// 状态变化时记录日志
			hc.logger.Error("DataSource health check failed",
				zap.String("datasource", pool.Name),
				zap.String("host", pool.Config.DbHost),
				zap.Error(err))
		}
	} else {
		pool.Health = HealthStatusHealthy
		if oldStatus != HealthStatusHealthy {
			// 从不健康恢复到健康时记录日志
			hc.logger.Info("DataSource recovered to healthy",
				zap.String("datasource", pool.Name),
				zap.String("host", pool.Config.DbHost))
		}
	}
	pool.mu.Unlock()

	// 如果连接不健康，尝试重连
	if err != nil {
		hc.tryReconnect(pool)
	}
}

// tryReconnect 尝试重新连接
func (hc *HealthChecker) tryReconnect(pool *DataSourcePool) {
	hc.logger.Info("Attempting to reconnect datasource",
		zap.String("datasource", pool.Name))

	// 关闭旧连接
	if pool.DB != nil {
		pool.DB.Close()
	}

	// 重新创建连接
	dsn := hc.poolManager.buildDSN(pool.Config)
	db, err := sql.Open("dm", dsn)
	if err != nil {
		hc.logger.Error("Failed to reopen database connection",
			zap.String("datasource", pool.Name),
			zap.Error(err))
		return
	}

	// 设置连接池参数
	db.SetMaxOpenConns(pool.Config.MaxOpenConns)
	db.SetMaxIdleConns(pool.Config.MaxIdleConns)
	db.SetConnMaxLifetime(time.Duration(pool.Config.ConnMaxLifetime) * time.Minute)

	// 测试连接
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := db.PingContext(ctx); err != nil {
		db.Close()
		hc.logger.Error("Failed to ping database after reconnect",
			zap.String("datasource", pool.Name),
			zap.Error(err))
		return
	}

	// 更新连接池
	pool.mu.Lock()
	pool.DB = db
	pool.Health = HealthStatusHealthy
	pool.LastCheck = time.Now()
	pool.LastError = nil
	pool.mu.Unlock()

	hc.logger.Info("Successfully reconnected datasource",
		zap.String("datasource", pool.Name))
}

// CheckDataSourceHealth 手动检查指定数据源健康状态
func CheckDataSourceHealth(poolManager *DBPoolManager, dataSourceName string) (HealthStatus, error) {
	pool := poolManager.GetPool(dataSourceName)
	if pool == nil {
		return HealthStatusUnknown, fmt.Errorf("datasource not found: %s", dataSourceName)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	err := pool.DB.PingContext(ctx)

	pool.mu.Lock()
	pool.LastCheck = time.Now()
	pool.LastError = err
	if err != nil {
		pool.Health = HealthStatusUnhealthy
	} else {
		pool.Health = HealthStatusHealthy
	}
	status := pool.Health
	pool.mu.Unlock()

	return status, err
}

// GetHealthReport 获取健康报告
func GetHealthReport(poolManager *DBPoolManager) map[string]interface{} {
	report := make(map[string]interface{})

	pools := poolManager.GetPools()
	totalPools := len(pools)
	healthyPools := 0
	unhealthyPools := 0

	poolReports := make([]map[string]interface{}, 0)

	for _, pool := range pools {
		pool.mu.RLock()
		poolReport := map[string]interface{}{
			"name":       pool.Name,
			"host":       pool.Config.DbHost,
			"priority":   pool.Priority,
			"health":     healthStatusToString(pool.Health),
			"last_check": pool.LastCheck.Format(time.RFC3339),
			"enabled":    pool.Config.Enabled,
		}

		if pool.LastError != nil {
			poolReport["last_error"] = pool.LastError.Error()
		}

		if pool.Health == HealthStatusHealthy {
			healthyPools++
		} else if pool.Health == HealthStatusUnhealthy {
			unhealthyPools++
		}

		pool.mu.RUnlock()
		poolReports = append(poolReports, poolReport)
	}

	report["total_pools"] = totalPools
	report["healthy_pools"] = healthyPools
	report["unhealthy_pools"] = unhealthyPools
	report["pools"] = poolReports
	report["timestamp"] = time.Now().Format(time.RFC3339)

	return report
}

// healthStatusToString 将健康状态转换为字符串
func healthStatusToString(status HealthStatus) string {
	switch status {
	case HealthStatusHealthy:
		return "healthy"
	case HealthStatusUnhealthy:
		return "unhealthy"
	case HealthStatusDisabled:
		return "disabled"
	default:
		return "unknown"
	}
}

// StartHealthChecker 启动全局健康检查器（便捷函数）
func StartHealthChecker(poolManager *DBPoolManager) *HealthChecker {
	checker := NewHealthChecker(poolManager, 30*time.Second)
	checker.Start()
	return checker
}
