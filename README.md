<p align="center">
  <img src="docs/images/logo.png" width="128" height="128" alt="SkyHook Logo" />
</p>

<h1 align="center">SkyHook (天钩)</h1>

<p align="center">
  <strong>🛸 高速远程中继下载与 Hook 自动化转存流水线引擎</strong><br />
  <em>"本地一键投送，云端高速吸纳，流水线自动归档，多渠道即时知晓。"</em>
</p>

<p align="center">
  <a href="https://phpgao.github.io/skyhook/"><img src="https://img.shields.io/badge/Documentation-GitHub%20Pages-00ADD8?style=flat-square&logo=github" alt="Docs"></a>
  <a href="https://github.com/phpgao/skyhook/actions/workflows/docker.yml"><img src="https://img.shields.io/github/actions/workflow/status/phpgao/skyhook/docker.yml?branch=main&label=CI%20Build&logo=github&style=flat-square" alt="CI Status"></a>
  <a href="https://github.com/phpgao/skyhook/pkgs/container/skyhook"><img src="https://img.shields.io/badge/GHCR-Docker%20Image-blue?logo=docker&style=flat-square" alt="GHCR"></a>
  <a href="https://github.com/phpgao/skyhook/blob/main/LICENSE"><img src="https://img.shields.io/badge/License-AGPL--3.0-purple?style=flat-square" alt="License"></a>
</p>

---

## 🌟 核心特性

- ⚡ **全协议下载中继与智能分流**：支持普通 HTTP/HTTPS 批量链接、磁力链接（Magnet）、BT 种子文件以及流媒体网页视频（YouTube、Bilibili 等）一键投送。
- 🧭 **配置驱动的下载引擎池 (Driver Registry & Priorities)**：
  - 下载器与优先级由 `config.yaml` 动态指定，拒绝代码写死。
  - 支持配置优先级 `0-99`（99 最高）、支持自定义 `bin` 路径（未设置时默认从系统 PATH 查找）。
  - 开箱支持 `aria2_rpc` (BT/磁链 优先级88)、`aria2c` CLI fork (HTTP 优先级88)、`wget` (优先级19)、`curl` (优先级10)、`yt-dlp` (视频专用 优先级90) 以及纯 Go 内置保底引擎 `native` (优先级1)。
- 💾 **无依赖任务持久化与进程重启自动接管 (Zero-Dependency Task Persistence & Recovery)**：
  - 内置原子落盘任务存储器（`JSONFileTaskStore`），无需外挂 MySQL/Redis 或 CGO SQLite，维持单一静态二进制零环境依赖。
  - 进程因升级或意外重启后，启动时自动执行 `RecoverAndResume` 智能扫描未决任务：
    - 若底层下载器仍处于下载状态，自动重新挂载（Re-attach）到 Watcher 巡检轮询。
    - 若底层任务在停机期间已离线完成，自动接管并触发 `on_complete` 流水线及完成通知。
    - 支持断点续传（HTTP Range Header 206 续传、Aria2 `continue: true` 续传、Wget `-c` / Curl `-C -` 续传）。
- 🛡️ **磁盘容量预留与爆盘安全熔断 (Disk Reserve & Quota Protection)**：
  - 统一落盘目录配置（`storage.download_dir`），支持自然语言空间预留（`storage.min_free_space: "2GB"`）。
  - 基于 OS 级 `Statfs` 物理剩余块检测：
    - **投送预检**：任务提交时若磁盘低于安全水位，立即拦截拒绝（返回 HTTP 507 并触发 `on_create_failed` 告警）。
    - **运行时巡检**：下载或做种过程中若可用空间跌破阈值，立即中止下载并告警，杜绝 VPS 磁盘被占满引发系统死机。
- 📢 **多渠道通知广播矩阵 (Multi-Channel Notification Center)**：
  - 支持 **Telegram Bot**、**企业微信群机器人 (WeCom)**、**钉钉群机器人 (DingTalk)**、**飞书机器人 (Feishu)**、**Bark (iOS)** 以及**通用 HTTP Webhook**。
  - 多渠道并发并行推送，自动适配各平台专有渲染语法（Markdown、富文本 post 卡片、标准 JSON）。
