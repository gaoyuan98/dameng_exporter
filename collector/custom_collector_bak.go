package collector

//
//import (
//	"context"
//	"dameng_exporter/config"
//	"dameng_exporter/logger"
//	"database/sql"
//	"fmt"
//	"github.com/duke-git/lancet/v2/convertor"
//	"github.com/prometheus/client_golang/prometheus"
//	"go.uber.org/zap"
//	"strings"
//	"time"
//)
//
//// CustomMetrics 结构体，封装多个 Prometheus Collectors
//type CustomMetrics struct {
//	metrics   map[string]prometheus.Collector
//	db        *sql.DB
//	sqlConfig config.CustomConfig
//}
//
//// 初始化 CustomMetrics，避免重复注册指标
//func NewCustomMetrics(db *sql.DB, sqlConfig config.CustomConfig) *CustomMetrics {
//	metrics := make(map[string]prometheus.Collector)
//
//	for _, metric := range sqlConfig.Metrics {
//		// 添加固定标签 host_name
//		labels := append([]string{"host_name"}, metric.Labels...)
//
//		for field, desc := range metric.MetricsDesc {
//			metricName := "dmdbms_" + metric.Context + "_" + field
//			if _, exists := metrics[metricName]; exists {
//				// 如果该指标已存在，跳过注册
//				continue
//			}
//
//			// 根据指标类型创建 CounterVec 或 GaugeVec
//			initField := metric.MetricsType[field]
//			switch initField {
//			case "counter":
//				counter := prometheus.NewCounterVec(
//					prometheus.CounterOpts{
//						Name: metricName,
//						Help: desc,
//					},
//					labels,
//				)
//				metrics[metricName] = counter
//			default:
//				gauge := prometheus.NewGaugeVec(
//					prometheus.GaugeOpts{
//						Name: metricName,
//						Help: desc,
//					},
//					labels,
//				)
//				metrics[metricName] = gauge
//			}
//		}
//	}
//	return &CustomMetrics{
//		metrics:   metrics,
//		db:        db,
//		sqlConfig: sqlConfig,
//	}
//}
//
//// Describe 方法，用于实现 prometheus.Collector 接口
//func (cm *CustomMetrics) Describe(ch chan<- *prometheus.Desc) {
//	for _, metric := range cm.metrics {
//		metric.Describe(ch)
//	}
//}
//
//// Collect 方法，用于实现 prometheus.Collector 接口
//func (cm *CustomMetrics) Collect(ch chan<- prometheus.Metric) {
//
//	if err := cm.db.Ping(); err != nil {
//		logger.Logger.Error("Database connection is not available", zap.Error(err))
//		return
//	}
//	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(config.GlobalConfig.QueryTimeout)*time.Second)
//	defer cancel()
//
//	for _, metric := range cm.sqlConfig.Metrics {
//		results, err := queryDynamicDatabase(ctx, cm.db, metric.Request)
//		if err != nil {
//			logger.Logger.Error("查询数据库时发生错误", zap.Error(err))
//			continue
//		}
//
//		for _, result := range results {
//			// 构造标签值数组，并记录 host_name
//			labelValues := make([]string, len(metric.Labels)+1)
//			labelValues[0] = config.GetHostName() // host_name 是固定标签
//
//			for i, label := range metric.Labels {
//				if val, ok := result[label]; ok {
//					labelValues[i+1] = fmt.Sprintf("%v", val)
//				}
//			}
//
//			// 遍历 result 中的每个字段，匹配 MetricsDesc 的字段名称
//			for field, value := range result {
//				metricName := "dmdbms_" + metric.Context + "_" + field
//				if collector, ok := cm.metrics[metricName]; ok {
//					// 将值转换为 float 类型
//					converFloat, err := convertor.ToFloat(value)
//					if err != nil {
//						converFloat = 0.0
//					}
//
//					// 根据指标类型更新值，并记录调试日志
//					switch metric.MetricsType[field] {
//					case "counter":
//						collector.(*prometheus.CounterVec).WithLabelValues(labelValues...).Add(converFloat)
//					default:
//						collector.(*prometheus.GaugeVec).WithLabelValues(labelValues...).Set(converFloat)
//					}
//
//					// 输出调试信息，显示当前更新的指标名称、标签值、和数值
//					//logger.Logger.Debug("更新指标信息",
//					//	zap.String("指标名称", metricName),
//					//	zap.Any("标签值", labelValues),
//					//	zap.Float64("数值", converFloat),
//					//)
//				} else {
//					// 输出警告，说明当前字段没有对应的指标
//					//logger.Logger.Warn("未找到匹配的指标",
//					//	zap.String("字段", field),
//					//	zap.String("构造的指标名称", metricName),
//					//	zap.Any("所有指标", cm.metrics),
//					//)
//				}
//			}
//		}
//	}
//
//	// 注册所有更新的指标到 Prometheus
//	for _, collector := range cm.metrics {
//		collector.Collect(ch)
//	}
//}
//
//// queryDynamicDatabase 函数返回 SQL 查询结果，包括所有字段及数据
//func queryDynamicDatabase(ctx context.Context, db *sql.DB, query string) ([]map[string]interface{}, error) {
//
//	rows, err := db.QueryContext(ctx, query)
//	if err != nil {
//		return nil, fmt.Errorf("数据库查询出错: %w", err)
//	}
//	defer rows.Close()
//
//	columns, err := rows.Columns()
//	if err != nil {
//		return nil, fmt.Errorf("获取列名出错: %w", err)
//	}
//
//	var results []map[string]interface{}
//	for rows.Next() {
//		row := make(map[string]interface{})
//		values := make([]interface{}, len(columns))
//		valuePtrs := make([]interface{}, len(columns))
//
//		for i := range columns {
//			valuePtrs[i] = &values[i]
//		}
//
//		if err := rows.Scan(valuePtrs...); err != nil {
//			return nil, fmt.Errorf("扫描行出错: %w", err)
//		}
//
//		for i, col := range columns {
//			//将字段转为小写
//			row[strings.ToLower(col)] = values[i]
//		}
//
//		results = append(results, row)
//	}
//
//	return results, nil
//}
