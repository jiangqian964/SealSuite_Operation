# 2026-06-08 — v0.2.0 SQLite 整体架构重构设计

## 1. 背景

当前项目的业务数据主要分散存储在多份 YAML / JSON 文件中，例如：

- `connections.yaml`
- `llm-apis.yaml`
- `webhooks.yaml`
- `task-drafts.yaml`
- `job-schedules.yaml`
- `job-runs.json`
- `api-templates.yaml`
- `output-templates.yaml`
- `complex-tasks.yaml`

与此同时，`web/server.go`、`runner/runner.go` 和调度相关逻辑会在启动或请求处理期间直接创建并使用这些文件存储对象。当前模式在 `v0.1.x` 阶段可以工作，但已经暴露出以下问题：

1. **业务状态分散**
   配置、任务、调度、执行日志分别落在多份文件中，长期维护和查询成本高。

2. **持久化边界不清晰**
   Handler、Runner、Scheduler 都在不同位置直接读写文件，业务规则和持久化细节耦合严重。

3. **不利于长期保存与审计**
   YAML / JSON 文件适合轻量配置，不适合作为长期运行日志和操作日志的核心存储。

4. **重载语义过重**
   当前大量依赖 `Reload()` 和内存快照，文件状态、运行状态和调度状态之间容易出现同步复杂度。

5. **后续扩展受限**
   当需要支持历史查询、审计、统计、筛选、分页时，文件存储的能力很快成为瓶颈。

因此，`v0.2.0` 的目标不是“新增一个数据库选项”，而是：

> 以 SQLite 作为唯一业务数据源，对项目做一次整体架构重构。

---

## 2. 目标

`v0.2.0` 的目标如下：

1. 用 SQLite 替代当前主要业务对象的 YAML / JSON 存储。
2. 保留极小的启动级 `config.yaml`，仅负责服务启动、日志和数据库路径等最小配置。
3. 引入清晰的 `Repository + Service + Web/Runner/Scheduler` 分层。
4. 让配置、任务、调度、运行日志、操作日志都具备长期保存能力。
5. 让 Web、Runner、Scheduler 共享统一的数据访问边界，而不是各自直接读写文件。
6. 最终代码形态不保留旧业务文件存储路径，SQLite 成为唯一事实来源。

---

## 3. 非目标

本轮设计不做以下事情：

1. 不保留 v0.1.x 文件存储与 SQLite 的长期双轨兼容。
2. 不设计复杂的多节点分布式调度。
3. 不在 v0.2.0 引入完整用户体系与复杂权限模型。
4. 不把所有复杂配置都做成强范式化的超细粒度表结构。
5. 不优先做 BI / 报表 / 可视化分析系统。

换句话说，`v0.2.0` 的重点是：

- **统一持久化**
- **架构重组**
- **长期保存**

而不是一次性做“平台级所有高级能力”。

---

## 4. 方案对比

## 4.1 方案 A：单库直连重构

### 描述

引入一个统一的 `app.db`，所有业务存储全部进入 SQLite，`internal/storage` 由文件实现改成数据库实现。`Web / Runner / Scheduler` 均通过新的数据访问层工作。

### 优点

- 架构最干净
- 唯一业务数据源明确
- 后续扩展空间最大

### 缺点

- 改动面最大
- 如果不控制分层，会导致代码重构风险高

---

## 4.2 方案 B：数据库为主，保留最小启动配置（推荐）

### 描述

保留极小 `config.yaml` 作为启动级配置，业务数据全部进入 SQLite。启动配置用于：

- HTTP 监听
- 日志配置
- SQLite 路径
- 可选开发模式开关

其余业务配置与业务状态全部入库。

### 优点

- 启动闭环清晰
- 业务数据边界明确
- 比“所有配置都进库”更稳
- 适合本项目当前演进阶段

### 缺点

- 严格意义上不是“零文件配置”
- 需要明确区分启动配置和业务配置

### 结论

**采用本方案。**

---

## 4.3 方案 C：事件日志导向重构

### 描述

