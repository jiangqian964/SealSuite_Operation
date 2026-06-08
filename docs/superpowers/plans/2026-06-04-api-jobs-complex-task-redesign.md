# API 工具箱 / Jobs / 复杂任务 重构 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 把 API 工具箱重构为“任务定义工作台”，把 Jobs 重构为“调度中心”，并新增“复杂任务”页面承载工作流编排与未来 Agent 增强能力。

**Architecture:** 第一阶段先引入新的核心对象 `TaskDraft` 与 `JobSchedule`，把“任务内容定义”和“时间计划绑定”彻底分离；API 工具箱负责生成和测试草稿，Jobs 只负责选择草稿并配置开始时间、结束时间、cron/interval 与启停；复杂任务页面以工作流步骤编排为核心，支持 API 查询、数据处理、LLM 调用与输出。当前阶段不实现完全自主决策型 Agent，只为后续增强预留模型与页面结构。

**Tech Stack:** Go 1.21、chi、yaml/json 文件存储、go:embed、原生 HTML/CSS/JS、httptest

---

## File Map

**Create**
- `internal/storage/task_drafts_store.go`
- `internal/storage/task_drafts_store_test.go`
- `internal/storage/job_schedules_store.go`
- `internal/storage/job_schedules_store_test.go`
- `internal/storage/complex_tasks_store.go`
- `internal/storage/complex_tasks_store_test.go`
- `task-drafts.yaml`
- `job-schedules.yaml`
- `complex-tasks.yaml`

**Modify**
- `internal/runner/runner.go`
- `internal/web/server.go`
- `internal/web/server_test.go`
- `internal/web/assets/ui/index.html`
- `internal/web/assets/ui/styles.css`
- `internal/web/assets/ui/app.js`
- `docs/usage/template-job-mode-guide.md`

---

### Task 1: 新增 TaskDraft 存储与测试

**Files:**
- Create: `internal/storage/task_drafts_store_test.go`
- Create: `internal/storage/task_drafts_store.go`
- Create: `task-drafts.yaml`

- [ ] **Step 1: Write the failing test**

```go
package storage

import (
	"path/filepath"
	"testing"
)

func TestTaskDraftsStoreUpsertGetDelete(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "task-drafts.yaml")
	s := TaskDraftsStore{Path: p}

	in := TaskDraft{
		ID:               "draft_users_sync",
		Name:             "用户同步草稿",
		Mode:             "api_only",
		SourceTemplateID: "users_list",
		InputConfig: map[string]interface{}{
			"query": map[string]interface{}{"page": "1"},
		},
	}
	if err := s.Upsert(in); err != nil {
		t.Fatalf("Upsert err=%v", err)
	}

	got, ok, err := s.Get("draft_users_sync")
	if err != nil || !ok || got.ID != "draft_users_sync" {
		t.Fatalf("unexpected get: got=%#v ok=%v err=%v", got, ok, err)
	}

	if err := s.Delete("draft_users_sync"); err != nil {
		t.Fatalf("Delete err=%v", err)
	}
	_, ok, err = s.Get("draft_users_sync")
	if err != nil || ok {
		t.Fatalf("expected deleted, ok=%v err=%v", ok, err)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run:
```bash
go test ./internal/storage -count=1
```
Expected: FAIL with `undefined: TaskDraftsStore` or similar missing symbol errors.

- [ ] **Step 3: Write minimal implementation**

`internal/storage/task_drafts_store.go`
```go
package storage

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

type TaskDraftsFile struct {
	Version int         `yaml:"version" json:"version"`
	Items   []TaskDraft `yaml:"items" json:"items"`
}

type TaskDraft struct {
	ID               string                 `yaml:"id" json:"id"`
	Name             string                 `yaml:"name" json:"name"`
	Mode             string                 `yaml:"mode" json:"mode"`
	SourceTemplateID string                 `yaml:"source_template_id,omitempty" json:"source_template_id,omitempty"`
	InputConfig      map[string]interface{} `yaml:"input_config,omitempty" json:"input_config,omitempty"`
	TransformConfig  map[string]interface{} `yaml:"transform_config,omitempty" json:"transform_config,omitempty"`
	LLMConfig        map[string]interface{} `yaml:"llm_config,omitempty" json:"llm_config,omitempty"`
	OutputConfig     map[string]interface{} `yaml:"output_config,omitempty" json:"output_config,omitempty"`
}

