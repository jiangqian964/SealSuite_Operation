# Connection Config Subpages And Webhook Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 将“连接配置”重构为三个独立子页面（飞连API配置 / LLM_API配置 / webhook配置），统一采用“左侧配置列表 + 右侧配置详情”布局，并把 LLM_API 改为“单条配置 + 标签”模型，同时新增 webhook 配置中心与测试推送能力。

**Architecture:** 复用当前 `view-connection` 单页容器，但将其内部重组为二级子页面切换器。后端层面保留飞连连接历史存储，废弃“LLM 配置集”方向，改为单条 `LLM_API` 记录存储，并新增 `webhook` 存储与测试接口；前端层面三个子页面共享统一的列表-工作台渲染模式和输出区交互。

**Tech Stack:** Go（chi + embed 静态资源）、Vanilla JS、HTML/CSS、YAML 存储、Node 校验脚本。

---

## 0. Files & Responsibilities

**后端**
- Modify: `internal/web/server.go`
  - 新增/调整 `LLM_API` 单条配置接口与 `webhook` 接口
  - 新增 webhook 测试接口
- Create: `internal/storage/llm_api_store.go`
  - 存储单条 LLM API 配置记录（标签化）
- Create: `internal/storage/webhook_store.go`
  - 存储 webhook 配置
- Modify/Delete/Deprecate: `internal/storage/llm_config_sets_store.go`
  - 若当前已引入配置集实现，则在本轮迁移中停止前端使用，并在后端保留兼容或移除暴露

**前端**
- Modify: `internal/web/assets/ui/index.html`
  - `view-connection` 改成三子页面结构
- Modify: `internal/web/assets/ui/app.js`
  - 列表/详情/新建/编辑/测试/删除逻辑
- Modify: `internal/web/assets/ui/styles.css`
  - 三子页面统一样式

**镜像目录**
- Modify: `web/ui/index.html`
- Modify: `web/ui/app.js`
- Modify: `web/ui/styles.css`

**校验**
- Modify: `work/ui_binding_check.js`
- Create: `work/ui_connection_subpages_check.js`

---

## Task 1: Replace “LLM 配置集” backend model with single-record `LLM_API` storage

**Files:**
- Create: `internal/storage/llm_api_store.go`
- Modify: `internal/web/server.go`
- Create: `work/ui_connection_subpages_check.js`

- [ ] **Step 1: Write the failing backend contract check**

Create `work/ui_connection_subpages_check.js` with assertions:

```js
const fs = require('fs');
const server = fs.readFileSync('internal/web/server.go', 'utf8');
const failures = [];
[
  '/settings/llm/apis',
  '/settings/llm/apis/{id}/activate',
  '/settings/llm/apis/{id}',
].forEach(k => { if (!server.includes(k)) failures.push(`missing route ${k}`); });
if (server.includes('/settings/llm/config-sets')) {
  failures.push('legacy llm config-set routes should not remain the primary API');
}
if (failures.length) { console.error(failures.join('\\n')); process.exit(1); }
console.log('connection subpages backend routes ok');
```

- [ ] **Step 2: Run the check to verify it fails**

Run:

```bash
node work/ui_connection_subpages_check.js
```

Expected: FAIL because the current code still exposes config-set routes.

- [ ] **Step 3: Create the new `LLMAPIStore`**

Create `internal/storage/llm_api_store.go`:

```go
package storage

import (
  "fmt"
  "os"
  "sync"
  "time"

  "gopkg.in/yaml.v3"
)

type LLMAPIItem struct {
  ID        string   `yaml:"id"`
  Name      string   `yaml:"name"`
  Tags      []string `yaml:"tags"`
  Enabled   bool     `yaml:"enabled"`
  Provider  string   `yaml:"provider"`
  BaseURL   string   `yaml:"base_url"`
  APIKey    string   `yaml:"api_key"`
  Model     string   `yaml:"model"`
  Timeout   int      `yaml:"timeout"`
  Temperature float64 `yaml:"temperature"`
  MaxTokens int      `yaml:"max_tokens"`
  Thinking  bool     `yaml:"thinking"`
  ReasoningEffort string `yaml:"reasoning_effort"`
  ResponseFormat string  `yaml:"response_format"`
  SystemPrompt string    `yaml:"system_prompt"`
  CreatedAt string `yaml:"created_at"`
}

type LLMAPIFile struct {
  ActiveID string       `yaml:"active_id"`
  Items    []LLMAPIItem `yaml:"items"`
}

type LLMAPIStore struct {
  path string
  mu sync.Mutex
}
```

