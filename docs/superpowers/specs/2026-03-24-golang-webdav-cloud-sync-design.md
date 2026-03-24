# Golang WebDAV Cloud Sync Design

## 1. 文档信息

- 文档名称：Golang WebDAV Cloud Sync Design
- 文档版本：v1.0
- 编写日期：2026-03-24
- 对应 PRD：`docs/prd/2026-03-24-golang-webdav-cloud-sync-prd.md`
- 设计范围：单机部署、WebDAV-only、商用首版、稳定优先

## 2. 设计目标与约束

### 2.1 设计目标

1. 在单机环境中提供稳定、可恢复、可观测的本地文件系统与 WebDAV 双向/单向同步能力。
2. 采用单二进制部署，降低首版运维复杂度，同时保留后续拆分调度器与执行器的演进空间。
3. 通过明确的模块边界，将控制面、同步引擎、WebDAV 协议适配、加密模块和持久化层解耦。
4. 通过元数据驱动的同步模型保证任务重启、网络抖动、事件遗漏后的恢复能力。

### 2.2 已确认约束

1. 技术路线采用单二进制 Go 单体方案，不引入多进程或分布式架构。
2. HTTP 路由框架采用 `chi`。
3. 数据库采用 `SQLite`，以单机场景稳定性和部署简单性为优先。
4. 部署环境以 Linux 为正式支持目标，macOS/Windows 不纳入首版正式支持。
5. 管理端采用轻量 React Web 管理台。
6. 客户端加密从主设计阶段纳入边界设计，不作为后续临时拼接能力。
7. 产品目标优先级为“稳定与可恢复优先”，不是极限性能优先。

### 2.3 非目标

1. 首版不支持除 WebDAV 之外的其他远端协议。
2. 首版不做多租户和复杂 RBAC。
3. 首版不做分布式调度和多节点高可用。
4. 首版不做 WebSocket 为主的实时前端推送体系。

## 3. 总体架构

### 3.1 架构总览

系统采用单二进制单进程部署，对外暴露 HTTP API 和 React 管理台，对内拆分为控制面、执行面、持久化与可观测性三层。

1. 控制面：提供连接管理、任务管理、审计、系统配置、状态查询。
2. 执行面：处理本地监听、远端轮询、差异对账、同步动作规划、任务队列执行、重试与限流。
3. 持久化与可观测性：负责 SQLite 元数据存储、结构化日志、Prometheus 指标和健康检查。

### 3.2 核心设计原则

1. 单体部署，不等于代码耦合；模块边界必须按职责拆清。
2. 同步引擎只处理统一动作和状态推进，不感知 WebDAV 协议细节。
3. WebDAV connector 只负责远端访问、能力探测和错误归一化，不参与同步策略判断。
4. 所有同步判断依赖元数据库，不依赖瞬时本地或远端快照做二方判断。
5. 本地监听和远端轮询都是正式入口，补偿扫描不是异常旁路，而是稳定性设计的一部分。
6. 加密能力以独立模块接入，避免侵入同步引擎和协议适配层。

## 4. 模块划分

### 4.1 推荐目录边界

1. `cmd/server`
   程序入口、配置加载、依赖注入、HTTP 服务与后台任务生命周期管理。
2. `internal/api`
   `chi` 路由、HTTP handler、DTO、参数校验、鉴权中间件。
3. `internal/app`
   连接服务、任务服务、审计服务、运行时服务等应用服务层。
4. `internal/sync`
   同步引擎核心，负责差异比较、动作规划、冲突决策、状态推进。
5. `internal/connector/webdav`
   WebDAV 协议适配、能力探测、目录遍历、上传下载、错误归一化。
6. `internal/watcher`
   本地文件事件监听与补偿扫描。
7. `internal/scheduler`
   任务 runner、队列调度、并发控制、限流与重试。
8. `internal/store`
   SQLite 仓储、迁移、事务边界。
9. `internal/crypto`
   加解密、路径编码、密钥包装与元数据编码。
10. `internal/obs`
   日志、指标、健康检查。

### 4.2 模块协作原则

1. API 层不直接操作文件系统和 WebDAV。
2. scheduler 决定何时执行，sync 决定执行什么，connector 决定如何访问远端。
3. store 负责事实落盘，obs 负责事实可见，不承载业务决策。
4. crypto 模块作为变换层插入上传下载链路，不接触任务调度逻辑。

## 5. 数据库与核心数据模型

### 5.1 数据库选型原则

采用 `SQLite + WAL`。

原因：

1. 单机部署下运维最简单。
2. 支持可靠本地持久化和恢复。
3. 对连接配置、任务配置、文件索引、队列、失败记录等元数据场景足够。

