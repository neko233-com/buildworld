// Helm 插件 — 部署管理：安装、升级、回滚、卸载、检查、模板渲染
// 配置项: release, chart, namespace, kubeconfig, values, set, version, repo, wait

function helmBase(ctx) {
    var cmd = "helm";
    if (ctx.config.kubeconfig) cmd += " --kubeconfig " + ctx.config.kubeconfig;
    if (ctx.config.namespace) cmd += " -n " + ctx.config.namespace;
    return cmd;
}

function helmValues(ctx) {
    var opts = "";
    if (ctx.config.values) {
        // 支持多文件逗号分隔
        ctx.config.values.split(",").forEach(function(f) {
            opts += " -f " + f.trim();
        });
    }
    if (ctx.config.set) {
        ctx.config.set.split(",").forEach(function(s) {
            opts += " --set " + s.trim();
        });
    }
    return opts;
}

// helm-install: 安装 chart
registerStep("helm-install", function(ctx) {
    var release = ctx.config.release || "";
    var chart = ctx.config.chart || "";
    if (!release || !chart) {
        ctx.fail("release 和 chart 配置项必填");
        return;
    }
    var cmd = helmBase(ctx) + " install " + release + " " + chart + helmValues(ctx);
    if (ctx.config.version) cmd += " --version " + ctx.config.version;
    if (ctx.config.wait === "true") cmd += " --wait";
    if (ctx.config.timeout) cmd += " --timeout " + ctx.config.timeout;

    ctx.log("安装 chart: " + cmd);
    var result = exec(cmd);
    if (result.error) {
        ctx.fail("helm install 失败: " + result.error + "\n" + result.output);
        return;
    }
    ctx.log(result.output);
    ctx.output("release", release);
    ctx.log("chart 安装成功");
});

// helm-upgrade: 升级或安装 chart
registerStep("helm-upgrade", function(ctx) {
    var release = ctx.config.release || "";
    var chart = ctx.config.chart || "";
    if (!release || !chart) {
        ctx.fail("release 和 chart 配置项必填");
        return;
    }
    var cmd = helmBase(ctx) + " upgrade --install " + release + " " + chart + helmValues(ctx);
    if (ctx.config.version) cmd += " --version " + ctx.config.version;
    if (ctx.config.wait === "true") cmd += " --wait";
    if (ctx.config.atomic === "true") cmd += " --atomic";
    if (ctx.config.timeout) cmd += " --timeout " + ctx.config.timeout;

    ctx.log("升级 chart: " + cmd);
    var result = exec(cmd);
    if (result.error) {
        ctx.fail("helm upgrade 失败: " + result.error + "\n" + result.output);
        return;
    }
    ctx.log(result.output);
    ctx.output("release", release);
    ctx.log("chart 升级成功");
});

// helm-rollback: 回滚到指定版本
registerStep("helm-rollback", function(ctx) {
    var release = ctx.config.release || "";
    if (!release) {
        ctx.fail("release 配置项必填");
        return;
    }
    var cmd = helmBase(ctx) + " rollback " + release;
    if (ctx.config.revision) cmd += " " + ctx.config.revision;
    if (ctx.config.wait === "true") cmd += " --wait";
    if (ctx.config.timeout) cmd += " --timeout " + ctx.config.timeout;

    ctx.log("回滚 release: " + cmd);
    var result = exec(cmd);
    if (result.error) {
        ctx.fail("helm rollback 失败: " + result.error + "\n" + result.output);
        return;
    }
    ctx.log(result.output);
    ctx.log("回滚成功");
});

// helm-uninstall: 卸载 release
registerStep("helm-uninstall", function(ctx) {
    var release = ctx.config.release || "";
    if (!release) {
        ctx.fail("release 配置项必填");
        return;
    }
    var cmd = helmBase(ctx) + " uninstall " + release;
    if (ctx.config.keep_history === "true") cmd += " --keep-history";
    if (ctx.config.timeout) cmd += " --timeout " + ctx.config.timeout;

    ctx.log("卸载 release: " + cmd);
    var result = exec(cmd);
    if (result.error) {
        ctx.fail("helm uninstall 失败: " + result.error + "\n" + result.output);
        return;
    }
    ctx.log(result.output);
    ctx.log("卸载成功");
});

// helm-lint: 检查 chart
registerStep("helm-lint", function(ctx) {
    var chart = ctx.config.chart || "";
    if (!chart) {
        ctx.fail("chart 配置项必填");
        return;
    }
    var cmd = "helm lint " + chart + helmValues(ctx);

    ctx.log("检查 chart: " + cmd);
    var result = exec(cmd);
    if (result.error) {
        ctx.fail("helm lint 失败: " + result.error + "\n" + result.output);
        return;
    }
    ctx.log(result.output);
    ctx.log("chart 检查通过");
});

// helm-template: 渲染模板
registerStep("helm-template", function(ctx) {
    var release = ctx.config.release || "";
    var chart = ctx.config.chart || "";
    if (!chart) {
        ctx.fail("chart 配置项必填");
        return;
    }
    var cmd = "helm template " + release + " " + chart + helmValues(ctx);
    if (ctx.config.output_dir) cmd += " --output-dir " + ctx.config.output_dir;

    ctx.log("渲染模板: " + cmd);
    var result = exec(cmd);
    if (result.error) {
        ctx.fail("helm template 失败: " + result.error + "\n" + result.output);
        return;
    }
    ctx.log(result.output);
    ctx.output("manifest", result.output);
    ctx.log("模板渲染完成");
});
