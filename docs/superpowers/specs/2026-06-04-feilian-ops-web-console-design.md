# 飞连 API 自动化运维小工具（本地 Web 控制台）设计稿

日期：2026-06-04  
范围：B 路线（内置 Web 控制台，静态前端 + REST API，任务定义独立 jobs.yaml，默认仅本机 localhost 访问）

## 1. 背景与现状
当前项目已具备：
- Go 服务骨架（`cmd/main.go` 入口）
- 定时任务调度（`internal/scheduler`，基于 `robfig/cron`）
- 任务处理示例（`internal/handler/jobs.go`）
- SealSuite/飞连 API 客户端雏形（`internal/sealsuite/client.go`，含 mockMode）
- 配置/日志/部署脚本（`internal/config`、`internal/logger`、`scripts/`、systemd）

但与“本地 Web 部署与配置”的目标相比，缺少：HTTP 服务与 Web 控制台、任务动态管理、真实 API 鉴权链路与可控 mock、以及 cron 秒级一致性。

## 2. 目标 / 非目标
### 2.1 目标
1) 在同一进程内提供本地 Web 控制台（默认只监听 `127.0.0.1`）  
2) 支持配置管理（读/校验/保存）、任务管理（增删改启停/手动触发）、状态与日志查看  
3) 任务定义持久化到 `jobs.yaml`，保存后可触发 Runner reload  
4) 统一 cron 语义为**秒级 6 段表达式**，避免“注释与实际调度不一致”  
5) 将 SealSuite/飞连 API 客户端从样例/Mock 推进到可调用真实 API（按 OpenAPI 规范补齐鉴权/重试/超时）
6) 至少支持两类飞连 API 使用场景：  
   - **单次任务**：在 Web 上通过“模板库 + 通用调试器”配置并执行飞连 API；对常用资源提供“查/增/改/删（CRUD）”的便捷操作  
   - **周期性任务**：通过定时任务周期性调用飞连指定 API（信息查阅/报表采集/策略写入等）

### 2.2 非目标
- 公网可访问的控制台（不做 TLS/复杂鉴权/用户体系，默认 localhost）
- 复杂的前端工程与组件体系（优先轻量静态资源）
- 分布式调度、分布式任务锁（本阶段不做）

## 3. 总体架构
进程内包含两条主链路：
- **Runner 链路**：加载 `config.yaml` 与 `jobs.yaml` → 初始化 SealSuite Client → 启动 Scheduler → 执行 jobs
- **Web Console 链路**：提供静态前端（内置） + REST API → 管理配置与任务 → 驱动 Runner reload / 手动触发

数据流：
1) 用户在 Web 控制台修改配置/任务  
2) 后端校验（cron、job type、参数 schema、配置合法性）  
3) 原子写盘（临时文件 + rename）  
4) Runner `Reload()`：停止调度器 → 重新加载任务 → 恢复调度  

## 3.1 飞连 API 两类使用场景（补充）
### A) 单次任务：API 工具箱（Web）
目标：让运维人员在 Web 上“即时”调用飞连 API，覆盖两条路径：
1) **模板库（推荐入口）**：对常用资源（如用户/设备/策略等）提供可视化表单与 CRUD 动作按钮  
2) **通用调试器**：类似 Postman，在页面上填写 method/path/query/body，系统负责鉴权、发包、展示响应，并支持“保存为模板/保存为周期性任务”

交互输出：
- 请求预览（method/url/headers 摘要；敏感信息不回显）
- 响应展示（状态码、业务码、耗时、body）
- 调用历史（按时间、模板、资源类型过滤）

### B) 周期性任务：定期调用/策略写入（Scheduler）
目标：将“查询/写入类 API”封装为任务类型，纳入调度器：
- 信息查阅：定期拉取用户/设备/组织/策略等信息（用于巡检、报表、告警输入）
- 策略写入：定期下发/修正策略（谨慎，建议加 dry-run 与变更预览）

周期性任务来源：
- 直接在“任务管理”页新增
- 或从“单次任务/API 工具箱”一键保存为周期性任务（带参数）

