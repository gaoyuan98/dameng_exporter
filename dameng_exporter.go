package main

import (
	"dameng_exporter/collector"
	"dameng_exporter/config"
	"dameng_exporter/db"
	"dameng_exporter/logger"
	"fmt"
	"github.com/alecthomas/kingpin/v2"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"go.uber.org/zap"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"time"
)

var (
	registerHostMetrics     bool = true //主机类的数据库指标 例如进程是否存活
	registerDatabaseMetrics bool = true
	registerDmhsMetrics     bool = true //DMHS的相关指标
)

/*if registerHostMetrics || registerDatabaseMetrics || registerMiddlewareMetrics {
// 注册自定义指标
collector.RegisterCollectors(registerHostMetrics, registerDatabaseMetrics, registerMiddlewareMetrics)
} else {
// 卸载所有注册器
collector.UnregisterCollectors()
}
*/

func main() {
	logger.InitLogger() // 初始化全局日志记录器
	defer logger.Sync() // 确保程序退出前同步日志

	var (
		configFile      = kingpin.Flag("configFile", "Path to configuration file").Default(config.DefaultConfig.ConfigFile).String()
		listenAddr      = kingpin.Flag("listenAddress", "Address to listen on").Default(config.DefaultConfig.ListenAddress).String()
		metricPath      = kingpin.Flag("metricPath", "Path for metrics").Default(config.DefaultConfig.MetricPath).String()
		dbHost          = kingpin.Flag("dbHost", "Database Host").Default(config.DefaultConfig.DbHost).String()
		dbUser          = kingpin.Flag("dbUser", "Database user").Default(config.DefaultConfig.DbUser).String()
		dbPwd           = kingpin.Flag("dbPwd", "Database password").Default(config.DefaultConfig.DbPwd).String()
		queryTimeout    = kingpin.Flag("queryTimeout", "Timeout for queries").Default(config.DefaultConfig.QueryTimeout.String()).Duration()
		maxIdleConns    = kingpin.Flag("maxIdleConns", "Maximum idle connections").Default(fmt.Sprint(config.DefaultConfig.MaxIdleConns)).Int()
		maxOpenConns    = kingpin.Flag("maxOpenConns", "Maximum open connections").Default(fmt.Sprint(config.DefaultConfig.MaxOpenConns)).Int()
		connMaxLife     = kingpin.Flag("connMaxLifetime", "Connection maximum lifetime").Default(config.DefaultConfig.ConnMaxLifetime.String()).Duration()
		logMaxSize      = kingpin.Flag("logMaxSize", "Maximum log file size").Default(fmt.Sprint(config.DefaultConfig.LogMaxSize)).Int()
		logMaxBackups   = kingpin.Flag("logMaxBackups", "Maximum log file backups").Default(fmt.Sprint(config.DefaultConfig.LogMaxBackups)).Int()
		logMaxAge       = kingpin.Flag("logMaxAge", "Maximum log file age").Default(fmt.Sprint(config.DefaultConfig.LogMaxAge)).Int()
		encryptPwd      = kingpin.Flag("encryptPwd", "Password to encrypt and exit").Default("").String()
		encodeConfigPwd = kingpin.Flag("encodeConfigPwd", "Encode the password in the config file").Default("true").Bool()
		Version         = "0.0.1"
	)
	kingpin.Parse()
	//加密密码口令返回
	if execEncryptPwdCmd(encryptPwd) {
		return
	}

	//读取预先设定的配置文件
	glocal_config, err := config.LoadConfig(*configFile)
	if err != nil {
		log.Printf("Error loading config file: %v", err)
	}

	// 对默认值以及配置文件的参数进行合并覆盖
	applyConfigFromFlags(&glocal_config, listenAddr, metricPath, queryTimeout, maxIdleConns, maxOpenConns, connMaxLife, logMaxSize, logMaxBackups, logMaxAge, dbUser, dbPwd, dbHost)
	//获取全路径
	configFilePath, err := filepath.Abs(*configFile)
	if err != nil {
		logger.Logger.Fatalf("Error determining absolute path for config file: %v", err)
	}
	// 修改密码以及配置文件
	if *encodeConfigPwd && glocal_config.DbPwd != "" && fileExists(configFilePath) {
		glocal_config.DbPwd = config.EncryptPassword(glocal_config.DbPwd)
		err = config.UpdateConfigPassword(configFilePath, glocal_config.DbPwd)
		if err != nil {
			logger.Logger.Fatalf("Error saving encoded password to config file: %v", err)
		}
		logger.Logger.Infof("Password in config file has been encoded")
		//return
	}

	// 创建一个新的注册器
	reg := prometheus.NewRegistry()

	//新建数据库连接
	// DSN (Data Source Name) format: user/password@host:port/service_name
	dsn := buildDSN(glocal_config.DbUser, glocal_config.DbPwd, glocal_config.DbHost)
	// 获取主机名
	hostname, host_err := os.Hostname()
	if host_err != nil {
		logger.Logger.Fatalf("Failed to get hostname", zap.Error(host_err))
	}
	config.SetHostName(hostname)

	// 初始化数据库连接池
	err = db.InitDBPool(dsn)
	if err != nil {
		logger.Logger.Fatalf("Failed to initialize database pool: %v", zap.Error(err))
	}
	defer db.CloseDBPool() // 关闭数据库连接池

	//注册指标
	collector.RegisterCollectors(reg, registerHostMetrics, registerDatabaseMetrics, registerDmhsMetrics)
	logger.Logger.Info("Starting dmdb_exporter " + Version)
	//设置metric路径
	http.Handle(glocal_config.MetricPath, promhttp.HandlerFor(reg, promhttp.HandlerOpts{}))
	//设置端口号
	if err := http.ListenAndServe(glocal_config.ListenAddress, nil); err != nil {
		logger.Logger.Errorf("Error occur when start server %v", zap.Error(err))
	}

}

