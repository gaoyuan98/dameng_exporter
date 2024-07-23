package config

import (
	"encoding/base64"
	"strings"
)

const salt = "dameng_exporter"
const xorKey = 0xAA
const encryptedPrefix = "ENC:"

// EncryptPassword encrypts the password with a simple XOR and Base64 encoding
func EncryptPassword(password string) string {
	if strings.HasPrefix(password, encryptedPrefix) {
		// The password is not encrypted
		return password
	}
	saltedPwd := salt + password
	encrypted := make([]byte, len(saltedPwd))
	for i := 0; i < len(saltedPwd); i++ {
		encrypted[i] = saltedPwd[i] ^ xorKey
	}
	return encryptedPrefix + base64.StdEncoding.EncodeToString(encrypted)
}

// DecryptPassword decrypts the password that was encrypted with EncryptPassword
func DecryptPassword(encoded string) (string, error) {
	if !strings.HasPrefix(encoded, encryptedPrefix) {
		// The password is not encrypted
		return encoded, nil
	}
	encoded = strings.TrimPrefix(encoded, encryptedPrefix)
	decodedBytes, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return "", err
	}
	decrypted := make([]byte, len(decodedBytes))
	for i := 0; i < len(decodedBytes); i++ {
		decrypted[i] = decodedBytes[i] ^ xorKey
	}
	return string(decrypted[len(salt):]), nil
}
