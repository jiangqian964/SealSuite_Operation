# Connection Settings API Center Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 将“连接设置”页重构为左右双栏 API 配置中心，左侧管理 `飞连_API` 与 `LLM_API` 历史配置列表，右侧提供统一详情 / 新增 / 编辑工作台，并保留配置输出与测试能力。

**Architecture:** 复用当前 `view-connection` 单页视图，但将前端逻辑从“两个表单 + 一个历史表”重构为“左侧列表 + 右侧工作台”的状态驱动模式。飞连连接继续沿用现有 `connectionsStore`，同时新增 `LLM_API` 配置集存储与 ACTIVE 切换/删除接口，让前端两类 API 都能以一致的列表-详情方式管理。

**Tech Stack:** Go（chi + embed 静态资源）、Vanilla JS、HTML/CSS、YAML 存储、Node 校验脚本。

---

## 0. Files & Responsibilities

**后端**
- Create: `internal/storage/llm_config_sets_store.go`
  - 保存/加载/激活/删除 LLM 配置集（planner + formatter 一组）
- Modify: `internal/web/server.go`
  - 新增 `LLM_API` 配置集列表/保存/激活/删除接口
  - 保留并复用现有飞连连接接口与 LLM 测试接口

**前端主实现**
- Modify: `internal/web/assets/ui/index.html`
  - 将 `view-connection` 改成左侧双列表、右侧统一工作台
- Modify: `internal/web/assets/ui/app.js`
  - 引入统一连接页状态模型
  - 渲染 `飞连_API` / `LLM_API` 列表
  - 管理右侧工作台查看/编辑/新建态
- Modify: `internal/web/assets/ui/styles.css`
  - 实现双栏布局、列表态、ACTIVE 高亮、右侧工作台状态样式

**镜像目录（如仍参与预览）**
- Modify: `web/ui/index.html`
- Modify: `web/ui/app.js`
- Modify: `web/ui/styles.css`

**校验**
- Create: `work/ui_connection_center_check.js`
  - 校验新布局 DOM、关键按钮 ID、JS helper、后端接口标记
- Modify: `work/ui_binding_check.js`
  - 覆盖新增 direct binding DOM

---

## Task 1: Add LLM config set storage and backend list API

**Files:**
- Create: `internal/storage/llm_config_sets_store.go`
- Modify: `internal/web/server.go`

- [ ] **Step 1: Write the failing backend contract check**

Create `work/ui_connection_center_check.js` with backend string assertions:

```js
const fs = require('fs');
const server = fs.readFileSync('internal/web/server.go', 'utf8');
const failures = [];
[
  '/settings/llm/config-sets',
  '/settings/llm/config-sets/{id}/activate',
  '/settings/llm/config-sets/{id}',
].forEach(k => { if (!server.includes(k)) failures.push(`missing route ${k}`); });
if (failures.length) { console.error(failures.join('\n')); process.exit(1); }
console.log('connection center backend routes ok');
```

- [ ] **Step 2: Run the check to verify it fails**

Run:

```bash
node work/ui_connection_center_check.js
```

Expected: FAIL because the LLM config set routes do not exist yet.

- [ ] **Step 3: Create minimal `LLMConfigSetsStore`**

Create `internal/storage/llm_config_sets_store.go` with a YAML-backed store similar in spirit to `connectionsStore`.

Production code for the first pass:

```go
package storage

import (
	"os"
	"sync"
	"time"

	"gopkg.in/yaml.v3"
)

type LLMConfigSetItem struct {
	ID        string                 `yaml:"id"`
	Name      string                 `yaml:"name"`
	Planner   map[string]interface{} `yaml:"planner"`
	Formatter map[string]interface{} `yaml:"formatter"`
	CreatedAt string                 `yaml:"created_at"`
}

type LLMConfigSetsFile struct {
	ActiveID string             `yaml:"active_id"`
	Items    []LLMConfigSetItem `yaml:"items"`
}

type LLMConfigSetsStore struct {
	path string
	mu   sync.Mutex
}

func NewLLMConfigSetsStore(path string) *LLMConfigSetsStore {
	return &LLMConfigSetsStore{path: path}
}

func (s *LLMConfigSetsStore) Load() (*LLMConfigSetsFile, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	data, err := os.ReadFile(s.path)
	if err != nil {
		if os.IsNotExist(err) {
			return &LLMConfigSetsFile{}, nil
		}
		return nil, err
	}
	var out LLMConfigSetsFile
	if err := yaml.Unmarshal(data, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

func (s *LLMConfigSetsStore) Save(f *LLMConfigSetsFile) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	data, err := yaml.Marshal(f)
	if err != nil {
		return err
	}
	return os.WriteFile(s.path, data, 0644)
}

func (s *LLMConfigSetsStore) UpsertAndMaybeActivate(item LLMConfigSetItem, activate bool) error {
	f, err := s.Load()
	if err != nil { return err }
	found := false
	for i := range f.Items {
		if f.Items[i].ID == item.ID {
			f.Items[i] = item
			found = true
			break
		}
	}
	if !found {
		if item.CreatedAt == "" {
			item.CreatedAt = time.Now().Format(time.RFC3339)
		}
		f.Items = append(f.Items, item)
	}
	if activate {
		f.ActiveID = item.ID
	}
	return s.Save(f)
}
```

- [ ] **Step 4: Wire minimal routes in `server.go`**

Add routes:

```go
apiR.Get("/settings/llm/config-sets", ...)
apiR.Post("/settings/llm/config-sets", ...)
apiR.Post("/settings/llm/config-sets/{id}/activate", ...)
apiR.Delete("/settings/llm/config-sets/{id}", ...)
```

The GET should return:

```go
writeJSON(w, http.StatusOK, map[string]interface{}{
  "active_id": file.ActiveID,
  "items": items,
})
```

The POST should:
- validate `id`, `name`
- sanitize planner/formatter payload
- write to store
- if `activate=true`, also call `configStore.UpdateLLMConfig(...)` and hot-reload

The activate route should:
- load the item from the store
- call `configStore.UpdateLLMConfig(...)`
- hot-reload runner
- set `active_id`

The delete route should:
- reject deleting the ACTIVE item
- remove the item otherwise

- [ ] **Step 5: Run the check to verify it passes**