func execEncryptPwdCmd(encryptPwd *string) bool {
	//命令行参数，对密码加密并返回结果
	if *encryptPwd != "" {
		encryptedPwd := config.EncryptPassword(*encryptPwd)
		logger.Logger.Info("Encrypted Password: %s\n", encryptedPwd)
		return true
	}
	return false
}

func buildDSN(user, password, host string) string {
	//dsn := "dm://SYSDBA:SYSDBA@120.53.103.235:5236?autoCommit=true"
	escapedPwd, _ := config.DecryptPassword(password)
	return fmt.Sprintf("dm://%s:%s@%s?autoCommit=true", user, escapedPwd, host)
}

func fileExists(filename string) bool {
	_, err := os.Stat(filename)
	if os.IsNotExist(err) {
		return false /*, nil*/
	}
	return true /*, err*/
}

func applyConfigFromFlags(glocal_config *config.Config, listenAddr, metricPath *string, queryTimeout *time.Duration, maxIdleConns, maxOpenConns *int, connMaxLife *time.Duration, logMaxSize, logMaxBackups, logMaxAge *int, dbUser, dbPwd, dbHost *string) {
	if listenAddr != nil && *listenAddr != config.DefaultConfig.ListenAddress {
		glocal_config.ListenAddress = *listenAddr
	}
	if metricPath != nil && *metricPath != config.DefaultConfig.MetricPath {
		glocal_config.MetricPath = *metricPath
	}
	if queryTimeout != nil && *queryTimeout != config.DefaultConfig.QueryTimeout {
		glocal_config.QueryTimeout = *queryTimeout
	}
	if maxIdleConns != nil && *maxIdleConns != config.DefaultConfig.MaxIdleConns {
		glocal_config.MaxIdleConns = *maxIdleConns
	}
	if maxOpenConns != nil && *maxOpenConns != config.DefaultConfig.MaxOpenConns {
		glocal_config.MaxOpenConns = *maxOpenConns
	}
	if connMaxLife != nil && *connMaxLife != config.DefaultConfig.ConnMaxLifetime {
		glocal_config.ConnMaxLifetime = *connMaxLife
	}
	if logMaxSize != nil && *logMaxSize != config.DefaultConfig.LogMaxSize {
		glocal_config.LogMaxSize = *logMaxSize
	}
	if logMaxBackups != nil && *logMaxBackups != config.DefaultConfig.LogMaxBackups {
		glocal_config.LogMaxBackups = *logMaxBackups
	}
	if logMaxAge != nil && *logMaxAge != config.DefaultConfig.LogMaxAge {
		glocal_config.LogMaxAge = *logMaxAge
	}
	if dbUser != nil && *dbUser != config.DefaultConfig.DbUser {
		glocal_config.DbUser = *dbUser
	}
	if dbPwd != nil && *dbPwd != config.DefaultConfig.DbPwd {
		glocal_config.DbPwd = *dbPwd
	}
	if dbHost != nil && *dbHost != config.DefaultConfig.DbHost {
		glocal_config.DbHost = *dbHost
	}

}
