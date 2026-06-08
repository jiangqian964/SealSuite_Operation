# 飞连 API 自动化运维小工具（本地 Web 控制台）Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 在现有 Go 项目中实现“本地 Web 控制台 + REST API + jobs.yaml 周期任务 + 飞连 API 单次调用（模板库+通用调试器）+ dry-run/变更预览”，默认仅本机 localhost 访问。

**Architecture:** 单进程内包含 Runner（scheduler/job 执行）与 Web Console（静态前端 go:embed + REST API）。任务与模板分别持久化到 `jobs.yaml`、`api-templates.yaml`，保存后触发 Runner reload。写入类请求在单次与周期任务中均支持 dry-run + 变更预览。

**Tech Stack:** Go 1.21、net/http、chi router、robfig/cron(v3, WithSeconds)、go:embed、yaml.v3、httptest 单测

---

## 0. File Map（将被新增/修改的文件）

**Modify**
- `cmd/main.go`：从“写死任务+mock”改为 Runner + WebServer 统一启动、优雅关闭
- `internal/scheduler/scheduler.go`：启用秒级 cron、暴露必要运行态
- `internal/sealsuite/client.go`：mockMode 改为配置驱动；为通用执行器提供更通用的 `Do(method, path, query, body)`（或等价能力）
- `internal/config/config.go`：补充 server.bind、sealsuite.mock_mode 等字段（如还未加）
- `config.yaml`：移除真实密钥（安全），增加 server.bind/mock_mode 示例（可保留现有字段并给占位）

**Create**
- `internal/runner/runner.go`：Runner 生命周期、Reload、RunOnce、运行态记录
- `internal/storage/atomic.go`：原子写文件工具（temp+rename+fsync）
- `internal/storage/jobs_store.go`：jobs.yaml 读写与校验
- `internal/storage/templates_store.go`：api-templates.yaml 读写与校验
- `internal/api/types.go`：API execute/preview 请求响应类型
- `internal/api/executor.go`：通用 API 执行器（模板/自由请求）、历史记录 ring buffer
- `internal/api/preview.go`：dry-run 与 diff-only 预览逻辑（含 diff 计算）
- `internal/web/server.go`：HTTP server 构建与启动（bind/port、静态资源、API 路由）
- `internal/web/routes.go`：路由注册（health/version/config/jobs/api-toolbox/logs）
- `internal/web/middleware.go`：基础中间件（日志、panic recover、JSON、仅 localhost 绑定说明）
- `internal/web/handlers_*.go`：按域拆分 handler（config/jobs/api/logs/health）
- `internal/web/assets/embed.go`：go:embed 静态前端资源
- `web/ui/index.html`、`web/ui/app.js`、`web/ui/styles.css`：最小可用前端
- `jobs.yaml`：默认任务示例（空也可，但建议提供）
- `api-templates.yaml`：内置模板（用户/设备/策略：list/update；策略追加 list；后续可加 get）

**Test (Create)**
- `internal/storage/jobs_store_test.go`
- `internal/storage/templates_store_test.go`
- `internal/scheduler/scheduler_test.go`
- `internal/api/executor_test.go`
- `internal/api/preview_test.go`
- `internal/web/server_test.go`（路由与静态资源最小冒烟）

---

## 1) Task 1: 引入 Web Router 与项目依赖

**Files:**
- Modify: `go.mod`, `go.sum`

- [ ] **Step 1: 添加 chi 依赖**

Run:
```bash
go get github.com/go-chi/chi/v5@latest
go mod tidy
```

- [ ] **Step 2: 验证 build 仍通过**

Run:
```bash
go test ./... -count=1
```
Expected: PASS（当前可能无测试；至少应编译通过）

---

## 2) Task 2: scheduler 启用秒级 cron + 时区

**Files:**
- Modify: `internal/scheduler/scheduler.go`
- Test: `internal/scheduler/scheduler_test.go` (create)

- [ ] **Step 1: 为 scheduler.New() 增加秒级与时区参数**

