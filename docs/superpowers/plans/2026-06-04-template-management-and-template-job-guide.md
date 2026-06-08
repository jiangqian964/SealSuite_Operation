# 模板管理与模板任务模式 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 为 API 工具箱补齐模板管理能力（新增 / 详情 / 测试 / 删除），并把“模板任务模式”的使用说明真正落到用户可见文档与页面交互中。

**Architecture:** 后端继续以 `api-templates.yaml` 作为模板定义源，但从“整文件 PUT”升级为资源化模板接口；前端 API 工具箱从“只读表格 + 点击填入 template_id”升级为模板管理页；同时新增一份面向使用者的模板任务模式操作说明文档。

**Tech Stack:** Go 1.21、chi、yaml.v3、go:embed 原生前端、httptest、原生 JS modal/编辑态

---

## File Map

**Create**
- `docs/usage/template-job-mode-guide.md`
- `internal/storage/templates_store_test.go`（追加资源化操作测试）

**Modify**
- `internal/storage/templates_store.go`
- `internal/web/server.go`
- `internal/web/server_test.go`
- `internal/web/assets/ui/index.html`
- `internal/web/assets/ui/styles.css`
- `internal/web/assets/ui/app.js`

---

### Task 1: 补模板任务模式使用说明文档

**Files:**
- Create: `docs/usage/template-job-mode-guide.md`

- [ ] **Step 1: Write the document**

写一份面向操作者的说明文档，必须覆盖：
- 模板任务模式是什么
- 模板和 Job 的职责分工
- Headers 为什么不用用户填写（系统自动注入 `Content-Type` / `Authorization`）
- 查询任务怎么创建
- 写入任务怎么创建
- 什么时候该用模板任务模式，什么时候不适合

建议骨架：
```md
# 模板任务模式使用说明

## 什么是模板任务模式
## 模板和任务分别负责什么
## Headers 怎么处理
## 查询任务创建示例
## 写入任务创建示例
## 常见问题
```

- [ ] **Step 2: Commit**

```bash
git add docs/usage/template-job-mode-guide.md
git commit -m "docs: add template job mode user guide"
```

---

### Task 2: TemplatesStore 增加 Get / Upsert / Delete

**Files:**
- Modify: `internal/storage/templates_store.go`
- Modify: `internal/storage/templates_store_test.go`

- [ ] **Step 1: Write the failing test**

在 `internal/storage/templates_store_test.go` 追加：
```go
func TestTemplatesStoreUpsertGetDelete(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "api-templates.yaml")
	s := TemplatesStore{Path: p}

	if err := s.Upsert(Template{
		ID:       "users_list",
		Name:     "用户-列表",
		Category: "users",
		Method:   "GET",
		Path:     "/api/open/v1/users",
	}); err != nil {
		t.Fatal(err)
	}

	got, ok, err := s.Get("users_list")
	if err != nil || !ok || got.ID != "users_list" {
		t.Fatalf("unexpected get: tpl=%#v ok=%v err=%v", got, ok, err)
	}

	if err := s.Delete("users_list"); err != nil {
		t.Fatal(err)
	}
	_, ok, err = s.Get("users_list")
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
Expected: FAIL with `TemplatesStore.Upsert undefined` or similar.

- [ ] **Step 3: Write minimal implementation**

在 `internal/storage/templates_store.go` 增加：
```go
func (s TemplatesStore) Get(id string) (*Template, bool, error) {
	tf, err := s.Load()
	if err != nil {
		return nil, false, err
	}
	for _, t := range tf.Templates {
		if t.ID == id {
			cp := t
			return &cp, true, nil
		}
	}
	return nil, false, nil
}

func (s TemplatesStore) Upsert(tpl Template) error {
	tf, err := s.Load()
	if err != nil {
		return err
	}
	replaced := false
	for i := range tf.Templates {
		if tf.Templates[i].ID == tpl.ID {
			tf.Templates[i] = tpl
			replaced = true
			break
		}
	}
	if !replaced {
		tf.Templates = append(tf.Templates, tpl)
	}
	return s.Save(tf)
}

