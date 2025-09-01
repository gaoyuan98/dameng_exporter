package collector

import (
	"context"
	"dameng_exporter/config"
	"dameng_exporter/logger"
	"database/sql"
	"fmt"
	"github.com/prometheus/client_golang/prometheus"
	"go.uber.org/zap"
	"strconv"
	"time"
)

// 定义数据结构
type DbLicenseInfo struct {
	ExpiredDate sql.NullString
}

// 定义收集器结构体
type DbLicenseCollector struct {
	db              *sql.DB
	licenseDateDesc *prometheus.Desc
	dataSource      string // 数据源名称
}

// SetDataSource 实现DataSourceAware接口
func (c *DbLicenseCollector) SetDataSource(name string) {
	c.dataSource = name
}

func NewDbLicenseCollector(db *sql.DB) MetricCollector {
	return &DbLicenseCollector{
		db: db,
		licenseDateDesc: prometheus.NewDesc(
			dmdbms_license_date,
			"Information about DM database license expiration date",
			[]string{"host_name", "date_day_str"},
			nil,
		),
	}
}

func (c *DbLicenseCollector) Describe(ch chan<- *prometheus.Desc) {
	ch <- c.licenseDateDesc
}

func (c *DbLicenseCollector) Collect(ch chan<- prometheus.Metric) {

	if err := checkDBConnectionWithSource(c.db, c.dataSource); err != nil {
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(config.Global.GetQueryTimeout())*time.Second)
	defer cancel()

	rows, err := c.db.QueryContext(ctx, config.QueryDbGrantInfoSql)
	if err != nil {
		handleDbQueryErrorWithSource(err, c.dataSource)
		return
	}
	defer rows.Close()

	var licenseInfos []DbLicenseInfo
	for rows.Next() {
		var info DbLicenseInfo
		if err := rows.Scan(&info.ExpiredDate); err != nil {
			logger.Logger.Error(fmt.Sprintf("[%s] Error scanning row", c.dataSource), zap.Error(err))
			continue
		}
		licenseInfos = append(licenseInfos, info)
	}
	if err := rows.Err(); err != nil {
		logger.Logger.Error(fmt.Sprintf("[%s] Error with rows", c.dataSource), zap.Error(err))
		return
	}

	hostname := config.GetHostName()
	for _, info := range licenseInfos {
		expiredDateStr := NullStringToString(info.ExpiredDate)
		var returnDateStr string
		var licenseStatus string
		if expiredDateStr != "" {
			expiredDate, err := time.Parse("20060102", expiredDateStr)
			if err != nil {
				logger.Logger.Error(fmt.Sprintf("[%s] Error parsing date", c.dataSource), zap.Error(err))
				continue
			}
			betweenDay := expiredDate.Sub(time.Now()).Hours() / 24
			returnDateStr = fmt.Sprintf("%.0f", betweenDay)
			licenseStatus = returnDateStr
			logger.Logger.Infof("[%s] Check Database License Date Info Success, betweenDay is %s day", c.dataSource, returnDateStr)
		} else {
			licenseStatus = "无限制"
			returnDateStr = "-1"
			logger.Logger.Debugf("[%s] Check Database License Date Info Success, Expired Unlimited", c.dataSource)
		}

		ch <- prometheus.MustNewConstMetric(
			c.licenseDateDesc,
			prometheus.GaugeValue,
			parseToFloat64(returnDateStr),
			hostname, licenseStatus,
		)
	}

}

// 辅助函数，将 string 转换为 float64
func parseToFloat64(s string) float64 {
	if s == "" {
		return 0
	}
	val, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return 0
	}
	return val
}
