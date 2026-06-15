# Google IP 同步定时任务 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 在“飞连任务列表-定时任务清单”中新增一种可手工配置的 `外部 IP 同步` 任务，支持每天抓取 Google `goog.json` 并把新增 CIDR 增量追加到指定飞连 IP 资源。

**Architecture:** 保持现有 `job_schedule` 作为调度层，新增 `external_ip_sync_task` 作为任务定义层，并在 Runner 中新增 `target_type=external_ip_sync` 的专用执行器。Web 侧在现有“新建调度”流程中增加场景化表单，列表和详情补充同步摘要展示。

**Tech Stack:** Go 1.21、SQLite、Chi、现有 Runner/Scheduler、原生前端 `internal/web/assets/ui/*`

---

## 文件结构

### 新增后端模型与仓储

- Create: `internal/storage/external_ip_sync_types.go`
  - `ExternalIPSyncTask` 类型与校验
- Modify: `internal/repository/interfaces.go`
  - 新增 `ExternalIPSyncTaskRepository`
- Create: `internal/repository/sqlite/external_ip_sync_tasks_repo.go`
  - SQLite 持久化
- Create: `internal/repository/sqlite/external_ip_sync_tasks_repo_test.go`
  - repo 测试
- Modify: `internal/db/migrate.go`
  - 新增 `external_ip_sync_tasks` 表

### 新增服务与执行器

- Create: `internal/service/external_ip_sync.go`
  - 任务保存、校验、资源选择规范化
- Create: `internal/service/external_ip_sync_test.go`
- Modify: `internal/service/services.go`
  - 挂入 `ExternalIPSync`
- Modify: `internal/runner/runner.go`
  - 支持 `target_type=external_ip_sync`
- Modify: `internal/runner/runner_test.go`
  - 新增执行器测试

### 新增 Web API 与前端页面

- Modify: `internal/web/server.go`
  - 新增任务 CRUD / 列表 / 详情 / 资源下拉接口
- Modify: `internal/web/server_test.go`
  - API 测试
- Modify: `internal/web/assets/ui/index.html`
  - 调度新建入口增加任务类型选择与专用表单
- Modify: `internal/web/assets/ui/app.js`
  - 表单渲染、列表展示、详情摘要
- Modify: `internal/web/assets/ui/styles.css`
  - 外部 IP 同步表单与摘要样式

### 文档

- Modify: `README.md`
  - 追加“外部 IP 同步任务”说明

---

### Task 1: 建立 `external_ip_sync_task` 数据模型与 SQLite 表

**Files:**
- Create: `internal/storage/external_ip_sync_types.go`
- Modify: `internal/db/migrate.go`
- Create: `internal/repository/sqlite/external_ip_sync_tasks_repo.go`
- Create: `internal/repository/sqlite/external_ip_sync_tasks_repo_test.go`
- Modify: `internal/repository/interfaces.go`

- [ ] **Step 1: 先写 SQLite repo 失败测试**

在 `internal/repository/sqlite/external_ip_sync_tasks_repo_test.go` 新建：

```go
package sqlite

import (
	"path/filepath"
	"testing"

	appdb "sealsuite-operation/internal/db"
	"sealsuite-operation/internal/storage"
)

func TestExternalIPSyncTaskRepositoryUpsertAndGet(t *testing.T) {
	db, err := appdb.OpenSQLite(filepath.Join(t.TempDir(), "app.db"))
	if err != nil {
		t.Fatalf("OpenSQLite err=%v", err)
	}
	defer db.Close()
	if err := appdb.Migrate(db); err != nil {
		t.Fatalf("Migrate err=%v", err)
	}

	repo := NewExternalIPSyncTaskRepository(db)
	task := storage.ExternalIPSyncTask{
		ID:             "google_ipv4_sync",
		Name:           "Google IPv4 同步",
		SourceType:     "google_ip_ranges",
		SourceURL:      "https://www.gstatic.com/ipranges/goog.json",
		IPVersion:      "ipv4",
		ResourceID:     "res_google",
		WriteAction:    "append_if_missing",
		FeilianAPIPath: "/api/open/v1/addr/management/add",
		SkipWhenEmpty:  true,
	}
	if err := repo.Upsert(task); err != nil {
		t.Fatalf("Upsert err=%v", err)
	}

	got, found, err := repo.Get("google_ipv4_sync")
	if err != nil || !found {
		t.Fatalf("Get found=%v err=%v", found, err)
	}
	if got.IPVersion != "ipv4" || got.ResourceID != "res_google" {
		t.Fatalf("unexpected task: %+v", got)
	}
}
```

