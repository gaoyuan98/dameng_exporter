# Dameng Exporter 参数配置说明

## 目录
- [配置方式](#配置方式)
- [参数优先级](#参数优先级)
- [全局系统参数](#全局系统参数)
- [数据源参数](#数据源参数)
- [特殊功能参数](#特殊功能参数)
- [配置文件示例](#配置文件示例)
- [命令行使用示例](#命令行使用示例)
- [注意事项](#注意事项)

## 配置方式

Dameng Exporter 支持三种配置方式：

1. **TOML配置文件**（推荐）：支持多数据源配置，功能最完整
2. **命令行参数**：适合单数据源或临时调试
3. **混合模式**：配置文件 + 命令行参数覆盖

## 参数优先级

优先级从高到低：
```
命令行参数 > 配置文件 > 默认值
```

## 全局系统参数

### 服务配置

| 参数名称 | 命令行参数 | 配置文件字段 | 默认值 | 说明 |
|---------|-----------|-------------|-------|------|
| 配置文件路径 | `--configFile` | - | `./dameng_exporter.toml` | TOML格式配置文件路径 |
| 监听地址 | `--listenAddress` | `listenAddress` | `:9200` | HTTP服务监听地址 |
| 指标路径 | `--metricPath` | `metricPath` | `/metrics` | Prometheus指标暴露路径 |
| 版本号 | - | `version` | `v1.2.0` | 程序版本号（只读） |

### 日志配置

| 参数名称 | 命令行参数 | 配置文件字段 | 默认值 | 说明 |
|---------|-----------|-------------|-------|------|
| 日志级别 | `--logLevel` | `logLevel` | `info` | 日志级别：debug/info/warn/error |
| 日志文件大小 | `--logMaxSize` | `logMaxSize` | `10` | 单个日志文件最大大小(MB) |
| 日志备份数量 | `--logMaxBackups` | `logMaxBackups` | `3` | 保留的旧日志文件数量 |
| 日志保留天数 | `--logMaxAge` | `logMaxAge` | `30` | 日志文件保留天数 |

### 安全配置

| 参数名称 | 命令行参数 | 配置文件字段 | 默认值 | 说明 |
|---------|-----------|-------------|-------|------|
| 密码加密存储 | `--encodeConfigPwd` | `encodeConfigPwd` | `false` | 是否加密存储配置文件中的密码 |
| 启用Basic认证 | `--enableBasicAuth` | `enableBasicAuth` | `false` | 是否启用HTTP Basic认证 |
| Basic认证用户名 | `--basicAuthUsername` | `basicAuthUsername` | `""` | Basic认证用户名 |
| Basic认证密码 | `--basicAuthPassword` | `basicAuthPassword` | `""` | Basic认证密码（支持加密） |

### 性能配置

| 参数名称 | 命令行参数 | 配置文件字段 | 默认值 | 说明 |
|---------|-----------|-------------|-------|------|
| 全局超时时间 | `--globalTimeoutSeconds` | `globalTimeoutSeconds` | `5` | 全局采集超时时间（秒） |
| 采集模式 | `--collectionMode` | `collectionMode` | `blocking` | 采集模式：blocking(阻塞)/fast(快速)，详见[采集模式详解](#采集模式详解) |

## 数据源参数

### 基本信息

| 参数名称 | 命令行参数 | 配置文件字段 | 默认值 | 说明 |
|---------|-----------|-------------|-------|------|
| 数据源名称 | `--dbName` | `name` | 自动生成 | 数据源唯一标识名称 |
| 数据源描述 | - | `description` | `DataSource: {name}` | 数据源描述信息 |
| 是否启用 | - | `enabled` | `true` | 是否启用该数据源 |

### 数据库连接

| 参数名称 | 命令行参数 | 配置文件字段 | 默认值 | 说明 | 取值范围 |
|---------|-----------|-------------|-------|------|---------|
| 数据库地址 | `--dbHost` | `dbHost` | `127.0.0.1:5236` | 达梦数据库地址和端口 | - |
| 数据库用户名 | `--dbUser` | `dbUser` | `SYSDBA` | 数据库连接用户名 | - |
| 数据库密码 | `--dbPwd` | `dbPwd` | `SYSDBA` | 数据库连接密码（支持加密） | - |
| 查询超时时间 | `--queryTimeout` | `queryTimeout` | `30` | SQL查询超时时间（秒） | 1-300 |
| 最大打开连接数 | `--maxOpenConns` | `maxOpenConns` | `10` | 连接池最大打开连接数 | 1-100 |
| 最大空闲连接数 | `--maxIdleConns` | `maxIdleConns` | `2` | 连接池最大空闲连接数 | 1-100 |
| 连接最大生命周期 | `--connMaxLifetime` | `connMaxLifetime` | `30` | 连接最大生命周期（分钟） | >0 |

### 缓存配置

| 参数名称 | 命令行参数 | 配置文件字段 | 默认值 | 说明 |
|---------|-----------|-------------|-------|------|
| 大数据缓存时间 | `--bigKeyDataCacheTime` | `bigKeyDataCacheTime` | `60` | 大数据量查询缓存时间（分钟），详见[缓存机制说明](#缓存机制说明) |
| 告警缓存时间 | `--alarmKeyCacheTime` | `alarmKeyCacheTime` | `5` | 告警数据缓存时间（分钟），详见[缓存机制说明](#缓存机制说明) |

### 慢SQL配置

| 参数名称 | 命令行参数 | 配置文件字段 | 默认值 | 说明 |
|---------|-----------|-------------|-------|------|
| 检查慢SQL | `--checkSlowSql` | `checkSlowSQL` | `false` | 是否启用慢SQL检查 |
| 慢SQL阈值 | `--slowSqlTime` | `slowSqlTime` | `10000` | 慢SQL时间阈值（毫秒） |
| 慢SQL返回行数 | `--slowSqlLimitRows` | `slowSqlMaxRows` | `10` | 慢SQL查询返回的最大行数 |

### 指标采集开关

| 参数名称 | 命令行参数 | 配置文件字段 | 默认值 | 说明 |
|---------|-----------|-------------|-------|------|
| 主机指标 | `--registerHostMetrics` | `registerHostMetrics` | `false` | 是否采集主机指标 |
| 数据库指标 | `--registerDatabaseMetrics` | `registerDatabaseMetrics` | `true` | 是否采集数据库指标 |
| DMHS指标 | `--registerDmhsMetrics` | `registerDmhsMetrics` | `false` | 是否采集DMHS同步指标 |
| 自定义指标 | `--registerCustomMetrics` | `registerCustomMetrics` | `true` | 是否采集自定义指标 |

### 其他配置

| 参数名称 | 命令行参数 | 配置文件字段 | 默认值 | 说明 |
|---------|-----------|-------------|-------|------|
| 标签配置 | - | `labels` | `""` | 额外标签，格式：`key1=val1,key2=val2` |
| 自定义指标文件 | - | `customMetricsFile` | `./custom_queries.metrics` | 自定义指标配置文件路径 |

## 特殊功能参数

### 密码加密工具

| 参数名称 | 命令行参数 | 说明 |
|---------|-----------|------|
| 加密密码 | `--encryptPwd` | 加密指定密码并退出，输出格式：`ENC(加密后的密码)` |
| 加密Basic认证密码 | `--encryptBasicAuthPwd` | 加密Basic认证密码并退出 |

## 配置文件示例

### 最小配置示例

```toml
# 最小配置 - 使用所有默认值
[[datasource]]
name = "dm_prod"
dbHost = "192.168.1.100:5236"
dbUser = "SYSDBA"
dbPwd = "SYSDBA"
```

### 完整配置示例

```toml
# 全局配置
listenAddress = ":9200"
metricPath = "/metrics"
logLevel = "info"
logMaxSize = 10
logMaxBackups = 3
logMaxAge = 30
encodeConfigPwd = true
enableBasicAuth = false
basicAuthUsername = "admin"
basicAuthPassword = "ENC(encrypted_password_here)"
globalTimeoutSeconds = 5
collectionMode = "blocking"

# 数据源1 - 生产环境
[[datasource]]
name = "dm_prod"
description = "生产环境达梦数据库"
enabled = true
dbHost = "192.168.1.100:5236"
dbUser = "SYSDBA"
dbPwd = "ENC(encrypted_password)"
queryTimeout = 30
maxOpenConns = 10
maxIdleConns = 2
connMaxLifetime = 30
bigKeyDataCacheTime = 60
alarmKeyCacheTime = 5
checkSlowSQL = true
slowSqlTime = 5000
slowSqlMaxRows = 20
registerHostMetrics = true
registerDatabaseMetrics = true
registerDmhsMetrics = false
registerCustomMetrics = true
labels = "env=production,region=cn-north"
customMetricsFile = "./custom_prod.metrics"

# 数据源2 - 测试环境
[[datasource]]
name = "dm_test"
description = "测试环境达梦数据库"
enabled = true
dbHost = "192.168.1.101:5236"
dbUser = "TEST_USER"
dbPwd = "test_password"
queryTimeout = 20
maxOpenConns = 5
maxIdleConns = 1
connMaxLifetime = 20
bigKeyDataCacheTime = 30
alarmKeyCacheTime = 3
checkSlowSQL = false
registerHostMetrics = false
registerDatabaseMetrics = true
registerDmhsMetrics = false
registerCustomMetrics = false
labels = "env=test,region=cn-north"
```

## 命令行使用示例

### 基本启动

```bash
# 使用默认配置文件
./dameng_exporter

# 指定配置文件
./dameng_exporter --configFile=/path/to/config.toml

# 纯命令行模式（单数据源）
./dameng_exporter \
  --dbHost=192.168.1.100:5236 \
  --dbUser=SYSDBA \
  --dbPwd=SYSDBA \
  --listenAddress=:9200
```

### 密码加密

```bash
# 加密数据库密码
./dameng_exporter --encryptPwd="your_password"
# 输出: ENC(encrypted_password_here)

# 加密Basic认证密码
./dameng_exporter --encryptBasicAuthPwd="auth_password"
# 输出: ENC(encrypted_auth_password)

# 使用加密后的密码启动
./dameng_exporter --dbPwd="ENC(encrypted_password_here)"
```

### 调试模式

```bash
# 启用调试日志
./dameng_exporter --logLevel=debug

# 快速采集模式（牺牲完整性换取响应速度）
./dameng_exporter --collectionMode=fast

# 自定义超时时间
./dameng_exporter --globalTimeoutSeconds=10 --queryTimeout=60
```

### 性能优化

```bash
# 增加连接池大小
./dameng_exporter \
  --maxOpenConns=20 \
  --maxIdleConns=5 \
  --connMaxLifetime=60

# 增加缓存时间
./dameng_exporter \
  --bigKeyDataCacheTime=120 \
  --alarmKeyCacheTime=10
```

## 注意事项

### 1. 参数验证规则

- **必填参数**（使用命令行时）：
  - 如果指定任一数据库参数，则必须同时指定：`dbHost`、`dbUser`、`dbPwd`
  - 否则将使用配置文件中的数据源配置

- **取值范围**：
  - `queryTimeout`: 1-300秒
  - `maxOpenConns`: 1-100
  - `maxIdleConns`: 1-100，且不应大于maxOpenConns
  - `globalTimeoutSeconds`: 建议不超过Prometheus的scrape_timeout

### 2. 密码安全

- 支持明文和加密两种方式
- 加密格式：`ENC(加密后的字符串)`
- 建议在生产环境使用加密密码
- 设置`encodeConfigPwd=true`可自动加密配置文件中的明文密码

### 3. 性能建议

#### 连接池配置
- **生产环境**：`maxOpenConns=10-20, maxIdleConns=2-5`
- **测试环境**：`maxOpenConns=5-10, maxIdleConns=1-2`
- **连接生命周期**：建议30-60分钟，避免长连接导致的问题

#### 缓存配置
- **大数据量查询**：建议缓存60-120分钟
- **告警数据**：建议缓存3-5分钟
- **实时性要求高**：可适当降低缓存时间

#### 采集模式选择
- **blocking模式**：确保数据完整性，适合正常监控
- **fast模式**：快速返回部分数据，适合大规模或网络不稳定环境
- 详细说明请参考[采集模式详解](#采集模式详解)

### 4. 多数据源配置

- 数据源名称必须唯一
- 同一主机地址不能被多个启用的数据源使用
- 可通过`enabled=false`临时禁用某个数据源
- 每个数据源可独立配置所有参数

### 5. 日志管理

- 日志文件自动轮转，无需手动清理
- Debug级别会输出详细的数据源配置信息
- 生产环境建议使用info或warn级别

### 6. 故障排查

如遇到问题，可按以下步骤排查：

1. 启用debug日志：`--logLevel=debug`
2. 检查配置验证输出
3. 查看连接池状态
4. 验证数据库连接：用户权限、网络连通性
5. 检查Prometheus抓取超时配置

### 7. 命令行参数特殊说明

- 命令行参数仅支持单数据源配置
- 如需多数据源，必须使用TOML配置文件
- 命令行参数会覆盖配置文件中的**所有**数据源（如果指定了数据库连接参数）

## 常见问题

### Q: 如何确认使用的是哪个配置？
A: 启动时会输出完整的配置摘要，包括所有生效的参数值。

### Q: 配置文件找不到怎么办？
A: 默认查找`./dameng_exporter.toml`，可通过`--configFile`指定其他路径。

### Q: 如何验证配置是否正确？
A: 配置加载时会自动验证，错误会在启动时报告。建议先用`--logLevel=debug`测试。

### Q: 密码加密后忘记原密码怎么办？
A: 加密是单向的，需要重新设置密码并重新加密。

### Q: 采集超时如何处理？
A: 调整`globalTimeoutSeconds`和`queryTimeout`，确保小于Prometheus的scrape_timeout。

## 采集模式详解

### Blocking模式（阻塞模式）

**特点**：
- 默认模式，优先保证数据完整性
- 等待所有采集器完成，即使超过超时时间
- 不会丢失任何指标数据

**工作原理**：
1. 启动所有采集器并发执行
2. 使用无缓冲channel传输指标数据
3. 即使超过`globalTimeoutSeconds`设定的超时时间，仍继续等待采集完成
4. 超时的采集器会被标记为"slow"并记录警告日志
5. 最终返回完整的指标数据给Prometheus

**适用场景**：
- 生产环境的常规监控
- 数据完整性要求高的场景
- 网络稳定、数据库响应正常的环境
- Prometheus scrape_timeout设置较宽松的情况

**日志示例**：
```
# 正常情况
INFO [dm_prod] database_metrics completed (blocking mode) | Cost: 100ms | Metrics: 500

# 超时但仍完成
WARN [dm_prod] database_metrics completed (slow, blocking mode) | Cost: 6000ms | Metrics: 500
```

### Fast模式（快速模式）

**特点**：
- 快速响应模式，优先保证响应时间
- 严格遵守超时限制，到时即返回
- 可能返回部分数据

**工作原理**：
1. 启动所有采集器并发执行
2. 使用大缓冲channel（容量500）传输指标数据
3. 严格遵守`globalTimeoutSeconds`设定的超时时间
4. 超时时立即停止等待，返回已收集到的部分数据
5. 未完成的采集器会被记录为"TIMEOUT"

**适用场景**：
- 大规模监控环境（数百个数据源）
- 网络不稳定或数据库响应慢的环境
- 对实时性要求高但可接受部分数据丢失
- Prometheus scrape_timeout设置较严格的情况
- 防止单个慢查询阻塞整个采集过程

**日志示例**：
```
# 正常完成
INFO [dm_test] database_metrics completed (fast mode) | Cost: 100ms | Metrics: 500

# 超时返回部分数据
WARN [dm_test] database_metrics TIMEOUT (fast mode) | Cost: 5000ms | Timeout: 5s | Metrics: 300 (partial)
```

### 模式对比

| 对比项 | Blocking模式 | Fast模式 |
|-------|-------------|----------|
| 数据完整性 | 100%保证 | 可能部分缺失 |
| 响应时间 | 可能超时 | 严格遵守超时 |
| Channel缓冲 | 无缓冲 | 500缓冲 |
| 超时处理 | 继续等待完成 | 立即返回 |
| 适用规模 | 中小规模 | 大规模 |
| 网络要求 | 稳定 | 可不稳定 |
| 默认选择 | ✓ | - |

### 配置建议

**选择Blocking模式的情况**：
- 监控的数据源少于10个
- 网络延迟低于100ms
- 数据库查询通常在2秒内完成
- Prometheus scrape_timeout >= 10秒

**选择Fast模式的情况**：
- 监控的数据源超过20个
- 存在跨地域或跨云的数据源
- 数据库偶尔出现慢查询（>5秒）
- Prometheus scrape_timeout <= 5秒

## 缓存机制说明

### BigKeyDataCacheTime（大数据缓存时间）

**作用范围**：
- **表空间数据缓存**：缓存表空间使用率查询结果，减少查询压力
- **数据文件信息缓存**：缓存数据文件状态信息，避免频繁扫描

**具体应用**：

1. **表空间使用率缓存**：
   - 缓存表空间总容量和剩余空间查询结果
   - 缓存键：`dmdbms_tablespace_size_total_info_{datasource_name}`
   - 避免频繁查询系统视图 `V$TABLESPACE`
   - 适合缓存时间较长，因为表空间容量变化通常较慢

2. **数据文件信息缓存**：
   - 缓存数据文件路径、大小、自动扩展等信息
   - 缓存键：`dmdbms_tablespace_file_total_info_{datasource_name}`
   - 避免频繁查询系统视图 `V$DATAFILE`
   - 数据文件配置信息相对稳定，适合较长缓存

**工作机制**：
- 查询结果JSON序列化后存储
- 缓存命中时直接返回，避免数据库查询
- 缓存过期后重新执行SQL查询并更新缓存

**配置建议**：
- **生产环境**：60-120分钟（表空间和文件信息变化缓慢）
- **测试环境**：30-60分钟（可能有频繁的表空间操作）
- **开发环境**：10-30分钟（便于测试和调试）

### AlarmKeyCacheTime（告警缓存时间）

**作用范围**：
- **主备切换检测**：控制模式基准值缓存和告警持续时间
- **短期状态缓存**：用于需要较短缓存时间的状态信息

**具体应用**：

1. **主备切换模式基准值缓存**：
   - 记录数据库运行模式作为对比基准
   - 缓存键：`AlarmSwitchStr_{datasource_name}`
   - 缓存时间为 `AlarmKeyCacheTime × 2`，在稳定性和灵敏度间取得平衡
   - 基准值过期后会重新记录当前模式

2. **主备切换告警持续时间**：
   - 控制切换告警的持续显示时间
   - 缓存键：`AlarmSwitchOccur_{datasource_name}`
   - 确保告警在指定时间内持续显示
   - 告警期满后恢复正常状态

**工作机制**：
- 模式基准：存储数据库运行模式值（1=主库，2=备库等）
- 告警标记：设置告警标志，期间内保持告警状态
- 双重缓存配合实现智能的主备切换检测

**配置建议**：
- **快速响应**：3-5分钟（需要快速检测切换并快速恢复）
- **常规监控**：5-10分钟（平衡检测灵敏度和告警持续性）
- **稳定环境**：10-15分钟（减少误报，适合稳定的生产环境）

### 缓存策略总结

| 参数 | 主要用途 | 缓存内容 | 建议时间 |
|-----|---------|---------|---------|
| **BigKeyDataCacheTime** | 大数据量查询缓存 | 表空间信息、数据文件信息 | 30-120分钟 |
| **AlarmKeyCacheTime** | 状态检测与告警 | 主备切换基准值、告警标记 | 5-15分钟 |

### 优化建议

1. **合理设置缓存时间**：
   - `BigKeyDataCacheTime` > `AlarmKeyCacheTime`（通常2-10倍）
   - 表空间查询耗时较长，适合长缓存
   - 主备切换基准值自动使用 `AlarmKeyCacheTime × 2`，平衡稳定性和灵敏度
   - 告警持续时间使用 `AlarmKeyCacheTime`，确保及时恢复

2. **根据环境调整**：
   - 稳定环境：加大缓存时间，减少查询压力
   - 测试环境：缩短缓存时间，提高实时性
   - 大规模部署：优先考虑性能，适当延长缓存

3. **监控缓存效果**：
   - 观察日志中的 "Use cache" 信息
   - 监控数据库查询频率
   - 根据实际情况调整参数

### 缓存性能影响

**内存占用**：
- 每个缓存项约占用：键(50字节) + 值(平均500字节) + 元数据(50字节) ≈ 600字节
- 100个数据源，每个20个缓存项：约12MB内存

**查询性能提升**：
- 缓存命中时响应时间：< 1ms
- 无缓存查询时间：100ms - 5s
- 性能提升：100-5000倍

**缓存策略优化**：
```toml
# 高性能配置（牺牲实时性）
bigKeyDataCacheTime = 120    # 2小时
alarmKeyCacheTime = 15        # 15分钟

# 平衡配置（默认）
bigKeyDataCacheTime = 60     # 1小时
alarmKeyCacheTime = 5         # 5分钟

# 高实时性配置（牺牲性能）
bigKeyDataCacheTime = 10     # 10分钟
alarmKeyCacheTime = 1         # 1分钟
```

### 缓存失效策略

1. **时间过期**：达到配置的缓存时间自动失效
2. **主动清除**：检测到状态变化时主动清除相关缓存
3. **内存压力**：使用LRU策略淘汰最少使用的缓存项

## 主备切换检测逻辑详解

### 检测机制概述

系统通过两个缓存键配合实现主备切换的智能检测和告警：

1. **`AlarmSwitchStr_{datasource}`**：存储数据库运行模式基准值（使用AlarmKeyCacheTime × 2）
2. **`AlarmSwitchOccur_{datasource}`**：标记切换事件已发生（使用AlarmKeyCacheTime）

### 检测流程

系统每次采集时执行以下判断逻辑：

#### Case 1: 切换告警持续期（switchOccurExists）
- **条件**：`AlarmSwitchOccur`缓存键存在
- **含义**：之前已检测到切换，当前处于告警持续期
- **动作**：继续输出告警状态（AlarmStatus_Unusual）
- **目的**：确保告警持续显示`AlarmKeyCacheTime`时间

#### Case 2: 模式未变化（modeExists && cachedMode == modeStr）
- **条件**：`AlarmSwitchStr`存在且值与当前模式相同
- **含义**：数据库运行模式保持稳定，未发生切换
- **动作**：输出正常状态（AlarmStatus_Normal）
- **目的**：确认系统运行正常

#### Case 3: 检测到切换（modeExists && cachedMode != modeStr）
- **条件**：`AlarmSwitchStr`存在但值与当前模式不同
- **含义**：刚刚发生了主备切换
- **动作**：
  1. 输出告警状态（AlarmStatus_Unusual）
  2. 删除旧的模式基准值
  3. 设置`AlarmSwitchOccur`标记，持续`AlarmKeyCacheTime`时间
- **目的**：触发切换告警并开始告警持续期

#### Case 4: 首次运行或基准过期（default）
- **条件**：`AlarmSwitchStr`不存在
- **含义**：系统首次启动或基准缓存已过期
- **动作**：
  1. 记录当前模式为基准值，缓存`AlarmKeyCacheTime × 2`时间
  2. 输出正常状态（AlarmStatus_Normal）
- **目的**：建立或更新基准，后续用于对比

### 时间线示例

```
时间轴 ────────────────────────────────────────────────>
假设 AlarmKeyCacheTime = 5分钟
基准缓存时间 = AlarmKeyCacheTime × 2 = 10分钟

T0: 系统启动
    └─> Case 4: 记录mode=1(主库)，缓存10分钟
    └─> 状态：正常

T0+5min: 常规检查
    └─> Case 2: mode=1，与缓存一致
    └─> 状态：正常

T0+10min: 基准过期
    └─> Case 4: 重新记录mode=1，缓存10分钟
    └─> 状态：正常

T0+15min: 发生主备切换，mode变为2(备库)
    └─> Case 3: 检测到mode变化(1→2)
    └─> 删除旧基准，设置告警标记(5分钟)
    └─> 状态：告警

T0+16min: 切换后检查
    └─> Case 1: 告警标记存在
    └─> 状态：告警（持续）

T0+20min: 告警期满
    └─> Case 4: 记录mode=2为新基准，缓存10分钟
    └─> 状态：正常
```

### 配置调优建议

#### 场景1：频繁切换环境
```toml
bigKeyDataCacheTime = 30      # 表空间缓存时间
alarmKeyCacheTime = 3          # 快速检测和恢复
```

#### 场景2：稳定生产环境
```toml
bigKeyDataCacheTime = 120     # 表空间长缓存
alarmKeyCacheTime = 10         # 稳定的检测周期
```

#### 场景3：性能优先环境
```toml
bigKeyDataCacheTime = 180     # 最大化缓存效果
alarmKeyCacheTime = 15         # 减少检测频率
```

---

*更多信息请参考项目文档或提交Issue：https://github.com/gaoyuan98/dameng_exporter*