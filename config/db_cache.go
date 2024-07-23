package config

import (
	"github.com/patrickmn/go-cache"
	"time"
)

// 全局缓存对象
var c *cache.Cache

// 初始化缓存，设置默认过期时间和清理间隔时间
func init() {
	//并设置默认过期时间为5分钟，清理间隔时间为10分钟。
	c = cache.New(5*time.Minute, 30*time.Minute)
}

// 从缓存中获取数据
func GetFromCache(query string) (string, bool) {
	if value, ok := c.Get(query); ok {
		return value.(string), true
	}
	return "", false
}

// 将数据存入缓存
func SetCache(query string, value string, duration time.Duration) {
	c.Set(query, value, duration)
}

// 删除缓存中的数据
func DeleteFromCache(query string) {
	c.Delete(query)
}
func GetKeyExists(key string) bool {
	_, found := c.Get(key)
	return found
}

/*// 示例主函数
func main() {
	// 设置缓存
	SetCache("key1", "value1", 2*time.Minute)

	// 获取缓存
	if value, ok := GetFromCache("key1"); ok {
		fmt.Println("Got from cache:", value)
	} else {
		fmt.Println("Cache miss for key1")
	}

	// 删除缓存
	DeleteFromCache("key1")

	// 尝试获取已删除的缓存
	if value, ok := GetFromCache("key1"); ok {
		fmt.Println("Got from cache:", value)
	} else {
		fmt.Println("Cache miss for key1")
	}
}*/
