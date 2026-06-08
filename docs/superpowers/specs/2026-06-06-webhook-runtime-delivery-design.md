# 2026-06-06 — Webhook 运行时投递设计说明

## 1. 背景

当前系统已经完成了两部分基础能力：

1. `webhook配置` 作为全局 webhook 配置中心已经建立
2. 以下三类业务对象已经支持保存 webhook 引用字段：
   - `飞连API任务`
   - `飞连周期任务`
   - `运营agent`

也就是说，配置层已经具备：

- `webhook_config_id`
- `webhook_enabled`

但运行时还没有真正把 webhook 接入执行链路。

因此，本轮设计的目标是：

> 在真正的执行成功链路中，按配置触发 webhook 推送

---

## 2. 本次设计目标

本次设计目标如下：

1. 在以下对象执行成功后触发 webhook 推送：
   - `飞连API任务`
   - `飞连周期任务`
   - `运营agent`
2. 只在“执行成功”时推送
3. 使用统一的 webhook 投递 helper
4. 使用统一的 webhook payload envelope
5. webhook 推送失败时：
   - 不影响主执行结果
   - 只作为附加状态返回和记录
6. 在主执行结果中增加 `webhook_delivery` 信息

---

## 3. 非目标

本轮不做以下事情：

- 不实现“失败时推送”
- 不实现“每个对象自定义成功/失败推送策略”
- 不实现异步消息队列
- 不实现第三方平台特定签名协议适配
- 不实现复杂重试策略编排

本轮聚焦：

- 同步触发
- 成功后投递
- 推送失败不阻断主流程

---

## 4. 推荐方案

采用你已确认的 **方案 A：各执行入口内联推送**。

也就是：

- 在三个真实执行入口中
- 当主业务执行成功后
- 直接调用同一个 webhook 投递 helper

而不是先引入一个事件总线或异步 dispatcher。

这样做的优点是：

- 第一版最稳
- 排障路径最短
- 易于验证

后续如果 webhook 能力继续变复杂，再把 helper 抽成统一分发器也不迟。

---

## 5. 触发规则

## 5.1 仅成功时推送

按你已确认的规则：

- 主执行成功 → 才尝试 webhook 推送
- 主执行失败 → 不推送

这个规则适用于三类对象：

- `飞连API任务`
- `飞连周期任务`
- `运营agent`

---

## 5.2 必须同时满足两个条件

只有同时满足以下条件时才触发 webhook：

1. `webhook_enabled = true`
2. `webhook_config_id` 非空且能找到对应 webhook 配置

否则视为：

- 不尝试推送

---

## 6. 三条执行链路

## 6.1 飞连API任务

对应的执行链路是：

- `POST /api/v1/api/execute`

但这里要区分两种情况：

### A. 临时执行

如果只是用户直接在工作台点击 Execute，而当前执行没有绑定到保存过的业务对象，则：

- 默认不推送 webhook

但按最新确认，这里需要支持：

> **临时 Execute 也可以推送，但默认关闭，需用户显式勾选本次推送**

也就是说：

- 未勾选时：不推送
- 勾选“本次执行成功后也推送 webhook”时：本次成功执行后推送

这个行为只影响当前执行，不回写为长期配置。

### B. 任务对象执行

如果这次执行来源于已保存并配置了 webhook 的任务对象，则：

- 成功后触发 webhook

因此这里建议：

- 已保存任务按持久化配置推送
- 临时执行按“本次是否显式勾选推送”决定是否推送

---

## 6.1.1 临时 Execute 的请求扩展

为了支持“临时执行单次推送”，建议在 `POST /api/v1/api/execute` 的请求中增加可选字段：

```json
{
  "webhook_config_id": "xxx",
  "webhook_enabled": true,
  "webhook_push_once": true
}
```

其中：

- `webhook_config_id`：本次临时执行使用哪个 webhook 配置
- `webhook_enabled`：是否启用 webhook
- `webhook_push_once`：是否对本次执行生效

语义约束：

- 只有 `webhook_push_once = true` 才允许临时执行触发推送
- 该字段不写回任务草稿，不改变长期配置

---

## 6.2 飞连周期任务

飞连周期任务需要覆盖两类执行方式：

1. 手动点击运行
2. 调度器自动运行

因此必须确保：

- 手动运行的入口会触发 webhook
- 定时调度真正执行的链路也会触发 webhook

不能只接手动按钮，否则自动运行与手动运行行为不一致。

---

## 6.3 运营agent

对应执行链路包括：

- `POST /api/v1/complex-tasks/run`
- `POST /api/v1/complex-tasks/{id}/run`

在 `RunComplexTask(...)` 或其 API 包装返回成功之后：

- 统一触发 webhook

---

## 7. 统一 payload 结构

为避免三类对象各自发送不同结构，建议定义统一 envelope：

```json
{
  "event": "task.completed",
  "source_type": "api_task | job_schedule | complex_task",
  "source_id": "xxx",
  "source_name": "xxx",
  "timestamp": "2026-06-06T12:00:00Z",
  "data": {}
}
```

说明：