Implement:
- `Load()`
- `Save()`
- `UpsertAndMaybeActivate()`
- `Activate(id string)`
- `Delete(id string)`

- [ ] **Step 4: Add new routes in `server.go`**

Replace/introduce:

```go
apiR.Get("/settings/llm/apis", ...)
apiR.Post("/settings/llm/apis", ...)
apiR.Post("/settings/llm/apis/{id}/activate", ...)
apiR.Delete("/settings/llm/apis/{id}", ...)
```

`POST /settings/llm/apis` should:
- validate `id`, `name`
- normalize tags
- save one single LLM API item
- if `activate=true`, map the item into the runtime LLM config and call `configStore.UpdateLLMConfig(...)`

For the runtime mapping in this phase, store the active single record into whichever runtime LLM slot is currently being used as default. Minimal implementation:

```go
llmCfg.Planner = mapSingleLLMAPIToRole(item)
llmCfg.Formatter = llmCfg.Formatter
```

or, if the current runtime only supports one “default active” concept, explicitly document that the active single record feeds the default planner path until business-side references are introduced.

The API response should include `active` boolean on each list item.

- [ ] **Step 5: Run the check to verify it passes**

```bash
node work/ui_connection_subpages_check.js
```

- [ ] **Step 6: Commit**

```bash
git add internal/storage/llm_api_store.go internal/web/server.go work/ui_connection_subpages_check.js
git commit -m "feat(connection): replace llm config-set routes with single llm api routes"
```

---

## Task 2: Add webhook storage and backend APIs including test delivery

**Files:**
- Create: `internal/storage/webhook_store.go`
- Modify: `internal/web/server.go`
- Create: `internal/storage/webhook_store_test.go`

- [ ] **Step 1: Extend the failing backend check**

Update `work/ui_connection_subpages_check.js`:

```js
[
  '/settings/webhooks',
  '/settings/webhooks/{id}',
  '/settings/webhooks/test',
].forEach(k => { if (!server.includes(k)) failures.push(`missing route ${k}`); });
```

- [ ] **Step 2: Run the check to verify it fails**

```bash
node work/ui_connection_subpages_check.js
```

- [ ] **Step 3: Create `WebhookStore`**

Create `internal/storage/webhook_store.go`:

```go
package storage

type WebhookItem struct {
  ID         string            `yaml:"id"`
  Name       string            `yaml:"name"`
  URL        string            `yaml:"url"`
  Method     string            `yaml:"method"`
  Headers    map[string]string `yaml:"headers"`
  AuthType   string            `yaml:"auth_type"`
  BodyTmpl   string            `yaml:"body_template"`
  TimeoutSec int               `yaml:"timeout_sec"`
  RetryCount int               `yaml:"retry_count"`
  Enabled    bool              `yaml:"enabled"`
  CreatedAt  string            `yaml:"created_at"`
}
```

Implement `Load/Save/Upsert/Delete` using the same YAML store pattern.

- [ ] **Step 4: Add webhook routes**

In `server.go`:

```go
apiR.Get("/settings/webhooks", ...)
apiR.Post("/settings/webhooks", ...)
apiR.Delete("/settings/webhooks/{id}", ...)
apiR.Post("/settings/webhooks/test", ...)
```

`POST /settings/webhooks/test` should accept:

```json
{
  "url": "...",
  "method": "POST",
  "headers": {"Content-Type":"application/json"},
  "payload": {"title":"测试推送"}
}
```

