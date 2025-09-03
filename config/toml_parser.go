package config

import (
	"fmt"
	"os"
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

	// 解析TOML配置
	config := &MultiSourceConfig{}
	if _, err := toml.Decode(string(content), config); err != nil {
		return nil, fmt.Errorf("failed to parse config file as TOML: %w", err)
	}

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

// MergeMultiSourceConfigFromCmdArgs 合并命令行参数到多数据源配置
// 统一的合并逻辑：命令行参数总是覆盖配置文件的值
func MergeMultiSourceConfigFromCmdArgs(config *MultiSourceConfig, args *CmdArgs) {
	// 检查是否通过命令行指定了数据库连接三要素
	hasDbHost := args.DbHost != nil && *args.DbHost != ""
	hasDbUser := args.DbUser != nil && *args.DbUser != ""
	hasDbPwd := args.DbPwd != nil && *args.DbPwd != ""

	// 如果命令行指定了完整的数据库连接参数，替换配置文件中的数据源
	if hasDbHost && hasDbUser && hasDbPwd {
		// 使用命令行参数创建新的数据源
		config.DataSources = []DataSourceConfig{{
			Name:                    fmt.Sprintf("cmdline_%s", strings.Split(*args.DbHost, ":")[0]),
			Description:             fmt.Sprintf("DataSource from command line (Host: %s)", *args.DbHost),
			Enabled:                 true,
			DbHost:                  *args.DbHost,
			DbUser:                  *args.DbUser,
			DbPwd:                   *args.DbPwd,
			QueryTimeout:            *args.QueryTimeout,
			MaxOpenConns:            *args.MaxOpenConns,
			MaxIdleConns:            *args.MaxIdleConns,
			ConnMaxLifetime:         *args.ConnMaxLife,
			BigKeyDataCacheTime:     *args.BigKeyDataCacheTime,
			AlarmKeyCacheTime:       *args.AlarmKeyCacheTime,
			CheckSlowSQL:            *args.CheckSlowSQL,
			SlowSqlTime:             *args.SlowSqlTime,
			SlowSqlMaxRows:          *args.SlowSqlMaxRows,
			RegisterHostMetrics:     *args.RegisterHostMetrics,
			RegisterDatabaseMetrics: *args.RegisterDatabaseMetrics,
			RegisterDmhsMetrics:     *args.RegisterDmhsMetrics,
			RegisterCustomMetrics:   *args.RegisterCustomMetrics,
			CustomMetricsFile:       DefaultDataSourceConfig.CustomMetricsFile,
		}}
	} else if hasDbHost || hasDbUser || hasDbPwd {
		// 如果只指定了部分数据库连接参数，报错
		fmt.Println("错误：数据库连接参数不完整！")
		fmt.Println("必须同时指定：--dbHost、--dbUser、--dbPwd")
		fmt.Println("\n示例：")
		fmt.Println("  ./dameng_exporter --dbHost=127.0.0.1:5236 --dbUser=SYSDBA --dbPwd=SYSDBA")
		os.Exit(1)
	}

	// 如果是命令行模式（指定了数据库参数），使用命令行的所有参数
	// 否则保持配置文件的值不变（配置文件已经应用了默认值）
	if hasDbHost && hasDbUser && hasDbPwd {
		// 命令行模式：覆盖所有全局参数
		config.ListenAddress = *args.ListenAddr
		config.MetricPath = *args.MetricPath
		config.LogMaxSize = *args.LogMaxSize
		config.LogMaxBackups = *args.LogMaxBackups
		config.LogMaxAge = *args.LogMaxAge
		config.LogLevel = *args.LogLevel
		config.EncodeConfigPwd = *args.EncodeConfigPwd
		config.EnableBasicAuth = *args.EnableBasicAuth
		config.BasicAuthUsername = *args.BasicAuthUsername
		config.BasicAuthPassword = *args.BasicAuthPassword
		config.GlobalTimeoutSeconds = *args.GlobalTimeoutSeconds
		config.CollectionMode = *args.CollectionMode
	}
	// 配置文件模式：不覆盖，保持配置文件的值

	// 验证最终配置
	for i := range config.DataSources {
		ds := &config.DataSources[i]
		if ds.DbHost == "" || ds.DbUser == "" || ds.DbPwd == "" {
			fmt.Printf("错误：数据源 %s 缺少必需的数据库连接参数\n", ds.Name)
			fmt.Println("必需参数：dbHost、dbUser、dbPwd")
			os.Exit(1)
		}
	}
}

// DecryptPasswords 解密配置中的密码
func (msc *MultiSourceConfig) DecryptPasswords() error {
	// 解密数据源密码
	for i := range msc.DataSources {
		if strings.HasPrefix(msc.DataSources[i].DbPwd, "ENC(") && strings.HasSuffix(msc.DataSources[i].DbPwd, ")") {
			decPwd, err := DecryptPassword(msc.DataSources[i].DbPwd)
			if err != nil {
				return fmt.Errorf("failed to decrypt password for datasource %s: %w", msc.DataSources[i].Name, err)
			}
			msc.DataSources[i].DbPwd = decPwd
		}
	}

	// 解密Basic Auth密码
	if msc.EnableBasicAuth && strings.HasPrefix(msc.BasicAuthPassword, "ENC(") && strings.HasSuffix(msc.BasicAuthPassword, ")") {
		decPwd, err := DecryptPassword(msc.BasicAuthPassword)
		if err != nil {
			return fmt.Errorf("failed to decrypt basic auth password: %w", err)
		}
		msc.BasicAuthPassword = decPwd
	}

	return nil
}

// SaveMultiSourceConfig 保存多数据源配置
func SaveMultiSourceConfig(config *MultiSourceConfig, configFile string) error {
	if configFile == "" {
		configFile = config.ConfigFile
	}
	if configFile == "" {
		return fmt.Errorf("config file path is empty")
	}

	// 保存配置
	file, err := os.Create(configFile)
	if err != nil {
		return fmt.Errorf("failed to create config file: %w", err)
	}
	defer file.Close()

	// 应用默认值
	saveConfig := *config
	saveConfig.ApplyAllDefaults()

	// 编码为TOML
	encoder := toml.NewEncoder(file)
	if err := encoder.Encode(saveConfig); err != nil {
		return fmt.Errorf("failed to encode config to TOML: %w", err)
	}

	return nil
}
