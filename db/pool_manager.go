package db

import (
	"context"
	"dameng_exporter/config"
	"dameng_exporter/logger"
	"database/sql"
	"fmt"
	"net"
	"net/url"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	_ "github.com/gaoyuan98/dm"
	"go.uber.org/zap"
)

// DataSourcePool 数据源连接池
type DataSourcePool struct {
	Name            string                   // 数据源名称
	DB              *sql.DB                  // 数据库连接
	Config          *config.DataSourceConfig // 数据源配置
	Labels          map[string]string        // 标签
	mu              sync.RWMutex             // 读写锁
	healthy         atomic.Bool              // 健康状态标志
	lastHealthCheck atomic.Int64             // 最近一次健康检查的时间戳（Unix 秒）
}

// markHealthy 更新健康状态为健康并记录时间
func (p *DataSourcePool) markHealthy(ts time.Time) {
	if p == nil {
		return
	}
	if ts.IsZero() {
		ts = time.Now()
	}
	p.healthy.Store(true)
	p.lastHealthCheck.Store(ts.Unix())
}

// markUnhealthy 更新健康状态为不可用并记录时间
func (p *DataSourcePool) markUnhealthy(ts time.Time) {
	if p == nil {
		return
	}
	if ts.IsZero() {
		ts = time.Now()
	}
	p.healthy.Store(false)
	p.lastHealthCheck.Store(ts.Unix())
}

// IsHealthy 返回当前健康标志
func (p *DataSourcePool) IsHealthy() bool {
	if p == nil {
		return false
	}
	return p.healthy.Load()
}

// LastHealthCheck 获取最近一次健康检查的时间
func (p *DataSourcePool) LastHealthCheck() time.Time {
	if p == nil {
		return time.Time{}
	}
	ts := p.lastHealthCheck.Load()
	if ts == 0 {
		return time.Time{}
	}
	return time.Unix(ts, 0)
}

// FailedDataSource 失败列表中的数据源信息
type FailedDataSource struct {
	Config      *config.DataSourceConfig // 对应的数据源配置，用于后续重试
	FailedAt    time.Time                // 首次检测到失败的时间
	LastAttempt time.Time                // 最近一次尝试恢复的时间
	LastError   string                   // 最近一次失败的错误信息
}

// DatasourceHealthStatus 描述数据源的健康状态
type DatasourceHealthStatus struct {
	Healthy    bool      // 是否健康
	LastCheck  time.Time // 最近一次健康检查时间
	LastError  string    // 最近一次错误信息
	Registered bool      // 是否注册过（存在于健康或失败列表）
}

// DBPoolManager 连接池管理器
type DBPoolManager struct {
	pools         map[string]*DataSourcePool   // 成功列表：当前健康的连接池
	failedSources map[string]*FailedDataSource // 失败列表：待恢复的数据源
	config        *config.MultiSourceConfig    // 多数据源配置
	mu            sync.RWMutex                 // 读写锁
	logger        *zap.SugaredLogger           // 日志记录器
	stopChan      chan struct{}                // 停止信号
	monitorOnce   sync.Once                    // 确保后台监控只启动一次
	stopOnce      sync.Once                    // 确保停止信号只发送一次
	wg            sync.WaitGroup               // 等待组
}

// 全局DBPoolManager实例
var GlobalPoolManager *DBPoolManager

// NewDBPoolManager 创建连接池管理器
func NewDBPoolManager(config *config.MultiSourceConfig) *DBPoolManager {
	return &DBPoolManager{
		pools:         make(map[string]*DataSourcePool),
		failedSources: make(map[string]*FailedDataSource),
		config:        config,
		logger:        logger.Logger,
		stopChan:      make(chan struct{}),
	}
}

