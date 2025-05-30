package collector

import (
	"context"
	"dameng_exporter/config"
	"dameng_exporter/logger"
	"database/sql"
	"fmt"
	"github.com/prometheus/client_golang/prometheus"
	"go.uber.org/zap"
	"strings"
	"time"
)

// 定义收集器结构体
type DbVersionCollector struct {
	db              *sql.DB
	versionInfoDesc *prometheus.Desc
}

// 初始化收集器
func NewDbVersionCollector(db *sql.DB) MetricCollector {
	return &DbVersionCollector{
		db: db,
		versionInfoDesc: prometheus.NewDesc(
			dmdbms_version,
			"Information about DM database version",
			[]string{"host_name", "db_version_str"},
			nil,
		),
	}
}

func (c *DbVersionCollector) Describe(ch chan<- *prometheus.Desc) {
	ch <- c.versionInfoDesc
}

func (c *DbVersionCollector) Collect(ch chan<- prometheus.Metric) {
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

	// 获取数据库版本信息
	dbVersion, err := getDbVersion(ctx, c.db)
	if err != nil {
		logger.Logger.Error("exec getDbVersion func error", zap.Error(err))
		setVersionMetric(ch, c.versionInfoDesc, 1, "error")
		return
	}

	setVersionMetric(ch, c.versionInfoDesc, 0, dbVersion)
}

// 辅助函数：设置指标
func setVersionMetric(ch chan<- prometheus.Metric, desc *prometheus.Desc, value float64, dbVersion string) {
	hostname := config.GetHostName()
	ch <- prometheus.MustNewConstMetric(
		desc,
		prometheus.GaugeValue,
		value,
		hostname, dbVersion,
	)
}

// 获取数据库版本信息
func getDbVersion(ctx context.Context, db *sql.DB) (string, error) {
	var dbVersion string

	query := `SELECT /*+DM_EXPORTER*/ position('BUILD_VERSION', to_char(TABLEDEF('SYS', 'V$INSTANCE'))) POS FROM dual`
	row := db.QueryRowContext(ctx, query)

	var pos int
	err := row.Scan(&pos)
	if err != nil {
		return "", fmt.Errorf("query error: %v", err)
	}

	if pos > 0 {
		query = `SELECT /*+DM_EXPORTER*/ svr_version || '-' || BUILD_VERSION VERSION FROM v$instance`
	} else {
		query = `SELECT /*+DM_EXPORTER*/ TOP 1 banner || ' ' || id_code VERSION FROM v$version WHERE banner LIKE 'DM Database Server%'`
	}

	row = db.QueryRowContext(ctx, query)
	err = row.Scan(&dbVersion)
	if err != nil {
		return "", fmt.Errorf("query error: %v", err)
	}

	// 移除换行符
	dbVersion = strings.ReplaceAll(dbVersion, "\n", "")

	// 如果字符串中包含 "DM Database Server" 则去掉
	targetStr := "DM Database Server"
	if strings.Contains(dbVersion, targetStr) {
		dbVersion = strings.Replace(dbVersion, targetStr, "", -1)
		dbVersion = strings.TrimSpace(dbVersion)
	}

	logger.Logger.Debugf("Check Database version Info Success, version value %s", dbVersion)
	return dbVersion, nil
}
