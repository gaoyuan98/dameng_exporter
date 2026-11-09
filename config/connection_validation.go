package config

import (
	"fmt"
	"strings"
	"unicode/utf8"
)

// ValidateConnection 在创建单个连接池前校验当前 dm 驱动的限制及本项目的凭据配置规则。
// 密码在调用前已经解密；这里不改变凭据，也不在错误中输出其内容。
func (ds *DataSourceConfig) ValidateConnection() error {
	if strings.Contains(ds.DbUser, ":") {
		return fmt.Errorf("dbUser 不能包含冒号：当前达梦驱动使用第一个冒号分隔用户名和密码")
	}
	for _, credential := range []struct{ field, value string }{
		{"dbUser", ds.DbUser},
		{"dbPwd", ds.DbPwd},
	} {
		if !utf8.ValidString(credential.value) {
			return fmt.Errorf("%s 必须是有效的 UTF-8 文本", credential.field)
		}
		// 每个字符编码后至少占一个字节；非 ASCII 的实际字节数由数据库字符集决定。
		if utf8.RuneCountInString(credential.value) > 128 {
			return fmt.Errorf("%s 超出当前达梦驱动的 128 字节上限，请缩短凭据", credential.field)
		}
	}
	_, query, hasQuery := strings.Cut(ds.DbHost, "?")
	if hasQuery {
		if strings.Contains(query, "?") {
			return fmt.Errorf("dbHost 查询参数不能包含额外的问号：当前达梦驱动使用最后一个问号分隔参数")
		}
		for _, param := range strings.Split(query, "&") {
			key, _, _ := strings.Cut(param, "=")
			if strings.EqualFold(key, "user") || strings.EqualFold(key, "password") {
				return fmt.Errorf("dbHost 不能通过 user/password 参数覆盖凭据，请使用 dbUser 和 dbPwd")
			}
		}
	}
	return nil
}