除了当前状态表，还同步引入强事件表模型，让所有对象修改都形成一套可回放的事件流。

### 优点

- 审计能力最强
- 后续分析与回放空间大

### 缺点

- 设计与实现复杂度明显高于 v0.2.0 所需
- 很容易导致范围膨胀

### 结论

不采用。

---

## 5. v0.2.0 总体架构

`v0.2.0` 推荐收敛为 4 层：

### 5.1 启动层

职责：

- 读取极小 `config.yaml`
- 初始化日志
- 初始化 SQLite
- 执行 schema migration
- 装配 repository / service / runner / web

推荐位置：

- `cmd/main.go`
- `internal/bootstrap`
- `internal/db`

---

### 5.2 Repository 层

职责：

- 封装 SQLite 访问
- 提供 CRUD、查询、事务
- 不掺业务编排

建议对象：

- `ConnectionRepository`
- `LLMAPIRepository`
- `WebhookRepository`
- `TemplateRepository`
- `OutputTemplateRepository`
- `TaskDraftRepository`
- `ComplexTaskRepository`
- `ScheduleRepository`
- `RunLogRepository`
- `AuditLogRepository`

---

### 5.3 Service 层

职责：

- 承接业务规则
- 处理跨表校验与编排
- 写入审计日志
- 触发运行组件局部刷新

建议服务：

- `ConnectionService`
- `LLMService`
- `WebhookService`
- `TemplateService`
- `TaskDraftService`
- `ComplexTaskService`
- `ScheduleService`
- `ExecutionService`
- `RunLogService`
- `AuditLogService`

---

### 5.4 接入层

职责：

- Web API 请求与响应
- Runner 执行链
- Scheduler 调度触发

它们只依赖 service，不直接依赖 SQLite 实现。

---

## 6. 启动配置与业务配置边界

## 6.1 保留在文件中的启动配置

推荐只保留以下内容在 `config.yaml`：

- `server.bind`
- `server.port`
- `log.level`
- `log.filename`
- `database.path`
- `app.env`
- `debug`

这类配置修改后通常需要重启服务，不做热更新。

---

## 6.2 迁入 SQLite 的业务数据

以下对象全部进入 SQLite：

- 飞连连接配置
- LLM API 配置
- Webhook 配置
- API 模板
- 输出模板
- 任务草稿
- 复杂任务
- 复杂任务步骤
- 调度任务
- 运行日志
- 操作日志

---

## 7. SQLite 表设计

设计原则：

- 核心元数据结构化
- 复杂配置 JSON 化
- 引用关系显式化
- 运行日志与操作日志独立建表

---

## 7.1 `app_connections`

字段建议：

- `id`
- `name`
- `scheme`
- `host`
- `port`
- `access_key_id`
- `secret_ref`
- `secret_value`
- `is_active`
- `created_at`
- `updated_at`

说明：

- 当前项目没有引入密钥加密层时，`secret_value` 属于本地 SQLite 明文敏感数据。
- `secret_ref` 可保留，为后续接入安全存储预留接口。

---

## 7.2 `llm_apis`

字段建议：

- `id`
- `name`
- `provider`
- `base_url`
- `api_key`
- `model`
- `enabled`
- `is_active`
- `tags_json`
- `settings_json`
- `created_at`
- `updated_at`

说明：

- `temperature / max_tokens / thinking / reasoning_effort / response_format / system_prompt`
  可先放入 `settings_json`。

---

## 7.3 `webhooks`

字段建议：

- `id`
- `name`
- `provider`
- `url`
- `method`
- `headers_json`
- `auth_type`
- `body_template`
- `timeout_sec`
- `retry_count`
- `enabled`
- `created_at`
- `updated_at`

---

## 7.4 `api_templates`

字段建议：

- `id`
- `name`
- `method`
- `path`
- `category`
- `description`
- `query_schema_json`
- `path_params_schema_json`
- `body_schema_json`
- `created_at`
- `updated_at`

---

## 7.5 `output_templates`

字段建议：

- `id`
- `name`
- `format`
- `content_template`
- `description`
- `created_at`
- `updated_at`

---

