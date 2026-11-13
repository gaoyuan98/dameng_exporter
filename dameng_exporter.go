package main

import (
	"dameng_exporter/auth"
	"dameng_exporter/collector"
	"dameng_exporter/config"
	"dameng_exporter/db"
	"dameng_exporter/logger"
	"dameng_exporter/server"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"os"
	"strconv"

	"github.com/alecthomas/kingpin/v2"
	"github.com/prometheus/client_golang/prometheus"
	"go.uber.org/zap"
)

// Version 定义版本号
const Version = "v1.2.7_20261109"

// parseFlags 解析命令行参数
func parseFlags() *config.CmdArgs {
	args := &config.CmdArgs{
		ConfigFile:              kingpin.Flag("configFile", "Path to configuration file").Default("./dameng_exporter.toml").String(),
		ListenAddr:              kingpin.Flag("listenAddress", "Address to listen on").Default(config.DefaultMultiSourceConfig.ListenAddress).String(),
		MetricPath:              kingpin.Flag("metricPath", "Path for metrics").Default(config.DefaultMultiSourceConfig.MetricPath).String(),
		DbHost:                  kingpin.Flag("dbHost", "Database Host (when specified, requires dbUser and dbPwd)").String(),
		DbUser:                  kingpin.Flag("dbUser", "Database user (required with dbHost)").String(),
		DbPwd:                   kingpin.Flag("dbPwd", "Database password (required with dbHost)").String(),
		DbPwdMode:               kingpin.Flag("dbPwdMode", "Database password format: auto, plain or encrypted").Default("auto").Enum("auto", "plain", "encrypted"),
		DbName:                  kingpin.Flag("dbName", "Name for the database (optional, defaults to generated name)").String(),
		QueryTimeout:            kingpin.Flag("queryTimeout", "Timeout for queries (Second)").Default(fmt.Sprint(config.DefaultDataSourceConfig.QueryTimeout)).Int(),
		MaxOpenConns:            kingpin.Flag("maxOpenConns", "Maximum open connections (number)").Default(fmt.Sprint(config.DefaultDataSourceConfig.MaxOpenConns)).Int(),
		ConnMaxLife:             kingpin.Flag("connMaxLifetime", "Connection maximum lifetime (Minute)").Default(fmt.Sprint(config.DefaultDataSourceConfig.ConnMaxLifetime)).Int(),
		CheckSlowSQL:            kingpin.Flag("checkSlowSql", "Check slow SQL,default:"+strconv.FormatBool(config.DefaultDataSourceConfig.CheckSlowSQL)).Default(strconv.FormatBool(config.DefaultDataSourceConfig.CheckSlowSQL)).Bool(),
		SlowSqlTime:             kingpin.Flag("slowSqlTime", "Slow SQL time (Millisecond)").Default(fmt.Sprint(config.DefaultDataSourceConfig.SlowSqlTime)).Int(),
		SlowSqlMaxRows:          kingpin.Flag("slowSqlLimitRows", "Slow SQL return limit row").Default(fmt.Sprint(config.DefaultDataSourceConfig.SlowSqlMaxRows)).Int(),
		RegisterHostMetrics:     kingpin.Flag("registerHostMetrics", "Register host metrics,default:"+strconv.FormatBool(config.DefaultDataSourceConfig.RegisterHostMetrics)).Default(strconv.FormatBool(config.DefaultDataSourceConfig.RegisterHostMetrics)).Bool(),
		RegisterDatabaseMetrics: kingpin.Flag("registerDatabaseMetrics", "Register database metrics,default:"+strconv.FormatBool(config.DefaultDataSourceConfig.RegisterDatabaseMetrics)).Default(strconv.FormatBool(config.DefaultDataSourceConfig.RegisterDatabaseMetrics)).Bool(),
		RegisterDmhsMetrics:     kingpin.Flag("registerDmhsMetrics", "Register dmhs metrics,default:"+strconv.FormatBool(config.DefaultDataSourceConfig.RegisterDmhsMetrics)).Default(strconv.FormatBool(config.DefaultDataSourceConfig.RegisterDmhsMetrics)).Bool(),
		RegisterCustomMetrics:   kingpin.Flag("registerCustomMetrics", "Register custom metrics,default:"+strconv.FormatBool(config.DefaultDataSourceConfig.RegisterCustomMetrics)).Default(strconv.FormatBool(config.DefaultDataSourceConfig.RegisterCustomMetrics)).Bool(),
		BigKeyDataCacheTime:     kingpin.Flag("bigKeyDataCacheTime", "Big key data cache time (Minute)").Default(fmt.Sprint(config.DefaultDataSourceConfig.BigKeyDataCacheTime)).Int(),
		AlarmKeyCacheTime:       kingpin.Flag("alarmKeyCacheTime", "Alarm key cache time (Minute)").Default(fmt.Sprint(config.DefaultDataSourceConfig.AlarmKeyCacheTime)).Int(),
		LogMaxSize:              kingpin.Flag("logMaxSize", "Maximum log file size(MB)").Default(fmt.Sprint(config.DefaultMultiSourceConfig.LogMaxSize)).Int(),
		LogMaxBackups:           kingpin.Flag("logMaxBackups", "Maximum log file backups (number)").Default(fmt.Sprint(config.DefaultMultiSourceConfig.LogMaxBackups)).Int(),
		LogMaxAge:               kingpin.Flag("logMaxAge", "Maximum log file age (Day)").Default(fmt.Sprint(config.DefaultMultiSourceConfig.LogMaxAge)).Int(),
		LogLevel:                kingpin.Flag("logLevel", "Log level (debug|info|warn|error)").Default(config.DefaultMultiSourceConfig.LogLevel).String(),
		EncryptPwd:              kingpin.Flag("encryptPwd", "Password to encrypt and exit").Default("").String(),
		EncodeConfigPwd:         kingpin.Flag("encodeConfigPwd", "Encode the password in the config file,default:"+strconv.FormatBool(config.DefaultMultiSourceConfig.EncodeConfigPwd)).Default(strconv.FormatBool(config.DefaultMultiSourceConfig.EncodeConfigPwd)).Bool(),
		EnableBasicAuth:         kingpin.Flag("enableBasicAuth", "Enable basic auth for metrics endpoint,default:"+strconv.FormatBool(config.DefaultMultiSourceConfig.EnableBasicAuth)).Default(strconv.FormatBool(config.DefaultMultiSourceConfig.EnableBasicAuth)).Bool(),
		BasicAuthUsername:       kingpin.Flag("basicAuthUsername", "Username for basic auth").Default(config.DefaultMultiSourceConfig.BasicAuthUsername).String(),
		BasicAuthPassword:       kingpin.Flag("basicAuthPassword", "Password for basic auth").Default(config.DefaultMultiSourceConfig.BasicAuthPassword).String(),
		EncryptBasicAuthPwd:     kingpin.Flag("encryptBasicAuthPwd", "Password to encrypt for basic auth and exit").Default("").String(),

		// 全局超时控制参数
		GlobalTimeoutSeconds: kingpin.Flag("globalTimeoutSeconds", "Global timeout for metrics collection (seconds)").Default(fmt.Sprint(config.DefaultMultiSourceConfig.GlobalTimeoutSeconds)).Int(),

		// 采集模式参数
		CollectionMode: kingpin.Flag("collectionMode", "Collection mode: blocking (default) or fast").Default(config.DefaultMultiSourceConfig.CollectionMode).String(),

		// 健康检查参数
		EnableHealthPing: kingpin.Flag("enableHealthPing", "Enable periodic health ping for datasource pools").Default(strconv.FormatBool(config.DefaultMultiSourceConfig.EnableHealthPing)).Bool(),
	}
	kingpin.Parse()
	return args
}