// InitPools 初始化所有连接池
func (m *DBPoolManager) InitPools() error {
	var shouldStartMonitor bool

	m.mu.Lock()
	defer func() {
		m.mu.Unlock()
		if shouldStartMonitor {
			//启动定时任务扫描失败列表的后台线程
			m.startBackgroundMonitor()
		}
	}()

	// 步骤1：清空旧的连接池和失败列表，避免残留状态干扰后续初始化
	for _, pool := range m.pools {
		if pool.DB != nil {
			pool.DB.Close()
		}
	}
	m.pools = make(map[string]*DataSourcePool)
	m.failedSources = make(map[string]*FailedDataSource)

	// 步骤2：构建名称与地址去重索引，确保配置合法
	nameMap := make(map[string]bool)
	hostMap := make(map[string]string) // host -> name mapping

	// 步骤3：遍历配置，为每个启用的数据源创建连接池或登记失败
	for i := range m.config.DataSources {
		dsConfig := &m.config.DataSources[i]

		if !dsConfig.Enabled {
			m.logger.Info("数据源被禁用，跳过初始化",
				zap.String("datasource", dsConfig.Name))
			continue
		}

		// 运行时进行名称重复检查
		if nameMap[dsConfig.Name] {
			m.logger.Error("检测到重复的数据源名称",
				zap.String("datasource", dsConfig.Name))
			return fmt.Errorf("数据源名称重复: %s", dsConfig.Name)
		}
		nameMap[dsConfig.Name] = true

		// 运行时进行地址重复检查
		hostAddr := dsConfig.DbHost
		if idx := strings.Index(hostAddr, "?"); idx != -1 {
			hostAddr = hostAddr[:idx]
		}

		if existingName, exists := hostMap[hostAddr]; exists {
			m.logger.Error("检测到重复的数据源地址",
				zap.String("host", hostAddr),
				zap.String("existing_datasource", existingName),
				zap.String("duplicate_datasource", dsConfig.Name))
			return fmt.Errorf("数据源地址重复: %s (被 '%s' 和 '%s' 同时使用)",
				hostAddr, existingName, dsConfig.Name)
		}
		hostMap[hostAddr] = dsConfig.Name

		// 尝试建立真实连接
		pool, err := m.createPool(dsConfig)
		if err != nil {

			m.logger.Error("创建数据源连接池失败",
				zap.String("datasource", dsConfig.Name),
				zap.Error(err))

			// 初始化阶段失败：登记失败信息，等待后台自动恢复
			m.noteFailedDataSourceLocked(dsConfig, err, time.Now())
			continue
		}

		// 初始化成功：放入健康列表供业务使用
		m.pools[dsConfig.Name] = pool

		m.logger.Info("成功创建数据源连接池",
			zap.String("datasource", dsConfig.Name),
			zap.String("host", dsConfig.DbHost))
	}

	// 步骤4：若全部失败，提示依赖后台自动恢复
	if len(m.pools) == 0 {
		m.logger.Warn("初始化阶段没有任何数据源连接成功，将依赖后台恢复机制")
	}

	// 步骤5：更新全局实例并在解锁后启动后台监控
	GlobalPoolManager = m
	shouldStartMonitor = true

	return nil
}

// createPool 创建单个连接池
func (m *DBPoolManager) createPool(dsConfig *config.DataSourceConfig) (*DataSourcePool, error) {
	// 步骤1：根据数据源配置构建连接 DSN
	dsn := m.buildDSN(dsConfig)

	// 步骤2：打开底层数据库连接（不立即验证）
	db, err := sql.Open("dm", dsn)
	if err != nil {
		return nil, fmt.Errorf("打开数据库连接失败: %w", err)
	}

	// 步骤3：设置连接池参数，确保与配置保持一致
	db.SetMaxOpenConns(dsConfig.MaxOpenConns)
	db.SetMaxIdleConns(dsConfig.MaxIdleConns)
	db.SetConnMaxLifetime(time.Duration(dsConfig.ConnMaxLifetime) * time.Minute)

	// 步骤4：执行带超时的 Ping 验证，确保连接可达
	timeoutSeconds := dsConfig.QueryTimeout
	if timeoutSeconds <= 0 {
		timeoutSeconds = config.DefaultDataSourceConfig.QueryTimeout
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(timeoutSeconds)*time.Second)
	defer cancel()

	if err := db.PingContext(ctx); err != nil {
		db.Close()
		return nil, fmt.Errorf("测试数据库连接失败: %w", err)
	}

	// 解析 host、port，供后续标签注入使用
	cleanHost, hostLabel, portLabel := normalizeDBHost(dsConfig.DbHost)

	// 步骤5：封装成 DataSourcePool 统一管理
	pool := &DataSourcePool{
		Name:   dsConfig.Name,
		DB:     db,
		Config: dsConfig,
		Labels: dsConfig.ParseLabels(),
	}
	pool.markHealthy(time.Now())

	// 步骤6：追加标准化标签，便于指标及日志 tracing
	datasourceLabel := dsConfig.Name
	hostForDatasource := formatHostPortLabel(hostLabel, portLabel)
	if hostForDatasource == "" {
		hostForDatasource = cleanHost
	}
	if hostForDatasource != "" {
		datasourceLabel = fmt.Sprintf("%s@%s", dsConfig.Name, hostForDatasource)
	}
	pool.Labels["datasource"] = datasourceLabel

	return pool, nil
}

