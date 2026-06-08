# 模板任务模式使用说明

## 什么是模板任务模式

模板任务模式 = **先定义一个标准 API 请求模板，再让 Jobs 按计划定时执行这个模板**。

在这种模式下：
- 模板负责定义请求结构
- Job 负责定义调度规则和输入参数

你在 Jobs 里通常不会直接填写：
- `Headers`
- `Method`
- `Path`

你主要填写的是：
- `template_id`
- `cron`
- `input`
- `safety`

---

## 模板和 Job 分别负责什么

### 模板负责
- `method`
- `path`
- `query_schema`
- `path_params_schema`
- `body_schema`
- `dry_run_query_param`

### Job 负责
- `name`
- `enabled`
- `cron`
- `type`
- `params.template_id`
- `params.input`
- `params.safety`

---

## Headers 怎么处理

在模板任务模式下，请求头由系统自动注入，不需要手动填写：

- `Content-Type`：系统自动加
- `Authorization`：系统自动获取 `access_token` 后自动加

这意味着模板里只关心“请求结构”，不关心鉴权细节。

---

## 查询任务怎么创建

目标示例：每 5 分钟查询一次用户列表

操作步骤：
1. 打开 **API 工具箱**
2. 在 **飞连API列表** 中找到查询类模板，例如 `users_list`
3. 先手动测试模板，确认能正常执行
4. 打开 **Jobs**
5. 新建任务并填写：
   - `name`：例如 `users-check-5m`
   - `type`：`api_poll`
   - `cron`：例如 `0 */5 * * * *`
   - `template_id`：`users_list`
   - `input`：例如 `page=1`、`page_size=200`
6. 保存并启用任务

---

## 写入任务怎么创建

目标示例：每周一凌晨更新一次策略

操作步骤：
1. 在 **API 工具箱** 中选择写入类模板，例如 `policy_update`
2. 先执行一次：
   - Preview
   - dry-run / diff
3. 确认 body 正确后，再进入 **Jobs**
4. 新建任务并填写：
   - `name`：例如 `weekly-policy-update`
   - `type`：`api_write`
   - `cron`：例如 `0 0 2 * * 1`
   - `template_id`：`policy_update`
   - `input`：如 `policy_id`
   - `body` / `__raw_json__`：写入内容
   - `safety`：例如 `preview_confirmed=true`
5. 保存，并按安全策略决定是否启用

---

## 什么时候适合用模板任务模式

适合：
- 稳定、常用、会重复执行的飞连 API
- 需要复用请求结构的周期任务
- 希望统一管理查询类 / 写入类标准 API 的场景

---

## 什么时候不适合用模板任务模式

不适合：
- 临时试验从未定义过的新接口
- 需要自由指定 `Method / Path`
- 需要做通用请求型周期任务

这种情况下，更适合未来扩展的：
- **通用请求任务模式**

---

## 常见问题

### 为什么 Jobs 新建任务里没有 `Method` 和 `Path`？
因为当前 Jobs 走的是模板驱动模式，`Method / Path` 由模板定义，Job 只关心调度和输入参数。

### 为什么看不到 `Headers` 配置？
因为系统会统一自动带上：
- `Content-Type`
- `Authorization`

### 写入类任务为什么建议先 Preview？
因为写入类任务影响真实数据，Preview / dry-run 可以帮助确认变更内容是否符合预期，降低误操作风险。
