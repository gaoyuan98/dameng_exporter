package config

import (
	"fmt"
	"os"
	"strings"

	"github.com/BurntSushi/toml"
	"github.com/duke-git/lancet/v2/fileutil"
)

// LoadMultiSourceConfig 加载多数据源配置。
// 它依次执行：文件存在性检查、读取字节、解析为原始 map 以记录字段显式情况、
// 反解析为结构体、结合字段存在信息补齐默认值、解密敏感字段并最终校验配置合法性。
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
	var raw rawMultiSourceConfig
	if _, err := toml.Decode(string(content), &raw); err != nil {
		return nil, fmt.Errorf("failed to parse config file as TOML: %w", err)
	}

	config := raw.toConfig()

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

// rawMultiSourceConfig 对应配置文件的原始映射，使用指针布尔字段以保留“是否显式配置”信息。
type rawMultiSourceConfig struct {
	ListenAddress        string                `toml:"listenAddress"`
	MetricPath           string                `toml:"metricPath"`
	Version              string                `toml:"version"`
	LogMaxSize           int                   `toml:"logMaxSize"`
	LogMaxBackups        int                   `toml:"logMaxBackups"`
	LogMaxAge            int                   `toml:"logMaxAge"`
	LogLevel             string                `toml:"logLevel"`
	EncodeConfigPwd      bool                  `toml:"encodeConfigPwd"`
	EnableBasicAuth      bool                  `toml:"enableBasicAuth"`
	BasicAuthUsername    string                `toml:"basicAuthUsername"`
	BasicAuthPassword    string                `toml:"basicAuthPassword"`
	GlobalTimeoutSeconds int                   `toml:"globalTimeoutSeconds"`
	CollectionMode       string                `toml:"collectionMode"`
	RetryIntervalSeconds int                   `toml:"retryIntervalSeconds"`
	EnableHealthPing     *bool                 `toml:"enableHealthPing"`
	DataSources          []rawDataSourceConfig `toml:"datasource"`
}

// toConfig 将原始结构转换为应用了默认值的最终配置结构。
func (raw rawMultiSourceConfig) toConfig() *MultiSourceConfig {
	cfg := DefaultMultiSourceConfig

	if raw.ListenAddress != "" {
		cfg.ListenAddress = raw.ListenAddress
	}
	if raw.MetricPath != "" {
		cfg.MetricPath = raw.MetricPath
	}
	if raw.Version != "" {
		cfg.Version = raw.Version
	}
	if raw.LogMaxSize != 0 {
		cfg.LogMaxSize = raw.LogMaxSize
	}
	if raw.LogMaxBackups != 0 {
		cfg.LogMaxBackups = raw.LogMaxBackups
	}
	if raw.LogMaxAge != 0 {
		cfg.LogMaxAge = raw.LogMaxAge
	}
	if raw.LogLevel != "" {
		cfg.LogLevel = raw.LogLevel
	}
	cfg.EncodeConfigPwd = raw.EncodeConfigPwd
	cfg.EnableBasicAuth = raw.EnableBasicAuth
	if raw.BasicAuthUsername != "" {
		cfg.BasicAuthUsername = raw.BasicAuthUsername
	}
	if raw.BasicAuthPassword != "" {
		cfg.BasicAuthPassword = raw.BasicAuthPassword
	}
	if raw.GlobalTimeoutSeconds != 0 {
		cfg.GlobalTimeoutSeconds = raw.GlobalTimeoutSeconds
	}
	if raw.CollectionMode != "" {
		cfg.CollectionMode = raw.CollectionMode
	}
	if raw.RetryIntervalSeconds != 0 {
		cfg.RetryIntervalSeconds = raw.RetryIntervalSeconds
	}
	if raw.EnableHealthPing != nil {
		cfg.EnableHealthPing = *raw.EnableHealthPing
		cfg.healthPingConfigured = true
	}

	cfg.DataSources = make([]DataSourceConfig, len(raw.DataSources))
	for i, dsRaw := range raw.DataSources {
		cfg.DataSources[i] = dsRaw.toConfig()
	}

	return &cfg
}