## 4. 技术选型
### 4.1 Web 后端
推荐：`net/http + chi`
- 依赖轻、足够灵活，适合本地运维小工具
- 便于将来加 token 鉴权、中间件、CORS（如需）

备选：Gin（团队熟悉、生态成熟），但会引入更重的框架依赖。

### 4.2 前端静态资源
使用 `go:embed` 打包静态资源进入二进制：
- 部署路径简单（systemd 下无需额外分发前端文件）
- 支持单页应用（SPA）或轻多页

### 4.3 调度器
使用 `robfig/cron/v3` 的秒级模式与时区：
- `cron.New(cron.WithSeconds(), cron.WithLocation(location))`
- 统一采用 6 段表达式：`sec min hour dom mon dow`

## 5. 配置与任务模型
### 5.1 config.yaml（系统配置）
保留现有结构并扩展：
- `server.bind`（默认 `127.0.0.1`）
- `server.port`（默认 8080 或沿用现有 8080）
- `sealsuite.mock_mode`（默认 `false`）
- `sealsuite.*`（base_url/access_key/secret_key/timeout/retry_times）

敏感字段（access_key/secret_key）在 API 返回时必须脱敏，前端展示只显示前后若干位。

### 5.2 jobs.yaml（任务定义，独立文件）
建议结构：
```yaml
version: 1
jobs:
  - name: users-sync
    enabled: true
    cron: "0 */1 * * * *"
    type: "sync_users"
    params:
      page_size: 200

  - name: devices-sync
    enabled: true
    cron: "0 */2 * * * *"
    type: "sync_devices"
    params: {}
```

约束：
- `name` 全局唯一
- `cron` 必须可被秒级解析
- `type` 必须在服务端注册表中存在（例如 `sync_users`、`sync_devices`、`example_sync`）
- `params` 需要按 job type 的 schema 校验（v0 可先宽松校验，后续收紧）

#### 5.2.1 通用“API 周期任务”类型（推荐 v0 就支持）
为覆盖“周期性任务：定期调用飞连指定 API 信息查阅 / 策略写入”，建议引入两种通用 job type，
其本质是“按模板执行 API”，从而避免为每个 API 单独写 handler：

```yaml
jobs:
  - name: daily-users-check
    enabled: true
    cron: "0 0 9 * * *"           # 每天 09:00:00
    type: "api_poll"              # 查询类
    params:
      template_id: "users_list"
      input:
        page: 1
        page_size: 200

  - name: weekly-policy-write
    enabled: false                 # 默认禁用，需预览确认后启用
    cron: "0 0 2 * * 1"            # 每周一 02:00:00
    type: "api_write"              # 写入类
    params:
      template_id: "policy_update"
      input:
        policy_id: "xxx"
        __raw_json__: {}
      safety:
        dry_run: true              # v0 强制支持：dry-run + 变更预览
        require_preview_token: true
```

建议约束：
- `api_poll`：允许 GET/查询型模板；支持保存响应摘要、失败告警（v0 可先只记录日志）
- `api_write`：仅允许写入型模板；默认 `enabled=false`，必须在 Web 控制台完成预览后才能启用
  - `require_preview_token`：仅当用户在控制台完成一次“预览确认”并获得 token 后，才允许真正写入（v0 可简化为：启用任务前必须执行一次 preview）

### 5.3 api-templates.yaml（可选，但推荐 v0 就支持）
目的：支撑“模板库 + 可保存的自定义请求”。建议独立文件，避免与 jobs/config 互相污染。

