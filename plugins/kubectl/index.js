// kubectl 插件 — 原生 kubectl 操作：应用、删除、查询、日志、执行、滚动管理
// 配置项: kubeconfig, namespace, manifest, resource, name, selector, container, follow, tail

function kubectlBase(ctx) {
    var cmd = "kubectl";
    if (ctx.config.kubeconfig) cmd += " --kubeconfig " + ctx.config.kubeconfig;
    if (ctx.config.namespace) cmd += " -n " + ctx.config.namespace;
    return cmd;
}

// kubectl-apply: 应用清单
registerStep("kubectl-apply", function(ctx) {
    var manifest = ctx.config.manifest || "";
    if (!manifest) {
        ctx.fail("manifest 配置项必填");
        return;
    }
    var cmd = kubectlBase(ctx) + " apply -f " + manifest;
    if (ctx.config.dry_run === "true") cmd += " --dry-run=client";
    if (ctx.config.prune === "true") cmd += " --prune";

    ctx.log("应用清单: " + cmd);
    var result = exec(cmd);
    if (result.error) {
        ctx.fail("kubectl apply 失败: " + result.error + "\n" + result.output);
        return;
    }
    ctx.log(result.output);
    ctx.log("清单应用成功");
});

// kubectl-delete: 删除资源
registerStep("kubectl-delete", function(ctx) {
    var resource = ctx.config.resource || "";
    var name = ctx.config.name || "";
    var manifest = ctx.config.manifest || "";
    if (!resource && !manifest) {
        ctx.fail("resource 或 manifest 配置项必填");
        return;
    }
    var cmd = kubectlBase(ctx) + " delete";
    if (manifest) {
        cmd += " -f " + manifest;
    } else {
        cmd += " " + resource;
        if (name) cmd += " " + name;
    }
    if (ctx.config.selector) cmd += " -l " + ctx.config.selector;
    if (ctx.config.force === "true") cmd += " --force --grace-period=0";

    ctx.log("删除资源: " + cmd);
    var result = exec(cmd);
    if (result.error) {
        ctx.fail("kubectl delete 失败: " + result.error + "\n" + result.output);
        return;
    }
    ctx.log(result.output);
    ctx.log("资源删除成功");
});

// kubectl-get: 查询资源
registerStep("kubectl-get", function(ctx) {
    var resource = ctx.config.resource || "pods";
    var name = ctx.config.name || "";
    var output = ctx.config.output || "";
    var cmd = kubectlBase(ctx) + " get " + resource;
    if (name) cmd += " " + name;
    if (ctx.config.selector) cmd += " -l " + ctx.config.selector;
    if (output) cmd += " -o " + output;
    if (ctx.config.all_namespaces === "true") cmd += " --all-namespaces";
    if (ctx.config.watch === "true") cmd += " -w";

    ctx.log("查询资源: " + cmd);
    var result = exec(cmd);
    if (result.error) {
        ctx.fail("kubectl get 失败: " + result.error + "\n" + result.output);
        return;
    }
    ctx.log(result.output);
    ctx.output("result", result.output);
    ctx.log("查询完成");
});

// kubectl-logs: 获取日志
registerStep("kubectl-logs", function(ctx) {
    var resource = ctx.config.resource || "pod";
    var name = ctx.config.name || "";
    if (!name) {
        ctx.fail("name 配置项必填（pod 名称）");
        return;
    }
    var cmd = kubectlBase(ctx) + " logs " + resource + "/" + name;
    if (ctx.config.container) cmd += " -c " + ctx.config.container;
    if (ctx.config.follow === "true") cmd += " -f";
    if (ctx.config.tail) cmd += " --tail=" + ctx.config.tail;
    if (ctx.config.previous === "true") cmd += " --previous";
    if (ctx.config.since) cmd += " --since=" + ctx.config.since;

    ctx.log("获取日志: " + cmd);
    var result = exec(cmd);
    if (result.error) {
        ctx.fail("kubectl logs 失败: " + result.error + "\n" + result.output);
        return;
    }
    ctx.log(result.output);
    ctx.output("logs", result.output);
    ctx.log("日志获取完成");
});

// kubectl-exec: 在容器中执行命令
registerStep("kubectl-exec", function(ctx) {
    var resource = ctx.config.resource || "pod";
    var name = ctx.config.name || "";
    var command = ctx.config.command || "";
    if (!name || !command) {
        ctx.fail("name 和 command 配置项必填");
        return;
    }
    var cmd = kubectlBase(ctx) + " exec " + resource + "/" + name;
    if (ctx.config.container) cmd += " -c " + ctx.config.container;
    if (ctx.config.stdin === "true") cmd += " -i";
    if (ctx.config.tty === "true") cmd += " -t";
    cmd += " -- " + command;

    ctx.log("执行命令: " + cmd);
    var result = exec(cmd);
    if (result.error) {
        ctx.fail("kubectl exec 失败: " + result.error + "\n" + result.output);
        return;
    }
    ctx.log(result.output);
    ctx.output("output", result.output);
    ctx.log("命令执行完成");
});

// kubectl-rollout: 滚动管理
registerStep("kubectl-rollout", function(ctx) {
    var action = ctx.config.action || "status"; // status, history, undo, pause, resume
    var resource = ctx.config.resource || "deployment";
    var name = ctx.config.name || "";
    if (!name) {
        ctx.fail("name 配置项必填");
        return;
    }
    var cmd = kubectlBase(ctx) + " rollout " + action + " " + resource + "/" + name;
    if (action === "undo" && ctx.config.revision) {
        cmd += " --to-revision=" + ctx.config.revision;
    }
    if (action === "status" && ctx.config.timeout) {
        cmd += " --timeout=" + ctx.config.timeout;
    }

    ctx.log("滚动管理 (" + action + "): " + cmd);
    var result = exec(cmd);
    if (result.error) {
        ctx.fail("kubectl rollout 失败: " + result.error + "\n" + result.output);
        return;
    }
    ctx.log(result.output);
    ctx.output("action", action);
    ctx.log("滚动操作完成");
});
