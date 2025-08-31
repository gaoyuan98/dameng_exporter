package db

import (
	"context"
	"dameng_exporter/config"
	"dameng_exporter/logger"
	"database/sql"
	"fmt"
	"net/url"
	"sort"
	"strings"
	"sync"
	"time"

	_ "github.com/gaoyuan98/dm"
	"go.uber.org/zap"
)

// HealthStatus 健康状态
type HealthStatus int

const (
	// HealthStatusUnknown 未知状态
	HealthStatusUnknown HealthStatus = iota
	// HealthStatusHealthy 健康
	HealthStatusHealthy
	// HealthStatusUnhealthy 不健康
	HealthStatusUnhealthy
	// HealthStatusDisabled 已禁用
	HealthStatusDisabled
)

// DataSourcePool 数据源连接池
type DataSourcePool struct {
	Name      string                   // 数据源名称
	DB        *sql.DB                  // 数据库连接
	Config    *config.DataSourceConfig // 数据源配置
	Labels    map[string]string        // 标签
	Priority  int                      // 优先级
	Health    HealthStatus             // 健康状态
	LastCheck time.Time                // 最后检查时间
	LastError error                    // 最后错误
	mu        sync.RWMutex             // 读写锁
}

// DBPoolManager 连接池管理器
type DBPoolManager struct {
	pools    map[string]*DataSourcePool // 按名称索引的连接池
	groups   map[int][]*DataSourcePool  // 按优先级分组的连接池
	strategy config.CollectStrategy     // 采集策略
	config   *config.MultiSourceConfig  // 多数据源配置
	mu       sync.RWMutex               // 读写锁
	logger   *zap.SugaredLogger         // 日志记录器
	stopChan chan struct{}              // 停止信号
	wg       sync.WaitGroup             // 等待组
}

// 全局DBPoolManager实例（用于兼容旧代码）
var GlobalPoolManager *DBPoolManager

// NewDBPoolManager 创建连接池管理器
func NewDBPoolManager(config *config.MultiSourceConfig) *DBPoolManager {
	return &DBPoolManager{
		pools:    make(map[string]*DataSourcePool),
		groups:   make(map[int][]*DataSourcePool),
		strategy: config.CollectStrategy,
		config:   config,
		logger:   logger.Logger,
		stopChan: make(chan struct{}),
	}
}

// InitPools 初始化所有连接池
func (m *DBPoolManager) InitPools() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	// 清空现有连接池
	for _, pool := range m.pools {
		if pool.DB != nil {
			pool.DB.Close()
		}
	}
	m.pools = make(map[string]*DataSourcePool)
	m.groups = make(map[int][]*DataSourcePool)

	// 创建新连接池
	for _, dsConfig := range m.config.DataSources {
		if !dsConfig.Enabled {
			m.logger.Info("DataSource is disabled, skipping",
				zap.String("datasource", dsConfig.Name))
			continue
		}

		pool, err := m.createPool(&dsConfig)
		if err != nil {
			m.logger.Error("Failed to create pool for datasource",
				zap.String("datasource", dsConfig.Name),
				zap.Error(err))
			// 继续创建其他连接池，不因单个失败而中断
			continue
		}

		// 添加到索引
		m.pools[dsConfig.Name] = pool

		// 添加到优先级分组
		if m.groups[dsConfig.Priority] == nil {
			m.groups[dsConfig.Priority] = make([]*DataSourcePool, 0)
		}
		m.groups[dsConfig.Priority] = append(m.groups[dsConfig.Priority], pool)

		m.logger.Info("Successfully created pool for datasource",
			zap.String("datasource", dsConfig.Name),
			zap.String("host", dsConfig.DbHost),
			zap.Int("priority", dsConfig.Priority))
	}

	if len(m.pools) == 0 {
		return fmt.Errorf("no valid datasource pools created")
	}

	// 设置全局实例（用于兼容）
	GlobalPoolManager = m

	return nil
}

// createPool 创建单个连接池
func (m *DBPoolManager) createPool(dsConfig *config.DataSourceConfig) (*DataSourcePool, error) {
	// 构建DSN
	dsn := m.buildDSN(dsConfig)

	// 创建数据库连接
	db, err := sql.Open("dm", dsn)
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}

	// 设置连接池参数
	db.SetMaxOpenConns(dsConfig.MaxOpenConns)
	db.SetMaxIdleConns(dsConfig.MaxIdleConns)
	db.SetConnMaxLifetime(time.Duration(dsConfig.ConnMaxLifetime) * time.Minute)

	// 测试连接
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := db.PingContext(ctx); err != nil {
		db.Close()
		return nil, fmt.Errorf("failed to ping database: %w", err)
	}

	// 创建连接池对象
	pool := &DataSourcePool{
		Name:      dsConfig.Name,
		DB:        db,
		Config:    dsConfig,
		Labels:    dsConfig.ParseLabels(),
		Priority:  dsConfig.Priority,
		Health:    HealthStatusHealthy,
		LastCheck: time.Now(),
	}

	// 添加datasource标签
	pool.Labels["datasource"] = dsConfig.Name

	return pool, nil
}

// buildDSN 构建数据源连接字符串
func (m *DBPoolManager) buildDSN(dsConfig *config.DataSourceConfig) string {
	// 处理DbHost中可能包含的查询参数
	hostWithParams := dsConfig.DbHost
	queryParams := ""

	if idx := strings.Index(hostWithParams, "?"); idx != -1 {
		queryParams = hostWithParams[idx:]
		hostWithParams = hostWithParams[:idx]
	}

	// 基础DSN格式: dm://username:password@host:port
	dsn := fmt.Sprintf("dm://%s:%s@%s",
		url.QueryEscape(dsConfig.DbUser),
		url.QueryEscape(dsConfig.DbPwd),
		hostWithParams)

	// 添加查询参数
	if queryParams != "" {
		dsn += queryParams
	} else {
		dsn += "?autoCommit=true"
	}

	return dsn
}

