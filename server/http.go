package server

import (
	"dameng_exporter/auth"
	"net/http"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// NewHandler 使用独立路由，避免暴露依赖在 DefaultServeMux 中注册的调试接口。
func NewHandler(metricPath string, reg prometheus.Gatherer, landingPage []byte) http.Handler {
	mux := http.NewServeMux()
	mux.Handle(metricPath, auth.BasicAuthMiddleware(promhttp.HandlerFor(reg, promhttp.HandlerOpts{})))
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" {
			http.NotFound(w, r)
			return
		}
		w.Write(landingPage)
	})
	return mux
}
