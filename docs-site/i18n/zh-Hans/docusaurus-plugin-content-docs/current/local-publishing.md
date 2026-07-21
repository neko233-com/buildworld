---
sidebar_position: 3
---

# 本地验证与发布

BuildWorld 不使用 GitHub Actions。CI 检查、文档构建与 GitHub Pages 发布、
六平台打包和 Release 上传都在受控机器本地完成。

## 前置条件

- Node.js 24 与 npm；
- Git 与 PowerShell；
- 已通过 `gh auth login` 登录 GitHub CLI；
- 拥有 `neko233-com/buildworld233` 的管理员或维护者权限。

即使仓库为私有，GitHub Pages 仍公开可见。文档源码中禁止放入密钥、私有
主机名、Token 或凭据。

## 仅构建的 dry-run

默认命令安装锁定依赖、构建全部语言、检查必要入口并拒绝符号链接。它不
fetch、不提交、不推送，也不修改 GitHub 设置。

```powershell
.\scripts\publish-docs-local.ps1
```

## 发布

先运行规范化本地门禁，复核并提交源码，把该提交推送到 `main`，并保持
工作区干净。随后运行：

```powershell
.\scripts\verify-local.ps1
.\scripts\publish-docs-local.ps1 -Publish
```

发布具有高影响确认提示。确认后，脚本会再次强制执行同一本地门禁，只
接受预期的 GitHub fetch 与 push URL，校验本地 `HEAD` 等于
`origin/main`，创建隔离的临时 worktree，以 `docs-site/build` 替换其
内容，添加 `.nojekyll`，再把一个静态站点提交正常推送到 `gh-pages`。
随后脚本把 GitHub Pages 配置为从 `gh-pages:/` 分支发布，并等待本次发布
commit 对应的 Pages 构建成功。

分支推送后，GitHub 可能显示平台托管的 Pages 部署记录。它不会运行仓库
workflow，也不会在 GitHub runner 上构建文档；`.nojekyll` 会让服务直接
发布本地构建好的静态文件。

当前源码分支和索引不会被切换或重写。脚本使用普通 push，因此并发的
`gh-pages` 更新会导致发布失败，不会被强制覆盖。即使发布失败，临时
worktree 也会清理。

使用 PowerShell 标准预览模式，可在不读取或修改 Git/GitHub 发布状态的
情况下检查文档构建和确认边界。依赖安装仍可能访问配置的 npm registry：

```powershell
.\scripts\publish-docs-local.ps1 -Publish -WhatIf
```

不要使用 `npm run deploy`、手工 force-push `gh-pages`，也不要添加 Actions
workflow 作为备用发布路径。

## 本地 Release 发布

Release 发布器同样默认 dry-run：执行完整本地门禁、构建六平台 bundle、
逐项验证 SHA-256，但不写 GitHub：

```powershell
.\scripts\publish-release-local.ps1 -Version 1.0.0
```

复核提交已在 `origin/main` 且生产检查通过后，替换 `v1.0.0` 并删除其他
旧 Release：

```powershell
.\scripts\publish-release-local.ps1 -Version 1.0.0 -Publish -ReplaceExisting -PruneOtherReleases
```

正式发布需要高影响操作确认。脚本先创建 draft，本地分批上传，逐项核对
远端资产名称与大小，全部正确后才公开为 Latest。`-PruneOtherReleases`
删除旧 Release 二进制，但保留旧源码 tag。全程不使用 GitHub Actions。
