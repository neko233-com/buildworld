// Maven 插件 — Java 构建：编译、测试、打包、部署、清理
// 配置项: goals, profiles, skip_tests, pom, settings, repo_url, repo_id

// 构建公共参数
function mavenArgs(ctx) {
    var args = "";
    var pom = ctx.config.pom || "";
    var profiles = ctx.config.profiles || "";
    var settings = ctx.config.settings || "";
    if (pom) args += " -f " + pom;
    if (profiles) args += " -P " + profiles;
    if (settings) args += " -s " + settings;
    return args;
}

// maven-clean: 清理构建产物
registerStep("maven-clean", function(ctx) {
    var cmd = "mvn clean" + mavenArgs(ctx);
    ctx.log("清理构建产物: " + cmd);
    var result = exec(cmd);
    if (result.error) {
        ctx.fail("maven clean 失败: " + result.error + "\n" + result.output);
        return;
    }
    ctx.log(result.output);
    ctx.log("清理完成");
});

// maven-compile: 编译源码
registerStep("maven-compile", function(ctx) {
    var cmd = "mvn compile" + mavenArgs(ctx);
    ctx.log("编译源码: " + cmd);
    var result = exec(cmd);
    if (result.error) {
        ctx.fail("maven compile 失败: " + result.error + "\n" + result.output);
        return;
    }
    ctx.log(result.output);
    ctx.output("classes", ctx.workspace + "/target/classes");
    ctx.log("编译成功");
});

// maven-test: 执行单元测试
registerStep("maven-test", function(ctx) {
    var cmd = "mvn test" + mavenArgs(ctx);
    if (ctx.config.test) {
        cmd += ' -Dtest="' + ctx.config.test + '"';
    }
    ctx.log("执行测试: " + cmd);
    var result = exec(cmd);
    if (result.error) {
        ctx.fail("maven test 失败: " + result.error + "\n" + result.output);
        return;
    }
    ctx.log(result.output);
    // 解析测试结果摘要
    var match = result.output.match(/Tests run:\s*(\d+),\s*Failures:\s*(\d+),\s*Errors:\s*(\d+),\s*Skipped:\s*(\d+)/);
    if (match) {
        ctx.output("total", match[1]);
        ctx.output("failures", match[2]);
        ctx.output("errors", match[3]);
        ctx.output("skipped", match[4]);
    }
    ctx.log("测试完成");
});

// maven-package: 打包
registerStep("maven-package", function(ctx) {
    var skipTests = ctx.config.skip_tests === "true" || ctx.config.skip_tests !== "false";
    var cmd = "mvn package" + mavenArgs(ctx);
    if (skipTests) cmd += " -DskipTests";

    ctx.log("打包项目: " + cmd);
    var result = exec(cmd);
    if (result.error) {
        ctx.fail("maven package 失败: " + result.error + "\n" + result.output);
        return;
    }
    ctx.log(result.output);
    // 尝试识别生成的 jar/war
    var jarMatch = result.output.match(/Building jar:\s*(\S+\.jar)/);
    if (jarMatch) {
        ctx.output("artifact", jarMatch[1]);
        ctx.log("产物: " + jarMatch[1]);
    }
    ctx.log("打包完成");
});

// maven-deploy: 部署到远程仓库
registerStep("maven-deploy", function(ctx) {
    var skipTests = ctx.config.skip_tests === "true";
    var cmd = "mvn deploy" + mavenArgs(ctx);
    if (skipTests) cmd += " -DskipTests";

    // 配置仓库认证
    var repoId = ctx.config.repo_id || "";
    var repoUser = ctx.config.repo_username || "";
    var repoPass = ctx.config.repo_password || "";
    if (repoId && repoUser && repoPass) {
        ctx.log("配置仓库认证: " + repoId);
        var srvCmd = 'mvn org.apache.maven.plugins:maven-help-plugin:evaluate ' +
            '-Dexpression=settings.servers -Doutput=' + ctx.workspace + '/.servers.xml';
        // 直接通过 settings.xml 注入 server
        var settingsXml = ctx.workspace + '/.m2/settings-deploy.xml';
        var xml = '<?xml version="1.0" encoding="UTF-8"?>\n<settings>\n  <servers>\n' +
            '    <server>\n      <id>' + repoId + '</id>\n' +
            '      <username>' + repoUser + '</username>\n' +
            '      <password>' + repoPass + '</password>\n' +
            '    </server>\n  </servers>\n</settings>';
        exec('mkdir -p ' + ctx.workspace + '/.m2');
        exec('echo "' + xml.replace(/"/g, '\\"') + '" > ' + settingsXml);
        cmd = "mvn deploy -s " + settingsXml + mavenArgs(ctx);
        if (skipTests) cmd += " -DskipTests";
    }

    ctx.log("部署到远程仓库: " + cmd);
    var result = exec(cmd);
    if (result.error) {
        ctx.fail("maven deploy 失败: " + result.error + "\n" + result.output);
        return;
    }
    ctx.log(result.output);
    ctx.log("部署成功");
});
