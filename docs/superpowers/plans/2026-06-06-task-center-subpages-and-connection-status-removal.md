# 任务中心子页面改版与连接配置状态窗移除 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 删除 `连接配置` 页面中间状态窗，将 `飞连任务列表` 从 Tabs 结构改成“连接配置式子页面切换”，并把 `单次任务清单` 升级为 `单次/周期任务清单`，新增周期执行、任务次数和结束时间配置。

**Architecture:** 主要在前端 `index.html + app.js + styles.css` 完成这轮信息架构重排。保留当前一级页面 `飞连任务列表`，但将页内 Tabs 切换改成与 `连接配置` 相同的子页面切换条；单次任务工作台继续沿用现有表单结构，只在基础信息区域新增周期执行相关字段，并以最小方式映射到任务草稿 payload。

**Tech Stack:** Vanilla JS、HTML、CSS、既有 REST API、Node.js 契约检查、Go test（如环境具备）

---

## 文件结构

### 主要前端文件

- Modify: `internal/web/assets/ui/index.html`
  - 删除 `连接配置` 中间状态窗
  - 将 `飞连任务列表` 的 Tabs 改为连接配置式子页面切换条
  - 把 `单次任务清单` 文案与 UI 升级为 `单次/周期任务清单`
  - 新增 `周期执行 / 任务次数 / 结束时间` 字段容器
- Modify: `internal/web/assets/ui/app.js`
  - 移除旧的 `task-center-tab-once / task-center-tab-schedule` 切换逻辑
  - 增加与 `连接配置` 同风格的任务子页面切换函数
  - 让单次任务 payload 带上周期执行、任务次数、结束时间
  - 控制周期字段在“仅本次”与其他模式下的显示/启用关系
- Modify: `internal/web/assets/ui/styles.css`
  - 调整连接配置页面首屏密度
  - 将任务中心切换条样式对齐到 `连接配置` 的 `.seg`
  - 增加周期执行配置区域的表单布局样式

### 后端与测试文件

- Modify: `internal/web/server_test.go`
  - 如后端已有任务草稿保存测试，则补一条周期字段持久化用例
- Modify: `internal/storage/task_drafts_store.go`
  - 如需要为任务草稿增加新字段，则在这里最小扩展

### 轻量前端契约检查

- Create: `work/ui_task_center_subpages_contract_check.js`
  - 检查连接配置状态窗移除、任务中心切换样式变更、周期字段挂载

---

### Task 1: 删除连接配置中间状态窗，并保持子页面切换与左右主体布局

**Files:**
- Modify: `internal/web/assets/ui/index.html`
- Modify: `internal/web/assets/ui/styles.css`
- Create: `work/ui_task_center_subpages_contract_check.js`

- [ ] **Step 1: 先写契约检查脚本**

创建 `work/ui_task_center_subpages_contract_check.js`：

```js
const fs = require('fs');

const html = fs.readFileSync('internal/web/assets/ui/index.html', 'utf8');
const js = fs.readFileSync('internal/web/assets/ui/app.js', 'utf8');
const css = fs.readFileSync('internal/web/assets/ui/styles.css', 'utf8');
const failures = [];

[
  '飞连API配置',
  'LLM_API配置',
  'webhook配置',
  'id="subpage-feilian"',
  'id="subpage-llm"',
  'id="subpage-webhook"',
].forEach((token) => {
  if (!html.includes(token)) failures.push(`html missing ${token}`);
});

[
  '查看状态窗口',
  '运行节点摘要',
].forEach((token) => {
  if (html.includes(token)) failures.push(`html should not include ${token}`);
});

[
  '.seg',
  '.config-record-list',
  '.connection-form-shell',
].forEach((token) => {
  if (!css.includes(token)) failures.push(`css missing ${token}`);
});

if (failures.length) {
  console.error(failures.join('\n'));
  process.exit(1);
}
console.log('ui task center subpages contract ok');
```

- [ ] **Step 2: 先运行脚本，确认失败**

Run:

```bash
node work/ui_task_center_subpages_contract_check.js
```

Expected:

- FAIL
- 至少会因为 `查看状态窗口` / `运行节点摘要` 仍在 HTML 中而失败