- [ ] **Step 2: 运行测试，确认失败**

Run:

```bash
go test ./internal/repository/sqlite -run TestExternalIPSyncTaskRepositoryUpsertAndGet -count=1
```

Expected:

- FAIL
- 缺少 `ExternalIPSyncTask`、repo 或表结构

- [ ] **Step 3: 新增类型定义与最小校验**

在 `internal/storage/external_ip_sync_types.go` 写：

```go
package storage

import (
	"fmt"
	"strings"
)

type ExternalIPSyncTask struct {
	ID                   string `json:"id"`
	Name                 string `json:"name"`
	SourceType           string `json:"source_type"`
	SourceURL            string `json:"source_url"`
	IPVersion            string `json:"ip_version"`
	ResourceID           string `json:"resource_id"`
	ResourceNameSnapshot string `json:"resource_name_snapshot"`
	WriteAction          string `json:"write_action"`
	FeilianAPIPath       string `json:"feilian_api_path"`
	DryRun               bool   `json:"dry_run"`
	SkipWhenEmpty        bool   `json:"skip_when_empty"`
	Enabled              bool   `json:"enabled"`
	CreatedAt            string `json:"created_at"`
	UpdatedAt            string `json:"updated_at"`
}

func NormalizeExternalIPSyncTask(in ExternalIPSyncTask) ExternalIPSyncTask {
	in.ID = strings.TrimSpace(in.ID)
	in.Name = strings.TrimSpace(in.Name)
	in.SourceType = strings.TrimSpace(in.SourceType)
	in.SourceURL = strings.TrimSpace(in.SourceURL)
	in.IPVersion = strings.TrimSpace(in.IPVersion)
	in.ResourceID = strings.TrimSpace(in.ResourceID)
	in.ResourceNameSnapshot = strings.TrimSpace(in.ResourceNameSnapshot)
	in.WriteAction = strings.TrimSpace(in.WriteAction)
	in.FeilianAPIPath = strings.TrimSpace(in.FeilianAPIPath)
	if in.SourceType == "" {
		in.SourceType = "google_ip_ranges"
	}
	if in.SourceURL == "" {
		in.SourceURL = "https://www.gstatic.com/ipranges/goog.json"
	}
	if in.IPVersion == "" {
		in.IPVersion = "ipv4"
	}
	if in.WriteAction == "" {
		in.WriteAction = "append_if_missing"
	}
	if in.FeilianAPIPath == "" {
		in.FeilianAPIPath = "/api/open/v1/addr/management/add"
	}
	return in
}

func (t ExternalIPSyncTask) Validate() error {
	if t.ID == "" {
		return fmt.Errorf("external ip sync task: id is required")
	}
	if t.ResourceID == "" {
		return fmt.Errorf("external ip sync task: resource_id is required")
	}
	switch t.IPVersion {
	case "ipv4", "ipv6", "all":
	default:
		return fmt.Errorf("external ip sync task: invalid ip_version %q", t.IPVersion)
	}
	return nil
}
```

- [ ] **Step 4: 新增 migration 与 repo**

在 `internal/db/migrate.go` 增加表：

```go
`CREATE TABLE IF NOT EXISTS external_ip_sync_tasks (
	id TEXT PRIMARY KEY,
	name TEXT NOT NULL,
	source_type TEXT NOT NULL,
	source_url TEXT NOT NULL,
	ip_version TEXT NOT NULL,
	resource_id TEXT NOT NULL,
	resource_name_snapshot TEXT NOT NULL DEFAULT '',
	write_action TEXT NOT NULL,
	feilian_api_path TEXT NOT NULL,
	dry_run INTEGER NOT NULL DEFAULT 0,
	skip_when_empty INTEGER NOT NULL DEFAULT 1,
	enabled INTEGER NOT NULL DEFAULT 1,
	created_at TEXT NOT NULL,
	updated_at TEXT NOT NULL
);`,
```

在 `internal/repository/interfaces.go` 增加：

```go
type ExternalIPSyncTaskRepository interface {
	List() ([]storage.ExternalIPSyncTask, error)
	Get(id string) (storage.ExternalIPSyncTask, bool, error)
	Upsert(task storage.ExternalIPSyncTask) error
	Delete(id string) error
}
```

