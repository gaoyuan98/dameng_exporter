package collector

import (
	"context"
	"dameng_exporter/config"
	"dameng_exporter/logger"
	"database/sql"
	"fmt"
	"github.com/prometheus/client_golang/prometheus"
	"go.uber.org/zap"
	"sync"
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
	db                       *sql.DB
	archStatusDesc           *prometheus.Desc //归档状态(本地)
	archSwitchRateDesc       *prometheus.Desc //归档切换频率
	archSwitchRateDetailInfo *prometheus.Desc //归档切换频率详情
	archStatusInfo           *prometheus.Desc //归档所有状态
	archSendDetailInfo       *prometheus.Desc //归档状态的发送详情
	archSendDiffValue        *prometheus.Desc //归档发送的差值
}

// 20250311 新增如果归档开启的话 返回这个归档跟上一个归档的间隔时间
// 定义数据结构
type DbArchSwitchRateInfo struct {
	status     sql.NullString
	createTime sql.NullString
	path       sql.NullString
	clsn       sql.NullString
	srcDbMagic sql.NullString
	minusDiff  sql.NullFloat64
}

// 20250401 新增归档状态的所有信息不仅限于local状态
type DbArchStatusInfo struct {
	archType   sql.NullString
	archDest   sql.NullString
	archSrc    sql.NullString
	archStatus sql.NullFloat64
}

// 20250401 新增归档状态的发送详情
// SELECT ARCH_DEST,ARCH_TYPE,LSN_DIFFERENCE,LAST_SEND_CODE,LAST_SEND_DESC,TO_CHAR(LAST_START_TIME,'YYYY-MM-DD HH24:MI:SS') AS LAST_START_TIME,TO_CHAR(LAST_END_TIME,'YYYY-MM-DD HH24:MI:SS') AS LAST_END_TIME,LAST_SEND_TIME FROM V$ARCH_SEND_INFO;
type DbArchSendDetailInfo struct {
	archDest      sql.NullString
	archType      sql.NullString
	lsnDiff       sql.NullFloat64
	lastSendCode  sql.NullString
	lastSendDesc  sql.NullString
	lastStartTime sql.NullString
	lastEndTime   sql.NullString
	lastSendTime  sql.NullString
}

// 初始化收集器
func NewDbArchStatusCollector(db *sql.DB) MetricCollector {
	return &DbArchStatusCollector{
		db: db,
		archStatusDesc: prometheus.NewDesc(
			dmdbms_arch_status,
			"Information about DM database archive status, value info: vaild = 1,invaild = 2,no_enable= -1",
			[]string{"host_name"},
			nil,
		),
		archSwitchRateDesc: prometheus.NewDesc(
			dmdbms_arch_switch_rate,
			"Information about DM database archive switch rate，Always output the most recent piece of data",
			[]string{"host_name" /*, "status", "createTime", "path", "clsn", "srcDbMagic"*/},
			nil,
		),
		archSwitchRateDetailInfo: prometheus.NewDesc(
			dmdbms_arch_switch_rate_detail_info,
			"Information about DM database archive switch rate info, return MAX_SEND_LSN - LAST_SEND_LSN = diffValue",
			[]string{"host_name", "status", "createTime", "path", "clsn", "srcDbMagic"},
			nil,
		),

		archStatusInfo: prometheus.NewDesc(
			dmdbms_arch_status_info,
			"Information about DM database archive status, value info: vaild = 1,invaild = 0",
			[]string{"host_name", "arch_type", "arch_dest", "arch_src"},
			nil,
		),

		archSendDetailInfo: prometheus.NewDesc(
			dmdbms_arch_send_detail_info,
			"Information about DM database archive send detail info, return MAX_SEND_LSN - LAST_SEND_LSN = diffValue",
			[]string{"host_name", "arch_type", "arch_dest", "last_send_code", "last_send_desc", "last_start_time", "last_end_time", "last_send_time"},
			nil,
		),
		archSendDiffValue: prometheus.NewDesc(
			dmdbms_arch_send_diff_value,
			"Information about DM database archive send detail info, return MAX_SEND_LSN - LAST_SEND_LSN = diffValue",
			[]string{"host_name", "arch_type", "arch_dest"},
			nil,
		),
	}
}

