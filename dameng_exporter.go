package main

import (
	"dameng_exporter/auth"
	"dameng_exporter/collector"
	"dameng_exporter/config"
	"dameng_exporter/db"
	"dameng_exporter/logger"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"

	"github.com/alecthomas/kingpin/v2"
	"github.com/duke-git/lancet/v2/fileutil"
	"github.com/prometheus/client_golang/prometheus"
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
	reg := prometheus.NewRegistry()

	//新建数据库连接
	// DSN (Data Source Name) format: user/password@host:port/service_name
	dsn := buildDSN(config.GlobalConfig.DbUser, config.GlobalConfig.DbPwd, config.GlobalConfig.DbHost)
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

	// 初始化数据库连接池
	err := db.InitDBPool(dsn)
	if err != nil {
		logger.Logger.Fatalf("Failed to initialize database pool: %v", zap.Error(err))
	}
	defer db.CloseDBPool() // 关闭数据库连接池

	//对配置文件密码进行加密,确认密码无误后在进行加密
	EncryptPasswordConfig(args.ConfigFile, args.EncodeConfigPwd)

	//注册指标
	collector.RegisterCollectors(reg)
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

// 对配置文件的密码进行加密
func EncryptPasswordConfig(configFile *string, encodeConfigPwd *bool) {
	//获取全路径
	configFilePath, err := filepath.Abs(*configFile)
	if err != nil {
		logger.Logger.Fatalf("Error determining absolute path for config file: %v", err)
	}
	// 修改密码以及配置文件
	if *encodeConfigPwd && config.GlobalConfig.DbPwd != "" && fileExists(configFilePath) {
		config.GlobalConfig.DbPwd = config.EncryptPassword(config.GlobalConfig.DbPwd)
		err = config.UpdateConfigPassword(configFilePath, config.GlobalConfig.DbPwd)
		if err != nil {
			logger.Logger.Fatalf("Error saving encoded password to config file: %v", err)
		}
		logger.Logger.Infof("Password in config file has been encoded")
	}
}

// mergeConfigParam 合并配置文件和命令行参数
func mergeConfigParam(args *config.CmdArgs) {
	//读取预先设定的配置文件
	glocal_config, err := config.LoadConfig(*args.ConfigFile)
	if err != nil {
		fmt.Printf("no loading default config file\n")
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

func buildDSN(user, password, host string) string {
	escapedPwd, _ := config.DecryptPassword(password)
	return fmt.Sprintf("dm://%s:%s@%s?autoCommit=true", user, url.PathEscape(escapedPwd), host)
}

func fileExists(filename string) bool {
	_, err := os.Stat(filename)
	if os.IsNotExist(err) {
		return false
	}
	return true
}
