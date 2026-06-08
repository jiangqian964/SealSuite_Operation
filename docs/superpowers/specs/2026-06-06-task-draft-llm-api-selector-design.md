# 2026-06-06 — 单次/周期任务清单大模型调用绑定具体 LLM_API 设计

## 1. 背景

当前 `飞连任务列表 -> 单次/周期任务清单 -> 单次/周期任务定义 -> 处理与输出 -> 大模型调用` 的配置方式，仍然主要依赖：

- `role = planner`
- `role = formatter`

再由运行时配置去解析对应角色的 LLM 配置。

这套模式存在两个明显问题：

1. **可见性不足**
   用户在任务草稿中无法明确知道自己实际调用的是 `LLM_API列表` 里的哪一条模型记录。

2. **控制力不足**
   即使 `LLM_API` 已经配置并连通，任务执行仍可能因为：
   - ACTIVE 角色映射错误
   - formatter / planner 未同步到同一条模型
   - 角色配置与用户预期不一致
   
   导致执行结果没有真正命中用户想用的模型。

用户已经明确提出：

> 在“大模型调用”这里增加一个配置，直接选择 `连接配置 -> LLM_API列表` 里的具体模型。

因此，本轮设计的目标是：

- 让任务草稿中的 `大模型调用` 能直接绑定某一条具体 `LLM_API`
- 不再只依赖 `role` 的隐式映射
- 保留旧草稿兼容

---

## 2. 目标

本次设计目标如下：

1. 在 `大模型调用` 区域新增“模型选择”控件
2. 模型列表直接来自 `连接配置 -> LLM_API列表`
3. 任务草稿保存时能记录所选具体模型
4. 执行时优先使用选中的具体 `LLM_API`
5. 若未选择具体模型，则继续兼容旧的 `role` 逻辑
6. 执行结果中能反查本次实际调用的模型信息

---

## 3. 非目标

本轮不做以下事情：

- 不移除现有 `role = planner / formatter` 语义
- 不重构整个 LLM 配置中心
- 不把 `LLM_API` 配置快照完整复制进任务草稿
- 不新增复杂多模型编排能力

本轮只解决：

- 任务草稿如何明确选择并绑定一条具体的 `LLM_API`

---

## 4. 方案对比

## 4.1 方案 A：绑定具体 `LLM_API` 记录 ID（推荐）

### 方案描述

在 `大模型调用` 模块增加一个下拉框，让用户从 `LLM_API列表` 中直接选择某一条具体模型记录。

任务草稿保存时，仅保存这条记录的引用标识，例如：

- `llm_api_id = llm_pro_main`

执行时根据 `llm_api_id` 实时读取该模型配置。

### 优点

- 语义最明确
- 用户清楚知道调用的是哪条模型记录
- 不依赖角色映射是否正确
- 不复制密钥等敏感配置
- 后续修改 `LLM_API` 配置时，草稿能自动跟随最新连接参数

### 缺点

- 执行阶段需要增加一层 `llm_api_id -> 具体配置` 解析

### 结论

**采用本方案。**

---

## 4.2 方案 B：把 `LLM_API` 参数完整快照进草稿

### 方案描述

用户选择某一条 `LLM_API` 后，将其：

- `provider`
- `base_url`
- `api_key`
- `model`
- `system_prompt`

等完整写入任务草稿的 `llm_config`。

### 优点

- 草稿执行时更“自包含”
- 不受后续 `LLM_API` 修改影响

### 缺点

- 密钥与配置重复存储
- 容易产生过期快照
- 配置维护复杂

### 结论

不采用。

---

## 4.3 方案 C：继续只用 role，但增强可见性

### 方案描述

不新增具体模型绑定，只在 UI 上增强提示：

- 当前 `formatter` 会使用哪条模型
- 当前 `planner` 会使用哪条模型

### 优点

- 改动最小

### 缺点

- 无法让用户真正指定具体模型
- 不能解决“任务需要绑定某条模型”的核心诉求

### 结论

不采用。

---

## 5. UI 设计

## 5.1 新增“模型选择”控件

在 `大模型调用` 区域顶部新增：

1. `模型选择`
2. `模型说明/状态提示`

