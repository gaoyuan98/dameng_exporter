package utils

import (
	"context"
	"dameng_exporter/db"
	"dameng_exporter/logger"
	"database/sql"
	"errors"
	"fmt"
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
