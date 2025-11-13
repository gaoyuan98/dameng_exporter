package collector

import (
	"context"
	"dameng_exporter/config"
	"dameng_exporter/logger"
	"dameng_exporter/utils"
	"database/sql"
	"sync"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"go.uber.org/zap"
)

// DbSystemEventWaitInfo 保存单行系统事件等待信息，其中字段使用 sql.Null* 类型以兼容数据库的 NULL 值。
type DbSystemEventWaitInfo struct {
	Event      sql.NullString
	TotalWaits sql.NullFloat64
}

// DbSystemEventWaitCollector 负责查询 V$SYSTEM_EVENT 并将事件名称及等待次数转换为 Prometheus 指标。
type DbSystemEventWaitCollector struct {
	db             *sql.DB
	eventWaitsDesc *prometheus.Desc
	dataSource     string

	// viewCheckOnce 用于确保视图存在性检查仅被执行一次，避免频繁访问系统表。
	viewCheckOnce sync.Once
	// viewExists 记录 V$SYSTEM_EVENT 视图检查结果，结合 viewCheckOnce 构成惰性缓存。
	viewExists bool
	// columnsCheckOnce 用于缓存列检查行为，只要一次确认即可复用结果。
	columnsCheckOnce sync.Once
	// columnsExist 标记 EVENT 与 TOTAL_WAITS 列是否存在，可防止后续扫描时报错。
	columnsExist bool
}

// SetDataSource 实现 DataSourceAware，用于在多数据源模式下注入当前数据源名称，便于日志与错误定位。
func (c *DbSystemEventWaitCollector) SetDataSource(name string) {
	c.dataSource = name
}

// NewDbSystemEventWaitCollector 构造函数，初始化指标描述符，标签为事件名称，指标类型为 Counter。
func NewDbSystemEventWaitCollector(db *sql.DB) MetricCollector {
	return &DbSystemEventWaitCollector{
		db: db,
		eventWaitsDesc: prometheus.NewDesc(
			dmdbms_system_event_waits_total,
			"Total waits grouped by system event",
			[]string{"event"},
			nil,
		),
	}
}

// Describe 将指标描述符发送给 Prometheus，用于在注册阶段声明指标元信息。
func (c *DbSystemEventWaitCollector) Describe(ch chan<- *prometheus.Desc) {
	ch <- c.eventWaitsDesc
}

// Collect 执行查询并逐行生成指标，包含数据库连接健康检查、查询超时控制以及错误处理。
func (c *DbSystemEventWaitCollector) Collect(ch chan<- prometheus.Metric) {
	// 1. 数据库连接健康检查，异常时直接跳过采集。
	if err := utils.CheckDBConnectionWithSource(c.db, c.dataSource); err != nil {
		return
	}

	// 2. 构建带超时的查询上下文。
	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(config.Global.GetQueryTimeout())*time.Second)
	defer cancel()

	// 3. 检查视图是否可用，兼容旧版本数据库缺失 V$SYSTEM_EVENT 的情况。
	if !c.checkSystemEventViewExists(ctx) {
		return
	}

	// 4. 检查 EVENT、TOTAL_WAITS 字段是否存在，避免后续扫描时报错。
	if !c.checkSystemEventColumnsExist(ctx) {
		return
	}

	// 5. 执行核心查询并将结果转换为指标。
	rows, err := c.db.QueryContext(ctx, config.QuerySystemEventWaitsSqlStr)
	if err != nil {
		utils.HandleDbQueryErrorWithSource(err, c.dataSource)
		return
	}
	defer rows.Close()

	for rows.Next() {
		var info DbSystemEventWaitInfo
		// 5.1 扫描单行记录，保留 NULL 状态以便统一转换。
		if err := rows.Scan(&info.Event, &info.TotalWaits); err != nil {
			logger.Logger.Error("Error scanning system event waits row",
				zap.Error(err),
				zap.String("data_source", c.dataSource))
			continue
		}

		// 5.2 将事件名称归一化，避免 Prometheus 标签留空。
		event := utils.NullStringToString(info.Event)
		if event == "" {
			event = "UNKNOWN"
		}

		// 5.3 输出 Counter 指标，TOTAL_WAITS 作为等待次数。
		ch <- prometheus.MustNewConstMetric(
			c.eventWaitsDesc,
			prometheus.CounterValue,
			utils.NullFloat64ToFloat64(info.TotalWaits),
			event,
		)
	}

	if err := rows.Err(); err != nil {
		logger.Logger.Error("Error iterating system event waits rows",
			zap.Error(err),
			zap.String("data_source", c.dataSource))
	}
}

// checkSystemEventViewExists 检查 V$SYSTEM_EVENT 视图是否存在，缺失时直接跳过采集。
func (c *DbSystemEventWaitCollector) checkSystemEventViewExists(ctx context.Context) bool {
	c.viewCheckOnce.Do(func() {
		// 通过 V$DYNAMIC_TABLES 判断目标视图是否被注册，避免访问不存在视图导致的 SQL 错误。
		var count int
		if err := c.db.QueryRowContext(ctx, config.QuerySystemEventViewExistsSqlStr).Scan(&count); err != nil {
			logger.Logger.Warnf("[%s] Failed to check V$SYSTEM_EVENT existence: %v", c.dataSource, err)
			c.viewExists = false
			return
		}
		c.viewExists = count > 0
		logger.Logger.Debugf("[%s] V$SYSTEM_EVENT exists: %v", c.dataSource, c.viewExists)
	})
	return c.viewExists
}

// checkSystemEventColumnsExist 检查 EVENT 与 TOTAL_WAITS 字段是否存在，缺失时跳过采集。
func (c *DbSystemEventWaitCollector) checkSystemEventColumnsExist(ctx context.Context) bool {
	c.columnsCheckOnce.Do(func() {
		// 查询系统列元数据，确认 EVENT 与 TOTAL_WAITS 是否都可用。
		var count int
		if err := c.db.QueryRowContext(ctx, config.QuerySystemEventColumnsExistSqlStr).Scan(&count); err != nil {
			logger.Logger.Warnf("[%s] Failed to check V$SYSTEM_EVENT columns: %v", c.dataSource, err)
			c.columnsExist = false
			return
		}
		c.columnsExist = count >= 2
		logger.Logger.Debugf("[%s] V$SYSTEM_EVENT columns valid: %v", c.dataSource, c.columnsExist)
	})
	return c.columnsExist
}
