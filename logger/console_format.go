package logger

import (
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"unicode"

	"go.uber.org/zap/zapcore"
)

type consoleEvent struct {
	fields map[string]interface{}
	used   map[string]bool
	parts  []string
}

func (v *consoleEvent) take(k string) (interface{}, bool) {
	x, ok := v.fields[k]
	if ok {
		v.used[k] = true
	}
	return x, ok
}
func (v *consoleEvent) add(k, label string) {
	if x, ok := v.take(k); ok {
		v.parts = append(v.parts, label+" "+displayValue(x))
	}
}
func (v *consoleEvent) duration(k, label string) {
	if x, ok := v.take(k); ok {
		value := "无限制"
		if x != nil {
			value = formatMilliseconds(x)
		}
		v.parts = append(v.parts, label+" "+value)
	}
}
func plainValue(x interface{}) string {
	if s, ok := x.(string); ok {
		return s
	}
	return fmt.Sprint(x)
}
func displayValue(x interface{}) string {
	if x == nil {
		return "未知"
	}
	switch n := x.(type) {
	case int:
		return strconv.Itoa(n)
	case int64:
		return strconv.FormatInt(n, 10)
	case uint64:
		return strconv.FormatUint(n, 10)
	case bool:
		return strconv.FormatBool(n)
	}
	s, ok := x.(string)
	if !ok {
		if b, err := json.Marshal(x); err == nil {
			s = string(b)
		} else {
			s = plainValue(x)
		}
	}
	if strings.ContainsAny(s, "|\"\\") || strings.IndexFunc(s, unicode.IsControl) >= 0 || strings.ContainsAny(s, "\u2028\u2029") {
		return strconv.Quote(s)
	}
	return s
}
func formatMilliseconds(x interface{}) string {
	var n int64
	switch v := x.(type) {
	case int64:
		n = v
	case int:
		n = int64(v)
	case float64:
		n = int64(v)
	default:
		return displayValue(x) + "ms"
	}
	if n < 1000 {
		return strconv.FormatInt(n, 10) + "ms"
	}
	whole, frac := n/1000, n%1000
	if frac == 0 {
		return strconv.FormatInt(whole, 10) + "s"
	}
	return fmt.Sprintf("%d.%ss", whole, strings.TrimRight(fmt.Sprintf("%03d", frac), "0"))
}
func shortScrape(id string) string {
	if i := strings.LastIndexByte(id, '-'); i > 0 && strings.Contains(id[:i], "T") {
		if _, err := strconv.ParseUint(id[i+1:], 10, 64); err == nil {
			return "S" + id[i+1:]
		}
	}
	return id
}
func (v *consoleEvent) reference() string {
	x, ok := v.take("scrape_id")
	if !ok {
		return ""
	}
	ref := shortScrape(plainValue(x))
	for _, k := range []string{"run_id", "query_id"} {
		if x, ok := v.take(k); ok {
			ref += "/" + plainValue(x)
		}
	}
	if x, ok := v.take("snapshot_id"); ok {
		ref += "/P" + plainValue(x)
	}
	return displayValue(ref)
}
func (v *consoleEvent) location() { v.add("datasource", "数据源"); v.add("collector", "采集器") }

var consoleNames = map[string]string{
	"ok": "成功", "empty": "空结果", "error": "失败", "pending": "待定",
	"running": "执行中", "finished": "已结束", "detached": "前台已截止", "skipped": "已跳过",
	"query": "查询", "next": "读取下一行", "scan": "解析当前行", "row_scan": "读取首行",
	"err": "检查读取错误", "close": "关闭结果", "columns": "读取列名", "column_types": "读取列类型",
	"between_calls": "调用间等待", "done": "已结束", "partial": "读取不完整", "unconfirmed": "读取未确认",
	"not_started": "尚未读取", "reading": "读取中", "first_row": "首行", "first_result_set": "首个结果集",
	"deadline_exceeded": "启动时预算已耗尽", "canceled": "启动时已取消",
	"database": "数据库", "cache": "缓存", "worker": "采集执行", "forwarder": "样本转发",
	"preflight": "采集前检查", "read_rows": "读取结果", "process": "处理采集", "metric_forward": "转发指标",
	"error_handling": "处理错误", "health_check": "健康检查", "pool_close": "关闭连接池",
}