// rawDataSourceConfig 保留数据源级布尔字段的显式设置情况。
type rawDataSourceConfig struct {
	Name                    string `toml:"name"`
	Description             string `toml:"description"`
	Enabled                 *bool  `toml:"enabled"`
	DbHost                  string `toml:"dbHost"`
	DbUser                  string `toml:"dbUser"`
	DbPwd                   string `toml:"dbPwd"`
	QueryTimeout            int    `toml:"queryTimeout"`
	MaxOpenConns            int    `toml:"maxOpenConns"`
	MaxIdleConns            int    `toml:"maxIdleConns"` // Deprecated
	ConnMaxLifetime         int    `toml:"connMaxLifetime"`
	BigKeyDataCacheTime     int    `toml:"bigKeyDataCacheTime"`
	AlarmKeyCacheTime       int    `toml:"alarmKeyCacheTime"`
	CheckSlowSQL            *bool  `toml:"checkSlowSQL"`
	SlowSqlTime             int    `toml:"slowSqlTime"`
	SlowSqlMaxRows          int    `toml:"slowSqlMaxRows"`
	RegisterHostMetrics     *bool  `toml:"registerHostMetrics"`
	RegisterDatabaseMetrics *bool  `toml:"registerDatabaseMetrics"`
	RegisterDmhsMetrics     *bool  `toml:"registerDmhsMetrics"`
	RegisterCustomMetrics   *bool  `toml:"registerCustomMetrics"`
	Labels                  string `toml:"labels"`
	CustomMetricsFile       string `toml:"customMetricsFile"`
}

// toConfig 将原始数据源配置转换为最终结构，并在必要时套用默认值。
func (raw rawDataSourceConfig) toConfig() DataSourceConfig {
	cfg := DefaultDataSourceConfig

	cfg.Name = raw.Name
	cfg.Description = raw.Description
	if raw.Enabled != nil {
		cfg.Enabled = *raw.Enabled
	}
	cfg.DbHost = raw.DbHost
	cfg.DbUser = raw.DbUser
	cfg.DbPwd = raw.DbPwd
	if raw.QueryTimeout != 0 {
		cfg.QueryTimeout = raw.QueryTimeout
	}
	if raw.MaxOpenConns != 0 {
		cfg.MaxOpenConns = raw.MaxOpenConns
	}
	if raw.ConnMaxLifetime != 0 {
		cfg.ConnMaxLifetime = raw.ConnMaxLifetime
	}
	if raw.BigKeyDataCacheTime != 0 {
		cfg.BigKeyDataCacheTime = raw.BigKeyDataCacheTime
	}
	if raw.AlarmKeyCacheTime != 0 {
		cfg.AlarmKeyCacheTime = raw.AlarmKeyCacheTime
	}
	if raw.CheckSlowSQL != nil {
		cfg.CheckSlowSQL = *raw.CheckSlowSQL
	}
	if raw.SlowSqlTime != 0 {
		cfg.SlowSqlTime = raw.SlowSqlTime
	}
	if raw.SlowSqlMaxRows != 0 {
		cfg.SlowSqlMaxRows = raw.SlowSqlMaxRows
	}
	if raw.RegisterHostMetrics != nil {
		cfg.RegisterHostMetrics = *raw.RegisterHostMetrics
	}
	if raw.RegisterDatabaseMetrics != nil {
		cfg.RegisterDatabaseMetrics = *raw.RegisterDatabaseMetrics
	}
	if raw.RegisterDmhsMetrics != nil {
		cfg.RegisterDmhsMetrics = *raw.RegisterDmhsMetrics
	}
	if raw.RegisterCustomMetrics != nil {
		cfg.RegisterCustomMetrics = *raw.RegisterCustomMetrics
	}
	cfg.Labels = raw.Labels
	cfg.CustomMetricsFile = raw.CustomMetricsFile

	cfg.ApplyDefaults()

	if raw.MaxIdleConns != 0 {
		fmt.Printf("警告：数据源 %s 的 maxIdleConns 参数已废弃，将强制使用 maxOpenConns=%d（原配置值=%d）\n",
			cfg.Name, cfg.MaxOpenConns, raw.MaxIdleConns)
	}

	return cfg
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
			CustomMetricsFile:       "", // 命令行模式下默认不使用自定义指标文件
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
		if args.EnableHealthPing != nil {
			config.EnableHealthPing = *args.EnableHealthPing
			config.healthPingConfigured = true
		}
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