建议结构：
```yaml
version: 1
templates:
  # ========== 用户 ==========
  - id: users_list
    name: 用户-列表
    category: users
    method: GET
    path: /api/v1/users
    query_schema:
      page: { type: int, default: 1 }
      page_size: { type: int, default: 200 }

  - id: users_update
    name: 用户-更新
    category: users
    method: PUT
    path: /api/v1/users/{user_id}
    path_params_schema:
      user_id: { type: string, required: true }
    body_schema:
      __raw_json__: { type: json }

  # ========== 设备 ==========
  - id: devices_list
    name: 设备-列表
    category: devices
    method: GET
    path: /api/v1/devices
    query_schema:
      page: { type: int, default: 1 }
      page_size: { type: int, default: 200 }

  - id: devices_update
    name: 设备-更新
    category: devices
    method: PUT
    path: /api/v1/devices/{device_id}
    path_params_schema:
      device_id: { type: string, required: true }
    body_schema:
      __raw_json__: { type: json }

  - id: policy_update
    name: 策略-更新
    category: policy
    method: PUT
    path: /api/v1/policies/{policy_id}
    path_params_schema:
      policy_id: { type: string, required: true }
    body_schema:
      # v0 允许自由 JSON；v1 再做精细 schema
      __raw_json__: { type: json }

  - id: policy_list
    name: 策略-列表
    category: policy
    method: GET
    path: /api/v1/policies
    query_schema:
      page: { type: int, default: 1 }
      page_size: { type: int, default: 200 }
```

说明：
- “模板库 CRUD”：不是对模板做 CRUD，而是对“模板所指向的资源”提供 CRUD 快捷操作（由模板集合覆盖：list/get/create/update/delete）。
- “通用调试器保存为模板”：保存后写入 `api-templates.yaml` 的自定义模板（category=custom）。
- 本项目 v0 优先覆盖：**用户 / 设备 / 策略** 的“查询 + 写入”（list/get + create/update 的组合，具体以飞连 OpenAPI 能力为准）。

## 6. REST API 设计（v0）
### 6.1 基础
- `GET /api/v1/health`：健康检查（含 Runner 状态：scheduler running、jobs count）
- `GET /api/v1/version`：版本/构建信息（可选：build time、commit）

### 6.2 配置管理
- `GET /api/v1/config`：返回配置（敏感字段脱敏）
- `PUT /api/v1/config`：提交新配置（校验 → 原子写盘 → 触发 reload 或提示需重启）

### 6.3 任务管理（jobs.yaml）
- `GET /api/v1/jobs`：返回 jobs 列表（含运行态：next_run、last_run、last_status、last_error）
- `PUT /api/v1/jobs`：整体保存 jobs.yaml（校验 → 原子写盘 → Runner reload）
- `POST /api/v1/jobs/{name}/enable`：启用任务（写回 jobs.yaml + reload）
- `POST /api/v1/jobs/{name}/disable`：禁用任务（写回 jobs.yaml + reload）
- `POST /api/v1/jobs/{name}/run`：手动触发一次（不依赖 cron）
- `POST /api/v1/reload`：显式触发 Runner reload（用于排障）

说明：
- v0 先用“整体 PUT”降低接口复杂度；后续可加增量式 CRUD（POST/PUT/DELETE /jobs/{name}）。

### 6.4 日志
v0：
- `GET /api/v1/logs?tail=500&level=info`：读取日志文件尾部 N 行（或按字节截断），支持简单过滤

v1（可选）：
- SSE：`GET /api/v1/logs/stream` 实时推送新增日志

### 6.5 API 工具箱（单次任务 + 模板）
模板管理（文件：`api-templates.yaml`）：
- `GET /api/v1/api/templates`：获取模板列表
- `PUT /api/v1/api/templates`：整体保存模板（校验 → 原子写盘）

单次执行：
- `POST /api/v1/api/execute`：通用调试器执行
  - 入参：`method/path/query/body`（body 为任意 JSON），可选 `template_id`
  - 出参：`http_status/latency/response_body/business_code/business_message`

按模板执行（便于页面“CRUD 按钮”绑定）：
- `POST /api/v1/api/templates/{id}/execute`：以模板为基础执行（页面只需填 path/query/body 的变量）

写入类操作的 dry-run + 变更预览：
- `POST /api/v1/api/preview`：预览一次“写入型请求”
  - 入参：同 `execute`，额外字段：`preview_mode`（`dry_run`/`diff_only`），可选 `read_before_write`（bool）
  - 行为：
    - 若飞连 API 支持 dry-run：使用 dry-run 参数发起预演请求，返回服务端校验结果
    - 否则：可选先做一次“读当前状态”（read_before_write），再对“当前响应 vs 即将写入 body”做 diff，返回给前端展示
  - 出参：`preview.request`（method/path/body 摘要）、`preview.diff`（结构化差异）、`preview.dry_run_supported`、`preview.warnings`

