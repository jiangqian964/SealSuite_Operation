# 飞连API列表 / 飞连任务列表 UI 改版 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 把 `飞连API列表` 改成左列表右上下详情/输出结构，把 `飞连API任务` 与 `飞连周期任务` 合并为 `飞连任务列表`，并在页内用 Tabs 承载 `单次任务清单 / 定时任务清单`，同时修复单次任务列表只显示最近一条的 bug，并将定时配置表单改成更业务化的时间选择方式。

**Architecture:** 前端以 `index.html + app.js + styles.css` 为主完成页面结构重排，保持已有 API 与大部分后端存储模型不变。调度时间改版优先做 UI 层映射：把“每日定时”与“固定间隔”包装成更友好的控件，保存时再转换回现有 `schedule_type / cron / interval / timezone` 结构，以避免后端链路大改。

**Tech Stack:** Vanilla JS、HTML、CSS、既有 REST API、Node.js 前端契约检查、Go test（如环境具备）

---

## 文件结构

### 主要前端文件

- Modify: `internal/web/assets/ui/index.html`
  - 调整一级导航文案
  - 新增 `飞连任务列表` 容器与顶部 Tabs
  - 重组 `飞连API列表` 页面为左列表右上下结构
  - 删除独立 `飞连周期任务` 一级页面入口
- Modify: `internal/web/assets/ui/app.js`
  - 调整 view 切换逻辑
  - 合并 `api` / `jobs` 页面入口为 `tasks`
  - 修复单次任务列表只显示最近一条的问题
  - 增加任务中心 Tabs 切换
  - 改造调度编辑器表单，新增每日定时与固定间隔映射逻辑
- Modify: `internal/web/assets/ui/styles.css`
  - 新增任务中心 Tabs、双栏与上下分区布局样式
  - 新增时间选择器行内样式与响应式规则

### 后端与测试文件

- Modify: `internal/web/server_test.go`
  - 如有必要，补充任务草稿列表完整返回 / 调度保存映射相关测试
- Modify: `internal/storage/job_schedules_store.go`
  - 如果需要增加轻量归一化辅助函数，可在这里补

### 轻量前端契约检查

- Create: `work/ui_task_center_relayout_contract_check.js`
  - 静态检查导航、Tabs、新的布局容器、定时表单控件 token

---

### Task 1: 重组导航与页面容器

**Files:**
- Modify: `internal/web/assets/ui/index.html`
- Modify: `internal/web/assets/ui/app.js`
- Modify: `internal/web/assets/ui/styles.css`
- Test: `work/ui_task_center_relayout_contract_check.js`

- [ ] **Step 1: 先写前端契约检查脚本**

创建 `work/ui_task_center_relayout_contract_check.js`，先锁定新页面结构 token。

```js
const fs = require('fs');

const html = fs.readFileSync('internal/web/assets/ui/index.html', 'utf8');
const js = fs.readFileSync('internal/web/assets/ui/app.js', 'utf8');
const css = fs.readFileSync('internal/web/assets/ui/styles.css', 'utf8');
const failures = [];

[
  '飞连API列表',
  '飞连任务列表',
  '单次任务清单',
  '定时任务清单',
  'id="task-center-tabs"',
  'id="task-center-tab-once"',
  'id="task-center-tab-schedule"',
  'id="view-tasks"',
].forEach((token) => {
  if (!html.includes(token)) failures.push(`missing html token: ${token}`);
});

[
  'function switchTaskCenterTab(',
  "setView('tasks')",
  'window.__taskCenterTab',
].forEach((token) => {
  if (!js.includes(token)) failures.push(`missing js token: ${token}`);
});

[
  '.task-center-tabs',
  '.template-layout',
  '.template-detail-stack',
].forEach((token) => {
  if (!css.includes(token)) failures.push(`missing css token: ${token}`);
});

if (failures.length) {
  console.error(failures.join('\n'));
  process.exit(1);
}
console.log('ui task center relayout contract ok');
```

- [ ] **Step 2: 先运行脚本，确认失败**

Run:

```bash
node work/ui_task_center_relayout_contract_check.js
```

Expected:

