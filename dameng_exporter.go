package main

import (
	"dameng_exporter/auth"
	"dameng_exporter/collector"
	"dameng_exporter/config"
	"dameng_exporter/db"
	"dameng_exporter/logger"
	"dameng_exporter/metrics"
	"fmt"
	"net/http"
	"os"
	"strconv"
	"strings"

	"github.com/BurntSushi/toml"
	"github.com/alecthomas/kingpin/v2"
	"github.com/duke-git/lancet/v2/fileutil"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"go.uber.org/zap"
)

// Version 定义版本号
const Version = "v1.1.6"

// parseFlags 解析命令行参数
func parseFlags() *config.CmdArgs {
	args := &config.CmdArgs{
		ConfigFile:              kingpin.Flag("configFile", "Path to configuration file").Default(config.DefaultConfig.ConfigFile).String(),
		ListenAddr:              kingpin.Flag("listenAddress", "Address to listen on").Default(config.DefaultConfig.ListenAddress).String(),
		MetricPath:              kingpin.Flag("metricPath", "Path for metrics").Default(config.DefaultConfig.MetricPath).String(),
		DbHost:                  kingpin.Flag("dbHost", "Database Host").Default(config.DefaultConfig.DbHost).String(),
		DbUser:                  kingpin.Flag("dbUser", "Database user").Default(config.DefaultConfig.DbUser).String(),
		DbPwd:                   kingpin.Flag("dbPwd", "Database password").Default(config.DefaultConfig.DbPwd).String(),
		QueryTimeout:            kingpin.Flag("queryTimeout", "Timeout for queries (Second)").Default(fmt.Sprint(config.DefaultConfig.QueryTimeout)).Int(),
		MaxOpenConns:            kingpin.Flag("maxOpenConns", "Maximum open connections (number)").Default(fmt.Sprint(config.DefaultConfig.MaxOpenConns)).Int(),
		MaxIdleConns:            kingpin.Flag("maxIdleConns", "Maximum idle connections (number)").Default(fmt.Sprint(config.DefaultConfig.MaxIdleConns)).Int(),
		ConnMaxLife:             kingpin.Flag("connMaxLifetime", "Connection maximum lifetime (Minute)").Default(fmt.Sprint(config.DefaultConfig.ConnMaxLifetime)).Int(),
		CheckSlowSQL:            kingpin.Flag("checkSlowSql", "Check slow SQL,default:"+strconv.FormatBool(config.DefaultConfig.CheckSlowSQL)).Default(strconv.FormatBool(config.DefaultConfig.CheckSlowSQL)).Bool(),
		SlowSqlTime:             kingpin.Flag("slowSqlTime", "Slow SQL time (Millisecond)").Default(fmt.Sprint(config.DefaultConfig.SlowSqlTime)).Int(),
		SlowSqlMaxRows:          kingpin.Flag("slowSqlLimitRows", "Slow SQL return limit row").Default(fmt.Sprint(config.DefaultConfig.SlowSqlMaxRows)).Int(),
		RegisterHostMetrics:     kingpin.Flag("registerHostMetrics", "Register host metrics,default:"+strconv.FormatBool(config.DefaultConfig.RegisterHostMetrics)).Default(strconv.FormatBool(config.DefaultConfig.RegisterHostMetrics)).Bool(),
		RegisterDatabaseMetrics: kingpin.Flag("registerDatabaseMetrics", "Register database metrics,default:"+strconv.FormatBool(config.DefaultConfig.RegisterDatabaseMetrics)).Default(strconv.FormatBool(config.DefaultConfig.RegisterDatabaseMetrics)).Bool(),
		RegisterDmhsMetrics:     kingpin.Flag("registerDmhsMetrics", "Register dmhs metrics,default:"+strconv.FormatBool(config.DefaultConfig.RegisterDmhsMetrics)).Default(strconv.FormatBool(config.DefaultConfig.RegisterDmhsMetrics)).Bool(),
		RegisterCustomMetrics:   kingpin.Flag("registerCustomMetrics", "Register custom metrics,default:"+strconv.FormatBool(config.DefaultConfig.RegisterCustomMetrics)).Default(strconv.FormatBool(config.DefaultConfig.RegisterCustomMetrics)).Bool(),
		BigKeyDataCacheTime:     kingpin.Flag("bigKeyDataCacheTime", "Big key data cache time (Minute)").Default(fmt.Sprint(config.DefaultConfig.BigKeyDataCacheTime)).Int(),
		AlarmKeyCacheTime:       kingpin.Flag("alarmKeyCacheTime", "Alarm key cache time (Minute)").Default(fmt.Sprint(config.DefaultConfig.AlarmKeyCacheTime)).Int(),
		LogMaxSize:              kingpin.Flag("logMaxSize", "Maximum log file size(MB)").Default(fmt.Sprint(config.DefaultConfig.LogMaxSize)).Int(),
		LogMaxBackups:           kingpin.Flag("logMaxBackups", "Maximum log file backups (number)").Default(fmt.Sprint(config.DefaultConfig.LogMaxBackups)).Int(),
		LogMaxAge:               kingpin.Flag("logMaxAge", "Maximum log file age (Day)").Default(fmt.Sprint(config.DefaultConfig.LogMaxAge)).Int(),
		LogLevel:                kingpin.Flag("logLevel", "Log level (debug|info|warn|error)").Default(config.DefaultConfig.LogLevel).String(),
		EncryptPwd:              kingpin.Flag("encryptPwd", "Password to encrypt and exit").Default("").String(),
		EncodeConfigPwd:         kingpin.Flag("encodeConfigPwd", "Encode the password in the config file,default:"+strconv.FormatBool(config.DefaultConfig.EncodeConfigPwd)).Default(strconv.FormatBool(config.DefaultConfig.EncodeConfigPwd)).Bool(),
		EnableBasicAuth:         kingpin.Flag("enableBasicAuth", "Enable basic auth for metrics endpoint,default:"+strconv.FormatBool(config.DefaultConfig.EnableBasicAuth)).Default(strconv.FormatBool(config.DefaultConfig.EnableBasicAuth)).Bool(),
		BasicAuthUsername:       kingpin.Flag("basicAuthUsername", "Username for basic auth").Default(config.DefaultConfig.BasicAuthUsername).String(),
		BasicAuthPassword:       kingpin.Flag("basicAuthPassword", "Password for basic auth").Default(config.DefaultConfig.BasicAuthPassword).String(),
		EncryptBasicAuthPwd:     kingpin.Flag("encryptBasicAuthPwd", "Password to encrypt for basic auth and exit").Default("").String(),

		// 全局超时控制参数
		GlobalTimeoutSeconds: kingpin.Flag("globalTimeoutSeconds", "Global timeout for metrics collection (seconds)").Default(fmt.Sprint(config.DefaultConfig.GlobalTimeoutSeconds)).Int(),
		P99LatencyTarget:     kingpin.Flag("p99LatencyTarget", "P99 latency target in seconds").Default(fmt.Sprintf("%.1f", config.DefaultConfig.P99LatencyTarget)).Float64(),
		EnablePartialReturn:  kingpin.Flag("enablePartialReturn", "Enable partial result return on timeout").Default(strconv.FormatBool(config.DefaultConfig.EnablePartialReturn)).Bool(),
		LatencyWindowSize:    kingpin.Flag("latencyWindowSize", "Sliding window size for P99 latency calculation").Default(fmt.Sprint(config.DefaultConfig.LatencyWindowSize)).Int(),
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
	// 设置版本号
	config.SetVersion(Version)
	config.GlobalConfig.Version = Version
	// eg:初始化全局日志记录器，必须合并完配置在初始化 不然日志控制参数会失效
	logger.InitLogger()
	defer logger.Sync()

	// 使用分类输出格式，每个类别一行
	logger.Logger.Infof("%s", config.GlobalConfig.StringCategorized())
	//项目开源地址
	logger.Logger.Infof("The open source address of the project: https://github.com/gaoyuan98/dameng_exporter")

	// 创建一个新的注册器，如果使用系统自带的,会多余出很多指标
	// 使用TimedRegistry以支持每次scrape的耗时统计
	reg := metrics.NewTimedRegistry()

	// 获取主机名
	hostname, host_err := os.Hostname()
	if host_err != nil {
		logger.Logger.Fatalf("Failed to get hostname", zap.Error(host_err))
	}
	config.SetHostName(hostname)

	//如果自定义参数开启，则读取配置文件
	if config.GlobalConfig.RegisterCustomMetrics && fileutil.IsExist(config.GlobalConfig.CustomMetricsFile) {

		// 解析配置文件
		custom_config, err := config.ParseCustomConfig(config.GlobalConfig.CustomMetricsFile)
		if err != nil {
			logger.Logger.Fatal(err)
		} else {
			logger.Logger.Infof("Parse custom config file size %v", len(custom_config.Metrics))
		}
	}

	// 初始化数据库连接池（统一使用多数据源架构）
	if config.GlobalMultiConfig == nil {
		logger.Logger.Fatalf("No multi-datasource config loaded, please check config file")
	}

	logger.Logger.Infof("Initializing with %d datasource(s)", len(config.GlobalMultiConfig.DataSources))
	poolManager := db.NewDBPoolManager(config.GlobalMultiConfig)
	err := poolManager.InitPools()
	if err != nil {
		logger.Logger.Fatalf("Failed to initialize datasource pools: %v", zap.Error(err))
	}
	defer poolManager.Close()

	// 设置全局DBPool为兼容模式（向后兼容）
	db.DBPool = poolManager.GetLegacyPool()

	//注册指标（统一使用多数据源架构）
	collector.RegisterCollectorsWithPoolManager(reg.Registry, poolManager)
	logger.Logger.Info("Starting dmdb_exporter version " + Version)
	logger.Logger.Info("Please visit: http://localhost" + config.GlobalConfig.ListenAddress + config.GlobalConfig.MetricPath)
	//设置metric路径
	http.Handle(config.GlobalConfig.MetricPath, auth.BasicAuthMiddleware(promhttp.HandlerFor(reg, promhttp.HandlerOpts{})))
	//配置引导页
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.Write(landingPage)
	})
	//设置端口号
	if err := http.ListenAndServe(config.GlobalConfig.ListenAddress, nil); err != nil {
		logger.Logger.Errorf("Error occur when start server %v", zap.Error(err))
	}

}

