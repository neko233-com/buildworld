# BuildWorld Web

BuildWorld 的 React + TypeScript 管理界面。生产构建由 Go 控制面提供，默认地址为 `http://127.0.0.1:8080`。

## 本地开发

需要 Node.js 24 和 npm。先启动 BuildWorld 服务端，再运行：

```powershell
npm ci
npm run dev
```

Vite 开发服务器监听 `http://127.0.0.1:8701`，并将 `/api` 与 `/ws` 代理到 `http://127.0.0.1:8080`。可通过 `BUILDWORLD_API_TARGET` 覆盖后端地址。

HTML、CSS、JavaScript 和 TypeScript 修改由 Vite 热更新，无需重新启动开发服务器。

## 本地验证

```powershell
npm test
npm run lint
npm run build
```

`npm run build` 输出到 `web/dist`，由仓库根目录的本地发布脚本打入各平台安装包。项目不使用 GitHub Actions。
