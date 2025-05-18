package auth

import (
	"crypto/subtle"
	"dameng_exporter/config"
	"dameng_exporter/logger"
	"fmt"
	"net/http"
	"strings"

	"golang.org/x/crypto/bcrypt"
)

// isEncryptedPassword 判断密码是否为bcrypt加密格式
func isEncryptedPassword(password string) bool {
	// bcrypt加密后的密码格式: $2a$10$...
	return strings.HasPrefix(password, "$2a$")
}

// BasicAuthMiddleware 处理Basic认证的中间件
func BasicAuthMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !config.GlobalConfig.EnableBasicAuth {
			next.ServeHTTP(w, r)
			return
		}

		username, password, ok := r.BasicAuth()
		if !ok {
			w.Header().Set("WWW-Authenticate", `Basic realm="DAMENG Exporter Metrics"`)
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			w.WriteHeader(http.StatusUnauthorized)
			fmt.Fprintf(w, `
				<html>
					<head>
						<title>认证失败</title>
						<style>
							body { font-family: Arial, sans-serif; margin: 40px; }
							.error { color: #d32f2f; }
							.retry { margin-top: 20px; }
							a { color: #1976d2; text-decoration: none; }
							a:hover { text-decoration: underline; }
						</style>
					</head>
					<body>
						<h1 class="error">认证失败</h1>
						<p>请提供有效的认证信息</p>
						<div class="retry">
							<a href="%s">重新登录</a>
						</div>
					</body>
				</html>`, r.URL.Path)
			logger.Logger.Warn("Basic auth failed: no credentials provided")
			return
		}

		// 验证用户名
		if subtle.ConstantTimeCompare([]byte(username), []byte(config.GlobalConfig.BasicAuthUsername)) != 1 {
			w.Header().Set("WWW-Authenticate", `Basic realm="DAMENG Exporter Metrics"`)
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			w.WriteHeader(http.StatusUnauthorized)
			fmt.Fprintf(w, `
				<html>
					<head>
						<title>认证失败</title>
						<style>
							body { font-family: Arial, sans-serif; margin: 40px; }
							.error { color: #d32f2f; }
							.retry { margin-top: 20px; }
							a { color: #1976d2; text-decoration: none; }
							a:hover { text-decoration: underline; }
						</style>
					</head>
					<body>
						<h1 class="error">认证失败</h1>
						<p>用户名 '%s' 不正确</p>
						<div class="retry">
							<a href="%s">重新登录</a>
						</div>
					</body>
				</html>`, username, r.URL.Path)
			logger.Logger.Warnf("Basic auth failed: invalid username '%s'", username)
			return
		}

		// 获取配置的密码
		configuredPassword := config.GlobalConfig.BasicAuthPassword
		//logger.Logger.Debugf("Configured password type: %v", isEncryptedPassword(configuredPassword))
		//logger.Logger.Debugf("Configured password: %s", configuredPassword)
		//logger.Logger.Debugf("Received password type: %v", isEncryptedPassword(password))
		//logger.Logger.Debugf("Received password: %s", password)

		// 验证密码
		var authSuccess bool

		// 如果配置的是加密密码
		if isEncryptedPassword(configuredPassword) {
			// 尝试使用bcrypt验证
			err := bcrypt.CompareHashAndPassword([]byte(configuredPassword), []byte(password))
			if err == nil {
				authSuccess = true
				//logger.Logger.Debugf("Password verified using bcrypt")
			} else {
				// 如果bcrypt验证失败，且传入的也是加密密码，尝试直接比较
				if isEncryptedPassword(password) {
					authSuccess = subtle.ConstantTimeCompare([]byte(password), []byte(configuredPassword)) == 1
					//logger.Logger.Debugf("Comparing encrypted passwords directly")
				}
			}
		} else {
			// 如果配置的是明文密码
			if isEncryptedPassword(password) {
				// 如果传入的是加密密码，尝试用传入的密文验证配置的明文
				err := bcrypt.CompareHashAndPassword([]byte(password), []byte(configuredPassword))
				authSuccess = err == nil
				//logger.Logger.Debugf("Verifying plain password against encrypted password")
			} else {
				// 如果传入的也是明文，直接比较
				authSuccess = subtle.ConstantTimeCompare([]byte(password), []byte(configuredPassword)) == 1
				//logger.Logger.Debugf("Comparing plain passwords directly")
			}
		}

		if !authSuccess {
			w.Header().Set("WWW-Authenticate", `Basic realm="DAMENG Exporter Metrics"`)
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			w.WriteHeader(http.StatusUnauthorized)
			fmt.Fprintf(w, `
				<html>
					<head>
						<title>认证失败</title>
						<style>
							body { font-family: Arial, sans-serif; margin: 40px; }
							.error { color: #d32f2f; }
							.retry { margin-top: 20px; }
							a { color: #1976d2; text-decoration: none; }
							a:hover { text-decoration: underline; }
						</style>
					</head>
					<body>
						<h1 class="error">认证失败</h1>
						<p>用户 '%s' 的密码不正确</p>
						<div class="retry">
							<a href="%s">重新登录</a>
						</div>
					</body>
				</html>`, username, r.URL.Path)
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
