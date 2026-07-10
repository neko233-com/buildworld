// Gradle 插件 — 构建、测试、打包、发布、清理
// 配置项: wrapper(是否用gradlew), project_dir, tasks, skip_tests, publish_repo

// 选择 gradle 命令（优先使用 wrapper）
function gradleCmd(ctx) {
    var wrapper = ctx.config.wrapper !== "false"; // 默认使用 gradlew
    var dir = ctx.config.project_dir || "";
    var cmd = wrapper ? (dir ? dir + "/gradlew" : "./gradlew") : "gradle";
    return cmd;
}

function gradleOpts(ctx) {
    var opts = "";
    if (ctx.config.project_dir) opts += " -p " + ctx.config.project_dir;
    if (ctx.config.skip_tests === "true") opts += " -x test";
    return opts;
}

// gradle-clean: 清理
registerStep("gradle-clean", function(ctx) {
    var cmd = gradleCmd(ctx) + " clean" + gradleOpts(ctx);
    ctx.log("清理构建: " + cmd);
    var result = exec(cmd);
    if (result.error) {
        ctx.fail("gradle clean 失败: " + result.error + "\n" + result.output);
        return;
    }
    ctx.log(result.output);
    ctx.log("清理完成");
});

// gradle-build: 构建
registerStep("gradle-build", function(ctx) {
    var cmd = gradleCmd(ctx) + " build" + gradleOpts(ctx);
    ctx.log("构建项目: " + cmd);
    var result = exec(cmd);
    if (result.error) {
        ctx.fail("gradle build 失败: " + result.error + "\n" + result.output);
        return;
    }
    ctx.log(result.output);
    ctx.output("build_dir", ctx.workspace + "/build");
    ctx.log("构建成功");
});

// gradle-test: 测试
registerStep("gradle-test", function(ctx) {
    var cmd = gradleCmd(ctx) + " test" + gradleOpts(ctx);
    if (ctx.config.test_filter) {
        cmd += ' --tests "' + ctx.config.test_filter + '"';
    }
    ctx.log("执行测试: " + cmd);
    var result = exec(cmd);
    if (result.error) {
        ctx.fail("gradle test 失败: " + result.error + "\n" + result.output);
        return;
    }
    ctx.log(result.output);
    ctx.output("report", ctx.workspace + "/build/reports/tests/test");
    ctx.log("测试完成");
});

// gradle-jar: 生成 jar
registerStep("gradle-jar", function(ctx) {
    var cmd = gradleCmd(ctx) + " jar" + gradleOpts(ctx);
    ctx.log("生成 jar: " + cmd);
    var result = exec(cmd);
    if (result.error) {
        ctx.fail("gradle jar 失败: " + result.error + "\n" + result.output);
        return;
    }
    ctx.log(result.output);
    // 查找生成的 jar
    var findResult = exec("find " + (ctx.config.project_dir || ".") + "/build/libs -name '*.jar' 2>/dev/null | head -1");
    if (findResult.output) {
        ctx.output("jar", findResult.output.trim());
        ctx.log("产物: " + findResult.output.trim());
    }
    ctx.log("jar 生成完成");
});

// gradle-publish: 发布到仓库
registerStep("gradle-publish", function(ctx) {
    var cmd = gradleCmd(ctx) + " publish" + gradleOpts(ctx);
    // 通过属性传递仓库配置
    if (ctx.config.repo_url) {
        cmd += " -PrepoUrl=" + ctx.config.repo_url;
    }
    if (ctx.config.repo_username) {
        cmd += " -PrepoUsername=" + ctx.config.repo_username;
    }
    if (ctx.config.repo_password) {
        cmd += " -PrepoPassword=" + ctx.config.repo_password;
    }
    ctx.log("发布到仓库: " + cmd);
    var result = exec(cmd);
    if (result.error) {
        ctx.fail("gradle publish 失败: " + result.error + "\n" + result.output);
        return;
    }
    ctx.log(result.output);
    ctx.log("发布成功");
});
