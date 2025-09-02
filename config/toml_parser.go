package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/BurntSushi/toml"
	"github.com/duke-git/lancet/v2/fileutil"
)

// LoadMultiSourceConfig 加载多数据源配置
func LoadMultiSourceConfig(configFile string) (*MultiSourceConfig, error) {
	if configFile == "" {
		return nil, fmt.Errorf("config file path is empty")
	}

	// 检查文件是否存在
	if !fileutil.IsExist(configFile) {
		return nil, fmt.Errorf("config file not found: %s", configFile)
	}

	// 读取文件内容
	content, err := os.ReadFile(configFile)
	if err != nil {
		return nil, fmt.Errorf("failed to read config file: %w", err)
	}

	// 尝试解析为TOML格式
	config := &MultiSourceConfig{}
	if _, err := toml.Decode(string(content), config); err == nil {
		// TOML格式解析成功
		config.ConfigFile = configFile

		// 应用默认值
		config.ApplyAllDefaults()

		// 解密密码
		if err := config.DecryptPasswords(); err != nil {
			return nil, fmt.Errorf("failed to decrypt passwords: %w", err)
		}

		// 验证配置
		if err := config.ValidateAll(); err != nil {
			return nil, fmt.Errorf("config validation failed: %w", err)
		}

		return config, nil
	}

	// TOML解析失败
	return nil, fmt.Errorf("failed to parse config file as TOML: %w", err)
}

// SaveMultiSourceConfig 保存多数据源配置
func SaveMultiSourceConfig(config *MultiSourceConfig, configFile string) error {
	if configFile == "" {
		configFile = config.ConfigFile
	}
	if configFile == "" {
		return fmt.Errorf("config file path is empty")
	}

	// 创建目录（如果不存在）
	dir := filepath.Dir(configFile)
	if !fileutil.IsExist(dir) {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return fmt.Errorf("failed to create directory: %w", err)
		}
	}

	// 保存配置（直接保存传入的配置，不做任何修改）
	file, err := os.Create(configFile)
	if err != nil {
		return fmt.Errorf("failed to create config file: %w", err)
	}
	defer file.Close()

	// 应用默认值（但不修改已加密的密码）
	saveConfig := *config
	saveConfig.ApplyAllDefaults()

	// 编码为TOML
	encoder := toml.NewEncoder(file)
	if err := encoder.Encode(saveConfig); err != nil {
		return fmt.Errorf("failed to encode config to TOML: %w", err)
	}

	return nil
}

// DecryptPasswords 解密配置中的密码
func (msc *MultiSourceConfig) DecryptPasswords() error {
	// 解密数据源密码
	for i := range msc.DataSources {
		if strings.HasPrefix(msc.DataSources[i].DbPwd, "ENC(") && strings.HasSuffix(msc.DataSources[i].DbPwd, ")") {
			// 传递完整的 ENC(...) 字符串给 DecryptPassword
			decPwd, err := DecryptPassword(msc.DataSources[i].DbPwd)
			if err != nil {
				return fmt.Errorf("failed to decrypt password for datasource %s: %w", msc.DataSources[i].Name, err)
			}
			msc.DataSources[i].DbPwd = decPwd
		}
	}

	// 解密Basic Auth密码
	if msc.EnableBasicAuth && strings.HasPrefix(msc.BasicAuthPassword, "ENC(") && strings.HasSuffix(msc.BasicAuthPassword, ")") {
		// 传递完整的 ENC(...) 字符串给 DecryptPassword
		decPwd, err := DecryptPassword(msc.BasicAuthPassword)
		if err != nil {
			return fmt.Errorf("failed to decrypt basic auth password: %w", err)
		}
		msc.BasicAuthPassword = decPwd
	}

	return nil
}

// UpdateMultiSourceConfigPasswords 更新多数据源配置文件中的密码为加密格式
func UpdateMultiSourceConfigPasswords(configFile string) error {
	// 加载配置
	config, err := LoadMultiSourceConfig(configFile)
	if err != nil {
		return err
	}

	// 设置加密标志
	config.EncodeConfigPwd = true

	// 保存配置（SaveMultiSourceConfig会自动加密密码）
	return SaveMultiSourceConfig(config, configFile)
}