type TaskDraftsStore struct {
	Path string
}

func (s TaskDraftsStore) Load() (*TaskDraftsFile, error) {
	b, err := os.ReadFile(s.Path)
	if err != nil {
		if os.IsNotExist(err) {
			return &TaskDraftsFile{Version: 1, Items: []TaskDraft{}}, nil
		}
		return nil, fmt.Errorf("read task drafts file: %w", err)
	}
	var tf TaskDraftsFile
	if err := yaml.Unmarshal(b, &tf); err != nil {
		return nil, fmt.Errorf("unmarshal task drafts yaml: %w", err)
	}
	if tf.Version == 0 {
		tf.Version = 1
	}
	if tf.Items == nil {
		tf.Items = []TaskDraft{}
	}
	return &tf, nil
}

func (s TaskDraftsStore) Save(tf *TaskDraftsFile) error {
	if tf == nil {
		return fmt.Errorf("task drafts file is nil")
	}
	if tf.Version == 0 {
		tf.Version = 1
	}
	b, err := yaml.Marshal(tf)
	if err != nil {
		return fmt.Errorf("marshal task drafts yaml: %w", err)
	}
	return AtomicWriteFile(s.Path, b, 0o644)
}

func (s TaskDraftsStore) Get(id string) (*TaskDraft, bool, error) {
	tf, err := s.Load()
	if err != nil {
		return nil, false, err
	}
	for _, item := range tf.Items {
		if item.ID == id {
			cp := item
			return &cp, true, nil
		}
	}
	return nil, false, nil
}

func (s TaskDraftsStore) Upsert(draft TaskDraft) error {
	tf, err := s.Load()
	if err != nil {
		return err
	}
	replaced := false
	for i := range tf.Items {
		if tf.Items[i].ID == draft.ID {
			tf.Items[i] = draft
			replaced = true
			break
		}
	}
	if !replaced {
		tf.Items = append(tf.Items, draft)
	}
	return s.Save(tf)
}

func (s TaskDraftsStore) Delete(id string) error {
	tf, err := s.Load()
	if err != nil {
		return err
	}
	out := make([]TaskDraft, 0, len(tf.Items))
	for _, item := range tf.Items {
		if item.ID != id {
			out = append(out, item)
		}
	}
	tf.Items = out
	return s.Save(tf)
}
```

`task-drafts.yaml`
```yaml
version: 1
items: []
```

- [ ] **Step 4: Run test to verify it passes**

Run:
```bash
go test ./internal/storage -count=1
```
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/storage/task_drafts_store.go internal/storage/task_drafts_store_test.go task-drafts.yaml
git commit -m "feat: add task draft store"
```

---

### Task 2: 新增 JobSchedule 存储与测试

**Files:**
- Create: `internal/storage/job_schedules_store_test.go`
- Create: `internal/storage/job_schedules_store.go`
- Create: `job-schedules.yaml`

- [ ] **Step 1: Write the failing test**

```go
package storage

import (
	"path/filepath"
	"testing"
)

func TestJobSchedulesStoreUpsertGetDelete(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "job-schedules.yaml")
	s := JobSchedulesStore{Path: p}

	in := JobSchedule{
		ID:           "sched_users_daily",
		DraftID:      "draft_users_sync",
		Enabled:      true,
		ScheduleType: "cron",
		Cron:         "0 0 9 * * *",
		StartAt:      "2026-06-05T09:00:00+08:00",
		EndAt:        "",
		Timezone:     "Asia/Shanghai",
	}
	if err := s.Upsert(in); err != nil {
		t.Fatalf("Upsert err=%v", err)
	}

	got, ok, err := s.Get("sched_users_daily")
	if err != nil || !ok || got.DraftID != "draft_users_sync" {
		t.Fatalf("unexpected get: got=%#v ok=%v err=%v", got, ok, err)
	}

	if err := s.Delete("sched_users_daily"); err != nil {
		t.Fatalf("Delete err=%v", err)
	}
	_, ok, err = s.Get("sched_users_daily")
	if err != nil || ok {
		t.Fatalf("expected deleted, ok=%v err=%v", ok, err)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run:
```bash
go test ./internal/storage -count=1
```
Expected: FAIL with `undefined: JobSchedulesStore` or similar.

- [ ] **Step 3: Write minimal implementation**

Create `internal/storage/job_schedules_store.go` mirroring the task draft store pattern:
```go
package storage

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

