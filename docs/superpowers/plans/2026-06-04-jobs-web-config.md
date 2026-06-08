# Jobs Web 配置台 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 将现有 Jobs 模块从“只读列表 + 启停/运行”升级为完整的 Web 配置台，支持列表、详情、快速编辑抽屉、高级编辑页，以及最近 1 次运行记录落盘。

**Architecture:** 后端继续以 `jobs.yaml` 作为任务定义源，并新增 `job-runs.json` 保存每个任务最近 1 次运行结果。前端保留原生 HTML/CSS/JS + go:embed 的模式，在 Jobs 区域增加列表页、详情视图、抽屉与高级编辑页面状态；后端补齐 `GET/POST/PUT/DELETE /api/v1/jobs*` 与运行记录读写。

**Tech Stack:** Go 1.21、chi、yaml.v3、JSON 文件存储、go:embed、原生 JavaScript、httptest

---

## File Map

**Create**
- `internal/storage/job_runs_store.go`
- `internal/storage/job_runs_store_test.go`
- `job-runs.json`

**Modify**
- `internal/runner/runner.go`
- `internal/storage/jobs_store.go`
- `internal/web/server.go`
- `internal/web/server_test.go`
- `internal/web/assets/ui/index.html`
- `internal/web/assets/ui/styles.css`
- `internal/web/assets/ui/app.js`

---

### Task 1: 任务运行记录落盘存储

**Files:**
- Create: `internal/storage/job_runs_store_test.go`
- Create: `internal/storage/job_runs_store.go`
- Create: `job-runs.json`

- [ ] **Step 1: Write the failing test**

```go
package storage

import (
	"path/filepath"
	"testing"
)

func TestJobRunsStoreSaveAndLoad(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "job-runs.json")
	s := JobRunsStore{Path: p}

	in := &JobRunsFile{
		Version: 1,
		Items: map[string]JobRunRecord{
			"users-sync": {
				LastRun:    "2026-06-04T19:30:00+08:00",
				OK:         true,
				DurationMs: 321,
				Error:      "",
			},
		},
	}

	if err := s.Save(in); err != nil {
		t.Fatalf("Save err=%v", err)
	}
	got, err := s.Load()
	if err != nil {
		t.Fatalf("Load err=%v", err)
	}
	if got.Items["users-sync"].DurationMs != 321 {
		t.Fatalf("unexpected record: %#v", got.Items["users-sync"])
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run:
```bash
go test ./... -count=1
```
Expected: FAIL with `undefined: JobRunsStore` or similar missing symbol errors.

- [ ] **Step 3: Write minimal implementation**

`internal/storage/job_runs_store.go`
```go
package storage

import (
	"encoding/json"
	"fmt"
	"os"
)

type JobRunsFile struct {
	Version int                     `json:"version"`
	Items    map[string]JobRunRecord `json:"items"`
}

type JobRunRecord struct {
	LastRun    string `json:"last_run"`
	OK         bool   `json:"ok"`
	DurationMs int64  `json:"duration_ms"`
	Error      string `json:"error"`
}

type JobRunsStore struct {
	Path string
}

func (s JobRunsStore) Load() (*JobRunsFile, error) {
	b, err := os.ReadFile(s.Path)
	if err != nil {
		if os.IsNotExist(err) {
			return &JobRunsFile{Version: 1, Items: map[string]JobRunRecord{}}, nil
		}
		return nil, fmt.Errorf("read job-runs file: %w", err)
	}
	var jf JobRunsFile
	if err := json.Unmarshal(b, &jf); err != nil {
		return nil, fmt.Errorf("unmarshal job-runs json: %w", err)
	}
	if jf.Version == 0 {
		jf.Version = 1
	}
	if jf.Items == nil {
		jf.Items = map[string]JobRunRecord{}
	}
	return &jf, nil
}

func (s JobRunsStore) Save(jf *JobRunsFile) error {
	if jf == nil {
		return fmt.Errorf("job runs file is nil")
	}
	if jf.Version == 0 {
		jf.Version = 1
	}
	if jf.Items == nil {
		jf.Items = map[string]JobRunRecord{}
	}
	b, err := json.MarshalIndent(jf, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal job-runs json: %w", err)
	}
	return AtomicWriteFile(s.Path, b, 0o644)
}

