# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project follows [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [v0.1.0] - 2026-06-08

### Added
- 初始化飞连运维控制台项目仓库，并完成 `v0.1.0` 首次版本标记。
- 增加任务中心子页面结构，支持 `单次/周期任务清单` 与 `定时任务清单` 的切换。
- 增加任务草稿字段：
  - `cycle_mode`
  - `run_count`
  - `run_until`
- 在 `大模型调用` 区域新增具体 `LLM_API` 模型选择下拉框。
- 新增多组前端契约检查脚本，覆盖任务中心、Webhook、LLM 选择器、调度详情等关键 UI 结构。

### Changed
- 将飞连任务列表从旧 Tabs 结构调整为连接配置式子页面切换。
- 右侧 `Execute` 现在可携带：
  - `transform_config`
  - `llm_config`
  - `output_config`
- `llm_inference` 执行链优先按 `llm_config.llm_api_id` 解析具体模型，再回退到 `role` 默认模型。
- `ACTIVE` 的 `LLM_API` 角色映射逻辑已修正，默认可覆盖 `planner` 与 `formatter`。
- 输出格式逻辑已增强，优先提取大模型返回的可读 `content`，而不是直接输出整包 JSON。

### Fixed
- 修复任务草稿保存成功后，刷新任务表格时报 `targetId is not defined` 的前端运行时错误。
- 修复调度详情页把零值时间 `0001-01-01T00:00:00Z` 误显示为 `FAILED` 的问题。
- 修复指定具体 `LLM_API` 后仍错误回退到默认角色模型的问题。
- 修复 `llm_api_id` 指向已删除或已禁用模型时静默 fallback 的问题，改为明确可读错误。

### Notes
- 当前版本已配置远程仓库地址并完成本地 `git init`、首次提交与 `v0.1.0` tag。
- 推送到 GitHub 前，建议先清理仓库中的敏感配置与运行态数据。
