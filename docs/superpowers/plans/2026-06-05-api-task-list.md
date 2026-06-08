# API Task List Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 将“飞连API任务”页中的“模板入口”替换为真实的“任务列表”，展示已保存的 `task-drafts`，并支持测试、创建周期任务、编辑、删除。

**Architecture:** 保持现有单页 `setView()` 结构不变，重用 `task-drafts` 与 `job-schedules` 已有接口和前端缓存。左侧区域由“模板入口卡片”切换为“任务列表面板”，右侧继续保留任务定义工作台与执行输出；“创建周期任务”通过预填状态跳转到 `jobs` 视图的新建调度流程。

**Tech Stack:** Go（chi 路由 + embed 静态资源）、Vanilla JS、HTML/CSS、Node 脚本校验。

---

## 0. Files & Responsibilities

**前端主实现**
- Modify: `internal/web/assets/ui/index.html`
  - 将 `view-api` 左侧“模板入口”区域替换为“任务列表”容器
  - 新增任务列表搜索框、刷新按钮、列表容器和空态文案
- Modify: `internal/web/assets/ui/app.js`
  - 渲染 `task-drafts` 左侧列表
  - 新增行内动作：测试、创建周期任务、编辑、删除
  - 复用并收敛现有 `refreshTaskDrafts()` / `loadTaskDraftIntoWorkbench()` / 删除逻辑
  - 新增“创建周期任务”跳转预填状态
- Modify: `internal/web/assets/ui/styles.css`
  - 微调 `view-api` 左右栏布局，让左侧任务列表更像管理侧栏而不是提示卡
  - 增加列表项、行内动作、空态与选中态样式

**镜像目录（如仍参与开发预览）**
- Modify: `web/ui/index.html`
- Modify: `web/ui/app.js`
- Modify: `web/ui/styles.css`

**校验脚本**
- Modify: `work/ui_binding_check.js`
  - 校验新增按钮/输入框 ID 存在
- Create/Modify: `work/ui_api_task_list_check.js`
  - 校验 `view-api` 中不再有 `模板入口`
  - 校验存在 `任务列表` 所需关键 DOM
  - 校验 JS 中存在“创建周期任务预填”相关函数

---

## Task 1: Replace “模板入口” with “任务列表” skeleton

**Files:**
- Modify: `internal/web/assets/ui/index.html`
- Modify: `web/ui/index.html`
- Create: `work/ui_api_task_list_check.js`

- [ ] **Step 1: Write the failing DOM contract check**

Create `work/ui_api_task_list_check.js` with assertions for the new API page skeleton:

```js
const fs = require('fs');
const html = fs.readFileSync('internal/web/assets/ui/index.html', 'utf8');
const failures = [];
if (html.includes('模板入口')) failures.push('view-api should not show 模板入口 card');
[
  'id="api-task-list-card"',
  'id="api-task-filter"',
  'id="btn-api-tasks-refresh"',
  'id="api-task-list"',
  'id="btn-api-task-new-schedule"',
].forEach(k => { if (!html.includes(k)) failures.push(`missing ${k}`); });
if (failures.length) { console.error(failures.join('\\n')); process.exit(1); }
console.log('api task list layout ok');
```

- [ ] **Step 2: Run the check to verify it fails**

Run:

```bash
node work/ui_api_task_list_check.js
```

Expected: FAIL, because `模板入口` still exists and new DOM is missing.

- [ ] **Step 3: Implement minimal HTML skeleton**

In `internal/web/assets/ui/index.html`, replace the `模板入口` card inside `#view-api` with a real task list card:

```html
<div class="card" id="api-task-list-card">
  <div class="panel-head">
    <div>
      <div class="panel-title">任务列表</div>
      <div class="subtitle">展示已保存的飞连API任务草稿，可直接测试、创建周期任务、编辑或删除。</div>
    </div>
    <div class="panel-actions">
      <button class="btn" id="btn-api-tasks-refresh">刷新</button>
    </div>
  </div>
  <div class="field">
    <label>搜索任务</label>
    <input id="api-task-filter" placeholder="按 id / 名称 / template_id 过滤" />
  </div>
  <div id="api-task-list" class="subtitle">暂无任务草稿</div>
</div>
```

Also remove the old “模板入口” hint block and button.

Mirror the same structure into `web/ui/index.html` if it still powers preview flows.

- [ ] **Step 4: Run the check to verify it passes**

```bash
node work/ui_api_task_list_check.js
```

Expected: PASS with `api task list layout ok`.

- [ ] **Step 5: Commit**

```bash
git add internal/web/assets/ui/index.html web/ui/index.html work/ui_api_task_list_check.js
git commit -m "feat(ui): replace template entry with api task list skeleton"
```

