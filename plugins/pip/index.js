// pip 插件 — Python 包管理：安装、测试、构建、发布、代码检查
// 配置项: requirements, python, index_url, repository, username, password, package_dir

function pythonCmd(ctx) {
    return ctx.config.python || "python";
}

// pip-install: 安装依赖
registerStep("pip-install", function(ctx) {
    var py = pythonCmd(ctx);
    var requirements = ctx.config.requirements || "requirements.txt";
    var indexUrl = ctx.config.index_url || "";
    var cmd = py + " -m pip install -r " + requirements;
    if (indexUrl) cmd += " -i " + indexUrl;
    if (ctx.config.extra_index_url) cmd += " --extra-index-url " + ctx.config.extra_index_url;

    ctx.log("安装依赖: " + cmd);
    var result = exec(cmd);
    if (result.error) {
        ctx.fail("pip install 失败: " + result.error + "\n" + result.output);
        return;
    }
    ctx.log(result.output);
    ctx.log("依赖安装成功");
});

// pip-test: 执行测试
registerStep("pip-test", function(ctx) {
    var py = pythonCmd(ctx);
    var runner = ctx.config.runner || "pytest";
    var cmd = py + " -m " + runner;
    var testDir = ctx.config.test_dir || "tests";
    cmd += " " + testDir;
    if (ctx.config.coverage === "true") {
        cmd = py + " -m coverage run -m " + runner + " " + testDir;
    }
    if (ctx.config.verbose === "true") cmd += " -v";

    ctx.log("执行测试: " + cmd);
    var result = exec(cmd);
    if (result.error) {
        ctx.fail("测试失败: " + result.error + "\n" + result.output);
        return;
    }
    ctx.log(result.output);

    // 若启用覆盖率，生成报告
    if (ctx.config.coverage === "true") {
        var reportResult = exec(py + " -m coverage report");
        if (!reportResult.error) {
            ctx.log(reportResult.output);
        }
        ctx.output("coverage_report", ctx.workspace + "/.coverage");
    }
    ctx.log("测试完成");
});

// pip-build: 构建 sdist/wheel
registerStep("pip-build", function(ctx) {
    var py = pythonCmd(ctx);
    // 优先使用 build 模块，回退到 setup.py
    var checkBuild = exec(py + " -c \"import build\" 2>nul");
    var cmd;
    if (checkBuild.error) {
        // 回退: 先装 build
        ctx.log("安装 build 工具");
        exec(py + " -m pip install build");
    }
    cmd = py + " -m build";
    if (ctx.config.dist_dir) cmd += " --outdir " + ctx.config.dist_dir;

    ctx.log("构建包: " + cmd);
    var result = exec(cmd);
    if (result.error) {
        ctx.fail("构建失败: " + result.error + "\n" + result.output);
        return;
    }
    ctx.log(result.output);
    ctx.output("dist", ctx.config.dist_dir || ctx.workspace + "/dist");
    ctx.log("包构建完成");
});

// pip-publish: 发布到 PyPI
registerStep("pip-publish", function(ctx) {
    var py = pythonCmd(ctx);
    var repository = ctx.config.repository || "pypi";
    var username = ctx.config.username || "";
    var password = ctx.config.password || "";
    var distDir = ctx.config.dist_dir || ctx.workspace + "/dist";

    // 确保 twine 已安装
    var twineCheck = exec(py + " -m twine --version 2>nul");
    if (twineCheck.error) {
        ctx.log("安装 twine");
        var installRes = exec(py + " -m pip install twine");
        if (installRes.error) {
            ctx.fail("twine 安装失败: " + installRes.error);
            return;
        }
    }

    var cmd = py + " -m twine upload " + distDir + "/*";
    if (repository !== "pypi") {
        cmd += " --repository " + repository;
    }
    // 通过环境变量传递凭据（避免命令行暴露）
    if (username && password) {
        cmd = "TWINE_USERNAME=" + username + " TWINE_PASSWORD=" + password + " " + cmd;
    }

    ctx.log("发布到 PyPI");
    var result = exec(cmd);
    if (result.error) {
        ctx.fail("发布失败: " + result.error + "\n" + result.output);
        return;
    }
    ctx.log(result.output);
    ctx.log("发布成功");
});

// pip-lint: 代码检查
registerStep("pip-lint", function(ctx) {
    var py = pythonCmd(ctx);
    var linter = ctx.config.linter || "flake8";
    var path = ctx.config.path || ".";
    var cmd = py + " -m " + linter + " " + path;
    if (ctx.config.config) cmd += " --config " + ctx.config.config;

    ctx.log("代码检查: " + cmd);
    var result = exec(cmd);
    if (result.error) {
        ctx.fail(linter + " 发现问题: " + result.error + "\n" + result.output);
        return;
    }
    ctx.log(result.output);
    ctx.log("代码检查通过");
});
