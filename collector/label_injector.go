// Package collector 提供 Prometheus 指标采集相关功能
package collector

import (
	"dameng_exporter/db"
)

// LabelInjector 标签注入器
//
// 标签注入器负责管理和提供多数据源环境下的标签数据，是装饰器模式中的策略组件。
// 它从数据源连接池中提取标签配置，并确保每个指标都包含数据源标识标签。
//
// 设计目的：
// - 统一管理多数据源环境下的标签注入逻辑
// - 提供标签数据的统一访问接口
// - 支持用户自定义标签和系统自动标签的合并
// - 缓存标签字符串指针，减少内存分配
//
// 使用场景：
// 在多数据源监控环境中，每个采集到的指标都需要标识其来源数据源，
// LabelInjector 确保所有指标都包含必要的数据源标签信息。
//
// 性能优化：
// - 预先创建并缓存所有标签的字符串指针
// - 避免在每次注入时重复创建字符串
// - 使用指针映射提供快速查找
type LabelInjector struct {
	dataSourceName string             // 数据源名称，用于标识指标来源
	labels         map[string]string  // 标签键值对映射，包含用户自定义标签和系统标签
	labelNamePtrs  map[string]*string // 标签名称指针缓存，避免重复创建
	labelValuePtrs map[string]*string // 标签值指针缓存，避免重复创建
}

// NewLabelInjectorFromPool 从数据源连接池创建标签注入器
//
// 该构造函数从数据源连接池中提取标签配置，创建一个标签注入器实例。
// 它会复制连接池中的所有用户自定义标签，并自动添加 "datasource" 系统标签。
//
// 参数：
//   - pool: 数据源连接池，包含数据源名称和用户配置的标签信息
//
// 返回：
//   - *LabelInjector: 标签注入器实例，包含完整的标签映射
//
// 标签处理逻辑：
//  1. 复制用户在配置文件中定义的所有标签 (如 role=master)
//  2. 自动添加 "datasource" 标签，值为数据源名称
//  3. 创建标签映射的深拷贝，避免并发修改问题
//  4. 预创建所有标签的字符串指针，减少运行时内存分配
//
// 使用示例：
//
//	pool := dbPoolManager.GetPool("master")
//	injector := NewLabelInjectorFromPool(pool)
//	labels := injector.GetLabels() // {"datasource": "master", "role": "master"}
func NewLabelInjectorFromPool(pool *db.DataSourcePool) *LabelInjector {
	// 计算标签总数，用于预分配映射容量
	labelCount := len(pool.Labels) + 1 // +1 for datasource label

	// 创建标签映射的深拷贝，避免修改原始数据
	labels := make(map[string]string, labelCount)
	for k, v := range pool.Labels {
		labels[k] = v
	}

	// 强制添加数据源标识标签，这是多数据源监控的核心标签
	// 该标签用于在 Prometheus 和 Grafana 中区分不同数据源的指标
	labels["datasource"] = pool.Name

	// 性能优化：预创建所有标签的字符串指针
	// 这样在 Write 方法中就不需要每次都创建新的字符串
	labelNamePtrs := make(map[string]*string, labelCount)
	labelValuePtrs := make(map[string]*string, labelCount)

	for key, value := range labels {
		// 创建标签名称和值的持久化副本
		keyCopy := key
		valueCopy := value
		labelNamePtrs[key] = &keyCopy
		labelValuePtrs[key] = &valueCopy
	}

	return &LabelInjector{
		dataSourceName: pool.Name,
		labels:         labels,
		labelNamePtrs:  labelNamePtrs,
		labelValuePtrs: labelValuePtrs,
	}
}

// GetLabels 获取完整的标签映射
//
// 返回包含所有标签的映射，包括用户自定义标签和系统自动添加的标签。
// 返回的映射是内部映射的引用，调用者不应修改返回的映射内容。
//
// 返回：
//   - map[string]string: 标签键值对映射
//   - 用户自定义标签：来自配置文件的 labels 字段
//   - 系统标签：自动添加的 "datasource" 标签
//
// 注意：
// 返回的是内部映射的引用，为了性能考虑没有进行拷贝。
// 调用者应该只读使用，不要修改返回的映射内容。
func (li *LabelInjector) GetLabels() map[string]string {
	return li.labels
}

// GetLabelNamePtr 获取缓存的标签名称指针
//
// 该方法返回预先创建的标签名称字符串指针，避免在每次标签注入时
// 重复创建相同的字符串，从而减少内存分配和 GC 压力。
//
// 参数：
//   - key: 标签名称
//
// 返回：
//   - *string: 标签名称的指针，如果不存在则返回 nil
//
// 性能优势：
// - 避免重复创建字符串对象
// - 减少内存分配次数
// - 降低 GC 压力
func (li *LabelInjector) GetLabelNamePtr(key string) *string {
	return li.labelNamePtrs[key]
}

// GetLabelValuePtr 获取缓存的标签值指针
//
// 该方法返回预先创建的标签值字符串指针，避免在每次标签注入时
// 重复创建相同的字符串，从而减少内存分配和 GC 压力。
//

func (li *LabelInjector) GetLabelValuePtr(key string) *string {
	return li.labelValuePtrs[key]
}
