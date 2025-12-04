package collector

import (
	"dameng_exporter/db"
	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"
)

// LabelInjector 标签注入器
type LabelInjector struct {
	dataSourceName string            // 数据源名称
	labels         map[string]string // 标签键值对
}

// NewLabelInjectorFromPool 从连接池创建标签注入器
func NewLabelInjectorFromPool(pool *db.DataSourcePool) *LabelInjector {
	// 复制池的标签
	labels := make(map[string]string)
	for k, v := range pool.Labels {
		labels[k] = v
	}

	// 确保包含数据源名称，优先使用注入的 datasource 标签
	if dsLabel, ok := labels["datasource"]; !ok || dsLabel == "" {
		labels["datasource"] = pool.Name
	}

	return &LabelInjector{
		dataSourceName: pool.Name,
		labels:         labels,
	}
}

// GetLabels 获取标签映射
func (li *LabelInjector) GetLabels() map[string]string {
	return li.labels
}

// MetricWrapper 用于包装原始指标并注入数据源标签
type MetricWrapper struct {
	metric   prometheus.Metric
	injector *LabelInjector
}

// NewMetricWrapper 创建指标包装器
func NewMetricWrapper(metric prometheus.Metric, injector *LabelInjector) *MetricWrapper {
	return &MetricWrapper{
		metric:   metric,
		injector: injector,
	}
}

// Desc 返回带有数据源标签的描述
func (mw *MetricWrapper) Desc() *prometheus.Desc {
	// 直接返回原始描述，描述符在创建时就应该包含所有标签
	return mw.metric.Desc()
}

// Write 将指标写入到dto.Metric，并注入数据源标签
func (mw *MetricWrapper) Write(out *dto.Metric) error {
	// 先让原始指标写入
	if err := mw.metric.Write(out); err != nil {
		return err
	}

	// 注入数据源标签 - 优化版本使用map提升查找效率
	labels := mw.injector.GetLabels()

	// 构建已存在标签的map，避免嵌套循环
	existingLabels := make(map[string]bool)
	for _, label := range out.Label {
		if label.Name != nil {
			existingLabels[*label.Name] = true
		}
	}

	// 只添加不存在的标签
	for key, value := range labels {
		if !existingLabels[key] {
			name, val := key, value
			out.Label = append(out.Label, &dto.LabelPair{
				Name:  &name,
				Value: &val,
			})
		}
	}

	return nil
}
