package db

import (
	"context"
	"dameng_exporter/config"
	"dameng_exporter/logger"
	"database/sql"
	"fmt"
	"net/url"
	"strings"
	"sync"
	"time"

	_ "github.com/gaoyuan98/dm"
	"go.uber.org/zap"
)

// DataSourcePool 数据源连接池
type DataSourcePool struct {
	Name   string                   // 数据源名称
	DB     *sql.DB                  // 数据库连接
	Config *config.DataSourceConfig // 数据源配置
	Labels map[string]string        // 标签
	mu     sync.RWMutex             // 读写锁
}

// DBPoolManager 连接池管理器
type DBPoolManager struct {
	pools    map[string]*DataSourcePool // 按名称索引的连接池
	config   *config.MultiSourceConfig  // 多数据源配置
	mu       sync.RWMutex               // 读写锁
	logger   *zap.SugaredLogger         // 日志记录器
	stopChan chan struct{}              // 停止信号
	wg       sync.WaitGroup             // 等待组
}

// 全局DBPoolManager实例
var GlobalPoolManager *DBPoolManager

// NewDBPoolManager 创建连接池管理器
func NewDBPoolManager(config *config.MultiSourceConfig) *DBPoolManager {
	return &DBPoolManager{
		pools:    make(map[string]*DataSourcePool),
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

	// 运行时重复检测
	nameMap := make(map[string]bool)
	hostMap := make(map[string]string) // host -> name mapping

	// 创建新连接池
	for _, dsConfig := range m.config.DataSources {
		if !dsConfig.Enabled {
			m.logger.Info("DataSource is disabled, skipping",
				zap.String("datasource", dsConfig.Name))
			continue
		}

		// 检查名称重复（运行时二次检查）
		if nameMap[dsConfig.Name] {
			m.logger.Error("Duplicate datasource name detected, skipping",
				zap.String("datasource", dsConfig.Name))
			return fmt.Errorf("duplicate datasource name: %s", dsConfig.Name)
		}
		nameMap[dsConfig.Name] = true

		// 检查地址重复（运行时二次检查）
		hostAddr := dsConfig.DbHost
		if idx := strings.Index(hostAddr, "?"); idx != -1 {
			hostAddr = hostAddr[:idx]
		}

		if existingName, exists := hostMap[hostAddr]; exists {
			m.logger.Error("Duplicate datasource host detected",
				zap.String("host", hostAddr),
				zap.String("existing_datasource", existingName),
				zap.String("duplicate_datasource", dsConfig.Name))
			return fmt.Errorf("duplicate datasource host: %s (used by both '%s' and '%s')",
				hostAddr, existingName, dsConfig.Name)
		}
		hostMap[hostAddr] = dsConfig.Name

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

		m.logger.Info("Successfully created pool for datasource",
			zap.String("datasource", dsConfig.Name),
			zap.String("host", dsConfig.DbHost))
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
		Name:   dsConfig.Name,
		DB:     db,
		Config: dsConfig,
		Labels: dsConfig.ParseLabels(),
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
		if pool.Config.Enabled {
			pools = append(pools, pool)
		}
	}

	return pools
}

// GetHealthyPools 获取所有启用的连接池
func (m *DBPoolManager) GetHealthyPools() []*DataSourcePool {
	m.mu.RLock()
	defer m.mu.RUnlock()

	pools := make([]*DataSourcePool, 0)
	for _, pool := range m.pools {
		if pool.Config.Enabled {
			pools = append(pools, pool)
		}
	}

	return pools
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
}