在 `internal/repository/sqlite/external_ip_sync_tasks_repo.go` 实现最小版本：

```go
type ExternalIPSyncTaskRepository struct{ baseRepo }

func NewExternalIPSyncTaskRepository(db *sql.DB) *ExternalIPSyncTaskRepository {
	return &ExternalIPSyncTaskRepository{baseRepo{db: db}}
}
```

并补 `List/Get/Upsert/Delete`，字段按表结构直存。

- [ ] **Step 5: 运行测试，确认通过**

Run:

```bash
go test ./internal/repository/sqlite -run TestExternalIPSyncTaskRepositoryUpsertAndGet -count=1
```

Expected:

- PASS

- [ ] **Step 6: Commit**

```bash
git add internal/storage/external_ip_sync_types.go internal/db/migrate.go internal/repository/interfaces.go internal/repository/sqlite/external_ip_sync_tasks_repo.go internal/repository/sqlite/external_ip_sync_tasks_repo_test.go
git commit -m "feat: add external ip sync task storage"
```

---

### Task 2: 接入 service 层并提供任务保存规则

**Files:**
- Create: `internal/service/external_ip_sync.go`
- Create: `internal/service/external_ip_sync_test.go`
- Modify: `internal/service/services.go`

- [ ] **Step 1: 先写 service 失败测试**

在 `internal/service/external_ip_sync_test.go` 写：

```go
package service

import (
	"testing"

	"sealsuite-operation/internal/storage"
)

type fakeExternalIPSyncRepo struct {
	last storage.ExternalIPSyncTask
}

func (f *fakeExternalIPSyncRepo) List() ([]storage.ExternalIPSyncTask, error) { return nil, nil }
func (f *fakeExternalIPSyncRepo) Get(id string) (storage.ExternalIPSyncTask, bool, error) {
	return storage.ExternalIPSyncTask{}, false, nil
}
func (f *fakeExternalIPSyncRepo) Upsert(task storage.ExternalIPSyncTask) error {
	f.last = task
	return nil
}
func (f *fakeExternalIPSyncRepo) Delete(id string) error { return nil }

func TestExternalIPSyncServiceSaveNormalizesDefaults(t *testing.T) {
	repo := &fakeExternalIPSyncRepo{}
	svc := NewExternalIPSyncService(repo)
	err := svc.Save(storage.ExternalIPSyncTask{
		ID:         "google_ipv6_sync",
		Name:       "Google IPv6 同步",
		IPVersion:  "ipv6",
		ResourceID: "res_v6",
	})
	if err != nil {
		t.Fatalf("Save err=%v", err)
	}
	if repo.last.SourceType != "google_ip_ranges" {
		t.Fatalf("unexpected source type: %+v", repo.last)
	}
	if repo.last.FeilianAPIPath != "/api/open/v1/addr/management/add" {
		t.Fatalf("unexpected api path: %+v", repo.last)
	}
}
```

- [ ] **Step 2: 运行测试，确认失败**

Run:

```bash
go test ./internal/service -run TestExternalIPSyncServiceSaveNormalizesDefaults -count=1
```

Expected:

- FAIL
- `NewExternalIPSyncService` 尚不存在

- [ ] **Step 3: 实现 service**

在 `internal/service/external_ip_sync.go` 写：

```go
package service

import (
	"sealsuite-operation/internal/repository"
	"sealsuite-operation/internal/storage"
)

type ExternalIPSyncService struct {
	repo repository.ExternalIPSyncTaskRepository
}

func NewExternalIPSyncService(repo repository.ExternalIPSyncTaskRepository) *ExternalIPSyncService {
	return &ExternalIPSyncService{repo: repo}
}

func (s *ExternalIPSyncService) Save(task storage.ExternalIPSyncTask) error {
	task = storage.NormalizeExternalIPSyncTask(task)
	if err := task.Validate(); err != nil {
		return err
	}
	return s.repo.Upsert(task)
}
```

在 `internal/service/services.go` 挂入：

```go
type Services struct {
	TaskDrafts     *TaskDraftService
	Schedules      *ScheduleService
	ComplexTasks   *ComplexTaskService
	ExternalIPSync *ExternalIPSyncService
}
```

- [ ] **Step 4: 运行测试，确认通过**

Run:

```bash
go test ./internal/service -run TestExternalIPSyncServiceSaveNormalizesDefaults -count=1
```

Expected:

- PASS

