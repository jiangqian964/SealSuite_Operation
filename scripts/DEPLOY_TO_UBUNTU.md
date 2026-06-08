# SealSuite Operation - Ubuntu 部署指南

## 📋 目录

- [快速开始](#快速开始)
- [方案对比](#方案对比)
- [方案一：完整打包（推荐新手）](#方案一完整打包推荐新手)
- [方案二：仅打包编译产物（推荐生产）](#方案二仅打包编译产物推荐生产)
- [方案三：Git 部署（推荐开发）](#方案三git-部署推荐开发)
- [远程传输方式](#远程传输方式)

---

## 🚀 快速开始

**最简单的方式：**

```bash
# 1. 在本地 macOS 上打包完整项目
./scripts/package.sh -f

# 2. 传输到 Ubuntu 服务器
scp dist/sealsuite-operation_full_*.tar.gz user@your-server:/tmp/

# 3. 在 Ubuntu 服务器上解压部署
ssh user@your-server
cd /opt
sudo tar -xzf /tmp/sealsuite-operation_full_*.tar.gz
cd sealsuite-operation
sudo ./scripts/build.sh
sudo ./scripts/start.sh -d
```

---

## 💡 方案对比

| 方案 | 服务器需要 Go | 体积 | 灵活性 | 推荐场景 |
|------|--------------|------|--------|---------|
| **完整打包** | ✅ 需要 | 大 | 高 | 开发/测试环境 |
| **仅打包编译产物** | ❌ 不需要 | 小 | 中 | 生产环境 |
| **Git 部署** | ✅ 需要 | 小 | 最高 | 有版本控制需求 |

---

## 📦 方案一：完整打包（推荐新手）

**优点：**
- 可以在服务器上修改代码
- 部署流程简单
- 包含所有脚本和文档

**步骤：**

### 1. 在本地 macOS 上打包

```bash
cd /path/to/sealsuite-operation
./scripts/package.sh -f
```

这会在 `dist/` 目录下生成一个包含完整项目的压缩包。

### 2. 传输到 Ubuntu 服务器

```bash
# 使用 scp 传输（替换 user 和 your-server）
scp dist/sealsuite-operation_full_*.tar.gz user@your-server:/tmp/
```

### 3. 在 Ubuntu 服务器上部署

```bash
# SSH 连接到服务器
ssh user@your-server

# 解压到 /opt 目录
cd /opt
sudo tar -xzf /tmp/sealsuite-operation_full_*.tar.gz
cd sealsuite-operation

# 编译项目
sudo ./scripts/build.sh

# 修改配置文件（重要！）
sudo nano config.yaml

# 启动服务
sudo ./scripts/start.sh -d

# 查看状态
sudo ./scripts/status.sh
```

---

## 🎯 方案二：仅打包编译产物（推荐生产）

**优点：**
- 体积小，传输快
- 服务器不需要安装 Go
- 更安全（不暴露源代码）

**步骤：**

### 1. 在本地 macOS 上先编译并打包

```bash
cd /path/to/sealsuite-operation

# 先编译（会自动编译为 Linux 可执行文件）
./scripts/build.sh

# 打包编译产物
./scripts/package.sh -b
```

### 2. 传输到 Ubuntu 服务器

```bash
scp dist/sealsuite-operation_binary_*.tar.gz user@your-server:/tmp/
```

### 3. 在 Ubuntu 服务器上部署

```bash
# SSH 连接到服务器
ssh user@your-server

# 解压到 /opt 目录
cd /opt
sudo tar -xzf /tmp/sealsuite-operation_binary_*.tar.gz
cd sealsuite-operation

# 设置执行权限
sudo chmod +x scripts/*.sh bin/sealsuite-operation

# 修改配置文件（重要！）
sudo nano config.yaml

# 启动服务
sudo ./scripts/start.sh -d

# 查看状态
sudo ./scripts/status.sh
```

---

## 🔄 方案三：Git 部署（推荐开发）

**如果代码在 Git 仓库中，可以这样部署：**

### 在 Ubuntu 服务器上：

```bash
# 1. 安装 Git 和 Go
sudo apt update
sudo apt install -y git golang-go

# 2. 克隆项目
cd /opt
sudo git clone https://your-git-repo/sealsuite-operation.git
cd sealsuite-operation

# 3. 编译并运行
sudo ./scripts/build.sh
sudo ./scripts/start.sh -d
```

**更新代码：**

```bash
cd /opt/sealsuite-operation
sudo git pull
sudo ./scripts/stop.sh
sudo ./scripts/build.sh
sudo ./scripts/start.sh -d
```

---

## 🔗 远程传输方式

### SCP（最简单）

```bash
# 传输文件
scp dist/sealsuite-operation_full_*.tar.gz user@your-server:/tmp/

# 传输整个目录
scp -r dist/ user@your-server:/tmp/
```

### Rsync（推荐，支持增量同步）

```bash
# 同步本地项目到服务器
rsync -avz --exclude='.git' --exclude='logs' --exclude='dist' \
    ./ user@your-server:/opt/sealsuite-operation/
```

### SFTP（图形化工具）

可以使用：
- FileZilla
- WinSCP（Windows）
- Transmit（macOS）

---

## ⚙️ Ubuntu 服务器环境准备

### 1. 基础工具安装

```bash
sudo apt update
sudo apt install -y curl wget tar vim
```

### 2. 如果使用方案一，需要安装 Go

```bash
# 下载 Go（访问 https://go.dev/dl/ 查看最新版本）
wget https://go.dev/dl/go1.21.10.linux-amd64.tar.gz

# 解压
sudo tar -C /usr/local -xzf go1.21.10.linux-amd64.tar.gz

# 配置环境变量
echo 'export PATH=$PATH:/usr/local/go/bin' >> ~/.bashrc
source ~/.bashrc

# 验证安装
go version
```

### 3. 创建专用用户

```bash
sudo useradd -r -s /bin/false sealsuite
sudo mkdir -p /opt/sealsuite-operation
sudo chown -R sealsuite:sealsuite /opt/sealsuite-operation
```

---

## 📋 完整部署检查清单

- [ ] 打包项目（根据需求选择方案）
- [ ] 传输到服务器
- [ ] 解压到正确位置（推荐 /opt）
- [ ] 修改 config.yaml 配置
- [ ] 测试运行服务
- [ ] 配置 Systemd 服务（可选，生产环境推荐）
- [ ] 验证服务正常运行
- [ ] 配置防火墙（如需要）

---

## 💻 示例部署流程（方案一）

```bash
# ========== 本地 macOS ==========
# 1. 打包
cd ~/Documents/trae_projects/SealSuite_Operation
./scripts/package.sh -f

# 2. 传输（假设服务器是 192.168.1.100，用户是 ubuntu）
scp dist/sealsuite-operation_full_*.tar.gz ubuntu@192.168.1.100:/tmp/

# ========== 服务器 Ubuntu ==========
# 3. 连接服务器
ssh ubuntu@192.168.1.100

# 4. 解压
cd /opt
sudo tar -xzf /tmp/sealsuite-operation_full_*.tar.gz
cd sealsuite-operation

# 5. 编译
sudo ./scripts/build.sh

# 6. 启动服务
sudo ./scripts/start.sh -d

# 7. 检查状态
sudo ./scripts/status.sh

# 8. 查看日志
tail -f logs/app.log
```

---

## 🔧 故障排除

### 问题：权限拒绝 Permission denied

```bash
# 确保脚本有执行权限
sudo chmod +x scripts/*.sh bin/sealsuite-operation
```

### 问题：找不到命令

```bash
# 确保在正确的目录
cd /opt/sealsuite-operation

# 或者使用完整路径
/opt/sealsuite-operation/scripts/start.sh -d
```

### 问题：无法连接到 API

检查 `config.yaml` 中的 API 地址和密钥是否正确。

---

## 📞 需要帮助？

查看详细文档：
- [Linux 部署完整指南](./README_LINUX_DEPLOY.md)
- 查看服务状态：`./scripts/status.sh`
- 查看日志：`tail -f logs/app.log`