type JobSchedulesFile struct {
	Version int           `yaml:"version" json:"version"`
	Items   []JobSchedule `yaml:"items" json:"items"`
}

type JobSchedule struct {
	ID           string `yaml:"id" json:"id"`
	DraftID      string `yaml:"draft_id" json:"draft_id"`
	Enabled      bool   `yaml:"enabled" json:"enabled"`
	StartAt      string `yaml:"start_at,omitempty" json:"start_at,omitempty"`
	EndAt        string `yaml:"end_at,omitempty" json:"end_at,omitempty"`
	ScheduleType string `yaml:"schedule_type" json:"schedule_type"`
	Cron         string `yaml:"cron,omitempty" json:"cron,omitempty"`
	Interval     string `yaml:"interval,omitempty" json:"interval,omitempty"`
	Timezone     string `yaml:"timezone,omitempty" json:"timezone,omitempty"`
}

type JobSchedulesStore struct {
	Path string
}

// Load / Save / Get / Upsert / Delete with same structure as TaskDraftsStore.
```

`job-schedules.yaml`
```yaml
version: 1
items: []
```

- [ ] **Step 4: Run test to verify it passes**

Run:
```bash
go test ./internal/storage -count=1
```
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/storage/job_schedules_store.go internal/storage/job_schedules_store_test.go job-schedules.yaml
git commit -m "feat: add job schedule store"
```

---

### Task 3: 新增 ComplexTask 存储与测试

**Files:**
- Create: `internal/storage/complex_tasks_store_test.go`
- Create: `internal/storage/complex_tasks_store.go`
- Create: `complex-tasks.yaml`

- [ ] **Step 1: Write the failing test**

```go
package storage

import (
	"path/filepath"
	"testing"
)

func TestComplexTasksStoreUpsertGetDelete(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "complex-tasks.yaml")
	s := ComplexTasksStore{Path: p}

	in := ComplexTask{
		ID:            "weekly-security-summary",
		Name:          "每周安全汇总",
		Goal:          "汇总安全异常并生成中文总结",
		ExecutionMode: "manual",
		Steps: []ComplexTaskStep{
			{ID: "s1", Type: "api_call", Name: "查询设备"},
			{ID: "s2", Type: "llm_inference", Name: "生成总结"},
		},
	}
	if err := s.Upsert(in); err != nil {
		t.Fatalf("Upsert err=%v", err)
	}

	got, ok, err := s.Get("weekly-security-summary")
	if err != nil || !ok || len(got.Steps) != 2 {
		t.Fatalf("unexpected get: got=%#v ok=%v err=%v", got, ok, err)
	}

	if err := s.Delete("weekly-security-summary"); err != nil {
		t.Fatalf("Delete err=%v", err)
	}
	_, ok, err = s.Get("weekly-security-summary")
	if err != nil || ok {
		t.Fatalf("expected deleted, ok=%v err=%v", ok, err)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run:
```bash
go test ./internal/storage -count=1
```
Expected: FAIL with `undefined: ComplexTasksStore` or similar.

- [ ] **Step 3: Write minimal implementation**

`internal/storage/complex_tasks_store.go`
```go
package storage

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

type ComplexTasksFile struct {
	Version int           `yaml:"version" json:"version"`
	Items   []ComplexTask `yaml:"items" json:"items"`
}

type ComplexTask struct {
	ID            string            `yaml:"id" json:"id"`
	Name          string            `yaml:"name" json:"name"`
	Goal          string            `yaml:"goal,omitempty" json:"goal,omitempty"`
	ExecutionMode string            `yaml:"execution_mode" json:"execution_mode"`
	Steps         []ComplexTaskStep `yaml:"steps" json:"steps"`
}

type ComplexTaskStep struct {
	ID     string                 `yaml:"id" json:"id"`
	Type   string                 `yaml:"type" json:"type"`
	Name   string                 `yaml:"name" json:"name"`
	Config map[string]interface{} `yaml:"config,omitempty" json:"config,omitempty"`
}

type ComplexTasksStore struct {
	Path string
}

// Load / Save / Get / Upsert / Delete with the same pattern as TaskDraftsStore.
```

`complex-tasks.yaml`
```yaml
version: 1
items: []
```

- [ ] **Step 4: Run test to verify it passes**

Run:
```bash
go test ./internal/storage -count=1
```
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/storage/complex_tasks_store.go internal/storage/complex_tasks_store_test.go complex-tasks.yaml
git commit -m "feat: add complex task store"
```

