package collector

import (
	"dameng_exporter/config"
	"dameng_exporter/db"
	"github.com/prometheus/client_golang/prometheus"
	"sort"
)

// LabelInjector 标签注入器
type LabelInjector struct {
	dataSourceName string            // 数据源名称
	labels         map[string]string // 标签键值对
	labelKeys      []string          // 标签键列表（用于保证顺序）
	labelValues    []string          // 标签值列表（与键对应）
}

// NewLabelInjector 创建标签注入器
func NewLabelInjector(dsConfig *config.DataSourceConfig) *LabelInjector {
	// 解析配置中的标签
	labels := dsConfig.ParseLabels()

	// 自动注入数据源名称标签
	labels["datasource"] = dsConfig.Name

	// 如果配置了优先级，也作为标签
	if dsConfig.Priority > 0 {
		labels["priority"] = priorityToString(dsConfig.Priority)
	}

	// 准备有序的标签键和值
	labelKeys := make([]string, 0, len(labels))
	for key := range labels {
		labelKeys = append(labelKeys, key)
	}
	sort.Strings(labelKeys) // 保证标签顺序一致

	labelValues := make([]string, len(labelKeys))
	for i, key := range labelKeys {
		labelValues[i] = labels[key]
	}

	return &LabelInjector{
		dataSourceName: dsConfig.Name,
		labels:         labels,
		labelKeys:      labelKeys,
		labelValues:    labelValues,
	}
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

	// 准备有序的标签键和值
	labelKeys := make([]string, 0, len(labels))
	for key := range labels {
		labelKeys = append(labelKeys, key)
	}
	sort.Strings(labelKeys)

	labelValues := make([]string, len(labelKeys))
	for i, key := range labelKeys {
		labelValues[i] = labels[key]
	}

	return &LabelInjector{
		dataSourceName: pool.Name,
		labels:         labels,
		labelKeys:      labelKeys,
		labelValues:    labelValues,
	}
}

// GetLabels 获取标签映射
func (li *LabelInjector) GetLabels() map[string]string {
	return li.labels
}

// GetLabelKeys 获取标签键列表
func (li *LabelInjector) GetLabelKeys() []string {
	return li.labelKeys
}

// GetLabelValues 获取标签值列表
func (li *LabelInjector) GetLabelValues() []string {
	return li.labelValues
}

// GetDataSourceName 获取数据源名称
func (li *LabelInjector) GetDataSourceName() string {
	return li.dataSourceName
}

// CreateMetricDesc 创建带标签的指标描述
func (li *LabelInjector) CreateMetricDesc(name, help string, variableLabels ...string) *prometheus.Desc {
	// 合并固定标签和可变标签
	allLabels := append(li.labelKeys, variableLabels...)
	return prometheus.NewDesc(name, help, allLabels, nil)
}

// CreateConstMetric 创建带标签的常量指标
func (li *LabelInjector) CreateConstMetric(desc *prometheus.Desc, valueType prometheus.ValueType, value float64, variableLabelValues ...string) (prometheus.Metric, error) {
	// 合并固定标签值和可变标签值
	allLabelValues := append(li.labelValues, variableLabelValues...)
	return prometheus.NewConstMetric(desc, valueType, value, allLabelValues...)
}

// AddLabelsToDesc 为现有描述添加数据源标签
func (li *LabelInjector) AddLabelsToDesc(originalDesc *prometheus.Desc) *prometheus.Desc {
	// 这个方法用于改造现有的Desc，添加数据源相关标签
	// 注意：Prometheus的Desc一旦创建就不能修改，所以需要重新创建

	// 获取原始描述的信息（这里简化处理，实际可能需要反射或其他方式）
	// 在实际使用中，建议直接使用CreateMetricDesc创建新的描述
	return li.CreateMetricDesc(
		"", // 名称需要从原始描述获取
		"", // 帮助信息需要从原始描述获取
	)
}

// priorityToString 将优先级转换为字符串
func priorityToString(priority int) string {
	switch priority {
	case 1:
		return "high"
	case 2:
		return "medium"
	case 3:
		return "low"
	default:
		return "unknown"
	}
}

// MultiSourceLabelManager 多数据源标签管理器
type MultiSourceLabelManager struct {
	injectors map[string]*LabelInjector // 按数据源名称索引的注入器
}

// NewMultiSourceLabelManager 创建多数据源标签管理器
func NewMultiSourceLabelManager(poolManager *db.DBPoolManager) *MultiSourceLabelManager {
	manager := &MultiSourceLabelManager{
		injectors: make(map[string]*LabelInjector),
	}

	// 为每个连接池创建标签注入器
	for _, pool := range poolManager.GetPools() {
		injector := NewLabelInjectorFromPool(pool)
		manager.injectors[pool.Name] = injector
	}

	return manager
}

// GetInjector 获取指定数据源的标签注入器
func (m *MultiSourceLabelManager) GetInjector(dataSourceName string) *LabelInjector {
	return m.injectors[dataSourceName]
}

// GetAllInjectors 获取所有标签注入器
func (m *MultiSourceLabelManager) GetAllInjectors() map[string]*LabelInjector {
	return m.injectors
}

// CreateGlobalDesc 创建全局指标描述（包含datasource标签）
func CreateGlobalDesc(name, help string, datasourceLabels []string, variableLabels ...string) *prometheus.Desc {
	// 确保包含datasource标签
	allLabels := []string{"datasource"}

	// 添加其他数据源相关标签
	for _, label := range datasourceLabels {
		if label != "datasource" {
			allLabels = append(allLabels, label)
		}
	}

	// 添加可变标签
	allLabels = append(allLabels, variableLabels...)

	return prometheus.NewDesc(name, help, allLabels, nil)
}

// ExtractLabelsFromConfig 从配置中提取所有可能的标签键
func ExtractLabelsFromConfig(config *config.MultiSourceConfig) []string {
	labelSet := make(map[string]bool)

	// 收集所有数据源中出现的标签键
	for _, ds := range config.DataSources {
		labels := ds.ParseLabels()
		for key := range labels {
			labelSet[key] = true
		}
	}

	// 添加固定标签
	labelSet["datasource"] = true
	labelSet["priority"] = true

	// 转换为有序列表
	labelKeys := make([]string, 0, len(labelSet))
	for key := range labelSet {
		labelKeys = append(labelKeys, key)
	}
	sort.Strings(labelKeys)

	return labelKeys
}