func main() {
	landingPage := []byte("<html><head><title>DAMENG DB Exporter " + Version + "</title></head><body><h1>DAMENG DB Exporter " + Version + "</h1><p><a href='/metrics'>Metrics</a></p></body></html>")

	// 解析命令行参数
	args := parseFlags()
	//加密密码口令返回
	if execEncryptPwdCmd(args.EncryptPwd) {
		return
	}
	//加密basic auth密码并返回
	if auth.ExecEncryptBasicAuthPwdCmd(args.EncryptBasicAuthPwd) {
		return
	}
	//合并配置文件属性
	mergeConfigParam(args)
	// 设置版本号到全局变量（用于build info等）
	config.SetVersion(Version)
	// 确保全局配置已初始化
	if config.GlobalMultiConfig == nil {
		fmt.Fprintln(os.Stderr, "Error: Failed to load configuration")
		os.Exit(1)
	}
	// eg:初始化全局日志记录器，必须合并完配置在初始化 不然日志控制参数会失效
	logger.InitLogger()
	defer logger.Sync()

	logger.Logger.Infow("exporter_starting", "version", Version)
	logStartupConfig(config.GlobalMultiConfig)

	// 创建一个新的注册器，如果使用系统自带的,会多余出很多指标
	reg := prometheus.NewRegistry()

	// 初始化数据库连接池（统一使用多数据源架构）
	if config.GlobalMultiConfig == nil {
		logger.Logger.Fatalf("No multi-datasource config loaded, please check config file")
	}

	poolManager := db.NewDBPoolManager(config.GlobalMultiConfig)
	err := poolManager.InitPools()
	if err != nil {
		logger.Logger.Desugar().Fatal("datasource_init_failed", zap.Error(err))
	}
	defer poolManager.Close()

	//注册指标（统一使用多数据源架构）
	collector.RegisterMultiSourceCollectors(reg, poolManager)
	logger.Logger.Infow("server_listening", "address", config.Global.GetListenAddress(), "metric_path", config.Global.GetMetricPath(),
		"url", metricAccessURL(config.Global.GetListenAddress(), config.Global.GetMetricPath()))
	handler := server.NewHandler(config.Global.GetMetricPath(), reg, landingPage)
	//设置端口号
	if err := http.ListenAndServe(config.Global.GetListenAddress(), handler); err != nil {
		logger.Logger.Errorw("server_failed", "address", config.Global.GetListenAddress(), "error", err)
	}

}