布局建议：

- 上方：具体模型下拉框
- 下方：保留原 `llm_config` JSON 文本框

这样用户的操作路径会变成：

1. 先选具体模型
2. 再填写 prompt、role、temperature 等调用参数

---

## 5.2 下拉框数据来源

下拉框直接读取：

- `连接配置 -> LLM_API列表`

展示内容建议包含：

- `name`
- `provider`
- `model`
- `ACTIVE` 状态（如适用）

示例展示文案：

- `主模型 · deepseek / deepseek-v4-flash · ACTIVE`
- `摘要模型 · openai / gpt-4.1-mini`

如果没有可用模型，显示：

- `暂无可用 LLM_API，请先到连接配置创建并启用`

---

## 5.3 JSON 文本框语义调整

新增模型选择后，`大模型调用` 文本框不再要求用户手动填写连接参数：

- 不再鼓励手写 `provider / base_url / api_key / model`
- 优先让用户只填写调用意图参数

例如：

- `role`
- `prompt`
- `system_prompt`
- `temperature`
- `max_tokens`
- `response_format`

也就是说：

- **模型连接参数** 由下拉选择提供
- **调用行为参数** 由 JSON 文本框提供

---

## 6. 草稿存储设计

## 6.1 存储位置

推荐将具体模型引用保存在：

- `llm_config.llm_api_id`

而不是新增顶层字段。

示例：

```json
{
  "role": "formatter",
  "llm_api_id": "llm_pro_main",
  "prompt": "请把输入结果整理成简洁中文摘要，列出关键数据和异常项。"
}
```

### 原因

- 语义集中在 `llm_config`
- 与现有 `transform_config / llm_config / output_config` 结构一致
- 最小化对任务草稿整体结构的影响

---

## 6.2 草稿回填

加载任务草稿时：

- 如果 `llm_config.llm_api_id` 有值
  - 下拉框应回填到对应模型
- 同时文本框继续显示其余 `llm_config`

如果引用的 `llm_api_id` 已不存在：

- 下拉框显示空
- 页面给出提示：
  - `当前草稿引用的模型不存在或已删除`

---

## 7. 执行逻辑设计

## 7.1 优先级规则

执行 `llm_inference` 时，配置来源优先级调整为：

### 第一优先级：`llm_api_id`

如果 `llm_config.llm_api_id` 有值：

- 从 `LLM_APIStore` 中读取该条记录
- 用这条记录构造最终 LLM 连接参数

### 第二优先级：当前 `llm_config` 中的显式覆盖字段

例如：

- `system_prompt`
- `temperature`
- `max_tokens`
- `response_format`

这些仍允许在任务草稿里覆盖默认模型参数。

### 第三优先级：旧 `role` 逻辑

如果没有指定 `llm_api_id`：

- 继续按 `role = planner / formatter`
- 从当前运行时配置中读取对应角色

---

## 7.2 实际解析顺序

建议后端执行顺序为：

1. 读取 `llm_config.llm_api_id`
2. 如果存在：
   - 去 `LLM_APIStore` 查询该记录
   - 构造基础 LLM 配置
3. 再用 `llm_config` 中的调用参数覆盖
4. 如果 `llm_api_id` 不存在：
   - 返回明确错误
5. 如果没有 `llm_api_id`：
   - 回退到旧的 `resolveLLMRoleConfig(role, cfg)`

---

## 7.3 错误提示

如果用户选择了某个模型，但执行时该模型不可用，应返回可读错误：

- `指定的 LLM_API 不存在：llm_pro_main`
- `指定的 LLM_API 已禁用`
- `指定的 LLM_API 缺少 base_url/api_key/model`

不要再只返回笼统的：

- `当前未真正调用大模型`

因为这类错误已经可以定位到具体模型记录。

---

## 8. 结果回显设计

## 8.1 执行结果元信息

为方便调试，建议在 LLM 成功执行后的结果中附带：

- `llm_api_id`
- `provider`
- `model`
- `role`

示例：

```json
{
  "llm_api_id": "llm_pro_main",
  "provider": "deepseek",
  "model": "deepseek-v4-flash",
  "role": "formatter",
  "content": "……模型真正生成的摘要……"
}
```