## 7.6 `task_drafts`

字段建议：

- `id`
- `name`
- `mode`
- `cycle_mode`
- `run_count`
- `run_until`
- `source_template_id`
- `webhook_config_id`
- `webhook_enabled`
- `input_config_json`
- `transform_config_json`
- `llm_config_json`
- `output_config_json`
- `created_at`
- `updated_at`

说明：

- `task_drafts` 是 v0.2.0 的核心表之一。
- 当前前端和执行链对其概念依赖较深，建议保留其业务概念，仅替换持久化实现。

---

## 7.7 `complex_tasks`

字段建议：

- `id`
- `name`
- `goal`
- `execution_mode`
- `webhook_config_id`
- `webhook_enabled`
- `created_at`
- `updated_at`

---

## 7.8 `complex_task_steps`

字段建议：

- `id`
- `task_id`
- `step_order`
- `type`
- `name`
- `config_json`
- `created_at`
- `updated_at`

说明：

- 步骤应独立建表，而不是整包 steps 放一列 JSON。
- 这样更适合顺序执行、局部编辑和后续单步调试。

---

## 7.9 `job_schedules`

字段建议：

- `id`
- `draft_id`
- `target_type`
- `target_id`
- `enabled`
- `webhook_config_id`
- `webhook_enabled`
- `start_at`
- `end_at`
- `schedule_type`
- `cron_expr`
- `interval_expr`
- `timezone`
- `post_filter_json`
- `field_select_json`
- `truncate_rules_json`
- `created_at`
- `updated_at`

说明：

- 保留 `target_type + target_id`，不要退化成只存 `draft_id`。
- 这样后续可以统一支持 `task_draft` 与 `complex_task`。

---

## 7.10 `job_runs`

字段建议：

- `id`
- `source_type`
- `source_id`
- `target_type`
- `target_id`
- `status`
- `trigger_source`
- `started_at`
- `finished_at`
- `duration_ms`
- `error_message`
- `result_json`

说明：

- 这张表替代当前 `job-runs.json`
- 应统一容纳：
  - 手动运行
  - 调度运行
  - 复杂任务运行
  - Execute 临时执行

---

## 7.11 `audit_logs`

字段建议：

- `id`
- `actor`
- `action`
- `resource_type`
- `resource_id`
- `payload_json`
- `created_at`

说明：

- 至少覆盖：
  - 配置新增 / 修改 / 删除
  - 草稿保存 / 删除
  - 调度创建 / 更新 / 删除
  - 手动触发执行

---

## 8. SQLite 中的引用关系

推荐在 service 层做强校验，并在数据库层提供索引。

关键引用包括：

- `task_drafts.source_template_id -> api_templates.id`
- `task_drafts.webhook_config_id -> webhooks.id`
- `task_drafts.llm_config_json.llm_api_id -> llm_apis.id`
- `complex_tasks.webhook_config_id -> webhooks.id`
- `complex_task_steps.task_id -> complex_tasks.id`
- `job_schedules.target_id -> task_drafts.id / complex_tasks.id`

为了控制复杂度，v0.2.0 不要求一开始把所有 polymorphic 引用都做成强外键。

---

## 9. Web / Runner / Scheduler 重组

## 9.1 Web API

改造目标：

- Handler 不再直接读写 SQLite 或文件
- 所有写操作进入 service
- API 层只负责请求解析、service 调用和响应组装

示例：

- `/task-drafts` → `TaskDraftService`
- `/job-schedules` → `ScheduleService`
- `/settings/llm/apis` → `LLMService`

---

## 9.2 Runner

改造目标：

- Runner 不再直接依赖文件路径
- Runner 只依赖任务读取、模板读取、运行日志写入、Webhook 读取等接口
- 执行链路聚焦在：
  - API 调用
  - transform
  - llm
  - output
  - webhook

建议将当前职责拆分为：

- `ExecutionService`
- `RunLogService`
- `WebhookDispatchService`

Runner 可以收敛为更薄的协调器，甚至最终只保留执行入口。

---

## 9.3 Scheduler

改造目标：