```bash
node work/ui_connection_center_check.js
```

Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add internal/storage/llm_config_sets_store.go internal/web/server.go work/ui_connection_center_check.js
git commit -m "feat(connection): add llm config set store and APIs"
```

---

## Task 2: Add delete support for 飞连_API history

**Files:**
- Modify: `internal/web/server.go`
- (If needed) Modify: `internal/storage/connections_store.go`

- [ ] **Step 1: Write a failing route check**

Extend `work/ui_connection_center_check.js`:

```js
if (!server.includes('/connections/{id}')) failures.push('missing DELETE /connections/{id}');
```

- [ ] **Step 2: Run the check to verify it fails**

```bash
node work/ui_connection_center_check.js
```

- [ ] **Step 3: Implement connection delete route**

Add in `server.go`:

```go
apiR.Delete("/connections/{id}", func(w http.ResponseWriter, req *http.Request) {
  id := chi.URLParam(req, "id")
  cf, err := connectionsStore.Load()
  if err != nil {
    writeJSON(w, http.StatusInternalServerError, map[string]interface{}{"error": err.Error()})
    return
  }
  if cf.ActiveID == id {
    writeJSON(w, http.StatusBadRequest, map[string]interface{}{"error": "cannot delete active connection"})
    return
  }
  if err := connectionsStore.Delete(id); err != nil {
    writeJSON(w, http.StatusBadRequest, map[string]interface{}{"error": err.Error()})
    return
  }
  writeJSON(w, http.StatusOK, map[string]interface{}{"ok": true})
})
```

If `connectionsStore.Delete()` does not exist, add it with the same pattern used by other stores.

- [ ] **Step 4: Run the check to verify it passes**

```bash
node work/ui_connection_center_check.js
```

- [ ] **Step 5: Commit**

```bash
git add internal/web/server.go internal/storage/connections_store.go work/ui_connection_center_check.js
git commit -m "feat(connection): support deleting inactive connection records"
```

---

## Task 3: Replace current connection page layout with left-list / right-workbench skeleton

**Files:**
- Modify: `internal/web/assets/ui/index.html`
- Modify: `web/ui/index.html`
- Modify: `work/ui_connection_center_check.js`

- [ ] **Step 1: Write the failing DOM contract**

Extend/create checks for required DOM:

```js
const html = fs.readFileSync('internal/web/assets/ui/index.html', 'utf8');
[
  'id="conn-center-layout"',
  'id="conn-list-feilian"',
  'id="conn-list-llm"',
  'id="btn-conn-create"',
  'id="btn-llm-set-create"',
  'id="conn-workbench"',
].forEach(k => { if (!html.includes(k)) failures.push(`missing ${k}`); });
```

- [ ] **Step 2: Run the check to verify it fails**

```bash
node work/ui_connection_center_check.js
```

- [ ] **Step 3: Implement the new skeleton**

In `internal/web/assets/ui/index.html`, replace the current stacked structure of `view-connection` with:

```html
<div class="grid connection-center-layout" id="conn-center-layout">
  <div class="card connection-side">
    <div class="panel-head">
      <div>
        <div class="panel-title">最近启用 API</div>
        <div class="subtitle">左侧统一管理飞连_API 与 LLM_API 配置。</div>
      </div>
    </div>
    <div class="card" style="box-shadow:none">
      <div class="panel-head">
        <div>
          <div class="panel-title">飞连_API</div>
        </div>
        <div class="panel-actions">
          <button class="btn" id="btn-conn-refresh">刷新</button>
          <button class="btn primary" id="btn-conn-create">新增</button>
        </div>
      </div>
      <div id="conn-list-feilian" class="subtitle">暂无飞连_API 配置</div>
    </div>
    <div class="card" style="box-shadow:none; margin-top:12px">
      <div class="panel-head">
        <div>
          <div class="panel-title">LLM_API</div>
        </div>
        <div class="panel-actions">
          <button class="btn" id="btn-llm-set-refresh">刷新</button>
          <button class="btn primary" id="btn-llm-set-create">新增</button>
        </div>
      </div>
      <div id="conn-list-llm" class="subtitle">暂无 LLM_API 配置集</div>
    </div>
  </div>

  <div class="card connection-workbench" id="conn-workbench">
    <!-- current type/mode badge -->
    <!-- connection or llm form host -->
    <!-- output area -->
  </div>