将 `New()` 改为 `New(location *time.Location) *Scheduler`，并使用：
```go
cron.New(
  cron.WithSeconds(),
  cron.WithLocation(location),
)
```

- [ ] **Step 2: 为 AddJob 的 spec 约束为 6 段（秒级）**

实现方式：在 `AddJob` 内部先调用 `cron.ParseStandard` 不适用（5 段），因此用 `s.cron.AddFunc(spec, ...)` 的返回错误作为校验，并在错误消息中提示“需要 6 段秒级表达式”。

- [ ] **Step 3: 增加运行态查询方法**

在 `Scheduler` 上新增：
```go
func (s *Scheduler) Entries() []cron.Entry { return s.cron.Entries() }
```
用于 Web 展示 next/prev。

- [ ] **Step 4: 添加单测确保 6 段表达式可被接受**

`internal/scheduler/scheduler_test.go`：
```go
package scheduler

import (
  "testing"
  "time"
)

func TestSchedulerAcceptsSecondsSpec(t *testing.T) {
  s := New(time.Local)
  if err := s.AddJob("t", "0 */1 * * * *", func() error { return nil }); err != nil {
    t.Fatalf("expected seconds spec accepted, got err=%v", err)
  }
}
```

- [ ] **Step 5: 运行测试**

Run:
```bash
go test ./... -count=1
```

---

## 3) Task 3: 定义 jobs.yaml 与 api-templates.yaml 的数据结构与原子落盘

**Files:**
- Create: `internal/storage/atomic.go`
- Create: `internal/storage/jobs_store.go`
- Create: `internal/storage/templates_store.go`
- Create: `internal/storage/jobs_store_test.go`
- Create: `internal/storage/templates_store_test.go`
- Create: `jobs.yaml`
- Create: `api-templates.yaml`

- [ ] **Step 1: 原子写文件工具**

`internal/storage/atomic.go`：
```go
package storage

import (
  "fmt"
  "os"
  "path/filepath"
)

func AtomicWriteFile(filename string, data []byte, perm os.FileMode) error {
  dir := filepath.Dir(filename)
  base := filepath.Base(filename)
  tmp, err := os.CreateTemp(dir, base+".tmp-*")
  if err != nil {
    return fmt.Errorf("create temp: %w", err)
  }
  tmpName := tmp.Name()
  defer os.Remove(tmpName)

  if err := tmp.Chmod(perm); err != nil {
    _ = tmp.Close()
    return fmt.Errorf("chmod temp: %w", err)
  }
  if _, err := tmp.Write(data); err != nil {
    _ = tmp.Close()
    return fmt.Errorf("write temp: %w", err)
  }
  if err := tmp.Sync(); err != nil {
    _ = tmp.Close()
    return fmt.Errorf("sync temp: %w", err)
  }
  if err := tmp.Close(); err != nil {
    return fmt.Errorf("close temp: %w", err)
  }
  if err := os.Rename(tmpName, filename); err != nil {
    return fmt.Errorf("rename temp: %w", err)
  }
  return nil
}
```

- [ ] **Step 2: jobs.yaml 结构与校验**

`internal/storage/jobs_store.go`（要点）：
```go
type JobsFile struct {
  Version int   `yaml:"version"`
  Jobs    []Job `yaml:"jobs"`
}

type Job struct {
  Name    string                 `yaml:"name"`
  Enabled bool                   `yaml:"enabled"`
  Cron    string                 `yaml:"cron"`
  Type    string                 `yaml:"type"`
  Params  map[string]interface{} `yaml:"params"`
}

func (jf *JobsFile) Validate() error {
  // name 唯一、必填；cron 非空；type 非空
}
```

并提供：
```go
type JobsStore struct{ Path string }
func (s JobsStore) Load() (*JobsFile, error)
func (s JobsStore) Save(jf *JobsFile) error
```

注意：cron 的“可解析性”在 store 层只做非空与格式粗校验，精校验在 Runner reload 时用 scheduler.AddJob 的错误完成（避免 store 依赖 scheduler）。

- [ ] **Step 3: api-templates.yaml 结构与校验**

