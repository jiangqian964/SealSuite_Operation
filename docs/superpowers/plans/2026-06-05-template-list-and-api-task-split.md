# Template List & API Task Split Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 将“模板列表”与“飞连API任务定义”拆分为两个一级页面，并在“模板列表”页增加右侧固定的独立执行输出窗口，保证模板测试结果始终可见。

**Architecture:** 仍维持单页应用的 `setView()` 切换机制：新增 `view-templates` 作为“模板列表”页，原 `view-api` 收敛为“任务定义”页。模板测试输出从任务页 `#api-out` 分流到模板页 `#tpl-out`，并提供固定输出窗口（可复制/清空/自动聚焦）。

**Tech Stack:** Go（chi 路由 + go:embed 静态资源）、Vanilla JS、HTML/CSS。

---

## 0. Files & Ownership（先锁定改动面）

**前端（embedded，生产使用）**
- Modify: `internal/web/assets/ui/index.html`（新增 view-templates、导航按钮、输出窗口 DOM）
- Modify: `internal/web/assets/ui/app.js`（新增 view 切换、模板页逻辑、输出窗口写入/聚焦、按钮行为分流）
- Modify: `internal/web/assets/ui/styles.css`（三栏布局 + 固定右侧输出窗口样式）

**前端（可选：dev 目录，避免两份 UI 不一致）**
- Modify: `web/ui/index.html`
- Modify: `web/ui/app.js`
- Modify: `web/ui/styles.css`

**后端**
- No change expected（复用现有 `/api/v1/api/templates`、`/api/v1/api/templates/{id}/test` 等）

**测试/契约检查（Node 脚本）**
- Modify/Create: `work/ui_binding_check.js`（若新增 direct bindings，需要扩充检查）
- Create: `work/ui_templates_layout_check.js`（新增：模板页关键 DOM 存在性检查）

---

## Task 1: Add “模板列表”一级页面与导航入口

**Files:**
- Modify: `internal/web/assets/ui/index.html`
- (Optional mirror) Modify: `web/ui/index.html`

- [ ] **Step 1: Write failing DOM contract check（RED）**

Create `work/ui_templates_layout_check.js`，要求模板页关键 DOM 存在：

```js
const fs = require('fs');
const html = fs.readFileSync('internal/web/assets/ui/index.html', 'utf8');
const must = [
  'id="view-templates"',
  'id="btn-open-templates"',
  'id="tpl-out"',
  'id="tpl-detail"',
  'id="tpl-table"',
];
const missing = must.filter(x => !html.includes(x));
if (missing.length) { console.error('missing:', missing); process.exit(1); }
console.log('templates layout ok');
```

- [ ] **Step 2: Run check to verify it fails（RED verify）**

Run:
```bash
node work/ui_templates_layout_check.js
```
Expected: FAIL（因为 `view-templates` 尚未加入）

- [ ] **Step 3: Implement minimal HTML skeleton（GREEN）**

在 `index.html`：
1) 左侧导航增加一个按钮（顺序：连接设置后、飞连API任务前）：
```html
<button class="navbtn" id="btn-open-templates">模板列表</button>
```

2) 新增一个 `section.view`：
```html
<section class="view hidden" id="view-templates">
  <div class="page-title">
    <div>
      <h2>模板列表</h2>
      <div class="subtitle">管理/测试飞连 OpenAPI 模板，并在右侧查看独立执行输出。</div>
    </div>
    <div class="panel-actions">
      <button class="btn" id="btn-templates-refresh">刷新</button>
      <button class="btn primary" id="btn-template-create-2">新增模板</button>
    </div>
  </div>

  <div class="grid templates-split">
    <div class="card">
      <div class="panel-head">
        <div>
          <div class="panel-title">模板列表</div>
          <div class="subtitle">支持按 id/name/category 搜索；点击“测试”结果会写入右侧输出窗口。</div>
        </div>
        <div class="panel-actions">
          <input id="tpl-filter-2" placeholder="按 id/name/category 过滤" />
        </div>
      </div>
      <table class="table" id="tpl-table">
        <thead><tr><th>id</th><th>category</th><th>method</th><th>path</th><th>actions</th></tr></thead>
        <tbody></tbody>
      </table>
    </div>

    <div class="card">
      <div class="panel-head">
        <div>
          <div class="panel-title">模板详情</div>
          <div class="subtitle">展示当前选中模板的只读 JSON。</div>
        </div>
      </div>
      <pre class="pre" id="tpl-detail">请选择一个模板</pre>
    </div>

    <div class="card tpl-out-card">
      <div class="panel-head">
        <div>
          <div class="panel-title">执行输出</div>
          <div class="subtitle">模板测试的响应/错误会固定显示在此处。</div>
        </div>
        <div class="panel-actions">
          <button class="btn" id="btn-tpl-out-copy">复制</button>
          <button class="btn" id="btn-tpl-out-clear">清空</button>
        </div>
      </div>
      <pre class="pre" id="tpl-out">暂无输出</pre>
    </div>
  </div>
</section>
```

