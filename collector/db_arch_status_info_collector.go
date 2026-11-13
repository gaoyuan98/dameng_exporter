package collector

import (
	"context"
	"dameng_exporter/config"
	"dameng_exporter/utils"
	"database/sql"
	"fmt"
	"github.com/prometheus/client_golang/prometheus"
	"time"
)

const (
	// DB_ARCH_NO_ENABLE 表示数据库未启用归档。
	DB_ARCH_NO_ENABLE     = -1
	DB_ARCH_EMPTY_OR_NULL = -3
	// DB_ARCH_UNKNOWN 表示遇到当前版本未识别的归档状态。
	DB_ARCH_UNKNOWN = -2
	// DB_ARCH_INVALID 表示本地归档状态无效。
	DB_ARCH_INVALID = 0
	// DB_ARCH_VALID 表示本地归档状态正常。
	DB_ARCH_VALID = 1
	// DB_ARCH_ASYNC_SEND 表示本地归档处于异步发送状态。
	DB_ARCH_ASYNC_SEND = 2
)

// DbArchStatusInfo 表示单个归档目的地的状态信息。
type DbArchStatusInfo struct {
	archType   sql.NullString
	archDest   sql.NullString
	archSrc    sql.NullString
	archStatus sql.NullFloat64
}

// DbArchStatusCollector 负责采集归档总状态和归档目的地状态。
type DbArchStatusCollector struct {
	collectorLog
	db             *sql.DB
	archStatusDesc *prometheus.Desc
	archStatusInfo *prometheus.Desc
	dataSource     string
}

// SetDataSource 实现 DataSourceAware 接口。
func (c *DbArchStatusCollector) SetDataSource(name string) {
	c.dataSource = name
}

// NewDbArchStatusCollector 初始化归档状态采集器。
func NewDbArchStatusCollector(db *sql.DB) MetricCollector {
	return &DbArchStatusCollector{
		db: db,
		archStatusDesc: prometheus.NewDesc(
			dmdbms_arch_status,
			"Information about DM database archive status, value info: invalid = 0, valid = 1, async_send = 2, unknown = -2, no_enable = -1",
			[]string{},
			nil,
		),
		archStatusInfo: prometheus.NewDesc(
			dmdbms_arch_status_info,
			"Information about DM database archive status detail, value info: invalid = 0, valid = 1, async_send = 2, unknown = -2, empty_or_null = -3",
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
	if err := utils.CheckDBConnectionWithSource(c.db, c.dataSource, c.log()); err != nil {
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(config.Global.GetQueryTimeout())*time.Second)
	defer cancel()

	// 先采集本地归档总状态。
	dbArchStatus, err := c.getDbArchStatus(ctx, c.db)
	if err != nil {

		setArchMetric(ch, c.archStatusDesc, DB_ARCH_UNKNOWN)
		return
	}

	// 输出本地归档总状态指标。
	setArchMetric(ch, c.archStatusDesc, dbArchStatus)

	// 只要归档已启用，就继续采集各归档目的地状态，避免 INVALID 时指标缺失。
	if dbArchStatus != DB_ARCH_NO_ENABLE {
		dbArchStatusInfos, err := c.getDbArchStatusInfo(ctx, c.db)
		if err != nil {
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

// setArchMetric 输出单值归档状态指标。
func setArchMetric(ch chan<- prometheus.Metric, desc *prometheus.Desc, value int) {
	ch <- prometheus.MustNewConstMetric(
		desc,
		prometheus.GaugeValue,
		float64(value),
	)
}

// getDbArchStatus 查询本地归档总状态。
func (c *DbArchStatusCollector) getDbArchStatus(ctx context.Context, db *sql.DB) (int, error) {
	var dbArchStatus string

	query := `select /*+DMDB_CHECK_FLAG*/ PARA_VALUE from v$dm_ini where para_name='ARCH_INI'`
	row := c.database(db).QueryRowContext(ctx, query)
	err := row.Scan(&dbArchStatus)
	if err != nil {
		return DB_ARCH_UNKNOWN, fmt.Errorf("query error: %v", err)
	}

	// 当 ARCH_INI=1 时，再判断 LOCAL 归档的具体状态。
	if dbArchStatus == "1" {
		query = `select /*+DMDB_CHECK_FLAG*/ case arch_status when 'VALID' then 'VALID' when 'INVALID' then 'INVALID' when 'ASYNC_SEND' then 'ASYNC_SEND' else 'UNKNOWN' end ARCH_STATUS from v$arch_status where arch_type='LOCAL'`
		row = c.database(db).QueryRowContext(ctx, query)
		err = row.Scan(&dbArchStatus)
		if err != nil {
			return DB_ARCH_UNKNOWN, fmt.Errorf("query error: %v", err)
		}

		switch dbArchStatus {
		case "VALID":
			return DB_ARCH_VALID, nil
		case "INVALID":
			return DB_ARCH_INVALID, nil
		case "ASYNC_SEND":
			return DB_ARCH_ASYNC_SEND, nil
		case "UNKNOWN":
			return DB_ARCH_UNKNOWN, nil
		}
	} else if dbArchStatus == "0" {
		return DB_ARCH_NO_ENABLE, nil
	}

	c.log().Debugf("[%s] Check Database Arch Status Info Success", c.dataSource)
	return DB_ARCH_UNKNOWN, nil
}

// getDbArchStatusInfo 查询所有归档目的地的状态明细。
func (c *DbArchStatusCollector) getDbArchStatusInfo(ctx context.Context, db *sql.DB) ([]DbArchStatusInfo, error) {
	var dbArchStatusInfos []DbArchStatusInfo
	rows, err := c.database(db).QueryContext(ctx, config.QueryArchiveSendStatusSql)
	if err != nil {
		utils.HandleDbQueryErrorWithSource(err, c.dataSource, c.trace, c.log())
		return dbArchStatusInfos, err
	}
	defer rows.Close()

	for rows.Next() {
		var dbArchStatusInfo DbArchStatusInfo
		if err := rows.Scan(&dbArchStatusInfo.archStatus, &dbArchStatusInfo.archType,
			&dbArchStatusInfo.archDest, &dbArchStatusInfo.archSrc); err != nil {

			continue
		}
		dbArchStatusInfos = append(dbArchStatusInfos, dbArchStatusInfo)
	}

	_ = rows.Err()

	return dbArchStatusInfos, nil
}