func (s TemplatesStore) Delete(id string) error {
	tf, err := s.Load()
	if err != nil {
		return err
	}
	out := make([]Template, 0, len(tf.Templates))
	for _, t := range tf.Templates {
		if t.ID != id {
			out = append(out, t)
		}
	}
	tf.Templates = out
	return s.Save(tf)
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
git add internal/storage/templates_store.go internal/storage/templates_store_test.go
git commit -m "feat: add template store resource helpers"
```

---

### Task 3: 后端模板资源化 API

**Files:**
- Modify: `internal/web/server.go`
- Modify: `internal/web/server_test.go`

- [ ] **Step 1: Write the failing test**

在 `internal/web/server_test.go` 追加：
```go
func TestTemplateCRUDEndpoints(t *testing.T) {
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
	req := httptest.NewRequest(http.MethodPost, "/api/v1/api/templates", strings.NewReader(`{"id":"users_list_tmp","name":"用户-列表","category":"users","method":"GET","path":"/api/open/v1/users"}`))
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
Expected: FAIL because `POST /api/v1/api/templates` or detail/delete/test route not yet present.

- [ ] **Step 3: Write minimal implementation**

在 `internal/web/server.go` 增加：
- `GET /api/v1/api/templates/{id}`
- `POST /api/v1/api/templates`
- `PUT /api/v1/api/templates/{id}`
- `DELETE /api/v1/api/templates/{id}`
- `POST /api/v1/api/templates/{id}/test`

其中：
- `DELETE` 前扫描 `jobs.yaml`，若有 job 的 `params.template_id == id`，返回 400 + 引用任务名列表
- `test` 接口输入：
```json
{
  "path_params": {"user_id":"1"},
  "query": {"page":"1"},
  "body": {"name":"demo"},
  "mode": "execute"
}
```
后端通过：
```go
api.ExecuteRequest{
  TemplateID: id,
  PathParams: in.PathParams,
  Query: in.Query,
  Body: in.Body,
}
```
调用执行器。

- [ ] **Step 4: Run test to verify it passes**

Run:
```bash
go test ./internal/web -count=1
```
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/web/server.go internal/web/server_test.go
git commit -m "feat: add template CRUD and test endpoints"
```

---

### Task 4: 模板列表页升级为模板管理页

**Files:**
- Modify: `internal/web/assets/ui/index.html`
- Modify: `internal/web/assets/ui/styles.css`
- Modify: `internal/web/assets/ui/app.js`

- [ ] **Step 1: Write the failing test**

在 `internal/web/server_test.go` 的首页断言增加：
```go
if !strings.Contains(rr.Body.String(), "新增模板") {
	t.Fatalf("expected template create button")
}
if !strings.Contains(rr.Body.String(), "模板详情") {
	t.Fatalf("expected template detail section")
}
```

- [ ] **Step 2: Run test to verify it fails**

Run:
```bash
go test ./internal/web -count=1
```
Expected: FAIL because index 尚未包含新增按钮和详情区域。

- [ ] **Step 3: Write minimal implementation**

在 `index.html` 的 API 工具箱左侧区域新增：
- 搜索框
- 新增模板按钮
- 模板列表表格增加 `category` / `supports dry-run`
- 右侧新增模板详情面板

最小骨架：
```html
<div class="grid api-layout">
  <div class="card">
    <div class="row">
      <input id="tpl-filter" placeholder="按 id/name/category 过滤…" />
      <button class="btn primary" id="btn-template-create">新增模板</button>
    </div>
    <table class="table" id="tpl-table">...</table>
  </div>
  <div class="card" id="tpl-detail-card">
    <div style="font-weight:750">模板详情</div>
    <div class="subtitle" id="tpl-detail-empty">选择一个模板查看详情</div>
    <div id="tpl-detail"></div>
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
git commit -m "feat: add template management layout"
```

---

### Task 5: 模板详情、测试、删除交互

**Files:**
- Modify: `internal/web/assets/ui/app.js`

- [ ] **Step 1: Write the failing test**

采用最小手动 RED：先在 `renderTplTable()` 的点击逻辑中调用一个不存在的函数：
```js
function loadTemplateDetail(id) {
  throw new Error('not implemented')
}
```

- [ ] **Step 2: Run manual verification to see the failure**

Run:
```bash
go run ./cmd/main.go
```
Manual Expected: 点击模板行后，前端控制台报 `not implemented`。

- [ ] **Step 3: Write minimal implementation**

在 `app.js` 中增加：
- `loadTemplateDetail(id)` -> `GET /api/v1/api/templates/{id}`
- `renderTemplateDetail(payload)`：展示 schema、dry-run 配置
- 行内按钮：
  - `测试`
  - `删除`
  - `编辑`

删除逻辑：
```js
if (!confirm(`确认删除模板 ${tpl.id}？`)) return;
await fetchJSON(`/api/v1/api/templates/${encodeURIComponent(tpl.id)}`, { method: 'DELETE' });
```
若后端返回“被 Jobs 引用”，toast 展示引用任务。

测试逻辑：
- 先打开测试 modal/面板，允许填 path_params/query/body
- 再调 `POST /api/v1/api/templates/{id}/test`

- [ ] **Step 4: Run manual verification**

Run:
```bash
go run ./cmd/main.go
```
Manual Expected:
- 点击模板行右侧出现详情
- 点击测试可看到响应结果
- 若模板未被 Job 引用，则可删除
- 若模板被 Job 引用，则删除被阻止并显示原因

- [ ] **Step 5: Commit**

```bash
git add internal/web/assets/ui/app.js
git commit -m "feat: add template detail, test, and delete flows"
```

---

### Task 6: 模板新增 / 编辑页（表单 + JSON 双模式）

**Files:**
- Modify: `internal/web/assets/ui/index.html`
- Modify: `internal/web/assets/ui/styles.css`
- Modify: `internal/web/assets/ui/app.js`

- [ ] **Step 1: Write the failing test**

在首页断言增加：
```go
if !strings.Contains(rr.Body.String(), "模板编辑") {
	t.Fatalf("expected template editor section")
}
if !strings.Contains(rr.Body.String(), "dry_run_query_param") {
	t.Fatalf("expected dry_run_query_param field")
}
```

- [ ] **Step 2: Run test to verify it fails**

Run:
```bash
go test ./internal/web -count=1
```
Expected: FAIL

- [ ] **Step 3: Write minimal implementation**

在 `index.html` 新增隐藏的模板编辑页：
```html
<section class="view hidden" id="view-template-editor">
  <div class="page-title">
    <div>
      <h2>模板编辑</h2>
      <div class="subtitle">系统自动注入 Content-Type 与 Authorization，无需手填 Headers</div>
    </div>
  </div>
  <div class="card">
    <div class="row">
      <button class="btn" id="btn-template-editor-form">表单模式</button>
      <button class="btn" id="btn-template-editor-json-btn">JSON 模式</button>
      <button class="btn ghost" id="btn-template-editor-back">返回模板列表</button>
    </div>
    <div id="template-editor-form"></div>
    <textarea id="template-editor-json" class="hidden" rows="18"></textarea>
    <div class="row">
      <button class="btn primary" id="btn-template-editor-save">保存</button>
    </div>
  </div>
</section>
```

表单字段至少包括：
- `id`
- `name`
- `category`
- `method`
- `path`
- `dry_run_query_param`
- `query_schema (JSON textarea)`
- `path_params_schema (JSON textarea)`
- `body_schema (JSON textarea)`

保存逻辑：
- create -> `POST /api/v1/api/templates`
- update -> `PUT /api/v1/api/templates/{id}`

- [ ] **Step 4: Run manual verification**

Run:
```bash
go run ./cmd/main.go
```
Manual Expected:
- 点击“新增模板”进入编辑页
- 可切换表单 / JSON 模式
- 保存后模板列表刷新

- [ ] **Step 5: Commit**

```bash
git add internal/web/assets/ui/index.html internal/web/assets/ui/styles.css internal/web/assets/ui/app.js
git commit -m "feat: add template editor with form and json modes"
```

---

## Self-Review

- Spec coverage:
  - 模板任务模式说明：Task 1
  - 模板列表支持新增 / 测试 / 删除：Task 3/4/5/6
  - 资源化模板接口：Task 2/3
  - Headers 固定系统注入：Task 1/6 文案与表单约束
  - 删除模板引用检查：Task 3/5
- Placeholder scan: 无 TBD/TODO；每个任务都有明确文件、步骤、命令。
- Type consistency:
  - 统一使用 `TemplatesStore.Get/Upsert/Delete`
  - 统一使用 `loadTemplateDetail / renderTemplateDetail / openTemplateEditor`

---

## Execution Handoff

Plan complete and saved to `docs/superpowers/plans/2026-06-04-template-management-and-template-job-guide.md`. Two execution options:

1. **Subagent-Driven (recommended)** - I dispatch a fresh subagent per task, review between tasks, fast iteration
2. **Inline Execution** - Execute tasks in this session using executing-plans, batch execution with checkpoints

Which approach?