备注：`btn-template-create-2` 与 `tpl-filter-2` 为避免与原 API 页 ID 冲突（实现阶段可统一抽象）。

- [ ] **Step 4: Run check to verify it passes（GREEN verify）**

```bash
node work/ui_templates_layout_check.js
```
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/web/assets/ui/index.html work/ui_templates_layout_check.js
git commit -m "feat(ui): add templates page skeleton and output window"
```

---

## Task 2: Styles — 三栏布局 + 右侧固定输出窗口

**Files:**
- Modify: `internal/web/assets/ui/styles.css`
- (Optional mirror) Modify: `web/ui/styles.css`

- [ ] **Step 1: Write a small style grep check（RED）**

Add to `work/ui_templates_layout_check.js`（或新脚本）断言存在 `.templates-split`、`.tpl-out-card` 样式标记：

```js
const css = fs.readFileSync('internal/web/assets/ui/styles.css','utf8');
['.templates-split', '.tpl-out-card'].forEach(k => { if (!css.includes(k)) throw new Error('missing css '+k); });
```

- [ ] **Step 2: Run and see it fail（RED verify）**

```bash
node work/ui_templates_layout_check.js
```

- [ ] **Step 3: Implement minimal styles（GREEN）**

在 `styles.css` 追加：

```css
.templates-split{
  grid-template-columns: 1.1fr 1.2fr .9fr;
  align-items: start;
}
.tpl-out-card{
  position: sticky;
  top: 18px;
  align-self: start;
  max-height: calc(100vh - 140px);
  overflow: hidden;
}
#tpl-out{
  max-height: calc(100vh - 240px);
}
```

并确保移动端响应式下改为单列（沿用已有 media query）：

```css
@media (max-width: 1020px){
  .templates-split{ grid-template-columns: 1fr; }
  .tpl-out-card{ position: static; max-height: none; }
  #tpl-out{ max-height: 260px; }
}
```

- [ ] **Step 4: Verify check passes（GREEN verify）**

```bash
node work/ui_templates_layout_check.js
```

- [ ] **Step 5: Commit**

```bash
git add internal/web/assets/ui/styles.css work/ui_templates_layout_check.js
git commit -m "feat(ui): add templates page split layout styles"
```

---

## Task 3: JS — 模板页数据流、输出窗口写入与聚焦

**Files:**
- Modify: `internal/web/assets/ui/app.js`
- (Optional mirror) Modify: `web/ui/app.js`
- Modify: `work/ui_binding_check.js`（若新增 direct bindings）

- [ ] **Step 1: Write failing binding check update（RED）**

若模板页新增 `qs('#btn-open-templates').addEventListener(...)` 等 direct binding，需要把 `ui_binding_check.js` 的缺失 ID 列表跑一遍，确保能抓到缺失。

Run:
```bash
node work/ui_binding_check.js
```
Expected: 如果还未添加 JS 绑定，会 PASS；本步骤的 RED 来自下一步写行为测试。

- [ ] **Step 2: Add minimal view navigation（RED -> GREEN）**

在 `app.js`：
1) 绑定导航按钮：

```js
qs('#btn-open-templates')?.addEventListener('click', () => setView('templates'));
```

2) 在 `setView()` 中确保能识别 `templates`（如果 setView 是基于 id 拼接，则无需改动）。

- [ ] **Step 3: Implement templates page rendering**

新增/复用函数：
- `renderTplTable2(items, filter)`：写入 `#tpl-table tbody`
- `renderTplDetail(tpl)`：写入 `#tpl-detail`
- `renderTplOut(payload)`：写入 `#tpl-out` 并调用 `focusTplOut()`
- `focusTplOut()`：与 `focusApiOut()` 类似（滚动 + flash）

核心交互：
- 点击表格行：
  - 设置 `window.__currentTplId = id`
  - `renderTplDetail(tpl)`
- 点击“测试”：
  - 调用 `/api/v1/api/templates/${id}/test`
  - `renderTplOut({title:'模板测试', template_id:id, ...response})`
- 点击“复制/清空”：
  - Copy：复制 `#tpl-out` 内容
  - Clear：置为 `暂无输出`

- [ ] **Step 4: Ensure “新增模板”在模板页可用**

模板页按钮 `#btn-template-create-2` 直接复用既有编辑器逻辑：
```js
qs('#btn-template-create-2')?.addEventListener('click', () => qs('#btn-template-create')?.click());
```
或直接调用 `openTemplateEditor(draft, {isCreate:true})`。

