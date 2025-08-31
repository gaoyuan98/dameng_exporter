package collector

import (
	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"
)

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
	// 获取原始描述
	originalDesc := mw.metric.Desc()
	if originalDesc == nil {
		return nil
	}

	// 返回原始描述，描述符在创建时就应该包含所有标签
	return originalDesc
}

// Write 将指标写入到dto.Metric，并注入数据源标签
func (mw *MetricWrapper) Write(out *dto.Metric) error {
	// 先让原始指标写入
	if err := mw.metric.Write(out); err != nil {
		return err
	}

	// 注入数据源标签
	labels := mw.injector.GetLabels()
	for key, value := range labels {
		// 检查是否已存在该标签
		exists := false
		for _, label := range out.Label {
			if label.Name != nil && *label.Name == key {
				exists = true
				break
			}
		}

		// 如果不存在，添加标签
		if !exists {
			name := key
			val := value
			out.Label = append(out.Label, &dto.LabelPair{
				Name:  &name,
				Value: &val,
			})
		}
	}

	return nil
}
