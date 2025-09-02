package collector

import (
	"context"
	"dameng_exporter/config"
	"dameng_exporter/logger"
	"database/sql"
	"fmt"
	"github.com/duke-git/lancet/v2/convertor"
	"github.com/prometheus/client_golang/prometheus"
	"go.uber.org/zap"
	"strings"
	"time"
)

// CustomMetrics 结构体，封装多个 Prometheus Collectors
type CustomMetrics struct {
	metrics    map[string]prometheus.Collector
	db         *sql.DB
	sqlConfig  config.CustomConfig
	dataSource string // 数据源名称
}

// SetDataSource 实现DataSourceAware接口
func (cm *CustomMetrics) SetDataSource(name string) {
	cm.dataSource = name
}

// NewCustomMetrics 返回一个封装了数据库和配置的 CustomMetrics 实例
func NewCustomMetrics(db *sql.DB, sqlConfig config.CustomConfig) *CustomMetrics {

	// 预定义所有指标
	metrics := make(map[string]prometheus.Collector)
	for _, metric := range sqlConfig.Metrics {
		// 使用原始标签列表
		labels := metric.Labels

		for field, desc := range metric.MetricsDesc {
			// 根据 MetricsType 创建 CounterVec 或 GaugeVec
			initField := metric.MetricsType[field]
			field = "dmdbms_" + metric.Context + "_" + field
			switch initField {
			case "counter":
				counter := prometheus.NewCounterVec(
					prometheus.CounterOpts{
						Name: field,
						Help: desc,
					},
					labels,
				)
				metrics[field] = counter
			default:
				gauge := prometheus.NewGaugeVec(
					prometheus.GaugeOpts{
						Name: field,
						Help: desc,
					},
					labels,
				)
				metrics[field] = gauge
			}
		}
	}
	return &CustomMetrics{
		metrics:    metrics,
		db:         db,
		sqlConfig:  sqlConfig,
		dataSource: "default", // 设置默认数据源名称，避免空值
	}
}

// Describe 方法，用于实现 prometheus.Collector 接口
func (cm *CustomMetrics) Describe(ch chan<- *prometheus.Desc) {
	for _, metric := range cm.metrics {
		metric.Describe(ch)
	}
}

// Collect 方法，用于实现 prometheus.Collector 接口
func (cm *CustomMetrics) Collect(ch chan<- prometheus.Metric) {
	// 始终使用数据源名称，如果为空则使用"default"
	dsName := cm.dataSource
	if dsName == "" {
		dsName = "default"
	}
	logger.Logger.Debugf("[%s] Collecting custom metrics...", dsName)

	if err := cm.db.Ping(); err != nil {
		logger.Logger.Error("Database connection is not available",
			zap.String("datasource", dsName),
			zap.Error(err))
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(config.Global.GetQueryTimeout())*time.Second)
	defer cancel()

	// 遍历配置中的每个 Metric，执行查询并收集数据
	for _, metric := range cm.sqlConfig.Metrics {
		results, err := queryDynamicDatabase(ctx, cm.db, metric.Request)
		if err != nil {
			logger.Logger.Error("查询数据库错误",
				zap.String("datasource", dsName),
				zap.String("context", metric.Context),
				zap.Error(err))
			continue
		}

		for _, result := range results {
			// 创建标签值列表
			labelValues := make([]string, len(metric.Labels))

			for i, label := range metric.Labels {
				if val, ok := result[label]; ok {
					labelValues[i] = fmt.Sprintf("%v", val)
				}
			}

			for field, value := range result {
				//如果metric.Labels中包含field 忽略大小写 则跳过
				if strings.EqualFold(field, strings.Join(metric.Labels, "")) {
					continue
				}
				if collector, ok := cm.metrics["dmdbms_"+metric.Context+"_"+field]; ok {
					conver_float, err := convertor.ToFloat(value)
					if err != nil {
						conver_float = 0.0
					}
					switch metric.MetricsType[field] {
					case "counter":
						collector.(*prometheus.CounterVec).WithLabelValues(labelValues...).Add(conver_float)
					default:
						collector.(*prometheus.GaugeVec).WithLabelValues(labelValues...).Set(conver_float)
					}
					//	logger.Logger.Debugf("metric %s value %v labelValues %v", field, conver_float, labelValues)
				}
			}

		}
	}
	// 手动调用 Prometheus 的 Collect 方法来注册所有已更新的 metrics
	for _, collector := range cm.metrics {
		collector.Collect(ch)
	}
}

// queryDynamicDatabase 函数返回 SQL 查询结果，包括所有字段及数据
func queryDynamicDatabase(ctx context.Context, db *sql.DB, query string) ([]map[string]interface{}, error) {

	rows, err := db.QueryContext(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("数据库查询出错: %w", err)
	}
	defer rows.Close()

	columns, err := rows.Columns()
	if err != nil {
		return nil, fmt.Errorf("获取列名出错: %w", err)
	}

	var results []map[string]interface{}
	for rows.Next() {
		row := make(map[string]interface{})
		values := make([]interface{}, len(columns))
		valuePtrs := make([]interface{}, len(columns))

		for i := range columns {
			valuePtrs[i] = &values[i]
		}

		if err := rows.Scan(valuePtrs...); err != nil {
			return nil, fmt.Errorf("扫描行出错: %w", err)
		}

		for i, col := range columns {
			//将字段转为小写
			row[strings.ToLower(col)] = values[i]
		}

		results = append(results, row)
	}

	return results, nil
}
