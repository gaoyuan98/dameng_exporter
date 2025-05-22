package collector

import (
	"dameng_exporter/config"
	"dameng_exporter/logger"
	"runtime"
	"time"

	"github.com/prometheus/client_golang/prometheus"
)

// BuildInfoCollector 结构体
type BuildInfoCollector struct {
	buildInfoDesc *prometheus.Desc
}

// NewBuildInfoCollector 创建一个新的构建信息收集器
/*# HELP node_exporter_build_info A metric with a constant '1' value labeled by version, revision, branch, goversion from which node_exporter was built, and the goos and goarch for the build.
# TYPE node_exporter_build_info gauge
node_exporter_build_info{branch="HEAD",goarch="amd64",goos="linux",goversion="go1.21.4",revision="7333465abf9efba81876303bb57e6fadb946041b",tags="netgo osusergo static_build",version="1.7.0"} 1
*/

func NewBuildInfoCollector() *BuildInfoCollector {
	return &BuildInfoCollector{
		buildInfoDesc: prometheus.NewDesc(
			dameng_exporter_build_info,
			"A metric with a constant '1' value labeled by version, revision, branch, goversion from which dameng_exporter was built, and the goos and goarch for the build.",
			[]string{"host_name", "version", "revision", "branch", "goversion", "goos", "goarch"},
			nil,
		),
	}
}

// Describe 实现 prometheus.Collector 接口
func (c *BuildInfoCollector) Describe(ch chan<- *prometheus.Desc) {
	ch <- c.buildInfoDesc
}

// Collect 实现 prometheus.Collector 接口
func (c *BuildInfoCollector) Collect(ch chan<- prometheus.Metric) {
	funcStart := time.Now()
	defer func() {
		duration := time.Since(funcStart)
		logger.Logger.Debugf("build info func exec time: %vms", duration.Milliseconds())
	}()

	// 获取构建信息
	revision := "d49755e011130f8e4d8e48f4d143fb8d97af56ba"
	branch := "HEAD"
	goversion := runtime.Version()
	goos := runtime.GOOS
	goarch := runtime.GOARCH
	hostname := config.GetHostName()
	ch <- prometheus.MustNewConstMetric(
		c.buildInfoDesc,
		prometheus.GaugeValue,
		1,
		config.GetVersion(),
		hostname,
		revision,
		branch,
		goversion,
		goos,
		goarch,
	)
}
