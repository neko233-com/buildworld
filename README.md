<p align="center">
  <h1 align="center">BuildWorld</h1>
  <p align="center"><strong>生产级 CI/CD 服务器 — Jenkins 现代替代方案</strong></p>
  <p align="center">
    TypeScript DSL · 分布式 Worker · React 仪表盘 · 飞书通知 · 28+ 插件
  </p>
</p>

<p align="center">
  <a href="https://github.com/neko233-com/buildworld/releases"><img alt="GitHub Release" src="https://img.shields.io/github/v/release/neko233-com/buildworld"></a>
  <a href="https://github.com/neko233-com/buildworld/blob/main/README_EN.md"><img alt="English" src="https://img.shields.io/badge/English-README_EN.md-blue"></a>
</p>

---

## 一键安装

> 默认通过 GitHub 镜像安装，适用于无法直连 GitHub 的网络；如需直连，设置 `BUILDWORLD_GITHUB_MIRROR=off`。

<table>
<tr>
<td><strong>macOS / Linux</strong></td>
<td><strong>Windows PowerShell</strong></td>
</tr>
<tr>
<td>

```sh
curl -fsSL https://gh-proxy.com/https://raw.githubusercontent.com/neko233-com/buildworld/main/scripts/install.sh | sh
buildworld status
```

</td>
<td>

```powershell
& ([scriptblock]::Create((
  (Invoke-WebRequest -UseBasicParsing `
    https://gh-proxy.com/https://raw.githubusercontent.com/neko233-com/buildworld/main/scripts/install.ps1).Content
)))
buildworld status
```

</td>
</tr>
</table>

浏览器打开 `http://127.0.0.1:8080`，默认账号 `root` / `root`，**首次登录后立即修改密码**。

<details>
<summary>指定版本 / 内网离线安装</summary>

```sh
# macOS / Linux
sh install.sh 1.15.0

# Windows
.\install.ps1 -Version 1.15.0
```

内网部署：将 `release/vX.Y.Z/` 中的安装脚本、bundle 与 `checksums.txt` 一起发布到可信下载地址。
</details>

---

## 架构总览

```
┌─────────────────────────────────────────────────────────────┐
│                      BuildWorld Server                       │
│                    Go · Chi · SQLite · JWT                    │
│                                                              │
│  ┌──────────┐  ┌──────────┐  ┌──────────┐  ┌────────────┐  │
│  │ REST API │  │ WebSocket│  │  gRPC    │  │  Webhooks  │  │
│  │ /api/*   │  │ 实时日志  │  │ Worker   │  │ GH/GL/Gitea│  │
│  └────┬─────┘  └────┬─────┘  └────┬─────┘  └─────┬──────┘  │
│       │              │              │               │         │
│  ┌────┴──────────────┴──────────────┴───────────────┴──────┐ │
│  │                    Build Engine                          │ │
│  │   调度 · 队列 · 执行 · 插件 · 日志 · 制品 · 通知       │ │
│  └────────────────────────┬────────────────────────────────┘ │
│                           │                                   │
│  ┌────────────────────────┴────────────────────────────────┐ │
│  │                  SQLite + Migrations                     │ │
│  └──────────────────────────────────────────────────────────┘ │
└─────────────────────────────────────────────────────────────┘
         │                              │
    REST / WS                        gRPC
         │                              │
┌────────┴────────┐          ┌──────────┴──────────┐
│   React SPA     │          │   Distributed       │
│   Dashboard     │          │   Workers           │
│   Vite + Monaco │          │   cmd/worker        │
└─────────────────┘          └─────────────────────┘
```

**核心组件：**

| 组件 | 目录 | 职责 |
|------|------|------|
| **Server** | `cmd/server` | HTTP API、WebSocket 实时日志、gRPC Worker 管理、构建引擎调度 |
| **CLI** | `cmd/cli` | 用户交互：start / stop / restart / status / reset-password |
| **Worker** | `cmd/worker` + `sdk/` | 分布式构建执行器，通过 gRPC 自动注册到 Server |
| **Frontend** | `web/` | React SPA — Jenkins 风格仪表盘，Monaco 编辑器，实时日志 |
| **Plugins** | `plugins/` | 28+ 构建/部署/通知插件，运行时动态加载 |
| **Migrator** | `internal/jenkins` | Jenkinsfile → TypeScript DSL 自动转换 |

---

## 核心功能

### Pipeline DSL

支持 **TypeScript** 和 **YAML** 两种声明式 Pipeline：

```typescript
// TypeScript DSL — Monaco 编辑器提供校验与补全
pipeline({
  stages: [
    stage("Build", async () => {
      await sh("go build -o app ./cmd/server");
    }),
    stage("Test", async () => {
      await sh("go test ./...");
    }),
    stage("Deploy", async () => {
      await notify({ type: "feishu", message: "部署完成" });
    }),
  ],
});
```

