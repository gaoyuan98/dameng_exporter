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

// 定义常量
const (
	DB_ARCH_NO_ENABLE = -1
	DB_ARCH_VALID     = 1
	DB_ARCH_INVALID   = 2
)

// DbArchStatusInfo 归档状态信息
type DbArchStatusInfo struct {
	archType   sql.NullString
	archDest   sql.NullString
	archSrc    sql.NullString
	archStatus sql.NullFloat64
}

// DbArchStatusCollector 归档基础状态采集器
type DbArchStatusCollector struct {
	db             *sql.DB
	archStatusDesc *prometheus.Desc // 归档状态(本地)
	archStatusInfo *prometheus.Desc // 归档所有状态
	dataSource     string           // 数据源名称
}

// SetDataSource 实现DataSourceAware接口
func (c *DbArchStatusCollector) SetDataSource(name string) {
	c.dataSource = name
}

// NewDbArchStatusCollector 初始化归档状态采集器
func NewDbArchStatusCollector(db *sql.DB) MetricCollector {
	return &DbArchStatusCollector{
		db: db,
		archStatusDesc: prometheus.NewDesc(
			dmdbms_arch_status,
			"Information about DM database archive status, value info: vaild = 1,invaild = 2,no_enable= -1",
			[]string{},
			nil,
		),
		archStatusInfo: prometheus.NewDesc(
			dmdbms_arch_status_info,
			"Information about DM database archive status, value info: vaild = 1,invaild = 0",
			[]string{"arch_type", "arch_dest", "arch_src"},
			nil,
		),
	}
}

func (c *DbArchStatusCollector) Describe(ch chan<- *prometheus.Desc) {
	ch <- c.archStatusDesc
	ch <- c.archStatusInfo
}

func (c *DbArchStatusCollector) Collect(ch chan<- prometheus.Metric) {
	if err := utils.CheckDBConnectionWithSource(c.db, c.dataSource); err != nil {
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(config.Global.GetQueryTimeout())*time.Second)
	defer cancel()

	// 获取数据库归档状态信息
	dbArchStatus, err := c.getDbArchStatus(ctx, c.db)
	if err != nil {
		logger.Logger.Error(fmt.Sprintf("[%s] exec getDbArchStatus func error", c.dataSource), zap.Error(err))
		setArchMetric(ch, c.archStatusDesc, DB_ARCH_INVALID)
		return
	}

	// 发送归档状态指标
	setArchMetric(ch, c.archStatusDesc, dbArchStatus)

	// 如果归档开启，查询所有归档的状态信息
	if dbArchStatus == DB_ARCH_VALID {
		dbArchStatusInfos, err := c.getDbArchStatusInfo(ctx, c.db)
		if err != nil {
			logger.Logger.Error(fmt.Sprintf("[%s] exec getDbArchStatusInfo func error", c.dataSource), zap.Error(err))
			return
		}

		for _, dbArchStatusInfo := range dbArchStatusInfos {
			archType := utils.NullStringToString(dbArchStatusInfo.archType)
			archDest := utils.NullStringToString(dbArchStatusInfo.archDest)
			archSrc := utils.NullStringToString(dbArchStatusInfo.archSrc)
			archStatus := utils.NullFloat64ToFloat64(dbArchStatusInfo.archStatus)

			ch <- prometheus.MustNewConstMetric(
				c.archStatusInfo,
				prometheus.GaugeValue,
				archStatus,
				archType, archDest, archSrc,
			)
		}
	}
}

func setArchMetric(ch chan<- prometheus.Metric, desc *prometheus.Desc, value int) {
	ch <- prometheus.MustNewConstMetric(
		desc,
		prometheus.GaugeValue,
		float64(value),
	)
}

// getDbArchStatus 获取数据库归档状态信息
func (c *DbArchStatusCollector) getDbArchStatus(ctx context.Context, db *sql.DB) (int, error) {
	var dbArchStatus string

	// 查询 PARA_VALUE
	query := `select /*+DMDB_CHECK_FLAG*/ PARA_VALUE from v$dm_ini where para_name='ARCH_INI'`
	row := db.QueryRowContext(ctx, query)
	err := row.Scan(&dbArchStatus)
	if err != nil {
		return DB_ARCH_INVALID, fmt.Errorf("query error: %v", err)
	}

	// 处理 PARA_VALUE 为 '1' 的情况
	if dbArchStatus == "1" {
		query = `select /*+DMDB_CHECK_FLAG*/ case arch_status when 'VALID' then 1 when 'INVALID' then 0 end ARCH_STATUS from v$arch_status where arch_type='LOCAL'`
		row = db.QueryRowContext(ctx, query)
		err = row.Scan(&dbArchStatus)
		if err != nil {
			return DB_ARCH_INVALID, fmt.Errorf("query error: %v", err)
		}
		if dbArchStatus == "1" {
			return DB_ARCH_VALID, nil
		} else if dbArchStatus == "0" {
			return DB_ARCH_INVALID, nil
		}
	} else if dbArchStatus == "0" {
		return DB_ARCH_NO_ENABLE, nil
	}

	logger.Logger.Infof("[%s] Check Database Arch Status Info Success", c.dataSource)
	return DB_ARCH_INVALID, nil
}

// getDbArchStatusInfo 查询归档的所有状态信息
func (c *DbArchStatusCollector) getDbArchStatusInfo(ctx context.Context, db *sql.DB) ([]DbArchStatusInfo, error) {
	var dbArchStatusInfos []DbArchStatusInfo
	rows, err := db.QueryContext(ctx, config.QueryArchiveSendStatusSql)
	if err != nil {
		utils.HandleDbQueryErrorWithSource(err, c.dataSource)
		return dbArchStatusInfos, err
	}
	defer rows.Close()

	for rows.Next() {
		var dbArchStatusInfo DbArchStatusInfo
		if err := rows.Scan(&dbArchStatusInfo.archStatus, &dbArchStatusInfo.archType,
			&dbArchStatusInfo.archDest, &dbArchStatusInfo.archSrc); err != nil {
			logger.Logger.Error(fmt.Sprintf("[%s] Error scanning row", c.dataSource), zap.Error(err))
			continue
		}
		dbArchStatusInfos = append(dbArchStatusInfos, dbArchStatusInfo)
	}

	if err := rows.Err(); err != nil {
		logger.Logger.Error(fmt.Sprintf("[%s] Error with rows", c.dataSource), zap.Error(err))
	}

	return dbArchStatusInfos, nil
}