- `event`：当前统一为 `task.completed`
- `source_type`：标识是哪一类对象
- `source_id`：对象 ID
- `source_name`：对象显示名称
- `timestamp`：推送发生时间
- `data`：对象自己的结果内容

---

## 8. 各对象的 data 内容

## 8.1 飞连API任务

`source_type = api_task`

建议 `data` 包含：

- `template_id`
- `method`
- `path`
- 请求结果原始响应
- transform / llm / output 的结果（如果本次链路中存在）

## 8.2 飞连周期任务

`source_type = job_schedule`

建议 `data` 包含：

- `schedule_id`
- `target_type`
- `target_id`
- 本次运行结果
- `next_run`（如果当前链路可拿到）

## 8.3 运营agent

`source_type = complex_task`

建议 `data` 包含：

- `task_id`
- `steps`
- `summary`
- `final_output`

---

## 9. 统一投递 helper

建议新增一个统一 helper，例如：

- `deliverWebhookForSuccess(...)`

它的职责是：

1. 检查业务对象是否启用 webhook
2. 根据 `webhook_config_id` 读取 webhook 配置
3. 组装统一 payload
4. 调用 webhook 投递
5. 返回结构化投递结果

建议 helper 返回：

```json
{
  "attempted": true,
  "ok": true,
  "status_code": 200,
  "response_body": "..."
}
```

或：

```json
{
  "attempted": false
}
```

或：

```json
{
  "attempted": true,
  "ok": false,
  "error": "..."
}
```

---

## 10. 错误处理策略

这是本轮最关键的行为约束。

## 10.1 主业务成功 + webhook 失败

处理策略：

- 主业务仍然视为成功
- webhook 失败只作为附加状态
- 不影响主接口返回成功语义

原因：

- webhook 是结果分发能力
- 不是主执行链路的核心成功条件

---

## 10.2 主业务失败

处理策略：

- 不推送 webhook

理由：

- 本轮只做“成功推送”

---

## 10.3 未配置 webhook

处理策略：

- 不推送
- `webhook_delivery.attempted = false`

---

## 11. 返回结构设计

建议在三类执行结果中增加一个统一字段：

```json
"webhook_delivery": {
  "attempted": true,
  "ok": true,
  "status_code": 200
}
```

如果未尝试：

```json
"webhook_delivery": {
  "attempted": false
}
```

如果失败：

```json
"webhook_delivery": {
  "attempted": true,
  "ok": false,
  "error": "..."
}
```

这样前端可以决定是否显示：

- 已推送
- 未启用推送
- 推送失败
- 本次临时执行已单次推送

同时日志里也更容易排查。

---

## 12. 具体接入点建议

## 12.1 飞连API任务

建议在 API 执行成功后、返回前接入：

- 仅对“有任务对象上下文”的执行做推送
- 直接在纯临时执行中不做推送

## 12.2 飞连周期任务

建议在：

- 手动运行接口
- 调度器内部真正执行完成处

都统一接入到同一个 helper。

## 12.3 运营agent

建议在：

- `RunComplexTask(...)` 成功返回之后
- 或对外 API 包装返回之前

统一接入 webhook helper。

---

## 13. 日志建议

建议在服务端日志中记录以下内容：

- source_type
- source_id
- webhook_config_id
- attempted
- ok
- status_code
- error

这样即使主接口不展开展示，也能从日志快速判断：

- 是否尝试推送
- 推送到了哪个 webhook
- 是否成功

---

## 14. 分阶段建议

虽然目标是“最后一个阶段”，但仍建议按一个清晰顺序实施：

1. 先实现统一 helper
2. 接入飞连API任务成功链路
3. 接入飞连周期任务成功链路
4. 接入运营agent 成功链路
5. 在返回结构中补齐 `webhook_delivery`

这样每一条链路都可以单独验证。

---

## 15. 验收标准

以下条件全部满足，视为本轮设计达成：

1. 飞连API任务执行成功后，若启用了 webhook，则会推送
2. 飞连API任务工作台临时 Execute 默认不推送，但在勾选“本次执行成功后也推送”后会推送
3. 飞连周期任务手动运行成功后会推送
4. 飞连周期任务自动调度成功后会推送
5. 运营agent 成功运行后会推送
6. 未配置 webhook 的对象不推送
7. webhook 推送失败不会让主任务失败
8. 返回结果中能看到 `webhook_delivery`

---

## 16. 风险与注意事项

## 16.1 第一版不做失败推送

如果后续要支持失败推送，就需要增加：

- 失败 payload 结构
- 失败时机定义
- 每对象策略配置

因此本轮刻意不做。

## 16.2 第一版不做异步队列

如果 webhook 对端响应慢，当前同步推送可能会拉长接口时长。

但第一版优先保证：

- 实现简单
- 调试方便
- 行为可见

后续若量大，再考虑异步化。

## 16.3 webhook 是附加能力，不是主成功条件

这个约束必须在实现中严格遵守：

- 主任务成功
- webhook 失败
- 最终仍返回主任务成功 + `webhook_delivery.ok=false`
