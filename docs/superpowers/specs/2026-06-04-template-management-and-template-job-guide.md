# API 工具箱模板管理 + 模板任务模式使用说明

日期：2026-06-04  
范围：本地 Web 控制台中的 API 工具箱模板列表、Jobs 模块中的模板任务模式

---

## 1. 背景

当前系统已经有：
- `api-templates.yaml`：模板定义
- API 工具箱中的模板列表：可展示并辅助填入 `template_id`
- Jobs 中的模板型任务：通过 `template_id` 定时执行查询或写入

但目前仍存在两个明显问题：
1. 模板列表还不能在 Web 页面里做完整管理，缺少“新增模板 / 测试模板 / 删除模板 / 查看详情”
2. 用户对“模板任务模式”的理解不清晰，不知道它与自由请求、通用任务之间的边界和操作方式

因此本稿同时解决两件事：
- 提供一份**模板任务模式操作说明**
- 设计一版**模板列表管理能力**

---

## 2. 模板任务模式操作说明

### 2.1 一句话理解
模板任务模式 = **先定义一个标准 API 请求模板，再让 Jobs 按计划定时执行这个模板。**

你在 Jobs 中不直接填写：
- `Headers`
- 通常也不直接填写 `Method`
- 通常也不直接填写 `Path`

你主要填写的是：
- `template_id`
- `cron`
- `input`
- `safety`（写入类任务常用）

### 2.2 模板和任务的分工

#### 模板负责
- `method`
- `path`
- `query_schema`
- `path_params_schema`
- `body_schema`
- `dry_run_query_param`

#### Job 负责
- `name`
- `enabled`
- `cron`
- `type`
- `params.template_id`
- `params.input`
- `params.safety`

### 2.3 为什么要这样设计
模板任务模式的好处：
- **减少重复配置**：常用 API 的 `method/path` 不必每次重填
- **更安全**：写入类请求可统一挂 dry-run / preview 约束
- **更易维护**：多个任务可复用同一模板
- **更适合运维**：把“请求结构”和“调度规则”分离

### 2.4 Headers 怎么处理
在模板任务模式下，Headers 由系统统一注入，不让用户手动配置：
- `Content-Type`：系统自动带
- `Authorization`：系统自动先获取 `access_token`，再自动带入

这意味着：
- 用户无需在模板中配置这两个请求头
- 模板的重点是请求结构，而不是鉴权细节

### 2.5 查询任务怎么建

目标示例：每 5 分钟查询一次用户列表

操作步骤：
1. 打开 **API 工具箱**
2. 在 **模板列表** 找到查询类模板，例如 `users_list`
3. 先手动测试该模板，确认能正常执行
4. 打开 **Jobs**
5. 新建任务：
   - `name`：例如 `users-check-5m`
   - `type`：`api_poll`
   - `cron`：例如 `0 */5 * * * *`
   - `template_id`：`users_list`
   - `input`：例如 `page=1`、`page_size=200`
6. 保存并启用任务

### 2.6 写入任务怎么建

目标示例：每周一凌晨更新一次策略

操作步骤：
1. 在 **API 工具箱** 中选择写入类模板，例如 `policy_update`
2. 先在单次调试区做：
   - Preview
   - dry-run / diff
3. 确认 body 正确后，再进入 **Jobs**
4. 新建任务：
   - `name`：例如 `weekly-policy-update`
   - `type`：`api_write`
   - `cron`：例如 `0 0 2 * * 1`
   - `template_id`：`policy_update`
   - `input`：如 `policy_id`
   - `body` / `__raw_json__`：写入内容
   - `safety`：例如 `preview_confirmed=true`
5. 保存，并按安全策略决定是否直接启用

### 2.7 模板任务模式适合什么
适合：
- 稳定、常用、会重复执行的飞连 API
- 需要复用请求结构的周期任务
- 希望统一管理查询类 / 写入类标准 API 的场景

### 2.8 模板任务模式不适合什么
不适合：
- 临时试验从未定义过的新接口
- 需要自由指定 `Method / Path`
- 需要做通用请求型周期任务

这种情况下，更适合未来的：
- **通用请求任务模式**

---

## 3. API 工具箱模板列表增强设计

### 3.1 目标
把“模板列表”从只读展示升级成真正的模板管理中心：
- 查看模板列表
- 新增模板
- 查看模板详情
- 测试模板
- 删除模板
- 后续支持编辑模板

### 3.2 核心原则
- 模板不暴露系统鉴权头配置
- `Content-Type` / `Authorization` 继续由系统自动注入
- 模板本身只定义请求结构
- 保留“从调试器一键保存为模板”的快捷路径

---

## 4. 页面结构设计

### 4.1 模板列表页

展示字段：
- `id`
- `name`
- `category`
- `method`
- `path`
- 是否支持 dry-run

支持操作：
- 搜索
- 按 category 筛选
- 新增模板
- 点击进入详情
- 行内测试
- 行内删除

### 4.2 模板详情页

展示内容：
- 基础信息：`id / name / category`
- 请求定义：`method / path`
- schema：
  - `query_schema`
  - `path_params_schema`
  - `body_schema`
- dry-run 配置

支持操作：
- 测试模板
- 编辑模板
- 删除模板

### 4.3 模板编辑页

支持：
- 新建模板
- 编辑模板

字段：
- `id`
- `name`
- `category`
- `method`
- `path`
- `query_schema`
- `path_params_schema`
- `body_schema`
- `dry_run_query_param`

模式：
- **基础表单模式**
- **JSON 高级模式**

建议：
- 常见字段走表单
- 复杂 schema 走 JSON 模式

### 4.4 模板测试窗口

模板测试应支持输入：
- path params
- query
- body

输出：
- HTTP 状态
- 业务 `code/message`
- 响应 body
- 若为写入类模板，可提供 Preview/dry-run 测试入口

---

## 5. 后端 API 设计

### 5.1 当前现状
当前已存在：
- `GET /api/v1/api/templates`
- `PUT /api/v1/api/templates`

问题：
- 仍是整文件读写接口，不适合细粒度前端模板管理

### 5.2 建议新增资源化接口

- `GET /api/v1/api/templates`
  - 获取模板列表

- `GET /api/v1/api/templates/{id}`
  - 获取单模板详情

- `POST /api/v1/api/templates`
  - 新建模板

- `PUT /api/v1/api/templates/{id}`
  - 更新模板

- `DELETE /api/v1/api/templates/{id}`
  - 删除模板

- `POST /api/v1/api/templates/{id}/test`
  - 用输入变量测试该模板

底层仍写回：
- `api-templates.yaml`

---

## 6. 删除模板的保护策略

建议删除模板前扫描 `jobs.yaml`：
- 若有 Job 正在引用该模板
- 默认**禁止直接删除**
- 返回提示：哪些任务引用了该模板

这样可以避免：
- 模板被删后，周期任务执行失败

---

## 7. 建议落地顺序

### 第一阶段
- 新增模板
- 模板列表增强
- 测试模板
- 删除模板
- 模板任务模式使用说明文档

### 第二阶段
- 模板详情页
- 模板编辑页
- 表单 + JSON 双模式
- 删除模板引用检查

---

## 8. 验收标准（DoD）

1. 有一份明确的《模板任务模式使用说明》  
2. API 工具箱模板列表支持新增模板  
3. 模板列表支持测试模板  
4. 模板列表支持删除模板  
5. 模板管理不要求用户手动配置 `Content-Type` / `Authorization`  
6. 系统仍能自动注入鉴权与内容类型请求头  
7. 后续删除模板时，可检查是否被 Jobs 引用  

