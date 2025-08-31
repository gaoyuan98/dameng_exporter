package config

import (
	"bufio"
	"fmt"
	"os"
	"strconv"
	"strings"
)

var hostName string
var version string

func SetHostName(hn string) {
	hostName = hn
}

func GetHostName() string {
	return hostName
}

func SetVersion(v string) {
	version = v
}

func GetVersion() string {
	return version
}

var GlobalConfig *Config

// GlobalMultiConfig 全局多数据源配置实例（用于多数据源模式）
var GlobalMultiConfig *MultiSourceConfig

type Config struct {
	ConfigFile        string
	CustomMetricsFile string
	ListenAddress     string
	MetricPath        string
	Version           string

	//QueryTimeout  time.Duration
	QueryTimeout int
	MaxIdleConns int
	MaxOpenConns int
	//ConnMaxLifetime time.Duration
	ConnMaxLifetime int
	LogMaxSize      int
	LogMaxBackups   int
	LogMaxAge       int
	LogLevel        string
	DbHost          string
	DbUser          string
	DbPwd           string

	//大key的保留时间
	BigKeyDataCacheTime int
	AlarmKeyCacheTime   int

	//指标是否注册选项
	RegisterHostMetrics     bool
	RegisterDatabaseMetrics bool
	RegisterDmhsMetrics     bool
	RegisterCustomMetrics   bool
	EncodeConfigPwd         bool
	CheckSlowSQL            bool
	SlowSqlTime             int
	SlowSqlMaxRows          int

	// Basic Auth配置
	EnableBasicAuth   bool
	BasicAuthUsername string
	BasicAuthPassword string
}

var DefaultConfig = Config{
	ConfigFile:        "./dameng_exporter.config",
	CustomMetricsFile: "./custom_metrics.toml",
	ListenAddress:     ":9200",
	MetricPath:        "/metrics",
	Version:           "v1.1.6",
	//QueryTimeout:    30 * time.Second,
	QueryTimeout:            30, //秒
	MaxIdleConns:            1,  //个数
	MaxOpenConns:            10, //个数
	ConnMaxLifetime:         30, //分钟
	LogMaxSize:              10, //MB
	LogMaxBackups:           3,  //个数
	LogMaxAge:               30, //天
	LogLevel:                "debug",
	BigKeyDataCacheTime:     60, //分
	AlarmKeyCacheTime:       5,  //分
	RegisterHostMetrics:     false,
	RegisterDatabaseMetrics: true,
	RegisterDmhsMetrics:     false,
	RegisterCustomMetrics:   true,
	EncodeConfigPwd:         true,
	DbUser:                  "SYSDBA",
	DbPwd:                   "SYSDBA",
	DbHost:                  "127.0.0.1:5236",
	CheckSlowSQL:            false,
	SlowSqlTime:             10000,
	SlowSqlMaxRows:          10,
	EnableBasicAuth:         false,
	BasicAuthUsername:       "",
	BasicAuthPassword:       "",
}