- [ ] **Step 5: Commit**

```bash
git add internal/service/external_ip_sync.go internal/service/external_ip_sync_test.go internal/service/services.go
git commit -m "feat: add external ip sync service"
```

---

### Task 3: 在 Web API 中增加外部 IP 同步任务 CRUD

**Files:**
- Modify: `internal/web/server.go`
- Modify: `internal/web/server_test.go`

- [ ] **Step 1: 先写保存与查询 API 失败测试**

在 `internal/web/server_test.go` 增加：

```go
func TestExternalIPSyncTaskRoutesSaveAndList(t *testing.T) {
	srv, _ := newTestServer(t)

	body := `{
	  "id":"google_ipv4_sync",
	  "name":"Google IPv4 同步",
	  "ip_version":"ipv4",
	  "resource_id":"res_google",
	  "feilian_api_path":"/api/open/v1/addr/management/add"
	}`

	resp := doJSONRequest(t, srv, "POST", "/api/v1/external-ip-sync-tasks", body)
	assertStatus(t, resp, 200)

	list := doJSONRequest(t, srv, "GET", "/api/v1/external-ip-sync-tasks", "")
	assertStatus(t, list, 200)
	assertBodyContains(t, list, `"google_ipv4_sync"`)
	assertBodyContains(t, list, `"ipv4"`)
}
```

- [ ] **Step 2: 运行测试，确认失败**

Run:

```bash
go test ./internal/web -run TestExternalIPSyncTaskRoutesSaveAndList -count=1
```

Expected:

- FAIL
- 接口不存在或返回 404

- [ ] **Step 3: 在 `server.go` 接入 repo/service 与路由**

在 `newRouter` 初始化区加入：

```go
var externalIPSyncRepo *sqliteRepo.ExternalIPSyncTaskRepository
if appDB != nil {
	externalIPSyncRepo = sqliteRepo.NewExternalIPSyncTaskRepository(appDB)
}
if services == nil {
	services = service.NewServices(
		taskDraftRepo,
		scheduleRepo,
		complexTaskRepo,
		externalIPSyncRepo,
	)
}
```

并新增路由：

```go
rr.Route("/api/v1/external-ip-sync-tasks", func(r chi.Router) {
	r.Get("/", func(w http.ResponseWriter, r *http.Request) { ... })
	r.Post("/", func(w http.ResponseWriter, r *http.Request) { ... })
	r.Get("/{id}", func(w http.ResponseWriter, r *http.Request) { ... })
	r.Delete("/{id}", func(w http.ResponseWriter, r *http.Request) { ... })
})
```

POST 路由里调用：

```go
if err := services.ExternalIPSync.Save(in); err != nil {
	writeJSON(w, 400, map[string]interface{}{"error": err.Error()})
	return
}
```

- [ ] **Step 4: 运行测试，确认通过**

Run:

```bash
go test ./internal/web -run TestExternalIPSyncTaskRoutesSaveAndList -count=1
```

Expected:

- PASS

- [ ] **Step 5: Commit**

```bash
git add internal/web/server.go internal/web/server_test.go
git commit -m "feat: add external ip sync task api routes"
```

---

### Task 4: 在 Runner 中增加 `target_type=external_ip_sync` 执行路径

**Files:**
- Modify: `internal/runner/runner.go`
- Modify: `internal/runner/runner_test.go`

- [ ] **Step 1: 先写执行路由失败测试**

在 `internal/runner/runner_test.go` 增加：

```go
func TestRunScheduleExecutesExternalIPSyncTarget(t *testing.T) {
	r := newTestRunner(t)
	called := false
	r.executeExternalIPSync = func(id string) error {
		called = id == "google_ipv4_sync"
		return nil
	}

	err := r.executeTargetOutput("external_ip_sync", "google_ipv4_sync")
	if err != nil {
		t.Fatalf("executeTargetOutput err=%v", err)
	}
	if !called {
		t.Fatal("expected external ip sync executor called")
	}
}
```

- [ ] **Step 2: 运行测试，确认失败**

Run:

```bash
go test ./internal/runner -run TestRunScheduleExecutesExternalIPSyncTarget -count=1
```

Expected:

- FAIL
- 缺少执行分支

- [ ] **Step 3: 增加执行分支和执行器骨架**

在 `internal/runner/runner.go` 的结构体中新增：

```go
externalIPSyncRepo *sqliteRepo.ExternalIPSyncTaskRepository
executeExternalIPSync func(id string) error
```