func (s JobRunsStore) Put(name string, rec JobRunRecord) error {
	jf, err := s.Load()
	if err != nil {
		return err
	}
	jf.Items[name] = rec
	return s.Save(jf)
}
```

`job-runs.json`
```json
{
  "version": 1,
  "items": {}
}
```

- [ ] **Step 4: Run test to verify it passes**

Run:
```bash
go test ./internal/storage -count=1
```
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/storage/job_runs_store.go internal/storage/job_runs_store_test.go job-runs.json
git commit -m "feat: add persisted job run store"
```

---

### Task 2: Runner 写入最近一次运行记录

**Files:**
- Modify: `internal/runner/runner.go`
- Test: `internal/storage/job_runs_store_test.go`

- [ ] **Step 1: Write the failing test**

在 `internal/storage/job_runs_store_test.go` 追加：
```go
func TestJobRunsStorePutOverwritesLatestRecord(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "job-runs.json")
	s := JobRunsStore{Path: p}

	if err := s.Put("users-sync", JobRunRecord{LastRun: "t1", OK: false, DurationMs: 1, Error: "e1"}); err != nil {
		t.Fatal(err)
	}
	if err := s.Put("users-sync", JobRunRecord{LastRun: "t2", OK: true, DurationMs: 2, Error: ""}); err != nil {
		t.Fatal(err)
	}
	got, err := s.Load()
	if err != nil {
		t.Fatal(err)
	}
	if got.Items["users-sync"].LastRun != "t2" || !got.Items["users-sync"].OK {
		t.Fatalf("expected latest record overwritten, got=%#v", got.Items["users-sync"])
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run:
```bash
go test ./internal/storage -count=1
```
Expected: FAIL if `Put` is missing or behaves incorrectly.

- [ ] **Step 3: Write minimal implementation**

在 `internal/runner/runner.go`：
```go
type Runner struct {
  // ...
  runStore storage.JobRunsStore
}

func New(cfg *config.Config, client *sealsuite.Client, jobsPath, templatesPath string) *Runner {
  return &Runner{
    // ...
    runStore: storage.JobRunsStore{Path: "job-runs.json"},
  }
}
```

在 `runJob()` 更新状态后追加：
```go
_ = r.runStore.Put(j.Name, storage.JobRunRecord{
  LastRun:    st.LastRun.Format(time.RFC3339),
  OK:         st.LastOK,
  DurationMs: st.DurationMs,
  Error:      st.LastError,
})
```

在 `LoadSnapshot()` 里读取 `job-runs.json` 并把已有落盘结果合并回 `smap`，确保服务重启后还能显示最近一次结果：
```go
rf, err := r.runStore.Load()
if err == nil {
  for name, rec := range rf.Items {
    st := smap[name]
    if st == nil {
      st = &JobStatus{}
      smap[name] = st
    }
    if rec.LastRun != "" {
      if ts, e := time.Parse(time.RFC3339, rec.LastRun); e == nil {
        st.LastRun = ts
      }
    }
    st.LastOK = rec.OK
    st.DurationMs = rec.DurationMs
    st.LastError = rec.Error
  }
}
```

- [ ] **Step 4: Run test to verify it passes**

Run:
```bash
go test ./internal/storage -count=1
```
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/runner/runner.go internal/storage/job_runs_store_test.go
git commit -m "feat: persist latest job run result"
```

---

### Task 3: JobsStore 增加按名查询、 upsert 和删除能力

**Files:**
- Modify: `internal/storage/jobs_store.go`
- Test: `internal/storage/jobs_store_test.go`

- [ ] **Step 1: Write the failing test**