从单次任务“一键保存为周期任务”：
- `POST /api/v1/api/save-as-job`：
  - 入参：`job_name/cron/type(api_poll|api_write)/template_id/input/safety`
  - 行为：校验 → 写入 jobs.yaml →（可选）触发 reload
  - 写入类任务：必须先完成 `preview`，并将预览结果摘要（或 token）写入 job.safety，默认创建为 `enabled=false`

调用历史（v0 先落内存 + 可选落盘）：
- `GET /api/v1/api/history?limit=100&template_id=&category=`：查看最近调用记录（用于排障与复现）

## 7. Runner 与调度控制
### 7.1 Runner 接口（内部）
- `Start()`：加载配置与 jobs，启动 scheduler
- `Stop()`：停止 scheduler
- `Reload()`：重新加载 jobs.yaml 并重建调度表
- `RunOnce(jobName)`：按名称执行一次（供手动触发）

### 7.2 运行态记录（用于 Web 展示）
需要在内存中维护：
- jobName → last_run_time / last_status / last_error / duration
- jobName → next_run_time（可从 cron entry 推导或记录）

## 8. SealSuite/飞连 API 客户端推进
### 8.1 mockMode
- 禁止在 main.go 中写死 `SetMockMode(true)`
- 改为读取 `sealsuite.mock_mode`（默认 false）

### 8.2 鉴权与签名
按《飞连 Open API 说明文档》实现请求鉴权（header/签名/时间戳/nonce 等）。  
当前项目已有 `utils.GenerateSignature`，可复用或替换为与规范一致的签名串拼接方式。

### 8.3 重试与超时
- `timeout`：已在 http.Client 上生效
- `retry_times`：需要真正用于请求重试（可复用 `utils.Retry`，并对可重试错误做分类）

## 9. 部署与运维
### 9.1 部署方式
- `scripts/build.sh` 产出二进制
- `scripts/start.sh` 启动（前台/后台）
- systemd：保持现有方式，但补齐环境变量与工作目录配置说明

### 9.2 本地访问
- 默认 `server.bind=127.0.0.1`，只能本机访问
- 远程访问通过 SSH 端口转发（非本 spec 强制实现，但文档建议提供）

## 10. 风险与边界
- cron 表达式迁移：从 5 段到 6 段会影响已有配置，需要明确规范并在校验时报错提示
- 配置写回：必须原子写盘，避免写坏导致服务不可启动
- secrets：禁止将真实密钥提交到仓库；建议提供 `.env.example` 或仅文档说明环境变量注入
 - 策略写入类周期任务：需要更强的保护（dry-run、变更预览、幂等性/回滚策略），建议默认关闭或要求二次确认

## 11. 验收标准（Definition of Done）
1) 启动后可在本机打开 Web 控制台（静态前端加载正常）  
2) 可查看 jobs 列表与运行态（next/last）  
3) 修改 jobs.yaml（通过 API）后调度器 reload 生效  
4) 手动触发单个 job 可执行并在 UI 展示结果  
5) cron 语义为秒级且与实际执行一致  
6) mockMode 可配置开关，默认调用真实 API（若鉴权未完成则明确返回错误）  
7) API 工具箱可执行一次性请求（通用调试器），并能基于模板执行常用 API  
8) 模板库至少覆盖以下资源的“查询 + 写入”快捷操作：**用户 / 设备 / 策略**  
   - 查询：list/get（至少其一，推荐 list+get）  
   - 写入：create/update（至少其一；策略写入一般为 update/patch）  
9) 写入类操作（单次与周期性任务）支持 **dry-run + 变更预览**：  
   - 前端在提交写入前可查看“即将写入的请求体/关键字段变化”  
   - 若飞连 API 支持 dry-run 参数，则以服务端 dry-run 响应为准；否则以“读取当前状态 → diff 计算 → 预览展示”方式实现
