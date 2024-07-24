package collector

import (
	"context"
	"dameng_exporter/config"
	"dameng_exporter/logger"
	"database/sql"
	"fmt"
	"github.com/prometheus/client_golang/prometheus"
	"go.uber.org/zap"
	"time"
)

// 定义常量
const (
	DB_ARCH_NO_ENABLE = -1
	DB_ARCH_VALID     = 1
	DB_ARCH_INVALID   = 2
)

// 定义收集器结构体
type DbArchStatusCollector struct {
	db             *sql.DB
	archStatusDesc *prometheus.Desc
}

// 初始化收集器
func NewDbArchStatusCollector(db *sql.DB) MetricCollector {
	return &DbArchStatusCollector{
		db: db,
		archStatusDesc: prometheus.NewDesc(
			dmdbms_arch_status,
			"Information about DM database archive status",
			[]string{"hostname"},
			nil,
		),
	}
}

func (c *DbArchStatusCollector) Describe(ch chan<- *prometheus.Desc) {
	ch <- c.archStatusDesc
}

func (c *DbArchStatusCollector) Collect(ch chan<- prometheus.Metric) {
	funcStart := time.Now()
	// 时间间隔的计算发生在 defer 语句执行时，确保能够获取到正确的函数执行时间。
	defer func() {
		duration := time.Since(funcStart)
		logger.Logger.Debugf("func exec time：%vms", duration.Milliseconds())
	}()

	if err := c.db.Ping(); err != nil {
		logger.Logger.Error("Database connection is not available: %v", zap.Error(err))
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(config.GlobalConfig.QueryTimeout)*time.Second)
	defer cancel()

	rows, err := c.db.QueryContext(ctx, config.QueryDbGrantInfoSql)
	if err != nil {
		handleDbQueryError(err)
		return
	}
	defer rows.Close()

	// 获取数据库归档状态信息
	dbArchStatus, err := getDbArchStatus(ctx, c.db)
	if err != nil {
		logger.Logger.Error("exec getDbArchStatus func error", zap.Error(err))
		setArchMetric(ch, c.archStatusDesc, DB_ARCH_INVALID)
		return
	}
	setArchMetric(ch, c.archStatusDesc, dbArchStatus)
}

// 辅助函数：设置指标
func setArchMetric(ch chan<- prometheus.Metric, desc *prometheus.Desc, value int) {
	hostname := config.GetHostName()
	ch <- prometheus.MustNewConstMetric(
		desc,
		prometheus.GaugeValue,
		float64(value),
		hostname,
	)
}

// 获取数据库归档状态信息
func getDbArchStatus(ctx context.Context, db *sql.DB) (int, error) {
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

	logger.Logger.Info("Check Database Arch Status Info Success")
	return DB_ARCH_INVALID, nil
}
