package auth

import (
	"dameng_exporter/config"
	"dameng_exporter/logger"
	"fmt"
	"net/http"

	"golang.org/x/crypto/bcrypt"
)

// BasicAuthMiddleware 处理Basic认证的中间件
func BasicAuthMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !config.GlobalConfig.EnableBasicAuth {
			next.ServeHTTP(w, r)
			return
		}

		username, password, ok := r.BasicAuth()
		if !ok {
			w.Header().Set("WWW-Authenticate", `Basic realm="Restricted"`)
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			w.WriteHeader(http.StatusUnauthorized)
			fmt.Fprintf(w, "<html><body><h1>认证失败</h1><p>请提供Basic认证信息</p></body></html>")
			logger.Logger.Warn("Basic auth failed: no credentials provided")
			return
		}

		// 验证用户名
		if username != config.GlobalConfig.BasicAuthUsername {
			w.Header().Set("WWW-Authenticate", `Basic realm="Restricted"`)
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			w.WriteHeader(http.StatusUnauthorized)
			fmt.Fprintf(w, "<html><body><h1>认证失败</h1><p>用户名 '%s' 不正确</p></body></html>", username)
			logger.Logger.Warnf("Basic auth failed: invalid username '%s'", username)
			return
		}

		// 验证密码
		err := bcrypt.CompareHashAndPassword([]byte(config.GlobalConfig.BasicAuthPassword), []byte(password))
		if err != nil {
			w.Header().Set("WWW-Authenticate", `Basic realm="Restricted"`)
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			w.WriteHeader(http.StatusUnauthorized)
			fmt.Fprintf(w, "<html><body><h1>认证失败</h1><p>用户 '%s' 的密码不正确</p></body></html>", username)
			logger.Logger.Warnf("Basic auth failed: invalid password for user '%s'", username)
			return
		}

		next.ServeHTTP(w, r)
	})
}

// GenerateBcryptPassword 生成bcrypt加密的密码
func GenerateBcryptPassword(password string) (string, error) {
	hash, err := bcrypt.GenerateFromPassword([]byte(password), 12)
	if err != nil {
		return "", err
	}
	return string(hash), nil
}

// 加密basic auth密码并返回
func ExecEncryptBasicAuthPwdCmd(encryptBasicAuthPwd *string) bool {
	//命令行参数，对密码加密并返回结果
	if *encryptBasicAuthPwd != "" {
		encryptedPwd, err := GenerateBcryptPassword(*encryptBasicAuthPwd)
		if err != nil {
			fmt.Printf("Error encrypting password: %v\n", err)
			return true
		}
		fmt.Printf("Encrypted Basic Auth Password: %s\n", encryptedPwd)
		return true
	}
	return false
}