在 `internal/storage/jobs_store_test.go` 追加：
```go
func TestJobsStoreUpsertGetDelete(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "jobs.yaml")
	s := JobsStore{Path: p}

	if err := s.Upsert(Job{Name: "a", Enabled: true, Cron: "0 */1 * * * *", Type: "api_poll", Params: map[string]interface{}{"template_id": "users_list"}}); err != nil {
		t.Fatal(err)
	}
	got, ok, err := s.Get("a")
	if err != nil || !ok || got.Name != "a" {
		t.Fatalf("unexpected get: job=%#v ok=%v err=%v", got, ok, err)
	}
	if err := s.Delete("a"); err != nil {
		t.Fatal(err)
	}
	_, ok, err = s.Get("a")
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
Expected: FAIL with `JobsStore.Upsert undefined` or similar.

- [ ] **Step 3: Write minimal implementation**

在 `internal/storage/jobs_store.go` 增加：
```go
func (s JobsStore) Get(name string) (*Job, bool, error) {
  jf, err := s.Load()
  if err != nil {
    return nil, false, err
  }
  for _, j := range jf.Jobs {
    if j.Name == name {
      cp := j
      return &cp, true, nil
    }
  }
  return nil, false, nil
}

func (s JobsStore) Upsert(job Job) error {
  jf, err := s.Load()
  if err != nil {
    return err
  }
  replaced := false
  for i := range jf.Jobs {
    if jf.Jobs[i].Name == job.Name {
      jf.Jobs[i] = job
      replaced = true
      break
    }
  }
  if !replaced {
    jf.Jobs = append(jf.Jobs, job)
  }
  return s.Save(jf)
}

func (s JobsStore) Delete(name string) error {
  jf, err := s.Load()
  if err != nil {
    return err
  }
  out := make([]Job, 0, len(jf.Jobs))
  for _, j := range jf.Jobs {
    if j.Name != name {
      out = append(out, j)
    }
  }
  jf.Jobs = out
  return s.Save(jf)
}
```

- [ ] **Step 4: Run test to verify it passes**

Run:
```bash
go test ./internal/storage -count=1
```
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/storage/jobs_store.go internal/storage/jobs_store_test.go
git commit -m "feat: add jobs store query and mutation helpers"
```

---

### Task 4: 后端 Jobs API 补齐列表/详情/新建/更新/删除

**Files:**
- Modify: `internal/web/server.go`
- Modify: `internal/web/server_test.go`

- [ ] **Step 1: Write the failing test**

在 `internal/web/server_test.go` 追加一个 API 契约测试：
```go
func TestJobsCRUDEndpoints(t *testing.T) {
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

  // POST create
  rr := httptest.NewRecorder()
  req := httptest.NewRequest(http.MethodPost, "/api/v1/jobs", strings.NewReader(`{"name":"job-a","enabled":true,"cron":"0 */1 * * * *","type":"api_poll","params":{"template_id":"users_list"}}`))
  req.Header.Set("Content-Type", "application/json")
  h.ServeHTTP(rr, req)
  if rr.Code != http.StatusOK { t.Fatalf("create got %d body=%s", rr.Code, rr.Body.String()) }
}
```

- [ ] **Step 2: Run test to verify it fails**

Run:
```bash
go test ./internal/web -count=1
```
Expected: FAIL because `POST /api/v1/jobs` or `GET /api/v1/jobs/{name}` / `DELETE` are missing.

- [ ] **Step 3: Write minimal implementation**

在 `internal/web/server.go` 增加：
- `GET /api/v1/jobs/{name}`：返回单任务 + `job_status[name]`
- `POST /api/v1/jobs`：新建任务（JSON body 直接对应 `storage.Job`，保存后 reload）
- `PUT /api/v1/jobs/{name}`：更新任务（path name 与 body name 不一致时返回 400）
- `DELETE /api/v1/jobs/{name}`：删除任务（删除后 reload）

示意代码：
```go
apiR.Get("/jobs/{name}", func(w http.ResponseWriter, req *http.Request) {
  name := chi.URLParam(req, "name")
  job, ok, err := jobsStore.Get(name)
  if err != nil {
    writeJSON(w, http.StatusInternalServerError, map[string]interface{}{"error": err.Error()})
    return
  }
  if !ok {
    writeJSON(w, http.StatusNotFound, map[string]interface{}{"error": "job not found"})
    return
  }
  _, _, smap, _ := r.LoadSnapshot()
  writeJSON(w, http.StatusOK, map[string]interface{}{
    "job": job,
    "run": smap[name],
  })
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
git commit -m "feat: add jobs CRUD endpoints"
```

