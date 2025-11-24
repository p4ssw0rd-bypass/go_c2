# Go_C&C - Command & Control Center

<div align="center">

![Go Version](https://img.shields.io/badge/Go-1.23+-blue.svg)
![Vue Version](https://img.shields.io/badge/Vue-3.4+-green.svg)
![License](https://img.shields.io/badge/License-MIT-yellow.svg)
![Platform](https://img.shields.io/badge/Platform-Windows%20%7C%20Linux%20%7C%20macOS-lightgrey.svg)

**一个基于Go语言开发的命令与控制中心，提供Beacon管理、文件操作、命令执行等核心功能**

**此项目更适于内部钓鱼演练**

[🚀 快速开始](#-快速开始) • [📖 文档](#-功能特性) • [🛠️ 部署](#-部署指南) • [🔧 开发](#-开发指南)

</div>

---

## ✨ 项目概述

**Go_C&C** 是一个专为网络安全研究和渗透测试设计的现代化命令与控制中心。项目采用前后端分离架构，后端基于Go语言开发，前端使用Vue3构建，提供直观的Web管理界面。

### 🎯 核心特性

- **🔐 安全认证**: JWT token认证和权限控制
- **📡 Beacon管理**: 实时监控和管理所有活跃连接
- **📁 文件操作**: 支持文件上传、下载、列表等操作
- **⚡ 任务调度**: 智能任务队列管理和超时处理
- **🎨 现代化UI**: 基于Vue3 + Element Plus的美观界面
- **🌐 流量伪装**: 支持自定义流量伪装配置
- **📊 实时监控**: 实时状态监控和日志记录

## 🏗️ 技术架构

### 后端技术栈
- **语言**: Go 1.23+
- **Web框架**: Gin
- **数据库**: MySQL 8.0+
- **认证**: JWT
- **加密**: AES-256-GCM

### 前端技术栈
- **框架**: Vue 3.4+
- **UI组件**: Element Plus
- **构建工具**: Vite
- **状态管理**: Pinia
- **路由**: Vue Router 4

### Beacon客户端
- **语言**: C
- **编译**: 支持跨平台编译
- **通信**: HTTP/HTTPS
- **加密**: SSL/TLS支持

## 📁 项目结构

```
GO_C&C/
├── 📁 beacon/                    # C语言Beacon客户端
│   ├── beacon.c                  # 主程序逻辑
│   ├── beacon.h                  # 头文件
│   ├── config.h                  # Beacon配置
│   ├── tasks.c                   # 任务处理逻辑
│   ├── http.c                    # HTTP通信模块
│   ├── utils.c                   # 工具函数
│   └── build.bat                 # Windows编译脚本
├── 📁 webserver/                 # Go语言服务器
│   ├── 📁 backend/               # 后端服务
│   │   ├── main.go               # 主程序入口
│   │   ├── config.json           # 服务器配置
│   │   ├── 📁 handler/           # 请求处理器
│   │   ├── 📁 db/                # 数据库操作
│   │   ├── 📁 utils/             # 工具函数
│   │   └── 📁 storage/           # 文件存储
│   └── 📁 frontend/              # Vue3前端
│       ├── 📁 src/               # 源代码
│       ├── package.json          # 依赖配置
│       └── vite.config.ts        # 构建配置
└── 📄 README.md                  # 项目说明文档
```

## 🚀 快速开始

### 环境要求

| 组件 | 版本要求 | 说明 |
|------|----------|------|
| **Go** | 1.23+ | 后端开发环境 |
| **MySQL** | 8.0+ | 数据库服务 |
| **Node.js** | 16.0+ | 前端开发环境 |
| **GCC/MSVC** | 最新版本 | Beacon编译 |

### 1️⃣ 克隆项目

```bash
git clone <your-repository-url>
cd GO_C&C
```

### 2️⃣ 配置数据库

```sql
-- 创建数据库
CREATE DATABASE sql CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci;

-- 创建用户（可选）
CREATE USER 'sql'@'localhost' IDENTIFIED BY 'your_password';
GRANT ALL PRIVILEGES ON sql.* TO 'sql'@'localhost';
FLUSH PRIVILEGES;
```

### 3️⃣ 配置后端

```bash
cd webserver/backend

# 复制配置模板
cp config.json.example config.json

# 编辑配置文件
vim config.json
```

**关键配置项**:
```json
{
  "database": {
    "mysql": {
      "host": "127.0.0.1",
      "port": 3306,
      "user": "root",
      "password": "your_password",
      "dbname": "gocc"
    }
  },
  "admin_user": "admin",
  "admin_pass": "admin123",
  "jwt_secret": "your-very-long-and-random-jwt-secret-key"
}
```

### 4️⃣ 启动后端服务

```bash
# 安装依赖
go mod tidy

# 编译运行
go run main.go

# 或编译后运行
go build -o webserver .
./webserver
```

### 5️⃣ 启动前端服务

```bash
cd webserver/frontend

# 安装依赖
npm install

yara insall

# 开发模式
npm run dev

# 生产构建
npm run build
```

### 6️⃣ 访问管理界面

- **URL**: http://localhost:18080
- **默认账号**: `admin`
- **默认密码**: `admin123`

## 🔧 部署指南

### Docker部署（推荐）

```dockerfile
# Dockerfile
FROM golang:1.23-alpine AS builder
WORKDIR /app
COPY . .
RUN go mod download && go build -o webserver .

FROM alpine:latest
RUN apk --no-cache add ca-certificates
WORKDIR /root/
COPY --from=builder /app/webserver .
EXPOSE 18080
CMD ["./webserver"]
```

### 系统服务部署

```bash
# 创建系统服务文件
sudo tee /etc/systemd/system/goc2.service << EOF
[Unit]
Description=Go_C&C Server
After=network.target

[Service]
Type=simple
User=goc2
WorkingDirectory=/opt/goc2
ExecStart=/opt/goc2/webserver
Restart=always
RestartSec=5

[Install]
WantedBy=multi-user.target
EOF

# 启动服务
sudo systemctl enable goc2
sudo systemctl start goc2
```

## 📖 功能特性详解

<img width="2494" height="1323" alt="image" src="https://github.com/user-attachments/assets/2233b444-021b-441c-9e61-cc84c50f1dbe" />

### 🔐 Beacon管理

- **连接监控**: 实时显示所有活跃Beacon
- **状态跟踪**: 心跳检测和连接状态管理
- **备注管理**: 为每个Beacon添加描述信息
- **生命周期**: 支持Beacon的创建、暂停和销毁

### 📁 文件操作

- **文件上传**: 支持大文件上传和断点续传
- **文件下载**: 从目标机器下载指定文件
- **文件浏览**: 浏览目标机器文件系统
- **存储管理**: 自动管理本地文件存储

### ⚡ 任务调度

- **任务队列**: 智能任务排队和优先级管理
- **超时处理**: 自动处理任务超时和重试
- **状态跟踪**: 实时监控任务执行状态
- **日志记录**: 完整的任务执行日志

### 🌐 流量伪装

- **自定义编码**: 支持自定义Base64字母表
- **流量混淆**: 可配置的流量伪装前缀和后缀
- **协议伪装**: HTTP请求头伪装和路径混淆

## 🔒 安全配置

### 生产环境安全建议

1. **修改默认密码**
   ```json
   {
     "admin_user": "your_secure_username",
     "admin_pass": "your_very_strong_password"
   }
   ```

2. **使用强JWT密钥**
   ```json
   {
     "jwt_secret": "use-a-very-long-random-string-at-least-64-characters"
   }
   ```

3. **启用HTTPS**
   ```json
   {
     "server": {
       "enable_https": true,
       "cert_file": "./certs/server.crt",
       "key_file": "./certs/server.key"
     }
   }
   ```

4. **配置防火墙**
   ```bash
   # 只开放必要端口
   sudo ufw allow 18080/tcp  # 管理界面
   sudo ufw allow 8083/tcp   # Beacon通信
   sudo ufw enable
   ```

## 🐛 故障排除

### 常见问题

| 问题 | 解决方案 |
|------|----------|
| **Beacon连接失败** | 检查服务器IP、端口和防火墙设置 |
| **数据库连接错误** | 验证MySQL服务状态和连接信息 |
| **文件上传失败** | 检查storage目录权限和磁盘空间 |
| **前端编译错误** | 确认Node.js版本和依赖完整性 |

### 日志查看

```bash
# 后端日志
tail -f webserver/backend/webserver.log

# 系统服务日志
sudo journalctl -u goc2 -f

# 前端开发日志
npm run dev
```

## 🚀 性能优化

### 数据库优化

```sql
-- 创建索引
CREATE INDEX idx_beacon_uuid ON beacons(uuid);
CREATE INDEX idx_task_status ON tasks(status);
CREATE INDEX idx_file_created ON files(created_at);

-- 优化查询
EXPLAIN SELECT * FROM beacons WHERE status = 'active';
```

### 服务器优化

```json
{
  "database": {
    "max_open_conns": 100,
    "max_idle_conns": 25,
    "conn_max_lifetime": 300
  },
  "server": {
    "read_timeout": 30,
    "write_timeout": 30,
    "max_body_size": 50
  }
}
```



## 🤝 贡献指南

我们欢迎所有形式的贡献！

### 贡献方式

1. **提交Issue**: 报告bug或提出新功能建议
2. **提交PR**: 修复bug或添加新功能
3. **改进文档**: 完善README或添加使用说明
4. **分享经验**: 在讨论区分享使用心得

## 更新记录

2025年11月19日 增加 bof、inline、进程注入等武器中心功能。

2025年11月25日 修正 bof 参数，修正进程注入方式，增加beacon SEH/VEH方便调试，修正进程获取前端。

## 🙏 致谢

感谢所有为这个项目做出贡献的开发者和用户！

感谢：[GateSentinel](https://github.com/kyxiaxiang/GateSentinel)

---

<div align="center">

**⭐ 如果这个项目对你有帮助，请给我一个Star！⭐**

**⚠️ 免责声明**: 本项目仅供学习和研究使用，请遵守相关法律法规，不得用于非法用途。

</div>