func (c *DbArchStatusCollector) Describe(ch chan<- *prometheus.Desc) {
	ch <- c.archStatusDesc
	ch <- c.archSwitchRateDesc
	ch <- c.archSwitchRateDetailInfo
	ch <- c.archStatusInfo
	ch <- c.archSendDetailInfo
	ch <- c.archSendDiffValue
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

	// 获取数据库归档状态信息
	dbArchStatus, err := getDbArchStatus(ctx, c.db)
	if err != nil {
		logger.Logger.Error("exec getDbArchStatus func error", zap.Error(err))
		setArchMetric(ch, c.archStatusDesc, DB_ARCH_INVALID)
		return
	}

	setArchMetric(ch, c.archStatusDesc, dbArchStatus)
	//如果归档是开启的，则查询归档切换频率
	if dbArchStatus == DB_ARCH_VALID {
		//查询语句并封装对象
		dbArchSwitchRateInfo, err := getDbArchSwitchRate(ctx, c.db)
		if err != nil {
			//logger.Logger.Error("exec getDbArchSwitchRate func error", zap.Error(err))
			return
		}
		clsn := NullStringToString(dbArchSwitchRateInfo.clsn)
		srcDbMagic := NullStringToString(dbArchSwitchRateInfo.srcDbMagic)
		status := NullStringToString(dbArchSwitchRateInfo.status)
		path := NullStringToString(dbArchSwitchRateInfo.path)
		createTime := NullStringToString(dbArchSwitchRateInfo.createTime)
		minusDiff := NullFloat64ToFloat64(dbArchSwitchRateInfo.minusDiff)
		hostname := config.GetHostName()
		//做折线图
		ch <- prometheus.MustNewConstMetric(
			c.archSwitchRateDesc,
			prometheus.GaugeValue,
			minusDiff,
			hostname,
		)
		//归档切换的详细信息
		ch <- prometheus.MustNewConstMetric(
			c.archSwitchRateDetailInfo,
			prometheus.GaugeValue,
			minusDiff,
			hostname, status, createTime, path, clsn, srcDbMagic,
		)

		//查询所有归档的状态信息
		dbArchStatusInfos, err := getDbArchStatusInfo(ctx, c.db)
		if err != nil {
			logger.Logger.Error("exec getDbArchStatusInfo func error", zap.Error(err))
			return
		}
		for _, dbArchStatusInfo := range dbArchStatusInfos {
			archType := NullStringToString(dbArchStatusInfo.archType)
			archDest := NullStringToString(dbArchStatusInfo.archDest)
			archSrc := NullStringToString(dbArchStatusInfo.archSrc)
			archStatus := NullFloat64ToFloat64(dbArchStatusInfo.archStatus)
			ch <- prometheus.MustNewConstMetric(
				c.archStatusInfo,
				prometheus.GaugeValue,
				archStatus,
				hostname, archType, archDest, archSrc,
			)
		}

		//查询所有归档发送详情信息
		dbArchSendInfos, err := getDbArchSendDetailInfo(ctx, c.db)
		if err != nil {
			logger.Logger.Error("exec getDbArchSendDetailInfo func error", zap.Error(err))
			return
		}
		for _, dbArchSendInfo := range dbArchSendInfos {
			archType := NullStringToString(dbArchSendInfo.archType)
			archDest := NullStringToString(dbArchSendInfo.archDest)
			lsnDiff := NullFloat64ToFloat64(dbArchSendInfo.lsnDiff)
			lastSendCode := NullStringToString(dbArchSendInfo.lastSendCode)
			lastSendDesc := NullStringToString(dbArchSendInfo.lastSendDesc)
			lastStartTime := NullStringToString(dbArchSendInfo.lastStartTime)
			lastEndTime := NullStringToString(dbArchSendInfo.lastEndTime)
			lastSendTime := NullStringToString(dbArchSendInfo.lastSendTime)
			ch <- prometheus.MustNewConstMetric(
				c.archSendDetailInfo,
				prometheus.GaugeValue,
				lsnDiff,
				hostname, archType, archDest, lastSendCode, lastSendDesc, lastStartTime, lastEndTime, lastSendTime,
			)
			//20250605 存放diff差值
			ch <- prometheus.MustNewConstMetric(
				c.archSendDiffValue,
				prometheus.GaugeValue,
				lsnDiff,
				hostname, archType, archDest,
			)

		}

	}

}

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

