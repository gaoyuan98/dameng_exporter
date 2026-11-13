package db

import (
	"context"
	"dameng_exporter/config"
	"dameng_exporter/logger"
	"database/sql"
	"time"
)

// NodeType 数据库节点类型
type NodeType string

const (
	NodeTypeDefault NodeType = "DEFAULT"
	NodeTypeDW      NodeType = "DW"
	NodeTypeDSC     NodeType = "DSC"
	NodeTypeDPC     NodeType = "DPC"
)

// DetectNodeType 识别数据库节点类型（DW > DSC > DPC > DEFAULT）
func DetectNodeType(db *sql.DB, timeoutSeconds int, dataSource string) NodeType {
	if db == nil {
		return NodeTypeDefault
	}
	if timeoutSeconds <= 0 {
		timeoutSeconds = config.DefaultDataSourceConfig.QueryTimeout
	}

	start := time.Now()

	if ok, err := queryCountGreaterThanZero(db, timeoutSeconds, config.QueryCheckDwNodeTypeSql); err == nil {
		if ok {
			logNodeTypeDetected(dataSource, NodeTypeDW, time.Since(start))
			return NodeTypeDW
		}
	} else {
		logger.Logger.Warnf("[%s] DW节点检测失败: %v", dataSource, err)
	}

	if ok, err := queryCountGreaterThanZero(db, timeoutSeconds, config.QueryCheckDscNodeTypeSql); err == nil {
		if ok {
			logNodeTypeDetected(dataSource, NodeTypeDSC, time.Since(start))
			return NodeTypeDSC
		}
	} else {
		logger.Logger.Warnf("[%s] DSC节点检测失败: %v", dataSource, err)
	}
	/*
		if ok, err := queryCountGreaterThanZero(db, timeoutSeconds, config.QueryCheckDpcNodeTypeSql); err == nil {
			if ok {
				logNodeTypeDetected(dataSource, NodeTypeDPC, time.Since(start))
				return NodeTypeDPC
			}
		} else {
			logger.Logger.Warnf("[%s] DPC节点检测失败: %v", dataSource, err)
		}*/

	logNodeTypeDetected(dataSource, NodeTypeDefault, time.Since(start))
	return NodeTypeDefault
}

func queryCountGreaterThanZero(db *sql.DB, timeoutSeconds int, query string) (bool, error) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(timeoutSeconds)*time.Second)
	defer cancel()

	var count int
	if err := db.QueryRowContext(ctx, query).Scan(&count); err != nil {
		return false, err
	}
	return count > 0, nil
}

func logNodeTypeDetected(dataSource string, nodeType NodeType, cost time.Duration) {
	logger.Logger.Debugf("[%s] 数据库节点类型: %s (耗时 %dms)", dataSource, nodeType, cost.Milliseconds())
}
