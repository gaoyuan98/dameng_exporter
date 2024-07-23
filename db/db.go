package db

import (
	"context"
	"dameng_exporter/config"
	"dameng_exporter/logger"
	"database/sql"
	"fmt"
	"go.uber.org/zap"
	"log"
	"time"

	_ "gitee.com/chunanyong/dm"
)

var (
	DBPool *sql.DB
	dsn    string
)

// InitDBPool initializes the database connection pool
func InitDBPool(dsnStr string) error {
	var err error
	dsn = dsnStr
	//DBPool, err = sql.Open("godror", dsn)
	//"dm://SYSDBA:SYSDBA@localhost:5236?autoCommit=true"
	DBPool, err = sql.Open("dm", dsnStr)
	if err != nil {
		logger.Logger.Error("failed to open database: %v", zap.Error(err))
		return fmt.Errorf("failed to open database: %v", err)
	}

	// Set the maximum number of open connections
	DBPool.SetMaxOpenConns(config.GlobalConfig.MaxOpenConns)
	// Set the maximum number of idle connections
	DBPool.SetMaxIdleConns(config.GlobalConfig.MaxIdleConns)
	// Set the maximum lifetime of each connection
	DBPool.SetConnMaxLifetime(time.Duration(config.GlobalConfig.ConnMaxLifetime) * time.Minute)

	// Test the database connection
	err = DBPool.Ping()
	if err != nil {
		return fmt.Errorf("failed to ping database: %v", err)
	}

	return nil
}

// CloseDBPool closes the database connection pool
func CloseDBPool() {
	if DBPool != nil {
		DBPool.Close()
	}
}

/*说明
重连机制：reconnect 函数尝试重新建立数据库连接，如果连接失败则在2秒后重试，最多重试5次。如果所有尝试都失败，则返回错误。
重试查询：在 QueryData 函数中，如果首次查询失败且数据库连接丢失，会尝试重新连接并重试查询一次。
连接超时：使用 context 包来设置连接超时。调用 QueryContext 方法时传入上下文，以确保在指定时间内完成查询。*/

func reconnect() error {
	for attempts := 0; attempts < 5; attempts++ {
		log.Printf("Attempting to reconnect to the database, attempt %d", attempts+1)
		err := InitDBPool(dsn)
		if err == nil {
			log.Println("Reconnected to the database successfully")
			return nil
		}
		log.Printf("Reconnect attempt %d failed: %v", attempts+1, err)
		time.Sleep(2 * time.Second) // Wait for 2 seconds before the next attempt
	}
	return fmt.Errorf("failed to reconnect to the database after 5 attempts")
}

// QueryData executes a SELECT query and returns the results
func QueryData(ctx context.Context, db *sql.DB, query string, args ...interface{}) ([]map[string]interface{}, error) {
	for attempts := 0; attempts < 2; attempts++ {
		err := db.Ping()
		if err != nil {
			log.Println("Database connection lost, attempting to reconnect")
			reconnectErr := reconnect()
			if reconnectErr != nil {
				return nil, fmt.Errorf("failed to reconnect to the database: %v", reconnectErr)
			}
		}

		rows, err := db.QueryContext(ctx, query, args...)
		if err != nil {
			if attempts == 0 {
				log.Printf("Query failed: %v. Retrying...", err)
				continue
			}
			return nil, fmt.Errorf("failed to execute query: %v", err)
		}
		defer rows.Close()

		columns, err := rows.Columns()
		if err != nil {
			return nil, fmt.Errorf("failed to get columns: %v", err)
		}

		var results []map[string]interface{}
		for rows.Next() {
			rowData := make(map[string]interface{})
			columnPointers := make([]interface{}, len(columns))
			for i := range columns {
				columnPointers[i] = new(interface{})
			}

			if err := rows.Scan(columnPointers...); err != nil {
				return nil, fmt.Errorf("failed to scan row: %v", err)
			}

			for i, colName := range columns {
				val := columnPointers[i].(*interface{})
				rowData[colName] = *val
			}

			results = append(results, rowData)
		}

		return results, nil
	}
	return nil, fmt.Errorf("query failed after 2 attempts")
}

//
//// QueryData executes a SELECT query and returns the results
//func QueryData(ctx context.Context, query string, args ...interface{}) ([]map[string]interface{}, error) {
//	rows, err := DBPool.QueryContext(ctx, query, args...)
//	if err != nil {
//		return nil, fmt.Errorf("failed to execute query: %v", err)
//	}
//	defer rows.Close()
//
//	columns, err := rows.Columns()
//	if err != nil {
//		return nil, fmt.Errorf("failed to get columns: %v", err)
//	}
//
//	var results []map[string]interface{}
//	for rows.Next() {
//		rowData := make(map[string]interface{})
//		columnPointers := make([]interface{}, len(columns))
//		for i := range columns {
//			columnPointers[i] = new(interface{})
//		}
//
//		if err := rows.Scan(columnPointers...); err != nil {
//			return nil, fmt.Errorf("failed to scan row: %v", err)
//		}
//
//		for i, colName := range columns {
//			val := columnPointers[i].(*interface{})
//			rowData[colName] = *val
//		}
//
//		results = append(results, rowData)
//	}
//
//	return results, nil
//}
