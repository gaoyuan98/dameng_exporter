package utils

import (
	"context"
	"dameng_exporter/logger"
	"database/sql"
	"errors"
	"fmt"
	"go.uber.org/zap"
	"time"
)

// 统一的数据库连接检查（带数据源标识）
func CheckDBConnectionWithSource(db *sql.DB, dataSource string) error {
	if err := db.Ping(); err != nil {
		logger.Logger.Errorf("[%s] Database connection is not available: %v", dataSource, err)
		return err
	}
	return nil
}

// 封装通用的错误处理逻辑（带数据源标识）
func HandleDbQueryErrorWithSource(err error, dataSource string) {
	if errors.Is(err, context.DeadlineExceeded) {
		logger.Logger.Errorf("[%s] Query timed out: %v", dataSource, err)
	} else {
		logger.Logger.Errorf("[%s] Error querying database: %v", dataSource, err)
	}
}

// 封装通用的错误处理逻辑（保留以向后兼容，后续将逐步废弃）
func HandleDbQueryError(err error) {
	if errors.Is(err, context.DeadlineExceeded) {
		logger.Logger.Error("Query timed out", zap.Error(err))
	} else {
		logger.Logger.Error("Error querying database", zap.Error(err))
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
