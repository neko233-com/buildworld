# BuildWorld233 使用手册

本文覆盖生产安装、更新、回滚、macOS 生命周期、Jenkins 并行迁移和飞书通知。默认服务端口为 `8080`，保留 Jenkins 端口为 `8081`；初始账号为 `root/root`。首次登录后立刻改密码。

## 安装

以拥有 Git、签名证书和构建工具的用户安装。不要用另一个管理员账户安装后再切换到构建账户。

### macOS

Apple Silicon 与 Intel 自动识别：

```sh
curl -fsSL https://raw.githubusercontent.com/neko233-com/buildworld233/main/scripts/install.sh | sh
buildworld status
open http://127.0.0.1:8080
```

安装器写入 `~/.zprofile`，安装完整 bundle 到 `~/.local/lib/buildworld`，创建 `~/.local/bin/buildworld`，并启用 `com.buildworld.server` LaunchAgent。新 SSH/Terminal 会话自动获得 CLI PATH；当前会话可执行：

```sh
export PATH="$HOME/.local/bin:$PATH"
```

### Linux

```sh
curl -fsSL https://raw.githubusercontent.com/neko233-com/buildworld233/main/scripts/install.sh | sh
buildworld status
```

安装器创建 systemd user service。无用户 systemd 的服务器，设置 `BUILDWORLD_NO_AUTOSTART=1` 后自行用受控服务管理器启动。

### Windows

在 PowerShell 执行：

```powershell
irm https://raw.githubusercontent.com/neko233-com/buildworld233/main/scripts/install.ps1 | iex
buildworld status
Start-Process http://127.0.0.1:8080
```

默认安装到 `%LOCALAPPDATA%\BuildWorld`，注册登录启动任务，并写入用户 PATH。

## 日常操作

```sh
buildworld start      # 启动
buildworld status     # 健康检查
buildworld pause      # 停机，不会被服务管理器立即重拉
buildworld resume     # 恢复
buildworld restart    # 停机并重启
buildworld stop       # 停止后台服务
```

macOS 服务日志：

```sh
tail -n 200 "$HOME/Library/Application Support/buildworld/server.log"
launchctl print "gui/$(id -u)/com.buildworld.server"
```

重启恢复默认开启。进程意外退出、机器重启、安装更新时处于 `running` 的构建会被标记为 `pending` 后按原队列优先级继续执行，日志含 `Requeued after interrupted server restart`。`cancelled`、`failed`、`success` 不会自动重跑。对于非幂等部署步骤，应使用审批、部署锁或可重入脚本。

## 服务器更新

先在测试机验证指定版本，再在生产环境执行相同安装器：

```sh
curl -fsSL https://raw.githubusercontent.com/neko233-com/buildworld233/main/scripts/install.sh | sh -s -- 1.0.0
buildworld status
```

```powershell
& ([scriptblock]::Create((irm https://raw.githubusercontent.com/neko233-com/buildworld233/main/scripts/install.ps1))) -Version 1.0.0
buildworld status
```

更新顺序：下载 -> SHA-256 校验 -> `pause` -> bundle 移至 `.previous` -> 解压新 bundle -> 重新启用自启动 -> `start` 健康检查。数据库、工作区、制品与配置不在 bundle 内，因此更新不会覆盖它们。

生产更新前检查：

1. 导出或备份配置目录和 SQLite 数据库。
2. 记录当前 `buildworld version` 与 `buildworld status`。
3. 确认运行中部署任务具备幂等性；它们会在重启后恢复。
4. 更新后登录控制台，验证项目数、队列、最近制品和飞书测试消息。

## 回滚

安装器保留一个上一版本目录：macOS/Linux 为 `~/.local/lib/buildworld.previous`；Windows 为 `%LOCALAPPDATA%\BuildWorld.previous`。回滚不改数据库。

macOS/Linux：

```sh
export PATH="$HOME/.local/bin:$PATH"
buildworld pause
mv "$HOME/.local/lib/buildworld" "$HOME/.local/lib/buildworld.failed"
mv "$HOME/.local/lib/buildworld.previous" "$HOME/.local/lib/buildworld"
ln -sfn "$HOME/.local/lib/buildworld/buildworld" "$HOME/.local/bin/buildworld"
buildworld enable-autostart
buildworld start
```

Windows PowerShell：

```powershell
& "$env:LOCALAPPDATA\BuildWorld\buildworld.exe" pause
Move-Item "$env:LOCALAPPDATA\BuildWorld" "$env:LOCALAPPDATA\BuildWorld.failed"
Move-Item "$env:LOCALAPPDATA\BuildWorld.previous" "$env:LOCALAPPDATA\BuildWorld"
& "$env:LOCALAPPDATA\BuildWorld\buildworld.exe" enable-autostart
& "$env:LOCALAPPDATA\BuildWorld\buildworld.exe" start
```

确认 `buildworld status` 后再删除 `.failed`。如果数据库迁移导致应用层不兼容，从更新前备份恢复数据库后再启动旧版本。

## Jenkins 并行迁移

BuildWorld `8080` 与 Jenkins `8081` 可在同一 Mac Mini 并存。切换后只允许 BuildWorld 对生产目标自动部署。

1. 从 Jenkins 导出每个 Pipeline 的 `config.xml` / Jenkinsfile，并记录分组、仓库、分支、参数、cron、凭据和飞书行为。
2. 在 BuildWorld 创建同名分组和项目，导入 Jenkinsfile。转换器 warning 必须逐项处理，尤其 `credentials`、共享库、`post`、动态 Groovy、复杂 `if`。
3. 初始触发器保留为手动。选择无副作用任务先跑，再对相同 commit 执行双跑比对。
4. 对比步骤、环境、`WORKSPACE`、构建时间、制品 SHA-256、部署结果、飞书消息；差异先修复再继续。
5. 多次一致后，先关闭 Jenkins trigger，观察一个发布周期，再启用 BuildWorld trigger。保留 Jenkins job 和备份到回滚窗口结束。

## 飞书通知

BuildWorld 飞书通道由 Go 原生服务发送，不运行 Python。创建通道时：

1. 控制台进入通知设置，新建类型 `feishu`。
2. 配置 JSON：`{"webhook_url":"https://open.feishu.cn/open-apis/bot/v2/hook/..."}`。
3. 先执行测试项目，检查成功、失败、开始事件以及通知事件记录。
4. 如果机器人启用签名或 IP 白名单，按飞书机器人策略配置可达网络；不要在 pipeline 日志输出 webhook。

旧 Jenkins 自定义机器人已迁到 Go 时，BuildWorld 项目可继续调用该 Go 二进制作为过渡；新项目优先使用 BuildWorld 原生飞书通道。

## 安全基线

- 立即替换 `root/root`：`printf '%s\n' '至少12位新密码' | buildworld reset-root-password --password-stdin`。
- 仅用反向代理/TLS 或可信内网公开 `8080`；限制来源 IP。
- 数据库含构建所需的凭据原文：限制服务账号和备份权限，并使用磁盘/备份加密。
- Git 密钥、飞书 webhook、token 只放凭据/通知配置，不提交 Jenkinsfile、Pipeline 或仓库。
- 每次更新前备份 SQLite 数据库与配置目录；定期校验制品存储。
- 把生产部署配置为审批或人工触发，直到双工具并行验证结束。