func (v *consoleEvent) translated(k, label string) {
	if x, ok := v.take(k); ok {
		s := plainValue(x)

		if t, ok := consoleNames[s]; ok {
			s = t
		}
		v.parts = append(v.parts, label+" "+displayValue(s))
	}
}
func (v *consoleEvent) abnormalities() {
	for _, item := range [][2]string{
		{"issue_runs", "异常任务"}, {"sql_error_calls", "SQL失败"}, {"frontend_timeout_runs", "前台超时"},
		{"dropped_samples", "丢弃样本"}, {"unavailable_datasource_count", "数据源不可用"},
		{"sql_unfinished_at_cutoff", "截止时未完成SQL"}, {"skipped_runs", "跳过任务"},
	} {
		v.add(item[0], item[1])
	}
}
func formatConsole(e zapcore.Entry, fields map[string]interface{}) string {
	v := &consoleEvent{fields: fields, used: make(map[string]bool, len(fields)), parts: make([]string, 0, 12)}
	title := e.Message
	detailSQL := false
	switch e.Message {
	case "scrape_started":
		title = "开始采集"
	case "scrape_finished":
		title = "采集完成"
		v.duration("collection_duration_ms", "耗时")
		v.add("runs", "采集任务")
		v.add("sql_calls", "SQL")
		v.add("samples", "样本")
		v.abnormalities()
	case "scrape_diagnostic":
		title = "采集耗时达到阈值"
		v.duration("collection_duration_ms", "已耗时")
		v.duration("threshold_ms", "阈值")
		v.add("runs_active", "未完成任务")
		v.add("sql_active", "活跃SQL")
		v.add("active_stages", "活动阶段")
		v.abnormalities()
		for _, k := range []string{"runs", "sql_calls", "samples"} {
			if _, exists := fields[k]; exists {
				v.used[k] = true
			}
		}
	case "sql_started":
		title = "SQL开始"
		v.location()
		v.add("query_name", "查询")
		v.add("query_index", "查询序号")
		v.duration("budget_remaining_ms_at_start", "启动时剩余预算")
	case "sql_finished":
		title = "SQL完成"
		v.translated("result", "结果")
		v.duration("sql_duration_ms", "API耗时")
		v.duration("sql_wall_ms", "总历时")
		v.add("rows_read", "读取行数")
		v.translated("read_scope", "读取范围")
		v.translated("read_state", "读取状态")
		v.translated("error_stage", "失败阶段")
	case "sql_error", "sql_probe_expected":
		title = "SQL执行失败"
		if e.Message == "sql_probe_expected" {
			title = "SQL兼容探测未通过"
		}
		v.location()
		v.translated("error_stage", "阶段")
		v.duration("sql_duration_ms", "API耗时")
		v.add("error", "原因")
		v.duration("sql_wall_ms", "总历时")
		v.translated("error_stage_observed", "本次错误阶段")
		if fields["error_stage"] != "query" {
			v.translated("read_state", "读取状态")
			v.add("rows_read", "读取行数")
		} else {
			v.used["read_state"] = true
			v.used["rows_read"] = true
		}
		detailSQL = true
		for _, k := range []string{"timeout", "context_expired"} {
			if x, ok := v.take(k); ok && x == true {
				label := "SQL超时"
				if k == "context_expired" {
					label = "上下文预算已耗尽"
				}
				v.parts = append(v.parts, label)
			}
		}
		// Budget matters for timeout/cancellation; otherwise keep it in the file.
		if fields["timeout"] == true || fields["context_expired"] == true || fields["context_state_at_start"] != nil {
			v.duration("budget_remaining_ms_at_start", "启动时剩余预算")
		}
	case "sql_top":
		title = "SQL耗时排名"
		if rank, ok := v.take("rank"); ok {
			title += " #" + fmt.Sprint(rank)
		}
		v.location()
		v.duration("sql_duration_ms", "API耗时")
		v.duration("sql_wall_ms", "总历时")
		v.translated("state", "状态")
		v.translated("result", "结果")
		if fields["state"] == "running" || fields["state"] == "detached" {
			v.translated("sql_phase", "阶段")
		} else {
			v.used["sql_phase"] = true
		}
		v.translated("error_stage", "失败阶段")
		v.add("rows_read", "读取行数")
		v.duration("budget_remaining_ms_at_start", "启动时剩余预算")
		v.translated("read_state", "读取状态")
		detailSQL = true
	case "collector_started":
		title = "采集器开始"
		v.location()
		v.add("mode", "模式")
		v.duration("timeout_ms", "前台预算")
	case "collector_finished":
		title = "采集器完成"
		v.location()
		v.duration("duration_ms", "耗时")
		v.add("sql_calls", "SQL")
		v.add("samples", "样本")
		v.translated("run_state", "状态")
		v.add("skip_reason", "跳过原因")
		v.translated("data_origin", "数据来源")
		v.add("cache_fallback", "缓存回退")
		v.add("issue", "异常")
		v.add("sql_error_calls", "SQL失败")
		v.add("dropped_samples", "丢弃样本")
	case "collector_stage_started", "collector_stage_finished":
		title = "采集阶段开始"
		if e.Message == "collector_stage_finished" {
			title = "采集阶段结束"
		}
		v.translated("flow", "执行流")
		v.translated("stage", "阶段")
	case "collector_error":
		title = "采集处理失败"
		v.location()
		v.add("reason", "原因类型")
		v.add("error", "原因")
	case "collector_panic":
		title = "采集发生异常"
		v.location()
		v.add("panic", "原因")
		v.used["stack"] = true
	case "collector_timeout":
		title = "采集前台超时"
		v.location()
		v.duration("timeout_ms", "前台预算")
		v.parts = append(v.parts, "后台结束待确认")
	case "collector_background_finished":
		title = "采集后台结束"
		v.location()
		v.duration("background_duration_ms", "启动后历时")
		v.add("background_sql_calls", "后台SQL")
		v.add("background_sql_error_calls", "后台SQL失败")
	case "exporter_starting":
		title = "启动服务"
		v.add("version", "版本")
	case "config_loaded":
		title = "加载配置"
		v.add("enabled_datasources", "启用数据源")
		v.add("collection_mode", "采集模式")
		v.duration("threshold_ms", "慢采集阈值")
	case "datasource_ready":
		title = "数据源就绪"
		v.add("datasource", "数据源")
		v.add("node_type", "节点类型")
	case "collectors_registered":
		title = "注册完成"
		v.add("collectors", "采集器")
	case "custom_metrics_loaded":
		title = "自定义指标加载完成"
		v.add("datasource", "数据源")
		v.add("definitions", "定义数")
	case "server_listening":
		title = "启动监听"
		if _, ok := fields["url"]; ok {
			v.add("url", "指标地址")
			v.take("address")
			v.take("metric_path")
		} else {
			v.add("address", "地址")
			v.add("metric_path", "指标路径")
		}
	case "server_failed":
		title = "监听失败"
		v.add("address", "地址")
		v.add("error", "原因")
	case "job_metrics_unavailable":
		title = "任务指标未采集"
		v.location()
		v.parts = append(v.parts, "请检查任务功能配置")
	}
	if strings.HasPrefix(e.Message, "sql_") {
		for _, item := range [][2]string{{"query_name", "查询"}, {"query_index", "查询序号"}} {
			if !v.used[item[0]] {
				v.add(item[0], item[1])
			}
		}
	}
	known := title != e.Message
	// Fields deliberately reserved for file diagnostics, not the main display.
	if known {
		for _, k := range []string{"sql", "api_ms", "purpose", "read_scope", "start_offset_ms", "budget_remaining_ms_at_start", "scope", "datasource_label", "timeout_scope", "foreground_cutoff_at", "forwarding_finished_at", "snapshot_at", "trigger", "top_emitted_count", "file"} {
			if _, exists := fields[k]; exists {
				v.used[k] = true
			}
		}
		if e.Message == "sql_finished" {
			v.used["datasource"] = true
			v.used["collector"] = true
		}
		if e.Message == "collector_timeout" {
			v.used["mode"] = true
		}
	}
	if x, ok := fields["phase"]; ok && (known || x == "background") {
		v.used["phase"] = true
		if x == "background" && e.Message != "collector_background_finished" {
			v.parts = append(v.parts, "后台执行")
		} else if x != "collection" && x != "background" {
			v.add("phase", "阶段")
		}
	}
	v.translated("context_state_at_start", "启动状态")
	ref := v.reference()
	// Preserve new/unknown fields instead of silently losing future evidence.
	keys := make([]string, 0)
	for k := range fields {
		if !v.used[k] {
			keys = append(keys, k)
		}
	}
	sort.Strings(keys)
	for _, k := range keys {
		v.add(k, k)
	}
	if ref != "" {
		v.parts = append(v.parts, "关联 "+ref)
	}
	if e.LoggerName != "" {
		v.parts = append(v.parts, "日志 "+displayValue(e.LoggerName))
	}
	if e.Level == zapcore.DebugLevel && e.Caller.Defined {
		v.parts = append(v.parts, "位置 "+displayValue(e.Caller.TrimmedPath()))
	}
	prefix := e.Time.Format("2006-01-02 15:04:05.000") + fmt.Sprintf("  %-5s  ", e.Level.CapitalString())
	var line strings.Builder
	line.Grow(len(prefix) + 128)
	line.WriteString(prefix)
	line.WriteString(displayValue(title))
	if len(v.parts) > 0 {
		line.WriteString(" | ")
		line.WriteString(strings.Join(v.parts, " | "))
	}
	line.WriteByte('\n')
	if detailSQL {
		if sql, ok := fields["sql"]; ok {
			line.WriteString(prefix)
			line.WriteString("SQL详情")
			if ref != "" {
				line.WriteString(" | 关联 ")
				line.WriteString(ref)
			}
			line.WriteString(" | SQL ")
			line.WriteString(strconv.Quote(plainValue(sql)))
			line.WriteByte('\n')
		}
	}
	if stack, ok := fields["stack"]; ok && e.Message == "collector_panic" {
		line.WriteString(prefix)
		line.WriteString("异常堆栈")
		if ref != "" {
			line.WriteString(" | 关联 ")
			line.WriteString(ref)
		}
		line.WriteString(" | stack ")
		line.WriteString(strconv.Quote(plainValue(stack)))
		line.WriteByte('\n')
	}
	if e.Stack != "" {
		line.WriteString(prefix)
		line.WriteString("调用堆栈 | stack ")
		line.WriteString(strconv.Quote(e.Stack))
		line.WriteByte('\n')
	}

	return line.String()
}
