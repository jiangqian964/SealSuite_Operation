# SealSuite Operation - Linux 部署指南

## 📋 目录

- [环境要求](#环境要求)
- [快速开始](#快速开始)
- [脚本说明](#脚本说明)
- [Systemd 服务部署](#systemd-服务部署)
- [常见问题](#常见问题)

---

## 🖥️ 环境要求

- Linux 操作系统（Ubuntu 20.04+, CentOS 7+, Debian 10+ 等
- Go 1.21 或更高版本
- 至少 100MB 可用磁盘空间

检查 Go 安装：

```bash
go version
```

---

## 🚀 快速开始

### 1. 编译项目

```bash
cd /path/to/sealsuite-operation
./scripts/build.sh
```

### 2. 运行服务

#### 前台运行（调试用）

```bash
./scripts/start.sh
```

#### 后台运行（生产用）

```bash
./scripts/start.sh -d
```

### 3. 查看状态

```bash
./scripts/status.sh
```

### 4. 停止服务

```bash
./scripts/stop.sh
```

---

## 📜 脚本说明

### build.sh - 编译脚本

**功能：**
- 自动检测 Go 环境
- 下载依赖
- 编译为 Linux amd64 可执行文件
- 优化编译（strip 调试信息

**使用：**

```bash
./scripts/build.sh
```

### start.sh - 启动脚本

**功能：**
- 支持前台和后台运行模式
- 自动检测已运行的进程
- 自动创建日志目录

**使用：**

```bash
# 前台运行
./scripts/start.sh
# 或
./scripts/start.sh -f

# 后台运行
./scripts/start.sh -d

# 查看帮助
./scripts/start.sh -h
```

### stop.sh - 停止脚本

**功能：**
- 优雅停止服务
- 支持超时强制停止

**使用：**

```bash
./scripts/stop.sh
```

### status.sh - 状态脚本

**功能：**
- 查看进程状态
- 查看最新日志

**使用：**

```bash
./scripts/status.sh
```

---

## 🔧 Systemd 服务部署

### 1. 创建专用用户

```bash
sudo useradd -r -s /bin/false sealsuite
```

### 2. 部署项目

```bash
sudo mkdir -p /opt/sealsuite-operation
sudo cp -r . /opt/sealsuite-operation/
sudo chown -R sealsuite:sealsuite /opt/sealsuite-operation
```

### 3. 配置 systemd 服务

```bash
sudo cp scripts/systemd/sealsuite-operation.service /etc/systemd/system/
# 编辑服务文件，修改 WorkingDirectory 等配置
sudo nano /etc/systemd/system/sealsuite-operation.service
```

### 4. 启动服务

```bash
# 重载 systemd 配置
sudo systemctl daemon-reload

# 启动服务
sudo systemctl start sealsuite-operation

# 设置开机自启
sudo systemctl enable sealsuite-operation

# 查看状态
sudo systemctl status sealsuite-operation

# 查看日志
sudo journalctl -u sealsuite-operation -f
```

### 5. 服务管理命令

```bash
# 启动
sudo systemctl start sealsuite-operation

# 停止
sudo systemctl stop sealsuite-operation

# 重启
sudo systemctl restart sealsuite-operation

# 查看状态
sudo systemctl status sealsuite-operation

# 查看日志
sudo journalctl -u sealsuite-operation -n 100 -f
```

---

## 📁 目录结构

部署后的目录结构：

```
sealsuite-operation/
├── bin/
│   └── sealsuite-operation    # 可执行文件
├── logs/
│   ├── app.log         # 应用日志
│   └── sealsuite-operation.pid
├── cmd/
│   └── main.go
├── config.yaml         # 配置文件
├── scripts/            # 脚本目录
│   ├── build.sh
│   ├── start.sh
│   ├── stop.sh
│   ├── status.sh
│   └── systemd/
│       └── sealsuite-operation.service
└── ...
```

---

## ⚙️ 配置说明

编辑 `config.yaml` 中的配置项：

```yaml
sealsuite:
  base_url: "https://feilian-yanshi.feilian.cn"
  access_key: "your_access_key"
  secret_key: "your_secret_key"
  timeout: 30
  retry_times: 3

scheduler:
  enabled: true
  timezone: "Asia/Shanghai"

log:
  level: "info"
  filename: "./logs/app.log
  max_size: 100
  max_backups: 3
  max_age: 30
```

---

## ❓ 常见问题

### Q: 提示 "Permission denied" 权限不足

A: 确保脚本有执行权限：

```bash
chmod +x scripts/*.sh
```

### Q: 如何查看实时日志

```bash
# 使用脚本查看
tail -f logs/app.log

# 或使用状态脚本
./scripts/status.sh
```

### Q: 如何修改日志级别

编辑 `config.yaml` 中的 `log.level`：
- `debug`: 调试级别
- `info`: 信息级别（默认）
- `warn`: 警告级别
- `error`: 错误级别

### Q: 如何修改定时任务的执行频率

编辑 `cmd/main.go` 中的 cron 表达式，重新编译即可。

---

## 🔐 安全建议

1. 使用专用用户运行服务
2. 妥善保管 API 密钥
3. 定期备份日志轮转
4. 限制配置文件权限：

```bash
chmod 600 config.yaml
```

---

## 📞 技术支持

如有问题，请检查：

1. 日志文件：`logs/app.log`
2. Systemd 日志：`journalctl -u sealsuite-operation`
3. 脚本输出信息