// MergeMultiSourceConfigFromCmdArgs 合并命令行参数到多数据源配置
func MergeMultiSourceConfigFromCmdArgs(config *MultiSourceConfig, args *CmdArgs) {
	// 检查是否通过命令行指定了数据库连接三要素
	hasDbHost := args.DbHost != nil && *args.DbHost != ""
	hasDbUser := args.DbUser != nil && *args.DbUser != ""
	hasDbPwd := args.DbPwd != nil && *args.DbPwd != ""

	// 如果指定了数据库连接参数，则必须同时指定 DbHost、DbUser、DbPwd
	if hasDbHost || hasDbUser || hasDbPwd {
		if hasDbHost && hasDbUser && hasDbPwd {
			// 清空配置文件中的数据源，使用命令行参数创建新的数据源
			config.DataSources = []DataSourceConfig{
				{
					Name:                    "cmdline_datasource",
					Description:             "DataSource from command line",
					Enabled:                 true,
					DbHost:                  *args.DbHost,
					DbUser:                  *args.DbUser,
					DbPwd:                   *args.DbPwd,
					QueryTimeout:            getIntValue(args.QueryTimeout, DefaultDataSourceConfig.QueryTimeout),
					MaxOpenConns:            getIntValue(args.MaxOpenConns, DefaultDataSourceConfig.MaxOpenConns),
					MaxIdleConns:            getIntValue(args.MaxIdleConns, DefaultDataSourceConfig.MaxIdleConns),
					ConnMaxLifetime:         getIntValue(args.ConnMaxLife, DefaultDataSourceConfig.ConnMaxLifetime),
					BigKeyDataCacheTime:     getIntValue(args.BigKeyDataCacheTime, DefaultDataSourceConfig.BigKeyDataCacheTime),
					AlarmKeyCacheTime:       getIntValue(args.AlarmKeyCacheTime, DefaultDataSourceConfig.AlarmKeyCacheTime),
					CheckSlowSQL:            getBoolValue(args.CheckSlowSQL, DefaultDataSourceConfig.CheckSlowSQL),
					SlowSqlTime:             getIntValue(args.SlowSqlTime, DefaultDataSourceConfig.SlowSqlTime),
					SlowSqlMaxRows:          getIntValue(args.SlowSqlMaxRows, DefaultDataSourceConfig.SlowSqlMaxRows),
					RegisterHostMetrics:     getBoolValue(args.RegisterHostMetrics, DefaultDataSourceConfig.RegisterHostMetrics),
					RegisterDatabaseMetrics: getBoolValue(args.RegisterDatabaseMetrics, DefaultDataSourceConfig.RegisterDatabaseMetrics),
					RegisterDmhsMetrics:     getBoolValue(args.RegisterDmhsMetrics, DefaultDataSourceConfig.RegisterDmhsMetrics),
					RegisterCustomMetrics:   getBoolValue(args.RegisterCustomMetrics, DefaultDataSourceConfig.RegisterCustomMetrics),
					CustomMetricsFile:       DefaultDataSourceConfig.CustomMetricsFile,
				},
			}
			// 应用默认值
			config.DataSources[0].ApplyDefaults()
		} else {
			// 如果只指定了部分数据库连接参数，保持原有逻辑不变
			// 这里可以选择报错或者忽略
			fmt.Printf("Warning: Database connection parameters must all be specified together (dbHost, dbUser, dbPwd)\n")
		}
	}

	// 合并全局参数（这些参数总是会被合并，不管是否指定了数据库连接参数）
	if args.ListenAddr != nil && *args.ListenAddr != "" {
		config.ListenAddress = *args.ListenAddr
	}
	if args.MetricPath != nil && *args.MetricPath != "" {
		config.MetricPath = *args.MetricPath
	}
	if args.LogMaxSize != nil && *args.LogMaxSize > 0 {
		config.LogMaxSize = *args.LogMaxSize
	}
	if args.LogMaxBackups != nil && *args.LogMaxBackups > 0 {
		config.LogMaxBackups = *args.LogMaxBackups
	}
	if args.LogMaxAge != nil && *args.LogMaxAge > 0 {
		config.LogMaxAge = *args.LogMaxAge
	}
	if args.LogLevel != nil && *args.LogLevel != "" {
		config.LogLevel = *args.LogLevel
	}
	if args.EncodeConfigPwd != nil {
		config.EncodeConfigPwd = *args.EncodeConfigPwd
	}
	if args.EnableBasicAuth != nil {
		config.EnableBasicAuth = *args.EnableBasicAuth
	}
	if args.BasicAuthUsername != nil && *args.BasicAuthUsername != "" {
		config.BasicAuthUsername = *args.BasicAuthUsername
	}
	if args.BasicAuthPassword != nil && *args.BasicAuthPassword != "" {
		config.BasicAuthPassword = *args.BasicAuthPassword
	}
	if args.GlobalTimeoutSeconds != nil && *args.GlobalTimeoutSeconds > 0 {
		config.GlobalTimeoutSeconds = *args.GlobalTimeoutSeconds
	}

	// 合并采集模式参数
	if args.CollectionMode != nil && *args.CollectionMode != "" {
		config.CollectionMode = *args.CollectionMode
	}
}

// 辅助函数：获取int指针的值，如果为nil则返回默认值
func getIntValue(ptr *int, defaultValue int) int {
	if ptr != nil && *ptr > 0 {
		return *ptr
	}
	return defaultValue
}

// 辅助函数：获取bool指针的值，如果为nil则返回默认值
func getBoolValue(ptr *bool, defaultValue bool) bool {
	if ptr != nil {
		return *ptr
	}
	return defaultValue
}
