# SealSuite_Operation v0.1.0

首个可用版本发布。

## 版本概览

`v0.1.0` 提供了一个基于 Go 的飞连/SealSuite 自动运营控制台，包含：

- 飞连 API 接入与运行时连接配置
- Web 控制台
- API 模板管理
- 任务草稿与调度管理
- Webhook 推送能力
- 大模型调用与输出格式化链路

## 本版本主要能力

### 1. Web 控制台与配置中心

- 提供统一的 Web 控制台界面
- 支持连接配置、模板、任务、日志等模块管理
- 支持飞连连接测试、运行态状态查看和配置保存

### 2. API 模板与任务草稿

- 支持 API 模板定义与管理
- 支持从模板快速创建任务草稿
- 任务草稿支持：
  - `处理逻辑`
  - `大模型调用`
  - `输出格式`

### 3. 调度与执行

- 支持单次/周期任务清单
- 支持定时任务清单与调度详情页
- 支持手动执行与定时执行
- 支持 webhook 推送执行结果

### 4. 大模型链路增强

- `Execute` 已接入：
  - `transform_config`
  - `llm_config`
  - `output_config`
- 支持在 `大模型调用` 中直接选择具体 `LLM_API`
- 执行优先级为：
  1. `llm_config.llm_api_id`
  2. `llm_config` 显式覆盖字段
  3. `role` 默认模型

### 5. 稳定性修复

本次还修复了多项关键问题，包括：

- 任务草稿保存成功后页面误报错误
- 调度详情页把零值时间误显示为 `FAILED`
- 指定具体 `LLM_API` 后仍错误回退到默认角色模型
- 无效 `llm_api_id` 未返回明确可读错误

## 安全与仓库整理

本次发布前已完成以下整理：

- 敏感/运行态文件已加入 `.gitignore`
- 本地 Git 历史已重建，避免真实密钥进入远程历史
- 增加示例配置文件：
  - `config.example.yaml`
  - `connections.example.yaml`
  - `llm-apis.example.yaml`
  - `webhooks.example.yaml`
  - `task-drafts.example.yaml`
  - `job-schedules.example.yaml`
- 增加：
  - `CHANGELOG.md`
  - `LICENSE`
  - `SECURITY.md`

## 使用建议

首次使用建议按以下顺序：

1. 从 `*.example.yaml` 复制本地配置
2. 填写飞连与 LLM 的真实密钥
3. 启动服务并打开控制台
4. 先验证连接，再验证任务执行与调度链路

## 注意事项

- 本地运行态文件不要提交到 Git 仓库
- 若历史上曾暴露过真实密钥，请在正式使用前轮换：
  - 飞连 `access_key / secret_key`
  - LLM `api_key`