and return:

```json
{
  "ok": true,
  "status_code": 200,
  "response_body": "...",
  "request_preview": {...}
}
```

Minimal implementation may use Go `http.Client` with timeout and JSON body.

- [ ] **Step 5: Run the check to verify it passes**

```bash
node work/ui_connection_subpages_check.js
```

- [ ] **Step 6: Commit**

```bash
git add internal/storage/webhook_store.go internal/web/server.go internal/storage/webhook_store_test.go work/ui_connection_subpages_check.js
git commit -m "feat(connection): add webhook config store and test endpoint"
```

---

## Task 3: Restructure `view-connection` into three subpages

**Files:**
- Modify: `internal/web/assets/ui/index.html`
- Modify: `web/ui/index.html`

- [ ] **Step 1: Write the failing DOM contract**

Extend `work/ui_connection_subpages_check.js`:

```js
const html = fs.readFileSync('internal/web/assets/ui/index.html', 'utf8');
[
  'id="conn-subnav"',
  'id="btn-sub-feilian"',
  'id="btn-sub-llm"',
  'id="btn-sub-webhook"',
  'id="subpage-feilian"',
  'id="subpage-llm"',
  'id="subpage-webhook"',
].forEach(k => { if (!html.includes(k)) failures.push(`missing ${k}`); });
```

- [ ] **Step 2: Run the check to verify it fails**

```bash
node work/ui_connection_subpages_check.js
```

- [ ] **Step 3: Replace the current connection layout**

In `internal/web/assets/ui/index.html`, restructure `view-connection`:

```html
<div class="segmented" id="conn-subnav">
  <button class="seg active" id="btn-sub-feilian">飞连API配置</button>
  <button class="seg" id="btn-sub-llm">LLM_API配置</button>
  <button class="seg" id="btn-sub-webhook">webhook配置</button>
</div>

<section id="subpage-feilian">...</section>
<section id="subpage-llm" class="hidden">...</section>
<section id="subpage-webhook" class="hidden">...</section>
```

Each subpage must contain:
- left list container
- right detail/workbench container

Keep the current `conn-out` and `llm-out` areas inside the relevant right panels.

Mirror equivalent structure into `web/ui/index.html` if that mirror is still used.

- [ ] **Step 4: Run the check to verify it passes**

```bash
node work/ui_connection_subpages_check.js
```

- [ ] **Step 5: Commit**

```bash
git add internal/web/assets/ui/index.html web/ui/index.html work/ui_connection_subpages_check.js
git commit -m "feat(connection): split connection config into three subpages"
```

---

## Task 4: Add styles for the three subpages and reusable list/detail layout

**Files:**
- Modify: `internal/web/assets/ui/styles.css`
- Modify: `web/ui/styles.css`

- [ ] **Step 1: Extend the CSS contract**

Add checks:

```js
const css = fs.readFileSync('internal/web/assets/ui/styles.css', 'utf8');
[
  '.conn-subnav',
  '.config-subpage-layout',
  '.config-record-list',
  '.config-record-item',
  '.config-record-item-active',
].forEach(k => { if (!css.includes(k)) failures.push(`missing css ${k}`); });
```

- [ ] **Step 2: Run the check to verify it fails**

```bash
node work/ui_connection_subpages_check.js
```

- [ ] **Step 3: Add the new styles**

Append:

```css
.conn-subnav{ display:flex; gap:10px; margin-bottom:16px; }
.config-subpage-layout{ display:grid; grid-template-columns:.92fr 1.38fr; gap:18px; align-items:start; }
.config-record-list{ display:flex; flex-direction:column; gap:10px; }
.config-record-item{ border:1px solid rgba(140,161,201,.18); border-radius:16px; padding:14px; background:linear-gradient(180deg, rgba(255,255,255,.96), rgba(247,250,255,.94)); }
.config-record-item-active{ border-color: rgba(47,107,255,.28); box-shadow: 0 12px 24px rgba(47,107,255,.08); }
@media (max-width:1100px){ .config-subpage-layout{ grid-template-columns:1fr; } }
```

