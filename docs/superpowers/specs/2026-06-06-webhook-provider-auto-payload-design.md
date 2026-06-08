# 2026-06-06 — Webhook 类型驱动自动 Payload 匹配设计

## 1. 背景

当前系统已经完成了以下基础能力：

1. 已有全局 `webhook配置` 页面，可保存：
   - `webhook-id`
   - `url`
   - `method`
   - `headers`
   - `body_template`
2. 以下三类业务对象已支持绑定 webhook 引用：
   - `飞连API任务`
   - `飞连周期任务`
   - `运营agent`
3. 运行时已具备 webhook 成功后投递链路，但当前发送逻辑仍是：
   - 直接将统一 envelope 作为请求体发送
   - 没有真正使用 `body_template`

因此，虽然任务侧已经可以勾选某个 `webhook-id`，但还存在一个明显问题：

> 用户选择了 webhook 之后，仍然需要自己关心 payload 应该长什么样，甚至手动调整模板内容。

这与“任务只做业务配置、协议细节交给 webhook 配置中心”的目标不一致。

---

## 2. 本次设计目标

本次设计目标如下：

1. 当用户在以下对象中勾选某个 `webhook-id` 时：
   - `飞连API任务`
   - `飞连周期任务`
   - `运营agent`
   系统可以按该 webhook 的类型自动生成匹配的 payload
2. 任务侧无需手动调整模板内容
3. 先优先支持：
   - `飞书自定义机器人`
4. 同时保留：
   - 通用 webhook 的自定义模板能力
5. 保持“全局配置 + 业务引用”的关系不变
6. 保持“webhook 失败不阻断主任务”的既有运行时语义不变

---

## 3. 非目标

本轮不做以下内容：

- 不引入新的事件总线
- 不实现失败时推送
- 不实现不同业务对象各自定义不同平台协议
- 不自动识别所有第三方平台
- 不移除通用 webhook 的手工模板能力

本轮只聚焦：

- 用 webhook 类型驱动 payload 自动匹配
- 优先落地飞书机器人自动适配

---

## 4. 核心结论

推荐方案为：

> **在 webhook 配置中增加“类型 / 协议提供方”字段，由 webhook 自己声明目标平台；任务侧只负责引用该 webhook，不再承担模板编辑职责。**

也就是：

- webhook 配置中心定义“这是什么平台”
- 运行时根据 webhook 类型自动生成请求体
- 业务对象只绑定 `webhook_config_id`
- 任务侧不再要求用户手调 payload 模板

这是当前最符合你已确认方向的方案。

---

## 5. 方案对比

## 5.1 方案 A：按 webhook 类型自动套用 payload

### 方案描述

在 `WebhookItem` 中增加类型字段，例如：

- `provider = generic`
- `provider = feishu_bot`

运行时按 `provider` 决定请求体生成方式。

### 优点

- 真正实现“勾选 webhook-id 后自动匹配 payload”
- 任务配置最轻
- 三类业务对象行为统一
- 后续可扩展更多平台类型

### 缺点

- 需要在配置模型、前端表单、运行时 helper 同时补一层 provider 适配

### 结论

**推荐采用。**

---

## 5.2 方案 B：按 webhook 默认模板自动套用

### 方案描述

每条 webhook 配置仍以 `body_template` 为核心，只是任务侧不再暴露模板输入，统一复用 webhook 配置页里保存的默认模板。

### 优点

- 改动较小

### 缺点

- 本质仍然是模板驱动，不是协议驱动
- 飞书这类有明确格式要求的平台，仍然依赖用户自己维护模板文本
- 不利于后续扩展多平台

### 结论

不作为主方案。

---

## 5.3 方案 C：只对飞书做硬编码自动适配

### 方案描述

如果 URL 命中飞书 webhook 规则，则直接硬编码走飞书格式；否则走通用逻辑。

### 优点

- 第一版实现快

### 缺点