---

### Task 5: Jobs 列表页升级为真正的管理页

**Files:**
- Modify: `internal/web/assets/ui/index.html`
- Modify: `internal/web/assets/ui/styles.css`
- Modify: `internal/web/assets/ui/app.js`

- [ ] **Step 1: Write the failing test**

用最小 DOM 断言思路（无需浏览器自动化，至少先定义结构要求）：
在 `internal/web/server_test.go` 的 `TestIndexServed` 之后追加断言，要求首页包含 Jobs 详情区容器与新建按钮的标识文本：
```go
if !strings.Contains(rr.Body.String(), "新建任务") {
  t.Fatalf("expected jobs create button in index")
}
if !strings.Contains(rr.Body.String(), "任务详情") {
  t.Fatalf("expected jobs detail section in index")
}
```

- [ ] **Step 2: Run test to verify it fails**

Run:
```bash
go test ./internal/web -count=1
```
Expected: FAIL because index 还没有这些文本/区域。

- [ ] **Step 3: Write minimal implementation**

在 `index.html` 的 Jobs 区块下新增：
- 顶部筛选区：搜索框、类型筛选、状态筛选、新建按钮
- 左侧列表表格
- 右侧详情面板（默认提示“选择一个任务查看详情”）
- 抽屉容器（隐藏）
- 高级编辑页容器（隐藏）

最小结构片段：
```html
<div class="grid jobs-layout">
  <div class="card">
    <div class="row">
      <input id="jobs-search" placeholder="搜索任务名…" />
      <select id="jobs-filter-type">
        <option value="">全部类型</option>
        <option value="api_poll">api_poll</option>
        <option value="api_write">api_write</option>
      </select>
      <select id="jobs-filter-enabled">
        <option value="">全部状态</option>
        <option value="enabled">启用</option>
        <option value="disabled">禁用</option>
      </select>
      <button class="btn primary" id="btn-job-create">新建任务</button>
    </div>
    <table class="table" id="jobs-table">...</table>
  </div>
  <div class="card" id="job-detail-card">
    <div style="font-weight:750">任务详情</div>
    <div class="subtitle" id="job-detail-empty">选择一个任务查看详情</div>
    <div id="job-detail"></div>
  </div>
</div>
```

- [ ] **Step 4: Run test to verify it passes**

