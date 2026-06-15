# SealSuite 自动运营系统

基于 Go 语言开发的 SealSuite/飞连 API 集成与自动运营框架，用于定时获取业务数据并执行自动化运营任务。

## 📋 项目概述

本项目是一个用于对接 SealSuite（飞连）API 的自动运营系统，支持定时任务调度、数据同步、日志管理等功能，可用于自动化运维、数据采集、业务监控等场景。

## 📁 项目结构

```
SealSuite_Operation/
├── API文档/                    # API 文档目录
│   └── 飞连_Open API说明文档.pdf
├── cmd/
│   └── main.go                 # 程序入口，启动服务
├── internal/
│   ├── config/                 # 配置管理模块
│   │   └── config.go           # 配置结构定义和加载
│   ├── handler/                # 业务处理模块
│   │   └── jobs.go             # 定时任务业务逻辑
│   ├── logger/                 # 日志模块
│   │   └── logger.go           # 结构化日志实现
│   ├── scheduler/              # 定时任务调度
│   │   └── scheduler.go        # Cron 任务调度器
│   └── sealsuite/              # SealSuite API 客户端
│       └── client.go           # API 请求封装和模拟模式
├── pkg/
│   └── utils/                  # 工具函数
│       └── http.go             # HTTP 工具和签名函数
├── scripts/                    # 部署脚本
│   ├── build.sh                # 多平台编译脚本
│   ├── start.sh                # 启动脚本
│   ├── stop.sh                 # 停止脚本
│   ├── status.sh               # 状态检查脚本
│   ├── package.sh              # 打包脚本
│   ├── DEPLOY_TO_UBUNTU.md     # Ubuntu 部署指南
│   ├── README_LINUX_DEPLOY.md  # Linux 部署完整指南
│   └── systemd/
│       └── sealsuite-operation.service  # Systemd 服务配置
├── config.example.yaml         # 主配置示例文件
├── jobs.yaml                   # 旧版周期任务导入示例（首次启动可一次性迁入 SQLite）
├── api-templates.yaml          # API 模板示例数据（仓库参考，不再作为 Runner 独立运行态文件）
├── connections.example.yaml    # 飞连连接示例（历史/导入参考）
├── llm-apis.example.yaml       # LLM API 示例（历史/导入参考）
├── webhooks.example.yaml       # Webhook 示例（历史/导入参考）
├── task-drafts.example.yaml    # 任务草稿示例（历史/导入参考）
├── job-schedules.example.yaml  # 定时任务示例（历史/导入参考）
├── config.yaml                 # 本地运行配置（不提交）
├── go.mod                      # Go 模块依赖
├── go.sum
├── Makefile                    # Make 构建脚本
└── .gitignore                  # Git 忽略文件
```

## 🚀 快速开始

### 环境要求

- Go 1.21+
- Linux / macOS / Windows（跨平台支持）

### 1. 安装依赖

```bash
make tidy
```

### 2. 初始化本地配置

先从示例文件复制一份本地启动配置：

```bash
cp config.example.yaml config.yaml
```

然后根据你的环境修改这些本地文件：

- `config.yaml`

其中 `database.path` 指向业务 SQLite 文件，v0.2.0 起连接、LLM API、Webhook、模板、任务草稿、调度与运行记录都会优先落在 SQLite 中；`*.example.yaml` 主要用于初始化、示例和旧数据参考，不再是唯一运行态存储。

根目录 `jobs.yaml` 仅保留为旧版周期任务的一次性导入入口：当 SQLite 中还没有 legacy jobs 且尚未做过导入标记时，Runner 启动会尝试把它迁入 SQLite。`api-templates.yaml` 现阶段仅作为仓库示例数据保留，不再通过 `runner.New(...)` 传入或作为独立运行态文件源。

其中最关键的是：

- 飞连 `access_key / secret_key`
- 大模型 `api_key`

以下是 `config.yaml` 的基本示例：

```yaml
# SealSuite API 配置
sealsuite:
  base_url: "https://feilian-yanshi.feilian.cn"
  access_key: "your_access_key"
  secret_key: "your_secret_key"
  timeout: 30
  retry_times: 3

# 定时任务调度器配置
scheduler:
  enabled: true
  timezone: "Asia/Shanghai"

# 日志配置
log:
  level: "info"
  filename: "./logs/app.log"
  max_size: 100
  max_backups: 3
  max_age: 30
```

### 3. 编译

```bash
# 编译当前平台版本
./scripts/build.sh -c

# 编译 Linux 版本（用于部署）
./scripts/build.sh -l

# 编译所有平台版本
./scripts/build.sh -a
```

### 4. 运行

```bash
# 前台运行
./scripts/start.sh

# 后台运行
./scripts/start.sh -d
```

### 5. 管理命令

```bash
# 查看状态
./scripts/status.sh

# 停止服务
./scripts/stop.sh
```

### 6. Git 与敏感文件说明

以下文件属于**本地运行态文件或敏感配置文件**，默认已加入 `.gitignore`，不要提交到 GitHub：

- `config.yaml`
- `data/app.db`
- `connections.yaml`
- `llm-apis.yaml`
- `webhooks.yaml`
- `task-drafts.yaml`
- `job-schedules.yaml`
- `job-runs.json`
- `logs/`
- `*.pid`
- `.env`
- `.env.*`

推荐做法：

