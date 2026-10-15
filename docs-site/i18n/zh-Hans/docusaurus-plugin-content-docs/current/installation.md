---
title: 安装指南
description: 使用 PowerShell 或 Shell 在 Windows、Linux 和 macOS 安装 BuildWorld，验证 SHA-256、PATH 与服务状态。
sidebar_position: 2
---

# 安装 BuildWorld

## 一键安装

### Windows PowerShell

```powershell
& ([scriptblock]::Create((Invoke-WebRequest -UseBasicParsing https://gh-proxy.com/https://raw.githubusercontent.com/neko233-com/buildworld/main/scripts/install.ps1).Content))
buildworld status
```

### macOS / Linux

```sh
curl -fsSL https://gh-proxy.com/https://raw.githubusercontent.com/neko233-com/buildworld/main/scripts/install.sh | sh
buildworld status
```

默认使用 GitHub 镜像。需要直连 GitHub 时，在执行安装命令前设置：

```powershell
$env:BUILDWORLD_GITHUB_MIRROR = 'off'
```

```sh
export BUILDWORLD_GITHUB_MIRROR=off
```

也可将 `BUILDWORLD_GITHUB_MIRROR` 设为可信的 HTTPS 镜像地址。
镜像只负责传输公开元数据与文件，安装器仍会校验已发布的 SHA-256。

## 安装内容与路径

每个平台的 bundle 包含 CLI、Server、Worker、Web UI 和 Pipeline SDK。
默认构建由内置 `builtin` 执行器完成，无需启动远程 Worker。

| 平台 | 支持架构 | 默认安装目录 |
| --- | --- | --- |
| Windows | amd64、arm64 | `%LOCALAPPDATA%\BuildWorld` |
| Linux | amd64、arm64 | `~/.local/lib/buildworld` |
| macOS | amd64、arm64 | `~/.local/lib/buildworld` |

安装器将 CLI 加入用户 PATH。macOS/Linux 的 CLI 链接位于 `~/.local/bin`。
可用 `BUILDWORLD_INSTALL_DIR` 自定义安装目录，Shell 安装器另支持 `BUILDWORLD_BIN_DIR`。
安装目录与[用户配置和运行数据](./configuration)分别保存。

## 固定版本与自动启动

从 [GitHub Releases](https://github.com/neko233-com/buildworld/releases) 下载安装脚本后，可指定版本：

```powershell
.\install.ps1 -Version 1.16.23
```

```sh
sh install.sh 1.16.23
```

默认注册静默的用户级自动启动，并启动服务。Windows 使用任务计划程序，macOS 使用
launchd，Linux 使用 systemd user service。仅跳过自动启动注册：

```powershell
.\install.ps1 -Version 1.16.23 -NoAutostart
```

```sh
BUILDWORLD_NO_AUTOSTART=1 sh install.sh 1.16.23
```

`-NoAutostart` 仍会启动本次安装的服务；停止服务使用 `buildworld pause`。

## 分片下载与完整性

Release 的 bundle 以 `.part000`、`.part001` 等分片上传，以适应受限网络。
安装器自动重组分片，校验每个分片与完整 archive，然后才替换安装目录。
不能将单个分片作为 ZIP 或 tar.gz 解压。

每次发布提供 `checksums.txt`；从 `v1.16.23` 开始，还在 bundle 内及 Release 资产中
提供 `release-info.json`，记录版本、源码 commit、Go 工具链、构建时间和全部目标平台。
该 JSON 的 SHA-256 也收录在 `checksums.txt`。它用于核对产物身份，不是数字签名。

内网安装需同时提供安装脚本、全部对应平台分片及 `checksums.txt`，并通过可信下载地址分发。

## 首次登录与验证

打开 `http://127.0.0.1:8080`，使用默认账号 `root` / `root` 登录，首次登录后立即修改密码。

```sh
buildworld version
buildworld status
buildworld pause
buildworld resume
buildworld restart
```

需要恢复管理员密码时，使用 `buildworld reset-root-password --password-stdin` 从标准输入
读取至少 12 个字符的密码。由交互式安全输入或密钥管理工具提供输入，避免将密码写入命令历史。
升级时保留上一 bundle 到 `.previous`，安装失败会尝试恢复；数据库、配置与日志保存在用户数据目录。

## 下一步

- [配置与数据目录](./configuration)
- [编写流水线](./pipelines)
- [本地验证和发布](./local-publishing.md)
