# Dameng Exporter 自定义指标使用指南

## 目录

- [概述](#概述)
- [快速开始](#快速开始)
- [配置文件格式](#配置文件格式)
- [参数详解](#参数详解)
- [实用示例](#实用示例)
  - [基础示例](#基础示例)
  - [性能监控](#性能监控)
  - [业务指标](#业务指标)
  - [安全审计](#安全审计)
- [高级用法](#高级用法)
- [最佳实践](#最佳实践)
- [常见问题](#常见问题)
- [调试技巧](#调试技巧)

## 概述

自定义指标功能允许用户通过 SQL 查询定义自己的监控指标，无需修改 Exporter 源代码。这为监控特定业务逻辑、定制化性能指标提供了极大的灵活性。

### 核心特性

- ✅ **灵活定义** - 通过 SQL 查询自由定义指标
- ✅ **实时生效** - 修改配置文件后自动加载
- ✅ **类型支持** - 支持 Counter 和 Gauge 两种指标类型
- ✅ **标签支持** - 支持多维度标签，便于数据聚合
- ✅ **兼容性好** - 与 oracledb_exporter 语法兼容

### 工作原理

```
custom_metrics.toml → SQL 执行 → 结果转换 → Prometheus 指标
```

## 快速开始

### 步骤 1：创建配置文件

在 Exporter 同级目录创建 `custom_metrics.toml` 文件：

```bash
touch custom_metrics.toml
chmod 644 custom_metrics.toml
```

### 步骤 2：添加第一个指标

```toml
[[metric]]
context = "database_size"
request = """
SELECT /*+DAMENG_EXPORTER*/ 
    ROUND(SUM(BYTES / 1048576 / 1024), 2) as size_gb 
FROM DBA_DATA_FILES
"""
metricsdesc = { size_gb = "Database total size in GB" }
```

### 步骤 3：验证指标

重启 Exporter 后访问 metrics 端点：

```bash
curl http://localhost:9200/metrics | grep database_size
# 输出: dmdbms_database_size_size_gb{host_name="dm01"} 1024.5
```

## 配置文件格式

### 基本结构

```toml
# 每个 [[metric]] 定义一个指标组
[[metric]]
context = "指标上下文名称"
request = "SQL 查询语句"
metricsdesc = { 列名 = "指标描述" }

# 可选参数
labels = ["标签列名1", "标签列名2"]
metricstype = { 列名 = "counter|gauge" }
ignorezeroresult = true|false
```

### 参数详解

#### 必填参数

| 参数 | 类型 | 说明 | 示例 |
|-----|------|------|------|
| `context` | string | 指标上下文，作为指标名称的一部分 | `"user_activity"` |
| `request` | string | SQL 查询语句 | `"SELECT COUNT(*) as count FROM V$SESSIONS"` |
| `metricsdesc` | map | 指标列的描述信息 | `{ count = "Active session count" }` |

#### 可选参数

| 参数 | 类型 | 默认值 | 说明 | 示例 |
|-----|------|--------|------|------|
| `labels` | array | `[]` | 作为标签的列名 | `["username", "status"]` |
| `metricstype` | map | `gauge` | 指标类型定义 | `{ total = "counter" }` |
| `ignorezeroresult` | bool | `false` | 是否忽略零值结果 | `true` |

### 指标命名规则

最终的 Prometheus 指标名称格式：
```
dmdbms_{context}_{column_name}
```

例如：
- context = `"session"`
- column = `"active_count"`
- 指标名 = `dmdbms_session_active_count`

## 实用示例

### 基础示例

#### 1. 简单计数指标

```toml
[[metric]]
context = "simple_count"
request = "SELECT /*+DAMENG_EXPORTER*/ COUNT(*) as total FROM V$SESSIONS"
metricsdesc = { total = "Total session count" }
```

#### 2. 带标签的指标

```toml
[[metric]]
context = "session_by_state"
labels = ["state_type"]
request = """
SELECT /*+DAMENG_EXPORTER*/
    DECODE(STATE, NULL, 'TOTAL', STATE) AS state_type,
    COUNT(SESS_ID) AS count
FROM V$SESSIONS
WHERE STATE IN ('IDLE', 'ACTIVE')
GROUP BY ROLLUP(STATE)
"""
metricsdesc = { count = "Session count by state" }
```

#### 3. 多值指标

```toml
[[metric]]
context = "database_stats"
request = """
SELECT /*+DAMENG_EXPORTER*/
    (SELECT COUNT(*) FROM V$SESSIONS WHERE STATE = 'ACTIVE') as active_sessions,
    (SELECT COUNT(*) FROM V$LOCK WHERE BLOCKED=1) as blocked_locks,
    (SELECT COUNT(*) FROM V$TRXWAIT) as trx_waits,
    (SELECT COUNT(*) FROM V$THREADS) as thread_count
FROM DUAL
"""
metricsdesc = { 
    active_sessions = "Active sessions",
    blocked_locks = "Blocked locks",
    trx_waits = "Transaction waits",
    thread_count = "Thread count"
}
```

### 性能监控

#### 1. 表空间使用率监控

```toml
[[metric]]
context = "tablespace_usage"
labels = ["tablespace_name"]
request = """
SELECT /*+DAMENG_EXPORTER*/ 
    F.TABLESPACE_NAME as tablespace_name,
    T.TOTAL_SPACE as total_mb,
    T.TOTAL_SPACE - F.FREE_SPACE as used_mb,
    F.FREE_SPACE as free_mb,
    ROUND((T.TOTAL_SPACE - F.FREE_SPACE) * 100.0 / T.TOTAL_SPACE, 2) as used_percent
FROM (
    SELECT TABLESPACE_NAME, 
           ROUND(SUM(BLOCKS * (SELECT PARA_VALUE / 1024 FROM V$DM_INI WHERE PARA_NAME = 'GLOBAL_PAGE_SIZE') / 1024)) FREE_SPACE
    FROM DBA_FREE_SPACE
    GROUP BY TABLESPACE_NAME
) F,
(
    SELECT TABLESPACE_NAME, 
           ROUND(SUM(BYTES / 1048576)) TOTAL_SPACE
    FROM DBA_DATA_FILES
    GROUP BY TABLESPACE_NAME
) T
WHERE F.TABLESPACE_NAME = T.TABLESPACE_NAME
"""
metricsdesc = {
    total_mb = "Tablespace total size in MB",
    used_mb = "Tablespace used size in MB", 
    free_mb = "Tablespace free size in MB",
    used_percent = "Tablespace usage percentage"
}
```

#### 2. SQL 执行统计

```toml
[[metric]]
context = "sql_statistics"
request = """
SELECT /*+DAMENG_EXPORTER*/ 
    NAME,
    STAT_VAL
FROM V$SYSSTAT 
WHERE NAME IN (
    'select statements',
    'insert statements',
    'delete statements',
    'update statements',
    'ddl statements',
    'transaction total count',
    'DB time(ms)',
    'parse time(ms)',
    'hard parse time(ms)',
    'logic read count',
    'physical read count',
    'physical write count'
)
"""
labels = ["name"]
metricsdesc = {
    stat_val = "SQL execution statistics value"
}
metricstype = {
    stat_val = "counter"
}
```

#### 3. 缓冲池命中率

```toml
[[metric]]
context = "buffer_pool_stats"
labels = ["pool_name"]
request = """
SELECT /*+DAMENG_EXPORTER*/ 
    NAME as pool_name,
    ROUND(SUM(RAT_HIT) / COUNT(*), 4) as hit_ratio
FROM V$BUFFERPOOL 
WHERE NAME = 'FAST' 
GROUP BY NAME
"""
metricsdesc = {
    hit_ratio = "Buffer pool hit ratio"
}
```

#### 4. 慢查询监控

```toml
[[metric]]
context = "slow_queries"
request = """
SELECT /*+DAMENG_EXPORTER*/ 
    COUNT(*) as slow_count,
    AVG(EXEC_TIME) as avg_time,
    MAX(EXEC_TIME) as max_time
FROM (
    SELECT DATEDIFF(MS, LAST_RECV_TIME, SYSDATE) as EXEC_TIME
    FROM V$SESSIONS
    WHERE STATE = 'ACTIVE'
        AND DATEDIFF(MS, LAST_RECV_TIME, SYSDATE) >= 5000
        AND LAST_RECV_TIME > TO_TIMESTAMP('2000-01-01 00:00:00', 'YYYY-MM-DD HH24:MI:SS')
)
"""
metricsdesc = {
    slow_count = "Current slow query count",
    avg_time = "Average slow query time in ms",
    max_time = "Maximum slow query time in ms"
}
```

### 业务指标

#### 1. 业务表数据量监控

```toml
[[metric]]
context = "business_table_rows"
labels = ["table_name"]
request = """
SELECT 
    'ORDERS' as table_name, COUNT(*) as row_count FROM ORDERS
UNION ALL
SELECT 
    'CUSTOMERS' as table_name, COUNT(*) as row_count FROM CUSTOMERS
UNION ALL
SELECT 
    'PRODUCTS' as table_name, COUNT(*) as row_count FROM PRODUCTS
"""
metricsdesc = { row_count = "Business table row count" }
```

#### 2. 订单状态统计

```toml
[[metric]]
context = "order_status"
labels = ["status"]
request = """
SELECT 
    STATUS as status,
    COUNT(*) as count,
    SUM(AMOUNT) as total_amount
FROM 
    ORDERS
WHERE 
    CREATE_TIME > SYSDATE - 1  -- 最近24小时
GROUP BY 
    STATUS
"""
metricsdesc = {
    count = "Order count by status",
    total_amount = "Total order amount by status"
}
```

#### 3. 用户活跃度统计

```toml
[[metric]]
context = "user_activity"
request = """
SELECT 
    COUNT(DISTINCT USER_ID) as daily_active_users,
    COUNT(*) as total_actions,
    AVG(RESPONSE_TIME) as avg_response_time
FROM 
    USER_ACTIVITY_LOG
WHERE 
    ACTION_TIME > SYSDATE - 1
"""
metricsdesc = {
    daily_active_users = "Daily active user count",
    total_actions = "Total user actions in 24h",
    avg_response_time = "Average response time in ms"
}
metricstype = { total_actions = "counter" }
```

### 安全审计

#### 1. 用户状态监控

```toml
[[metric]]
context = "user_status"
labels = ["username", "account_status"]
request = """
SELECT /*+DAMENG_EXPORTER*/
    A.USERNAME as username,
    CASE A.ACCOUNT_STATUS 
        WHEN 'LOCKED' THEN '锁定' 
        WHEN 'OPEN' THEN '正常' 
        ELSE '异常' 
    END AS account_status,
    TO_NUMBER(ROUND(DATEDIFF(DAY, SYSDATE, A.EXPIRY_DATE), 0)) AS expiry_days
FROM DBA_USERS A
WHERE A.USERNAME NOT IN ('SYS', 'SYSSSO', 'SYSAUDITOR')
    AND A.EXPIRY_DATE IS NOT NULL
"""
metricsdesc = { 
    expiry_days = "Days until user account expires" 
}
```

#### 2. 权限变更监控

```toml
[[metric]]
context = "privilege_changes"
request = """
SELECT 
    COUNT(*) as grant_count,
    COUNT(DISTINCT GRANTEE) as unique_grantees
FROM 
    V$AUDIT_RECORDS
WHERE 
    ACTION_NAME IN ('GRANT', 'REVOKE')
    AND TIMESTAMP > SYSDATE - INTERVAL '1' DAY
"""
metricsdesc = {
    grant_count = "Privilege change count in 24h",
    unique_grantees = "Unique grantees affected"
}
```

#### 3. 实例监控

```toml
[[metric]]
context = "instance_status"
request = """
SELECT /*+DAMENG_EXPORTER*/
    TO_CHAR(START_TIME, 'YYYY-MM-DD HH24:MI:SS') as start_time,
    CASE STATUS$ 
        WHEN 'OPEN' THEN 1 
        WHEN 'MOUNT' THEN 2 
        WHEN 'SUSPEND' THEN 3 
        ELSE 4 
    END AS status_code,
    CASE MODE$ 
        WHEN 'PRIMARY' THEN 1 
        WHEN 'NORMAL' THEN 2 
        WHEN 'STANDBY' THEN 3 
        ELSE 4 
    END AS mode_code,
    DATEDIFF(SQL_TSI_DAY, START_TIME, SYSDATE) as uptime_days
FROM V$INSTANCE
"""
metricsdesc = { 
    status_code = "Instance status (1=OPEN, 2=MOUNT, 3=SUSPEND, 4=OTHER)",
    mode_code = "Instance mode (1=PRIMARY, 2=NORMAL, 3=STANDBY, 4=OTHER)",
    uptime_days = "Instance uptime in days"
}
```

#### 4. 归档监控

```toml
[[metric]]
context = "archive_status"
labels = ["arch_type", "arch_dest"]
request = """
SELECT /*+DAMENG_EXPORTER*/ 
    ARCH_TYPE as arch_type,
    ARCH_DEST as arch_dest,
    CASE ARCH_STATUS 
        WHEN 'VALID' THEN 1 
        WHEN 'INVALID' THEN 0 
    END as status,
    ARCH_SRC as arch_source
FROM V$ARCH_STATUS
"""
metricsdesc = { 
    status = "Archive status (1=VALID, 0=INVALID)" 
}
```

## 高级用法

### 1. 动态阈值指标

```toml
[[metric]]
context = "dynamic_threshold"
request = """
WITH tablespace_info AS (
    SELECT /*+DAMENG_EXPORTER*/ 
        F.TABLESPACE_NAME,
        T.TOTAL_SPACE,
        F.FREE_SPACE,
        ROUND((T.TOTAL_SPACE - F.FREE_SPACE) * 100.0 / T.TOTAL_SPACE, 2) as usage_percent
    FROM (
        SELECT TABLESPACE_NAME, 
               ROUND(SUM(BLOCKS * (SELECT PARA_VALUE / 1024 FROM V$DM_INI WHERE PARA_NAME = 'GLOBAL_PAGE_SIZE') / 1024)) FREE_SPACE
        FROM DBA_FREE_SPACE
        GROUP BY TABLESPACE_NAME
    ) F,
    (
        SELECT TABLESPACE_NAME, 
               ROUND(SUM(BYTES / 1048576)) TOTAL_SPACE
        FROM DBA_DATA_FILES
        GROUP BY TABLESPACE_NAME
    ) T
    WHERE F.TABLESPACE_NAME = T.TABLESPACE_NAME
)
SELECT 
    TABLESPACE_NAME as tablespace_name,
    CASE 
        WHEN usage_percent > 90 THEN 2
        WHEN usage_percent > 80 THEN 1
        ELSE 0
    END as alert_level,
    usage_percent
FROM tablespace_info
WHERE usage_percent > 80
"""
labels = ["tablespace_name"]
metricsdesc = {
    alert_level = "Alert level (0=ok, 1=warning, 2=critical)",
    usage_percent = "Usage percentage"
}
```

### 2. 时间序列数据

```toml
[[metric]]
context = "hourly_stats"
labels = ["hour"]
request = """
SELECT 
    TO_CHAR(ACTION_TIME, 'HH24') as hour,
    COUNT(*) as transaction_count,
    AVG(RESPONSE_TIME) as avg_response_time,
    MAX(RESPONSE_TIME) as max_response_time
FROM 
    TRANSACTION_LOG
WHERE 
    ACTION_TIME > SYSDATE - 1
GROUP BY 
    TO_CHAR(ACTION_TIME, 'HH24')
"""
metricsdesc = {
    transaction_count = "Transaction count by hour",
    avg_response_time = "Average response time by hour",
    max_response_time = "Maximum response time by hour"
}
```

### 3. 复杂计算指标

```toml
[[metric]]
context = "complex_calculation"
request = """
WITH base_stats AS (
    SELECT /*+DAMENG_EXPORTER*/
        COUNT(*) as total_sessions,
        SUM(CASE WHEN STATE = 'ACTIVE' THEN 1 ELSE 0 END) as active_sessions,
        SUM(CASE WHEN STATE = 'IDLE' THEN 1 ELSE 0 END) as idle_sessions
    FROM V$SESSIONS
)
SELECT 
    total_sessions,
    active_sessions,
    idle_sessions,
    ROUND(active_sessions * 100.0 / NULLIF(total_sessions, 0), 2) as active_ratio,
    ROUND(idle_sessions * 100.0 / NULLIF(total_sessions, 0), 2) as idle_ratio
FROM base_stats
"""
metricsdesc = {
    total_sessions = "Total session count",
    active_sessions = "Active session count",
    idle_sessions = "Idle session count",
    active_ratio = "Active session percentage",
    idle_ratio = "Idle session percentage"
}
```

### 4. 条件指标

```toml
[[metric]]
context = "conditional_metrics"
request = """
SELECT /*+DAMENG_EXPORTER*/
    'replication_lag' as metric_name,
    CASE 
        WHEN EXISTS (SELECT 1 FROM V$INSTANCE WHERE MODE$ = 'STANDBY') THEN
            NVL((SELECT TIMESTAMPDIFF(SQL_TSI_SECOND, APPLY_CMT_TIME, LAST_CMT_TIME) 
                 FROM V$RAPPLY_STAT), 0)
        ELSE 
            0
    END as value
FROM DUAL
"""
labels = ["metric_name"]
metricsdesc = { value = "Replication lag in seconds" }
ignorezeroresult = true  # 主库时忽略零值
```

## 最佳实践

### 1. 性能优化

#### ✅ 推荐做法

```toml
# 使用索引列进行过滤
[[metric]]
context = "optimized_query"
request = """
SELECT /*+ INDEX(t idx_create_time) */
    COUNT(*) as count 
FROM transactions t
WHERE create_time > SYSDATE - INTERVAL '5' MINUTE
"""

# 限制返回行数
[[metric]]
context = "top_queries"
request = """
SELECT * FROM (
    SELECT username, COUNT(*) as count 
    FROM V$SESSIONS 
    GROUP BY username
    ORDER BY count DESC
) WHERE ROWNUM <= 10
"""

# 使用聚合函数减少数据传输
[[metric]]
context = "aggregated_stats"
request = """
SELECT 
    MIN(value) as min_val,
    MAX(value) as max_val,
    AVG(value) as avg_val,
    COUNT(*) as count
FROM large_table
"""
```

#### ❌ 避免做法

```toml
# 避免：全表扫描
request = "SELECT COUNT(*) FROM large_table"

# 避免：返回大量行
request = "SELECT * FROM V$SQL"

# 避免：复杂子查询
request = """
SELECT 
    (SELECT COUNT(*) FROM t1 WHERE t1.id = t2.id) as count
FROM t2
"""
```

### 2. 标签设计

#### 标签数量控制

```toml
# 好：控制标签基数
[[metric]]
context = "session_by_app"
labels = ["application"]  # 应用数量有限
request = """
SELECT 
    PROGRAM as application,
    COUNT(*) as count
FROM V$SESSIONS
GROUP BY PROGRAM
"""

# 避免：高基数标签
[[metric]]
context = "session_by_sql"
labels = ["sql_id"]  # SQL ID 数量可能非常多
request = """
SELECT 
    SQL_ID as sql_id,
    COUNT(*) as count
FROM V$SESSIONS
GROUP BY SQL_ID
"""
```

#### 标签命名规范

```toml
# 使用下划线分隔
labels = ["table_name", "index_name", "user_name"]

# 避免特殊字符
# 错误：labels = ["table-name", "index.name", "user@name"]
```

### 3. 错误处理

```toml
[[metric]]
context = "safe_division"
request = """
SELECT 
    numerator,
    denominator,
    CASE 
        WHEN denominator = 0 THEN 0
        ELSE ROUND(numerator * 100.0 / denominator, 2)
    END as percentage
FROM stats_table
"""
metricsdesc = { percentage = "Safe percentage calculation" }
```

### 4. 定期维护

```toml
# 添加注释说明
[[metric]]
# 监控目的：跟踪订单处理延迟
# 维护人：DBA Team
# 更新日期：2024-01-01
context = "order_processing_delay"
request = """
-- 计算订单平均处理时间
SELECT 
    AVG(PROCESS_TIME) as avg_delay
FROM ORDERS
WHERE STATUS = 'COMPLETED'
    AND COMPLETE_TIME > SYSDATE - INTERVAL '1' HOUR
"""
metricsdesc = { avg_delay = "Average order processing delay in seconds" }
```

## 常见问题

### Q1: 指标没有出现在 /metrics 端点

**可能原因：**
1. SQL 语法错误
2. 权限不足
3. 配置文件格式错误

**解决方法：**
```bash
# 检查日志
tail -f logs/dameng_exporter.log | grep custom

# 验证 SQL
./dameng_exporter --test-custom-metrics

# 检查权限
GRANT SELECT ON target_table TO prometheus_user;
```

### Q2: 指标值始终为 0

**可能原因：**
1. SQL 返回空结果集
2. 类型转换错误
3. ignorezeroresult 设置问题

**解决方法：**
```toml
# 确保返回非空值
[[metric]]
context = "safe_count"
request = """
SELECT COALESCE(COUNT(*), 0) as count 
FROM table_name
"""

# 设置忽略零值
ignorezeroresult = true
```

### Q3: 标签值显示为 null

**可能原因：**
1. 标签列返回 NULL 值
2. 列名大小写不匹配

**解决方法：**
```toml
# 处理 NULL 值
[[metric]]
labels = ["category"]
request = """
SELECT 
    COALESCE(category, 'unknown') as category,
    COUNT(*) as count
FROM products
GROUP BY category
"""
```

### Q4: 性能影响过大

**可能原因：**
1. 查询过于复杂
2. 扫描数据量过大
3. 缺少索引

**解决方法：**
```toml
# 优化查询
[[metric]]
context = "optimized_stats"
request = """
SELECT /*+ FIRST_ROWS(10) */
    category,
    COUNT(*) as count
FROM (
    SELECT category 
    FROM products 
    WHERE created_date > SYSDATE - 1
) 
GROUP BY category
"""
```

### Q5: Counter 类型指标不递增

**可能原因：**
1. 查询返回的不是累计值
2. 类型定义错误

**解决方法：**
```toml
# Counter 应返回累计值
[[metric]]
context = "total_transactions"
request = """
SELECT 
    SUM(transaction_count) as total  -- 累计值
FROM daily_stats
"""
metricstype = { total = "counter" }

# 而不是
request = """
SELECT 
    COUNT(*) as count  -- 当前值
FROM transactions
WHERE date = TODAY
"""
```

## 调试技巧

### 1. 启用调试日志

```bash
# 修改配置
logLevel = "debug"

# 查看自定义指标加载日志
grep "custom_metrics" logs/dameng_exporter.log
```

### 2. SQL 测试工具

```sql
-- 在数据库中直接测试 SQL
SET TIMING ON;
SET AUTOTRACE ON;

-- 执行自定义指标的 SQL
SELECT 
    ts.NAME as tablespace_name,
    ROUND((ts.TOTAL_SIZE - ts.FREE_SIZE) * 100.0 / ts.TOTAL_SIZE, 2) as used_percent
FROM V$TABLESPACE ts;

-- 查看执行计划和性能
```

### 3. 指标验证脚本

```bash
#!/bin/bash
# test_custom_metrics.sh

EXPORTER_URL="http://localhost:9200/metrics"
METRIC_PREFIX="dmdbms_"

echo "Testing custom metrics..."

# 获取所有自定义指标
curl -s $EXPORTER_URL | grep "^${METRIC_PREFIX}" | while read line; do
    metric_name=$(echo $line | cut -d'{' -f1)
    metric_value=$(echo $line | awk '{print $NF}')
    
    echo "Metric: $metric_name"
    echo "Value: $metric_value"
    
    # 检查值是否合理
    if [[ $metric_value == "NaN" ]] || [[ $metric_value == "null" ]]; then
        echo "WARNING: Invalid value for $metric_name"
    fi
    echo "---"
done
```

### 4. Prometheus 查询测试

```promql
# 查看所有自定义指标
{__name__=~"dmdbms_.*"}

# 验证标签
dmdbms_tablespace_usage_used_percent{tablespace_name="SYSTEM"}

# 检查指标更新
rate(dmdbms_total_transactions_total[5m])

# 聚合测试
sum by (status) (dmdbms_order_status_count)
```

### 5. Grafana 面板调试

```json
{
  "targets": [
    {
      "expr": "dmdbms_custom_metric_name",
      "legendFormat": "{{label_name}}",
      "interval": "30s",
      "format": "time_series"
    }
  ],
  "options": {
    "alerting": {
      "enabled": true,
      "threshold": 80
    }
  }
}
```

## 配置模板

### 完整配置示例

```toml
# custom_metrics.toml
# Dameng Exporter 自定义指标配置
# 版本: 1.0
# 更新时间: 2024-01-01

# ========================================
# 性能监控指标
# ========================================

[[metric]]
context = "performance_overview"
request = """
SELECT /*+DAMENG_EXPORTER*/
    (SELECT COUNT(*) FROM V$SESSIONS WHERE STATE = 'ACTIVE') as active_sessions,
    (SELECT COUNT(*) FROM V$LOCK WHERE BLOCKED = 1) as blocked_locks,
    (SELECT COUNT(*) FROM V$TRXWAIT) as trx_waits,
    (SELECT COUNT(*) FROM V$THREADS) as thread_count,
    (SELECT STAT_VAL FROM V$SYSSTAT WHERE NAME = 'DB time(ms)') as db_time_ms
FROM DUAL
"""
metricsdesc = {
    active_sessions = "Number of active sessions",
    blocked_locks = "Number of blocked locks",
    trx_waits = "Number of transaction waits",
    thread_count = "Number of threads",
    db_time_ms = "Database time in milliseconds"
}
metricstype = {
    db_time_ms = "counter"
}

# ========================================
# 表空间监控
# ========================================

[[metric]]
context = "tablespace_details"
labels = ["tablespace_name"]
request = """
SELECT /*+DAMENG_EXPORTER*/ 
    F.TABLESPACE_NAME as tablespace_name,
    ROUND(T.TOTAL_SPACE / 1024, 2) as total_gb,
    ROUND(F.FREE_SPACE / 1024, 2) as free_gb,
    ROUND((T.TOTAL_SPACE - F.FREE_SPACE) * 100.0 / T.TOTAL_SPACE, 2) as used_percent
FROM (
    SELECT TABLESPACE_NAME, 
           ROUND(SUM(BLOCKS * (SELECT PARA_VALUE / 1024 FROM V$DM_INI WHERE PARA_NAME = 'GLOBAL_PAGE_SIZE') / 1024)) FREE_SPACE
    FROM DBA_FREE_SPACE
    GROUP BY TABLESPACE_NAME
) F,
(
    SELECT TABLESPACE_NAME, 
           ROUND(SUM(BYTES / 1048576)) TOTAL_SPACE
    FROM DBA_DATA_FILES
    GROUP BY TABLESPACE_NAME
) T
WHERE F.TABLESPACE_NAME = T.TABLESPACE_NAME
"""
metricsdesc = {
    total_gb = "Tablespace total size in GB",
    free_gb = "Tablespace free size in GB",
    used_percent = "Tablespace usage percentage"
}

# ========================================
# 业务监控
# ========================================

[[metric]]
context = "business_metrics"
request = """
SELECT 
    (SELECT COUNT(*) FROM ORDERS WHERE CREATE_TIME > SYSDATE - INTERVAL '1' HOUR) as hourly_orders,
    (SELECT SUM(AMOUNT) FROM ORDERS WHERE CREATE_TIME > SYSDATE - INTERVAL '1' HOUR) as hourly_revenue,
    (SELECT COUNT(DISTINCT CUSTOMER_ID) FROM ORDERS WHERE CREATE_TIME > SYSDATE - INTERVAL '1' DAY) as daily_customers
FROM DUAL
"""
metricsdesc = {
    hourly_orders = "Orders created in last hour",
    hourly_revenue = "Revenue in last hour",
    daily_customers = "Unique customers in last 24 hours"
}
metricstype = {
    hourly_orders = "counter",
    hourly_revenue = "counter"
}

# ========================================
# 安全审计
# ========================================

[[metric]]
context = "security_audit"
labels = ["event_type"]
request = """
SELECT 
    ACTION_NAME as event_type,
    COUNT(*) as event_count
FROM 
    V$AUDIT_RECORDS
WHERE 
    TIMESTAMP > SYSDATE - INTERVAL '1' HOUR
    AND RESULT = 'FAILURE'
GROUP BY 
    ACTION_NAME
"""
metricsdesc = {
    event_count = "Failed security event count by type"
}
ignorezeroresult = true

# ========================================
# 自定义告警指标
# ========================================

[[metric]]
context = "custom_alerts"
request = """
SELECT /*+DAMENG_EXPORTER*/
    CASE 
        WHEN (SELECT COUNT(*) FROM V$LOCK WHERE BLOCKED = 1) > 0 THEN 1
        ELSE 0
    END as has_blocking_locks,
    CASE 
        WHEN (SELECT COUNT(*) FROM V$SESSIONS WHERE STATE = 'ACTIVE' 
              AND DATEDIFF(MS, LAST_RECV_TIME, SYSDATE) > 10000) > 0 THEN 1
        ELSE 0
    END as has_slow_queries,
    CASE 
        WHEN (SELECT COUNT(*) FROM V$SESSIONS) > 
             (SELECT PARA_VALUE FROM V$DM_INI WHERE PARA_NAME = 'MAX_SESSIONS') * 0.8 THEN 1
        ELSE 0
    END as high_session_count,
    CASE
        WHEN (SELECT COUNT(*) FROM V$TRXWAIT) > 10 THEN 1
        ELSE 0
    END as high_trx_waits
FROM DUAL
"""
metricsdesc = {
    has_blocking_locks = "Indicates if blocking locks exist (0=no, 1=yes)",
    has_slow_queries = "Indicates if slow queries exist (0=no, 1=yes)",
    high_session_count = "Indicates if session count is high (0=no, 1=yes)",
    high_trx_waits = "Indicates if transaction waits are high (0=no, 1=yes)"
}
```

## 与 Prometheus 集成

### Alert Rules 示例

```yaml
# prometheus_rules.yml
groups:
  - name: custom_metrics_alerts
    rules:
      - alert: HighTablespaceUsage
        expr: dmdbms_tablespace_usage_used_percent > 80
        for: 5m
        labels:
          severity: warning
        annotations:
          summary: "Tablespace {{ $labels.tablespace_name }} usage is high"
          description: "Tablespace {{ $labels.tablespace_name }} is {{ $value }}% full"
      
      - alert: SlowQueriesDetected
        expr: dmdbms_slow_queries_count > 10
        for: 5m
        labels:
          severity: warning
        annotations:
          summary: "High number of slow queries detected"
          description: "{{ $value }} slow queries in the last 10 minutes"
      
      - alert: BusinessOrdersDropped
        expr: rate(dmdbms_business_metrics_hourly_orders[1h]) < 10
        for: 10m
        labels:
          severity: critical
        annotations:
          summary: "Order rate has dropped significantly"
          description: "Order rate is {{ $value }} per hour"
```

### Grafana Dashboard JSON

```json
{
  "dashboard": {
    "title": "Custom Metrics Dashboard",
    "panels": [
      {
        "title": "Tablespace Usage",
        "targets": [
          {
            "expr": "dmdbms_tablespace_usage_used_percent",
            "legendFormat": "{{ tablespace_name }}"
          }
        ],
        "type": "graph",
        "yaxis": {
          "format": "percent",
          "max": 100
        }
      },
      {
        "title": "Business Metrics",
        "targets": [
          {
            "expr": "rate(dmdbms_business_metrics_hourly_orders[5m])",
            "legendFormat": "Orders/min"
          },
          {
            "expr": "rate(dmdbms_business_metrics_hourly_revenue[5m])",
            "legendFormat": "Revenue/min"
          }
        ],
        "type": "graph"
      }
    ]
  }
}
```

## 迁移指南

### 从 OracleDB Exporter 迁移

```toml
# oracledb_exporter 格式
[[metric]]
context = "sessions"
metricsdesc = { value="Gauge metric with metadata." }
request = "SELECT COUNT(*) as value FROM v$session"

# dameng_exporter 格式（基本兼容）
[[metric]]
context = "sessions"
metricsdesc = { value="Gauge metric with metadata." }
request = "SELECT COUNT(*) as value FROM V$SESSIONS"  # 注意表名差异
```

### 主要差异

1. **表名差异**
   - Oracle: `v$session`
   - DM: `V$SESSIONS`

2. **函数差异**
   - Oracle: `SYSDATE`
   - DM: `SYSDATE` (相同)
   - Oracle: `ROWNUM`
   - DM: `ROWNUM` (相同)

3. **数据类型**
   - 基本兼容，注意 NUMBER 精度差异

## 故障排除清单

### 检查步骤

- [ ] 1. 配置文件语法是否正确（TOML 格式）
- [ ] 2. SQL 语句是否有语法错误
- [ ] 3. 数据库用户是否有查询权限
- [ ] 4. 列名大小写是否匹配
- [ ] 5. 标签基数是否过高（建议 <100）
- [ ] 6. 查询性能是否影响数据库
- [ ] 7. 指标类型定义是否正确
- [ ] 8. 返回值是否为 NULL 或 NaN
- [ ] 9. Exporter 日志是否有错误信息
- [ ] 10. Prometheus 是否正确抓取指标

### 常用诊断命令

```bash
# 查看配置加载
grep "Loading custom metrics" logs/dameng_exporter.log

# 查看 SQL 执行错误
grep -i "error.*custom" logs/dameng_exporter.log

# 测试指标端点
curl -s http://localhost:9200/metrics | grep dmdbms_

# 统计自定义指标数量
curl -s http://localhost:9200/metrics | grep -c "^dmdbms_"

# 查看特定指标
curl -s http://localhost:9200/metrics | grep "dmdbms_tablespace"
```

## 性能基准

### 查询性能指南

| 查询复杂度 | 建议超时时间 | 最大返回行数 | 采集间隔 |
|-----------|-------------|-------------|---------|
| 简单查询 | < 1s | < 100 | 30s |
| 中等查询 | < 5s | < 1000 | 60s |
| 复杂查询 | < 10s | < 5000 | 300s |
| 重度查询 | < 30s | < 10000 | 600s |

### 资源占用估算

| 指标数量 | 内存占用 | CPU 占用 | 网络带宽 |
|---------|---------|----------|----------|
| 10 | ~5MB | <1% | <10KB/s |
| 50 | ~20MB | <2% | <50KB/s |
| 100 | ~50MB | <5% | <100KB/s |
| 500 | ~200MB | <10% | <500KB/s |

## 相关链接

- [Dameng Exporter 主文档](../README.md)
- [参数配置详解](./PARAMETERS_README.md)
- [Prometheus 查询语言](https://prometheus.io/docs/prometheus/latest/querying/basics/)
- [Grafana 面板配置](https://grafana.com/docs/grafana/latest/panels/)

---

*最后更新：2024-01-01*  
*维护者：Dameng Exporter Team*