- [ ] **Step 4: Run the check to verify it passes**

```bash
node work/ui_connection_subpages_check.js
```

- [ ] **Step 5: Commit**

```bash
git add internal/web/assets/ui/styles.css web/ui/styles.css work/ui_connection_subpages_check.js
git commit -m "feat(connection): add reusable subpage list-detail styles"
```

---

## Task 5: Implement 飞连API配置 subpage behaviors

**Files:**
- Modify: `internal/web/assets/ui/app.js`
- Modify: `web/ui/app.js`
- Modify: `work/ui_binding_check.js`

- [ ] **Step 1: Write the failing JS contract**

Extend `work/ui_connection_subpages_check.js`:

```js
const js = fs.readFileSync('internal/web/assets/ui/app.js', 'utf8');
[
  'function renderFeilianSubpage',
  'function openFeilianCreateMode',
  "qs('#btn-sub-feilian')",
].forEach(k => { if (!js.includes(k)) failures.push(`missing ${k}`); });
```

- [ ] **Step 2: Run the check to verify it fails**

```bash
node work/ui_connection_subpages_check.js
```

- [ ] **Step 3: Implement subpage switching**

In `app.js` add:

```js
window.__connSubpage = 'feilian';

function setConnectionSubpage(name) {
  window.__connSubpage = name;
  qs('#subpage-feilian')?.classList.toggle('hidden', name !== 'feilian');
  qs('#subpage-llm')?.classList.toggle('hidden', name !== 'llm');
  qs('#subpage-webhook')?.classList.toggle('hidden', name !== 'webhook');
}
```

Bind the subnav buttons.

- [ ] **Step 4: Render 飞连_API list + detail**

Refactor the existing connection-center rendering into the `feilian` subpage:
- left list uses current connection APIs
- right detail uses the existing connection workbench logic

Required functions:

```js
function renderFeilianSubpage() { ... }
function openFeilianCreateMode() { ... }
```

Reuse:
- `refreshConnections`
- `openConnectionRecord`
- `loadCurrentConnectionConfig`
- `getConnPayload`

- [ ] **Step 5: Run syntax + checks**

```bash
node --check internal/web/assets/ui/app.js
node work/ui_binding_check.js
node work/ui_connection_subpages_check.js
```

- [ ] **Step 6: Commit**

```bash
git add internal/web/assets/ui/app.js web/ui/app.js work/ui_binding_check.js work/ui_connection_subpages_check.js
git commit -m "feat(connection): implement feilian api config subpage"
```

---

## Task 6: Replace LLM “config set” UI with single-record + tags model

**Files:**
- Modify: `internal/web/assets/ui/index.html`
- Modify: `internal/web/assets/ui/app.js`
- Modify: `internal/web/server.go`

- [ ] **Step 1: Write the failing contract**

Require:

```js
[
  'id="llm-api-id"',
  'id="llm-api-name"',
  'id="llm-api-tags"',
  'function getLLMAPIPayload',
  '/api/v1/settings/llm/apis',
].forEach(k => { if (!html.includes(k) && !js.includes(k) && !server.includes(k)) failures.push(`missing ${k}`); });
```

- [ ] **Step 2: Run the check to verify it fails**

```bash
node work/ui_connection_subpages_check.js
```

- [ ] **Step 3: Replace the old LLM set form**

In `index.html`, remove the old:
- `llm-set-id`
- `llm-set-name`
- planner/formatter paired form

and replace with a single LLM API form:

```html
<input id="llm-api-id" />
<input id="llm-api-name" />
<input id="llm-api-tags" placeholder="Planner 模型, Formatter 模型, 自定义标签" />
<input id="llm-api-provider" />
<input id="llm-api-base-url" />
<input id="llm-api-api-key" />
<input id="llm-api-model" />
```