- FAIL
- 缺少 `飞连任务列表`、`view-tasks`、Tabs 相关 token

- [ ] **Step 3: 修改 HTML 导航和页面容器**

在 `internal/web/assets/ui/index.html` 中：

1. 把原导航中的：
   - `飞连API任务`
   - `飞连周期任务`

替换为：

```html
<button class="nav-btn" data-view="tasks">飞连任务列表</button>
```

2. 保留原 `view-templates`，但内部改成左右 + 右侧上下结构：

```html
<section id="view-templates" class="view">
  <div class="template-layout">
    <div class="template-list-pane">
      <!-- 原飞连API列表主体 -->
    </div>
    <div class="template-detail-stack">
      <div class="card">
        <div class="section-title">模板详情</div>
        <pre id="tpl-detail"></pre>
      </div>
      <div class="card">
        <div class="section-title">执行输出</div>
        <pre id="tpl-out"></pre>
      </div>
    </div>
  </div>
</section>
```

3. 新增合并后的 `view-tasks`：

```html
<section id="view-tasks" class="view hidden">
  <div class="row" id="task-center-tabs">
    <button class="btn primary" id="task-center-tab-once">单次任务清单</button>
    <button class="btn" id="task-center-tab-schedule">定时任务清单</button>
  </div>
  <div id="task-center-pane-once">
    <!-- 迁入原飞连API任务页面主体 -->
  </div>
  <div id="task-center-pane-schedule" class="hidden">
    <!-- 迁入原飞连周期任务页面主体 -->
  </div>
</section>
```

- [ ] **Step 4: 修改 JS 视图和 Tabs 切换逻辑**

在 `internal/web/assets/ui/app.js` 中增加：

```js
window.__taskCenterTab = 'once';

function switchTaskCenterTab(tab) {
  const next = tab === 'schedule' ? 'schedule' : 'once';
  window.__taskCenterTab = next;
  qs('#task-center-pane-once')?.classList.toggle('hidden', next !== 'once');
  qs('#task-center-pane-schedule')?.classList.toggle('hidden', next !== 'schedule');
  qs('#task-center-tab-once')?.classList.toggle('primary', next === 'once');
  qs('#task-center-tab-once')?.classList.toggle('ghost', next !== 'once');
  qs('#task-center-tab-schedule')?.classList.toggle('primary', next === 'schedule');
  qs('#task-center-tab-schedule')?.classList.toggle('ghost', next !== 'schedule');
}

qs('#task-center-tab-once')?.addEventListener('click', () => switchTaskCenterTab('once'));
qs('#task-center-tab-schedule')?.addEventListener('click', () => switchTaskCenterTab('schedule'));
```

并把所有与任务中心相关的跳转从：

```js
setView('api');
setView('jobs');
```

改成：

```js
setView('tasks');
switchTaskCenterTab('once');
// 或
setView('tasks');
switchTaskCenterTab('schedule');
```

特别是“创建周期任务”动作要直接跳到 `schedule` Tab。

- [ ] **Step 5: 增加样式并回跑契约检查**

在 `internal/web/assets/ui/styles.css` 中新增：

```css
.template-layout {
  display: grid;
  grid-template-columns: minmax(420px, 1.05fr) minmax(360px, 1fr);
  gap: 18px;
}

.template-detail-stack {
  display: grid;
  grid-template-rows: minmax(240px, 1fr) minmax(220px, 1fr);
  gap: 18px;
}

.task-center-tabs {
  display: flex;
  gap: 10px;
  margin-bottom: 16px;
}
```

Run:

```bash
node work/ui_task_center_relayout_contract_check.js
```

Expected:

- 输出 `ui task center relayout contract ok`

- [ ] **Step 6: 提交这一小步**

```bash
git add internal/web/assets/ui/index.html internal/web/assets/ui/app.js internal/web/assets/ui/styles.css work/ui_task_center_relayout_contract_check.js
git commit -m "feat: add task center page containers"
```

如果当前目录不是 git 仓库，跳过 commit，但保留变更。

---

### Task 2: 把单次任务工作台迁入任务中心并修复“只显示最近一条”问题