`internal/storage/templates_store.go`（要点）：
```go
type TemplatesFile struct {
  Version   int        `yaml:"version"`
  Templates []Template `yaml:"templates"`
}
type Template struct {
  ID       string `yaml:"id"`
  Name     string `yaml:"name"`
  Category string `yaml:"category"`
  Method   string `yaml:"method"`
  Path     string `yaml:"path"`
  // 可选 schema：v0 先保存，校验可只检查字段类型为 map
  QuerySchema      map[string]interface{} `yaml:"query_schema,omitempty"`
  PathParamsSchema map[string]interface{} `yaml:"path_params_schema,omitempty"`
  BodySchema       map[string]interface{} `yaml:"body_schema,omitempty"`

  // dry-run 支持：v0 用可选 query 参数名表示支持
  DryRunQueryParam string `yaml:"dry_run_query_param,omitempty"`
}
```

提供 `Load/Save` 与 `Validate`（ID 唯一、method 合法、path 以 `/` 开头）。

- [ ] **Step 4: 提供默认 jobs.yaml 与 api-templates.yaml**

`jobs.yaml`：初始可为空或仅提供示例（建议 `enabled:false` 的写入类示例）。

`api-templates.yaml`：至少提供（以飞连 OpenAPI 实际路径为准，先沿用当前项目示例路径）：
- users_list, users_update
- devices_list, devices_update
- policy_list, policy_update（policy_update 可设置 `dry_run_query_param: "dry_run"`，若飞连实际不同后续调整）

- [ ] **Step 5: 单测覆盖 load/save/validate**

`internal/storage/jobs_store_test.go` 示例：
```go
func TestJobsStoreLoadSave(t *testing.T) {
  dir := t.TempDir()
  p := filepath.Join(dir, "jobs.yaml")
  s := JobsStore{Path: p}
  jf := &JobsFile{Version: 1, Jobs: []Job{{Name: "a", Enabled: true, Cron: "0 */1 * * * *", Type: "api_poll"}}}
  if err := s.Save(jf); err != nil { t.Fatal(err) }
  got, err := s.Load()
  if err != nil { t.Fatal(err) }
  if got.Version != 1 || len(got.Jobs) != 1 || got.Jobs[0].Name != "a" { t.Fatalf("unexpected: %#v", got) }
}
```

- [ ] **Step 6: 运行测试**
```bash
go test ./... -count=1
```

---

## 4) Task 4: Runner（加载 config+jobs+templates、Reload、RunOnce、运行态）

**Files:**
- Create: `internal/runner/runner.go`
- Modify: `internal/config/config.go`（补齐字段）
- Test: `internal/runner/runner_test.go`（可选 v0；至少在 api/web 层覆盖）

- [ ] **Step 1: 扩展 Config 结构**

`internal/config/config.go` 增补：
```go
type SealSuiteConfig struct {
  // ...
  MockMode bool `mapstructure:"mock_mode"`
}
type ServerConfig struct {
  Bind string `mapstructure:"bind"`
  Port int    `mapstructure:"port"`
  Mode string `mapstructure:"mode"`
}
```
并在 README/样例 config 提示 `server.bind=127.0.0.1`。

- [ ] **Step 2: 实现 Runner 结构**

`internal/runner/runner.go`（核心接口）：
```go
type Runner struct {
  cfgPath       string
  jobsPath      string
  templatesPath string

  sched *scheduler.Scheduler
  client *sealsuite.Client

  jobsStore storage.JobsStore
  tplStore  storage.TemplatesStore

  mu sync.RWMutex
  jobStatus map[string]*JobStatus
  templates map[string]storage.Template
}

type JobStatus struct {
  LastRun   time.Time
  LastOK    bool
  LastError string
  DurationMs int64
  NextRun   *time.Time
}
```

- [ ] **Step 3: Runner.Start/Reload**

Reload 行为：
1) load jobs.yaml + templates.yaml
2) rebuild template map
3) stop old scheduler（若存在）
4) new scheduler（WithSeconds + location）
5) register enabled jobs
6) start scheduler

