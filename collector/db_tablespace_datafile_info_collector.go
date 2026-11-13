package collector

import (
	"context"
	"dameng_exporter/config"
	"dameng_exporter/utils"
	"database/sql"
	"encoding/json"
	"fmt"
	"github.com/prometheus/client_golang/prometheus"
	"time"
)

type TableSpaceDateFileInfoCollector struct {
	collectorLog
	db         *sql.DB
	totalDesc  *prometheus.Desc
	freeDesc   *prometheus.Desc
	dataSource string // 数据源名称
}

// SetDataSource 实现DataSourceAware接口
func (c *TableSpaceDateFileInfoCollector) SetDataSource(name string) {
	c.dataSource = name
}

type TableSpaceDateFileInfo struct {
	Path       string
	TotalSize  float64
	FreeSize   float64
	AutoExtend string
	NextSize   string
	MaxSize    string
}

func NewTableSpaceDateFileInfoCollector(db *sql.DB) MetricCollector {
	return &TableSpaceDateFileInfoCollector{
		db: db,
		totalDesc: prometheus.NewDesc(
			dmdbms_tablespace_file_total_info,
			"Tablespace file information",
			[]string{"tablespace_name", "auto_extend", "next_size", "max_size"}, // 添加标签
			nil,
		),
		freeDesc: prometheus.NewDesc(
			dmdbms_tablespace_file_free_info,
			"Tablespace file information",
			[]string{"tablespace_name", "auto_extend", "next_size", "max_size"}, // 添加标签
			nil,
		),
	}
}

func (c *TableSpaceDateFileInfoCollector) Describe(ch chan<- *prometheus.Desc) {
	ch <- c.totalDesc
	ch <- c.freeDesc
}

func (c *TableSpaceDateFileInfoCollector) Collect(ch chan<- prometheus.Metric) {
	readReason := ""
	c.cache("database", false)

	//保存全局结果对象，可以用来做缓存以及序列化
	var tablespaceInfos []TableSpaceDateFileInfo

	// 从缓存中获取数据，使用带数据源的缓存键
	cacheKey := fmt.Sprintf("%s_%s", dmdbms_tablespace_file_total_info, c.dataSource)
	if cachedJSON, found := config.GetFromCache(cacheKey); found {
		var cachedInfos []TableSpaceDateFileInfo
		if err := json.Unmarshal([]byte(cachedJSON), &cachedInfos); err != nil {
			c.cache("database", true)
			c.businessError("business_validation", err)
		} else {
			c.log().Debugf("[%s] Use cache TablespaceDateFile data", c.dataSource)
			// 使用缓存的数据
			for _, info := range cachedInfos {
				ch <- prometheus.MustNewConstMetric(c.totalDesc, prometheus.GaugeValue, info.TotalSize, info.Path, info.AutoExtend, info.NextSize, info.MaxSize)
				ch <- prometheus.MustNewConstMetric(c.freeDesc, prometheus.GaugeValue, info.FreeSize, info.Path, info.AutoExtend, info.NextSize, info.MaxSize)
			}
			c.cache("cache", false)
			return
		}
	}

	if err := utils.CheckDBConnectionWithSource(c.db, c.dataSource, c.log()); err != nil {
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(config.Global.GetQueryTimeout())*time.Second)
	defer cancel()

	rows, err := c.database(c.db).QueryContext(ctx, config.QueryTablespaceFileSqlStr)
	if err != nil {
		utils.HandleDbQueryErrorWithSource(err, c.dataSource, c.trace, c.log())
		return
	}
	defer rows.Close()

	for rows.Next() {
		var info TableSpaceDateFileInfo
		if err := rows.Scan(&info.Path, &info.TotalSize, &info.FreeSize, &info.AutoExtend, &info.NextSize, &info.MaxSize); err != nil {

			readReason = "scan_error"
			continue
		}
		tablespaceInfos = append(tablespaceInfos, info)
	}
	if err := rows.Err(); err != nil {
		readReason = "rows_error"

	}
	// 发送数据到 Prometheus
	for _, info := range tablespaceInfos {
		ch <- prometheus.MustNewConstMetric(c.totalDesc, prometheus.GaugeValue, info.TotalSize, info.Path, info.AutoExtend, info.NextSize, info.MaxSize)
		ch <- prometheus.MustNewConstMetric(c.freeDesc, prometheus.GaugeValue, info.FreeSize, info.Path, info.AutoExtend, info.NextSize, info.MaxSize)
	}
	// Keep partial rows in this scrape, but never reuse them as a complete cache.
	if readReason != "" {
		return
	}

	// 将 TablespaceInfo 切片序列化为 JSON 字符串
	valueJSON, err := json.Marshal(tablespaceInfos)
	if err != nil {
		// 处理序列化错误
		c.businessError("business_validation", err)
		return
	}
	// 将查询结果存入缓存，重用之前定义的cacheKey
	config.SetCache(cacheKey, string(valueJSON), time.Minute*time.Duration(config.Global.GetBigKeyDataCacheTime()))
	c.log().Debugf("[%s] TablespaceFileInfoCollector exec finish", c.dataSource)

	return
}