---

### Task 4: 后端 API 增加 TaskDraft 资源接口

**Files:**
- Modify: `internal/web/server.go`
- Modify: `internal/web/server_test.go`

- [ ] **Step 1: Write the failing test**

Append to `internal/web/server_test.go`:
```go
func TestTaskDraftCRUDEndpoints(t *testing.T) {
	cfg := &config.Config{
		Server:    config.ServerConfig{Bind: "127.0.0.1", Port: 0, Mode: "debug"},
		Scheduler: config.SchedulerConfig{Enabled: false, Timezone: "Asia/Shanghai"},
		SealSuite: config.SealSuiteConfig{BaseURL: "http://example.com", AccessKey: "ak", SecretKey: "sk", Timeout: 1},
		Log:       config.LogConfig{Level: "debug", Filename: "./logs/app.log"},
	}
	client := sealsuite.NewClient(&cfg.SealSuite)
	r := runner.New(cfg, client, "jobs.yaml", "api-templates.yaml")
	h, err := NewRouter(cfg, r)
	if err != nil { t.Fatal(err) }

	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/task-drafts", strings.NewReader(`{"id":"draft_users_sync","name":"用户同步草稿","mode":"api_only","source_template_id":"users_list"}`))
	req.Header.Set("Content-Type", "application/json")
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("create got %d body=%s", rr.Code, rr.Body.String())
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run:
```bash
go test ./internal/web -count=1
```
Expected: FAIL because `/api/v1/task-drafts` endpoints do not exist.

- [ ] **Step 3: Write minimal implementation**

In `internal/web/server.go`, initialize a store:
```go
taskDraftsStore := storage.TaskDraftsStore{Path: "task-drafts.yaml"}
```

Add endpoints:
- `GET /api/v1/task-drafts`
- `GET /api/v1/task-drafts/{id}`
- `POST /api/v1/task-drafts`
- `PUT /api/v1/task-drafts/{id}`
- `DELETE /api/v1/task-drafts/{id}`

Minimal handlers:
```go
apiR.Get("/task-drafts", func(w http.ResponseWriter, req *http.Request) {
	tf, err := taskDraftsStore.Load()
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]interface{}{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, tf)
})
```

- [ ] **Step 4: Run test to verify it passes**

Run:
```bash
go test ./internal/web -count=1
```
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/web/server.go internal/web/server_test.go
git commit -m "feat: add task draft CRUD endpoints"
```

---

### Task 5: 后端 API 增加 JobSchedule 资源接口

**Files:**
- Modify: `internal/web/server.go`
- Modify: `internal/web/server_test.go`

- [ ] **Step 1: Write the failing test**

Append to `internal/web/server_test.go`:
```go
func TestJobScheduleCRUDEndpoints(t *testing.T) {
	cfg := &config.Config{
		Server:    config.ServerConfig{Bind: "127.0.0.1", Port: 0, Mode: "debug"},
		Scheduler: config.SchedulerConfig{Enabled: false, Timezone: "Asia/Shanghai"},
		SealSuite: config.SealSuiteConfig{BaseURL: "http://example.com", AccessKey: "ak", SecretKey: "sk", Timeout: 1},
		Log:       config.LogConfig{Level: "debug", Filename: "./logs/app.log"},
	}
	client := sealsuite.NewClient(&cfg.SealSuite)
	r := runner.New(cfg, client, "jobs.yaml", "api-templates.yaml")
	h, err := NewRouter(cfg, r)
	if err != nil { t.Fatal(err) }

	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/job-schedules", strings.NewReader(`{"id":"sched_users_daily","draft_id":"draft_users_sync","enabled":true,"schedule_type":"cron","cron":"0 0 9 * * *","timezone":"Asia/Shanghai"}`))
	req.Header.Set("Content-Type", "application/json")
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("create got %d body=%s", rr.Code, rr.Body.String())
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run:
```bash
go test ./internal/web -count=1
```
Expected: FAIL because `/api/v1/job-schedules` endpoints do not exist.

- [ ] **Step 3: Write minimal implementation**

In `internal/web/server.go`, initialize:
```go
jobSchedulesStore := storage.JobSchedulesStore{Path: "job-schedules.yaml"}
```

Add endpoints:
- `GET /api/v1/job-schedules`
- `GET /api/v1/job-schedules/{id}`
- `POST /api/v1/job-schedules`
- `PUT /api/v1/job-schedules/{id}`
- `DELETE /api/v1/job-schedules/{id}`

Guard create/update so `draft_id` must point to an existing `TaskDraft`:
```go
if _, ok, err := taskDraftsStore.Get(in.DraftID); err != nil {
  writeJSON(w, http.StatusInternalServerError, map[string]interface{}{"error": err.Error()})
  return
} else if !ok {
  writeJSON(w, http.StatusBadRequest, map[string]interface{}{"error": "draft not found"})
  return
}
```

- [ ] **Step 4: Run test to verify it passes**

Run:
```bash
go test ./internal/web -count=1
```
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/web/server.go internal/web/server_test.go
git commit -m "feat: add job schedule CRUD endpoints"
```

