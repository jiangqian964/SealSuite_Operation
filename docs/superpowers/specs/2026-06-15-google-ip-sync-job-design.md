# Google IP 同步定时任务设计

## 背景

当前“飞连任务列表-定时任务清单”已经支持以 `job_schedule` 为中心管理定时调度，但还缺少一种适合运营人员直接配置的场景化任务：定时抓取 Google 公网 IP 段，并将新增 CIDR 增量追加到指定的飞连 IP 资源中。

目标场景如下：

1. 从 `https://www.gstatic.com/ipranges/goog.json` 拉取 Google IP 段。
2. 按用户选择过滤 `IPv4`、`IPv6` 或全部。
3. 每天凌晨定时执行。
4. 调用飞连 OpenAPI：
   - `/api/open/v1/addr/management/add`
   - 或 `/api/open/v1/addr/management/update`
5. 仅把飞连目标资源中尚不存在的 Google CIDR 增量追加进去，不删除已有的其他 IP。

本设计希望让用户在“飞连任务列表-定时任务清单”中手工完成上述配置，而不需要先理解任务草稿、运营agent 或 JSON 扩展配置。

## 设计目标

- 在“定时任务清单”中新增一种场景化任务类型：`外部 IP 同步`。
- 第一版固定支持 `Google IP Ranges` 数据源。
- 目标资源支持“下拉选择已有 IP 资源 + 手填 resource_id”两种方式。
- 第一版同步策略固定为 `增量追加`。
- 列表页与详情页需要能直接看出：
  - 数据源
  - IP 版本
  - 目标资源
  - 执行时间
  - 最近一次新增了多少条
- 复用现有调度框架，不把该能力塞回旧 `legacy jobs` 结构，也不强塞进 `task_draft`。

## 非目标

第一版不做以下能力：

- 自定义外部源 URL
- 支持多数据源编排
- 全量覆盖或删除旧 Google IP
- 按 `scope / service / note` 等 Google 字段进一步筛选
- 把该能力做成任务草稿的一种特殊模式
- 用 JSON 高级编辑作为主入口

## 用户交互设计

### 新建入口

保留现有“新建调度”入口，但在创建流程里先选择任务类型：

- `常规调度`
- `外部 IP 同步`

当用户选择 `外部 IP 同步` 时，右侧表单切换为专用配置面板。

### 表单结构

#### 基础信息

- `schedule_id`
- `名称`
- `启用状态`

#### 数据源

- `数据源`
  - 固定为 `Google IP Ranges`
- `源地址`
  - 只读展示 `https://www.gstatic.com/ipranges/goog.json`
- `IP 版本`
  - `IPv4`
  - `IPv6`
  - `全部`

#### 飞连目标

- `目标 IP 资源`
  - 下拉加载飞连已有 IP 资源
- `或手动填写 resource_id`
  - 用于下拉未命中、权限未完整返回或用户已知资源 ID 的情况
- `写入接口`
  - `/api/open/v1/addr/management/add`
  - `/api/open/v1/addr/management/update`
- `写入策略`
  - 固定展示 `增量追加`
- 说明文案
  - `仅新增当前资源里还不存在的 Google CIDR，不删除已有 IP。`

#### 调度配置

- `每日定时`
- 默认推荐 `00:00`

第一版不对这个任务开放复杂 Cron 作为主入口，避免把场景做复杂。

#### 运行选项

- `空结果时不执行写入`
- `仅预览，不落库`

### 列表展示

“调度列表”中对 `外部 IP 同步` 任务，应优先展示业务语义，而不是仅显示底层字段：

- 任务类型：`外部 IP 同步`
- 数据源：`Google IP Ranges`
- 版本：`IPv4 / IPv6 / 全部`
- 目标资源：资源名或 `resource_id`
- 计划：`每日 00:00`
- 最近结果摘要：
  - `新增 x 条`
  - `跳过 y 条`
  - `成功 / 失败 / 跳过`

### 详情展示

详情区除已有调度信息外，新增同步摘要：

- 数据源 URL
- IP 版本
- 目标资源
- 写入接口路径
- 增量策略说明
- 最近一次运行结果：
  - 源数据总数
  - 过滤后数量
  - 已存在数量
  - 实际新增数量
  - 是否 dry-run
  - 错误原因

## 后端模型设计

### 推荐结构

采用“调度负责时间、任务定义负责内容”的双层模型。

#### `job_schedule`

继续沿用现有调度表/模型，新增一种目标类型：

- `target_type = external_ip_sync`
- `target_id = <external_ip_sync_task.id>`

#### `external_ip_sync_task`

新增一张独立表或独立 repository/model，核心字段建议为：

- `id`
- `name`
- `source_type`
  - 第一版固定为 `google_ip_ranges`
- `source_url`
  - 默认 `https://www.gstatic.com/ipranges/goog.json`
- `ip_version`
  - `ipv4` / `ipv6` / `all`
