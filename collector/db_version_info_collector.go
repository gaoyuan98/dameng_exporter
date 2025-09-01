package collector

import (
	"context"
	"dameng_exporter/config"
	"dameng_exporter/logger"
	"database/sql"
	"fmt"
	"strings"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"go.uber.org/zap"
)

// 定义收集器结构体
type DbVersionCollector struct {
	db              *sql.DB
	versionInfoDesc *prometheus.Desc
	dataSource      string // 数据源名称
}

// SetDataSource 实现DataSourceAware接口
func (c *DbVersionCollector) SetDataSource(name string) {
	c.dataSource = name
}

// 版本信息结构体
type DbVersionInfo struct {
	idCode    sql.NullString
	buildType sql.NullString
	innerVer  sql.NullString
}

// 初始化收集器
func NewDbVersionCollector(db *sql.DB) MetricCollector {
	return &DbVersionCollector{
		db: db,
		versionInfoDesc: prometheus.NewDesc(
			dmdbms_version,
			"Information about DM database version",
			[]string{"host_name", "db_version_str", "build_type", "inner_ver"},
			nil,
		),
	}
}

func (c *DbVersionCollector) Describe(ch chan<- *prometheus.Desc) {
	ch <- c.versionInfoDesc
}

func (c *DbVersionCollector) Collect(ch chan<- prometheus.Metric) {

	if err := checkDBConnectionWithSource(c.db, c.dataSource); err != nil {
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(config.Global.GetQueryTimeout())*time.Second)
	defer cancel()

	// 尝试使用V2版本获取版本信息
	versionInfo, err := c.getDbVersionV2(ctx, c.db)
	if err != nil {
		logger.Logger.Warn("V2 version query failed, falling back to V1 version", zap.Error(err))
		// 如果V2版本失败，使用V1版本
		dbVersion, err := c.getDbVersionV1(ctx, c.db)
		if err != nil {
			logger.Logger.Error(fmt.Sprintf("[%s] exec getDbVersionV1 func error", c.dataSource), zap.Error(err))
			return
		}
		// 使用V1版本时，新增标签填充空值
		ch <- prometheus.MustNewConstMetric(
			c.versionInfoDesc,
			prometheus.GaugeValue,
			1,
			config.GetHostName(),
			dbVersion,
			"", // build_type为空
			"", // inner_ver为空
		)
		return
	}

	// 发送V2版本信息到Prometheus
	ch <- prometheus.MustNewConstMetric(
		c.versionInfoDesc,
		prometheus.GaugeValue,
		1,
		config.GetHostName(),
		NullStringToString(versionInfo.idCode),
		NullStringToString(versionInfo.buildType),
		NullStringToString(versionInfo.innerVer),
	)
}

// 获取数据库版本信息 V2版本
func (c *DbVersionCollector) getDbVersionV2(ctx context.Context, db *sql.DB) (*DbVersionInfo, error) {
	var versionInfo DbVersionInfo

	row := db.QueryRowContext(ctx, config.QueryVersionInfoSqlStr)
	err := row.Scan(&versionInfo.idCode, &versionInfo.buildType, &versionInfo.innerVer)
	if err != nil {
		return nil, err
	}

	logger.Logger.Debugf("[%s] Check Database version Info V2 Success, version info: %+v", c.dataSource, versionInfo)
	return &versionInfo, nil
}

// 获取数据库版本信息 V1版本
func (c *DbVersionCollector) getDbVersionV1(ctx context.Context, db *sql.DB) (string, error) {
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

	logger.Logger.Debugf("[%s] Check Database version Info V1 Success, version value %s", c.dataSource, dbVersion)
	return dbVersion, nil
}