---

## Task 2: Restyle the left panel as a task management list

**Files:**
- Modify: `internal/web/assets/ui/styles.css`
- Modify: `web/ui/styles.css`
- Modify: `work/ui_api_task_list_check.js`

- [ ] **Step 1: Extend the check to require CSS markers**

Add CSS checks:

```js
const css = fs.readFileSync('internal/web/assets/ui/styles.css', 'utf8');
['.api-task-list', '.api-task-item', '.api-task-item-active'].forEach(k => {
  if (!css.includes(k)) throw new Error(`missing css ${k}`);
});
```

- [ ] **Step 2: Run the check and verify it fails**

```bash
node work/ui_api_task_list_check.js
```

- [ ] **Step 3: Implement minimal styles**

Append styles in `internal/web/assets/ui/styles.css`:

```css
.api-task-list{
  display: flex;
  flex-direction: column;
  gap: 10px;
  margin-top: 12px;
}
.api-task-item{
  border: 1px solid rgba(140,161,201,.18);
  border-radius: 16px;
  padding: 14px;
  background: linear-gradient(180deg, rgba(255,255,255,.96), rgba(247,250,255,.94));
}
.api-task-item-active{
  border-color: rgba(47,107,255,.28);
  box-shadow: 0 12px 24px rgba(47,107,255,.08);
}
```

Add a compact action row style:

```css
.api-task-actions{
  display: flex;
  gap: 8px;
  flex-wrap: wrap;
  margin-top: 10px;
}
```

Mirror equivalent styles to `web/ui/styles.css`.

- [ ] **Step 4: Run the check to verify it passes**

```bash
node work/ui_api_task_list_check.js
```

- [ ] **Step 5: Commit**

```bash
git add internal/web/assets/ui/styles.css web/ui/styles.css work/ui_api_task_list_check.js
git commit -m "feat(ui): style api task list panel"
```

---

## Task 3: Render the task list from `task-drafts`

**Files:**
- Modify: `internal/web/assets/ui/app.js`
- Modify: `web/ui/app.js`
- Modify: `work/ui_binding_check.js`

- [ ] **Step 1: Write the failing behavior contract**

Extend `work/ui_api_task_list_check.js` to require:

```js
const js = fs.readFileSync('internal/web/assets/ui/app.js', 'utf8');
[
  'function renderAPITaskList',
  "qs('#btn-api-tasks-refresh')",
  "qs('#api-task-filter')",
  "qs('#api-task-list')",
].forEach(k => { if (!js.includes(k)) throw new Error(`missing ${k}`); });
```

- [ ] **Step 2: Run the check and verify it fails**

```bash
node work/ui_api_task_list_check.js
```

- [ ] **Step 3: Implement `renderAPITaskList()`**

In `internal/web/assets/ui/app.js`, add a dedicated renderer using the existing cache:

```js
function renderAPITaskList() {
  const host = qs('#api-task-list');
  if (!host) return;
  const filter = (qs('#api-task-filter')?.value || '').trim().toLowerCase();
  const items = (window.__taskDraftCache || []).filter(d => {
    const hay = `${d.id || ''} ${d.name || ''} ${d.source_template_id || (d.input_config || {}).template_id || ''}`.toLowerCase();
    return !filter || hay.includes(filter);
  });
  if (!items.length) {
    host.innerHTML = '<div class="subtitle">暂无任务草稿，请先在右侧保存第一条任务。</div>';
    return;
  }
  host.className = 'api-task-list';
  host.innerHTML = items.map(d => `
    <div class="api-task-item ${window.__currentDraftID === d.id ? 'api-task-item-active' : ''}" data-id="${escapeHtml(d.id || '')}">
      <div style="font-weight:750">${escapeHtml(d.name || d.id || '')}</div>
      <div class="subtitle"><code>${escapeHtml(d.id || '')}</code> · mode=${escapeHtml(d.mode || '')}</div>
      <div class="subtitle">template=${escapeHtml(d.source_template_id || (d.input_config || {}).template_id || '')}</div>
      <div class="api-task-actions">
        <button class="btn" data-act="test" data-id="${escapeHtml(d.id || '')}">测试</button>
        <button class="btn" data-act="schedule" data-id="${escapeHtml(d.id || '')}">创建周期任务</button>
        <button class="btn" data-act="edit" data-id="${escapeHtml(d.id || '')}">编辑</button>
        <button class="btn danger" data-act="delete" data-id="${escapeHtml(d.id || '')}">删除</button>
      </div>
    </div>
  `).join('');
}
```

- [ ] **Step 4: Wire refresh and filter**

Bind:

```js
qs('#btn-api-tasks-refresh')?.addEventListener('click', () => refreshTaskDrafts(false));
qs('#api-task-filter')?.addEventListener('input', () => renderAPITaskList());
```