- 🪝 **首等公民级 Hook 生命周期系统**：
  - `on_create_failed`：任务创建失败告警（例如磁盘不足、链接格式错误、无可匹配下载器）。
  - `on_start`：下载启动即时提醒（向各渠道发送任务启动卡片）。
  - `on_progress`：任务巡检实时监控与进度/速度广播（`[巡检] xx% 速度 xx/s`）。
  - `on_complete`：下载完成后触发命令流水线（上传夸克网盘、同步阿里云 OSS 等），执行结果全量捕获。
  - `on_error`：任务异常或超时熔断后自动捕获并发送故障日志。
- 🍸 **Gin 引擎驱动**：路由与中间件基于 `gin-gonic/gin`，在 Handler 中无缝注入全局生命周期 Context。
- 🔑 **一键凭据免密与显式覆盖**：
  - `skyhook login` 将 VPS 地址与 Token 自动保存在 `~/.skyhook`（严格 `0600` 权限）。
  - 命令行显式指定参数（`-s`、`-t`）拥有最高优先级，支持多节点灵活覆盖与自由切换。
- 🔁 **单步重试与容错 (Retry & Fault-Tolerance)**：
  - 单步独立配置 `retries`、`retry_interval` 与 `continue_on_error`。
- ⏳ **自然语言时长配置**：YAML 原生解析 `2h`、`1h30m`、`45m`、`10s` 等时间段。

---

## 📐 接口架构设计

本项目严格遵循面向接口设计原则：

```
                    ┌─────────────────────────┐
                    │     HTTP Server API     │
                    │ (AuthToken Middleware)  │
                    └────────────┬────────────┘
                                 │
     ┌───────────────────────────┼───────────────────────────┐
     ▼                           ▼                           ▼
┌──────────────┐         ┌───────────────┐           ┌──────────────┐
│  Downloader  │         │ PipelineEngine│           │   Notifier   │
│  (Interface) │         │  (Interface)  │           │ (Interface)  │
└──────┬───────┘         └───────┬───────┘           └──────┬───────┘
       │                         │                          │
 ┌─────┴─────┐             ┌─────┴─────┐              ┌─────┴─────┐
 │Aria2Client│             │ Standard  │              │ Telegram  │
 │(JSON-RPC) │             │  Engine   │              │ WeCom     │
 ├───────────┤             └─────┬─────┘              │ DingTalk  │
 │CLI Runner │                   │                    │ Feishu    │
 │(Aria2/Wget│             ┌─────┴─────┐              │ Bark      │
 │ /Curl/Yt) │             │ TaskStore │              │ Webhook   │
 ├───────────┤             │(Interface)│              └───────────┘
 │NativeGo   │             └─────┬─────┘
 └───────────┘             ┌─────┴─────┐
                           │JSONFile/  │
                           │MemoryStore│
                           └───────────┘
```

---

## 📁 目录结构

```
skyhook/
├── bin/
│   ├── skyhook               # 客户端命令行工具
│   └── skyhookd              # 服务端守护进程
├── cmd/
│   ├── skyhook/main.go       # 客户端 CLI 入口
│   └── skyhookd/main.go      # 服务端 Daemon 入口
├── deploy/                   # 容器化与一键部署配置
│   ├── Dockerfile            # 基于 Alpine 的极简高能一体化镜像 (内置全套下载器与 diskcli)
│   ├── docker-compose.yml    # Docker Compose 一键编排启动
│   ├── entrypoint.sh         # 容器自愈入口脚本 (自动拉起 Aria2 RPC、动态配置夸克网盘)
│   ├── config.docker.yaml    # 容器生产默认配置 (含夸克网盘转存流水线示例)
│   └── .dockerignore
├── internal/
│   ├── config/               # 配置加载、多渠道与存储配置解析 (含单测)
│   ├── downloader/           # 动态下载引擎驱动池 (Aria2 RPC, CLI, Native, 调度器与单测)
│   ├── executor/             # 系统命令与脚本执行器 (含单测与超时控制)
│   ├── hook/                 # 全生命周期 Hook 触发引擎 (含变量插值与单测)
│   ├── model/                # 领域模型 (Task, TaskRecord, Action, Step, Result, Hook)
│   ├── notifier/             # 多渠道通知矩阵 (Telegram, WeCom, DingTalk, Feishu, Bark, Webhook)
│   ├── pipeline/             # 流水线引擎 (重试、容错、统一清理，含单测)
│   ├── server/               # HTTP API 服务、任务查询与 Token 鉴权中间件 (含单测)
│   ├── storage/              # 磁盘预留检测 (Statfs) 与原子状态持久化 (JSONFileTaskStore)
│   └── watcher/              # 下载状态巡检、重启断点接管与生命周期调度 (含单测)
├── pkg/
│   └── client/               # Go 客户端 SDK (含单测)
├── config.example.yaml       # 完整生产级配置示例
└── go.mod
```