---

### Task 6: 后端 API 增加 ComplexTask 资源接口

**Files:**
- Modify: `internal/web/server.go`
- Modify: `internal/web/server_test.go`

- [ ] **Step 1: Write the failing test**

Append to `internal/web/server_test.go`:
```go
func TestComplexTaskCRUDEndpoints(t *testing.T) {
	cfg := &config.Config{
		Server:    config.ServerConfig{Bind: "127.0.0.1", Port: 0, Mode: "debug"},
		Scheduler: config.SchedulerConfig{Enabled: false, Timezone: "Asia/Shanghai"},
		SealSuite: config.SealSuiteConfig{BaseURL: "http://example.com", AccessKey: "ak", SecretKey: "sk", Timeout: 1},
		Log:       config.LogConfig{Level: "debug", Filename: "./logs/app.log"},
	}
	client := sealsuite.NewClient(&cfg.SealSuite)
	r := runner.New(cfg, client, "jobs.yaml", "api-templates.yaml")
	h, err := NewRouter(cfg, r)
	if err != nil { t.Fatal(err) }

	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/complex-tasks", strings.NewReader(`{"id":"weekly-security-summary","name":"每周安全汇总","execution_mode":"manual","steps":[{"id":"s1","type":"api_call","name":"查询设备"}]}`))
	req.Header.Set("Content-Type", "application/json")
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("create got %d body=%s", rr.Code, rr.Body.String())
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run:
```bash
go test ./internal/web -count=1
```
Expected: FAIL because `/api/v1/complex-tasks` endpoints do not exist.

- [ ] **Step 3: Write minimal implementation**

In `internal/web/server.go`, initialize:
```go
complexTasksStore := storage.ComplexTasksStore{Path: "complex-tasks.yaml"}
```

Add endpoints:
- `GET /api/v1/complex-tasks`
- `GET /api/v1/complex-tasks/{id}`
- `POST /api/v1/complex-tasks`
- `PUT /api/v1/complex-tasks/{id}`
- `DELETE /api/v1/complex-tasks/{id}`

Keep v1 validation intentionally light:
- `id` required
- `name` required
- `steps` may be empty for draft editing, but preferred non-empty

- [ ] **Step 4: Run test to verify it passes**

Run:
```bash
go test ./internal/web -count=1
```
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/web/server.go internal/web/server_test.go
git commit -m "feat: add complex task CRUD endpoints"
```

---

### Task 7: API 工具箱重构为任务定义工作台

**Files:**
- Modify: `internal/web/assets/ui/index.html`
- Modify: `internal/web/assets/ui/styles.css`
- Modify: `internal/web/assets/ui/app.js`

- [ ] **Step 1: Write the failing test**

In `internal/web/server_test.go`, extend `TestIndexServed` assertions:
```go
if !strings.Contains(rr.Body.String(), "保存为任务草稿") {
	t.Fatalf("expected save as task draft action in index")
}
if !strings.Contains(rr.Body.String(), "处理逻辑") {
	t.Fatalf("expected transform section in api workbench")
}
if !strings.Contains(rr.Body.String(), "大模型调用") {
	t.Fatalf("expected llm section in api workbench")
}
```

- [ ] **Step 2: Run test to verify it fails**

Run:
```bash
go test ./internal/web -count=1
```
Expected: FAIL because the current API tool page does not expose those sections.

- [ ] **Step 3: Write minimal implementation**

