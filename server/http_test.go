package server

import (
	"dameng_exporter/config"
	"dameng_exporter/logger"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	"go.uber.org/zap"
)

func TestHTTPHandlerRoutes(t *testing.T) {
	const landingPage = "<html><body>DAMENG DB Exporter</body></html>"
	previousConfig := config.Global.GetConfig()
	previousLogger := logger.Logger
	t.Cleanup(func() {
		config.Global.Init(previousConfig)
		logger.Logger = previousLogger
	})
	logger.Logger = zap.NewNop().Sugar()

	reg := prometheus.NewRegistry()
	metric := prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "test_exporter_value",
		Help: "Test metric for HTTP routing.",
	})
	metric.Set(42)
	reg.MustRegister(metric)

	for _, metricPath := range []string{"/metrics", "/custom/metrics", "/custom/metrics/"} {
		t.Run(metricPath, func(t *testing.T) {
			for _, enableAuth := range []bool{false, true} {
				name := "without_auth"
				if enableAuth {
					name = "with_auth"
				}
				t.Run(name, func(t *testing.T) {
					cfg := config.DefaultMultiSourceConfig
					cfg.EnableBasicAuth = enableAuth
					cfg.BasicAuthUsername = "monitor"
					cfg.BasicAuthPassword = "test-password"
					config.Global.Init(&cfg)
					handler := NewHandler(metricPath, reg, []byte(landingPage))

					for _, credentials := range []string{"none", "invalid", "valid"} {
						t.Run(credentials, func(t *testing.T) {
							req := httptest.NewRequest(http.MethodGet, metricPath, nil)
							if credentials == "valid" {
								req.SetBasicAuth("monitor", "test-password")
							} else if credentials == "invalid" {
								req.SetBasicAuth("monitor", "wrong-password")
							}
							rec := httptest.NewRecorder()
							handler.ServeHTTP(rec, req)
							wantStatus := http.StatusOK
							if enableAuth && credentials != "valid" {
								wantStatus = http.StatusUnauthorized
							}
							if rec.Code != wantStatus {
								t.Fatalf("metrics status = %d, want %d", rec.Code, wantStatus)
							}
							if wantStatus == http.StatusOK {
								if !strings.Contains(rec.Body.String(), "test_exporter_value 42\n") {
									t.Fatalf("metrics response missing registered metric: %s", rec.Body.String())
								}
							} else {
								if rec.Header().Get("WWW-Authenticate") == "" {
									t.Error("unauthorized response missing authentication challenge")
								}
								if strings.Contains(rec.Body.String(), "test_exporter_value") {
									t.Error("unauthorized response exposed metrics")
								}
							}
						})
					}

					rec := httptest.NewRecorder()
					handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))
					if rec.Code != http.StatusOK || rec.Body.String() != landingPage {
						t.Fatalf("unexpected landing page: status %d, body %s", rec.Code, rec.Body.String())
					}

					paths := []string{"/debug/vars", "/debug/vars?scan=1", "/debug/vars/", "/debug/pprof/", "/missing"}
					// 带尾斜杠的指标路径保留 ServeMux 原有的子路径匹配行为。
					if !strings.HasSuffix(metricPath, "/") {
						paths = append(paths, metricPath+"missing")
					}
					if metricPath != "/metrics" {
						paths = append(paths, "/metrics")
					}
					for _, path := range paths {
						rec := httptest.NewRecorder()
						handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))
						if rec.Code != http.StatusNotFound || rec.Body.String() != "404 page not found\n" {
							t.Errorf("GET %s: got status %d, body %q; want plain 404", path, rec.Code, rec.Body.String())
						}
					}
				})
			}
		})
	}
}

func TestHTTPHandlerIsolatesExpvar(t *testing.T) {
	// Prometheus 间接导入 expvar，默认路由仍会注册真实的信息泄露接口。
	defaultResponse := httptest.NewRecorder()
	http.DefaultServeMux.ServeHTTP(defaultResponse, httptest.NewRequest(http.MethodGet, "/debug/vars", nil))
	var vars map[string]json.RawMessage
	if err := json.Unmarshal(defaultResponse.Body.Bytes(), &vars); err != nil {
		t.Fatalf("expected expvar JSON on default mux: %v", err)
	}
	if defaultResponse.Code != http.StatusOK || vars["cmdline"] == nil || vars["memstats"] == nil {
		t.Fatal("expected cmdline and memstats on default mux to reproduce the vulnerability")
	}

	handler := NewHandler("/metrics", prometheus.NewRegistry(), []byte("landing page"))
	for _, method := range []string{http.MethodGet, http.MethodHead, http.MethodPost} {
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, httptest.NewRequest(method, "/debug/vars", nil))
		if rec.Code != http.StatusNotFound {
			t.Errorf("%s /debug/vars: status = %d, want 404", method, rec.Code)
		}
		if strings.Contains(rec.Body.String(), "cmdline") || strings.Contains(rec.Body.String(), "memstats") {
			t.Errorf("%s /debug/vars exposed runtime information", method)
		}
	}
}