- [ ] **Step 5: Add “回到模板列表/任务定义”入口一致性**

模板编辑器 “返回”按钮现在是 `setView('api')`，需要改成：
- 若编辑器是从模板页进入：返回 `templates`
- 若从任务页进入：返回 `api`

实现方式（最小）：
- 在进入编辑器时设置 `window.__templateEditorReturnView = 'templates' | 'api'`
- `btn-template-editor-back` 根据该变量跳转

- [ ] **Step 6: Verify JS syntax + binding check（GREEN verify）**

```bash
node --check internal/web/assets/ui/app.js
node work/ui_binding_check.js
node work/ui_templates_layout_check.js
```

- [ ] **Step 7: Commit**

```bash
git add internal/web/assets/ui/app.js internal/web/assets/ui/index.html internal/web/assets/ui/styles.css work/ui_binding_check.js work/ui_templates_layout_check.js
git commit -m "feat(ui): implement templates page behaviors and dedicated output window"
```

---

## Task 4: Refactor — 任务定义页移除模板列表（收敛职责）

**Files:**
- Modify: `internal/web/assets/ui/index.html`
- Modify: `internal/web/assets/ui/app.js`
- (Optional mirror) Modify: `web/ui/*`

- [ ] **Step 1: Write failing UI grep check（RED）**

在 `work/ui_templates_layout_check.js` 增加断言：`view-api` 不再包含模板列表表格 DOM（避免重复两套入口）。
例如断言 `id="tpl-table"` 只出现在 `view-templates`。

- [ ] **Step 2: Implement HTML move（GREEN）**

将原 `view-api` 内的“模板列表/模板详情/原始响应”区域：
- 从任务页删除或弱化为一段提示 + 跳转按钮：
  - “去模板列表管理模板与测试”
  - 按钮：`setView('templates')`

任务页只保留：
- 草稿/任务定义表单
- Preview/Execute/保存草稿
- `#api-out` 输出区

- [ ] **Step 3: Update JS references**

删除/迁移原来绑定在任务页模板列表上的事件，确保：
- 模板相关 DOM 仅在 `view-templates` 存在并被绑定
- 任务页不再调用 `renderTplTable(...)` 等旧 UI

- [ ] **Step 4: Verify checks（GREEN verify）**

```bash
node --check internal/web/assets/ui/app.js
node work/ui_binding_check.js
node work/ui_templates_layout_check.js
```

- [ ] **Step 5: Commit**

```bash
git add internal/web/assets/ui/index.html internal/web/assets/ui/app.js work/ui_templates_layout_check.js
git commit -m "refactor(ui): split templates from api task definition page"
```

---

## Task 5 (Optional): “用此模板创建任务”联动

**Files:**
- Modify: `internal/web/assets/ui/app.js`
- Modify: `internal/web/assets/ui/index.html`

- [ ] **Step 1: Add action button in templates table**

在模板表格 actions 增加：
- “用此模板创建任务”

- [ ] **Step 2: Implement behavior**

点击后：
- `setView('api')`
- `#draft-source-tpl` 或 `#exec-template` 自动填入模板 id
- 自动滚动到任务定义区

- [ ] **Step 3: Manual verification checklist**

1) 在模板列表页点“用此模板创建任务”
2) 切到任务页后 template_id 已预填

- [ ] **Step 4: Commit**

```bash
git commit -am "feat(ui): allow creating api task draft from selected template"
```

---

## Task 6: Mirror changes to `web/ui` (if used by your dev workflow)

> 若你实际运行/调试用的是 `web/ui`（而非 embed），则必须同步，避免“目录 A 改了目录 B 没改”的错乱。

**Files:**
- Modify: `web/ui/index.html`
- Modify: `web/ui/app.js`
- Modify: `web/ui/styles.css`

- [ ] **Step 1: Copy equivalent changes**
- [ ] **Step 2: Re-run node checks by pointing at `web/ui`**
- [ ] **Step 3: Commit**

---

## Self-Review Checklist (plan quality)

- [ ] 覆盖 spec 中的“两个一级页面 + 右侧固定输出窗口 + 输出分流”
- [ ] 输出窗口：成功/失败都写入、可复制/清空、自动聚焦高亮
- [ ] 没有 “TODO/TBD” 占位
- [ ] DOM id 与 JS 绑定一致（`ui_binding_check.js`、`ui_templates_layout_check.js` 保障）

---

## Execution Handoff

计划已写入：`docs/superpowers/plans/2026-06-05-template-list-and-api-task-split.md`。

两种执行方式你选一个：

1) **Subagent-Driven（推荐）**：我按 Task 逐个派发子代理实现，每个 Task 完成后我复核再进入下一步  
2) **Inline Execution**：我在当前会话按 Task 逐步实现（每完成一个 Task 进行一次回归/确认）