并在 register 时做 jobType 分发：
- `api_poll` / `api_write`：调用通用 API 执行器（Task 5）
- `sync_users` / `sync_devices` / `example_sync`：保留现有 handler（可在 v0 暂时兼容）

- [ ] **Step 4: Runner.RunOnce(jobName)**

直接执行一次 job func，并更新 `jobStatus[jobName]`。

- [ ] **Step 5: Runner.GetStatusSnapshot()**

返回 jobStatus 的拷贝，用于 Web 展示。

---

## 5) Task 5: 通用 API 执行器（模板库 + 通用调试器）与调用历史

**Files:**
- Create: `internal/api/types.go`
- Create: `internal/api/executor.go`
- Test: `internal/api/executor_test.go`
- Modify: `internal/sealsuite/client.go`（如需新增通用请求方法）

- [ ] **Step 1: 定义请求/响应结构**

`internal/api/types.go`：
```go
type ExecuteRequest struct {
  TemplateID string                 `json:"template_id,omitempty"`
  Method     string                 `json:"method,omitempty"`
  Path       string                 `json:"path,omitempty"`
  Query      map[string]string      `json:"query,omitempty"`
  PathParams map[string]string      `json:"path_params,omitempty"`
  Body       map[string]interface{} `json:"body,omitempty"`
}

type ExecuteResponse struct {
  HTTPStatus int             `json:"http_status"`
  LatencyMs  int64           `json:"latency_ms"`
  Body       json.RawMessage `json:"body"`

  BusinessCode    *int    `json:"business_code,omitempty"`
  BusinessMessage string  `json:"business_message,omitempty"`
}
```

- [ ] **Step 2: 实现 Executor**

`internal/api/executor.go`：
- `Executor.Execute(ctx, req)`：
  1) 若 TemplateID 非空：从 templates map 拿 Template；method/path 以模板为准
  2) 合并 pathParams：替换 `{var}`（缺失则报错）
  3) 拼接 query
  4) body：允许空；转 JSON
  5) 调用 sealsuite client 发请求（建议新增 `DoRaw(method, fullPath, query, bodyBytes)` 返回 `statusCode` 与 `rawBody`）
  6) 尝试从 JSON 中解析 `code/message`（与当前 CommonResponse 一致）填 BusinessCode/Message
  7) 记录 history（ring buffer，内存）

history：
```go
type HistoryItem struct {
  Time time.Time `json:"time"`
  TemplateID string `json:"template_id,omitempty"`
  Method string `json:"method"`
  Path string `json:"path"`
  HTTPStatus int `json:"http_status"`
  LatencyMs int64 `json:"latency_ms"`
  OK bool `json:"ok"`
}
```

- [ ] **Step 3: 单测（httptest 模拟飞连 API）**

`internal/api/executor_test.go`：
1) 用 `httptest.NewServer` 提供 `/api/v1/users` 返回 `{"code":0,"message":"success","data":{"total":1}}`
2) client baseURL 指向测试 server
3) templates map 含 `users_list`
4) 调用 `Execute` 断言：
   - HTTPStatus=200
   - BusinessCode=0

- [ ] **Step 4: 运行测试**
```bash
go test ./... -count=1
```

---

## 6) Task 6: dry-run + 变更预览（preview API）

**Files:**
- Create: `internal/api/preview.go`
- Test: `internal/api/preview_test.go`

- [ ] **Step 1: 定义 Preview API 结构**

在 `internal/api/types.go` 增加：
```go
type PreviewRequest struct {
  ExecuteRequest
  PreviewMode string `json:"preview_mode"` // "dry_run" | "diff_only"
  ReadBeforeWrite bool `json:"read_before_write"`
}

type DiffItem struct {
  Path string      `json:"path"`
  Before any       `json:"before,omitempty"`
  After  any       `json:"after,omitempty"`
}

type PreviewResponse struct {
  DryRunSupported bool       `json:"dry_run_supported"`
  RequestSummary  any        `json:"request_summary"`
  Diff            []DiffItem `json:"diff,omitempty"`
  Warnings        []string   `json:"warnings,omitempty"`
  DryRunResult    *ExecuteResponse `json:"dry_run_result,omitempty"`
}
```

