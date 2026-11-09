package config

import (
	"encoding/base64"
	"fmt"
	"os"
	"strings"

	"github.com/BurntSushi/toml"
)

const salt = "dameng_exporter"
const xorKey = 0xAA
const encryptedPrefix = "ENC("
const encryptedSuffix = ")"

// EncryptPassword encrypts the password with a simple XOR and Base64 encoding
func EncryptPassword(password string) string {
	saltedPwd := salt + password
	encrypted := make([]byte, len(saltedPwd))
	for i := 0; i < len(saltedPwd); i++ {
		encrypted[i] = saltedPwd[i] ^ xorKey
	}
	return encryptedPrefix + base64.StdEncoding.EncodeToString(encrypted) + encryptedSuffix
}

// DecryptPassword decrypts the password that was encrypted with EncryptPassword
func DecryptPassword(encoded string) (string, error) {
	if !strings.HasPrefix(encoded, encryptedPrefix) {
		// The password is not encrypted
		return encoded, nil
	}
	if !strings.HasSuffix(encoded, encryptedSuffix) {
		return "", fmt.Errorf("encrypted password must end with )")
	}
	encoded = strings.TrimPrefix(encoded, encryptedPrefix)
	encoded = strings.TrimSuffix(encoded, encryptedSuffix)
	decodedBytes, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return "", fmt.Errorf("invalid encrypted password encoding")
	}
	if len(decodedBytes) < len(salt) {
		return "", fmt.Errorf("encrypted password payload is too short")
	}
	decrypted := make([]byte, len(decodedBytes))
	for i := 0; i < len(decodedBytes); i++ {
		decrypted[i] = decodedBytes[i] ^ xorKey
	}
	if !strings.HasPrefix(string(decrypted), salt) {
		return "", fmt.Errorf("invalid encrypted password marker")
	}
	return string(decrypted[len(salt):]), nil
}

func hasEncryptedPasswordEnvelope(value string) bool {
	return strings.HasPrefix(value, encryptedPrefix) && strings.HasSuffix(value, encryptedSuffix)
}

// DecodePassword 保留旧配置的完整 ENC(...) 自动识别，plain 模式用于内容恰好为 ENC(...) 的明文。
func DecodePassword(value, mode string) (string, error) {
	switch mode {
	case "plain":
		return value, nil
	case "", "auto":
		if !hasEncryptedPasswordEnvelope(value) {
			return value, nil
		}
		return DecryptPassword(value)
	case "encrypted":
		if !hasEncryptedPasswordEnvelope(value) {
			return "", fmt.Errorf("encrypted password requires ENC(...) format")
		}
		return DecryptPassword(value)
	default:
		return "", fmt.Errorf("password mode must be auto, plain or encrypted")
	}
}

// CheckAndEncryptConfigPasswords 检查并加密配置文件中的密码
// 如果配置启用了密码加密（EncodeConfigPwd为true），会自动加密未加密的密码并更新配置文件
// 返回更新后的配置和错误信息
func CheckAndEncryptConfigPasswords(multiConfig *MultiSourceConfig, configFile string) (*MultiSourceConfig, error) {
	// 如果未启用密码加密，直接返回原配置
	if !multiConfig.EncodeConfigPwd {
		return multiConfig, nil
	}

	// 读取原始配置文件内容以检查密码格式
	rawContent, err := os.ReadFile(configFile)
	if err != nil {
		return multiConfig, fmt.Errorf("failed to read config file for password check: %v", err)
	}

	rawConfig := &rawMultiSourceConfig{}
	if _, err := toml.Decode(string(rawContent), rawConfig); err != nil {
		return multiConfig, fmt.Errorf("failed to decode config for password check: %v", err)
	}

	needUpdate := false
	// 检查每个数据源的密码是否需要加密
	for i := range rawConfig.DataSources {
		ds := &rawConfig.DataSources[i]
		if _, err := DecodePassword(ds.DbPwd, ds.DbPwdMode); err != nil {
			return multiConfig, fmt.Errorf("invalid password for datasource %s: %w", ds.Name, err)
		}
		isEncrypted := ds.DbPwdMode != "plain" && hasEncryptedPasswordEnvelope(ds.DbPwd)
		if ds.DbPwd != "" && !isEncrypted {
			// 加密密码（EncryptPassword 返回 ENC(...) 格式）
			ds.DbPwd = EncryptPassword(ds.DbPwd)
			ds.DbPwdMode = "encrypted"
			needUpdate = true
			fmt.Printf("Encrypted password for datasource: %s\n", rawConfig.DataSources[i].Name)
		}
	}

	// 如果有密码被加密，更新配置文件
	if needUpdate {
		if err := SaveMultiSourceConfig(rawConfig.toConfig(), configFile); err != nil {
			return multiConfig, fmt.Errorf("failed to update config file with encrypted passwords: %v", err)
		}
		fmt.Println("Config file updated with encrypted passwords successfully")

		// 重新加载配置以使用加密后的密码
		updatedConfig, err := LoadMultiSourceConfig(configFile)
		if err != nil {
			return multiConfig, fmt.Errorf("failed to reload config after encryption: %v", err)
		}
		return updatedConfig, nil
	}

	return multiConfig, nil
}