- [ ] **Step 4: Add payload and fill helpers**

In `app.js`:

```js
function parseTagInput(v) {
  return String(v || '').split(',').map(s => s.trim()).filter(Boolean);
}

function getLLMAPIPayload() {
  return {
    id: qs('#llm-api-id')?.value.trim() || '',
    name: qs('#llm-api-name')?.value.trim() || '',
    tags: parseTagInput(qs('#llm-api-tags')?.value || ''),
    enabled: qs('#llm-api-enabled')?.value === 'true',
    provider: qs('#llm-api-provider')?.value.trim() || '',
    base_url: qs('#llm-api-base-url')?.value.trim() || '',
    api_key: qs('#llm-api-api-key')?.value.trim() || '',
    model: qs('#llm-api-model')?.value.trim() || '',
    system_prompt: qs('#llm-api-system-prompt')?.value || ''
  };
}
```

Save to:

```js
await fetchJSON('/api/v1/settings/llm/apis', { method:'POST', body: JSON.stringify(getLLMAPIPayload()) });
```

- [ ] **Step 5: Rebuild the left LLM list**

Render each row using:
- name
- provider / model
- tags
- active

and support:
- open
- activate
- delete

- [ ] **Step 6: Run syntax + checks**

```bash
node --check internal/web/assets/ui/app.js
node work/ui_connection_subpages_check.js
```

- [ ] **Step 7: Commit**

```bash
git add internal/web/assets/ui/index.html internal/web/assets/ui/app.js internal/web/server.go work/ui_connection_subpages_check.js
git commit -m "feat(connection): replace llm config sets with single tagged llm api records"
```

---

## Task 7: Add webhook配置 subpage with test push

**Files:**
- Modify: `internal/web/assets/ui/index.html`
- Modify: `internal/web/assets/ui/app.js`
- Modify: `internal/web/server.go`

- [ ] **Step 1: Write the failing contract**

Require:

```js
[
  'id="webhook-list"',
  'id="webhook-id"',
  'id="webhook-url"',
  'id="webhook-test-payload"',
  'function getWebhookPayload',
  '/api/v1/settings/webhooks',
  '/api/v1/settings/webhooks/test',
].forEach(k => { if (!html.includes(k) && !js.includes(k) && !server.includes(k)) failures.push(`missing ${k}`); });
```

- [ ] **Step 2: Run the check to verify it fails**

```bash
node work/ui_connection_subpages_check.js
```

- [ ] **Step 3: Build webhook subpage HTML**

Add:

```html
<div id="webhook-list" class="subtitle">暂无 webhook 配置</div>
<input id="webhook-id" />
<input id="webhook-name" />
<input id="webhook-url" />
<select id="webhook-method"><option>POST</option><option>PUT</option></select>
<textarea id="webhook-headers"></textarea>
<textarea id="webhook-body-template"></textarea>
<textarea id="webhook-test-payload"></textarea>
<button id="btn-webhook-test">测试推送</button>
<button id="btn-webhook-save">保存</button>
```

- [ ] **Step 4: Add JS helpers**

```js
function getWebhookPayload() {
  return {
    id: qs('#webhook-id')?.value.trim() || '',
    name: qs('#webhook-name')?.value.trim() || '',
    url: qs('#webhook-url')?.value.trim() || '',
    method: qs('#webhook-method')?.value || 'POST',
    headers: parseJSONObjectSafe(qs('#webhook-headers')?.value || '{}'),
    body_template: qs('#webhook-body-template')?.value || '',
    enabled: qs('#webhook-enabled')?.checked === true
  };
}
```

Test action:

```js
await fetchJSON('/api/v1/settings/webhooks/test', {
  method:'POST',
  body: JSON.stringify({
    url: qs('#webhook-url').value,
    method: qs('#webhook-method').value,
    headers: parseJSONObjectSafe(qs('#webhook-headers').value || '{}'),
    payload: parseJSONObjectSafe(qs('#webhook-test-payload').value || '{}')
  })
});
```

