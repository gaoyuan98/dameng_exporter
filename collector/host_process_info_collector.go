package collector

import (
	"context"
	"dameng_exporter/config"
	"dameng_exporter/utils"
	"database/sql"
	"fmt"
	"github.com/prometheus/client_golang/prometheus"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

// DBInstanceInfo 结构体，用于存储 SQL 查询结果
type DBInstanceInfo struct {
	DBInstancePath string
	PID            string
}

// 定义收集器结构体
type DmapProcessCollector struct {
	collectorLog
	db                   *sql.DB
	dmapProcessDesc      *prometheus.Desc
	dmserverProcessDesc  *prometheus.Desc
	dmwatcherProcessDesc *prometheus.Desc
	dmmonitorProcessDesc *prometheus.Desc
	dmagentProcessDesc   *prometheus.Desc
	localInstallBinPath  string
	lastPID              string
	//mutex                sync.Mutex
	dataSource    string                       // 数据源名称
	processOutput func(string) ([]byte, error) // optional command runner for controlled diagnostics tests
}

// SetDataSource 实现DataSourceAware接口
func (c *DmapProcessCollector) SetDataSource(name string) {
	c.dataSource = name
}

// 初始化收集器
func NewDmapProcessCollector(db *sql.DB) *DmapProcessCollector {
	return &DmapProcessCollector{
		db: db,
		dmapProcessDesc: prometheus.NewDesc(
			dmdbms_dmap_process_is_exit,
			"Information about DM database dmap process existence",
			[]string{},
			nil,
		),
		dmserverProcessDesc: prometheus.NewDesc(
			dmdbms_dmserver_process_is_exit,
			"Information about DM database dmserver process existence",
			[]string{},
			nil,
		),
		dmwatcherProcessDesc: prometheus.NewDesc(
			dmdbms_dmwatcher_process_is_exit,
			"Information about DM database dmwatcher process existence",
			[]string{},
			nil,
		),
		dmmonitorProcessDesc: prometheus.NewDesc(
			dmdbms_dmmonitor_process_is_exit,
			"Information about DM database dmmonitor process existence",
			[]string{},
			nil,
		),
		dmagentProcessDesc: prometheus.NewDesc(
			dmdbms_dmagent_process_is_exit,
			"Information about DM database dmagent process existence",
			[]string{},
			nil,
		),
	}
}

// Describe 方法
func (c *DmapProcessCollector) Describe(ch chan<- *prometheus.Desc) {
	ch <- c.dmapProcessDesc
	ch <- c.dmserverProcessDesc
	ch <- c.dmwatcherProcessDesc
	ch <- c.dmmonitorProcessDesc
	ch <- c.dmagentProcessDesc
}

// Collect 方法
func (c *DmapProcessCollector) Collect(ch chan<- prometheus.Metric) {

	if err := utils.CheckDBConnectionWithSource(c.db, c.dataSource, c.log()); err != nil {
		return
	}

	// 获取数据库实例信息
	dbInstanceInfo, err := c.getDbInstanceInfo(c.db)
	if err != nil {
		return
	}

	// 如果 PID 发生变化，则更新 localInstallBinPath
	//c.mutex.Lock()
	if c.lastPID != dbInstanceInfo.PID {
		c.localInstallBinPath, err = c.getLocalInstallBinPath(dbInstanceInfo.PID)
		if err != nil {
			c.businessError("business_validation", err)
			c.localInstallBinPath = ""
			return
		}
		c.lastPID = dbInstanceInfo.PID
	}
	//c.mutex.Unlock()

	// 检查各个进程
	ch <- prometheus.MustNewConstMetric(
		c.dmapProcessDesc,
		prometheus.GaugeValue,
		c.checkProcess(c.localInstallBinPath, dbInstanceInfo.PID, "dmap"),
	)
	ch <- prometheus.MustNewConstMetric(
		c.dmserverProcessDesc,
		prometheus.GaugeValue,
		c.checkProcess(c.localInstallBinPath, dbInstanceInfo.PID, "dmserver"),
	)
	ch <- prometheus.MustNewConstMetric(
		c.dmwatcherProcessDesc,
		prometheus.GaugeValue,
		c.checkProcess(c.localInstallBinPath, dbInstanceInfo.PID, "dmwatcher"),
	)
	ch <- prometheus.MustNewConstMetric(
		c.dmmonitorProcessDesc,
		prometheus.GaugeValue,
		c.checkProcess(c.localInstallBinPath, dbInstanceInfo.PID, "dmmonitor"),
	)
	ch <- prometheus.MustNewConstMetric(
		c.dmagentProcessDesc,
		prometheus.GaugeValue,
		c.checkProcess(c.localInstallBinPath, dbInstanceInfo.PID, "dmagent"),
	)

}

// 检查进程
func (c *DmapProcessCollector) checkProcess(installBinPath, pid, processName string) float64 {
	if installBinPath == "" {
		return 0
	}

	if installBinPath[len(installBinPath)-1:] == "/" || installBinPath[len(installBinPath)-1:] == "\\" {
		installBinPath = installBinPath[:len(installBinPath)-1]
	}

	var shellStr string
	if processName == "dmap" {
		shellStr = fmt.Sprintf("ps -ef | grep %s/dmap | grep -v grep | wc -l", installBinPath)
	} else if processName == "dmserver" {
		shellStr = fmt.Sprintf("ps -ef | grep %s | grep dm.ini | grep -v grep | wc -l", pid)
	} else {
		shellStr = fmt.Sprintf("ps -ef | grep %s/%s | grep -v grep | wc -l", installBinPath, processName)
	}

	var output []byte
	var err error
	if c.processOutput != nil {
		output, err = c.processOutput(shellStr)
	} else {
		output, err = exec.Command("sh", "-c", shellStr).Output()
	}
	if err != nil {
		c.businessError("business_validation", err)
		return 0
	}

	processCount := strings.TrimSpace(string(output))
	count, err := strconv.ParseFloat(processCount, 64)
	if err != nil {
		c.businessError("business_validation", err)
		return 0
	}

	return count
}

// 获取数据库实例信息
func (c *DmapProcessCollector) getDbInstanceInfo(db *sql.DB) (DBInstanceInfo, error) {
	var info DBInstanceInfo

	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(config.Global.GetQueryTimeout())*time.Second)
	defer cancel()

	query := `
		SELECT /*+DMDB_CHECK_FLAG*/ PARA_VALUE AS DB_INSTANCE_PATH, (SELECT PID from V$PROCESS) PID
		FROM V$DM_INI
		WHERE PARA_NAME = 'CONFIG_PATH'
	`
	row := c.database(db).QueryRowContext(ctx, query)
	err := row.Scan(&info.DBInstancePath, &info.PID)
	if err != nil {
		return info, err
	}
	return info, nil
}

// 获取 localInstallBinPath
func (c *DmapProcessCollector) getLocalInstallBinPath(pid string) (string, error) {
	cmd := exec.Command("ls", "-l", fmt.Sprintf("/proc/%s/cwd", pid))
	output, err := cmd.Output()
	if err != nil {
		return "", err
	}
	c.log().Debugf("exec %v: %v", fmt.Sprintf("/proc/%s/cwd", pid), string(output))
	procStr := strings.TrimSpace(string(output))
	lineList := strings.Fields(procStr)
	lastElement := lineList[len(lineList)-1]
	c.log().Debugf("lastElement %v", lastElement)

	if strings.Contains(lastElement, "bin") {
		return lastElement, nil
	}

	shellStr := fmt.Sprintf("ls -l %s/dmserver | wc -l", lastElement)
	output, err = exec.Command("sh", "-c", shellStr).Output()
	c.log().Debugf("exec %v: %v", shellStr, string(output))
	if err != nil {
		return "", err
	}
	serverCount := strings.TrimSpace(string(output))
	if serverCount == "1" {
		return lastElement, nil
	}
	return "", fmt.Errorf("failed to get localInstallBinPath")
}