初始化：

```go
r.externalIPSyncRepo = sqliteRepo.NewExternalIPSyncTaskRepository(appDB)
r.executeExternalIPSync = r.runExternalIPSyncTask
```

在 `executeTargetOutput(...)` 中增加：

```go
case "external_ip_sync":
	return nil, r.executeExternalIPSync(targetID)
```

增加骨架：

```go
func (r *Runner) runExternalIPSyncTask(id string) error {
	task, found, err := r.externalIPSyncRepo.Get(id)
	if err != nil {
		return err
	}
	if !found {
		return fmt.Errorf("external ip sync task %s not found", id)
	}
	_ = task
	return nil
}
```

- [ ] **Step 4: 运行测试，确认通过**

Run:

```bash
go test ./internal/runner -run TestRunScheduleExecutesExternalIPSyncTarget -count=1
```

Expected:

- PASS

- [ ] **Step 5: Commit**

```bash
git add internal/runner/runner.go internal/runner/runner_test.go
git commit -m "feat: route schedules to external ip sync executor"
```

---

### Task 5: 实现 Google `goog.json` 拉取、过滤与增量计算

**Files:**
- Modify: `internal/runner/runner.go`
- Modify: `internal/runner/runner_test.go`

- [ ] **Step 1: 先写过滤与差集测试**

在 `internal/runner/runner_test.go` 增加：

```go
func TestFilterGooglePrefixesAndDiff(t *testing.T) {
	source := googleIPRanges{
		Prefixes: []googlePrefix{
			{IPv4Prefix: "1.1.1.0/24"},
			{IPv4Prefix: "2.2.2.0/24"},
			{IPv6Prefix: "2001:db8::/32"},
		},
	}
	got := filterGoogleCIDRs(source, "ipv4")
	if len(got) != 2 {
		t.Fatalf("expected 2 ipv4 prefixes, got=%v", got)
	}
	toAdd := diffCIDRs([]string{"1.1.1.0/24", "2.2.2.0/24"}, []string{"2.2.2.0/24"})
	if len(toAdd) != 1 || toAdd[0] != "1.1.1.0/24" {
		t.Fatalf("unexpected diff: %v", toAdd)
	}
}
```

- [ ] **Step 2: 运行测试，确认失败**

Run:

```bash
go test ./internal/runner -run TestFilterGooglePrefixesAndDiff -count=1
```

Expected:

- FAIL
- 相关类型或函数不存在

- [ ] **Step 3: 实现最小拉取与过滤逻辑**

在 `internal/runner/runner.go` 增加：

```go
type googlePrefix struct {
	IPv4Prefix string `json:"ipv4Prefix"`
	IPv6Prefix string `json:"ipv6Prefix"`
}

type googleIPRanges struct {
	Prefixes []googlePrefix `json:"prefixes"`
}

func filterGoogleCIDRs(in googleIPRanges, version string) []string {
	set := map[string]struct{}{}
	for _, p := range in.Prefixes {
		if (version == "ipv4" || version == "all") && strings.TrimSpace(p.IPv4Prefix) != "" {
			set[strings.TrimSpace(p.IPv4Prefix)] = struct{}{}
		}
		if (version == "ipv6" || version == "all") && strings.TrimSpace(p.IPv6Prefix) != "" {
			set[strings.TrimSpace(p.IPv6Prefix)] = struct{}{}
		}
	}
	out := make([]string, 0, len(set))
	for cidr := range set {
		out = append(out, cidr)
	}
	sort.Strings(out)
	return out
}

func diffCIDRs(source, existing []string) []string {
	exists := map[string]struct{}{}
	for _, cidr := range existing {
		exists[cidr] = struct{}{}
	}
	out := make([]string, 0)
	for _, cidr := range source {
		if _, ok := exists[cidr]; ok {
			continue
		}
		out = append(out, cidr)
	}
	return out
}
```

- [ ] **Step 4: 运行测试，确认通过**

Run:

```bash
go test ./internal/runner -run TestFilterGooglePrefixesAndDiff -count=1
```

Expected:

- PASS

- [ ] **Step 5: Commit**

```bash
git add internal/runner/runner.go internal/runner/runner_test.go
git commit -m "feat: add google ip filtering and cidr diff"
```

---

### Task 6: 完成飞连资源读取、写入摘要与 dry-run / skip-when-empty

