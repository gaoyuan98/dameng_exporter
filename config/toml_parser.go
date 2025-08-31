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

	// 如果TOML解析失败，尝试作为旧格式解析
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

	// 创建临时配置用于保存（避免保存已解密的密码）
	saveConfig := *config
	if config.EncodeConfigPwd {
		// 加密密码
		for i := range saveConfig.DataSources {
			if !strings.HasPrefix(saveConfig.DataSources[i].DbPwd, "ENC(") {
				saveConfig.DataSources[i].DbPwd = fmt.Sprintf("ENC(%s)", EncryptPassword(saveConfig.DataSources[i].DbPwd))
			}
		}
		if saveConfig.EnableBasicAuth && saveConfig.BasicAuthPassword != "" {
			if !strings.HasPrefix(saveConfig.BasicAuthPassword, "ENC(") {
				saveConfig.BasicAuthPassword = fmt.Sprintf("ENC(%s)", EncryptPassword(saveConfig.BasicAuthPassword))
			}
		}
	}

	// 打开文件
	file, err := os.Create(configFile)
	if err != nil {
		return fmt.Errorf("failed to create config file: %w", err)
	}
	defer file.Close()

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
			encPwd := msc.DataSources[i].DbPwd[4 : len(msc.DataSources[i].DbPwd)-1]
			decPwd, err := DecryptPassword(encPwd)
			if err != nil {
				return fmt.Errorf("failed to decrypt password for datasource %s: %w", msc.DataSources[i].Name, err)
			}
			msc.DataSources[i].DbPwd = decPwd
		}
	}

	// 解密Basic Auth密码
	if msc.EnableBasicAuth && strings.HasPrefix(msc.BasicAuthPassword, "ENC(") && strings.HasSuffix(msc.BasicAuthPassword, ")") {
		encPwd := msc.BasicAuthPassword[4 : len(msc.BasicAuthPassword)-1]
		decPwd, err := DecryptPassword(encPwd)
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
