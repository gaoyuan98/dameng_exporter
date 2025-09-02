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

// DbArchSwitchRateInfo 归档切换频率信息
type DbArchSwitchRateInfo struct {
	status     sql.NullString
	createTime sql.NullString
	path       sql.NullString
	clsn       sql.NullString
	srcDbMagic sql.NullString
	minusDiff  sql.NullFloat64
}

// DbArchSwitchCollector 归档切换监控采集器
type DbArchSwitchCollector struct {
	db                       *sql.DB
	archSwitchRateDesc       *prometheus.Desc // 归档切换频率
	archSwitchRateDetailInfo *prometheus.Desc // 归档切换频率详情
	dataSource               string           // 数据源名称
}

// SetDataSource 实现DataSourceAware接口
func (c *DbArchSwitchCollector) SetDataSource(name string) {
	c.dataSource = name
}

// NewDbArchSwitchCollector 初始化归档切换监控采集器
func NewDbArchSwitchCollector(db *sql.DB) MetricCollector {
	return &DbArchSwitchCollector{
		db: db,
		archSwitchRateDesc: prometheus.NewDesc(
			dmdbms_arch_switch_rate,
			"Information about DM database archive switch rate，Always output the most recent piece of data",
			[]string{},
			nil,
		),
		archSwitchRateDetailInfo: prometheus.NewDesc(
			dmdbms_arch_switch_rate_detail_info,
			"Information about DM database archive switch rate info, return MAX_SEND_LSN - LAST_SEND_LSN = diffValue",
			[]string{"status", "createTime", "path", "clsn", "srcDbMagic"},
			nil,
		),
	}
}

func (c *DbArchSwitchCollector) Describe(ch chan<- *prometheus.Desc) {
	ch <- c.archSwitchRateDesc
	ch <- c.archSwitchRateDetailInfo
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
			c.archSwitchRateDesc,
			prometheus.GaugeValue,
			0,
		)
		return
	}

	// 查询归档切换频率
	dbArchSwitchRateInfo, err := c.getDbArchSwitchRate(ctx, c.db)
	if err != nil {
		logger.Logger.Warnf("[%s] Failed to get archive switch rate: %v", c.dataSource, err)
		return
	}

	clsn := utils.NullStringToString(dbArchSwitchRateInfo.clsn)
	srcDbMagic := utils.NullStringToString(dbArchSwitchRateInfo.srcDbMagic)
	status := utils.NullStringToString(dbArchSwitchRateInfo.status)
	path := utils.NullStringToString(dbArchSwitchRateInfo.path)
	createTime := utils.NullStringToString(dbArchSwitchRateInfo.createTime)
	minusDiff := utils.NullFloat64ToFloat64(dbArchSwitchRateInfo.minusDiff)

	// 归档切换频率指标（用于折线图）
	ch <- prometheus.MustNewConstMetric(
		c.archSwitchRateDesc,
		prometheus.GaugeValue,
		minusDiff,
	)

	// 归档切换详细信息
	ch <- prometheus.MustNewConstMetric(
		c.archSwitchRateDetailInfo,
		prometheus.GaugeValue,
		minusDiff,
		status, createTime, path, clsn, srcDbMagic,
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

// getDbArchSwitchRate 查询归档切换频率
func (c *DbArchSwitchCollector) getDbArchSwitchRate(ctx context.Context, db *sql.DB) (DbArchSwitchRateInfo, error) {
	var dbArchSwitchRateInfo DbArchSwitchRateInfo

	rows, err := db.QueryContext(ctx, config.QueryArchiveSwitchRateSql)
	if err != nil {
		utils.HandleDbQueryErrorWithSource(err, c.dataSource)
		return dbArchSwitchRateInfo, err
	}
	defer rows.Close()

	if rows.Next() {
		if err := rows.Scan(&dbArchSwitchRateInfo.status, &dbArchSwitchRateInfo.createTime,
			&dbArchSwitchRateInfo.path, &dbArchSwitchRateInfo.clsn,
			&dbArchSwitchRateInfo.srcDbMagic, &dbArchSwitchRateInfo.minusDiff); err != nil {
			logger.Logger.Error(fmt.Sprintf("[%s] Error scanning row", c.dataSource), zap.Error(err))
			return dbArchSwitchRateInfo, err
		}
	}

	return dbArchSwitchRateInfo, nil
}