---

## 🧪 运行单元测试

项目内置了覆盖核心组件的高覆盖率单元测试（包含下载调度器、持久化恢复、多渠道通知、磁盘空间熔断、超时控制与鉴权）：

```bash
cd /Users/jimmygao/code/github/skyhook
go test -v ./...
```

---

## 🛠️ 编译与启动

### 1. 编译二进制 (原生运行)
```bash
# 本地编译服务端与客户端
go build -o bin/skyhookd ./cmd/skyhookd
go build -o bin/skyhook ./cmd/skyhook

# 跨平台静态编译 (Linux amd64)
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -trimpath -ldflags="-s -w" -o bin/skyhookd ./cmd/skyhookd
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -trimpath -ldflags="-s -w" -o bin/skyhook ./cmd/skyhook
```

### 2. 容器化一键部署 (推荐：基于 Alpine，内置全套工具链 + 夸克网盘 diskcli)
SkyHook 提供一体化轻量 Docker 镜像，内置：
- **`aria2c`** (多线程 & BT/磁力下载，开箱自启 `:6800` RPC)
- **`yt-dlp`** + **`ffmpeg`** (全网视频流提取与合并)
- **`diskcli`** / **`quark-cli`** (集成 [phpgao/diskcli](https://github.com/phpgao/diskcli) 夸克网盘转存工具)
- **`curl`**、**`wget`**、**`bash`**、**`jq`**

所有部署文件均收纳在 `deploy/` 目录下：

```bash
# 方式 A：直接拉取 GitHub 官方镜像运行 (免本地编译构建)
docker run -d \
  --name skyhook \
  --restart unless-stopped \
  -p 8080:8080 \
  -p 6800:6800 \
  -e AUTH_TOKEN=skyhook-secret-token \
  -e ENABLE_ARIA2_RPC=true \
  -e ARIA2_SECRET=skyhook-rpc-token \
  -e QUARK_COOKIE="your_quark_cookie_here" \
  -v $(pwd)/deploy/downloads:/data/downloads \
  -v $(pwd)/deploy/config:/data/config \
  ghcr.io/phpgao/skyhook:latest

# 方式 B：进入 deploy 目录一键编排启动
cd deploy
docker compose up -d

# 方式 C：在项目根目录通过 compose 启动
docker compose -f deploy/docker-compose.yml up -d
```

### 3. 服务端原生启动 (VPS)
```bash
# 复制并修改配置文件
cp config.example.yaml config.yaml

# 启动 SkyHook 守护进程（启动时自动扫描恢复未完成任务）
./bin/skyhookd -c config.yaml
```

### 3. 客户端使用 (本地电脑)

```bash
# 步骤 1：首次使用登录并保存凭证（保存在 ~/.skyhook，权限 0600）
./bin/skyhook login -s "http://YOUR_VPS_IP:8080" -t "skyhook-super-secret-key"

# 步骤 2：之后无需再传 -s 和 -t，直接一键投送！
# 提交单个或批量 HTTP/HTTPS 链接（自动执行默认动作）
./bin/skyhook https://example.com/file1.zip https://example.com/file2.iso

# 提交磁力链接并指定 OSS 动作
./bin/skyhook -a oss "magnet:?xt=urn:btih:..."

# 提交本地种子文件，限制 VPS 下载 5M/s、上传 500K/s，并指定上传夸克
./bin/skyhook -a quark --limit-dl 5M --limit-up 500K /Users/you/Downloads/ubuntu-22.04.torrent
```

---

## 🌐 HTTP RESTful API

服务端提供标准的 REST API，便于与外部系统集成：

| 接口 | 方法 | 说明 | 鉴权 |
| :--- | :---: | :--- | :---: |
| `/health` | `GET` | 检查守护进程运行状态与版本 | 无需 |
| `/api/tasks` | `POST` | 投送下载任务（支持 URL、磁力、BT Base64 上传） | 需 Token |
| `/api/tasks` | `GET` | 查询所有正在运行及历史任务的实时状态 | 需 Token |