- Scheduler 成为“数据库快照驱动的内存触发器”
- 修改调度后，通过 service 层触发局部刷新或必要的全量刷新
- 触发时统一调用：
  - `RunSchedule(id)`
  - 再由 schedule 解析 target 并进入执行链

建议执行入口统一成：

- `RunTaskDraft(id)`
- `RunComplexTask(id)`
- `RunSchedule(id)`

其中：

- `RunSchedule(id)` 只负责解析 schedule，再委托到前两者之一

---

## 10. 配置热更新策略

## 10.1 启动级配置

例如：

- 服务监听地址
- SQLite 路径
- 日志文件

处理策略：

- 修改后重启服务
- 不做热更新

---

## 10.2 业务级配置

例如：

- 飞连连接
- LLM API
- webhook
- 模板
- 调度

处理策略：

- 持久化到 SQLite
- 由 service 层通知对应运行组件局部刷新

---

## 11. 日志策略

必须明确区分：

### 11.1 运行日志

使用 `job_runs`：

- 记录执行结果
- 记录执行耗时
- 记录失败原因
- 记录触发来源

### 11.2 操作日志

使用 `audit_logs`：

- 记录谁做了什么操作
- 记录作用对象
- 记录操作 payload

这两者不能混在一张表中。

---

## 12. 迁移与落地方式

虽然最终不保留 v0.1.x 文件存储兼容，但为了控制实现风险，推荐开发过程按阶段推进：

### Phase 1：SQLite 底座

- 初始化 DB
- migration 机制
- repository 接口与实现
- 基础表可用

### Phase 2：配置中心切换

- 连接配置
- LLM API
- webhook
- API 模板
- 输出模板

### Phase 3：任务与调度切换

- 任务草稿
- 复杂任务
- 调度任务

### Phase 4：运行与审计日志切换

- `job_runs`
- `audit_logs`

### Phase 5：删除旧文件存储路径

- 删除旧 store 实现入口
- 删除旧初始化路径
- 删除旧 reload 语义依赖
- 清理文档与 example 文件中的旧业务文件依赖

注意：

- 这是开发过程中的阶段推进
- 最终发布产物只保留 SQLite 架构，不保留旧业务文件路径

---

## 13. 关键风险与对策

## 13.1 SQLite 并发锁风险

风险：

- Web 请求写库
- Scheduler 触发执行
- Runner 写运行日志

容易出现：

- `database is locked`

对策：

- 启用 WAL
- 配置 busy timeout
- 明确事务边界
- 控制连接池策略

---

## 13.2 JSON 字段过度膨胀

风险：

- 全部塞 JSON 会削弱查询能力

对策：

- 核心列结构化
- 复杂配置 JSON 化
- 后续根据查询需求再逐步拆列

---

## 13.3 Reload 机制失控

风险：

- 继续依赖粗粒度 `Reload()` 会导致状态复杂化

对策：

- 明确：
  - 哪些修改触发调度刷新
  - 哪些修改触发运行时 client 更新
  - 哪些只需落库

---

## 13.4 测试成本高

风险：

- 页面保存正常，但执行或调度链路断裂

对策：

- 加强：
  - repository 测试
  - service 测试
  - runner 集成测试
  - web handler 回归测试

---

## 14. v0.2.0 的硬约束

为防止范围失控，建议执行以下硬约束：

1. 不做文件存储与 SQLite 的长期双轨共存
2. 每一类对象切换后，旧路径立刻下线
3. 所有写操作必须经过 service
4. `job_runs` 与 `audit_logs` 必须分表

---

## 15. 设计结论

`v0.2.0` 推荐采用以下最终形态：

- SQLite 作为唯一业务数据源
- 保留极小 `config.yaml` 作为启动配置
- 通过 `Repository + Service + Web/Runner/Scheduler` 分层重构
- 配置、任务、调度、运行日志、操作日志统一纳入数据库
- 最终删除旧业务文件存储路径

一句话总结：

> v0.2.0 不是“把文件换成数据库”，而是把项目重构成以 SQLite 为中心的统一业务持久化架构。