- [ ] **Step 3: 删除连接配置中间状态窗**

在 `internal/web/assets/ui/index.html` 中删掉连接配置中部状态摘要区，保留：

```html
<section id="view-settings" class="view hidden">
  <div class="page-head">...</div>
  <div class="seg-tabs">
    <button class="seg active" id="btn-sub-feilian" type="button">飞连API配置</button>
    <button class="seg" id="btn-sub-llm" type="button">LLM_API配置</button>
    <button class="seg" id="btn-sub-webhook" type="button">webhook配置</button>
  </div>
  <section id="subpage-feilian">...</section>
  <section id="subpage-llm" class="hidden">...</section>
  <section id="subpage-webhook" class="hidden">...</section>
</section>
```

删除中间这类块：

```html
<div class="stats-grid">...</div>
<div class="status-window">...</div>
```

如果当前命名不是这两个类名，以“查看状态窗口 / 运行节点摘要”所在块为准整体移除。

- [ ] **Step 4: 微调首屏样式**

在 `internal/web/assets/ui/styles.css` 中，适当收紧连接配置首屏的间距，避免删掉状态窗后头部显得太空：

```css
#view-settings .page-head {
  margin-bottom: 14px;
}

#view-settings .seg-tabs,
#view-settings .seg-group {
  margin-bottom: 16px;
}
```

- [ ] **Step 5: 回跑契约检查**

Run:

```bash
node work/ui_task_center_subpages_contract_check.js
```

Expected:

- 输出 `ui task center subpages contract ok`

- [ ] **Step 6: 提交这一小步**

```bash
git add internal/web/assets/ui/index.html internal/web/assets/ui/styles.css work/ui_task_center_subpages_contract_check.js
git commit -m "feat: remove connection status window"
```

如果当前目录不是 git 仓库，跳过 commit，但保留变更。

---

### Task 2: 将飞连任务列表从 Tabs 改成连接配置式子页面切换

**Files:**
- Modify: `internal/web/assets/ui/index.html`
- Modify: `internal/web/assets/ui/app.js`
- Modify: `internal/web/assets/ui/styles.css`
- Test: `work/ui_task_center_subpages_contract_check.js`

- [ ] **Step 1: 扩展契约检查，锁定新的任务子页面切换结构**

在 `work/ui_task_center_subpages_contract_check.js` 中追加：

```js
[
  '单次/周期任务清单',
  '定时任务清单',
  'id="btn-task-sub-once-cycle"',
  'id="btn-task-sub-schedule"',
  'id="task-subpage-once-cycle"',
  'id="task-subpage-schedule"',
].forEach((token) => {
  if (!html.includes(token)) failures.push(`task html missing ${token}`);
});

[
  'task-center-tabs',
  'task-center-tab-once',
  'task-center-tab-schedule',
].forEach((token) => {
  if (html.includes(token)) failures.push(`task html should not include ${token}`);
});

[
  'function switchTaskSubpage(',
  "switchTaskSubpage('once-cycle')",
  "switchTaskSubpage('schedule')",
].forEach((token) => {
  if (!js.includes(token)) failures.push(`task js missing ${token}`);
});
```

- [ ] **Step 2: 先运行脚本，确认失败**

Run:

```bash
node work/ui_task_center_subpages_contract_check.js
```

Expected:

- FAIL
- 因为旧 Tabs 结构仍存在

- [ ] **Step 3: 修改 HTML，把任务中心 Tabs 换成子页面切换条**

在 `internal/web/assets/ui/index.html` 中，将：

```html
<div class="task-center-tabs" id="task-center-tabs">
  <button class="btn primary" id="task-center-tab-once" type="button">单次任务清单</button>
  <button class="btn ghost" id="task-center-tab-schedule" type="button">定时任务清单</button>
</div>
<div class="task-center-pane" id="task-center-pane-once">...</div>
<div class="task-center-pane hidden" id="task-center-pane-schedule">...</div>
```

改成：