- [ ] **Step 2: 实现 Preview(ctx, req)**

规则（v0）：
1) 若 `PreviewMode=="dry_run"` 且模板存在 `DryRunQueryParam`：
   - 在 query 里追加 `<param>=true`，调用 `Executor.Execute`，并将结果放入 `DryRunResult`
   - `DryRunSupported=true`
2) 否则走 `diff_only`：
   - 若 `ReadBeforeWrite=true`：先对同 path 发送 GET 获取当前状态（使用 executor 的底层请求能力），失败则在 warnings 记录并继续
   - 将“当前状态 JSON（若拿到）”与“即将写入 Body”做一个简化 diff：
     - 只对 body 的顶层 key 做对比（v0 简化），生成 `DiffItem{Path:"$.k",Before:x,After:y}`

注：v0 diff 先做“顶层字段变更”即可，后续再扩展深度 diff。

- [ ] **Step 3: 单测**

`internal/api/preview_test.go`：
- dry_run_supported：模板设置 `DryRunQueryParam:"dry_run"`，server 若收到 query `dry_run=true` 返回 `code:0`
- diff_only：server GET 返回 `{"name":"old"}`，写入 body 为 `{"name":"new"}`，断言 Diff 包含 `$.name`

- [ ] **Step 4: 运行测试**
```bash
go test ./... -count=1
```

---

## 7) Task 7: Web Server（静态前端 embed + REST API 路由）

**Files:**
- Create: `internal/web/assets/embed.go`
- Create: `internal/web/server.go`
- Create: `internal/web/routes.go`
- Create: `internal/web/handlers_health.go`
- Create: `internal/web/handlers_config.go`
- Create: `internal/web/handlers_jobs.go`
- Create: `internal/web/handlers_api.go`
- Create: `internal/web/handlers_logs.go`
- Create: `web/ui/index.html`, `web/ui/app.js`, `web/ui/styles.css`
- Test: `internal/web/server_test.go`

- [ ] **Step 1: go:embed 静态资源**

`internal/web/assets/embed.go`：
```go
package assets

import "embed"

//go:embed ../../web/ui/*
var FS embed.FS
```

- [ ] **Step 2: server 构建与路由**

`internal/web/server.go`：
- `NewServer(runner *runner.Runner, executor *api.Executor, addr string) *http.Server`
- 设置 ReadHeaderTimeout、IdleTimeout 等合理默认值

`internal/web/routes.go`：
- `GET /` 返回 `web/ui/index.html`
- `GET /assets/*` 返回 js/css
- `/api/v1/*` 注册各 handler

- [ ] **Step 3: handler 实现（v0）**

health：
- `GET /api/v1/health`：返回 `{ok:true, jobs:count}` + Runner 状态

config：
- `GET /api/v1/config`：读取 config.yaml（建议通过 config 包重新 Load），脱敏返回
- `PUT /api/v1/config`：保存（原子写盘）→ runner.Reload（若需要）→ 返回 ok

jobs：
- `GET /api/v1/jobs`：jobs.yaml + Runner status snapshot 合并返回
- `PUT /api/v1/jobs`：保存 jobs.yaml（原子写盘）→ runner.Reload
- `POST /api/v1/jobs/{name}/enable|disable`：修改 job.enabled 并保存 → reload
- `POST /api/v1/jobs/{name}/run`：runner.RunOnce

api toolbox：
- `GET /api/v1/api/templates`
- `PUT /api/v1/api/templates`
- `POST /api/v1/api/execute`
- `POST /api/v1/api/preview`
- `GET /api/v1/api/history`
- `POST /api/v1/api/save-as-job`：写入 jobs.yaml，写入类默认 enabled=false 且必须先 preview（v0 规则：请求里必须带 `safety.dry_run=true`）

logs：
- `GET /api/v1/logs?tail=500`：读取 `cfg.Log.Filename` 文件尾部（按字节截断）

- [ ] **Step 4: 前端最小实现**