TypeScript Pipeline 在受限沙箱中执行，**禁止 `eval` 和 `new Function`**。

### 分布式 Worker

- 内置 `builtin` 执行器：单机即可运行
- 远程 Worker 通过 gRPC 自动注册，支持水平扩展
- Worker SDK (`sdk/`) 支持自定义 Worker 实现
- 构建隔离：每次构建使用独立工作空间，自动清理

### Web 控制台

Jenkins 风格但更现代化的 React 仪表盘：

- **Dashboard** — 项目列表、状态总览、左侧队列/最近构建面板
- **Build Queue** — 实时构建队列，支持取消与重排
- **Build Detail** — 实时日志流、制品列表、环境变量、变更集
- **Project Configure** — 可视化配置 Pipeline、触发器、凭证
- **Big Screen** — 大屏监控模式
- **Credentials** — 凭证管理（用户名密码、SSH Key、API Token）
- **Audit Log** — 操作审计日志
- **Statistics** — 构建统计与趋势分析

### 插件生态

28+ 开箱即用的插件，覆盖主流构建工具与平台：

| 类别 | 插件 |
|------|------|
| **构建工具** | Go, Cargo, Gradle, Maven, npm, pip, .NET, Shell, PowerShell |
| **容器/部署** | Docker, Kubernetes, Helm, ArgoCD, Terraform, Ansible |
| **SCM** | GitHub, GitLab, Gitea, SVN, Mercurial |
| **通知** | Feishu (飞书), Slack, Discord, Telegram, Webhook |
| **制品/安全** | S3, Vault, SonarQube |

### Jenkins 迁移

支持渐进式从 Jenkins 迁移，零停机切换：

1. **导入** — 自动解析 Jenkinsfile，生成 TypeScript DSL
2. **双跑** — 同一 commit 同时在 Jenkins 和 BuildWorld 执行
3. **对比** — 比对环境变量、制品、通知、部署结果
4. **切换** — 连续一致后关闭 Jenkins 触发器

### 飞书通知

原生 Go 实现，无外部依赖：

```json
{
  "webhook_url": "https://open.feishu.cn/open-apis/bot/v2/hook/REDACTED"
}
```

支持构建开始、成功、失败事件，发送富文本卡片消息。

---

## 运维速查

```sh
buildworld start              # 启动服务
buildworld stop               # 停止服务
buildworld restart            # 重启
buildworld status             # 查看状态
buildworld pause              # 暂停（卸载后台服务，防止自动拉起）
buildworld resume             # 恢复
buildworld enable-autostart   # 开机自启
buildworld disable-autostart  # 关闭自启
```

```sh
# 重置管理员密码
printf '%s\n' 'new-strong-password' | buildworld reset-root-password --password-stdin
```

更新失败时，上个 bundle 保留在 `.previous` 目录，可随时回滚。

---

## 默认配置

| 项目 | 默认值 |
|------|--------|
| 控制台 / API | `http://127.0.0.1:8080` |
| 默认管理员 | `root` / `root` |
| 数据库 | SQLite (`data/buildworld.db`) |
| macOS bundle | `~/.local/lib/buildworld` |
| macOS CLI | `~/.local/bin/buildworld` |
| 数据与日志 | `~/Library/Application Support/buildworld` (macOS) |

局域网访问时，将 `server.host` 改为受控网卡地址或使用反向代理 + TLS。**不要将密码、webhook 或 Git 凭据写入仓库。**

---

## 开发

```bash
# 后端测试
go test ./...

# 前端测试与构建
cd web && npm ci && npm test -- --run && npm run build

# 文档站构建
cd docs-site && npm ci && npm run build
```

### 本地发布

```powershell
# 完整门禁验证
.\scripts\verify-local.ps1

# 六平台打包 (Windows / Linux / macOS × amd64 / arm64)
.\scripts\release-local.ps1 -Version 1.15.0

# 发布到 GitHub Release
.\scripts\publish-release-local.ps1 -Version 1.15.0 -Publish -ReplaceExisting
```

所有 CI、测试、打包和发布均在本地完成，不使用 GitHub Actions。

---

## 项目结构

```
buildworld/
├── cmd/
│   ├── cli/          # CLI 入口
│   ├── server/       # 服务端入口
│   └── worker/       # 分布式 Worker 入口
├── internal/         # 核心业务逻辑 (API · Engine · Store · Auth · Plugin)
├── web/              # React SPA (Vite · TypeScript · Monaco)
├── plugins/          # 28+ 插件 (构建 · 部署 · 通知 · SCM)
├── proto/            # gRPC 协议定义
├── sdk/              # Worker SDK
├── scripts/          # 安装 · 发布 · 验证脚本
├── docs-site/        # Docusaurus 文档站
└── integration/      # 集成测试
```

---

<p align="center">
  <a href="README_EN.md">English Version →</a>
</p>
