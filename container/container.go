package container

import (
	"dameng_exporter/config"
	"dameng_exporter/logger"
	"database/sql"
	"sync"

	"github.com/prometheus/client_golang/prometheus"
)

// Container 依赖注入容器
type Container struct {
	mu sync.RWMutex

	// 配置
	config *config.ConfigV2

	// 数据库连接池
	dbPool *sql.DB

	// Prometheus注册器
	registry *prometheus.Registry

	// 其他服务
	services map[string]interface{}
}

// NewContainer 创建新的依赖注入容器
func NewContainer(cfg *config.ConfigV2) *Container {
	return &Container{
		config:   cfg,
		registry: prometheus.NewRegistry(),
		services: make(map[string]interface{}),
	}
}

// GetConfig 获取配置
func (c *Container) GetConfig() *config.ConfigV2 {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.config
}

// SetDBPool 设置数据库连接池
func (c *Container) SetDBPool(db *sql.DB) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.dbPool = db
}

// GetDBPool 获取数据库连接池
func (c *Container) GetDBPool() *sql.DB {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.dbPool
}

// GetRegistry 获取Prometheus注册器
func (c *Container) GetRegistry() *prometheus.Registry {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.registry
}

// Register 注册服务
func (c *Container) Register(name string, service interface{}) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.services[name] = service
}

// Get 获取服务
func (c *Container) Get(name string) (interface{}, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	service, ok := c.services[name]
	return service, ok
}

// MustGet 获取服务（如果不存在则panic）
func (c *Container) MustGet(name string) interface{} {
	service, ok := c.Get(name)
	if !ok {
		panic("service not found: " + name)
	}
	return service
}

// Close 关闭容器，释放资源
func (c *Container) Close() {
	c.mu.Lock()
	defer c.mu.Unlock()

	// 关闭数据库连接
	if c.dbPool != nil {
		c.dbPool.Close()
		c.dbPool = nil
	}

	// 清理服务
	c.services = make(map[string]interface{})
}

// GlobalContainer 全局容器实例（用于过渡期的兼容性）
var globalContainer *Container
var globalMutex sync.RWMutex

// InitGlobalContainer 初始化全局容器
func InitGlobalContainer(cfg *config.ConfigV2) {
	globalMutex.Lock()
	defer globalMutex.Unlock()

	if globalContainer != nil {
		globalContainer.Close()
	}
	globalContainer = NewContainer(cfg)
}

// GetGlobalContainer 获取全局容器
func GetGlobalContainer() *Container {
	globalMutex.RLock()
	defer globalMutex.RUnlock()

	if globalContainer == nil {
		panic("global container not initialized")
	}
	return globalContainer
}

// CloseGlobalContainer 关闭全局容器
func CloseGlobalContainer() {
	globalMutex.Lock()
	defer globalMutex.Unlock()

	if globalContainer != nil {
		globalContainer.Close()
		globalContainer = nil
	}
}

// ServiceRegistry 服务注册器接口
type ServiceRegistry interface {
	RegisterServices(container *Container) error
}

// CollectorFactory 采集器工厂
type CollectorFactory struct {
	container *Container
}

// NewCollectorFactory 创建采集器工厂
func NewCollectorFactory(container *Container) *CollectorFactory {
	return &CollectorFactory{
		container: container,
	}
}

// CreateCollector 创建采集器
func (cf *CollectorFactory) CreateCollector(name string) (prometheus.Collector, error) {
	// 这里可以根据名称创建不同的采集器
	// 所有采集器都从容器中获取所需的依赖
	db := cf.container.GetDBPool()
	config := cf.container.GetConfig()

	// 根据采集器名称创建相应的实例
	// 这里需要导入collector包并创建具体的采集器
	// 示例代码，实际需要根据具体采集器类型创建
	switch name {
	case "system_info":
		// return collector.NewSystemInfoCollector(), nil
	case "tablespace":
		// return collector.NewTableSpaceInfoCollector(db), nil
	// ... 其他采集器
	default:
		return nil, fmt.Errorf("unknown collector: %s", name)
	}

	return nil, nil
}