`web/ui/index.html`：侧边栏 4 tab（Dashboard/Config/Jobs/API/Logs）中的简化版本即可（至少 Jobs + API + Preview 可操作）。

`web/ui/app.js`：封装 `fetchJSON(url, opts)`，实现：
- 列表展示 templates
- 调试器表单：method/path/query/body JSON textarea
- Preview 按钮调用 `/api/v1/api/preview` 并展示 diff
- Execute 按钮调用 `/api/v1/api/execute`
- Save-as-job 表单：job_name/cron/type(api_poll/api_write) 并提交

- [ ] **Step 5: server 冒烟测试**

`internal/web/server_test.go`：
```go
func TestStaticIndexServed(t *testing.T) {
  // 构建 router 后用 httptest.NewRecorder + ServeHTTP
  // 断言 GET / 返回 200 且包含 "<!doctype" 或页面标题
}
```

- [ ] **Step 6: 运行测试**
```bash
go test ./... -count=1
```

---

## 8) Task 8: main.go 组装 Runner + WebServer + 优雅退出

**Files:**
- Modify: `cmd/main.go`
- Modify: `config.yaml`（示例化/去密钥）

- [ ] **Step 1: main.go 改造**

目标：
- 加载 config
- init logger
- init sealsuite client（mockMode 来自 config）
- init Runner（paths：config.yaml/jobs.yaml/api-templates.yaml）
- init Executor（依赖 client + templates store/runner 提供模板 map）
- 启动 WebServer（bind+port）
- 捕获 SIGINT/SIGTERM：按顺序 stop http server（Shutdown）→ runner.Stop → logger.Sync

- [ ] **Step 2: 删除/避免写死 mockMode**

移除：
```go
sealSuiteClient.SetMockMode(true)
```
改为：
```go
sealSuiteClient.SetMockMode(cfg.SealSuite.MockMode)
```

- [ ] **Step 3: 更新 config.yaml 样例**

将 access_key/secret_key 改为占位符，并增加：
```yaml
sealsuite:
  mock_mode: false
server:
  bind: "127.0.0.1"
  port: 8080
```

- [ ] **Step 4: 手工验证（本地）**

Run:
```bash
go run ./cmd/main.go
```
Expected：
- 日志提示 web server listening on 127.0.0.1:8080
- 浏览器访问 `http://127.0.0.1:8080/` 能打开页面

---

## 9) Task 9: 端到端用例（验证两类场景）

**Files:**
- Doc: `README.md`（可选更新）

- [ ] **Step 1: 单次任务场景（API 工具箱）**
1) 在 Templates 页选择 `users_list`，执行 GET，看到响应与业务码解析
2) 在 Preview 中对 `policy_update` 输入 body，查看 diff 预览
3) 执行 dry-run（若模板配置 dry_run_query_param）返回服务端校验结果（或显示不支持并 fallback diff_only）

- [ ] **Step 2: 周期性任务场景**
1) 用 Save-as-job 将 `users_list` 保存为 `api_poll`（cron: `0 */1 * * * *`）
2) runner reload 后 jobs 列表看到 next_run
3) 创建 `api_write`（policy_update），确认默认 disabled，且必须先 preview；启用后定期执行（v0 可先仅日志记录）

---

## 10) Self-Review（对照 spec 覆盖检查）
- 覆盖“单次任务（模板库+调试器）”：Task 5 + Task 7 + Task 9
- 覆盖“周期性任务（查询/写入）”：Task 4 + Task 7(save-as-job) + Task 9
- 覆盖“dry-run + 变更预览”：Task 6 + Task 7(api/preview) + Task 9
- 覆盖“秒级 cron 与一致性”：Task 2 + Task 4 + Task 8(config/README)

---

## Execution Handoff

Plan complete and saved to `docs/superpowers/plans/2026-06-04-feilian-ops-web-console.md`. Two execution options:

1) **Subagent-Driven (recommended)** - Use superpowers:subagent-driven-development, fresh subagent per task, review between tasks
2) **Inline Execution** - Use superpowers:executing-plans, execute tasks in this session with checkpoints

Which approach?

