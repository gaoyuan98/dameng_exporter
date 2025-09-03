package collector

import (
	"context"
	"dameng_exporter/config"
	"dameng_exporter/logger"
	"dameng_exporter/utils"
	"database/sql"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"go.uber.org/zap"
)

// DictCacheInfo 数据字典缓存信息结构体
type DictCacheInfo struct {
	lruDiscard      sql.NullFloat64
	disabledSize    sql.NullFloat64
	disabledDictNum sql.NullFloat64
}

// DbDictCacheCollector 数据字典缓存信息收集器
type DbDictCacheCollector struct {
	db                 *sql.DB
	dictCacheTotalDesc *prometheus.Desc // 数据字典缓存计数指标（Counter）
	dataSource         string           // 数据源名称

	// 每个实例独立的字段检查缓存
	fieldCheckOnce  sync.Once
	availableFields []string // 存在的字段列表
}

// SetDataSource 实现DataSourceAware接口
func (c *DbDictCacheCollector) SetDataSource(name string) {
	c.dataSource = name
}

// NewDbDictCacheCollector 创建数据字典缓存信息收集器
// 参数:
//   - db: 数据库连接
//
// 返回值:
//   - MetricCollector: 实现了MetricCollector接口的收集器实例
func NewDbDictCacheCollector(db *sql.DB) MetricCollector {
	return &DbDictCacheCollector{
		db: db,
		dictCacheTotalDesc: prometheus.NewDesc(
			dmdbms_dict_cache_total,
			"DM database dictionary cache total counters",
			[]string{"cache_metric_type"}, // 使用标签区分不同的计数器类型
			nil,
		),
	}
}

func (c *DbDictCacheCollector) Describe(ch chan<- *prometheus.Desc) {
	ch <- c.dictCacheTotalDesc
}

// checkDictCacheFields 检查V$DB_CACHE视图中哪些字段存在
func (c *DbDictCacheCollector) checkDictCacheFields(ctx context.Context) []string {
	c.fieldCheckOnce.Do(func() {
		c.availableFields = []string{}

		// 检查每个字段是否存在
		fields := []string{"LRU_DISCARD", "DISABLED_SIZE", "DISABLED_DICT_NUM"}
		for _, field := range fields {
			query := fmt.Sprintf("SELECT COUNT(*) FROM V$DYNAMIC_TABLE_COLUMNS WHERE TABNAME = 'V$DB_CACHE' AND COLNAME = '%s'", field)
			var count int
			if err := c.db.QueryRowContext(ctx, query).Scan(&count); err != nil {
				logger.Logger.Warnf("[%s] Failed to check V$DB_CACHE field %s: %v", c.dataSource, field, err)
				continue
			}
			if count > 0 {
				c.availableFields = append(c.availableFields, field)
				logger.Logger.Debugf("[%s] V$DB_CACHE field %s exists", c.dataSource, field)
			}
		}

		logger.Logger.Infof("[%s] V$DB_CACHE available fields: %v", c.dataSource, c.availableFields)
	})
	return c.availableFields
}

// buildDynamicQuery 根据存在的字段动态构建查询SQL
func (c *DbDictCacheCollector) buildDynamicQuery(fields []string) string {
	if len(fields) == 0 {
		return ""
	}

	selectParts := []string{}
	for _, field := range fields {
		selectParts = append(selectParts, field)
	}

	// 为不存在的字段添加NULL占位符，保持结果集结构一致
	allFields := []string{"LRU_DISCARD", "DISABLED_SIZE", "DISABLED_DICT_NUM"}
	for _, field := range allFields {
		found := false
		for _, availField := range fields {
			if field == availField {
				found = true
				break
			}
		}
		if !found {
			selectParts = append(selectParts, "NULL AS "+field)
		}
	}

	// 按照固定顺序排列字段
	orderedParts := []string{}
	for _, field := range allFields {
		for _, part := range selectParts {
			if strings.Contains(part, field) {
				orderedParts = append(orderedParts, part)
				break
			}
		}
	}

	return fmt.Sprintf("SELECT /*+DM_EXPORTER*/ %s FROM V$DB_CACHE", strings.Join(orderedParts, ", "))
}

func (c *DbDictCacheCollector) Collect(ch chan<- prometheus.Metric) {
	if err := utils.CheckDBConnectionWithSource(c.db, c.dataSource); err != nil {
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(config.Global.GetQueryTimeout())*time.Second)
	defer cancel()

	// 检查可用字段
	availableFields := c.checkDictCacheFields(ctx)
	if len(availableFields) == 0 {
		logger.Logger.Warnf("[%s] No dictionary cache fields available in V$DB_CACHE", c.dataSource)
		return
	}

	// 构建动态查询SQL
	query := c.buildDynamicQuery(availableFields)
	if query == "" {
		return
	}

	//	logger.Logger.Debugf("[%s] Executing dictionary cache query: %s", c.dataSource, query)

	// 执行查询
	var dictCacheInfo DictCacheInfo
	err := c.db.QueryRowContext(ctx, query).Scan(
		&dictCacheInfo.lruDiscard,
		&dictCacheInfo.disabledSize,
		&dictCacheInfo.disabledDictNum,
	)

	if err != nil {
		if err == sql.ErrNoRows {
			logger.Logger.Debugf("[%s] No dictionary cache data available", c.dataSource)
		} else {
			logger.Logger.Error(fmt.Sprintf("[%s] Error querying dictionary cache info", c.dataSource), zap.Error(err))
		}
		return
	}

	// 发送指标到Prometheus
	// LRU_DISCARD - 由于缓存池已满导致字典对象被淘汰的次数
	if dictCacheInfo.lruDiscard.Valid {
		ch <- prometheus.MustNewConstMetric(
			c.dictCacheTotalDesc,
			prometheus.CounterValue,
			dictCacheInfo.lruDiscard.Float64,
			"lru_discard",
		)
	}

	// DISABLED_SIZE - 被淘汰字典对象的空间，单位字节（累计值）
	if dictCacheInfo.disabledSize.Valid {
		ch <- prometheus.MustNewConstMetric(
			c.dictCacheTotalDesc,
			prometheus.CounterValue,
			dictCacheInfo.disabledSize.Float64,
			"disabled_size_bytes",
		)
	}

	// DISABLED_DICT_NUM - 缓存池中被淘汰字典对象的总数（累计值）
	if dictCacheInfo.disabledDictNum.Valid {
		ch <- prometheus.MustNewConstMetric(
			c.dictCacheTotalDesc,
			prometheus.CounterValue,
			dictCacheInfo.disabledDictNum.Float64,
			"disabled_dict_num",
		)
	}
}