**Files:**
- Modify: `internal/web/assets/ui/app.js`
- Modify: `internal/web/assets/ui/index.html`
- Test: `work/ui_task_center_relayout_contract_check.js`

- [ ] **Step 1: 先加一个针对任务列表渲染的契约检查**

扩展 `work/ui_task_center_relayout_contract_check.js`，检查单次任务容器和渲染函数 token：

```js
[
  'id="task-center-pane-once"',
  'function renderAPITaskList(',
  'window.__taskDraftCache',
  'drafts.forEach(',
].forEach((token) => {
  if (!(html + '\n' + js).includes(token)) {
    failures.push(`missing once-task token: ${token}`);
  }
});
```

- [ ] **Step 2: 先运行脚本，确认当前可能失败**

Run:

```bash
node work/ui_task_center_relayout_contract_check.js
```

Expected:

- 如果还没迁入新容器，至少会因 `task-center-pane-once` 相关 token 缺失失败

- [ ] **Step 3: 把原飞连API任务页面主体迁入 `task-center-pane-once`**

在 `index.html` 中，把原 `view-api` 里的主体内容复制迁入 `task-center-pane-once`，删除旧的独立 `view-api` 容器，确保保留：

- 左侧任务列表
- 右侧工作台
- 原有按钮和 input id

关键要求：

- 不改已有表单 field id
- 尽量只改外层容器，避免一次性改太多事件绑定

- [ ] **Step 4: 修复单次任务列表只显示最近一条**

在 `internal/web/assets/ui/app.js` 的 `renderAPITaskList()` 中，确认不要用“当前选中草稿”覆盖整体列表，按缓存全量渲染。

目标代码应接近：

```js
function renderAPITaskList() {
  const host = qs('#api-task-list');
  if (!host) return;
  const keyword = (qs('#api-task-search')?.value || '').trim().toLowerCase();
  const drafts = (window.__taskDraftCache || []).filter((draft) => {
    const text = `${draft.id || ''} ${draft.name || ''} ${draft.source_template_id || ''}`.toLowerCase();
    return !keyword || text.includes(keyword);
  });
  host.innerHTML = drafts.map((draft) => renderDraftCard(draft)).join('') || '<div class="subtitle">暂无任务草稿</div>';
}
```

如果现有 bug 源于 `refreshTaskDrafts()` 覆盖了缓存，则修正为：

```js
window.__taskDraftCache = Array.isArray(payload.items) ? payload.items : [];
renderAPITaskList();
```

并避免任何地方把：

```js
window.__taskDraftCache = [currentDraft];
```

之类的逻辑写回缓存。

- [ ] **Step 5: 在“创建周期任务”动作里跳转到 schedule Tab**

将单次任务里的“创建周期任务”入口改为：

```js
setView('tasks');
switchTaskCenterTab('schedule');
openAdvancedJobEditor(draft, { isCreate: true });
```

或对应的 `openJobDrawer(...)` 路径，但必须保证页面先切到 `schedule`。

- [ ] **Step 6: 回跑契约检查**

Run:

```bash
node work/ui_task_center_relayout_contract_check.js
```

Expected:

- 输出 `ui task center relayout contract ok`

- [ ] **Step 7: 提交这一小步**

```bash
git add internal/web/assets/ui/index.html internal/web/assets/ui/app.js work/ui_task_center_relayout_contract_check.js
git commit -m "feat: move api task list into task center"
```

如果当前目录不是 git 仓库，跳过 commit，但保留变更。

---

### Task 3: 把定时任务页面迁入任务中心并保留现有主结构

**Files:**
- Modify: `internal/web/assets/ui/index.html`
- Modify: `internal/web/assets/ui/app.js`
- Modify: `internal/web/assets/ui/styles.css`

- [ ] **Step 1: 先为 schedule Tab 补契约 token**

在 `work/ui_task_center_relayout_contract_check.js` 中追加：

```js
[
  'id="task-center-pane-schedule"',
  'id="jobs-table"',
  'id="job-detail"',
].forEach((token) => {
  if (!html.includes(token)) failures.push(`missing schedule token: ${token}`);
});
```

