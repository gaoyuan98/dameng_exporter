package utils

import (
	"context"
	"dameng_exporter/config"
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

	timeoutSeconds := config.DefaultDataSourceConfig.QueryTimeout
	if config.GlobalMultiConfig != nil {
		if cfg := config.GlobalMultiConfig.GetDataSourceByName(dataSource); cfg != nil && cfg.QueryTimeout > 0 {
			timeoutSeconds = cfg.QueryTimeout
		}
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(timeoutSeconds)*time.Second)
	defer cancel()
	//ping一下连接是否可用
	if err := dbConn.PingContext(ctx); err != nil {
		logger.Logger.Errorf("[%s] 数据库连接不可用: %v", dataSource, err)
		if manager := db.GlobalPoolManager; manager != nil {
			manager.MarkDatasourceFailed(dataSource, err)
		}
		return err
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
