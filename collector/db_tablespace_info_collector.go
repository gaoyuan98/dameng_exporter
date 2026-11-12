package collector

import (
	"context"
	"dameng_exporter/config"
	"dameng_exporter/utils"
	"database/sql"
	"encoding/json"
	"fmt"
	"github.com/prometheus/client_golang/prometheus"
	"go.uber.org/zap"
	"time"
)

type TableSpaceInfoCollector struct {
	collectorLog
	db            *sql.DB
	totalDesc     *prometheus.Desc
	freeDesc      *prometheus.Desc
	actUsedRtDesc *prometheus.Desc
	dataSource    string // 数据源名称
}

// SetDataSource 实现DataSourceAware接口
func (c *TableSpaceInfoCollector) SetDataSource(name string) {
	c.dataSource = name
}

type TableSpaceInfo struct {
	TablespaceName string
	TotalSize      float64 // 对应 ASSIGNED_TOTAL_MB
	FreeSize       float64 // 对应 ASSIGNED_FREE_MB
	ActUsedRt      float64 // 对应 ACTUSED_RT%
}

func NewTableSpaceInfoCollector(db *sql.DB) MetricCollector {
	return &TableSpaceInfoCollector{
		db: db,
		totalDesc: prometheus.NewDesc(
			dmdbms_tablespace_size_total_info,
			"Tablespace info information",
			[]string{"tablespace_name"}, // 添加标签
			nil,
		),
		freeDesc: prometheus.NewDesc(
			dmdbms_tablespace_size_free_info,
			"Tablespace info information",
			[]string{"tablespace_name"}, // 添加标签
			nil,
		),
		actUsedRtDesc: prometheus.NewDesc(
			dmdbms_tablespace_size_act_used_rt_info,
			"Tablespace actual used rate information",
			[]string{"tablespace_name"}, // 添加标签
			nil,
		),
	}
}

func (c *TableSpaceInfoCollector) Describe(ch chan<- *prometheus.Desc) {
	ch <- c.totalDesc
	ch <- c.freeDesc
	ch <- c.actUsedRtDesc
}

func (c *TableSpaceInfoCollector) Collect(ch chan<- prometheus.Metric) {
	c.CollectWithResult(ch)
}

func (c *TableSpaceInfoCollector) CollectWithResult(ch chan<- prometheus.Metric) (result CollectorOutcome) {
	cacheFallback := false
	defer func() { result.CacheFallback = cacheFallback }()
	validRows, readReason := 0, ""

	//保存全局结果对象，可以用来做缓存以及序列化
	var tablespaceInfos []TableSpaceInfo

	// 从缓存中获取数据，使用带数据源的缓存键
	cacheKey := fmt.Sprintf("%s_%s", dmdbms_tablespace_size_total_info, c.dataSource)
	if cachedJSON, found := config.GetFromCache(cacheKey); found {
		var cachedInfos []TableSpaceInfo
		if err := json.Unmarshal([]byte(cachedJSON), &cachedInfos); err != nil {
			cacheFallback = true
			c.log().Desugar().Error(fmt.Sprintf("[%s] Error unmarshaling cached data", c.dataSource), zap.Error(err))
		} else {
			c.log().Debugf("[%s] Use cache TablespaceInfo data", c.dataSource)
			// 使用缓存的数据
			for _, info := range cachedInfos {
				ch <- prometheus.MustNewConstMetric(c.totalDesc, prometheus.GaugeValue, info.TotalSize, info.TablespaceName)
				ch <- prometheus.MustNewConstMetric(c.freeDesc, prometheus.GaugeValue, info.FreeSize, info.TablespaceName)
				ch <- prometheus.MustNewConstMetric(c.actUsedRtDesc, prometheus.GaugeValue, info.ActUsedRt, info.TablespaceName)
			}
			return CollectorOutcome{State: OutcomeUnknown, Reason: "cache_unverified", DataOrigin: "cache"}
		}
	}

	if err := utils.CheckDBConnectionWithSource(c.db, c.dataSource, c.log()); err != nil {
		return databaseOutcome(OutcomeFailed, "datasource_unavailable")
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(config.Global.GetQueryTimeout())*time.Second)
	defer cancel()

	rows, err := c.db.QueryContext(ctx, config.QueryTablespaceInfoSqlStr)
	if err != nil {
		outcome := queryFailure(ctx, err)
		utils.HandleDbQueryErrorWithSource(err, c.dataSource, c.log())
		return outcome
	}
	defer rows.Close()

	for rows.Next() {
		var info TableSpaceInfo
		var assignedTotalMB float64
		var assignedFreeMB float64
		var files int64
		var usedSzRt float64
		var extensMb float64
		var totalFreePct float64
		var actUsedRt float64
		if err := rows.Scan(
			&info.TablespaceName,
			&files,
			&assignedTotalMB,
			&assignedFreeMB,
			&usedSzRt,
			&extensMb,
			&totalFreePct,
			&actUsedRt,
		); err != nil {
			c.log().Desugar().Error(fmt.Sprintf("[%s] Error scanning row", c.dataSource), zap.Error(err))
			readReason = "scan_error"
			continue
		}
		info.TotalSize = assignedTotalMB
		info.FreeSize = assignedFreeMB
		info.ActUsedRt = actUsedRt
		validRows++
		tablespaceInfos = append(tablespaceInfos, info)
	}
	if err := rows.Err(); err != nil {
		readReason = "rows_error"
		c.log().Desugar().Error(fmt.Sprintf("[%s] Error with rows", c.dataSource), zap.Error(err))
	}
	result = tableOutcome(validRows, readReason, cacheFallback)
	// 发送数据到 Prometheus
	for _, info := range tablespaceInfos {
		ch <- prometheus.MustNewConstMetric(c.totalDesc, prometheus.GaugeValue, info.TotalSize, info.TablespaceName)
		ch <- prometheus.MustNewConstMetric(c.freeDesc, prometheus.GaugeValue, info.FreeSize, info.TablespaceName)
		ch <- prometheus.MustNewConstMetric(c.actUsedRtDesc, prometheus.GaugeValue, info.ActUsedRt, info.TablespaceName)
	}
	// Keep partial rows in this scrape, but never reuse them as a complete cache.
	if readReason != "" {
		return result
	}

	// 将 TablespaceInfo 切片序列化为 JSON 字符串
	valueJSON, err := json.Marshal(tablespaceInfos)
	if err != nil {
		// 处理序列化错误
		c.log().Desugar().Error(fmt.Sprintf("[%s] TablespaceInfo marshal error", c.dataSource), zap.Error(err))
		return result
	}
	// 将查询结果存入缓存，重用之前定义的cacheKey
	config.SetCache(cacheKey, string(valueJSON), time.Minute*time.Duration(config.Global.GetBigKeyDataCacheTime()))
	//	c.log().Debugf("TablespaceFileInfo exec finish")

	return result
}