- [ ] **Step 2: 运行脚本，确认迁移前失败**

Run:

```bash
node work/ui_task_center_relayout_contract_check.js
```

Expected:

- schedule pane token 缺失时报错

- [ ] **Step 3: 迁入原飞连周期任务页面主体**

把旧 `view-jobs` 里的主结构迁入 `task-center-pane-schedule`，保留：

- 搜索区
- jobs table
- detail pane
- 新增调度入口

注意：

- 只删除独立页面壳，不改表格和详情区的已有 id
- 保持原有 `renderJobsTable()`、`loadJobDetail()`、`openJobDrawer()` 的挂点不变

- [ ] **Step 4: 更新返回按钮和导航文案**

将 `job-editor` 页面中的返回按钮从：

```html
返回飞连周期任务
```

改为：

```html
返回飞连任务列表
```

并在点击逻辑中改成：

```js
setView('tasks');
switchTaskCenterTab('schedule');
```

- [ ] **Step 5: 回跑契约检查**

Run:

```bash
node work/ui_task_center_relayout_contract_check.js
```

Expected:

- 输出 `ui task center relayout contract ok`

- [ ] **Step 6: 提交这一小步**

```bash
git add internal/web/assets/ui/index.html internal/web/assets/ui/app.js internal/web/assets/ui/styles.css work/ui_task_center_relayout_contract_check.js
git commit -m "feat: move job schedule page into task center"
```

如果当前目录不是 git 仓库，跳过 commit，但保留变更。

---

### Task 4: 改造快捷调度抽屉，把 `cron / interval / timezone` 包装成业务化控件

**Files:**
- Modify: `internal/web/assets/ui/app.js`
- Modify: `internal/web/assets/ui/index.html`
- Test: `work/ui_task_center_relayout_contract_check.js`

- [ ] **Step 1: 先给调度抽屉加契约检查**

扩展 `work/ui_task_center_relayout_contract_check.js`：

```js
[
  'job-drawer-time-mode',
  'job-drawer-daily-hour',
  'job-drawer-daily-minute',
  'job-drawer-interval-preset',
  '默认使用当前服务器时区',
].forEach((token) => {
  if (!(html + '\n' + js).includes(token)) failures.push(`missing drawer schedule token: ${token}`);
});
```

- [ ] **Step 2: 先运行脚本，确认失败**

Run:

```bash
node work/ui_task_center_relayout_contract_check.js
```

Expected:

- FAIL
- 缺少新的时间模式与时间选择器 token

- [ ] **Step 3: 在 `app.js` 中新增调度映射辅助函数**

先写最小工具函数，用于在 UI 与现有 payload 间转换：

```js
function buildHourOptions(selected) {
  return Array.from({ length: 24 }, (_, i) => {
    const value = String(i).padStart(2, '0');
    return `<option value="${value}" ${value === selected ? 'selected' : ''}>${value}</option>`;
  }).join('');
}

function buildMinuteOptions(selected) {
  return Array.from({ length: 60 }, (_, i) => {
    const value = String(i).padStart(2, '0');
    return `<option value="${value}" ${value === selected ? 'selected' : ''}>${value}</option>`;
  }).join('');
}

function inferScheduleUIState(job) {
  if ((job.schedule_type || '') === 'interval') {
    return { mode: 'interval', intervalPreset: normalizeIntervalPreset(job.interval || '') };
  }
  const cron = String(job.cron || '').trim();
  const parts = cron.split(/\s+/);
  return {
    mode: 'daily',
    minute: parts.length >= 2 ? String(parts[0]).padStart(2, '0') : '00',
    hour: parts.length >= 2 ? String(parts[1]).padStart(2, '0') : '09',
  };
}

function buildSchedulePayloadFromUI(base, ids) {
  const payload = cloneValue(base || {});
  const mode = qs(ids.mode)?.value || 'daily';
  payload.timezone = 'Asia/Shanghai';
  if (mode === 'interval') {
    payload.schedule_type = 'interval';
    payload.interval = qs(ids.interval)?.value || '1h';
    payload.cron = '';
  } else {
    const hour = qs(ids.hour)?.value || '09';
    const minute = qs(ids.minute)?.value || '00';
    payload.schedule_type = 'cron';
    payload.cron = `${Number(minute)} ${Number(hour)} * * *`;
    payload.interval = '';
  }
  return payload;
}
```