Update `refreshTaskDrafts()` so it calls both:

```js
renderTaskDraftList();
renderAPITaskList();
```

Mirror the same behavior to `web/ui/app.js` if that mirror is still used.

- [ ] **Step 5: Run syntax + checks**

```bash
node --check internal/web/assets/ui/app.js
node --check web/ui/app.js
node work/ui_binding_check.js
node work/ui_api_task_list_check.js
```

- [ ] **Step 6: Commit**

```bash
git add internal/web/assets/ui/app.js web/ui/app.js work/ui_binding_check.js work/ui_api_task_list_check.js
git commit -m "feat(ui): render api task list from task drafts"
```

---

## Task 4: Hook up “编辑” and “删除” on the task list

**Files:**
- Modify: `internal/web/assets/ui/app.js`
- Modify: `web/ui/app.js`

- [ ] **Step 1: Add failing contract**

Extend the JS check to require:

```js
[
  "data-act=\"edit\"",
  "data-act=\"delete\"",
  "loadTaskDraftIntoWorkbench",
].forEach(k => { if (!js.includes(k)) throw new Error(`missing ${k}`); });
```

- [ ] **Step 2: Run check to verify it fails or is incomplete**

```bash
node work/ui_api_task_list_check.js
```

- [ ] **Step 3: Implement edit/delete handlers**

Add to `renderAPITaskList()` after `host.innerHTML = ...`:

```js
qsa('#api-task-list [data-act="edit"]').forEach(btn => btn.addEventListener('click', () => {
  const id = btn.dataset.id || '';
  const draft = (window.__taskDraftCache || []).find(it => it.id === id);
  if (!draft) return;
  loadTaskDraftIntoWorkbench(draft);
  renderAPITaskList();
  toast('任务列表', `已加载 ${id}`, 'ok');
}));

qsa('#api-task-list [data-act="delete"]').forEach(btn => btn.addEventListener('click', async () => {
  const id = btn.dataset.id || '';
  if (!confirm(`确认删除任务草稿 ${id}？`)) return;
  try {
    await fetchJSON(`/api/v1/task-drafts/${encodeURIComponent(id)}`, { method: 'DELETE' });
    if (window.__currentDraftID === id) {
      window.__currentDraftID = '';
      qs('#draft-id').value = '';
      qs('#draft-name').value = '';
    }
    await refreshTaskDrafts(true);
    toast('删除成功', id, 'ok');
  } catch (e) {
    toast('删除失败', String(e), '');
  }
}));
```

- [ ] **Step 4: Run syntax + checks**

```bash
node --check internal/web/assets/ui/app.js
node work/ui_api_task_list_check.js
```

- [ ] **Step 5: Commit**

```bash
git add internal/web/assets/ui/app.js web/ui/app.js work/ui_api_task_list_check.js
git commit -m "feat(ui): add edit and delete actions to api task list"
```

---

## Task 5: Add “测试” from the task list into the existing execution output

**Files:**
- Modify: `internal/web/assets/ui/app.js`
- Modify: `web/ui/app.js`

- [ ] **Step 1: Write the failing contract**

Require a helper for executing a draft:

```js
if (!js.includes('function runTaskDraftFromList')) throw new Error('missing runTaskDraftFromList');
```

- [ ] **Step 2: Run the check to verify it fails**

```bash
node work/ui_api_task_list_check.js
```

- [ ] **Step 3: Implement the helper**

In `internal/web/assets/ui/app.js`, add:

```js
async function runTaskDraftFromList(id) {
  const draft = (window.__taskDraftCache || []).find(it => it.id === id);
  if (!draft) return;
  loadTaskDraftIntoWorkbench(draft);
  const payload = {
    template_id: draft.source_template_id || (draft.input_config || {}).template_id || '',
    method: (draft.input_config || {}).method || '',
    path: (draft.input_config || {}).path || '',
    query: (draft.input_config || {}).query || {},
    path_params: (draft.input_config || {}).path_params || {},
    body: (draft.input_config || {}).body || {}
  };
  const data = await fetchJSON('/api/v1/api/execute', { method: 'POST', body: JSON.stringify(payload) });
  qs('#api-out').textContent = pretty(data);
  focusApiOut();
}
```

Then bind it:

```js
qsa('#api-task-list [data-act="test"]').forEach(btn => btn.addEventListener('click', async () => {
  try {
    await runTaskDraftFromList(btn.dataset.id || '');
    toast('任务列表', '任务测试完成', 'ok');
  } catch (e) {
    renderAPIError('任务测试失败', e);
    toast('任务测试失败', String(e), '');
  }
}));
```