// GetPool 获取指定名称的连接池
func (m *DBPoolManager) GetPool(name string) *DataSourcePool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.pools[name]
}

// GetPools 获取所有连接池
func (m *DBPoolManager) GetPools() []*DataSourcePool {
	m.mu.RLock()
	defer m.mu.RUnlock()

	pools := make([]*DataSourcePool, 0, len(m.pools))
	for _, pool := range m.pools {
		if pool.Health != HealthStatusDisabled {
			pools = append(pools, pool)
		}
	}

	// 按优先级排序
	sort.Slice(pools, func(i, j int) bool {
		return pools[i].Priority < pools[j].Priority
	})

	return pools
}

// GetPoolsByPriority 获取指定优先级的连接池
func (m *DBPoolManager) GetPoolsByPriority(priority int) []*DataSourcePool {
	m.mu.RLock()
	defer m.mu.RUnlock()

	pools := make([]*DataSourcePool, 0)
	for _, pool := range m.groups[priority] {
		if pool.Health != HealthStatusDisabled {
			pools = append(pools, pool)
		}
	}
	return pools
}

// GetHealthyPools 获取所有健康的连接池
func (m *DBPoolManager) GetHealthyPools() []*DataSourcePool {
	m.mu.RLock()
	defer m.mu.RUnlock()

	pools := make([]*DataSourcePool, 0)
	for _, pool := range m.pools {
		if pool.Health == HealthStatusHealthy {
			pools = append(pools, pool)
		}
	}

	// 按优先级排序
	sort.Slice(pools, func(i, j int) bool {
		return pools[i].Priority < pools[j].Priority
	})

	return pools
}

// UpdatePool 更新单个连接池
func (m *DBPoolManager) UpdatePool(dsConfig *config.DataSourceConfig) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	// 关闭旧连接池
	if oldPool, exists := m.pools[dsConfig.Name]; exists {
		if oldPool.DB != nil {
			oldPool.DB.Close()
		}
		// 从优先级分组中移除
		m.removeFromGroup(oldPool)
	}

	// 创建新连接池
	if dsConfig.Enabled {
		pool, err := m.createPool(dsConfig)
		if err != nil {
			return err
		}

		// 更新索引
		m.pools[dsConfig.Name] = pool

		// 添加到优先级分组
		if m.groups[dsConfig.Priority] == nil {
			m.groups[dsConfig.Priority] = make([]*DataSourcePool, 0)
		}
		m.groups[dsConfig.Priority] = append(m.groups[dsConfig.Priority], pool)
	} else {
		// 如果禁用，只删除不创建
		delete(m.pools, dsConfig.Name)
	}

	return nil
}

// removeFromGroup 从优先级分组中移除连接池
func (m *DBPoolManager) removeFromGroup(pool *DataSourcePool) {
	if group, exists := m.groups[pool.Priority]; exists {
		for i, p := range group {
			if p.Name == pool.Name {
				m.groups[pool.Priority] = append(group[:i], group[i+1:]...)
				break
			}
		}
	}
}

// Close 关闭所有连接池
func (m *DBPoolManager) Close() {
	m.mu.Lock()
	defer m.mu.Unlock()

	// 发送停止信号
	close(m.stopChan)

	// 等待所有goroutine结束
	m.wg.Wait()

	// 关闭所有连接池
	for name, pool := range m.pools {
		if pool.DB != nil {
			if err := pool.DB.Close(); err != nil {
				m.logger.Error("Failed to close pool",
					zap.String("datasource", name),
					zap.Error(err))
			}
		}
	}

	m.pools = make(map[string]*DataSourcePool)
	m.groups = make(map[int][]*DataSourcePool)
}

// GetStatus 获取连接池状态信息
func (m *DBPoolManager) GetStatus() map[string]interface{} {
	m.mu.RLock()
	defer m.mu.RUnlock()

	status := make(map[string]interface{})
	status["total_pools"] = len(m.pools)
	status["strategy"] = string(m.strategy)

	poolStatuses := make([]map[string]interface{}, 0)
	for name, pool := range m.pools {
		poolStatus := map[string]interface{}{
			"name":       name,
			"priority":   pool.Priority,
			"health":     m.healthStatusString(pool.Health),
			"last_check": pool.LastCheck.Format(time.RFC3339),
		}
		if pool.LastError != nil {
			poolStatus["last_error"] = pool.LastError.Error()
		}
		poolStatuses = append(poolStatuses, poolStatus)
	}
	status["pools"] = poolStatuses

	return status
}

// healthStatusString 健康状态转字符串
func (m *DBPoolManager) healthStatusString(status HealthStatus) string {
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

// SetPoolHealth 设置连接池健康状态
func (m *DBPoolManager) SetPoolHealth(name string, status HealthStatus, err error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if pool, exists := m.pools[name]; exists {
		pool.mu.Lock()
		pool.Health = status
		pool.LastCheck = time.Now()
		pool.LastError = err
		pool.mu.Unlock()
	}
}

// GetLegacyPool 获取默认连接池（用于向后兼容）
func (m *DBPoolManager) GetLegacyPool() *sql.DB {
	if pool := m.GetPool("default"); pool != nil {
		return pool.DB
	}
	// 如果没有default，返回第一个可用的
	pools := m.GetHealthyPools()
	if len(pools) > 0 {
		return pools[0].DB
	}
	return nil
}
