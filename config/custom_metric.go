package config

import (
	"fmt"
	"github.com/BurntSushi/toml"
	"os"
)

// 定义整体配置的结构体
type CustomConfig struct {
	Metrics []CustomMetric `toml:"metric"`
}

// 定义单个 metric 的结构体
type CustomMetric struct {
	Context          string            `toml:"context"`
	Labels           []string          `toml:"labels,omitempty"`
	Request          string            `toml:"request"`
	MetricsDesc      map[string]string `toml:"metricsdesc"`
	MetricsType      map[string]string `toml:"metricstype"`                // 新增字段，定义每个指标的类型
	IgnoreZeroResult bool              `toml:"ignorezeroresult,omitempty"` // 新增字段，是否忽略零值结果
}

// ParseCustomConfig 解析自定义指标定义文件
// 支持的文件扩展名：.metrics, .sql.toml, .queries.toml
func ParseCustomConfig(filePath string) (CustomConfig, error) {
	var config CustomConfig

	// 读取文件内容
	content, err := os.ReadFile(filePath)
	if err != nil {
		return CustomConfig{}, fmt.Errorf("读取自定义指标定义文件失败: %w", err)
	}

	// 解析 TOML 内容
	if _, err := toml.Decode(string(content), &config); err != nil {
		return CustomConfig{}, fmt.Errorf("解析自定义指标定义文件失败: %w", err)
	}
	return config, nil
}