- `resource_id`
- `resource_name_snapshot`
- `write_action`
  - 第一版固定为 `append_if_missing`
- `feilian_api_path`
  - `/api/open/v1/addr/management/add`
  - 或 `/api/open/v1/addr/management/update`
- `dry_run`
- `skip_when_empty`
- `enabled`
- `created_at`
- `updated_at`

### 为什么不复用 `task_draft`

`task_draft` 当前更适合承载 API 查询、处理和 LLM 相关任务草稿。把“外部 IP 同步”塞进 `task_draft` 会让其边界变模糊，并让 UI 配置路径更难理解。该场景更适合作为独立任务定义类型。

## 执行链路设计

新增一个专用执行器，例如 `external_ip_sync_executor`。执行流程分五步：

### 1. 拉取 Google 源数据

请求：

`https://www.gstatic.com/ipranges/goog.json`

解析其中的：

- `ipv4Prefix`
- `ipv6Prefix`

### 2. 按配置过滤并归一化

根据用户配置的 `ip_version` 过滤出：

- 仅 IPv4
- 仅 IPv6
- 全部

然后进行：

- 去重
- 去空值
- 稳定排序

形成本次候选 CIDR 列表。

### 3. 读取飞连目标资源当前 IP

根据 `resource_id` 查询飞连目标资源当前已有 IP 列表。

这一步是增量追加的前提。

### 4. 计算差集

执行差集计算：

`本次 Google CIDR - 当前资源已有 CIDR`

得到：

- `already_exists`
- `to_add`

如果 `to_add` 为空：

- 且 `skip_when_empty = true`
  - 本次记为 `skipped`
  - 不调用飞连写入接口

### 5. 调飞连写入接口

根据用户选择调用：

- `/api/open/v1/addr/management/add`
- 或 `/api/open/v1/addr/management/update`

第一版推荐默认使用 `/add`，因为和“增量追加”的语义最一致。

如果 `dry_run = true`：

- 不实际发起写入
- 只记录本次“本应新增多少条”

## 运行结果设计

每次执行都应该产出结构化摘要，而不仅仅是成功/失败：

- `source_total`
- `filtered_total`
- `existing_total`
- `to_add_total`
- `added_total`
- `write_api_path`
- `resource_id`
- `ip_version`
- `dry_run`
- `status`
- `error_message`

该摘要用于：

- 列表页最近一次运行展示
- 详情页同步摘要展示
- 运行日志追踪

## 错误处理

错误按四类处理：

### 源数据错误

- Google URL 请求失败
- 返回不是合法 JSON
- JSON 结构变更导致解析失败

### 飞连资源错误

- `resource_id` 不存在
- 无权限读取资源

### 写入错误

- `/add` 或 `/update` 调用失败
- 部分新增失败

### 配置错误

- 缺少 `resource_id`
- IP 版本非法
- 调度保存时字段不完整

所有错误都必须写入运行摘要，而不只是日志。

## 测试建议

### 后端

- `goog.json` 解析测试
- IPv4 / IPv6 / 全部过滤测试
- 差集计算测试
- `skip_when_empty` 行为测试
- `dry_run` 不写入测试
- 调用 `/add` 与 `/update` 的请求构造测试
- `job_schedule -> external_ip_sync_task` 执行路由测试

### 前端

- 新建调度时任务类型切换测试
- `外部 IP 同步` 表单字段回填测试
- 资源下拉与 `resource_id` 手填互斥/兜底逻辑测试
- 列表摘要展示测试

## 推荐实现方式

推荐第一版采用：

- 新增 `external_ip_sync_task` 独立定义
- 新增 `target_type = external_ip_sync`
- 新增专用执行器
- UI 在“定时任务清单”中提供场景化表单

不推荐第一版：

- 把能力塞进 `task_draft`
- 仅通过高级 JSON 编辑暴露
- 继续依赖 legacy jobs 扩展字段

## 分阶段落地建议

### 第一阶段

- 后端新增模型、repository、service、执行器
- 支持 Google 数据源
- 支持 IPv4 / IPv6 / 全部
- 支持下拉选资源 + 手填 `resource_id`
- 支持每日定时
- 支持 dry-run 与 skip-when-empty

### 第二阶段

- 增加更多外部 IP 数据源
- 增强筛选维度
- 支持更多同步策略
- 为详情页增加更丰富的运行统计图表

## 最终结论

对于“每天凌晨抓 Google IP，并增量追加到飞连指定 IP 资源”这个场景，最合适的第一版设计是：

- 在“飞连任务列表-定时任务清单”里新增场景化任务类型 `外部 IP 同步`
- 调度层继续使用现有 `job_schedule`
- 任务内容层新增 `external_ip_sync_task`
- 执行层新增专用同步执行器

这样既保留当前系统的结构清晰度，也能让用户以表单方式直接完成手工配置。