// 查询归档切换频率
func getDbArchSwitchRate(ctx context.Context, db *sql.DB) (DbArchSwitchRateInfo, error) {

	var dbArchSwitchRateInfo DbArchSwitchRateInfo

	rows, err := db.QueryContext(ctx, config.QueryArchiveSwitchRateSql)
	if err != nil {
		handleDbQueryError(err)
		return dbArchSwitchRateInfo, err
	}
	defer rows.Close()
	rows.Next()
	if err := rows.Scan(&dbArchSwitchRateInfo.status, &dbArchSwitchRateInfo.createTime, &dbArchSwitchRateInfo.path, &dbArchSwitchRateInfo.clsn, &dbArchSwitchRateInfo.srcDbMagic, &dbArchSwitchRateInfo.minusDiff); err != nil {
		logger.Logger.Error("Error scanning row", zap.Error(err))
		return dbArchSwitchRateInfo, err
	}
	return dbArchSwitchRateInfo, nil
}

// 查询归档的所有状态信息
func getDbArchStatusInfo(ctx context.Context, db *sql.DB) ([]DbArchStatusInfo, error) {
	var dbArchStatusInfos []DbArchStatusInfo
	rows, err := db.QueryContext(ctx, config.QueryArchiveSendStatusSql)
	if err != nil {
		handleDbQueryError(err)
		return dbArchStatusInfos, err
	}
	defer rows.Close()
	for rows.Next() {
		var dbArchStatusInfo DbArchStatusInfo
		if err := rows.Scan(&dbArchStatusInfo.archStatus, &dbArchStatusInfo.archType, &dbArchStatusInfo.archDest, &dbArchStatusInfo.archSrc); err != nil {
			logger.Logger.Error("Error scanning row", zap.Error(err))
			continue
		}
		dbArchStatusInfos = append(dbArchStatusInfos, dbArchStatusInfo)
	}
	if err := rows.Err(); err != nil {
		logger.Logger.Error("Error with rows", zap.Error(err))
	}
	return dbArchStatusInfos, nil
}

// 查询所有归档发送详情信息
func getDbArchSendDetailInfo(ctx context.Context, db *sql.DB) ([]DbArchSendDetailInfo, error) {
	//20250509 如果视图V$ARCH_APPLY_INFO存在,则使用视图V$ARCH_APPLY_INFO的RPKG_LSN字段
	var querySql string
	if ViewArchApplyInfoExists(ctx, db) {
		querySql = config.QueryArchSendDetailInfo2
	} else {
		querySql = config.QueryArchSendDetailInfo
	}

	var dbArchSendDetailInfos []DbArchSendDetailInfo
	rows, err := db.QueryContext(ctx, querySql)
	if err != nil {
		handleDbQueryError(err)
		return dbArchSendDetailInfos, err
	}
	defer rows.Close()
	for rows.Next() {
		var dbArchSendDetailInfo DbArchSendDetailInfo
		if err := rows.Scan(&dbArchSendDetailInfo.archDest, &dbArchSendDetailInfo.archType, &dbArchSendDetailInfo.lsnDiff, &dbArchSendDetailInfo.lastSendCode, &dbArchSendDetailInfo.lastSendDesc, &dbArchSendDetailInfo.lastStartTime, &dbArchSendDetailInfo.lastEndTime, &dbArchSendDetailInfo.lastSendTime); err != nil {
			logger.Logger.Error("Error scanning row", zap.Error(err))
			continue
		}
		dbArchSendDetailInfos = append(dbArchSendDetailInfos, dbArchSendDetailInfo)
	}
	if err := rows.Err(); err != nil {
		logger.Logger.Error("Error with rows", zap.Error(err))
	}

	return dbArchSendDetailInfos, nil
}

var (
	viewArchApplyInfoCheckOnce sync.Once
	viewArchApplyInfoExists    bool
)

func ViewArchApplyInfoExists(ctx context.Context, db *sql.DB) bool {
	viewArchApplyInfoCheckOnce.Do(func() {
		const query = "SELECT COUNT(1) FROM V$ARCH_APPLY_INFO"
		var count int
		if err := db.QueryRowContext(ctx, query).Scan(&count); err != nil {
			logger.Logger.Warn("V$ARCH_APPLY_INFO not accessible, fallback to alternative query", zap.Error(err))
			viewArchApplyInfoExists = false
			return
		}
		logger.Logger.Debugf("V$ARCH_APPLY_INFO accessible")
		viewArchApplyInfoExists = true
	})
	return viewArchApplyInfoExists
}
