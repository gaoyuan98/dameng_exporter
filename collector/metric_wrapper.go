package collector

import (
	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"
)

// MetricWrapper 指标包装器
//
// MetricWrapper 是装饰器模式的核心实现，用于包装原始的 Prometheus 指标并动态注入数据源标签。
// 它实现了 prometheus.Metric 接口，在不修改原始指标采集器的情况下，
// 为多数据源环境下的所有指标添加数据源标识和用户自定义标签。
//
// 设计模式：装饰器模式 (Decorator Pattern)
// - 组件接口：prometheus.Metric
// - 具体组件：各种原始指标 (如 prometheus.GaugeVec 创建的指标)
// - 装饰器：MetricWrapper
// - 策略：LabelInjector (提供标签注入策略)

// 工作流程：
//  1. 包装原始指标和标签注入器
//  2. 代理 Desc() 方法调用到原始指标
//  3. 在 Write() 方法中添加标签注入逻辑
//  4. 将增强后的指标输出到 Prometheus
type MetricWrapper struct {
	metric   prometheus.Metric // 原始 Prometheus 指标，保持所有原始功能
	injector *LabelInjector    // 标签注入器，提供标签数据和注入策略
}

// NewMetricWrapper 创建指标包装器
//
// 构造函数创建一个新的指标包装器实例，将原始指标和标签注入器组合起来。
// 这是装饰器模式的标准构造方式。
//
// 参数：
//   - metric: 原始的 Prometheus 指标实例，可以是任何实现了 prometheus.Metric 接口的对象
//   - injector: 标签注入器，提供需要注入的标签数据
//
// 返回：
//   - *MetricWrapper: 包装后的指标实例，实现了 prometheus.Metric 接口
//
// 使用示例：
//
//	originalMetric := prometheus.NewGauge(prometheus.GaugeOpts{...})
//	labelInjector := NewLabelInjectorFromPool(pool)
//	wrappedMetric := NewMetricWrapper(originalMetric, labelInjector)
//	ch <- wrappedMetric  // 发送到 Prometheus 采集通道
//
// 注意：
// 包装后的指标保持与原始指标相同的接口，可以无缝替代原始指标使用。
func NewMetricWrapper(metric prometheus.Metric, injector *LabelInjector) *MetricWrapper {
	return &MetricWrapper{
		metric:   metric,
		injector: injector,
	}
}

// Desc 返回指标描述符
//
// 该方法实现了 prometheus.Metric 接口的 Desc() 方法，直接代理到原始指标的 Desc() 方法。
// 指标描述符定义了指标的名称、帮助信息和标签名称等元数据。
//
// 设计说明：
// 我们不在这里修改描述符，因为：
// 1. 描述符是指标的静态元数据，应该在创建时确定
// 2. 标签的动态注入在 Write() 方法中进行，更加灵活
// 3. 保持与原始指标描述符的一致性，避免 Prometheus 注册冲突
//
// 返回：
//   - *prometheus.Desc: 原始指标的描述符，如果原始指标返回 nil 则返回 nil
func (mw *MetricWrapper) Desc() *prometheus.Desc {
	// 直接代理到原始指标，保持描述符的原始性
	originalDesc := mw.metric.Desc()
	if originalDesc == nil {
		return nil
	}

	// 返回原始描述符，标签注入在 Write() 阶段进行
	// 这样设计的好处是保持描述符的稳定性，避免注册时的冲突
	return originalDesc
}

// Write 将指标数据写入到 DTO 对象并注入标签
//
// 该方法实现了 prometheus.Metric 接口的核心方法，负责将指标数据序列化为 Prometheus 的数据传输对象。
// 这是标签注入的核心实现，采用两阶段处理：
// 1. 让原始指标写入基础数据
// 2. 动态注入数据源相关标签
//
// 参数：
//   - out: Prometheus 数据传输对象指针，用于承载指标数据和标签
//
// 返回：
//   - error: 写入过程中的错误，nil 表示成功
//
// 处理流程：
//  1. 调用原始指标的 Write() 方法，获取基础指标数据
//  2. 从标签注入器获取需要注入的标签列表
//  3. 遍历每个标签，检查是否已存在同名标签
//  4. 对于不存在的标签，添加到指标的标签列表中
//  5. 返回处理结果

// 错误处理：
// - 如果原始指标写入失败，直接返回错误，不进行标签注入
// - 标签注入过程不会产生错误，因为我们只是在内存中操作 DTO 对象
func (mw *MetricWrapper) Write(out *dto.Metric) error {
	// 第一阶段：让原始指标写入基础数据 (指标值、时间戳等)
	if err := mw.metric.Write(out); err != nil {
		return err
	}

	// 第二阶段：注入数据源标签
	// 获取需要注入的所有标签 (datasource、用户自定义标签等)
	labels := mw.injector.GetLabels()

	// 性能优化：预分配容量，避免切片动态扩容
	if out.Label == nil {
		out.Label = make([]*dto.LabelPair, 0, len(labels))
	} else {
		// 如果已有标签，预留额外容量
		currentCap := cap(out.Label)
		requiredCap := len(out.Label) + len(labels)
		if currentCap < requiredCap {
			newLabels := make([]*dto.LabelPair, len(out.Label), requiredCap)
			copy(newLabels, out.Label)
			out.Label = newLabels
		}
	}

	// 遍历每个需要注入的标签
	for key, value := range labels {
		// 检查该标签是否已存在于原始指标中
		// 这是为了避免覆盖原始指标的重要标签
		exists := false
		for _, existingLabel := range out.Label {
			if existingLabel.Name != nil && *existingLabel.Name == key {
				exists = true
				break
			}
		}

		// 只有当标签不存在时才添加，遵循"不覆盖原有标签"的原则
		if !exists {
			// 创建新的 LabelPair 对象
			newLabel := &dto.LabelPair{}

			// 获取标签的缓存指针（避免每次都创建新字符串）
			namePtr := mw.injector.GetLabelNamePtr(key)
			valuePtr := mw.injector.GetLabelValuePtr(key)

			// 如果缓存中没有，则创建新的（兜底方案）
			if namePtr == nil {
				name := key
				namePtr = &name
			}
			if valuePtr == nil {
				val := value
				valuePtr = &val
			}

			newLabel.Name = namePtr
			newLabel.Value = valuePtr

			// 将新标签添加到指标的标签列表中
			out.Label = append(out.Label, newLabel)
		}
	}

	return nil
}