func LoadConfig(filePath string) (Config, error) {
	config := DefaultConfig
	file, err := os.Open(filePath)
	if err != nil {
		return config, err
	}
	defer file.Close()

	// 定义配置项解析器（所有key使用小写，实现大小写不敏感）
	parsers := map[string]func(string){
		// 字符串类型配置
		"configfile":        func(v string) { config.ConfigFile = v },
		"custommetricsfile": func(v string) { config.CustomMetricsFile = v },
		"listenaddress":     func(v string) { config.ListenAddress = v },
		"metricpath":        func(v string) { config.MetricPath = v },
		"loglevel":          func(v string) { config.LogLevel = v },
		"dbuser":            func(v string) { config.DbUser = v },
		"dbpwd": func(v string) {
			// 如果密码是加密的，自动解密
			if decryptedPwd, err := DecryptPassword(v); err == nil {
				config.DbPwd = decryptedPwd
			} else {
				config.DbPwd = v
			}
		},
		"dbhost":            func(v string) { config.DbHost = v },
		"basicauthusername": func(v string) { config.BasicAuthUsername = v },
		"basicauthpassword": func(v string) {
			// 如果Basic Auth密码是加密的，自动解密
			if decryptedPwd, err := DecryptPassword(v); err == nil {
				config.BasicAuthPassword = decryptedPwd
			} else {
				config.BasicAuthPassword = v
			}
		},

		// 整数类型配置
		"querytimeout": func(v string) {
			if val, err := strconv.Atoi(v); err == nil {
				config.QueryTimeout = val
			}
		},
		"maxidleconns": func(v string) {
			if val, err := strconv.Atoi(v); err == nil {
				config.MaxIdleConns = val
			}
		},
		"maxopenconns": func(v string) {
			if val, err := strconv.Atoi(v); err == nil {
				config.MaxOpenConns = val
			}
		},
		"connmaxlifetime": func(v string) {
			if val, err := strconv.Atoi(v); err == nil {
				config.ConnMaxLifetime = val
			}
		},
		"logmaxsize": func(v string) {
			if val, err := strconv.Atoi(v); err == nil {
				config.LogMaxSize = val
			}
		},
		"logmaxbackups": func(v string) {
			if val, err := strconv.Atoi(v); err == nil {
				config.LogMaxBackups = val
			}
		},
		"logmaxage": func(v string) {
			if val, err := strconv.Atoi(v); err == nil {
				config.LogMaxAge = val
			}
		},
		"bigkeydatacachetime": func(v string) {
			if val, err := strconv.Atoi(v); err == nil {
				config.BigKeyDataCacheTime = val
			}
		},
		"alarmkeycachetime": func(v string) {
			if val, err := strconv.Atoi(v); err == nil {
				config.AlarmKeyCacheTime = val
			}
		},
		"slowsqltime": func(v string) {
			if val, err := strconv.Atoi(v); err == nil {
				config.SlowSqlTime = val
			}
		},
		"slowsqllimitrows": func(v string) {
			if val, err := strconv.Atoi(v); err == nil {
				config.SlowSqlMaxRows = val
			}
		},

		// 布尔类型配置
		"registerhostmetrics": func(v string) {
			if val, err := strconv.ParseBool(v); err == nil {
				config.RegisterHostMetrics = val
			}
		},
		"registerdatabasemetrics": func(v string) {
			if val, err := strconv.ParseBool(v); err == nil {
				config.RegisterDatabaseMetrics = val
			}
		},
		"registerdmhsmetrics": func(v string) {
			if val, err := strconv.ParseBool(v); err == nil {
				config.RegisterDmhsMetrics = val
			}
		},
		"registercustommetrics": func(v string) {
			if val, err := strconv.ParseBool(v); err == nil {
				config.RegisterCustomMetrics = val
			}
		},
		"encodeconfigpwd": func(v string) {
			if val, err := strconv.ParseBool(v); err == nil {
				config.EncodeConfigPwd = val
			}
		},
		"checkslowsql": func(v string) {
			if val, err := strconv.ParseBool(v); err == nil {
				config.CheckSlowSQL = val
			}
		},
		"enablebasicauth": func(v string) {
			if val, err := strconv.ParseBool(v); err == nil {
				config.EnableBasicAuth = val
			}
		},
	}

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		parts := strings.SplitN(line, "=", 2)
		if len(parts) != 2 {
			continue
		}
		// 将key转换为小写，实现大小写不敏感
		key := strings.ToLower(strings.TrimSpace(parts[0]))
		value := strings.TrimSpace(parts[1])

		// 使用解析器map处理配置项
		if parser, exists := parsers[key]; exists {
			parser(value)
		}
	}

	return config, scanner.Err()
}

// StringCategorized 按类别输出，每个类别一行
func (c *Config) StringCategorized() string {
	maskedPwd := "***"
	if c.DbPwd == "" {
		maskedPwd = "(empty)"
	}
	maskedBasicAuthPwd := ""
	if c.BasicAuthPassword != "" {
		maskedBasicAuthPwd = "***"
	} else {
		maskedBasicAuthPwd = "(empty)"
	}

	return fmt.Sprintf(`Configuration loaded:
  [Database] Host=%s, User=%s, Password=%s
  [Server] Listen=%s, MetricPath=%s, BasicAuth=%v(user=%s,pwd=%s)
  [Connection] MaxOpen=%d, MaxIdle=%d, Timeout=%ds, MaxLifetime=%dm
  [Logging] Level=%s, MaxSize=%dMB, MaxBackups=%d, MaxAge=%dd
  [Cache] BigKeyCache=%dm, AlarmKeyCache=%dm
  [Features] HostMetrics=%v, DBMetrics=%v, DmhsMetrics=%v, CustomMetrics=%v
  [SlowSQL] Enabled=%v, Threshold=%dms, MaxRows=%d
  [Security] EncodeConfigPwd=%v`,
		c.DbHost, c.DbUser, maskedPwd,
		c.ListenAddress, c.MetricPath, c.EnableBasicAuth, c.BasicAuthUsername, maskedBasicAuthPwd,
		c.MaxOpenConns, c.MaxIdleConns, c.QueryTimeout, c.ConnMaxLifetime,
		c.LogLevel, c.LogMaxSize, c.LogMaxBackups, c.LogMaxAge,
		c.BigKeyDataCacheTime, c.AlarmKeyCacheTime,
		c.RegisterHostMetrics, c.RegisterDatabaseMetrics, c.RegisterDmhsMetrics, c.RegisterCustomMetrics,
		c.CheckSlowSQL, c.SlowSqlTime, c.SlowSqlMaxRows,
		c.EncodeConfigPwd)
}

func UpdateConfigPassword(filePath, encryptedPwd string) error {
	// Read the existing file
	inputFile, err := os.Open(filePath)
	if err != nil {
		return err
	}
	defer inputFile.Close()

	var fileLines []string
	scanner := bufio.NewScanner(inputFile)
	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, "dbPwd=") {
			line = fmt.Sprintf("dbPwd=%s", encryptedPwd)
		}
		fileLines = append(fileLines, line)
	}
	if err := scanner.Err(); err != nil {
		return err
	}
	inputFile.Close()

	// Write the updated lines to the same file
	outputFile, err := os.OpenFile(filePath, os.O_WRONLY|os.O_TRUNC, 0644)
	if err != nil {
		return err
	}
	defer outputFile.Close()

	writer := bufio.NewWriter(outputFile)
	for _, line := range fileLines {
		fmt.Fprintln(writer, line)
	}
	writer.Flush()

	return nil
}
