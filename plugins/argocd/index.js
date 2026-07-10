// ArgoCD 插件 — GitOps 部署：同步、等待、回滚、应用创建、应用删除
// 配置项: server, auth_token, app, repo_url, path, dest_server, dest_namespace, revision, prune, dry_run

function argocdBase(ctx) {
    var cmd = "argocd";
    if (ctx.config.server) cmd += " --server " + ctx.config.server;
    if (ctx.config.auth_token) cmd += " --auth-token " + ctx.config.auth_token;
    if (ctx.config.insecure === "true") cmd += " --insecure";
    if (ctx.config.grpc_web === "true") cmd += " --grpc-web";
    return cmd;
}

// argocd-sync: 同步应用
registerStep("argocd-sync", function(ctx) {
    var app = ctx.config.app || "";
    if (!app) {
        ctx.fail("app 配置项必填");
        return;
    }
    var cmd = argocdBase(ctx) + " app sync " + app;
    if (ctx.config.revision) cmd += " --revision " + ctx.config.revision;
    if (ctx.config.prune === "true") cmd += " --prune";
    if (ctx.config.dry_run === "true") cmd += " --dry-run";
    if (ctx.config.force === "true") cmd += " --force";
    if (ctx.config.timeout) cmd += " --timeout " + ctx.config.timeout;
    if (ctx.config.strategy) cmd += " --strategy " + ctx.config.strategy;

    ctx.log("同步应用: " + cmd);
    var result = exec(cmd);
    if (result.error) {
        ctx.fail("argocd sync 失败: " + result.error + "\n" + result.output);
        return;
    }
    ctx.log(result.output);
    ctx.output("app", app);
    ctx.log("应用同步成功");
});

// argocd-wait: 等待应用达到 Healthy 状态
registerStep("argocd-wait", function(ctx) {
    var app = ctx.config.app || "";
    if (!app) {
        ctx.fail("app 配置项必填");
        return;
    }
    var cmd = argocdBase(ctx) + " app wait " + app;
    if (ctx.config.sync === "true") cmd += " --sync";
    if (ctx.config.health !== "false") cmd += " --health";
    if (ctx.config.operation === "true") cmd += " --operation";
    if (ctx.config.suspended === "true") cmd += " --suspended";
    if (ctx.config.timeout) cmd += " --timeout " + ctx.config.timeout;

    ctx.log("等待应用就绪: " + cmd);
    var result = exec(cmd);
    if (result.error) {
        ctx.fail("argocd wait 失败: " + result.error + "\n" + result.output);
        return;
    }
    ctx.log(result.output);
    ctx.output("status", "healthy");
    ctx.log("应用已就绪");
});

// argocd-rollback: 回滚应用
registerStep("argocd-rollback", function(ctx) {
    var app = ctx.config.app || "";
    if (!app) {
        ctx.fail("app 配置项必填");
        return;
    }
    var cmd = argocdBase(ctx) + " app rollback " + app;
    if (ctx.config.revision) {
        cmd += " " + ctx.config.revision;
    }
    if (ctx.config.prune === "true") cmd += " --prune";
    if (ctx.config.timeout) cmd += " --timeout " + ctx.config.timeout;

    ctx.log("回滚应用: " + cmd);
    var result = exec(cmd);
    if (result.error) {
        ctx.fail("argocd rollback 失败: " + result.error + "\n" + result.output);
        return;
    }
    ctx.log(result.output);
    ctx.log("应用回滚成功");
});

// argocd-app-create: 创建应用
registerStep("argocd-app-create", function(ctx) {
    var app = ctx.config.app || "";
    var repoUrl = ctx.config.repo_url || "";
    var path = ctx.config.path || "";
    var destServer = ctx.config.dest_server || "https://kubernetes.default.svc";
    var destNamespace = ctx.config.dest_namespace || "default";

    if (!app || !repoUrl) {
        ctx.fail("app 和 repo_url 配置项必填");
        return;
    }
    var cmd = argocdBase(ctx) + " app create " + app +
        " --repo " + repoUrl +
        " --path " + path +
        " --dest-server " + destServer +
        " --dest-namespace " + destNamespace;
    if (ctx.config.revision) cmd += " --revision " + ctx.config.revision;
    if (ctx.config.sync_policy === "auto") cmd += " --sync-policy automated";
    if (ctx.config.auto_prune === "true") cmd += " --auto-prune";
    if (ctx.config.self_heal === "true") cmd += " --self-heal";
    if (ctx.config.project) cmd += " --project " + ctx.config.project;
    if (ctx.config.upgrade === "true") cmd += " --upsert";

    ctx.log("创建应用: " + cmd);
    var result = exec(cmd);
    if (result.error) {
        ctx.fail("argocd app create 失败: " + result.error + "\n" + result.output);
        return;
    }
    ctx.log(result.output);
    ctx.output("app", app);
    ctx.log("应用创建成功");
});

// argocd-app-delete: 删除应用
registerStep("argocd-app-delete", function(ctx) {
    var app = ctx.config.app || "";
    if (!app) {
        ctx.fail("app 配置项必填");
        return;
    }
    var cmd = argocdBase(ctx) + " app delete " + app;
    if (ctx.config.cascade !== "false") cmd += " --cascade"; // 默认级联删除
    if (ctx.config.yes === "true") cmd += " --yes";

    ctx.log("删除应用: " + cmd);
    var result = exec(cmd);
    if (result.error) {
        ctx.fail("argocd app delete 失败: " + result.error + "\n" + result.output);
        return;
    }
    ctx.log(result.output);
    ctx.log("应用删除成功");
});