</div>
```

Keep the existing `配置输出` block in the right workbench.

Mirror to `web/ui/index.html` if that mirror is still used.

- [ ] **Step 4: Run the check to verify it passes**

```bash
node work/ui_connection_center_check.js
```

- [ ] **Step 5: Commit**

```bash
git add internal/web/assets/ui/index.html web/ui/index.html work/ui_connection_center_check.js
git commit -m "feat(connection): add api center dual-pane layout skeleton"
```

---

## Task 4: Add styles for dual-pane connection center

**Files:**
- Modify: `internal/web/assets/ui/styles.css`
- Modify: `web/ui/styles.css`
- Modify: `work/ui_connection_center_check.js`

- [ ] **Step 1: Extend the check with CSS markers**

```js
const css = fs.readFileSync('internal/web/assets/ui/styles.css', 'utf8');
[
  '.connection-center-layout',
  '.api-config-list',
  '.api-config-item',
  '.api-config-item-active',
  '.connection-workbench',
].forEach(k => { if (!css.includes(k)) failures.push(`missing css ${k}`); });
```

- [ ] **Step 2: Run and verify failure**

```bash
node work/ui_connection_center_check.js
```

- [ ] **Step 3: Add minimal styles**

Append:

```css
.connection-center-layout{
  grid-template-columns: .92fr 1.38fr;
  align-items: start;
}
.api-config-list{
  display:flex;
  flex-direction:column;
  gap:10px;
}
.api-config-item{
  border:1px solid rgba(140,161,201,.18);
  border-radius:16px;
  padding:14px;
  background:linear-gradient(180deg, rgba(255,255,255,.96), rgba(247,250,255,.94));
}
.api-config-item-active{
  border-color: rgba(47,107,255,.28);
  box-shadow: 0 12px 24px rgba(47,107,255,.08);
}
```

Add mobile fallback:

```css
@media (max-width: 1100px){
  .connection-center-layout{ grid-template-columns: 1fr; }
}
```

- [ ] **Step 4: Run the check to verify it passes**

```bash
node work/ui_connection_center_check.js
```

- [ ] **Step 5: Commit**

```bash
git add internal/web/assets/ui/styles.css web/ui/styles.css work/ui_connection_center_check.js
git commit -m "feat(connection): style api center dual-pane layout"
```

---

## Task 5: Refactor frontend state model for the connection center

**Files:**
- Modify: `internal/web/assets/ui/app.js`
- Modify: `web/ui/app.js`
- Modify: `work/ui_binding_check.js`

- [ ] **Step 1: Write the failing JS contract**

Extend `work/ui_connection_center_check.js`:

```js
const js = fs.readFileSync('internal/web/assets/ui/app.js', 'utf8');
[
  'window.__connCenterState',
  'function renderConnectionCenter',
  'function renderFeilianConfigList',
  'function renderLLMConfigSetList',
  'function setConnectionWorkbenchMode',
].forEach(k => { if (!js.includes(k)) failures.push(`missing ${k}`); });
```

- [ ] **Step 2: Run the check to verify it fails**

```bash
node work/ui_connection_center_check.js
```

- [ ] **Step 3: Add the new state object**

In `internal/web/assets/ui/app.js`:

```js
window.__connCenterState = {
  currentType: 'connection',
  currentMode: 'view',
  currentConnectionID: '',
  currentLLMConfigSetID: '',
  connList: [],
  llmConfigSets: []
};
```

- [ ] **Step 4: Add render helpers**

Define:

```js
function setConnectionWorkbenchMode(type, mode, id) { ... }
function renderConnectionCenter() {
  renderFeilianConfigList(window.__connCenterState.connList);
  renderLLMConfigSetList(window.__connCenterState.llmConfigSets);
  renderConnectionWorkbench();
}
```

`renderConnectionWorkbench()` decides whether the right pane shows:
- 飞连详情/编辑/新建
- LLM 配置集详情/编辑/新建

- [ ] **Step 5: Run syntax + checks**

```bash
node --check internal/web/assets/ui/app.js
node work/ui_connection_center_check.js
```

- [ ] **Step 6: Commit**

```bash
git add internal/web/assets/ui/app.js web/ui/app.js work/ui_binding_check.js work/ui_connection_center_check.js
git commit -m "refactor(connection): add api center frontend state model"
```

---

## Task 6: Render 飞连_API list and connect it to the right workbench

**Files:**
- Modify: `internal/web/assets/ui/app.js`
- Modify: `web/ui/app.js`

- [ ] **Step 1: Write failing contract**

Require:

```js
[
  'function refreshConnections',
  'function renderFeilianConfigList',
  '/api/v1/connections/',
].forEach(k => { if (!js.includes(k)) failures.push(`missing ${k}`); });
```

- [ ] **Step 2: Run the check to verify it fails or is incomplete**

```bash
node work/ui_connection_center_check.js
```

- [ ] **Step 3: Refactor `refreshConnections()`**

Update:

```js
async function refreshConnections() {
  const data = await fetchJSON('/api/v1/connections');
  window.__connCenterState.connList = data.items || [];
  renderConnectionCenter();
}
```

Replace `renderConnTable()` with `renderFeilianConfigList(items)`:

```js
function renderFeilianConfigList(items) {
  const host = qs('#conn-list-feilian');
  if (!host) return;
  if (!items.length) {
    host.innerHTML = '<div class="subtitle">暂无飞连_API 配置</div>';
    return;
  }
  host.className = 'api-config-list';
  host.innerHTML = items.map(it => `
    <div class="api-config-item ${it.active ? 'api-config-item-active' : ''}" data-id="${escapeHtml(it.id || '')}">
      <div style="font-weight:750">${escapeHtml(it.name || it.id || '')} ${it.active ? '<span class="pill ok">ACTIVE</span>' : ''}</div>
      <div class="subtitle"><code>${escapeHtml(`${it.scheme || 'https'}://${it.host || ''}:${it.port || ''}`)}</code></div>
      <div class="subtitle">AK: <code>${escapeHtml(it.access_key_id || '')}</code></div>
      <div class="api-task-actions">
        <button class="btn" data-act="open" data-id="${escapeHtml(it.id || '')}">查看</button>
        <button class="btn" data-act="activate" data-id="${escapeHtml(it.id || '')}" ${it.active ? 'disabled' : ''}>设为 ACTIVE</button>
        <button class="btn danger" data-act="delete" data-id="${escapeHtml(it.id || '')}" ${it.active ? 'disabled' : ''}>删除</button>
      </div>
    </div>
  `).join('');
}
```

- [ ] **Step 4: Bind actions**

After rendering:
- `open`: load the item into the right workbench view mode
- `activate`: call `POST /api/v1/connections/{id}/activate`
- `delete`: call `DELETE /api/v1/connections/{id}`

- [ ] **Step 5: Run syntax + checks**

```bash
node --check internal/web/assets/ui/app.js
node work/ui_connection_center_check.js
```

- [ ] **Step 6: Commit**

```bash
git add internal/web/assets/ui/app.js web/ui/app.js work/ui_connection_center_check.js
git commit -m "feat(connection): render feilian api list and actions"
```

---

## Task 7: Render `LLM_API` config set list and ACTIVE switching

**Files:**
- Modify: `internal/web/assets/ui/app.js`
- Modify: `internal/web/server.go`

- [ ] **Step 1: Write failing contract**

Require:

```js
[
  'function refreshLLMConfigSets',
  'function renderLLMConfigSetList',
  '/api/v1/settings/llm/config-sets',
].forEach(k => { if (!js.includes(k) && !server.includes(k)) failures.push(`missing ${k}`); });
```

- [ ] **Step 2: Run the check to verify it fails**

```bash
node work/ui_connection_center_check.js
```

- [ ] **Step 3: Implement front-end fetch + render**

Add:

```js
async function refreshLLMConfigSets() {
  const data = await fetchJSON('/api/v1/settings/llm/config-sets');
  window.__connCenterState.llmConfigSets = data.items || [];
  renderConnectionCenter();
}
```

Render:

```js
function renderLLMConfigSetList(items) {
  const host = qs('#conn-list-llm');
  if (!host) return;
  if (!items.length) {
    host.innerHTML = '<div class="subtitle">暂无 LLM_API 配置集</div>';
    return;
  }
  host.className = 'api-config-list';
  host.innerHTML = items.map(it => `
    <div class="api-config-item ${it.active ? 'api-config-item-active' : ''}">
      <div style="font-weight:750">${escapeHtml(it.name || it.id || '')} ${it.active ? '<span class="pill ok">ACTIVE</span>' : ''}</div>
      <div class="subtitle">Planner: ${escapeHtml((it.planner || {}).provider || '')} / ${escapeHtml((it.planner || {}).model || '')}</div>
      <div class="subtitle">Formatter: ${escapeHtml((it.formatter || {}).provider || '')} / ${escapeHtml((it.formatter || {}).model || '')}</div>
      <div class="api-task-actions">
        <button class="btn" data-act="open-llm" data-id="${escapeHtml(it.id || '')}">查看</button>
        <button class="btn" data-act="activate-llm" data-id="${escapeHtml(it.id || '')}" ${it.active ? 'disabled' : ''}>设为 ACTIVE</button>
        <button class="btn danger" data-act="delete-llm" data-id="${escapeHtml(it.id || '')}" ${it.active ? 'disabled' : ''}>删除</button>
      </div>
    </div>
  `).join('');
}
```

- [ ] **Step 4: Wire activate/delete/open**

Call:
- `POST /api/v1/settings/llm/config-sets/{id}/activate`
- `DELETE /api/v1/settings/llm/config-sets/{id}`
- `open` loads the config set into the right workbench

- [ ] **Step 5: Run syntax + checks**

```bash
node --check internal/web/assets/ui/app.js
node work/ui_connection_center_check.js
```

- [ ] **Step 6: Commit**

```bash
git add internal/web/assets/ui/app.js internal/web/server.go work/ui_connection_center_check.js
git commit -m "feat(connection): add llm api config set list and active switching"
```

---

## Task 8: Build the unified right workbench for 飞连_API view/edit/create

**Files:**
- Modify: `internal/web/assets/ui/index.html`
- Modify: `internal/web/assets/ui/app.js`

- [ ] **Step 1: Write failing contract**

Require DOM markers:

```js
[
  'id="conn-workbench-type"',
  'id="conn-workbench-mode"',
  'id="conn-form-feilian"',
  'id="conn-form-llm-set"',
].forEach(k => { if (!html.includes(k)) failures.push(`missing ${k}`); });
```

- [ ] **Step 2: Run the check to verify it fails**

```bash
node work/ui_connection_center_check.js
```

- [ ] **Step 3: Implement the workbench shell**

Add to `index.html` right panel:

```html
<div class="result-head">
  <div>
    <div class="section-title" id="conn-workbench-type">飞连_API</div>
    <div class="subtitle" id="conn-workbench-mode">查看态</div>
  </div>
  <div class="panel-actions">
    <button class="btn" id="btn-conn-create">新增飞连_API</button>
    <button class="btn primary" id="btn-llm-set-create">新增 LLM_API</button>
  </div>
</div>
<div id="conn-form-feilian"></div>
<div id="conn-form-llm-set" class="hidden"></div>
<div class="result-shell"><pre id="conn-out" class="pre"></pre></div>
<div class="result-shell"><pre id="llm-out" class="pre"></pre></div>
```

- [ ] **Step 4: Implement fill/open/create helpers**

In JS:
- `openConnectionRecord(item)`
- `openConnectionCreateMode()`
- `renderConnectionWorkbench()`

Reuse existing connection payload extraction and test/save handlers where possible.

- [ ] **Step 5: Run syntax + checks**

```bash
node --check internal/web/assets/ui/app.js
node work/ui_connection_center_check.js
```

- [ ] **Step 6: Commit**

```bash
git add internal/web/assets/ui/index.html internal/web/assets/ui/app.js work/ui_connection_center_check.js
git commit -m "feat(connection): add unified workbench for feilian api configs"
```

---

## Task 9: Build the unified right workbench for `LLM_API` config set view/edit/create

**Files:**
- Modify: `internal/web/assets/ui/index.html`
- Modify: `internal/web/assets/ui/app.js`

- [ ] **Step 1: Write failing contract**

Require JS helpers:

```js
[
  'function openLLMConfigSetRecord',
  'function openLLMConfigSetCreateMode',
  'function getLLMConfigSetPayload',
].forEach(k => { if (!js.includes(k)) failures.push(`missing ${k}`); });
```

- [ ] **Step 2: Run the check to verify it fails**

```bash
node work/ui_connection_center_check.js
```

- [ ] **Step 3: Implement config-set payload helpers**

Define:

```js
function getLLMConfigSetPayload() {
  return {
    id: qs('#llm-set-id').value.trim(),
    name: qs('#llm-set-name').value.trim(),
    activate: qs('#llm-set-activate')?.checked === true,
    planner: getLLMRolePayload('planner'),
    formatter: getLLMRolePayload('formatter'),
  };
}
```

`openLLMConfigSetRecord(item)` fills:
- `llm-set-id`
- `llm-set-name`
- planner/formatter sub-form

- [ ] **Step 4: Implement save and delete flows**

Save:

```js
await fetchJSON('/api/v1/settings/llm/config-sets', {
  method: 'POST',
  body: JSON.stringify(getLLMConfigSetPayload())
});
```

Delete selected set:

```js
await fetchJSON(`/api/v1/settings/llm/config-sets/${encodeURIComponent(id)}`, { method: 'DELETE' });
```

Test Planner / Formatter continue using the existing `/api/v1/settings/llm/test`.

- [ ] **Step 5: Run syntax + checks**

```bash
node --check internal/web/assets/ui/app.js
node work/ui_connection_center_check.js
```

- [ ] **Step 6: Commit**

```bash
git add internal/web/assets/ui/index.html internal/web/assets/ui/app.js work/ui_connection_center_check.js
git commit -m "feat(connection): add llm api config set workbench"
```

---

## Task 10: Remove legacy stacked forms and verify no dead bindings remain

**Files:**
- Modify: `internal/web/assets/ui/index.html`
- Modify: `internal/web/assets/ui/app.js`
- Modify: `work/ui_binding_check.js`
- Modify: `work/ui_connection_center_check.js`

- [ ] **Step 1: Remove old stacked sections**

Delete the old:
- standalone `飞连连接配置` large card
- standalone `LLM 双模型角色配置` large card
- bottom `最近启用（最多 3 条）` table

All of these are now superseded by the connection center layout.

- [ ] **Step 2: Make old render helpers no-op or remove them**

Ensure no dead bindings remain for:
- `conn-table`
- `btn-conn-load` in old position
- `btn-llm-load` old-only assumptions

Preferred minimal change: keep button ids if they’re reused in the new workbench; remove only the unused legacy containers.

- [ ] **Step 3: Run full checks**

```bash
node --check internal/web/assets/ui/app.js
node work/ui_binding_check.js
node work/ui_connection_center_check.js
```

Expected:
- no missing id bindings
- no references to removed legacy containers

- [ ] **Step 4: Commit**

```bash
git add internal/web/assets/ui/index.html internal/web/assets/ui/app.js work/ui_binding_check.js work/ui_connection_center_check.js
git commit -m "refactor(connection): replace legacy connection page with api center"
```

---

## Self-Review Checklist

- [ ] The plan covers both UI re-layout and backend data model upgrades for `LLM_API`.
- [ ] `LLM_API` is consistently treated as a config set (planner + formatter together).
- [ ] ACTIVE deletion protections are covered for both 飞连_API and LLM_API.
- [ ] The output window remains part of the right workbench and existing test endpoints are reused.
- [ ] There are no TODO/TBD placeholders in any task.

---

## Execution Handoff

Plan complete and saved to `docs/superpowers/plans/2026-06-06-connection-settings-api-center.md`.

Two execution options:

**1. Subagent-Driven (recommended)** - I dispatch a fresh subagent per task, review between tasks, fast iteration

**2. Inline Execution** - Execute tasks in this session using executing-plans, batch execution with checkpoints

Which approach?