- 平台规则隐藏在代码中，页面没有明确模型表达
- 可维护性差
- 后续接入更多平台容易失控

### 结论

不作为主方案。

---

## 6. 数据模型设计

## 6.1 `WebhookItem` 新增字段

建议在 `internal/storage/webhook_store.go` 的 `WebhookItem` 中新增字段：

```go
Provider string `yaml:"provider" json:"provider"`
```

字段语义：

- `generic`
  - 通用 webhook
  - 保留 `body_template`
  - 运行时按自定义模板渲染请求体
- `feishu_bot`
  - 飞书自定义机器人
  - 运行时自动生成符合飞书格式的 payload

---

## 6.2 默认值与兼容策略

为避免破坏现有数据：

1. 老数据默认视为 `provider = generic`
2. 如果配置中没有 `provider` 字段：
   - 后端读取时补默认值 `generic`
3. 前端保存旧记录时：
   - 未显式选择 provider 也保存为 `generic`

这样可以保证：

- 当前已有 webhook 配置全部继续可用
- 只有显式选择为 `feishu_bot` 的记录才启用自动飞书适配

---

## 7. 任务侧行为设计

## 7.1 业务对象保持“只引用、不配置协议”

以下对象仍然只保存：

- `webhook_config_id`
- `webhook_enabled`

不新增任务级协议字段，不新增任务级模板字段。

原因：

- 你已明确采用“全局配置 + 业务引用”
- 协议层属于 webhook 配置自身，不应散落到每个业务对象

---

## 7.2 任务侧交互目标

当用户在以下对象中勾选某个 `webhook-id` 后：

- `飞连API任务`
- `飞连周期任务`
- `运营agent`

系统行为应为：

1. 仅保存 webhook 引用关系
2. 不要求用户继续去改 payload 模板
3. 页面上明确提示：
   - 当前 payload 将按所选 webhook 类型自动生成

例如：

- 若选中的是飞书机器人类型 webhook
- 则任务页显示提示：
  - `已按飞书机器人协议自动匹配 payload`

---

## 7.3 临时 Execute 的语义

对于 `飞连API任务` 工作台里的临时 Execute：

- 仍沿用你已确认的规则：
  - 默认不推送
  - 勾选“本次执行成功后也推送 webhook”时才推送

但一旦本次执行已选定某个 `webhook-id`：

- payload 仍然由 webhook 类型自动决定
- 不要求用户在临时执行时再手调模板

---

## 8. webhook 配置页设计

## 8.1 新增“类型 / 协议”字段

在 `webhook配置` 页面中新增下拉项：

- `通用 webhook`
- `飞书机器人`

对应值建议为：

- `generic`
- `feishu_bot`

---

## 8.2 不同 provider 的页面行为

### `generic`

保持现状：

- `body_template` 可编辑
- 支持用户自定义 headers
- 测试推送使用用户填写的 payload

### `feishu_bot`

页面行为建议调整为：

1. `body_template` 输入框隐藏或改为只读
2. 显示固定说明：
   - `系统将自动按飞书自定义机器人格式生成 payload`
3. 保留 `headers` 编辑区，但默认可为空
4. 测试推送时不要求用户手工组织飞书 JSON 结构

这样可以把“飞书协议格式”从用户心智里拿掉。

---

## 8.3 webhook 列表展示建议

在 webhook 列表中补充 provider 信息，例如：

- `类型：飞书机器人`
- `类型：通用 webhook`

避免用户只看名称无法判断该 webhook 的协议类型。

---

## 8.4 飞书机器人配置示例

建议在文档和页面帮助文案中补一个最小可用示例，明确说明：

- `provider` 选择 `feishu_bot`
- `body_template` 不需要用户手工维护
- `headers` 可保留最常见的 `Content-Type: application/json`

示例 YAML：