// buildDSN 构建数据源连接字符串
func (m *DBPoolManager) buildDSN(dsConfig *config.DataSourceConfig) string {
	// 步骤1：拆分主机地址与附加查询参数
	hostWithParams := dsConfig.DbHost
	queryParams := ""

	if idx := strings.Index(hostWithParams, "?"); idx != -1 {
		queryParams = hostWithParams[idx:]
		hostWithParams = hostWithParams[:idx]
	}

	// 步骤2：按照达梦驱动要求拼接基础 DSN
	dsn := fmt.Sprintf("dm://%s:%s@%s",
		url.QueryEscape(dsConfig.DbUser),
		url.QueryEscape(dsConfig.DbPwd),
		hostWithParams)

	// 步骤3：带上原始查询参数，缺省情况下追加 autoCommit=true
	if queryParams != "" {
		dsn += queryParams
	} else {
		timeoutSeconds := dsConfig.QueryTimeout
		if timeoutSeconds <= 0 {
			timeoutSeconds = config.DefaultDataSourceConfig.QueryTimeout
		}
		socketTimeout := timeoutSeconds
		connectTimeout := timeoutSeconds * 1000
		dsn += fmt.Sprintf("?autoCommit=true&socketTimeout=%d&connectTimeout=%d",
			socketTimeout, connectTimeout)
	}

	return dsn
}

// normalizeDBHost 解析并归一化 dbHost，支持 IPv4/IPv6 以及带查询串的写法
func normalizeDBHost(rawHost string) (cleanHost string, host string, port string) {
	cleanHost = strings.TrimSpace(rawHost)
	if cleanHost == "" {
		return "", "", ""
	}

	if idx := strings.Index(cleanHost, "?"); idx != -1 {
		cleanHost = strings.TrimSpace(cleanHost[:idx])
	}

	host = cleanHost
	if strings.HasPrefix(host, "[") && strings.HasSuffix(host, "]") {
		host = strings.TrimSuffix(strings.TrimPrefix(host, "["), "]")
	}

	if parsedHost, parsedPort, err := net.SplitHostPort(cleanHost); err == nil {
		return cleanHost, parsedHost, parsedPort
	}

	// 兜底处理未加方括号的 IPv6:port 写法，尝试以最后一个冒号切分
	if strings.Count(cleanHost, ":") >= 2 {
		if idx := strings.LastIndex(cleanHost, ":"); idx != -1 && idx < len(cleanHost)-1 {
			pHost := cleanHost[:idx]
			pPort := cleanHost[idx+1:]
			if isNumericPort(pPort) {
				host = strings.TrimSuffix(strings.TrimPrefix(pHost, "["), "]")
				port = pPort
				return cleanHost, host, port
			}
		}
	}

	return cleanHost, host, port
}

