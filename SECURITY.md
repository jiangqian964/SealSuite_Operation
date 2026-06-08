# Security Policy

## 支持范围

当前仓库以最新版本为主进行维护。若你发现安全问题，请优先基于当前最新提交或最新 tag 复现。

## 报告方式

如果你发现潜在安全问题，请不要直接在公开 issue 中披露真实密钥、访问地址或可利用细节。

建议最少提供以下信息：

- 问题描述
- 影响范围
- 复现步骤
- 预期行为与实际行为
- 是否涉及密钥、令牌、Webhook 地址或运行态配置

## 敏感信息处理要求

以下文件默认属于本地敏感配置或运行态文件，不应提交到 Git 仓库：

- `config.yaml`
- `connections.yaml`
- `llm-apis.yaml`
- `webhooks.yaml`
- `task-drafts.yaml`
- `job-schedules.yaml`
- `job-runs.json`
- `logs/`
- `.env`
- `.env.*`

仓库中应只提交示例模板，例如：

- `config.example.yaml`
- `connections.example.yaml`
- `llm-apis.example.yaml`
- `webhooks.example.yaml`
- `task-drafts.example.yaml`
- `job-schedules.example.yaml`

## 密钥泄露处理建议

如果你怀疑以下信息已经泄露：

- 飞连 `access_key / secret_key`
- LLM `api_key`
- Webhook 地址或认证头

请立即执行以下动作：

1. 轮换或撤销对应密钥
2. 检查 Git 历史中是否存在真实敏感值
3. 确认 `.gitignore` 仍正确忽略本地运行态文件
4. 重新生成不含敏感信息的提交后再推送

## 推送前检查

推送到远程仓库前，建议至少执行：

```bash
git status --ignored
git grep -n -I -E "(api_key:|access_key:|secret_key:|sk-[A-Za-z0-9_-]{16,})" HEAD -- .
```

若发现真实密钥，请先清理本地提交历史，再执行推送。
