# BuildWorld233

BuildWorld233 是面向生产构建、发布与通知的 Jenkins 替代项目。它提供 Web 控制台、CLI、持久化构建队列、分布式 Worker、制品管理和飞书通知。迁移期间可与 Jenkins 并行运行：BuildWorld 默认 `8700`，Jenkins 通常为 `8080`。

首次登录账号：`root`  密码：`root`。首次进入控制台必须立即修改密码；生产环境不要把 `8700` 直接暴露到公网。

## 先安装 / 更新

同一安装命令同时用于首次安装和服务器更新。安装器下载本机平台完整 bundle（CLI、server、worker、Web）、校验 SHA-256、暂停旧服务、保留上个 bundle、替换、启动并启用用户级自启动。

当前仓库为私有仓库，先完成一次 `gh auth login`。macOS / Linux：

```sh
gh api -H "Accept: application/vnd.github.raw+json" repos/neko233-com/buildworld233/contents/scripts/install.sh | sh
buildworld status
```

Windows PowerShell：

```powershell
& ([scriptblock]::Create((gh api -H "Accept: application/vnd.github.raw+json" repos/neko233-com/buildworld233/contents/scripts/install.ps1 | Out-String)))
buildworld status
```

安装器自动使用 `GH_TOKEN`、`GITHUB_TOKEN` 或本机 `gh auth` 凭据读取私有 Release；仓库公开后也兼容匿名下载。指定版本：shell 脚本第一个参数为版本，例如 `sh install.sh 1.0.0`；PowerShell 使用 `-Version 1.0.0`。内网部署可将本仓库 `release/vX.Y.Z/` 中的 `install.sh` / `install.ps1` 与 bundle、`checksums.txt` 一起发布到可信下载地址。

macOS 一键安装默认创建 `LaunchAgent`，登录后自动运行。意外重启或更新中断的 `running` 构建会默认重新排队并恢复；用户主动取消的构建不会重跑。
Jenkins 与 BuildWorld 在 macOS 中拥有独立的隐私权限；Jenkins Java 已获授权不代表 BuildWorld 自动获得 `Desktop`、`Documents` 或 `Downloads` 访问权。引用的工作区、文件、脚本和可执行程序优先放在非受保护目录；必须保留原路径时，优先在“隐私与安全性 → 文件与文件夹”中只开放所需目录。没有更小权限且当前 macOS 允许添加可执行文件时，才为可信的实际安装 server（默认 `~/.local/lib/buildworld/buildworld-server`）手工开启可读取全部受保护用户数据的“完全磁盘访问权限”。无法添加或授权不生效时，应迁出所有受保护路径。安装器不会修改或绕过 TCC。

## 默认地址

| 项目 | 默认值 |
| --- | --- |
| 控制台 / API | `http://127.0.0.1:8700` |
| 默认管理员 | `root` / `root` |
| macOS bundle | `~/.local/lib/buildworld` |
| macOS CLI | `~/.local/bin/buildworld` |
| 数据与日志 | `~/Library/Application Support/buildworld` |

局域网访问时，把 `server.host` 改为受控网卡地址或使用反向代理/TLS；同时限制防火墙来源。不要把默认管理员密码、飞书 webhook 或 Git 凭据写入仓库。

## 常用运维

```sh
buildworld start
buildworld status
buildworld pause
buildworld resume
buildworld restart
buildworld enable-autostart
buildworld disable-autostart
printf '%s\n' 'new-strong-password' | buildworld reset-root-password --password-stdin
```

`pause` 会先卸载后台服务，避免 macOS LaunchAgent 立即拉起旧进程；`resume` 和 `restart` 恢复服务。更新失败时，上个 bundle 保留在安装目录同级的 `.previous` 目录，按 [how-to-use.md](how-to-use.md#回滚) 回滚。

## Jenkins 并行迁移

1. 保持 Jenkins 原项目和触发器不变，BuildWorld 项目先使用手动触发或测试环境。
2. 导入 Jenkinsfile，由迁移器生成受限 TypeScript 配置并处理所有转换 warning。凭据、共享库、`post`、复杂脚本条件必须人工复核。
3. 同一 commit 分别运行两个工具，比较环境变量、工具链、签名、日志、制品、飞书消息与部署结果。
4. 连续多次一致后，先关闭 Jenkins 对应触发器，再启用 BuildWorld 触发器。保留 Jenkins 配置用于回滚。

详细操作见 [how-to-use.md](how-to-use.md) 和 [macOS Jenkins 迁移说明](docs-site/docs/jenkins-migration-macos.md)。

## 飞书通知

BuildWorld 原生 Go 服务发送飞书机器人卡片，不依赖 Python。控制台中创建 `feishu` 通知通道，配置为：

```json
{"webhook_url":"https://open.feishu.cn/open-apis/bot/v2/hook/REDACTED"}
```

用一个无副作用项目先验证成功、失败和开始事件。Webhook 视为秘密：仅保存在通知通道或密钥管理系统，泄露后立刻在飞书侧重置。

## 本地验证与发布

仓库不使用 GitHub Actions。测试、文档构建与发布、六平台打包及 Release
上传都在受控机器本地完成。文档脚本默认只构建和校验，不写 Git：

```powershell
.\scripts\publish-docs-local.ps1
```

先显式运行规范化本地门禁；确认最终提交已推送到 `main`、工作区干净后，
再发布静态站点到 `gh-pages`：

```powershell
.\scripts\verify-local.ps1
.\scripts\publish-docs-local.ps1 -Publish
```

发布脚本会再次强制执行同一本地门禁与双语言文档构建，请求高影响操作
确认，使用临时 worktree、写入 `.nojekyll`，再配置 GitHub Pages 从分支
根目录发布。它只接受指向 `neko233-com/buildworld233` 的 GitHub fetch 与
push URL，并等待本次发布 commit 对应的 Pages 构建成功。

所有平台二进制同样必须本地打包：

```powershell
.\scripts\release-local.ps1 -Version 1.0.0
```

产物位于 `release/v1.0.0/`，包括 Windows、Linux、macOS 的 amd64/arm64 完整 bundle、安装脚本、可校验分片和 `checksums.txt`。

本地发布器默认只执行完整门禁、打包和校验，不写 GitHub。确认提交已推送且
生产验证通过后，显式替换 `v1.0.0` 并删除其他旧 Release（二进制删除，
旧源码 tag 保留）：

```powershell
.\scripts\publish-release-local.ps1 -Version 1.0.0
.\scripts\publish-release-local.ps1 -Version 1.0.0 -Publish -ReplaceExisting -PruneOtherReleases
```

正式发布需要高影响操作确认；上传先进入 draft，远端资产名称与大小全部
匹配后才公开为 Latest。

## 开发验证

```powershell
go test ./...
Set-Location web; npm ci; npm test -- --run; npm run build
Set-Location ..\docs-site; npm ci; npm run build
```

Unity/Tuanjie 构建不在自动验证范围内；仅按项目实际工具链手动验证。