// mergeConfigParam 合并配置文件和命令行参数
func mergeConfigParam(args *config.CmdArgs) {
	var glocal_config config.Config

	// 加载TOML格式配置文件
	if fileutil.IsExist(*args.ConfigFile) {
		fmt.Printf("Loading TOML config file: %s\n", *args.ConfigFile)
		multiConfig, err := config.LoadMultiSourceConfig(*args.ConfigFile)
		if err != nil {
			fmt.Printf("Error loading TOML config file: %v\n", err)
			// 使用默认配置
			glocal_config = config.DefaultConfig
		} else {
			fmt.Printf("Successfully loaded TOML config with %d datasources\n", len(multiConfig.DataSources))

			// 检查是否需要加密密码
			if multiConfig.EncodeConfigPwd {
				// 读取原始配置文件内容以检查密码格式
				rawContent, err := os.ReadFile(*args.ConfigFile)
				if err == nil {
					rawConfig := &config.MultiSourceConfig{}
					if _, err := toml.Decode(string(rawContent), rawConfig); err == nil {
						needUpdate := false
						// 检查每个数据源的密码是否需要加密
						for i := range rawConfig.DataSources {
							// 如果密码不是以 ENC( 开头，说明需要加密
							if rawConfig.DataSources[i].DbPwd != "" &&
								!strings.HasPrefix(rawConfig.DataSources[i].DbPwd, "ENC(") {
								// 加密密码（EncryptPassword 返回 ENC(...) 格式）
								encPwd := config.EncryptPassword(rawConfig.DataSources[i].DbPwd)
								rawConfig.DataSources[i].DbPwd = encPwd
								needUpdate = true
								fmt.Printf("Encrypted password for datasource: %s\n", rawConfig.DataSources[i].Name)
							}
						}
						// 如果有密码被加密，更新配置文件
						if needUpdate {
							if err := config.SaveMultiSourceConfig(rawConfig, *args.ConfigFile); err != nil {
								fmt.Printf("Failed to update config file with encrypted passwords: %v\n", err)
							} else {
								fmt.Println("Config file updated with encrypted passwords successfully")
								// 重新加载配置以使用加密后的密码
								multiConfig, err = config.LoadMultiSourceConfig(*args.ConfigFile)
								if err != nil {
									fmt.Printf("Failed to reload config after encryption: %v\n", err)
								}
							}
						}
					}
				}
			}

			// 转换多数据源配置为全局配置（用于兼容）
			glocal_config = config.ConvertMultiToGlobal(multiConfig)
			// 保存多数据源配置
			config.GlobalMultiConfig = multiConfig
		}
	} else {
		// 配置文件不存在，报错退出
		fmt.Printf("Error: Config file not found: %s\n", *args.ConfigFile)
		fmt.Println("Please create a config file or specify an existing one with --configFile parameter")
		os.Exit(1)
	}

	// 对默认值以及配置文件的参数进行合并覆盖
	config.MergeConfig(&glocal_config, args)

	config.GlobalConfig = &glocal_config
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