```html
<div class="seg-tabs" id="task-subpage-switcher">
  <button class="seg active" id="btn-task-sub-once-cycle" type="button">单次/周期任务清单</button>
  <button class="seg" id="btn-task-sub-schedule" type="button">定时任务清单</button>
</div>

<section id="task-subpage-once-cycle">
  <!-- 原单次任务主体 -->
</section>

<section id="task-subpage-schedule" class="hidden">
  <!-- 原定时任务主体 -->
</section>
```

- [ ] **Step 4: 修改 JS，替换旧 Tabs 逻辑**

在 `internal/web/assets/ui/app.js` 中删除或停用：

```js
window.__taskCenterTab = 'once';
function switchTaskCenterTab(tab) { ... }
function openTaskCenter(tab) { ... }
```

改为：

```js
window.__taskSubpage = 'once-cycle';

function switchTaskSubpage(name) {
  const next = name === 'schedule' ? 'schedule' : 'once-cycle';
  window.__taskSubpage = next;
  qs('#task-subpage-once-cycle')?.classList.toggle('hidden', next !== 'once-cycle');
  qs('#task-subpage-schedule')?.classList.toggle('hidden', next !== 'schedule');
  qs('#btn-task-sub-once-cycle')?.classList.toggle('active', next === 'once-cycle');
  qs('#btn-task-sub-schedule')?.classList.toggle('active', next === 'schedule');
}

function openTaskCenterSubpage(name) {
  setView('tasks');
  switchTaskSubpage(name);
}

qs('#btn-task-sub-once-cycle')?.addEventListener('click', () => switchTaskSubpage('once-cycle'));
qs('#btn-task-sub-schedule')?.addEventListener('click', () => switchTaskSubpage('schedule'));
```

并把所有旧跳转：

```js
openTaskCenter('once');
openTaskCenter('schedule');
switchTaskCenterTab('schedule');
```

分别替换为：

```js
openTaskCenterSubpage('once-cycle');
openTaskCenterSubpage('schedule');
switchTaskSubpage('schedule');
```

- [ ] **Step 5: 调整样式，让任务中心切换条对齐连接配置**

在 `internal/web/assets/ui/styles.css` 中移除或废弃：

```css
.task-center-tabs { ... }
```

改为直接复用 `.seg` / `.seg-tabs`，如需差异只补轻量钩子：

```css
#task-subpage-switcher {
  margin-bottom: 16px;
}
```

- [ ] **Step 6: 回跑契约检查**

Run:

```bash
node work/ui_task_center_subpages_contract_check.js
```

Expected:

- 输出 `ui task center subpages contract ok`

- [ ] **Step 7: 提交这一小步**

```bash
git add internal/web/assets/ui/index.html internal/web/assets/ui/app.js internal/web/assets/ui/styles.css work/ui_task_center_subpages_contract_check.js
git commit -m "feat: switch task center to subpages"
```

如果当前目录不是 git 仓库，跳过 commit，但保留变更。

---

### Task 3: 将单次任务清单升级为单次/周期任务清单

**Files:**
- Modify: `internal/web/assets/ui/index.html`
- Modify: `internal/web/assets/ui/app.js`
- Modify: `internal/storage/task_drafts_store.go`
- Modify: `internal/web/server_test.go`
- Test: `work/ui_task_center_subpages_contract_check.js`

- [ ] **Step 1: 扩展契约检查，要求存在新字段**

在 `work/ui_task_center_subpages_contract_check.js` 中追加：

```js
[
  '周期执行',
  '任务次数',
  '结束时间',
  'id="draft-cycle-mode"',
  'id="draft-run-count"',
  'id="draft-run-until"',
  '仅本次',
  '5min',
  '30min',
  '1h',
  '6h',
  '24h',
  '7day',
  '1month',
  '1year',
].forEach((token) => {
  if (!html.includes(token)) failures.push(`draft html missing ${token}`);
});
```

- [ ] **Step 2: 先运行脚本，确认失败**

Run:

```bash
node work/ui_task_center_subpages_contract_check.js
```

Expected:

- FAIL
- 缺少 `draft-cycle-mode / draft-run-count / draft-run-until`

- [ ] **Step 3: 在单次任务工作台新增 3 组字段**

在 `internal/web/assets/ui/index.html` 的单次任务基础信息区增加：