- [ ] **Step 4: 替换抽屉中的旧字段**

在 `openJobDrawer(...)` 生成 HTML 时，把：

- `schedule_type`
- `cron`
- `interval`
- `timezone`

替换成：

```js
const uiState = inferScheduleUIState(job);
```

```html
<div class="field">
  <label>任务定时窗口</label>
  <select id="job-drawer-time-mode">
    <option value="daily">每日定时</option>
    <option value="interval">固定间隔</option>
  </select>
  <div class="subtitle">默认使用当前服务器时区</div>
</div>
<div class="row" id="job-drawer-daily-row">
  <div class="field"><label>小时</label><select id="job-drawer-daily-hour">...</select></div>
  <div class="field"><label>分钟</label><select id="job-drawer-daily-minute">...</select></div>
</div>
<div class="row" id="job-drawer-interval-row">
  <div class="field">
    <label>间隔</label>
    <select id="job-drawer-interval-preset">
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
</div>
```

保存时改成：

```js
Object.assign(updated, buildSchedulePayloadFromUI(updated, {
  mode: '#job-drawer-time-mode',
  hour: '#job-drawer-daily-hour',
  minute: '#job-drawer-daily-minute',
  interval: '#job-drawer-interval-preset',
}));
```

- [ ] **Step 5: 回跑契约检查**

Run:

```bash
node work/ui_task_center_relayout_contract_check.js
```

Expected:

- 输出 `ui task center relayout contract ok`

- [ ] **Step 6: 提交这一小步**

```bash
git add internal/web/assets/ui/app.js internal/web/assets/ui/index.html work/ui_task_center_relayout_contract_check.js
git commit -m "feat: add schedule drawer time controls"
```

如果当前目录不是 git 仓库，跳过 commit，但保留变更。

---

### Task 5: 改造高级调度编辑器表单，去掉 `timezone` 暴露并统一 interval 预设

**Files:**
- Modify: `internal/web/assets/ui/app.js`
- Modify: `internal/web/assets/ui/index.html`
- Modify: `internal/web/assets/ui/styles.css`
- Test: `work/ui_task_center_relayout_contract_check.js`

- [ ] **Step 1: 先扩展契约检查**

在 `work/ui_task_center_relayout_contract_check.js` 中追加：

```js
[
  'job-form-time-mode',
  'job-form-daily-hour',
  'job-form-daily-minute',
  'job-form-interval-preset',
  '返回飞连任务列表',
].forEach((token) => {
  if (!(html + '\n' + js).includes(token)) failures.push(`missing advanced schedule token: ${token}`);
});
```

- [ ] **Step 2: 先运行脚本，确认失败**

Run:

```bash
node work/ui_task_center_relayout_contract_check.js
```

Expected:

- FAIL
- 缺少高级编辑器的新 token

- [ ] **Step 3: 在高级编辑器表单中替换旧字段**

参照 Task 4 的 helper，在 `renderAdvancedEditorForm(job)` 中替换：

- `job-form-type`
- `job-form-cron`
- `job-form-interval`
- `job-form-timezone`

改为：

```html
<div class="field">
  <label>任务定时窗口</label>
  <select id="job-form-time-mode">
    <option value="daily">每日定时</option>
    <option value="interval">固定间隔</option>
  </select>
  <div class="subtitle">默认使用当前服务器时区</div>
</div>
<div class="row" id="job-form-daily-row">
  <div class="field"><label>小时</label><select id="job-form-daily-hour">...</select></div>
  <div class="field"><label>分钟</label><select id="job-form-daily-minute">...</select></div>
</div>
<div class="row" id="job-form-interval-row">
  <div class="field"><label>间隔</label><select id="job-form-interval-preset">...</select></div>
</div>
```

保存时改成：

