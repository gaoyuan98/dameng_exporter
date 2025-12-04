package collector

import (
	"context"
	"dameng_exporter/config"
	"dameng_exporter/logger"
	"dameng_exporter/utils"
	"database/sql"
	"sync"
	"time"

	"github.com/prometheus/client_golang/prometheus"
)

const (
	requiredRlogColumnCount = 4
	rlogLabelTypeCkpt       = "CKPT_LSN"
	rlogLabelTypeFile       = "FILE_LSN"
	rlogLabelTypeFlush      = "FLUSH_LSN"
	rlogLabelTypeCurrent    = "CUR_LSN"
)

type rlogLsnInfo struct {
	ckptLsn  sql.NullFloat64
	fileLsn  sql.NullFloat64
	flushLsn sql.NullFloat64
	curLsn   sql.NullFloat64
}

// DbRedoLogLsnCollector 采集 V$RLOG 中的 LSN 指标
type DbRedoLogLsnCollector struct {
	db              *sql.DB
	lsnDesc         *prometheus.Desc
	dataSource      string
	viewCheckOnce   sync.Once
	viewExists      bool
	columnCheckOnce sync.Once
	columnsExist    bool
}

// SetDataSource 实现 DataSourceAware 接口
func (c *DbRedoLogLsnCollector) SetDataSource(name string) {
	c.dataSource = name
}

// NewDbRedoLogLsnCollector 创建新的 LSN 采集器
func NewDbRedoLogLsnCollector(db *sql.DB) MetricCollector {
	return &DbRedoLogLsnCollector{
		db: db,
		lsnDesc: prometheus.NewDesc(
			dmdbms_rlog_lsn_total,
			"Redo log LSN positions from V$RLOG, labeled by type",
			[]string{"lsn_type"},
			nil,
		),
		viewExists:   true,
		columnsExist: true,
	}
}

func (c *DbRedoLogLsnCollector) Describe(ch chan<- *prometheus.Desc) {
	ch <- c.lsnDesc
}

func (c *DbRedoLogLsnCollector) Collect(ch chan<- prometheus.Metric) {
	if err := utils.CheckDBConnectionWithSource(c.db, c.dataSource); err != nil {
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(config.Global.GetQueryTimeout())*time.Second)
	defer cancel()

	if !c.checkRlogView(ctx) || !c.checkRlogColumns(ctx) {
		return
	}

	info, err := c.queryRlogInfo(ctx)
	if err != nil {
		return
	}

	c.emitMetric(ch, rlogLabelTypeCkpt, info.ckptLsn)
	c.emitMetric(ch, rlogLabelTypeFile, info.fileLsn)
	c.emitMetric(ch, rlogLabelTypeFlush, info.flushLsn)
	c.emitMetric(ch, rlogLabelTypeCurrent, info.curLsn)
}

func (c *DbRedoLogLsnCollector) emitMetric(ch chan<- prometheus.Metric, lsnType string, value sql.NullFloat64) {
	if !value.Valid {
		return
	}

	ch <- prometheus.MustNewConstMetric(
		c.lsnDesc,
		prometheus.CounterValue,
		utils.NullFloat64ToFloat64(value),
		lsnType,
	)
}

func (c *DbRedoLogLsnCollector) checkRlogView(ctx context.Context) bool {
	c.viewCheckOnce.Do(func() {
		var count int
		if err := c.db.QueryRowContext(ctx, config.QueryRlogViewExists).Scan(&count); err != nil {
			logger.Logger.Warnf("[%s] Failed to check V$RLOG view existence: %v", c.dataSource, err)
			c.viewExists = false
			return
		}
		c.viewExists = count > 0
		if !c.viewExists {
			logger.Logger.Infof("[%s] V$RLOG view not found, skip redo LSN metrics", c.dataSource)
		}
	})
	return c.viewExists
}

func (c *DbRedoLogLsnCollector) checkRlogColumns(ctx context.Context) bool {
	c.columnCheckOnce.Do(func() {
		var count int
		if err := c.db.QueryRowContext(ctx, config.QueryRlogColumnsExist).Scan(&count); err != nil {
			logger.Logger.Warnf("[%s] Failed to check V$RLOG columns: %v", c.dataSource, err)
			c.columnsExist = false
			return
		}
		c.columnsExist = count == requiredRlogColumnCount
		if !c.columnsExist {
			logger.Logger.Infof("[%s] Required V$RLOG columns missing, skip redo LSN metrics", c.dataSource)
		}
	})
	return c.columnsExist
}

func (c *DbRedoLogLsnCollector) queryRlogInfo(ctx context.Context) (rlogLsnInfo, error) {
	var result rlogLsnInfo

	rows, err := c.db.QueryContext(ctx, config.QueryRedoLogLsnInfoSql)
	if err != nil {
		utils.HandleDbQueryErrorWithSource(err, c.dataSource)
		return result, err
	}
	defer rows.Close()

	for rows.Next() {
		var row rlogLsnInfo
		if err := rows.Scan(&row.ckptLsn, &row.fileLsn, &row.flushLsn, &row.curLsn); err != nil {
			logger.Logger.Warnf("[%s] Failed to scan V$RLOG row: %v", c.dataSource, err)
			continue
		}
		result = mergeRlogInfo(result, row)
	}

	if err := rows.Err(); err != nil {
		logger.Logger.Warnf("[%s] Iterating V$RLOG rows failed: %v", c.dataSource, err)
		return result, err
	}

	return result, nil
}

func mergeRlogInfo(base, candidate rlogLsnInfo) rlogLsnInfo {
	if candidate.ckptLsn.Valid && (!base.ckptLsn.Valid || candidate.ckptLsn.Float64 > base.ckptLsn.Float64) {
		base.ckptLsn = candidate.ckptLsn
	}
	if candidate.fileLsn.Valid && (!base.fileLsn.Valid || candidate.fileLsn.Float64 > base.fileLsn.Float64) {
		base.fileLsn = candidate.fileLsn
	}
	if candidate.flushLsn.Valid && (!base.flushLsn.Valid || candidate.flushLsn.Float64 > base.flushLsn.Float64) {
		base.flushLsn = candidate.flushLsn
	}
	if candidate.curLsn.Valid && (!base.curLsn.Valid || candidate.curLsn.Float64 > base.curLsn.Float64) {
		base.curLsn = candidate.curLsn
	}
	return base
}
