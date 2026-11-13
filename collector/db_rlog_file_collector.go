package collector

import (
	"context"
	"dameng_exporter/config"
	"dameng_exporter/utils"
	"database/sql"
	"github.com/prometheus/client_golang/prometheus"
	"strconv"
	"time"
)

// RLOG 日志文件记录
type rlogFileRow struct {
	fileID     sql.NullInt64
	path       sql.NullString
	createTime sql.NullTime
	size       sql.NullFloat64
}

// DbRlogFileCollector 采集 V$RLOGFILE 列表信息
type DbRlogFileCollector struct {
	collectorLog
	db         *sql.DB
	dataSource string
	sizeDesc   *prometheus.Desc
}

// SetDataSource 实现 DataSourceAware 接口
func (c *DbRlogFileCollector) SetDataSource(name string) {
	c.dataSource = name
}

// NewDbRlogFileCollector 构造函数
func NewDbRlogFileCollector(db *sql.DB) MetricCollector {
	return &DbRlogFileCollector{
		db: db,
		sizeDesc: prometheus.NewDesc(
			dmdbms_rlog_file_size_bytes,
			"Redo log file sizes from V$RLOGFILE",
			[]string{"file_id", "path", "create_time"},
			nil,
		),
	}
}

func (c *DbRlogFileCollector) Describe(ch chan<- *prometheus.Desc) {
	ch <- c.sizeDesc
}

func (c *DbRlogFileCollector) Collect(ch chan<- prometheus.Metric) {
	if err := utils.CheckDBConnectionWithSource(c.db, c.dataSource, c.log()); err != nil {
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(config.Global.GetQueryTimeout())*time.Second)
	defer cancel()

	rows, err := c.database(c.db).QueryContext(ctx, config.QueryRlogFileListSql)
	if err != nil {
		utils.HandleDbQueryErrorWithSource(err, c.dataSource, c.trace, c.log())
		return
	}
	defer rows.Close()

	for rows.Next() {
		var row rlogFileRow
		if err := rows.Scan(&row.fileID, &row.path, &row.createTime, &row.size); err != nil {

			continue
		}
		c.emitMetric(ch, row)
	}

	_ = rows.Err()
}

func (c *DbRlogFileCollector) emitMetric(ch chan<- prometheus.Metric, row rlogFileRow) {
	fileID := ""
	if row.fileID.Valid {
		fileID = strconv.FormatInt(row.fileID.Int64, 10)
	}

	ch <- prometheus.MustNewConstMetric(
		c.sizeDesc,
		prometheus.GaugeValue,
		utils.NullFloat64ToFloat64(row.size),
		fileID,
		utils.NullStringToString(row.path),
		utils.NullTimeToString(row.createTime),
	)
}
