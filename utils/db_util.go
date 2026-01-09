package utils

import (
	"context"
	"dameng_exporter/config"
	"dameng_exporter/db"
	"dameng_exporter/logger"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"
)

// 统一的数据库连接检查（带数据源标识）
func CheckDBConnectionWithSource(dbConn *sql.DB, dataSource string) error {
	if dbConn == nil {
		err := fmt.Errorf("数据库连接未初始化")
		logger.Logger.Errorf("[%s] 数据库连接未初始化，无法执行检查", dataSource)
		if manager := db.GlobalPoolManager; manager != nil {
			manager.MarkDatasourceFailed(dataSource, err)
		}
		return err
	}

	manager := db.GlobalPoolManager
	if manager == nil {
		return nil
	}

	status := manager.GetDatasourceHealthStatus(dataSource)
	if !status.Registered {
		err := fmt.Errorf("数据源[%s]未注册或已禁用", dataSource)
		logger.Logger.Warn(err.Error())
		return err
	}

	if !status.Healthy {
		lastCheck := "未知"
		if !status.LastCheck.IsZero() {
			lastCheck = status.LastCheck.Format(time.DateTime)
		}
		if status.LastError != "" {
			logger.Logger.Warnf("[%s] 数据源处于不可用状态，最近检查: %s，最近错误: %s", dataSource, lastCheck, status.LastError)
		} else {
			logger.Logger.Warnf("[%s] 数据源处于不可用状态，最近检查: %s", dataSource, lastCheck)
		}
		return fmt.Errorf("数据源[%s]当前不可用", dataSource)
	}

	return nil
}

// 封装通用的错误处理逻辑（带数据源标识）
func HandleDbQueryErrorWithSource(err error, dataSource string) {
	if errors.Is(err, context.DeadlineExceeded) {
		logger.Logger.Errorf("[%s] 查询超时: %v", dataSource, err)
	} else {
		logger.Logger.Errorf("[%s] 查询数据库时发生错误: %v", dataSource, err)
	}
	if dataSource == "" || err == nil {
		return
	}

	// 针对网络故障等致命错误直接触发降级，避免所有采集器在同一轮内重复阻塞
	if shouldForceDegrade(err) {
		triggerFastDegrade(dataSource, err)
		return
	}

	triggerHealthCheckOnError(dataSource)
}

// triggerHealthCheckOnError 在禁用周期探活时补充一次点对点健康检查
func triggerHealthCheckOnError(dataSource string) {
	if dataSource == "" || config.Global.GetEnableHealthPing() {
		return
	}

	manager := db.GlobalPoolManager
	if manager == nil {
		return
	}

	pool := manager.GetPool(dataSource)
	if pool == nil || pool.DB == nil || pool.Config == nil {
		return
	}

	timeoutSeconds := pool.Config.QueryTimeout
	if timeoutSeconds <= 0 {
		timeoutSeconds = config.DefaultDataSourceConfig.QueryTimeout
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(timeoutSeconds)*time.Second)
	defer cancel()

	if err := pool.DB.PingContext(ctx); err != nil {
		logger.Logger.Warnf("[%s] 确认性健康检查失败，标记数据源不可用: %v", dataSource, err)
		manager.MarkDatasourceFailed(dataSource, err)
	}
}

// shouldForceDegrade 判断错误是否足以触发立即降级
func shouldForceDegrade(err error) bool {
	if err == nil {
		return false
	}

	if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, sql.ErrConnDone) {
		return true
	}

	message := err.Error()
	lowerMessage := strings.ToLower(message)
	if containsAny(message, []string{
		"网络通信异常",
	}) {
		return true
	}
	if containsAny(lowerMessage, []string{
		"error 6001",
		"database is closed",
		"bad connection",
	}) {
		return true
	}

	return false
}

// triggerFastDegrade 在确认数据源仍处于健康状态时，直接标记为失败
func triggerFastDegrade(dataSource string, reason error) {
	if dataSource == "" {
		return
	}

	manager := db.GlobalPoolManager
	if manager == nil {
		return
	}

	status := manager.GetDatasourceHealthStatus(dataSource)
	if status.Registered && !status.Healthy {
		return
	}

	manager.MarkDatasourceFailed(dataSource, reason)
}

// containsAny 判断字符串是否包含关键字列表中的任意一个
func containsAny(source string, keywords []string) bool {
	if source == "" || len(keywords) == 0 {
		return false
	}

	for _, key := range keywords {
		if key != "" && strings.Contains(source, key) {
			return true
		}
	}
	return false
}

func NullStringToString(ns sql.NullString) string {
	if ns.Valid {
		return ns.String
	}
	return ""
}

func NullFloat64ToFloat64(nf sql.NullFloat64) float64 {
	if nf.Valid {
		return nf.Float64
	}
	return 0
}
func NullInt64ToFloat64(n sql.NullInt64) float64 {
	if n.Valid {
		return float64(n.Int64)
	}
	return 0
}

// 辅助函数，将 sql.NullTime 转换为 string
func NullTimeToString(n sql.NullTime) string {
	if n.Valid {
		return n.Time.Format(time.DateTime)
	}
	return ""
}

// 辅助函数，将 sql.NullFloat64 转换为 string
func NullFloat64ToString(n sql.NullFloat64) string {
	if n.Valid {
		return fmt.Sprintf("%f", n.Float64)
	}
	return "0"
}

// NullStringTimeToUnixSeconds 将时间字符串（YYYY-MM-DD HH24:MI:SS）转换为 Unix 秒
func NullStringTimeToUnixSeconds(n sql.NullString) (float64, error) {
	if !n.Valid {
		return 0, nil
	}

	value := n.String
	if value == "" {
		return 0, nil
	}

	parsedTime, err := time.ParseInLocation("2006-01-02 15:04:05", value, time.Local)
	if err != nil {
		return 0, err
	}

	return float64(parsedTime.Unix()), nil
}