In `index.html`, expand `view-api`:
- Keep template list on the left
- Replace the right-side generic debugger with a “任务定义工作台” card
- Add sections:
  - `输入参数`
  - `处理逻辑`
  - `大模型调用`
  - `输出格式`
  - `保存为任务草稿`

Minimal structure:
```html
<div class="card">
  <div style="font-weight:750">任务定义工作台</div>
  <div class="subtitle">定义 API 调用、处理逻辑、LLM 调用与输出，并可保存为任务草稿。</div>
  <div class="field"><label>草稿 ID</label><input id="draft-id" /></div>
  <div class="field"><label>草稿名称</label><input id="draft-name" /></div>
  <div class="field"><label>输入参数</label><textarea id="draft-input" rows="6"></textarea></div>
  <div class="field"><label>处理逻辑</label><textarea id="draft-transform" rows="6"></textarea></div>
  <div class="field"><label>大模型调用</label><textarea id="draft-llm" rows="6"></textarea></div>
  <div class="field"><label>输出格式</label><textarea id="draft-output" rows="6"></textarea></div>
  <div class="row">
    <button class="btn" id="btn-draft-test">单次测试</button>
    <button class="btn primary" id="btn-draft-save">保存为任务草稿</button>
  </div>
</div>
```

In `app.js`, wire:
- `btn-draft-save` -> `POST /api/v1/task-drafts`
- `btn-draft-test` -> reuse existing execute path for the API part; non-API parts can be stubbed as “仅结构保存” in v1

- [ ] **Step 4: Run manual verification**

Run:
```bash
go run ./cmd/main.go
```
Manual Expected:
- API 工具箱右侧变成任务定义工作台
- 可输入草稿信息并保存成功

- [ ] **Step 5: Commit**

```bash
git add internal/web/assets/ui/index.html internal/web/assets/ui/styles.css internal/web/assets/ui/app.js internal/web/server_test.go
git commit -m "feat: redesign api workbench around task drafts"
```

---

### Task 8: Jobs 重构为纯调度中心

**Files:**
- Modify: `internal/web/assets/ui/index.html`
- Modify: `internal/web/assets/ui/styles.css`
- Modify: `internal/web/assets/ui/app.js`

- [ ] **Step 1: Write the failing test**

Add index assertions:
```go
if !strings.Contains(rr.Body.String(), "开始时间") {
	t.Fatalf("expected start time field in jobs")
}
if !strings.Contains(rr.Body.String(), "结束时间") {
	t.Fatalf("expected end time field in jobs")
}
if !strings.Contains(rr.Body.String(), "执行间隔") {
	t.Fatalf("expected interval field in jobs")
}
```

- [ ] **Step 2: Run test to verify it fails**

Run:
```bash
go test ./internal/web -count=1
```
Expected: FAIL because Jobs page currently focuses on old `jobs.yaml` task objects.

- [ ] **Step 3: Write minimal implementation**

In `index.html`, replace Jobs “新建任务/高级编辑任务” mental model with schedule editing:
- list now shows schedules
- detail panel shows:
  - selected `draft_id`
  - `start_at`
  - `end_at`
  - `schedule_type`
  - `cron`
  - `interval`
  - `enabled`
  - latest run result

In `app.js`, add:
- `refreshJobSchedules()`
- `loadJobScheduleDetail(id)`
- `openJobScheduleEditor(schedule, {isCreate})`

Jobs create action should:
- fetch task drafts first
- let user select `draft_id`
- then fill schedule-only fields

- [ ] **Step 4: Run manual verification**

Run:
```bash
go run ./cmd/main.go
```
Manual Expected:
- Jobs 页面不再要求填写 API 细节
- 新建时只围绕“绑定草稿 + 时间计划”展开

- [ ] **Step 5: Commit**

```bash
git add internal/web/assets/ui/index.html internal/web/assets/ui/styles.css internal/web/assets/ui/app.js internal/web/server_test.go
git commit -m "feat: redesign jobs as scheduling center"
```

---

### Task 9: 新增复杂任务页面（工作流编排版）

**Files:**
- Modify: `internal/web/assets/ui/index.html`
- Modify: `internal/web/assets/ui/styles.css`
- Modify: `internal/web/assets/ui/app.js`

- [ ] **Step 1: Write the failing test**