### 5.2 核心表

#### `connections`

存储 WebDAV 连接定义与能力探测结果。

建议字段：

- `id`
- `name`
- `endpoint`
- `username`
- `password_ciphertext`
- `root_path`
- `tls_mode`
- `timeout_sec`
- `capabilities_json`
- `status`
- `created_at`
- `updated_at`

#### `tasks`

存储同步任务定义。

建议字段：

- `id`
- `name`
- `connection_id`
- `local_path`
- `remote_path`
- `direction`
- `poll_interval_sec`
- `conflict_policy`
- `delete_policy`
- `empty_dir_policy`
- `bandwidth_limit_kbps`
- `max_workers`
- `encryption_enabled`
- `hash_mode`
- `status`
- `desired_state`
- `last_error`
- `created_at`
- `updated_at`

#### `task_runtime_state`

存储任务运行期恢复信息与游标。

建议字段：

- `task_id`
- `phase`
- `last_local_scan_at`
- `last_remote_scan_at`
- `last_reconcile_at`
- `last_success_at`
- `last_event_seq`
- `backoff_until`
- `retry_streak`
- `checkpoint_json`
- `updated_at`

#### `file_index`

存储每个同步对象的最近一致基线，是系统的核心元数据库。

建议字段：

- `id`
- `task_id`
- `relative_path`
- `entry_type`
- `local_exists`
- `remote_exists`
- `local_size`
- `remote_size`
- `local_mtime`
- `remote_mtime`
- `local_file_id`
- `remote_etag`
- `content_hash`
- `last_sync_direction`
- `last_sync_at`
- `version`
- `sync_state`
- `conflict_flag`
- `deleted_tombstone`

#### `operation_queue`

存储待执行或重试中的动作，用于强恢复。

建议字段：

- `id`
- `task_id`
- `op_type`
- `target_path`
- `src_side`
- `reason`
- `payload_json`
- `priority`
- `status`
- `attempt_count`
- `next_attempt_at`
- `last_error`
- `created_at`
- `updated_at`

#### `failure_records`

用于 UI 查询失败项与手动重试。

建议字段：

- `id`
- `task_id`
- `path`
- `op_type`
- `error_code`
- `error_message`
- `retryable`
- `first_failed_at`
- `last_failed_at`
- `attempt_count`
- `resolved_at`

#### `audit_logs`

记录管理面关键操作。

#### `task_events`

记录任务级时间线事件，供 UI 展示运行历史。

### 5.3 数据约束

1. `file_index(task_id, relative_path)` 必须唯一。
2. 所有路径统一存相对路径。
3. 时间统一存 UTC。
4. 删除状态使用 tombstone，不直接删除索引记录。
5. `version` 字段用于乐观并发控制，避免扫描与执行线程覆盖写。

## 6. 同步状态机与任务执行模型

### 6.1 任务状态机

推荐任务状态：

1. `created`
2. `initializing`
3. `running`
4. `paused`
5. `degraded`
6. `retrying`
7. `stopping`
8. `stopped`
9. `failed`

设计原则：

1. `degraded` 表示降级运行而非完全停摆。
2. 文件级失败不能直接将任务打入 `failed`。
3. 只有凭据失效、路径不可访问、数据库损坏等关键能力丧失才进入 `failed`。

### 6.2 任务执行模型

每个任务由一个独立 `task runner` 管理，内部包含：

1. `local watcher`
2. `remote poller`
3. `reconciler`
4. `planner`
5. `executor pool`

数据流：

1. 本地事件、远端轮询结果、补偿扫描结果统一转换为变更候选。
2. planner 结合 `file_index` 和任务策略生成标准动作。
3. executor 从落库队列中 claim 动作执行。
4. 执行结果回写 `file_index`、`task_runtime_state`、`failure_records` 和指标。

### 6.3 文件动作状态

推荐文件动作状态：

1. `synced`
2. `discovered`
3. `planned`
4. `executing`
5. `retry_wait`
6. `conflict`
7. `error`
8. `ignored`

### 6.4 统一动作模型

同步引擎只产出统一动作：

1. `CreateDirLocal`
2. `CreateDirRemote`
3. `UploadFile`
4. `DownloadFile`
5. `DeleteLocal`
6. `DeleteRemote`
7. `MoveConflictLocal`
8. `MoveConflictRemote`
9. `MarkTombstone`
10. `RefreshMetadata`

### 6.5 首轮基线同步

首次任务启动不走普通增量流程，单独执行：

1. 扫描本地路径
2. 扫描远端路径
3. 基于方向和删除策略生成首批动作
4. 建立初始 `file_index`
5. 基线完成后切换到 `running`

