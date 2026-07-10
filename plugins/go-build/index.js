// Go 构建插件 — 编译、测试、检查、格式化、依赖下载、lint
// 配置项: package, output, os, arch, ldflags, race, cover, verbose

function goEnv(ctx) {
    var env = "";
    if (ctx.config.os) env += "GOOS=" + ctx.config.os + " ";
    if (ctx.config.arch) env += "GOARCH=" + ctx.config.arch + " ";
    return env;
}

// go-build: 编译
registerStep("go-build", function(ctx) {
    var pkg = ctx.config.package || ".";
    var output = ctx.config.output || "";
    var cmd = goEnv(ctx) + "go build";
    if (output) cmd += " -o " + output;
    if (ctx.config.ldflags) cmd += ' -ldflags "' + ctx.config.ldflags + '"';
    cmd += " " + pkg;

    ctx.log("编译 Go 项目: " + cmd);
    var result = exec(cmd);
    if (result.error) {
        ctx.fail("go build 失败: " + result.error + "\n" + result.output);
        return;
    }
    ctx.log(result.output);
    if (output) ctx.output("binary", output);
    ctx.log("编译成功");
});

// go-test: 测试
registerStep("go-test", function(ctx) {
    var pkg = ctx.config.package || "./...";
    var cmd = "go test";
    if (ctx.config.race === "true") cmd += " -race";
    if (ctx.config.cover === "true") cmd += " -cover";
    if (ctx.config.verbose === "true") cmd += " -v";
    if (ctx.config.timeout) cmd += " -timeout " + ctx.config.timeout;
    cmd += " " + pkg;

    ctx.log("执行测试: " + cmd);
    var result = exec(cmd);
    if (result.error) {
        ctx.fail("go test 失败: " + result.error + "\n" + result.output);
        return;
    }
    ctx.log(result.output);
    // 解析覆盖率
    if (ctx.config.cover === "true") {
        var coverMatch = result.output.match(/coverage:\s*(\d+\.\d+)%\s*of\s*statements/);
        if (coverMatch) {
            ctx.output("coverage", coverMatch[1] + "%");
            ctx.log("覆盖率: " + coverMatch[1] + "%");
        }
    }
    ctx.log("测试完成");
});

// go-vet: 静态检查
registerStep("go-vet", function(ctx) {
    var pkg = ctx.config.package || "./...";
    var cmd = "go vet " + pkg;
    ctx.log("静态检查: " + cmd);
    var result = exec(cmd);
    if (result.error) {
        ctx.fail("go vet 发现问题: " + result.error + "\n" + result.output);
        return;
    }
    ctx.log(result.output);
    ctx.log("静态检查通过");
});

// go-fmt: 格式化代码
registerStep("go-fmt", function(ctx) {
    var path = ctx.config.path || ".";
    var check = ctx.config.check === "true";
    var cmd = "gofmt";
    if (check) {
        cmd += " -l " + path;
        ctx.log("检查格式: " + cmd);
        var result = exec(cmd);
        if (result.error) {
            ctx.fail("gofmt 失败: " + result.error);
            return;
        }
        if (result.output.trim()) {
            ctx.fail("以下文件格式不符合要求:\n" + result.output);
            return;
        }
        ctx.log("所有文件格式正确");
    } else {
        cmd += " -w " + path;
        ctx.log("格式化代码: " + cmd);
        var result = exec(cmd);
        if (result.error) {
            ctx.fail("gofmt 失败: " + result.error);
            return;
        }
        ctx.log("格式化完成");
    }
});

// go-mod-download: 下载依赖
registerStep("go-mod-download", function(ctx) {
    var cmd = "go mod download";
    ctx.log("下载依赖: " + cmd);
    var result = exec(cmd);
    if (result.error) {
        ctx.fail("go mod download 失败: " + result.error + "\n" + result.output);
        return;
    }
    ctx.log(result.output);
    // 验证依赖
    var verifyResult = exec("go mod verify");
    if (!verifyResult.error) {
        ctx.log(verifyResult.output || "依赖验证通过");
    }
    ctx.log("依赖下载完成");
});

// go-lint: golangci-lint 检查
registerStep("go-lint", function(ctx) {
    var path = ctx.config.path || "./...";
    var cmd = "golangci-lint run " + path;
    if (ctx.config.config) cmd = "golangci-lint run -c " + ctx.config.config + " " + path;
    if (ctx.config.timeout) cmd += " --timeout " + ctx.config.timeout;

    ctx.log("运行 golangci-lint: " + cmd);
    var result = exec(cmd);
    if (result.error) {
        ctx.fail("golangci-lint 发现问题: " + result.error + "\n" + result.output);
        return;
    }
    ctx.log(result.output);
    ctx.log("lint 检查通过");
});
