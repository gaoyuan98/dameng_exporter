package collector

import (
	"context"
	"dameng_exporter/config"
	dmdb "dameng_exporter/db"
	"dameng_exporter/logger"
	"dameng_exporter/utils"
	"database/sql"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"go.uber.org/zap"
)

// DbArchSendDetailInfo 表示单个归档目的地的发送详情。
type DbArchSendDetailInfo struct {
	archDest sql.NullString
	archType sql.NullString
	lsnDiff  sql.NullFloat64
	// lastSendCode 对应 V$ARCH_SEND_INFO.LAST_SEND_CODE，用于归档同步异常告警。
	lastSendCode sql.NullFloat64
}

// DbArchSendCollector 负责采集归档发送差值和发送返回码。
type DbArchSendCollector struct {
	db                 *sql.DB
	archSendDetailInfo *prometheus.Desc
	archSendDiffValue  *prometheus.Desc
	// archSendLastCode 暴露 LAST_SEND_CODE 原始返回码。
	archSendLastCode *prometheus.Desc
	dataSource       string
	nodeType         string

	archSendFieldsCheckOnce sync.Once
	archSendFieldsExist     bool
	archApplyInfoCheckOnce  sync.Once
	archApplyInfoExists     bool
}

// SetDataSource 实现 DataSourceAware 接口。
func (c *DbArchSendCollector) SetDataSource(name string) {
	c.dataSource = name
}

// SetNodeType 实现 NodeTypeAware 接口。
func (c *DbArchSendCollector) SetNodeType(nodeType string) {
	c.nodeType = nodeType
}

// NewDbArchSendCollector 初始化归档发送采集器。
func NewDbArchSendCollector(db *sql.DB) MetricCollector {
	return &DbArchSendCollector{
		db: db,
		archSendDetailInfo: prometheus.NewDesc(
			dmdbms_arch_send_detail_info,
			"Information about DM database archive send detail info, return MAX_SEND_LSN - LAST_SEND_LSN = diffValue",
			[]string{"arch_type", "arch_dest"},
			nil,
		),
		archSendDiffValue: prometheus.NewDesc(
			dmdbms_arch_send_diff_value,
			"Information about DM database archive send diff value, return MAX_SEND_LSN - LAST_SEND_LSN = diffValue",
			[]string{"arch_type", "arch_dest"},
			nil,
		),
		archSendLastCode: prometheus.NewDesc(
			dmdbms_arch_send_last_code,
			"Information about DM database archive send last code, return LAST_SEND_CODE",
			[]string{"arch_type", "arch_dest"},
			nil,
		),
	}
}

func (c *DbArchSendCollector) Describe(ch chan<- *prometheus.Desc) {
	ch <- c.archSendDetailInfo
	ch <- c.archSendDiffValue
	ch <- c.archSendLastCode
}

func (c *DbArchSendCollector) Collect(ch chan<- prometheus.Metric) {
	if err := utils.CheckDBConnectionWithSource(c.db, c.dataSource); err != nil {
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(config.Global.GetQueryTimeout())*time.Second)
	defer cancel()

	// 这里只判断归档是否已开启，不要求本地状态必须为 VALID。
	if !c.isArchiveConfigured(ctx) {
		return
	}

	dbArchSendInfos, err := c.getDbArchSendDetailInfo(ctx, c.db)
	if err != nil {
		logger.Logger.Error(fmt.Sprintf("[%s] exec getDbArchSendDetailInfo func error", c.dataSource), zap.Error(err))
		return
	}

	for _, dbArchSendInfo := range dbArchSendInfos {
		archType := utils.NullStringToString(dbArchSendInfo.archType)
		archDest := utils.NullStringToString(dbArchSendInfo.archDest)
		lsnDiff := utils.NullFloat64ToFloat64(dbArchSendInfo.lsnDiff)

		ch <- prometheus.MustNewConstMetric(
			c.archSendDetailInfo,
			prometheus.GaugeValue,
			lsnDiff,
			archType, archDest,
		)

		// LSN差值指标（简化版，用于监控延迟）
		ch <- prometheus.MustNewConstMetric(
			c.archSendDiffValue,
			prometheus.GaugeValue,
			lsnDiff,
			archType, archDest,
		)

		// 仅在字段存在且值有效时输出 LAST_SEND_CODE。
		if dbArchSendInfo.lastSendCode.Valid {
			ch <- prometheus.MustNewConstMetric(
				c.archSendLastCode,
				prometheus.GaugeValue,
				dbArchSendInfo.lastSendCode.Float64,
				archType, archDest,
			)
		}
	}
}

// isArchiveConfigured 仅检查是否开启归档配置，不再要求状态必须为 VALID。
func (c *DbArchSendCollector) isArchiveConfigured(ctx context.Context) bool {
	var paraValue string
	query := `SELECT /*+DMDB_CHECK_FLAG*/ PARA_VALUE FROM v$dm_ini WHERE para_name='ARCH_INI'`
	err := c.db.QueryRowContext(ctx, query).Scan(&paraValue)
	if err != nil {
		logger.Logger.Debugf("[%s] Failed to check archive status: %v", c.dataSource, err)
		return false
	}

	return paraValue == "1"
}

