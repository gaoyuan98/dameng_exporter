package collector

import (
	"context"
	"dameng_exporter/config"
	"dameng_exporter/logger"
	"dameng_exporter/utils"
	"database/sql"
	"fmt"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"go.uber.org/zap"
)

// DbArchLatestCreateTimeInfo 最新归档创建时间信息
type DbArchLatestCreateTimeInfo struct {
	createTime sql.NullString
}

// DbArchSwitchCollector 归档切换监控采集器
type DbArchSwitchCollector struct {
	db                     *sql.DB
	archLastCreateTimeDesc *prometheus.Desc // 最新归档创建时间
	dataSource             string           // 数据源名称
}

// SetDataSource 实现DataSourceAware接口
func (c *DbArchSwitchCollector) SetDataSource(name string) {
	c.dataSource = name
}

// NewDbArchSwitchCollector 初始化归档切换监控采集器
func NewDbArchSwitchCollector(db *sql.DB) MetricCollector {
	return &DbArchSwitchCollector{
		db: db,
		archLastCreateTimeDesc: prometheus.NewDesc(
			dmdbms_arch_last_create_time_seconds,
			"Latest archive log creation time in Unix seconds; zero when archive logs are unavailable",
			[]string{},
			nil,
		),
	}
}

func (c *DbArchSwitchCollector) Describe(ch chan<- *prometheus.Desc) {
	ch <- c.archLastCreateTimeDesc
}

func (c *DbArchSwitchCollector) Collect(ch chan<- prometheus.Metric) {
	if err := utils.CheckDBConnectionWithSource(c.db, c.dataSource); err != nil {
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(config.Global.GetQueryTimeout())*time.Second)
	defer cancel()

	// 快速检查归档是否开启
	if !c.isArchiveEnabled(ctx) {
		// 归档未开启时返回默认值0
		ch <- prometheus.MustNewConstMetric(
			c.archLastCreateTimeDesc,
			prometheus.GaugeValue,
			0,
		)
		return
	}

	// 查询最新归档创建时间
	dbArchLatestCreateTimeInfo, err := c.getLatestArchCreateTime(ctx, c.db)
	if err != nil {
		logger.Logger.Warnf("[%s] Failed to get latest archive create time: %v", c.dataSource, err)
		return
	}

	lastCreateTime, err := utils.NullStringTimeToUnixSeconds(dbArchLatestCreateTimeInfo.createTime)
	if err != nil {
		logger.Logger.Warnf("[%s] Failed to parse archive create time %q: %v", c.dataSource, utils.NullStringToString(dbArchLatestCreateTimeInfo.createTime), err)
		lastCreateTime = 0
	}
	// 最新归档创建时间指标
	ch <- prometheus.MustNewConstMetric(
		c.archLastCreateTimeDesc,
		prometheus.GaugeValue,
		lastCreateTime,
	)
}

// isArchiveEnabled 快速检查归档是否开启
func (c *DbArchSwitchCollector) isArchiveEnabled(ctx context.Context) bool {
	var paraValue string
	query := `SELECT /*+DMDB_CHECK_FLAG*/ PARA_VALUE FROM v$dm_ini WHERE para_name='ARCH_INI'`
	err := c.db.QueryRowContext(ctx, query).Scan(&paraValue)
	if err != nil {
		logger.Logger.Debugf("[%s] Failed to check archive status: %v", c.dataSource, err)
		return false
	}

	if paraValue != "1" {
		return false
	}

	// 进一步检查归档状态是否VALID
	var archStatus string
	query = `SELECT /*+DMDB_CHECK_FLAG*/ CASE arch_status WHEN 'VALID' THEN '1' WHEN 'INVALID' THEN '0' END FROM v$arch_status WHERE arch_type='LOCAL'`
	err = c.db.QueryRowContext(ctx, query).Scan(&archStatus)
	if err != nil {
		logger.Logger.Debugf("[%s] Failed to check archive validity: %v", c.dataSource, err)
		return false
	}

	return archStatus == "1"
}

// getLatestArchCreateTime 查询最新归档创建时间
func (c *DbArchSwitchCollector) getLatestArchCreateTime(ctx context.Context, db *sql.DB) (DbArchLatestCreateTimeInfo, error) {
	var dbArchLatestCreateTimeInfo DbArchLatestCreateTimeInfo

	rows, err := db.QueryContext(ctx, config.QueryArchiveLatestCreateTimeSql)
	if err != nil {
		utils.HandleDbQueryErrorWithSource(err, c.dataSource)
		return dbArchLatestCreateTimeInfo, err
	}
	defer rows.Close()

	if rows.Next() {
		if err := rows.Scan(&dbArchLatestCreateTimeInfo.createTime); err != nil {
			logger.Logger.Error(fmt.Sprintf("[%s] Error scanning row", c.dataSource), zap.Error(err))
			return dbArchLatestCreateTimeInfo, err
		}
	}

	return dbArchLatestCreateTimeInfo, nil
}
