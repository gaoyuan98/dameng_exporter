package db

import (
	"dameng_exporter/config"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"testing"

	"github.com/gaoyuan98/dm"
	"go.uber.org/zap"
)

func TestBuildDSNMarksExporterAndPreservesParameters(t *testing.T) {
	for _, tt := range []struct {
		name    string
		host    string
		timeout int
		want    string
	}{
		{"defaults", "localhost:5236", 12, "localhost:5236?autoCommit=true&socketTimeout=12&connectTimeout=12000&appName=dameng_exporter"},
		{"fallback timeout", "localhost:5236", 0, "localhost:5236?autoCommit=true&socketTimeout=5&connectTimeout=5000&appName=dameng_exporter"},
		{"raw values", "localhost:5236?schema=业务&path=a+b%2Fc&autoCommit=false&socketTimeout=9&connectTimeout=7000", 12, "localhost:5236?schema=业务&path=a+b%2Fc&autoCommit=false&socketTimeout=9&connectTimeout=7000&appName=dameng_exporter"},
		{"duplicate application names", "localhost:5236?APPNAME=old&appName=another&ApPnAmE=&socketTimeout=8", 12, "localhost:5236?socketTimeout=8&appName=dameng_exporter"},
		{"only application name", "localhost:5236?appName=old", 12, "localhost:5236?appName=dameng_exporter"},
		{"empty query", "localhost:5236?", 12, "localhost:5236?appName=dameng_exporter"},
		{"ipv6", "[::1]:5236?autoCommit=true", 12, "[::1]:5236?autoCommit=true&appName=dameng_exporter"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			cfg := config.DataSourceConfig{DbHost: tt.host, DbUser: "monitor", DbPwd: "test-password", QueryTimeout: tt.timeout}
			got := (&DBPoolManager{}).buildDSN(&cfg)
			if want := "dm://monitor:test-password@" + tt.want; got != want {
				t.Fatalf("DSN = %q, want %q", got, want)
			}
			// Match the driver's case-insensitive, raw parameter parsing.
			_, query, _ := strings.Cut(got, "?")
			appNames := 0
			for _, param := range strings.Split(query, "&") {
				key, value, _ := strings.Cut(param, "=")
				if strings.EqualFold(key, "appName") {
					appNames++
					if value != config.ExporterAppName {
						t.Fatalf("connection marker %q differs from session filter %q", value, config.ExporterAppName)
					}
				}
			}
			if appNames != 1 {
				t.Fatalf("expected exactly one appName parameter, got %d", appNames)
			}
		})
	}
}

func TestBuildDSNPreservesCredentialsInDriver(t *testing.T) {
	for _, credentials := range []struct {
		name     string
		user     string
		password string
	}{
		{"plain", "monitor", "test-password"},
		{"reported password", "monitor", "test_f@2023!"},
		{"at sign", "monitor", "test@2023"},
		{"exclamation mark", "monitor", "test!2023"},
		{"plus and spaces", "monitor", " test+ 2023 "},
		{"literal percent escapes", "monitor", "test%40%21%2B%20%"},
		{"unicode", "monitor", "测试密码@2023!"},
		{"delimiters", "monitor", "test:@/?#&=+%!/"},
		{"trailing question mark", "monitor", "test?"},
		{"parameter-like password", "monitor", "test?user=other&password=wrong&appName=other"},
		{"special username", "监控@+!%/?#&=", "test_f@2023!"},
	} {
		for _, host := range []struct {
			name           string
			address        string
			wantHost       string
			connectTimeout int64
			socketTimeout  int64
		}{
			{"defaults", "127.0.0.1:5236", "127.0.0.1", 12000, 12},
			{"custom parameters", "[::1]:5236?schema=业务&appName=old&connectTimeout=7000&socketTimeout=9", "[::1]", 7000, 9},
		} {
			t.Run(credentials.name+"/"+host.name, func(t *testing.T) {
				cfg := config.DataSourceConfig{
					DbHost:       host.address,
					DbUser:       credentials.user,
					DbPwd:        credentials.password,
					QueryTimeout: 12,
				}
				dsn := (&DBPoolManager{}).buildDSN(&cfg)
				connector, err := (&dm.DmDriver{}).OpenConnector(dsn)
				if err != nil {
					t.Fatal(err)
				}
				// Inspect the real driver's parsed fields without opening a database connection.
				// A future driver change must preserve the same credentials and parameters.
				parsed := reflect.ValueOf(connector).Elem()
				for field, want := range map[string]string{
					"user":     credentials.user,
					"password": credentials.password,
					"host":     host.wantHost,
					"appName":  config.ExporterAppName,
				} {
					if got := parsed.FieldByName(field).String(); got != want {
						t.Errorf("driver %s = %q, want %q", field, got, want)
					}
				}
				for field, want := range map[string]int64{
					"port":           5236,
					"connectTimeout": host.connectTimeout,
					"socketTimeout":  host.socketTimeout,
				} {
					if got := parsed.FieldByName(field).Int(); got != want {
						t.Errorf("driver %s = %d, want %d", field, got, want)
					}
				}
			})
		}
	}
}