// checkArchSendInfoFields 检查发送视图是否包含 LAST_SEND_CODE/LAST_SEND_DESC 字段。
func (c *DbArchSendCollector) checkArchSendInfoFields(ctx context.Context) bool {
	c.archSendFieldsCheckOnce.Do(func() {
		var count int
		if err := c.db.QueryRowContext(ctx, config.QueryArchSendInfoFieldsExist).Scan(&count); err != nil {
			logger.Logger.Warnf("[%s] Failed to check V$ARCH_SEND_INFO fields existence: %v", c.dataSource, err)
			c.archSendFieldsExist = false
			return
		}
		c.archSendFieldsExist = count == 2
		logger.Logger.Debugf("[%s] V$ARCH_SEND_INFO fields exist: %v (LAST_SEND_CODE, LAST_SEND_DESC)", c.dataSource, c.archSendFieldsExist)
	})
	return c.archSendFieldsExist
}

// checkArchApplyInfoExists 检查 V$ARCH_APPLY_INFO 视图是否存在。
func (c *DbArchSendCollector) checkArchApplyInfoExists(ctx context.Context) bool {
	c.archApplyInfoCheckOnce.Do(func() {
		var count int
		if err := c.db.QueryRowContext(ctx, config.QueryArchApplyInfoExists).Scan(&count); err != nil {
			logger.Logger.Warnf("[%s] V$ARCH_APPLY_INFO not accessible: %v", c.dataSource, err)
			c.archApplyInfoExists = false
			return
		}
		c.archApplyInfoExists = count == 1
		logger.Logger.Debugf("[%s] V$ARCH_APPLY_INFO exists: %v", c.dataSource, c.archApplyInfoExists)
	})
	return c.archApplyInfoExists
}

// getDbArchSendDetailInfo 查询所有归档目的地的发送详情。
func (c *DbArchSendCollector) getDbArchSendDetailInfo(ctx context.Context, db *sql.DB) ([]DbArchSendDetailInfo, error) {
	var querySql string
	// 记录当前版本是否支持 LAST_SEND_CODE/LAST_SEND_DESC 字段。
	hasLastSendFields := c.checkArchSendInfoFields(ctx)
	if c.checkArchApplyInfoExists(ctx) {
		if c.nodeType == string(dmdb.NodeTypeDSC) {
			querySql = config.QueryArchSendDetailInfoDsc
		} else {
			querySql = config.QueryArchSendDetailInfo2
		}
	} else {
		querySql = config.QueryArchSendDetailInfo
	}

	// 老版本缺少字段时，用空字符串占位并跳过返回码指标输出。
	if !hasLastSendFields {
		querySql = strings.ReplaceAll(querySql, "LAST_SEND_CODE,", "'' AS LAST_SEND_CODE,")
		querySql = strings.ReplaceAll(querySql, "LAST_SEND_DESC,", "'' AS LAST_SEND_DESC,")
	}

	var dbArchSendDetailInfos []DbArchSendDetailInfo
	rows, err := db.QueryContext(ctx, querySql)
	if err != nil {
		utils.HandleDbQueryErrorWithSource(err, c.dataSource)
		return dbArchSendDetailInfos, err
	}
	defer rows.Close()

	for rows.Next() {
		var dbArchSendDetailInfo DbArchSendDetailInfo
		var lastSendCodeText, lastSendDesc, lastStartTime, lastEndTime, lastSendTime sql.NullString
		if err := rows.Scan(&dbArchSendDetailInfo.archDest, &dbArchSendDetailInfo.archType,
			&dbArchSendDetailInfo.lsnDiff, &lastSendCodeText,
			&lastSendDesc, &lastStartTime,
			&lastEndTime, &lastSendTime); err != nil {
			logger.Logger.Error(fmt.Sprintf("[%s] Error scanning row", c.dataSource), zap.Error(err))
			continue
		}

		// 仅在字段存在且值非空时解析 LAST_SEND_CODE。
		if hasLastSendFields && lastSendCodeText.Valid {
			if lastSendCode, err := strconv.ParseFloat(lastSendCodeText.String, 64); err == nil {
				dbArchSendDetailInfo.lastSendCode = sql.NullFloat64{Float64: lastSendCode, Valid: true}
			} else {
				logger.Logger.Warn(fmt.Sprintf("[%s] Failed to parse LAST_SEND_CODE: %s", c.dataSource, lastSendCodeText.String), zap.Error(err))
			}
		}

		dbArchSendDetailInfos = append(dbArchSendDetailInfos, dbArchSendDetailInfo)
	}

	if err := rows.Err(); err != nil {
		logger.Logger.Error(fmt.Sprintf("[%s] Error with rows", c.dataSource), zap.Error(err))
	}

	return dbArchSendDetailInfos, nil
}