Run:
```bash
go test ./internal/web -count=1
```
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/web/assets/ui/index.html internal/web/assets/ui/styles.css internal/web/assets/ui/app.js internal/web/server_test.go
git commit -m "feat: add jobs list and detail layout"
```

---

### Task 6: 前端列表交互、详情加载、快速操作

**Files:**
- Modify: `internal/web/assets/ui/app.js`

- [ ] **Step 1: Write the failing test**

在计划层面定义验收式最小行为：
- 点击任务行会调用 `GET /api/v1/jobs/{name}`
- 详情面板展示 name/type/cron/enabled/最近一次运行结果

由于当前项目没有前端自动化测试，先以可验证函数拆分的方式实现：
新增函数并让运行时报错作为 RED：
```js
function renderJobDetail(payload) {
  throw new Error('not implemented')
}
```

- [ ] **Step 2: Run app manually to verify it fails**

Run:
```bash
go run ./cmd/main.go
```
Manual Expected: 点击任务后前端控制台报 `not implemented`。

- [ ] **Step 3: Write minimal implementation**

在 `app.js` 中：
- `refreshJobs()`：缓存列表到 `window.__jobsCache`
- `renderJobsTable(jobs)`：点击 name/row 时调用 `loadJobDetail(j.name)`
- `loadJobDetail(name)`：`GET /api/v1/jobs/{name}`
- `renderJobDetail(payload)`：渲染基础信息、params 摘要、最近一次运行结果、操作按钮

详情区示意：
```js
function renderJobDetail(payload) {
  const job = payload.job || {};
  const run = payload.run || {};
  qs('#job-detail-empty').style.display = 'none';
  qs('#job-detail').innerHTML = `
    <div class="row"><div class="pill">${escapeHtml(job.type || '')}</div><div class="pill ${job.enabled ? 'ok' : ''}">${job.enabled ? 'ENABLED' : 'DISABLED'}</div></div>
    <div class="card"><div style="font-weight:750">${escapeHtml(job.name || '')}</div><div class="subtitle"><code>${escapeHtml(job.cron || '')}</code></div></div>
    <div class="card"><div style="font-weight:750">最近一次运行</div><pre class="pre">${pretty(run)}</pre></div>
    <div class="row">
      <button class="btn" id="btn-job-edit-quick">快速编辑</button>
      <button class="btn" id="btn-job-edit-advanced">高级编辑</button>
      <button class="btn danger" id="btn-job-delete">删除</button>
    </div>
  `;
}
```

- [ ] **Step 4: Run manual verification**

Run:
```bash
go run ./cmd/main.go
```
Manual Expected:
- 点击任务行后右侧出现详情
- 点击 Run / Enable / Disable 仍可用
- 详情中显示最近一次运行结果

- [ ] **Step 5: Commit**

```bash
git add internal/web/assets/ui/app.js
git commit -m "feat: add jobs detail view and quick actions"
```

---

### Task 7: 快速编辑抽屉

**Files:**
- Modify: `internal/web/assets/ui/index.html`
- Modify: `internal/web/assets/ui/styles.css`
- Modify: `internal/web/assets/ui/app.js`

- [ ] **Step 1: Write the failing test**

在 `index.html` 断言新增抽屉容器：
```go
if !strings.Contains(rr.Body.String(), "job-drawer") {
  t.Fatalf("expected job drawer container")
}
```

- [ ] **Step 2: Run test to verify it fails**

Run:
```bash
go test ./internal/web -count=1
```
Expected: FAIL

- [ ] **Step 3: Write minimal implementation**

在 `index.html` 添加：
```html
<div class="drawer hidden" id="job-drawer">
  <div class="drawer-backdrop" id="job-drawer-backdrop"></div>
  <div class="drawer-panel">
    <div class="modal-head">
      <div style="font-weight:800">快速编辑任务</div>
      <button class="btn ghost" id="job-drawer-close">关闭</button>
    </div>
    <div id="job-drawer-body"></div>
  </div>
</div>
```

在 `app.js` 增加：
- `openJobDrawer(job)` / `closeJobDrawer()`
- 表单字段：name/type/cron/enabled/template_id/preview_confirmed
- 保存时调用 `PUT /api/v1/jobs/{name}`

- [ ] **Step 4: Run manual verification**

Run:
```bash
go run ./cmd/main.go
```
Manual Expected:
- 点击“快速编辑”打开抽屉
- 修改 cron 或 enabled 后保存成功
- 列表与详情局部刷新

- [ ] **Step 5: Commit**

```bash
git add internal/web/assets/ui/index.html internal/web/assets/ui/styles.css internal/web/assets/ui/app.js
git commit -m "feat: add jobs quick edit drawer"
```

---

### Task 8: 高级编辑页（表单/JSON 双模式）

**Files:**
- Modify: `internal/web/assets/ui/index.html`
- Modify: `internal/web/assets/ui/styles.css`
- Modify: `internal/web/assets/ui/app.js`

- [ ] **Step 1: Write the failing test**

在 `index.html` 断言存在高级编辑页容器和“JSON 模式”文本：
```go
if !strings.Contains(rr.Body.String(), "JSON 模式") {
  t.Fatalf("expected advanced editor json mode")
}
```

- [ ] **Step 2: Run test to verify it fails**

Run:
```bash
go test ./internal/web -count=1
```
Expected: FAIL

- [ ] **Step 3: Write minimal implementation**

在 `index.html` 新增隐藏高级编辑页：
```html
<section class="view hidden" id="view-job-editor">
  <div class="page-title">
    <div>
      <h2>高级编辑任务</h2>
      <div class="subtitle">支持表单模式与 JSON 模式</div>
    </div>
  </div>
  <div class="card">
    <div class="row">
      <button class="btn" id="btn-job-editor-form">表单模式</button>
      <button class="btn" id="btn-job-editor-json">JSON 模式</button>
      <button class="btn ghost" id="btn-job-editor-back">返回详情</button>
    </div>
    <div id="job-editor-form"></div>
    <textarea id="job-editor-json" class="hidden" rows="18"></textarea>
    <div class="row">
      <button class="btn primary" id="btn-job-editor-save">保存</button>
    </div>
  </div>