1. 只提交 `*.example.yaml`
2. 本地复制出真实配置文件再填写密钥
3. 如果密钥曾经进入 Git 历史，先轮换密钥，再推送仓库

### 7. 首次启动检查清单

建议第一次启动时按下面顺序检查：

1. 复制示例文件为本地配置文件

```bash
cp config.example.yaml config.yaml
```

2. 填写真实密钥并确认 SQLite 路径

- 在 `config.yaml` 中填写飞连 `access_key / secret_key`
- 按需调整 `database.path`（默认 `./data/app.db`）
- 大模型与 Webhook 等业务配置可在启动后通过 Web 控制台写入 SQLite

3. 检查本地敏感文件仍处于 Git 忽略状态

```bash
git status --ignored
```

确认以下文件显示为 ignored：

- `config.yaml`
- `data/app.db`
- `connections.yaml`
- `llm-apis.yaml`
- `webhooks.yaml`
- `task-drafts.yaml`
- `job-schedules.yaml`

4. 启动服务

```bash
./scripts/start.sh
```

5. 打开页面并验证基础能力

- 能正常打开 Web 控制台
- 能读取 `连接配置`
- 能看到 `LLM_API列表`
- 能保存任务草稿
- 能执行一次 `Execute`

6. 验证连接

- 先测试飞连连接是否可用
- 再测试 `LLM_API` 是否能真实返回结果
- 最后再启用周期调度与 webhook 推送

## 🔧 核心功能

### 1. 定时任务调度

- 基于 Cron 表达式的任务调度
- 支持秒级精度
- 优雅的任务包装和错误处理

### 2. API 客户端

- 封装 SealSuite API 请求
- 支持模拟模式（开发测试）
- 自动重试机制
- 统一的响应处理

### 3. 配置管理

- YAML 配置文件支持
- 环境变量覆盖
- 热加载支持（预留）

### 4. 日志系统

- 结构化 JSON 日志
- 多级别日志输出
- 控制台和文件双输出
- 日志滚动管理

### 5. 部署支持

- 多平台编译脚本
- Linux 服务化部署
- Systemd 服务配置

## 📦 部署方式

### 开发环境

```bash
go run ./cmd/main.go
```

### Linux 生产环境

```bash
# 1. 编译 Linux 版本
./scripts/build.sh -l

# 2. 打包
./scripts/package.sh -b

# 3. 传输到服务器
scp dist/sealsuite-operation_binary_*.tar.gz user@server:/tmp/

# 4. 在服务器解压部署
ssh user@server
cd /opt
sudo tar -xzf /tmp/sealsuite-operation_binary_*.tar.gz
cd sealsuite-operation
./scripts/start.sh -d
```

### Systemd 服务部署

```bash
sudo cp scripts/systemd/sealsuite-operation.service /etc/systemd/system/
sudo systemctl daemon-reload
sudo systemctl start sealsuite-operation
sudo systemctl enable sealsuite-operation
```

## 📝 扩展开发

### 添加新的 API 接口

在 `internal/sealsuite/client.go` 中添加：

```go
func (c *Client) GetUsers() (*CommonResponse, error) {
    return c.Get("/api/v1/users")
}
```

### 添加新的定时任务

在 `internal/handler/jobs.go` 中添加任务函数，然后在 `cmd/main.go` 中注册：

```go
sched.AddJob("my-job", "*/30 * * * *", jobHandler.MyJob)
```

### 外部 IP 同步定时任务

Web 控制台的“飞连任务列表-定时任务清单”支持创建 `外部 IP 同步` 类型的调度。当前第一版固定对接 Google `goog.json`，用于把 Google 公布的 IPv4 / IPv6 网段按增量方式追加到指定飞连 IP 资源。

任务要点：

- 数据源固定为 `https://www.gstatic.com/ipranges/goog.json`
- 支持 `IPv4`、`IPv6`、`IPv4 + IPv6` 三种过滤方式
- 写入策略为增量追加：仅补充目标资源中尚不存在的 CIDR
- 支持 `dry-run`，可先预览“待新增多少条”而不实际写入
- 支持 `skip-when-empty`，当没有新增 CIDR 时直接跳过写入

列表与详情页会额外展示该任务的可读摘要，例如：

- `Google IP Ranges / IPv4 -> 目标资源`
- 最近一次同步的源总量、资源现有量、待新增量、实际新增量
- 当前写入接口路径、dry-run 状态、最近一次错误信息

适合用于按天维护 Google 相关出口网段白名单，减少手工比对和重复追加。

## 🛠️ 技术栈

| 组件 | 库 | 版本 | 用途 |
|------|-----|------|------|
| 语言 | Go | 1.21+ | 主开发语言 |
| 日志 | zap | 1.26.0 | 结构化日志 |
| 配置 | viper | 1.17.0 | 配置管理 |
| 定时任务 | cron | 3.0.1 | Cron 调度器 |
| 依赖管理 | Go Modules | - | 依赖管理 |

## 📊 项目状态

- ✅ 配置管理模块
- ✅ 日志系统
- ✅ API 客户端（支持模拟模式）
- ✅ 定时任务调度
- ✅ 多平台编译脚本
- ✅ Linux 部署脚本
- ✅ Systemd 服务配置

## 📄 许可证

MIT License

## 📞 技术支持

如有问题，请查看：
- 部署文档：`scripts/DEPLOY_TO_UBUNTU.md`
- 日志文件：`logs/app.log`
- 状态检查：`./scripts/status.sh`
