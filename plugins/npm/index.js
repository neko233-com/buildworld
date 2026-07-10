// npm 插件 — Node.js 包管理：安装、构建、测试、发布、运行脚本
// 配置项: registry, token, script, dist_tag, access, scope

// npm-install: 安装依赖
registerStep("npm-install", function(ctx) {
    var registry = ctx.config.registry || "";
    var useCi = ctx.config.ci !== "false"; // 默认使用 npm ci
    var cmd = useCi ? "npm ci" : "npm install";
    if (registry) {
        cmd += " --registry " + registry;
    }

    ctx.log("安装依赖: " + cmd);
    var result = exec(cmd);
    if (result.error) {
        ctx.fail("npm install 失败: " + result.error + "\n" + result.output);
        return;
    }
    ctx.log(result.output);
    ctx.log("依赖安装成功");
});

// npm-build: 执行构建脚本
registerStep("npm-build", function(ctx) {
    var script = ctx.config.script || "build";
    var cmd = "npm run " + script;

    ctx.log("执行构建: " + cmd);
    var result = exec(cmd);
    if (result.error) {
        ctx.fail("npm build 失败: " + result.error + "\n" + result.output);
        return;
    }
    ctx.log(result.output);
    ctx.output("dist", ctx.workspace + "/dist");
    ctx.log("构建完成");
});

// npm-test: 执行测试
registerStep("npm-test", function(ctx) {
    var script = ctx.config.script || "test";
    var cmd = "npm run " + script;
    if (ctx.config.coverage === "true") {
        cmd += " -- --coverage";
    }

    ctx.log("执行测试: " + cmd);
    var result = exec(cmd);
    if (result.error) {
        ctx.fail("npm test 失败: " + result.error + "\n" + result.output);
        return;
    }
    ctx.log(result.output);
    ctx.log("测试通过");
});

// npm-publish: 发布到 npm 仓库
registerStep("npm-publish", function(ctx) {
    var registry = ctx.config.registry || "";
    var token = ctx.config.token || "";
    var distTag = ctx.config.dist_tag || "";
    var access = ctx.config.access || "public";

    // 配置 registry 认证
    if (registry && token) {
        var scope = ctx.config.scope || "";
        var authKey = scope ? scope + ":registry" : "//" + registry.replace(/^https?:\/\//, "") + ":_authToken";
        ctx.log("配置 registry 认证: " + authKey);
        var setCmd = 'npm config set "' + authKey + '" ' + token;
        var setRes = exec(setCmd);
        if (setRes.error) {
            ctx.fail("配置认证失败: " + setRes.error);
            return;
        }
    }

    var cmd = "npm publish --access " + access;
    if (registry) cmd += " --registry " + registry;
    if (distTag) cmd += " --tag " + distTag;

    ctx.log("发布包: " + cmd);
    var result = exec(cmd);
    if (result.error) {
        ctx.fail("npm publish 失败: " + result.error + "\n" + result.output);
        return;
    }
    ctx.log(result.output);
    ctx.log("发布成功");
});

// npm-run: 执行任意 npm 脚本
registerStep("npm-run", function(ctx) {
    var script = ctx.config.script || "";
    var args = ctx.config.args || "";

    if (!script) {
        ctx.fail("script 配置项必填");
        return;
    }

    var cmd = "npm run " + script;
    if (args) cmd += " -- " + args;

    ctx.log("运行脚本: " + cmd);
    var result = exec(cmd);
    if (result.error) {
        ctx.fail("npm run 失败: " + result.error + "\n" + result.output);
        return;
    }
    ctx.log(result.output);
    ctx.output("script", script);
    ctx.log("脚本执行完成");
});