func isNumericPort(port string) bool {
	if port == "" {
		return false
	}
	for _, r := range port {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

func formatHostPortLabel(host, port string) string {
	if host == "" {
		return ""
	}
	if port == "" {
		return host
	}
	if strings.Contains(host, ":") {
		return fmt.Sprintf("[%s]:%s", host, port)
	}
	return fmt.Sprintf("%s:%s", host, port)
}

// noteFailedDataSourceLocked 在持有锁的情况下登记失败数据源
func (m *DBPoolManager) noteFailedDataSourceLocked(dsConfig *config.DataSourceConfig, err error, ts time.Time) {
	if dsConfig == nil || dsConfig.Name == "" {
		return
	}

	entry, exists := m.failedSources[dsConfig.Name]
	message := ""
	if err != nil {
		message = err.Error()
	}

	if !exists {
		// 首次失败：记录失败时间与最近尝试时间，持久化错误信息
		m.failedSources[dsConfig.Name] = &FailedDataSource{
			Config:      dsConfig,
			FailedAt:    ts,
			LastAttempt: ts,
			LastError:   message,
		}
		return
	}

	// 已存在：仅更新最近一次尝试时间与错误信息
	entry.LastAttempt = ts
	entry.LastError = message
}

// noteFailedDataSource 无锁登记失败数据源，供后台任务或其他调用方使用
func (m *DBPoolManager) noteFailedDataSource(dsConfig *config.DataSourceConfig, err error) {
	if dsConfig == nil {
		return
	}

	// 采用当前时间戳作为默认失败记录时间
	now := time.Now()
	m.mu.Lock()
	defer m.mu.Unlock()
	m.noteFailedDataSourceLocked(dsConfig, err, now)
}

// getFailedConfigsSnapshot 获取失败列表的快照，避免长时间持锁
func (m *DBPoolManager) getFailedConfigsSnapshot() []*config.DataSourceConfig {
	m.mu.RLock()
	defer m.mu.RUnlock()

	snapshot := make([]*config.DataSourceConfig, 0, len(m.failedSources))
	for _, failed := range m.failedSources {
		if failed != nil {
			// 仅抽取配置引用，避免在重试阶段长时间持锁
			snapshot = append(snapshot, failed.Config)
		}
	}
	return snapshot
}

// getHealthyPoolsSnapshot 获取健康连接池的快照
func (m *DBPoolManager) getHealthyPoolsSnapshot() []*DataSourcePool {
	m.mu.RLock()
	defer m.mu.RUnlock()

	snapshot := make([]*DataSourcePool, 0, len(m.pools))
	for _, pool := range m.pools {
		if pool != nil {
			// 返回连接池指针供只读检测使用，避免阻塞写锁
			snapshot = append(snapshot, pool)
		}
	}
	return snapshot
}

// startBackgroundMonitor 启动后台状态监控协程（只会启动一次），调度频率由 RetryIntervalSeconds 控制
func (m *DBPoolManager) startBackgroundMonitor() {
	if m == nil {
		return
	}

	m.monitorOnce.Do(func() {
		// 读取配置的 RetryIntervalSeconds 作为统一的调度周期
		retrySeconds := 60
		healthPingEnabled := true
		if m.config != nil {
			retrySeconds = m.config.GetRetryIntervalSeconds()
			healthPingEnabled = m.config.IsHealthPingEnabled()
		}
		if !healthPingEnabled {
			m.logger.Info("已禁用周期性健康检查，仅在查询报错时触发降级与自动重试")
		}

		interval := time.Duration(retrySeconds) * time.Second
		if interval <= 0 {
			interval = 60 * time.Second
		}

		// 启动单独协程执行周期性健康检测与失败重试
		m.wg.Add(1)
		go func() {
			defer m.wg.Done()

			ticker := time.NewTicker(interval)
			defer ticker.Stop()

			// 初始化阶段立刻执行一次健康检查和失败重试，保证状态立即收敛
			if healthPingEnabled {
				m.checkHealthyPools()
			}
			m.retryFailedDataSources()

			for {
				select {
				case <-m.stopChan:
					// 收到停止信号后直接退出循环
					return
				case <-ticker.C:
					// 每个周期依次处理健康检测与失败重连
					if healthPingEnabled {
						m.checkHealthyPools()
					}
					m.retryFailedDataSources()
				}
			}
		}()
	})
}

// retryFailedDataSources 重试失败列表中的数据源连接
func (m *DBPoolManager) retryFailedDataSources() {
	// 通过快照提高并发效率，后续重试不影响读写锁
	configs := m.getFailedConfigsSnapshot()
	if len(configs) == 0 {
		return
	}

	for _, cfg := range configs {
		if cfg == nil {
			continue
		}

		// 针对失败数据源尝试重新创建连接
		pool, err := m.createPool(cfg)
		if err != nil {
			m.logger.Warn("重试建立数据源连接失败",
				zap.String("datasource", cfg.Name),
				zap.Error(err))
			m.noteFailedDataSource(cfg, err)
			continue
		}

		// 恢复成功后转入健康列表
		if m.promoteToHealthy(cfg, pool) {
			m.logger.Info("数据源连接恢复成功，已移入健康列表",
				zap.String("datasource", cfg.Name))
		} else {
			// 转移失败时关闭刚建立的多余连接
			// 如果未能加入健康列表，需要释放刚刚创建的连接
			if pool.DB != nil {
				if closeErr := pool.DB.Close(); closeErr != nil {
					m.logger.Error("重试后关闭多余连接失败",
						zap.String("datasource", cfg.Name),
						zap.Error(closeErr))
				}
			}
		}
	}
}

// promoteToHealthy 将重连成功的数据源加入健康列表
func (m *DBPoolManager) promoteToHealthy(cfg *config.DataSourceConfig, pool *DataSourcePool) bool {
	if cfg == nil || pool == nil {
		return false
	}

	// 保持与主状态一致，写锁期间完成替换和失败记录清理
	m.mu.Lock()
	defer m.mu.Unlock()

	// 清理旧的连接实例，避免句柄泄漏
	if existing := m.pools[cfg.Name]; existing != nil {
		if existing.DB != nil {
			if err := existing.DB.Close(); err != nil {
				m.logger.Error("提升健康状态前关闭旧连接失败",
					zap.String("datasource", cfg.Name),
					zap.Error(err))
			}
		}
	}

	// 将新连接加入健康列表并移除失败记录
	pool.markHealthy(time.Now())
	m.pools[cfg.Name] = pool
	delete(m.failedSources, cfg.Name)
	return true
}

// checkHealthyPools 扫描健康列表，自动降级无效连接
func (m *DBPoolManager) checkHealthyPools() {
	// 仅针对当前健康列表做心跳检测，发现异常立即降级
	pools := m.getHealthyPoolsSnapshot()
	if len(pools) == 0 {
		return
	}

	for _, pool := range pools {
		if pool == nil || pool.DB == nil || pool.Config == nil {
			continue
		}

		timeoutSeconds := pool.Config.QueryTimeout
		if timeoutSeconds <= 0 {
			timeoutSeconds = config.DefaultDataSourceConfig.QueryTimeout
		}

		// 采用数据源超时配置完成健康 ping
		ctx, cancel := context.WithTimeout(context.Background(), time.Duration(timeoutSeconds)*time.Second)
		err := pool.DB.PingContext(ctx)
		cancel()
		if err == nil {
			pool.markHealthy(time.Now())
			continue
		}

		// 检测失败后执行降级流程
		m.logger.Warn("健康数据源心跳检测失败，已转移至失败列表",
			zap.String("datasource", pool.Name),
			zap.Error(err))
		pool.markUnhealthy(time.Now())
		m.demoteToFailed(pool, err)
	}
}

// demoteToFailed 将健康连接降级为失败列表并关闭连接
func (m *DBPoolManager) demoteToFailed(pool *DataSourcePool, reason error) {
	if pool == nil {
		return
	}

	// 捕获当前失败时间并准备关闭旧连接
	now := time.Now()
	var dbToClose *sql.DB

	m.mu.Lock()
	pool.markUnhealthy(now)
	delete(m.pools, pool.Name)

	if pool.DB != nil {
		dbToClose = pool.DB
	}
	m.noteFailedDataSourceLocked(pool.Config, reason, now)
	m.mu.Unlock()

	if dbToClose != nil {
		// 关闭无效连接，释放资源
		if err := dbToClose.Close(); err != nil {
			m.logger.Error("关闭异常数据源连接失败",
				zap.String("datasource", pool.Name),
				zap.Error(err))
		}
	}
}

// MarkDatasourceFailed 将指定数据源标记为失败状态，供采集器检测失败时快速降级
func (m *DBPoolManager) MarkDatasourceFailed(name string, reason error) {
	if m == nil || name == "" {
		return
	}

	// 优先尝试从健康列表中取出现有连接，若存在则执行标准降级流程
	m.mu.RLock()
	pool, exists := m.pools[name]
	m.mu.RUnlock()
	if exists && pool != nil {
		if reason != nil {
			m.logger.Warn("采集器检测到连接异常，执行快速降级",
				zap.String("datasource", name),
				zap.Error(reason))
		} else {
			m.logger.Warn("采集器检测到连接异常，执行快速降级",
				zap.String("datasource", name))
		}
		m.demoteToFailed(pool, reason)
		return
	}

	// 如果健康列表中不存在，说明已降级或尚未初始化，补充失败记录以便后台重试
	if m.config != nil {
		if cfg := m.config.GetDataSourceByName(name); cfg != nil {
			if reason != nil {
				m.logger.Warn("采集器检测到连接异常，记录失败等待自动恢复",
					zap.String("datasource", name),
					zap.Error(reason))
			} else {
				m.logger.Warn("采集器检测到连接异常，记录失败等待自动恢复",
					zap.String("datasource", name))
			}
			m.noteFailedDataSource(cfg, reason)
		}
	}
}

// GetDatasourceHealthStatus 返回指定数据源的健康状态快照
func (m *DBPoolManager) GetDatasourceHealthStatus(name string) DatasourceHealthStatus {
	status := DatasourceHealthStatus{}
	if m == nil || name == "" {
		return status
	}

	m.mu.RLock()
	if pool, ok := m.pools[name]; ok && pool != nil {
		status.Healthy = pool.IsHealthy()
		status.LastCheck = pool.LastHealthCheck()
		status.Registered = true
		m.mu.RUnlock()
		return status
	}

	if failed, ok := m.failedSources[name]; ok && failed != nil {
		status.Healthy = false
		status.LastCheck = failed.LastAttempt
		status.LastError = failed.LastError
		status.Registered = true
		m.mu.RUnlock()
		return status
	}

	m.mu.RUnlock()
	return status
}

// IsDatasourceHealthy 判断指定数据源是否健康
func (m *DBPoolManager) IsDatasourceHealthy(name string) bool {
	return m.GetDatasourceHealthStatus(name).Healthy
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
		if pool != nil && pool.Config != nil && pool.Config.Enabled && pool.IsHealthy() {
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
		if pool != nil && pool.Config != nil && pool.Config.Enabled {
			pools = append(pools, pool)
		}
	}

	return pools
}

// Close 关闭所有连接池
func (m *DBPoolManager) Close() {
	// 步骤1：发送停止信号，只执行一次避免 panic
	m.stopOnce.Do(func() {
		close(m.stopChan)
	})

	// 步骤2：等待后台协程优雅退出，避免与清理阶段竞争
	m.wg.Wait()

	// 步骤3：统一关闭剩余连接并清空状态
	m.mu.Lock()
	defer m.mu.Unlock()

	// 逐个关闭数据库连接，记录潜在错误
	for name, pool := range m.pools {
		if pool.DB != nil {
			if err := pool.DB.Close(); err != nil {
				m.logger.Error("关闭数据源连接池失败",
					zap.String("datasource", name),
					zap.Error(err))
			}
		}
	}

	m.pools = make(map[string]*DataSourcePool)
	m.failedSources = make(map[string]*FailedDataSource)
}