### 6.6 恢复策略

1. 暂停后恢复时先做补偿扫描，不直接信任暂停前事件流。
2. 进程重启后将 `executing` 动作回退到 `planned` 或 `retry_wait`。
3. 文件级失败与任务级退避分层处理。

## 7. WebDAV Connector 抽象与能力探测

### 7.1 职责边界

WebDAV connector 负责：

1. 连接与认证
2. 远端文件语义抽象
3. 能力探测
4. 错误归一化

不负责：

1. 冲突决策
2. 删除传播策略
3. 任务状态切换
4. 本地文件写入

### 7.2 抽象接口

建议定义统一的远端连接器接口，首版仅实现 WebDAV：

1. `Probe(ctx) -> Capabilities`
2. `Stat(ctx, path) -> RemoteEntry`
3. `List(ctx, path, opts) -> []RemoteEntry`
4. `MkdirAll(ctx, path)`
5. `Download(ctx, path) -> stream + metadata`
6. `Upload(ctx, path, stream, metadata)`
7. `Delete(ctx, path, isDir)`
8. `Move(ctx, src, dst)`
9. `HealthCheck(ctx)`

`RemoteEntry` 至少包含：

- `path`
- `is_dir`
- `size`
- `mtime`
- `etag`
- `content_type`
- `exists`

### 7.3 能力探测项

能力探测至少覆盖：

1. `supports_etag`
2. `supports_last_modified`
3. `supports_content_length`
4. `supports_recursive_propfind`
5. `supports_move`
6. `path_encoding_mode`
7. `mtime_precision`
8. `server_fingerprint`
9. `probe_warnings`

### 7.4 运行时策略回灌

1. 若 `ETag` 稳定，则优先用于远端变化判断。
2. 若递归 `PROPFIND` 不可靠，则退化为分层遍历。
3. 若 `MOVE` 不可用，则重命名与冲突副本退化为上传/下载/删除组合。
4. 若服务端属性不稳定，则提高补偿扫描与 hash 校验权重。

### 7.5 错误归一化

建议统一为：

1. `ErrUnauthorized`
2. `ErrNotFound`
3. `ErrConflict`
4. `ErrRateLimited`
5. `ErrTemporary`
6. `ErrPermanent`
7. `ErrCapabilityMismatch`

## 8. 加密模块接入点与密钥管理

### 8.1 设计原则

1. 加密模块作为独立层接入，不侵入同步引擎和 WebDAV connector。
2. 上传下载链路采用流式加解密。
3. 加密模式下显式区分明文属性与密文属性。

### 8.2 接入点

上传链路：

`local reader -> crypto.EncryptReader -> connector.Upload`

下载链路：

`connector.Download -> crypto.DecryptReader -> local writer`

### 8.3 密钥层级

推荐三层结构：

1. `Master Key`
2. `Task Key`
3. `Data Key`（首版可由 Task Key 派生，不强制独立持久化）

### 8.4 存储边界

1. `Master Key` 不入库，由环境变量、启动参数或本机受保护文件提供。
2. `Task Key` 使用 `Master Key` 包装后存储到 SQLite。
3. 连接密码同样使用 `Master Key` 包装后存储。
4. 临时数据密钥仅存在内存。

### 8.5 元数据保护策略

首版推荐：

1. 内容加密。
2. 文件名保护。
3. 保留目录结构，不进一步隐藏目录拓扑。

### 8.6 加密元数据

远端对象需要最小元数据，至少包含：

1. `encryption_version`
2. `nonce/iv`
3. `algorithm`
4. `original_size`（可选）
5. `content_digest`（可选）
6. `wrapped_key_id`

首版优先采用文件头部前缀存放加密元数据。

### 8.7 失败与恢复边界

1. 主密钥缺失时，加密任务不得静默降级，需进入不可运行状态。
2. 任务密钥解包失败时，仅影响对应任务。
3. 密文头损坏按文件级失败处理，不阻塞整个任务。
4. 用户丢失主密钥视为不可恢复，须在文档中明确。

## 9. React 管理台最小信息架构

### 9.1 页面结构

首版管理台控制在 6 个页面：

1. 登录页
2. 仪表盘
3. 连接管理
4. 任务管理
5. 任务详情
6. 系统页

### 9.2 任务详情页

任务详情是管理台核心页面，建议包含：

1. 概览区：状态、方向、路径、加密、最近错误
2. 运行面板：phase、队列长度、活跃 worker、最近扫描与最近成功时间、当前速率
3. 累计统计：上传、下载、删除、冲突、失败、重试、流量
4. 失败记录：支持筛选和手动重试
5. 事件时间线：运行、退避、恢复、严重错误

### 9.3 任务创建向导