**Files:**
- Modify: `internal/runner/runner.go`
- Modify: `internal/service/execution.go`
- Modify: `internal/runner/runner_test.go`

- [ ] **Step 1: 先写 dry-run 行为测试**

在 `internal/runner/runner_test.go` 增加：

```go
func TestRunExternalIPSyncTaskDryRunSkipsWrite(t *testing.T) {
	r := newTestRunner(t)
	r.fetchGoogleCIDRs = func(task storage.ExternalIPSyncTask) ([]string, error) {
		return []string{"1.1.1.0/24"}, nil
	}
	r.loadFeilianResourceCIDRs = func(resourceID string) ([]string, error) {
		return []string{}, nil
	}
	wrote := false
	r.writeFeilianCIDRs = func(task storage.ExternalIPSyncTask, cidrs []string) error {
		wrote = true
		return nil
	}

	err := r.runExternalIPSyncTaskWithTask(storage.ExternalIPSyncTask{
		ID:            "google_ipv4_sync",
		IPVersion:     "ipv4",
		ResourceID:    "res_google",
		DryRun:        true,
		SkipWhenEmpty: true,
	})
	if err != nil {
		t.Fatalf("runExternalIPSyncTaskWithTask err=%v", err)
	}
	if wrote {
		t.Fatal("expected no write in dry-run")
	}
}
```

- [ ] **Step 2: 运行测试，确认失败**

Run:

```bash
go test ./internal/runner -run TestRunExternalIPSyncTaskDryRunSkipsWrite -count=1
```

Expected:

- FAIL

- [ ] **Step 3: 实现执行器最小闭环**

在 `internal/runner/runner.go` 为 `Runner` 增加可替换依赖：

```go
fetchGoogleCIDRs func(task storage.ExternalIPSyncTask) ([]string, error)
loadFeilianResourceCIDRs func(resourceID string) ([]string, error)
writeFeilianCIDRs func(task storage.ExternalIPSyncTask, cidrs []string) error
```

并实现：

```go
func (r *Runner) runExternalIPSyncTaskWithTask(task storage.ExternalIPSyncTask) error {
	sourceCIDRs, err := r.fetchGoogleCIDRs(task)
	if err != nil {
		return err
	}
	existingCIDRs, err := r.loadFeilianResourceCIDRs(task.ResourceID)
	if err != nil {
		return err
	}
	toAdd := diffCIDRs(sourceCIDRs, existingCIDRs)
	if len(toAdd) == 0 && task.SkipWhenEmpty {
		return nil
	}
	if task.DryRun {
		return nil
	}
	return r.writeFeilianCIDRs(task, toAdd)
}
```

- [ ] **Step 4: 运行测试，确认通过**

Run:

```bash
go test ./internal/runner -run TestRunExternalIPSyncTaskDryRunSkipsWrite -count=1
```

Expected:

- PASS

- [ ] **Step 5: Commit**

```bash
git add internal/runner/runner.go internal/runner/runner_test.go internal/service/execution.go
git commit -m "feat: add external ip sync dry-run and append flow"
```

---

### Task 7: 在前端“定时任务清单”增加 `外部 IP 同步` 表单

**Files:**
- Modify: `internal/web/assets/ui/index.html`
- Modify: `internal/web/assets/ui/app.js`
- Modify: `internal/web/assets/ui/styles.css`

- [ ] **Step 1: 先写表单渲染失败测试说明**

由于当前前端没有独立 JS 测试基建，本任务使用最小人工验收脚本替代自动化测试。先记录预期：

```text
进入“飞连任务列表-定时任务清单” -> 新建调度
应先选择任务类型：常规调度 / 外部 IP 同步
选择“外部 IP 同步”后，应出现：
- 数据源（Google IP Ranges）
- IP 版本（IPv4 / IPv6 / 全部）
- 目标 IP 资源下拉
- resource_id 手填
- 写入接口路径
- 每日定时
- dry-run / 空结果跳过
```

- [ ] **Step 2: 在 `index.html` 增加任务类型选择和专用表单占位**

在任务抽屉或高级表单中追加：

```html
<div class="field">
  <label>任务类型</label>
  <select id="job-drawer-kind">
    <option value="schedule">常规调度</option>
    <option value="external_ip_sync">外部 IP 同步</option>
  </select>
</div>
<div id="job-drawer-external-ip-sync" class="hidden"></div>
```

- [ ] **Step 3: 在 `app.js` 实现表单切换与 payload 组装**