// Wildcard bind addresses use localhost for the local access link.
func metricAccessURL(address, path string) string {
	if host, port, err := net.SplitHostPort(address); err == nil {
		if host == "" || host == "0.0.0.0" || host == "::" {
			host = "localhost"
		}
		address = net.JoinHostPort(host, port)
	}
	return (&url.URL{Scheme: "http", Host: address, Path: path}).String()
}

// logStartupConfig uses an explicit allowlist so credentials never enter logs.
func logStartupConfig(cfg *config.MultiSourceConfig) {
	enabled := 0
	for _, ds := range cfg.DataSources {
		if ds.Enabled {
			enabled++
		}
	}
	logger.Logger.Infow("config_loaded", "enabled_datasources", enabled, "collection_mode", cfg.CollectionMode, "threshold_ms", cfg.ScrapeSlowThresholdMs)
	logger.Logger.Debugw("config_details", "listen_address", cfg.ListenAddress, "metric_path", cfg.MetricPath,
		"log_level", cfg.LogLevel, "log_max_size_mb", cfg.LogMaxSize, "log_max_backups", cfg.LogMaxBackups, "log_max_age_days", cfg.LogMaxAge,
		"enable_basic_auth", cfg.EnableBasicAuth, "encode_config_pwd", cfg.EncodeConfigPwd, "global_timeout_seconds", cfg.GlobalTimeoutSeconds,
		"retry_interval_seconds", cfg.RetryIntervalSeconds, "enable_health_ping", cfg.IsHealthPingEnabled())
	for _, ds := range cfg.DataSources {
		logger.Logger.Debugw("datasource_config", "datasource", ds.Name, "host", ds.DbHost, "user", ds.DbUser, "enabled", ds.Enabled,
			"max_open_conns", ds.MaxOpenConns, "conn_max_lifetime_minutes", ds.ConnMaxLifetime, "query_timeout_seconds", ds.QueryTimeout,
			"big_key_cache_minutes", ds.BigKeyDataCacheTime, "alarm_cache_minutes", ds.AlarmKeyCacheTime, "check_slow_sql", ds.CheckSlowSQL,
			"slow_sql_time_ms", ds.SlowSqlTime, "slow_sql_max_rows", ds.SlowSqlMaxRows, "host_metrics", ds.RegisterHostMetrics,
			"database_metrics", ds.RegisterDatabaseMetrics, "dmhs_metrics", ds.RegisterDmhsMetrics, "custom_metrics", ds.RegisterCustomMetrics,
			"labels", ds.Labels, "custom_metrics_file", ds.CustomMetricsFile)
	}
}

// mergeConfigParam 合并配置文件和命令行参数
func mergeConfigParam(args *config.CmdArgs) {
	// 加载TOML格式配置文件
	if _, err := os.Stat(*args.ConfigFile); os.IsNotExist(err) {
		// 配置文件不存在，报错退出
		fmt.Fprintf(os.Stderr, "Error: Config file not found: %s\n", *args.ConfigFile)
		fmt.Fprintln(os.Stderr, "Please create a config file or specify an existing one with --configFile parameter")
		os.Exit(1)
	}

	multiConfig, err := config.LoadMultiSourceConfig(*args.ConfigFile)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error loading TOML config file: %v\n", err)
		os.Exit(1)
	}

	// 检查并加密配置文件中的密码
	updatedConfig, err := config.CheckAndEncryptConfigPasswords(multiConfig, *args.ConfigFile)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: Failed to check/encrypt passwords: %v\n", err)
		os.Exit(1)
	} else {
		multiConfig = updatedConfig
	}

	// 设置版本号到配置中
	multiConfig.Version = Version

	// 合并命令行参数到配置
	if err := config.MergeMultiSourceConfigFromCmdArgs(multiConfig, args); err != nil {
		fmt.Fprintf(os.Stderr, "Error: Invalid configuration: %v\n", err)
		os.Exit(1)
	}

	// 保存多数据源配置
	config.GlobalMultiConfig = multiConfig
	// 初始化全局配置访问器
	config.Global.Init(multiConfig)
}

func execEncryptPwdCmd(encryptPwd *string) bool {
	//命令行参数，对密码加密并返回结果
	if *encryptPwd != "" {
		encryptedPwd := config.EncryptPassword(*encryptPwd)
		fmt.Printf("Encrypted Password: %s\n", encryptedPwd)
		return true
	}
	return false
}