建议 3 步：

1. 连接与路径
2. 同步策略
3. 性能与安全

高级配置折叠，不在首版 wizard 中无限展开。

### 9.4 前端技术建议

1. 路由：`react-router`
2. 服务端状态：`tanstack query`
3. 表单：`react-hook-form`
4. 刷新策略：列表页低频轮询，详情页较高频轮询，不首版引入 WebSocket

## 10. API 边界

### 10.1 API 原则

1. 只暴露资源和命令，不泄露内部 worker/调度实现。
2. 前端关注“状态是什么、能做什么、最近发生了什么”，不关心内部执行细节。
3. 对任务详情提供聚合运行视图接口，减少前端首屏拼装成本。

### 10.2 资源类 API

连接：

1. `GET /api/connections`
2. `POST /api/connections`
3. `GET /api/connections/:id`
4. `PATCH /api/connections/:id`
5. `POST /api/connections/:id/test`
6. `DELETE /api/connections/:id`

任务：

1. `GET /api/tasks`
2. `POST /api/tasks`
3. `GET /api/tasks/:id`
4. `PATCH /api/tasks/:id`
5. `DELETE /api/tasks/:id`
6. `POST /api/tasks/:id/start`
7. `POST /api/tasks/:id/pause`
8. `POST /api/tasks/:id/resume`
9. `POST /api/tasks/:id/stop`

任务运行与排障：

1. `GET /api/tasks/:id/runtime`
2. `GET /api/tasks/:id/stats`
3. `GET /api/tasks/:id/failures`
4. `GET /api/tasks/:id/events`
5. `POST /api/tasks/:id/retry-failures`

系统：

1. `GET /api/dashboard/summary`
2. `GET /api/system/health`
3. `GET /api/system/config`
4. `GET /api/audit-logs`

### 10.3 聚合运行视图

`GET /api/tasks/:id/runtime` 建议返回：

1. 当前任务状态与 phase
2. 最近错误摘要
3. 队列长度
4. 当前传输速率
5. 最近扫描时间
6. 最近成功时间
7. 活跃 worker 数
8. 任务级简版统计

## 11. 可观测性与运维边界

### 11.1 日志

采用结构化日志，分为：

1. 系统日志
2. 任务运行日志
3. 审计日志

日志不得输出密码、完整授权头、主密钥或可复用敏感令牌。

### 11.2 指标

至少暴露：

1. 任务状态
2. 成功数
3. 失败数
4. 重试数
5. 队列长度
6. 扫描耗时
7. 传输速率
8. 累计流量

### 11.3 健康检查

至少区分：

1. 进程存活
2. 任务子系统可工作
3. 数据库可访问
4. 后台调度是否存活

## 12. 风险与设计对策

### 12.1 WebDAV 服务端兼容性差异

对策：

1. 强制能力探测
2. 扫描策略支持递归与分层双模式
3. 错误归一化与降级运行

### 12.2 本地事件丢失

对策：

1. watcher 只作为加速入口
2. 补偿扫描作为正式兜底路径
3. 重启恢复后主动 reconcile

### 12.3 单机资源争用

对策：

1. 任务级并发限制
2. 任务级带宽限速
3. 执行队列与重试退避隔离

### 12.4 加密模式下的一致性判断复杂化

对策：

1. 显式区分明文属性和密文属性
2. 将加密模块限制在流转换与路径编码边界内
3. 不依赖远端密文大小直接推导明文一致性

## 13. 实施优先级建议

### 13.1 M1

1. `chi` API 骨架
2. SQLite schema 与 migration
3. 连接管理与能力探测
4. 单任务 runner
5. 本地 watcher + 远端轮询
6. 基础同步引擎
7. 轻量 React 管理台基础页

### 13.2 M2

1. 多任务并发
2. 任务级限速与并发控制
3. failure records 与手动重试
4. 审计日志与指标
5. 补偿扫描与一致性修复增强

### 13.3 M3

1. 客户端加密落地
2. 增强一致性校验
3. 健康检查与告警接口
4. 数据库恢复与修复工具

## 14. 结论

本设计选择了“单二进制 + 清晰模块边界 + 元数据驱动同步”的路线，以满足单机商用首版对稳定性、恢复能力、可观测性和后续演进空间的要求。首版不追求分布式调度和极限性能，而是通过：

1. `chi + SQLite` 降低部署与维护复杂度
2. `task runner + operation queue` 强化恢复能力
3. `WebDAV connector` 抽象隔离协议差异
4. `crypto` 独立分层控制加密边界
5. 轻量 React 管理台聚焦控制面和故障排查

为后续进入详细 implementation planning 提供稳定的设计基础。