</section>
```

在 `app.js` 中：
- `openAdvancedJobEditor(job)`：切换到 `view-job-editor`
- 表单模式填充常见字段
- JSON 模式展示完整任务对象
- 保存时：
  - JSON 模式先 `JSON.parse` 校验
  - 通过后调用 `POST /api/v1/jobs` 或 `PUT /api/v1/jobs/{name}`

- [ ] **Step 4: Run manual verification**

Run:
```bash
go run ./cmd/main.go
```
Manual Expected:
- 从详情进入高级编辑页
- 可在表单模式与 JSON 模式之间切换
- 非法 JSON 时阻止保存并提示错误

- [ ] **Step 5: Commit**

```bash
git add internal/web/assets/ui/index.html internal/web/assets/ui/styles.css internal/web/assets/ui/app.js
git commit -m "feat: add advanced jobs editor with form and json modes"
```

---

### Task 9: 新建任务与删除任务流程

**Files:**
- Modify: `internal/web/assets/ui/app.js`
- Modify: `internal/web/server.go`

- [ ] **Step 1: Write the failing test**

后端已有 CRUD 但需覆盖 delete/create 完整路径，在 `internal/web/server_test.go` 增补：
```go
// create job-b
// delete job-b
// GET /api/v1/jobs/{name} 应返回 404
```

- [ ] **Step 2: Run test to verify it fails**

Run:
```bash
go test ./internal/web -count=1
```
Expected: FAIL until create/delete 路径齐全且行为正确。

- [ ] **Step 3: Write minimal implementation**

前端：
- `新建任务`按钮默认创建草稿：
```js
const draft = {
  name: "",
  enabled: true,
  cron: "0 */5 * * * *",
  type: "api_poll",
  params: { template_id: "users_list", input: {} }
};
openAdvancedJobEditor(draft, { isCreate: true });
```
- 删除按钮二次确认：
```js
if (!confirm(`确认删除任务 ${job.name}？`)) return;
await fetchJSON(`/api/v1/jobs/${encodeURIComponent(job.name)}`, { method: 'DELETE' });
```

- [ ] **Step 4: Run manual verification**

Run:
```bash
go run ./cmd/main.go
```
Manual Expected:
- 可新建 `api_poll` / `api_write`
- 可删除任务并从列表消失

- [ ] **Step 5: Commit**

```bash
git add internal/web/assets/ui/app.js internal/web/server.go internal/web/server_test.go
git commit -m "feat: add jobs create and delete flows"
```

---

## Self-Review

- Spec coverage:
  - 列表页：Task 5/6
  - 详情页：Task 6
  - 快速编辑抽屉：Task 7
  - 高级编辑页：Task 8
  - 新建/编辑/删除/启停/手动运行：Task 4/6/7/8/9
  - 最近 1 次运行记录落盘：Task 1/2
- Placeholder scan: 未使用 TBD/TODO/implement later；每个任务都有明确文件与运行命令。
- Type consistency:
  - 统一使用 `JobRunsStore` / `JobRunRecord`
  - 前端统一使用 `loadJobDetail` / `renderJobDetail` / `openAdvancedJobEditor`

---

## Execution Handoff

Plan complete and saved to `docs/superpowers/plans/2026-06-04-jobs-web-config.md`. Two execution options:

1. **Subagent-Driven (recommended)** - I dispatch a fresh subagent per task, review between tasks, fast iteration
2. **Inline Execution** - Execute tasks in this session using executing-plans, batch execution with checkpoints

Which approach?