```html
<div class="row">
  <div class="field">
    <label>周期执行</label>
    <select id="draft-cycle-mode">
      <option value="once">仅本次</option>
      <option value="5min">5min</option>
      <option value="30min">30min</option>
      <option value="1h">1h</option>
      <option value="6h">6h</option>
      <option value="24h">24h</option>
      <option value="7day">7day</option>
      <option value="1month">1month</option>
      <option value="1year">1year</option>
    </select>
  </div>
  <div class="field">
    <label>任务次数</label>
    <input id="draft-run-count" type="number" min="1" step="1" value="1" />
  </div>
  <div class="field">
    <label>结束时间</label>
    <input id="draft-run-until" type="date" />
  </div>
</div>
```

- [ ] **Step 4: 在前端 payload 中保存周期字段**

在 `internal/web/assets/ui/app.js` 的 `collectTaskDraftPayload()` 中追加：

```js
function normalizePositiveIntegerInput(value, fallback) {
  const n = Number.parseInt(String(value || '').trim(), 10);
  return Number.isFinite(n) && n > 0 ? n : fallback;
}
```

```js
return {
  ...
  cycle_mode: qs('#draft-cycle-mode')?.value || 'once',
  run_count: normalizePositiveIntegerInput(qs('#draft-run-count')?.value || '1', 1),
  run_until: qs('#draft-run-until')?.value || '',
};
```

在 `loadTaskDraftIntoWorkbench(draft)` 中同步回填：

```js
if (qs('#draft-cycle-mode')) qs('#draft-cycle-mode').value = draft.cycle_mode || 'once';
if (qs('#draft-run-count')) qs('#draft-run-count').value = String(draft.run_count || 1);
if (qs('#draft-run-until')) qs('#draft-run-until').value = draft.run_until || '';
```

并补一个显示控制：

```js
function syncDraftCycleFields() {
  const cycleMode = qs('#draft-cycle-mode')?.value || 'once';
  const countInput = qs('#draft-run-count');
  const untilInput = qs('#draft-run-until');
  const disabled = cycleMode === 'once';
  if (countInput) countInput.disabled = disabled;
  if (untilInput) untilInput.disabled = disabled;
  if (disabled && countInput) countInput.value = '1';
}

qs('#draft-cycle-mode')?.addEventListener('change', syncDraftCycleFields);
```

- [ ] **Step 5: 最小扩展任务草稿结构与测试**

在 `internal/storage/task_drafts_store.go` 的 `TaskDraft` 结构中加：

```go
CycleMode string `yaml:"cycle_mode" json:"cycle_mode"`
RunCount  int    `yaml:"run_count" json:"run_count"`
RunUntil  string `yaml:"run_until" json:"run_until"`
```

并在归一化里保证：

```go
if strings.TrimSpace(item.CycleMode) == "" {
	item.CycleMode = "once"
}
if item.RunCount <= 0 {
	item.RunCount = 1
}
```

在 `internal/web/server_test.go` 增加一条保存任务草稿的测试，断言：

- `cycle_mode`
- `run_count`
- `run_until`

可持久化并通过接口返回。

- [ ] **Step 6: 回跑契约检查**

Run:

```bash
node work/ui_task_center_subpages_contract_check.js
```

Expected:

- 输出 `ui task center subpages contract ok`

- [ ] **Step 7: 提交这一小步**

```bash
git add internal/web/assets/ui/index.html internal/web/assets/ui/app.js internal/storage/task_drafts_store.go internal/web/server_test.go work/ui_task_center_subpages_contract_check.js
git commit -m "feat: add cycle options to draft workbench"
```

如果当前目录不是 git 仓库，跳过 commit，但保留变更。

---

### Task 4: 统一跳转、文案和“定时任务清单”页面壳

**Files:**
- Modify: `internal/web/assets/ui/index.html`
- Modify: `internal/web/assets/ui/app.js`
- Modify: `internal/web/assets/ui/styles.css`
- Test: `work/ui_task_center_subpages_contract_check.js`

- [ ] **Step 1: 为文案与跳转增加契约检查**