Add index assertions:
```go
if !strings.Contains(rr.Body.String(), "复杂任务") {
	t.Fatalf("expected complex task navigation and page")
}
if !strings.Contains(rr.Body.String(), "步骤编排") {
	t.Fatalf("expected workflow composition area")
}
if !strings.Contains(rr.Body.String(), "API 查询") {
	t.Fatalf("expected api step type")
}
```

- [ ] **Step 2: Run test to verify it fails**

Run:
```bash
go test ./internal/web -count=1
```
Expected: FAIL because there is no complex task page yet.

- [ ] **Step 3: Write minimal implementation**

In `index.html`:
- add sidebar nav item `复杂任务`
- add `view-complex-tasks`
- page layout:
  - left: task list
  - center: steps
  - right: selected step config
  - bottom/right card: run result

Minimal structure:
```html
<section class="view hidden" id="view-complex-tasks">
  <div class="page-title">
    <div>
      <h2>复杂任务</h2>
      <div class="subtitle">编排飞连 API、数据处理、LLM 调用与输出步骤。</div>
    </div>
  </div>
  <div class="grid" style="grid-template-columns: .9fr 1.2fr .9fr;">
    <div class="card"><div style="font-weight:750">任务列表</div><div id="complex-task-list"></div></div>
    <div class="card"><div style="font-weight:750">步骤编排</div><div id="complex-task-steps"></div></div>
    <div class="card"><div style="font-weight:750">步骤配置</div><div id="complex-task-config"></div></div>
  </div>
</section>
```

In `app.js`:
- `refreshComplexTasks()`
- `renderComplexTaskList()`
- `openComplexTaskEditor(task, {isCreate})`
- allow adding steps of type:
  - `api_call`
  - `data_transform`
  - `llm_inference`
  - `output`

For v1, “手动运行” can simply show the structured payload in the result pane; actual execution engine can come later.

- [ ] **Step 4: Run manual verification**

Run:
```bash
go run ./cmd/main.go
```
Manual Expected:
- Sidebar shows `复杂任务`
- Can create a multi-step task and save it
- Can view/edit the step list

- [ ] **Step 5: Commit**

```bash
git add internal/web/assets/ui/index.html internal/web/assets/ui/styles.css internal/web/assets/ui/app.js
git commit -m "feat: add complex task workflow page"
```

---

### Task 10: 文案与现有使用说明同步

**Files:**
- Modify: `docs/usage/template-job-mode-guide.md`
- Modify: `internal/web/assets/ui/index.html`

- [ ] **Step 1: Update the guide**

Adjust the existing guide so it reflects the new product model:
- API 工具箱 = 定义任务草稿
- Jobs = 绑定时间计划
- 复杂任务 = 编排多步骤与智能处理

Add a top-level “产品分工” section:
```md
## 产品分工
- API 工具箱：定义任务内容
- Jobs：定义执行时间
- 复杂任务：定义多步骤与智能化能力
```

- [ ] **Step 2: Update page subtitles**

In `index.html`, update subtitles:
- API 工具箱：`定义任务内容、测试链路、保存为任务草稿`
- Jobs：`绑定草稿并配置开始时间、结束时间、周期或间隔`
- 复杂任务：`编排 API、数据处理、LLM 与输出`

- [ ] **Step 3: Commit**

```bash
git add docs/usage/template-job-mode-guide.md internal/web/assets/ui/index.html
git commit -m "docs: align product copy with redesigned module roles"
```

---

## Self-Review

- Spec coverage:
  - API 工具箱变任务定义工作台：Task 4, Task 7, Task 10
  - Jobs 变调度中心：Task 2, Task 5, Task 8
  - 复杂任务页：Task 3, Task 6, Task 9
  - 工作流/Agent 分层：Task 3 and Task 9 establish workflow structure; Agent remains explicitly deferred
- Placeholder scan:
  - No TBD/TODO text
  - Each task includes files, tests, commands, and minimum implementation shapes
- Type consistency:
  - `TaskDraft`, `JobSchedule`, `ComplexTask` names are used consistently across storage and API layers
  - Endpoints align with storage resources: `/task-drafts`, `/job-schedules`, `/complex-tasks`

---

## Execution Handoff

Plan complete and saved to `docs/superpowers/plans/2026-06-04-api-jobs-complex-task-redesign.md`. Two execution options:

1. **Subagent-Driven (recommended)** - I dispatch a fresh subagent per task, review between tasks, fast iteration
2. **Inline Execution** - Execute tasks in this session using executing-plans, batch execution with checkpoints

Which approach?