```js
Object.assign(payload, buildSchedulePayloadFromUI(payload, {
  mode: '#job-form-time-mode',
  hour: '#job-form-daily-hour',
  minute: '#job-form-daily-minute',
  interval: '#job-form-interval-preset',
}));
```

并删除：

```js
payload.timezone = qs('#job-form-timezone').value.trim();
```

- [ ] **Step 4: 更新详情显示文案**

在 `renderJobDetail(job, payloadDraft, run)` 中，把原来的：

- `Cron`
- `Interval`
- `Timezone`

改成更用户向的展示：

```js
const scheduleSummary = job.schedule_type === 'interval'
  ? `固定间隔 · ${job.interval || '-'}`
  : `每日定时 · ${formatDailySchedule(job.cron || '')}`;
```

详情卡展示：

```html
<div class="k">调度方式</div><div class="v">${escapeHtml(scheduleSummary)}</div>
<div class="k">服务器时区</div><div class="v">${escapeHtml(job.timezone || 'Asia/Shanghai')}</div>
```

不要再把 `timezone` 暴露为用户输入字段，但详情里可以只读展示。

- [ ] **Step 5: 回跑契约检查**

Run:

```bash
node work/ui_task_center_relayout_contract_check.js
```

Expected:

- 输出 `ui task center relayout contract ok`

- [ ] **Step 6: 提交这一小步**

```bash
git add internal/web/assets/ui/app.js internal/web/assets/ui/index.html internal/web/assets/ui/styles.css work/ui_task_center_relayout_contract_check.js
git commit -m "feat: simplify advanced schedule editor"
```

如果当前目录不是 git 仓库，跳过 commit，但保留变更。

---

### Task 6: 做整页回归核对并补文档说明

**Files:**
- Modify: `docs/superpowers/specs/2026-06-06-api-template-task-job-ui-relayout-design.md`
- Test: `work/ui_task_center_relayout_contract_check.js`

- [ ] **Step 1: 在 spec 中补一个最终命名映射说明**

在 `docs/superpowers/specs/2026-06-06-api-template-task-job-ui-relayout-design.md` 末尾补充一节：

```md
## 14. 命名映射

- 旧：飞连API任务 → 新：飞连任务列表 / 单次任务清单
- 旧：飞连周期任务 → 新：飞连任务列表 / 定时任务清单
- 旧：cron 输入框 → 新：任务定时窗口（24h 时分选择器）
- 旧：interval 文本输入 → 新：固定间隔选项
```

- [ ] **Step 2: 跑前端契约检查**

Run:

```bash
node work/ui_task_center_relayout_contract_check.js
```

Expected:

- 输出 `ui task center relayout contract ok`

- [ ] **Step 3: 如环境具备，补一次后端回归**

Run:

```bash
go test ./internal/web -count=1
```

Expected:

- PASS

如果当前环境没有 Go，则记录说明：

```md
请在具备 Go 工具链的环境补跑：go test ./internal/web -count=1
```

- [ ] **Step 4: 最终提交**

```bash
git add internal/web/assets/ui/index.html internal/web/assets/ui/app.js internal/web/assets/ui/styles.css work/ui_task_center_relayout_contract_check.js docs/superpowers/specs/2026-06-06-api-template-task-job-ui-relayout-design.md
git commit -m "feat: relayout template and task center ui"
```

如果当前目录不是 git 仓库，跳过 commit，但保留变更。

---

## Self-Review

### Spec coverage

- `飞连API列表` 左右 + 右侧上下结构：Task 1
- `飞连任务列表` 一级页面与 Tabs：Task 1
- 单次任务工作台迁移与 bug 修复：Task 2
- 定时任务工作台迁移：Task 3
- `cron / interval / timezone` 业务化改造：Task 4、Task 5
- 文档命名映射与最终回归：Task 6

无明显需求遗漏。

### Placeholder scan

- 计划中没有 `TODO` / `TBD`
- 每个任务都包含具体 token、代码片段和命令

### Type consistency

- 合并后的一级 view 统一为 `tasks`
- 二级 tab 统一为 `once / schedule`
- 调度表单统一使用：
  - `time-mode`
  - `daily-hour`
  - `daily-minute`
  - `interval-preset`

---