- [ ] **Step 4: Verify syntax + checks**

```bash
node --check internal/web/assets/ui/app.js
node work/ui_api_task_list_check.js
```

- [ ] **Step 5: Commit**

```bash
git add internal/web/assets/ui/app.js web/ui/app.js work/ui_api_task_list_check.js
git commit -m "feat(ui): add test action for api task list"
```

---

## Task 6: Add “创建周期任务” with prefilled job editor state

**Files:**
- Modify: `internal/web/assets/ui/app.js`
- Modify: `web/ui/app.js` (only if jobs editor exists there; otherwise skip mirror and document why)

- [ ] **Step 1: Write the failing contract**

Require the prefill helper:

```js
if (!js.includes('function createScheduleFromTaskDraft')) throw new Error('missing createScheduleFromTaskDraft');
```

- [ ] **Step 2: Run the check to verify it fails**

```bash
node work/ui_api_task_list_check.js
```

- [ ] **Step 3: Implement prefill behavior**

Use a temporary window state:

```js
window.__pendingJobDraft = null;

function createScheduleFromTaskDraft(id) {
  const draft = (window.__taskDraftCache || []).find(it => it.id === id);
  if (!draft) return;
  window.__pendingJobDraft = {
    id: `job_${id}`,
    target_type: 'task_draft',
    target_id: draft.id,
    enabled: false
  };
  setView('jobs');
  if (typeof openJobEditorFromTaskDraft === 'function') {
    openJobEditorFromTaskDraft(window.__pendingJobDraft, draft);
  } else if (typeof openJobDrawer === 'function') {
    openJobDrawer(window.__pendingJobDraft);
  }
}
```

Then bind the action in the list:

```js
qsa('#api-task-list [data-act="schedule"]').forEach(btn => btn.addEventListener('click', () => {
  createScheduleFromTaskDraft(btn.dataset.id || '');
}));
```

Also update the jobs editor initialization path so that when `window.__pendingJobDraft` exists it pre-fills:
- `target_type`
- `target_id`
- optional display name based on draft

- [ ] **Step 4: Run syntax + checks**

```bash
node --check internal/web/assets/ui/app.js
node work/ui_api_task_list_check.js
```

- [ ] **Step 5: Commit**

```bash
git add internal/web/assets/ui/app.js work/ui_api_task_list_check.js
git commit -m "feat(ui): create job schedule from api task draft"
```

---

## Task 7: Remove redundancy and make the API page feel intentional

**Files:**
- Modify: `internal/web/assets/ui/index.html`
- Modify: `internal/web/assets/ui/app.js`
- Modify: `internal/web/assets/ui/styles.css`

- [ ] **Step 1: Review `view-api` for duplicate “saved drafts” UI**

The current page still has:
- left-side new task list (new feature)
- bottom “已保存任务草稿” block (old UI)

For clarity, reduce redundancy by either:
- removing the bottom saved-drafts card, or
- changing it into a compact “最近保存结果/帮助提示” block

Preferred minimal change:

```html
<!-- remove old task-draft-list block from bottom of view-api -->
```

- [ ] **Step 2: Update JS to avoid rendering into removed legacy container**

If removing `#task-draft-list`, then:
- change `refreshTaskDrafts()` so it no longer depends on `renderTaskDraftList()`, or
- keep `renderTaskDraftList()` as a no-op when container absent

Verify no old buttons remain:
- `btn-drafts-refresh`

- [ ] **Step 3: Run syntax + checks**

```bash
node --check internal/web/assets/ui/app.js
node work/ui_binding_check.js
node work/ui_api_task_list_check.js
```

- [ ] **Step 4: Commit**

```bash
git add internal/web/assets/ui/index.html internal/web/assets/ui/app.js internal/web/assets/ui/styles.css work/ui_binding_check.js work/ui_api_task_list_check.js
git commit -m "refactor(ui): streamline api page around task list and workbench"
```

---

## Self-Review Checklist

- [ ] The plan fully covers the approved spec: task list replaces template entry, uses task-drafts, supports test/schedule/edit/delete, and keeps right-side workbench.
- [ ] No placeholders like TBD/TODO remain.
- [ ] Function names are consistent across tasks (`renderAPITaskList`, `runTaskDraftFromList`, `createScheduleFromTaskDraft`).
- [ ] The jobs prefill path is explicit enough for an implementer to wire to the existing job editor.

---

## Execution Handoff

Plan complete and saved to `docs/superpowers/plans/2026-06-05-api-task-list.md`.

Two execution options:

**1. Subagent-Driven (recommended)** - I dispatch a fresh subagent per task, review between tasks, fast iteration

**2. Inline Execution** - Execute tasks in this session using executing-plans, batch execution with checkpoints

Which approach?