func TestBuildDSNPreservesPrintablePasswordPairsInDriver(t *testing.T) {
	// Cover every pair of printable ASCII characters, including quotes,
	// backslashes and combinations of DSN delimiters.
	for first := rune(' '); first <= '~'; first++ {
		for second := rune(' '); second <= '~'; second++ {
			password := string([]rune{first, second})
			cfg := config.DataSourceConfig{
				DbHost: "127.0.0.1:5236",
				DbUser: "monitor",
				DbPwd:  password,
			}
			dsn := (&DBPoolManager{}).buildDSN(&cfg)
			connector, err := (&dm.DmDriver{}).OpenConnector(dsn)
			if err != nil {
				t.Fatalf("password %q: %v", password, err)
			}
			if got := reflect.ValueOf(connector).Elem().FieldByName("password").String(); got != password {
				t.Fatalf("driver password = %q, want %q", got, password)
			}
		}
	}
}

func TestCreatePoolRejectsAmbiguousCredentialsBeforeConnecting(t *testing.T) {
	for _, cfg := range []config.DataSourceConfig{
		{DbHost: "127.0.0.1:1", DbUser: "monitor:ops", DbPwd: "test-password"},
		{DbHost: "127.0.0.1:1?password=override", DbUser: "monitor", DbPwd: "test-password"},
		{DbHost: "127.0.0.1:1", DbUser: "monitor", DbPwd: strings.Repeat("a", 129)},
	} {
		pool, err := (&DBPoolManager{}).createPool(&cfg)
		if err == nil || pool != nil || strings.Contains(err.Error(), "连接失败") {
			t.Fatalf("expected configuration rejection before connecting, got pool=%v, err=%v", pool, err)
		}
	}
}

func TestInitPoolsIsolatesConnectionValidationFailures(t *testing.T) {
	cfg := config.DefaultMultiSourceConfig
	cfg.DataSources = []config.DataSourceConfig{
		{Name: "disabled", Enabled: false, DbHost: "127.0.0.1:1", DbUser: "monitor", DbPwd: strings.Repeat("a", 129)},
		{Name: "too-long", Enabled: true, DbHost: "127.0.0.2:1", DbUser: "monitor", DbPwd: strings.Repeat("a", 129)},
		{Name: "colon-user", Enabled: true, DbHost: "127.0.0.3:1", DbUser: "monitor:ops", DbPwd: "test-password"},
	}
	manager := NewDBPoolManager(&cfg)
	manager.logger = zap.NewNop().Sugar()
	previousManager := GlobalPoolManager
	t.Cleanup(func() {
		manager.Close()
		GlobalPoolManager = previousManager
	})
	// All enabled sources fail before dialing; each failure must be recorded independently.
	if err := manager.InitPools(); err != nil {
		t.Fatalf("one source's connection constraint aborted initialization: %v", err)
	}
	if manager.GetDatasourceHealthStatus("disabled").Registered {
		t.Fatal("disabled source was initialized")
	}
	for name, wantError := range map[string]string{"too-long": "128", "colon-user": "冒号"} {
		status := manager.GetDatasourceHealthStatus(name)
		if !status.Registered || status.Healthy || !strings.Contains(status.LastError, wantError) {
			t.Fatalf("source %s did not retain its independent validation error: %+v", name, status)
		}
	}
}

func TestConnectionErrorExplainsDriverCredentialLimit(t *testing.T) {
	for _, driverError := range []*dm.DmError{dm.ECGO_PASSWORD_TOO_LONG, dm.ECGO_USERNAME_TOO_LONG} {
		err := explainConnectionError(fmt.Errorf("wrapped driver error: %w", driverError))
		if !strings.Contains(err.Error(), "128 字节") || !errors.Is(err, driverError) {
			t.Fatalf("expected actionable error preserving its cause, got %v", err)
		}
	}
	other := errors.New("unrelated connection error")
	if got := explainConnectionError(other); got != other {
		t.Fatalf("unexpected change to unrelated error: %v", got)
	}
}