```yaml
items:
  - id: feishu-success-bot
    name: 飞书任务成功通知
    provider: feishu_bot
    url: https://open.feishu.cn/open-apis/bot/v2/hook/xxxxxxxx-xxxx-xxxx-xxxx-xxxxxxxxxxxx
    method: POST
    headers:
      Content-Type: application/json
    auth_type: ""
    body_template: ""
    timeout_sec: 10
    retry_count: 0
    enabled: true
```

对应页面填写示例可理解为：

- `webhook-id`: `feishu-success-bot`
- `name`: `飞书任务成功通知`
- `provider`: `feishu_bot`
- `url`: 飞书机器人 webhook 地址
- `method`: `POST`
- `headers`: `{"Content-Type":"application/json"}`
- `body_template`: 留空，由系统自动生成飞书 payload

---

## 9. 运行时请求体生成策略

## 9.1 运行时统一入口

建议将当前 `DeliverWebhookForSuccess(...)` 升级为：

1. 读取 webhook 配置
2. 判断 `provider`
3. 生成最终请求体
4. 发送 HTTP 请求
5. 返回结构化投递结果

也就是说，运行时不再简单地：

- `json.Marshal(env)` 后直接发送

而是要先经过 payload 渲染步骤。

---

## 9.2 provider 到 payload 的映射规则

### `provider = generic`

规则：

- 若存在 `body_template`
  - 按模板渲染最终请求体
- 若不存在 `body_template`
  - 可回退为发送统一 envelope

这样通用 webhook 保持兼容。

### `provider = feishu_bot`

规则：

- 忽略 `body_template` 的手工输入
- 由系统自动生成飞书文本消息 payload

---

## 10. 飞书机器人自动适配规则

## 10.1 飞书官方格式约束

飞书自定义机器人最基础、最适合当前需求的消息体为：

```json
{
  "msg_type": "text",
  "content": {
    "text": "..."
  }
}
```

因此本轮飞书适配建议先只做：

- `msg_type = text`

不在第一版直接做：

- `post`
- `card`
- `image`

---

## 10.2 飞书默认 payload 结构

当 `provider = feishu_bot` 时，系统自动生成如下结构：

```json
{
  "msg_type": "text",
  "content": {
    "text": "【{{source_name}}】执行成功\n类型: {{source_type}}\n时间: {{timestamp}}\n结果:\n{{jsonString data}}"
  }
}
```

渲染后的文本应包含：

1. 任务名称
2. 任务类型
3. 执行时间
4. 本次任务运行结果 JSON

这里的关键点是：

> 任务结果 JSON 要自动放入飞书要求的 `content.text` 中，而不是第三方自定义的 `message` 字段中。

---

## 10.3 三类业务对象在飞书中的表现

### 飞连API任务

飞书消息文本中包含：

- `source_name = 飞连API任务名称`
- `source_type = api_task`
- `data = 本次 API 执行结果`

### 飞连周期任务

飞书消息文本中包含：

- `source_name = 飞连周期任务名称`
- `source_type = job_schedule`
- `data = 本次周期运行结果`

### 运营agent

飞书消息文本中包含：

- `source_name = 运营agent 名称`
- `source_type = complex_task`
- `data = 本次 agent 运行结果`

---

## 11. 模板变量与渲染约定

虽然任务侧不再需要手调模板，但为了兼容 `generic` 模式，仍建议统一定义渲染变量：

- `{{source_type}}`
- `{{source_id}}`
- `{{source_name}}`
- `{{timestamp}}`
- `{{json data}}`
- `{{jsonString data}}`
- `{{json envelope}}`
- `{{jsonString envelope}}`

其中：

- `{{json data}}`
  - 输出 JSON 对象文本，适合放在 JSON 结构中作为对象值
- `{{jsonString data}}`
  - 输出转义后的 JSON 字符串，适合嵌入文本字段

对于飞书文本消息，核心使用的是：

- `{{jsonString data}}`

---

## 12. 测试推送行为

## 12.1 通用 webhook

保持现有逻辑：

