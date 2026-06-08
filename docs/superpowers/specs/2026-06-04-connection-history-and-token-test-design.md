# 连接设置增强：最近启用清单 + Token 测试弹窗（设计稿）

日期：2026-06-04  
范围：本地 Web 控制台（原生 HTML/JS + go:embed），不引入前端框架  
目标：改进连接设置操作体验，并提升用户对“access_token 是否获取成功”的可见性

---

## 1. 背景

现状：
- 已支持在“连接设置”页面编辑 `scheme/host/port/access_key/secret_key`，保存写入 `config.yaml` 并在线热更新。
- “测试连接”目前只展示业务 API 探测结果，未显式展示 access_token 获取是否成功、token 是否有效。

新增需求：
1) 在连接设置页增加“最近三次录入并启用”的清单，展示飞连服务地址、AccessKey ID 等信息，支持一键切换。  
2) 连接设置-测试连接增加一个窗口，展示是否获取到有效的 access_token。

---

## 2. 目标 / 非目标

### 2.1 目标
- 连接设置页面新增“最近启用（最多 3 条）”区域，并支持一键切换到某条连接配置
- 测试连接时显式显示 token 获取结果：成功/失败、expires_in、token 摘要（脱敏）
- 切换连接后，服务在线热更新：重载 config、重建 client、runner/executor 使用新连接、Reload jobs/templates
- UI 文案解释清楚 Jobs / 模板库用途（减少“这是干啥的？”困惑）

### 2.2 非目标（v0）
- 不在 `connections.yaml` 中保存明文 secret
- 不做系统 Keychain 集成/本地加密存储（v1 选做）
- 不做复杂权限体系（仍默认 localhost）

---

## 3. 数据存储设计

### 3.1 connections.yaml（新增）
保存“最近启用”的连接信息，最多保留 3 条；并记录当前激活项。

```yaml
version: 1
active_id: "conn_20260604_001"
items:
  - id: "conn_20260604_001"
    name: "生产-飞连"
    scheme: "https"
    host: "feilian.example.com"
    port: 443
    access_key_id: "AKxxxxxx"
    secret_ref: "config"
    created_at: "2026-06-04T12:00:00+08:00"
```

字段说明：
- `access_key_id` 允许保存明文（前端展示时脱敏）
- `secret_ref`：v0 固定为 `"config"`（表示 secret 仍以 `config.yaml sealsuite.secret_key` 为准）

### 3.2 config.yaml（沿用）
- 仍作为真实生效配置来源：`sealsuite.scheme/host/port/access_key/secret_key`
- 切换连接时，会写回 `scheme/host/port/access_key`；`secret_key` 默认不覆盖（除非用户在当前表单输入了新的 secret）

---

## 4. 后端 API 设计

### 4.1 连接历史
- `GET /api/v1/connections`  
  返回：`{active_id, items:[...]}`（items 中 `access_key_id` 脱敏、secret 永不返回）

- `POST /api/v1/connections`  
  入参：`{name?, scheme, host, port, access_key_id, secret_key?}`  
  行为：将该连接写入 config.yaml（同现有 PUT /connection）并热更新；同时将该连接追加到 connections.yaml，并裁剪到最近 3 条；更新 active_id

- `POST /api/v1/connections/{id}/activate`  
  行为：读取对应项 → 写回 config.yaml（scheme/host/port/access_key_id）→ 热更新 → 更新 active_id  
  注意：v0 不携带 secret，若 secret 在 config.yaml 中为空/错误，激活后测试连接仍会失败；UI 需提示用户补齐 secret

### 4.2 测试连接（增强 token 可见性）
调整 `POST /api/v1/connection/test` 返回结构，新增 token 结果：
- `token_ok: boolean`
- `token_preview: string`（例如 `HNzu…xMF`）
- `token_expires_in: number`
- `token_error: string`（失败原因）
- `probe_ok: boolean`（业务 API 探测是否 ok，可用 users_list）
- `probe_result` / `probe_error`

实现建议：
- 先显式调用 token 获取（/api/open/v1/token），并捕获结果
- 若 token_ok，再对 users_list 发起探测请求，验证 Authorization 生效

---

## 5. 前端交互与页面结构

### 5.1 连接设置页（新增区域）
页面分为三块：
1) **当前连接**（原有表单）  
2) **最近启用（最多 3 条）**：表格展示 + 一键切换按钮  
3) **测试连接结果**：点击“测试连接”弹出 Modal（见下）

最近启用表格列：
- 名称（name）
- host:port
- AccessKey ID（脱敏）
- 激活状态（active）
- 操作：`切换`

### 5.2 测试连接弹窗（Modal）
点击“测试连接”后弹出窗口，显示两段：
- Token 获取：成功/失败、expires_in、token 摘要、失败原因
- API 探测：调用 users_list 的返回（或失败原因）

### 5.3 文案与引导（解决“这是干啥的？”）
在 Jobs / 模板页标题下增加简短说明：
- Jobs：周期性自动化任务中心（cron 定期执行 api_poll/api_write）
- 模板列表：常用飞连 API 请求模板库（减少手工填参数、支持 Preview、可一键保存为周期任务）

---

## 6. 安全与隐私
- 前端与 API 永不回显 secret_key 明文
- connections.yaml 不保存 secret（v0）
- 若未来需要“真正一键切换含 secret”，再做 v1：secret 加密存储（需要 master key 或系统 Keychain）

---

## 7. 验收标准（DoD）
1) 连接设置页出现“最近启用”清单，最多 3 条
2) 录入并保存连接后，会写入 connections.yaml，且可一键切换
3) 测试连接弹窗明确展示 token 是否成功、token 有效期与 token 摘要
4) 一键切换后在线热更新生效：后续 API 工具箱/周期任务使用新连接
5) Jobs/模板列表页面显示清晰说明文案

