// Kubernetes 插件 — K8s 部署：应用配置、滚动更新、扩缩容、回滚、状态查询
// 配置项: namespace, kubeconfig, manifest, deployment, image, replicas, record

function kubectlBase(ctx) {
    var cmd = "kubectl";
    if (ctx.config.kubeconfig) cmd += " --kubeconfig " + ctx.config.kubeconfig;
    if (ctx.config.namespace) cmd += " -n " + ctx.config.namespace;
    return cmd;
}

// k8s-apply: 应用清单文件
registerStep("k8s-apply", function(ctx) {
    var manifest = ctx.config.manifest || "";
    if (!manifest) {
        ctx.fail("manifest 配置项必填（文件路径或目录）");
        return;
    }
    var cmd = kubectlBase(ctx) + " apply -f " + manifest;
    if (ctx.config.record === "true") cmd += " --record";

    ctx.log("应用清单: " + cmd);
    var result = exec(cmd);
    if (result.error) {
        ctx.fail("k8s apply 失败: " + result.error + "\n" + result.output);
        return;
    }
    ctx.log(result.output);
    ctx.log("清单应用成功");
});

// k8s-deploy: 滚动更新镜像
registerStep("k8s-deploy", function(ctx) {
    var deployment = ctx.config.deployment || "";
    var image = ctx.config.image || "";
    if (!deployment || !image) {
        ctx.fail("deployment 和 image 配置项必填");
        return;
    }
    var cmd = kubectlBase(ctx) + " set image deployment/" + deployment +
        " " + ctx.config.container + "=" + image;
    if (ctx.config.record === "true") cmd += " --record";

    ctx.log("更新镜像: " + cmd);
    var result = exec(cmd);
    if (result.error) {
        ctx.fail("k8s deploy 失败: " + result.error + "\n" + result.output);
        return;
    }
    ctx.log(result.output);
    ctx.output("deployment", deployment);
    ctx.output("image", image);
    ctx.log("镜像更新已触发");
});

// k8s-scale: 扩缩容
registerStep("k8s-scale", function(ctx) {
    var deployment = ctx.config.deployment || "";
    var replicas = ctx.config.replicas || "";
    if (!deployment || !replicas) {
        ctx.fail("deployment 和 replicas 配置项必填");
        return;
    }
    var cmd = kubectlBase(ctx) + " scale deployment/" + deployment + " --replicas=" + replicas;

    ctx.log("扩缩容: " + cmd);
    var result = exec(cmd);
    if (result.error) {
        ctx.fail("k8s scale 失败: " + result.error + "\n" + result.output);
        return;
    }
    ctx.log(result.output);
    ctx.output("replicas", replicas);
    ctx.log("扩缩容已执行: " + deployment + " -> " + replicas);
});

// k8s-rollback: 回滚
registerStep("k8s-rollback", function(ctx) {
    var deployment = ctx.config.deployment || "";
    if (!deployment) {
        ctx.fail("deployment 配置项必填");
        return;
    }
    var cmd = kubectlBase(ctx) + " rollout undo deployment/" + deployment;
    if (ctx.config.revision) cmd += " --to-revision=" + ctx.config.revision;

    ctx.log("回滚: " + cmd);
    var result = exec(cmd);
    if (result.error) {
        ctx.fail("k8s rollback 失败: " + result.error + "\n" + result.output);
        return;
    }
    ctx.log(result.output);
    ctx.log("回滚已触发");
});

// k8s-status: 查询滚动状态
registerStep("k8s-status", function(ctx) {
    var deployment = ctx.config.deployment || "";
    if (!deployment) {
        ctx.fail("deployment 配置项必填");
        return;
    }
    var cmd = kubectlBase(ctx) + " rollout status deployment/" + deployment;
    if (ctx.config.timeout) cmd += " --timeout=" + ctx.config.timeout;

    ctx.log("查询状态: " + cmd);
    var result = exec(cmd);
    if (result.error) {
        ctx.fail("部署状态异常: " + result.error + "\n" + result.output);
        return;
    }
    ctx.log(result.output);
    ctx.output("status", "successful");
    ctx.log("部署完成");
});