- 用户填写测试 payload
- 服务端按该 payload 发出测试请求

## 12.2 飞书机器人

建议测试推送改为：

1. 允许用户输入一段“模拟结果 JSON”或“测试文本”
2. 服务端按飞书标准格式自动包装
3. 最终发出的仍是：

```json
{
  "msg_type": "text",
  "content": {
    "text": "..."
  }
}
```

也就是说：

- 测试区让用户提供的是“业务内容”
- 不是“飞书协议原始 JSON”

---

## 13. 错误处理与兼容行为

## 13.1 webhook 失败不阻断主流程

保持现有规则不变：

- 主任务成功
- webhook 推送失败
- 主任务仍然成功
- `webhook_delivery.ok = false`

---

## 13.2 provider 非法值

如果遇到非法 provider：

- 建议回退为 `generic`
- 并记录日志

避免因为配置污染直接导致全部 webhook 不可用。

---

## 13.3 飞书 body 限制

飞书机器人请求体有大小限制，因此第一版应注意：

- 当 `data` JSON 过大时，`content.text` 可能超限

建议第一版先采用以下策略：

1. 正常按完整 JSON 拼入文本
2. 若超出安全阈值，则截断并追加提示，例如：
   - `结果过长，已截断`

这样可避免因单次结果过大而导致飞书完全拒收。

---

## 14. UI 文案建议

为降低误解，建议在页面中统一加入这些文案：

### webhook 配置页

- 当选择 `飞书机器人` 时：
  - `系统将自动按飞书机器人格式生成 payload，无需手工填写 body 模板。`

### 任务引用区

- 当任务选择某个飞书 webhook 时：
  - `已按飞书机器人协议自动匹配 payload。`

### 通用 webhook

- `通用 webhook 仍使用自定义 body_template。`

---

## 15. 实现拆分建议

建议按以下顺序实施：

1. `WebhookItem` 增加 `provider`
2. webhook 配置页增加 provider 下拉与展示逻辑
3. runtime helper 增加 provider 分发
4. 实现 `feishu_bot` 自动 payload 生成
5. 实现 `generic` 的 `body_template` 真正渲染
6. 调整任务页提示文案，去除“任务侧还要改模板”的心智
7. 补充测试推送逻辑

这样每一步都能独立验证。

---

## 16. 验收标准

以下条件全部满足，视为本轮设计达成：

1. 在 `飞连API任务` 中选择某个飞书 webhook 后，无需编辑模板，执行成功即可按飞书格式推送
2. 在 `飞连周期任务` 中选择某个飞书 webhook 后，无需编辑模板，手动和自动执行都可按飞书格式推送
3. 在 `运营agent` 中选择某个飞书 webhook 后，无需编辑模板，执行成功即可按飞书格式推送
4. 老的通用 webhook 配置不受影响
5. `generic` 仍支持手工 `body_template`
6. `feishu_bot` 不要求用户手工维护飞书 JSON 结构
7. webhook 配置页能清楚看出该配置属于哪种 provider
8. 任务页能清楚表达“payload 已按 webhook 类型自动匹配”

---

## 17. 风险与后续演进

## 17.1 第一版只优先飞书

这是有意收敛范围的结果。

后续如果要扩展其他平台，可沿用相同模式新增：

- `dingtalk_bot`
- `wecom_bot`

而不需要把任务侧模型重新推翻。

---

## 17.2 通用模板与平台自动适配并存

未来系统会同时存在两条路线：

- `generic`：用户可控模板
- `provider-specific`：系统自动适配协议

这两类都应该保留，因为它们服务的场景不同。

---

## 17.3 与当前 runtime 重构的关系

这次设计与前一份 `webhook-runtime-delivery-design` 是递进关系：

- 前一份解决“何时发”
- 这一份解决“发什么、怎么自动匹配”

两者结合后，才算真正完成“用户勾选 webhook 即可无感推送”的完整体验。