Render results into a dedicated right-panel output area or reuse the connection output area for the webhook subpage.

- [ ] **Step 5: Run syntax + checks**

```bash
node --check internal/web/assets/ui/app.js
node work/ui_connection_subpages_check.js
```

- [ ] **Step 6: Commit**

```bash
git add internal/web/assets/ui/index.html internal/web/assets/ui/app.js internal/web/server.go work/ui_connection_subpages_check.js
git commit -m "feat(connection): add webhook config subpage with test delivery"
```

---

## Task 8: Remove legacy connection-center assumptions and verify the three-subpage flow

**Files:**
- Modify: `internal/web/assets/ui/app.js`
- Modify: `internal/web/assets/ui/index.html`
- Modify: `internal/web/assets/ui/styles.css`
- Modify: `work/ui_binding_check.js`
- Modify: `work/ui_connection_subpages_check.js`

- [ ] **Step 1: Remove obsolete assumptions**

Delete or neutralize:
- `llm-set-*` form fields
- old “LLM config set” helper names
- single combined connection-center hero copy that no longer matches the three-subpage structure

If needed, leave compatibility helpers as thin wrappers, but the new UI must not depend on them.

- [ ] **Step 2: Verify binding IDs**

Run:

```bash
node work/ui_binding_check.js
```

Expected: PASS with no missing binding ids.

- [ ] **Step 3: Verify the new subpage contract**

Run:

```bash
node work/ui_connection_subpages_check.js
```

Expected: PASS and no references to `config-sets`, `llm-set-id`, or removed legacy DOM.

- [ ] **Step 4: Commit**

```bash
git add internal/web/assets/ui/app.js internal/web/assets/ui/index.html internal/web/assets/ui/styles.css work/ui_binding_check.js work/ui_connection_subpages_check.js
git commit -m "refactor(connection): finalize subpages and remove legacy llm config-set assumptions"
```

---

## Task 9: Prepare business-object reference fields for webhook reuse

**Files:**
- Modify: `internal/web/assets/ui/index.html`
- Modify: `internal/web/assets/ui/app.js`
- Modify: `internal/web/server.go`

- [ ] **Step 1: Add non-breaking fields to relevant payloads**

Add optional fields to:
- API task draft payloads
- job schedule payloads
- complex task / 运营agent payloads

Fields:

```json
{
  "webhook_config_id": "",
  "webhook_enabled": false
}
```

- [ ] **Step 2: Add select dropdowns in the relevant workbenches**

Minimal non-breaking UI:
- one select for webhook config
- one checkbox for enable/disable push

Populate options via:

```js
await fetchJSON('/api/v1/settings/webhooks')
```

- [ ] **Step 3: Do not implement real push execution yet**

Ensure this task only saves reference fields and does not alter runtime execution paths yet.

- [ ] **Step 4: Run syntax + checks**

```bash
node --check internal/web/assets/ui/app.js
node work/ui_binding_check.js
```

- [ ] **Step 5: Commit**

```bash
git add internal/web/assets/ui/index.html internal/web/assets/ui/app.js internal/web/server.go
git commit -m "feat(webhook): add reference fields to tasks schedules and agents"
```

---

## Self-Review Checklist

- [ ] The plan fully replaces the old “LLM config set” direction with the approved “single LLM API + tags” model.
- [ ] Webhook is treated as a first-class config type with its own subpage and test endpoint.
- [ ] The three subpages all follow the same list/detail structure.
- [ ] Business objects only store webhook references in this phase; real push execution is explicitly deferred.
- [ ] No TODO/TBD placeholders remain.

---

## Execution Handoff

Plan complete and saved to `docs/superpowers/plans/2026-06-06-connection-config-subpages-and-webhook.md`.

Two execution options:

**1. Subagent-Driven (recommended)** - I dispatch a fresh subagent per task, review between tasks, fast iteration

**2. Inline Execution** - Execute tasks in this session using executing-plans, batch execution with checkpoints

Which approach?

