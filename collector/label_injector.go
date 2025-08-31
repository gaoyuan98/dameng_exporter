package collector

import (
	"dameng_exporter/db"
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

	// 确保包含数据源名称
	labels["datasource"] = pool.Name

	return &LabelInjector{
		dataSourceName: pool.Name,
		labels:         labels,
	}
}

// GetLabels 获取标签映射
func (li *LabelInjector) GetLabels() map[string]string {
	return li.labels
}
