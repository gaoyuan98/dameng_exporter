package config

import (
	"bufio"
	"fmt"
	"os"
	"strconv"
	"strings"
)

var hostName string

func SetHostName(hn string) {
	hostName = hn
}

func GetHostName() string {
	return hostName
}

var GlobalConfig *Config

type Config struct {
	ConfigFile        string
	CustomMetricsFile string
	ListenAddress     string
	MetricPath        string

	//QueryTimeout  time.Duration
	QueryTimeout int
	MaxIdleConns int
	MaxOpenConns int
	//ConnMaxLifetime time.Duration
	ConnMaxLifetime int
	LogMaxSize      int
	LogMaxBackups   int
	LogMaxAge       int
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
	//QueryTimeout:    30 * time.Second,
	QueryTimeout:            30, //秒
	MaxIdleConns:            1,  //个数
	MaxOpenConns:            10, //个数
	ConnMaxLifetime:         30, //分钟
	LogMaxSize:              10, //MB
	LogMaxBackups:           3,  //个数
	LogMaxAge:               30, //天
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
		key := strings.TrimSpace(parts[0])
		value := strings.TrimSpace(parts[1])
		switch key {
		case "configFile":
			config.ConfigFile = value
		case "customMetricsFile":
			config.CustomMetricsFile = value
		case "listenAddress":
			config.ListenAddress = value
		case "metricPath":
			config.MetricPath = value
		case "queryTimeout":
			/*			d, err := time.ParseDuration(value)
						if err == nil {
							config.QueryTimeout = d
						}*/
			if val, err := strconv.Atoi(value); err == nil {
				config.QueryTimeout = val
			}
		case "maxIdleConns":
			if val, err := strconv.Atoi(value); err == nil {
				config.MaxIdleConns = val
			}
		case "maxOpenConns":
			if val, err := strconv.Atoi(value); err == nil {
				config.MaxOpenConns = val
			}
		case "connMaxLifetime":
			/*			d, err := time.ParseDuration(value)
						if err == nil {
							config.ConnMaxLifetime = d
						}*/
			if val, err := strconv.Atoi(value); err == nil {
				config.ConnMaxLifetime = val
			}

		case "logMaxSize":
			if val, err := strconv.Atoi(value); err == nil {
				config.LogMaxSize = val
			}
		case "logMaxBackups":
			if val, err := strconv.Atoi(value); err == nil {
				config.LogMaxBackups = val
			}
		case "logMaxAge":
			if val, err := strconv.Atoi(value); err == nil {
				config.LogMaxAge = val
			}
		case "dbUser":
			config.DbUser = value
		case "dbPwd":
			config.DbPwd = value
		case "dbHost":
			/*			if val, err := strconv.Atoi(value); err == nil {
						config.DbHost = val
					}*/
			config.DbHost = value
		case "bigKeyDataCacheTime":
			if val, err := strconv.Atoi(value); err == nil {
				config.BigKeyDataCacheTime = val
			}
		case "alarmKeyCacheTime":
			if val, err := strconv.Atoi(value); err == nil {
				config.AlarmKeyCacheTime = val
			}
		case "registerHostMetrics":
			if val, err := strconv.ParseBool(value); err == nil {
				config.RegisterHostMetrics = val
			}
		case "registerDatabaseMetrics":
			if val, err := strconv.ParseBool(value); err == nil {
				config.RegisterDatabaseMetrics = val
			}
		case "registerDmhsMetrics":
			if val, err := strconv.ParseBool(value); err == nil {
				config.RegisterDmhsMetrics = val
			}
		case "registerCustomMetrics":
			if val, err := strconv.ParseBool(value); err == nil {
				config.RegisterCustomMetrics = val
			}
		case "encodeConfigPwd":
			if val, err := strconv.ParseBool(value); err == nil {
				config.EncodeConfigPwd = val
			}

		case "checkSlowSql":
			if val, err := strconv.ParseBool(value); err == nil {
				config.CheckSlowSQL = val
			}
		case "slowSqlTime":
			if val, err := strconv.Atoi(value); err == nil {
				config.SlowSqlTime = val
			}
		case "slowSqlLimitRows":
			if val, err := strconv.Atoi(value); err == nil {
				config.SlowSqlMaxRows = val
			}
		case "enableBasicAuth":
			if val, err := strconv.ParseBool(value); err == nil {
				config.EnableBasicAuth = val
			}
		case "basicAuthUsername":
			config.BasicAuthUsername = value
		case "basicAuthPassword":
			config.BasicAuthPassword = value
		}
	}

	return config, scanner.Err()
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