新增函数：

```js
function renderExternalIPSyncJobFields(task) { ... }
function readExternalIPSyncJobPayload() { ... }
```

`readExternalIPSyncJobPayload()` 需要返回：

```js
{
  id: qs('#external-sync-id').value.trim(),
  name: qs('#external-sync-name').value.trim(),
  source_type: 'google_ip_ranges',
  source_url: 'https://www.gstatic.com/ipranges/goog.json',
  ip_version: qs('#external-sync-ip-version').value,
  resource_id: qs('#external-sync-resource-id').value.trim(),
  resource_name_snapshot: qs('#external-sync-resource-name').value.trim(),
  feilian_api_path: qs('#external-sync-api-path').value,
  dry_run: !!qs('#external-sync-dry-run').checked,
  skip_when_empty: !!qs('#external-sync-skip-empty').checked,
  enabled: qs('#job-drawer-enabled').value === 'true'
}
```

- [ ] **Step 4: 手工验证**

Run:

```bash
go run ./cmd/main.go
```

Expected:

- “新建调度”里能选 `外部 IP 同步`
- 表单字段齐全
- 切回 `常规调度` 时旧表单仍可用

- [ ] **Step 5: Commit**

```bash
git add internal/web/assets/ui/index.html internal/web/assets/ui/app.js internal/web/assets/ui/styles.css
git commit -m "feat: add external ip sync job form"
```

---

### Task 8: 列表与详情增加同步摘要，并补文档

**Files:**
- Modify: `internal/web/assets/ui/app.js`
- Modify: `internal/web/server.go`
- Modify: `README.md`

- [ ] **Step 1: 先写列表摘要预期**

记录前端摘要预期：

```text
外部 IP 同步任务在列表中显示：
Google IP Ranges / IPv4 -> 目标资源
每日 00:00
最近一次：新增 x 条，跳过 y 条，成功/失败
```

- [ ] **Step 2: 在后端详情接口中补结构化摘要字段**

在 `server.go` 对 `external_ip_sync` 详情返回中补：

```go
map[string]interface{}{
  "source_total": ...,
  "filtered_total": ...,
  "existing_total": ...,
  "to_add_total": ...,
  "added_total": ...,
  "write_api_path": ...,
  "dry_run": ...,
}
```

- [ ] **Step 3: 在前端列表/详情展示摘要**

在 `app.js` 中对这类任务渲染：

```js
const summary = run && run.summary ? run.summary : {};
const meta = `最近一次：新增 ${summary.added_total || 0} 条，跳过 ${summary.existing_total || 0} 条`;
```

详情区显示：

```js
<div class="k">数据源</div><div class="v">Google IP Ranges</div>
<div class="k">IP 版本</div><div class="v">${escapeHtml(task.ip_version || '')}</div>
<div class="k">目标资源</div><div class="v"><code>${escapeHtml(task.resource_id || '')}</code></div>
<div class="k">新增数量</div><div class="v">${summary.added_total || 0}</div>
```

- [ ] **Step 4: 更新 README**

在 `README.md` 增加一段：

```md
### 外部 IP 同步定时任务

支持在“飞连任务列表-定时任务清单”中创建 `外部 IP 同步` 任务。第一版固定支持从 Google `goog.json` 抓取 IPv4/IPv6 CIDR，并按增量追加策略写入指定飞连 IP 资源。
```

- [ ] **Step 5: 手工验收并 Commit**

Run:

```bash
go test ./internal/web ./internal/runner
go run ./cmd/main.go
```

Expected:

- 列表有外部 IP 同步摘要
- 详情有同步结果摘要
- README 文案已补充

```bash
git add internal/web/assets/ui/app.js internal/web/server.go README.md
git commit -m "feat: show external ip sync summaries"
```

---

## 自检

### Spec 覆盖

- 页面与交互：Task 7、Task 8 覆盖
- 数据模型：Task 1、Task 2 覆盖
- 执行链路：Task 4、Task 5、Task 6 覆盖
- 错误处理与运行摘要：Task 6、Task 8 覆盖

### 占位检查

- 没有使用 `TODO/TBD`
- 所有任务都指定了文件和命令
- 代码片段中的类型与字段名已与设计稿保持一致

### 需要特别注意

- 第一版仅做 Google 源
- 第一版只支持增量追加
- 第一版优先日常表单，不把复杂 JSON 编辑作为主入口

