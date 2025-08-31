package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"
)

// TimedRegistry 支持时间统计的Registry包装器
type TimedRegistry struct {
	*prometheus.Registry
}

// NewTimedRegistry 创建一个新的带时间统计的Registry
func NewTimedRegistry() *TimedRegistry {
	return &TimedRegistry{
		Registry: prometheus.NewRegistry(),
	}
}

// Gather 实现prometheus.Gatherer接口，在每次scrape开始时记录时间
func (r *TimedRegistry) Gather() ([]*dto.MetricFamily, error) {
	// 标记新的scrape开始
	NewScrape()

	// 调用原始的Gather方法
	return r.Registry.Gather()
}