扩展 `work/ui_task_center_subpages_contract_check.js`：

```js
[
  '单次/周期任务清单',
  '定时任务清单',
  '返回飞连任务列表 · 定时任务清单',
].forEach((token) => {
  if (!(html + '\n' + js).includes(token)) failures.push(`navigation missing ${token}`);
});

[
  '单次任务清单',
].forEach((token) => {
  if (html.includes(token)) failures.push(`html should not include legacy label ${token}`);
});
```

- [ ] **Step 2: 先运行脚本，确认遗留旧文案**

Run:

```bash
node work/ui_task_center_subpages_contract_check.js
```

Expected:

- 若还残留 `单次任务清单` 则失败

- [ ] **Step 3: 替换任务中心旧文案**

在 `index.html` / `app.js` 中统一将：

- `单次任务清单`

改为：

- `单次/周期任务清单`

特别是：

- 页面标题
- 说明文案
- 入口按钮
- toast / 辅助说明

- [ ] **Step 4: 统一跳转逻辑**

确保所有进入任务中心默认页的入口都改为：

```js
openTaskCenterSubpage('once-cycle');
```

进入调度管理则改为：

```js
openTaskCenterSubpage('schedule');
```

例如：

- `btn-open-api`
- `btn-open-jobs`
- “创建周期任务”
- `btn-job-editor-back`

- [ ] **Step 5: 回跑契约检查**

Run:

```bash
node work/ui_task_center_subpages_contract_check.js
```

Expected:

- 输出 `ui task center subpages contract ok`

- [ ] **Step 6: 提交这一小步**

```bash
git add internal/web/assets/ui/index.html internal/web/assets/ui/app.js internal/web/assets/ui/styles.css work/ui_task_center_subpages_contract_check.js
git commit -m "feat: align task center subpage labels"
```

如果当前目录不是 git 仓库，跳过 commit，但保留变更。

---

### Task 5: 整体回归与文档说明补充

**Files:**
- Modify: `docs/superpowers/specs/2026-06-06-task-center-subpages-and-connection-status-removal-design.md`
- Test: `work/ui_task_center_subpages_contract_check.js`

- [ ] **Step 1: 在 spec 中补一个字段映射说明**

在 `docs/superpowers/specs/2026-06-06-task-center-subpages-and-connection-status-removal-design.md` 末尾追加：

```md
## 14. 周期执行字段映射建议

- `周期执行` → `cycle_mode`
- `任务次数` → `run_count`
- `结束时间` → `run_until`
```

- [ ] **Step 2: 运行前端契约检查**

Run:

```bash
node work/ui_task_center_subpages_contract_check.js
```

Expected:

- 输出 `ui task center subpages contract ok`

- [ ] **Step 3: 如环境具备，补一次后端测试**

Run:

```bash
go test ./internal/web -count=1
```

Expected:

- PASS

如果环境中没有 Go，则在本次说明中记录：

```md
请在具备 Go 工具链的环境补跑：go test ./internal/web -count=1
```

- [ ] **Step 4: 最终提交**

```bash
git add internal/web/assets/ui/index.html internal/web/assets/ui/app.js internal/web/assets/ui/styles.css internal/storage/task_drafts_store.go internal/web/server_test.go work/ui_task_center_subpages_contract_check.js docs/superpowers/specs/2026-06-06-task-center-subpages-and-connection-status-removal-design.md
git commit -m "feat: add task center subpages and cycle config"
```

如果当前目录不是 git 仓库，跳过 commit，但保留变更。

---

## Self-Review

### Spec coverage

- 删除连接配置状态窗：Task 1
- 任务中心从 Tabs 改成子页面切换：Task 2
- `单次/周期任务清单` 新增周期字段：Task 3
- 文案与跳转统一：Task 4
- 字段映射说明与回归：Task 5

无明显遗漏。

### Placeholder scan

- 计划中没有 `TODO` / `TBD`
- 每一步都写了具体文件、代码骨架、命令和预期结果

### Type consistency

- 统一使用：
  - `once-cycle`
  - `schedule`
- 周期字段统一使用：
  - `cycle_mode`
  - `run_count`
  - `run_until`

---
