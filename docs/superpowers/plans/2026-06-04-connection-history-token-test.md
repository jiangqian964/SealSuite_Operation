# 连接设置增强（历史清单 + Token 测试弹窗）Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 在本地 Web 控制台中新增连接历史（最近 3 条启用配置、一键切换）与测试连接弹窗（显式展示 access_token 获取结果），并补充 Jobs/模板模块说明文案。

**Architecture:** 新增 `connections.yaml` 作为历史清单存储（不保存 secret）；后端新增 connections API 并在激活时写回 `config.yaml` + runner 热更新；测试连接接口返回 token_ok/token_preview/expires_in；前端连接页新增历史表格与 modal 展示。

**Tech Stack:** Go 1.21、chi、go:embed 静态前端、yaml.v3、httptest 单测、原生 JS modal

---

## File Map

**Create**
- `internal/storage/connections_store.go`
- `internal/storage/connections_store_test.go`
- `connections.yaml`（默认空）

**Modify**
- `internal/web/server.go`：新增 connections API、增强 connection/test 返回 token 信息
- `internal/web/assets/ui/index.html`：连接页新增“最近启用”表格与 Token 测试 modal；Jobs/模板文案增强
- `internal/web/assets/ui/app.js`：拉取/渲染历史、一键切换、测试连接 modal 展示
- `internal/sealsuite/client.go`：暴露获取 token 的方法（供 test 接口显示 token）

---

## Task 1: connections.yaml 存储（TDD）

**Files:**
- Create: `internal/storage/connections_store_test.go`
- Create: `internal/storage/connections_store.go`
- Create: `connections.yaml`

- [ ] Step 1: 写 failing test：最多保留 3 条、active_id 更新
- [ ] Step 2: 运行测试确认失败：`go test ./... -count=1`
- [ ] Step 3: 实现 store：Load/Save/AddAndActivate/GetActive/Trim
- [ ] Step 4: 运行测试确认通过

---

## Task 2: Client token 获取结果可观测（TDD）

**Files:**
- Modify: `internal/sealsuite/client.go`
- Create/Modify: `internal/sealsuite/client_test.go`（如需）

- [ ] Step 1: 新增方法 `FetchAccessToken()`（返回 token/expires_in）
- [ ] Step 2: 在 `ensureAccessToken` 内复用该方法并缓存

---

## Task 3: 后端 API（connections + test token）

**Files:**
- Modify: `internal/web/server.go`

- [ ] Step 1: `GET /api/v1/connections`：返回 active_id + items（ak 脱敏）
- [ ] Step 2: `POST /api/v1/connections`：保存到 config.yaml + 写入 connections.yaml + 激活 + 热更新
- [ ] Step 3: `POST /api/v1/connections/{id}/activate`：按 id 激活并热更新
- [ ] Step 4: 增强 `POST /api/v1/connection/test`：返回 token_ok/token_preview/expires_in/token_error + probe 结果
- [ ] Step 5: 增加 httptest 覆盖（可放在 `internal/web/server_test.go`）

---

## Task 4: 前端连接页体验（历史表格 + modal）

**Files:**
- Modify: `internal/web/assets/ui/index.html`
- Modify: `internal/web/assets/ui/app.js`
- (Optional) Modify: `internal/web/assets/ui/styles.css`

- [ ] Step 1: 连接页新增“最近启用”表格（最多 3 条）与“切换”按钮
- [ ] Step 2: 测试连接按钮弹出 modal：Token 获取区 + Probe 区
- [ ] Step 3: Jobs/模板模块增加一行解释文案

---

## Verification

- `go test ./... -count=1`
- `go run ./cmd/main.go` 后浏览器打开：
  - 连接页可看到最近启用清单 + 一键切换
  - 测试连接弹窗能看到 token_ok/token_preview/expires_in

