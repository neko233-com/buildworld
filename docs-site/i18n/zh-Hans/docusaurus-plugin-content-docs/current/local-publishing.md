---
sidebar_position: 4
---

# 本地验证与 release tag 发布

BuildWorld 的验证、文档构建、六平台打包和 Release 上传都在本地完成，不使用 GitHub Actions。

**只有发布新的 `vX.Y.Z` release tag，才触发公开 HTML 文档的构建与发布。**
普通 `main` 推送、staging tag 和替换现有 Release 均不发布 GitHub Pages。
仍可显式执行本地预览构建。

## 前置条件

- Node.js 24 与 npm；
- Git 与 PowerShell 7（`pwsh`）；
- GitHub CLI 已通过 `gh auth login` 登录；
- 拥有 `neko233-com/buildworld` 的维护权限；
- 已完成必要的本地验证和生产检查。

文档公开可见，正文和截图中都不能包含密钥、凭据、Token 或私有主机地址。

## 预览文档

```powershell
.\scripts\publish-docs-local.ps1
```

默认 dry-run 安装锁定依赖、构建中英文 HTML、检查入口和截图资源，并拒绝符号链接。
它不读取远端发布状态、不 fetch、不提交、不推送，也不修改 Pages 设置。
推送 `main` 不会自动调用此命令。

## 发布新版本及其文档

复核源码、运行 `.\scripts\verify-local.ps1`，提交并推送到 `main`，保持工作区干净。
将下方 `X.Y.Z` 替换为新的版本号：

```powershell
.\scripts\publish-release-local.ps1 -Version X.Y.Z -Publish
```

确认后，本地发布器构建六平台 bundle、上传 staging draft、逐项校验资产大小和 SHA-256，
再创建最终 release tag 并公开 Release。仅当最终 tag 为新建时，才传入 `-ReleaseTag`
调用文档发布器。使用 `-ReplaceExisting` 替换现有 Release 时跳过文档发布。

文档发布器验证远端 tag 对应当前 `main` 上已复核的 `HEAD`，且 `HEAD` 等于 `origin/main`。
它拒绝旧版本和被改写的 tag，跳过已经发布的 tag，并在上传 HTML 前重新执行完整本地门禁。

隔离的临时 worktree 接收验证后的静态文件和 `.nojekyll`；发布器在
`.buildworld-release.json` 记录 `releaseTag` 与 `sourceCommit`，使用普通 push 推送到
`gh-pages`，并验证 GitHub Pages 成功构建的 commit 与本次 Pages commit 完全一致。
并发分支更新会导致失败，不会被覆盖。

GitHub 可能显示平台托管的 Pages 部署记录。服务从 `gh-pages:/` 提供本地生成的静态文件，
仓库不会运行 workflow，也不会在 GitHub runner 上构建文档。

## 恢复失败的文档阶段

若 Release 已成功而文档失败，错误信息会明确区分文档阶段。保留新 tag 和其源码提交，
在对应的干净、已复核源码上显式指定该 tag 重试：

```powershell
.\scripts\publish-docs-local.ps1 -ReleaseTag vX.Y.Z -Publish
```

此命令用于恢复同一次新版本的文档发布。成功发布后，重复运行相同 tag 会跳过 HTML 构建和推送。
若静态 commit 已提交，但 Pages 尚未确认构建成功，相同命令会重试该部署并校验准确 commit，
不重新构建 HTML，也不重复推送静态提交。

使用 `-ReleaseTag vX.Y.Z -Publish -WhatIf` 可读取远端状态、检查发布资格，不构建 HTML，也不写远端。
前面的默认 dry-run 用于本地构建，不读取远端发布状态。

不要使用 `npm run deploy`、force-push `gh-pages` 或 Actions workflow 作为备用发布路径。