这样用户在执行输出中能明确知道：

- 这次到底调用了哪条具体模型

---

## 8.2 预览结果元信息

如果模型调用失败或回退为预览结果，也应带上：

- `llm_api_id`
- `provider`
- `model`
- `role`

这样用户能看出：

- 是哪条模型引用出了问题

---

## 9. 兼容策略

## 9.1 兼容旧草稿

旧草稿没有 `llm_api_id` 时：

- 不报错
- 继续按旧逻辑执行

即：

- `role=planner` → 读取 planner 角色配置
- `role=formatter` → 读取 formatter 角色配置

---

## 9.2 兼容现有 UI

如果当前草稿的 `llm_config` 中仍写了：

- `provider`
- `base_url`
- `api_key`
- `model`

短期内允许保留，但在 UI 语义上不再鼓励这样使用。

后续可以考虑逐步弱化。

---

## 10. 验收标准

以下条件全部满足，视为本轮设计达成：

1. `大模型调用` 区域新增可选具体模型的下拉框
2. 下拉框数据来自 `LLM_API列表`
3. 任务草稿保存后，`llm_config.llm_api_id` 被正确写入
4. 加载草稿时，下拉框能正确回填选中的模型
5. 执行时若 `llm_api_id` 存在，优先使用该模型
6. 若 `llm_api_id` 不存在，继续兼容旧 `role` 逻辑
7. 执行结果中能看出实际调用的是哪条模型
8. 若指定模型失效，返回可读错误信息

---

## 11. 风险与注意事项

## 11.1 模型被删除或禁用

任务草稿绑定的是引用，不是快照。

因此：

- 如果 `LLM_API` 被删除
- 或后续被禁用

任务执行时会失败。

这是可接受的，但必须给出明确错误提示。

---

## 11.2 与 role 的关系要清楚

新增 `llm_api_id` 后，`role` 不应再承担“选择连接模型”的职责。

它更适合保留为：

- 业务语义角色
- prompt 风格语义

而不是实际连接路由。

---

## 11.3 不要把敏感配置复制进草稿

本轮必须坚持“引用记录”的方向，不要为了图省事把：

- `api_key`
- `base_url`
- `model`

完整复制进每一条任务草稿。

---

## 12. 推荐实施顺序

建议按以下顺序推进：

1. 在 UI 中增加 `LLM_API` 下拉选择
2. 将 `llm_api_id` 写入 `llm_config`
3. 加载草稿时支持回填
4. 后端执行链优先解析 `llm_api_id`
5. 为结果增加模型元信息
6. 补兼容与错误提示测试

---

## 13. 字段与优先级摘要

### 13.1 字段映射摘要

| 区域 | 字段 | 用途 |
| --- | --- | --- |
| UI 下拉选择 | `draft-llm-api-id` | 让用户直接选择 `连接配置 -> LLM_API列表` 中的具体模型 |
| 草稿存储 | `llm_config.llm_api_id` | 保存所选 `LLM_API` 的记录引用 |
| 草稿文本框 | `llm_config.role / prompt / system_prompt / temperature / max_tokens / response_format` | 保存调用行为参数与业务语义参数 |
| 执行结果元信息 | `llm_api_id / provider / model / role` | 回显本次实际命中的模型与角色信息，便于排查 |

### 13.2 执行优先级摘要

执行 `llm_inference` 时按以下顺序解析：

1. `llm_config.llm_api_id`
   - 若存在，优先按该 ID 到 `LLM_APIStore` 解析具体模型配置
2. `llm_config` 中的显式覆盖字段
   - 例如 `system_prompt / temperature / max_tokens / response_format`
   - 这些字段可覆盖所选模型记录中的默认值
3. `role` 对应的运行时默认模型
   - 仅当未指定 `llm_api_id` 时启用旧逻辑
   - `planner` / `formatter` 仍作为兼容回退路径存在

### 13.3 一句话规则

- 下拉选择保存到：`llm_config.llm_api_id`
- 连接模型选择优先看：`llm_api_id`
- 调用参数覆盖优先看：`llm_config` 显式字段
- 只有没选 `llm_api_id` 时，才回退到 `role` 的默认模型
