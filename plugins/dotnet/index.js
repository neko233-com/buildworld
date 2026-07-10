// .NET 插件 — 还原、构建、测试、发布、打包
// 配置项: project, configuration, runtime, framework, version, nuget_source

function dotnetProject(ctx) {
    return ctx.config.project || "";
}

// dotnet-restore: 还原 NuGet 依赖
registerStep("dotnet-restore", function(ctx) {
    var cmd = "dotnet restore " + dotnetProject(ctx);
    if (ctx.config.nuget_source) {
        cmd += " --source " + ctx.config.nuget_source;
    }
    if (ctx.config.runtime) {
        cmd += " --runtime " + ctx.config.runtime;
    }
    ctx.log("还原依赖: " + cmd);
    var result = exec(cmd);
    if (result.error) {
        ctx.fail("dotnet restore 失败: " + result.error + "\n" + result.output);
        return;
    }
    ctx.log(result.output);
    ctx.log("依赖还原成功");
});

// dotnet-build: 构建
registerStep("dotnet-build", function(ctx) {
    var config = ctx.config.configuration || "Release";
    var cmd = "dotnet build " + dotnetProject(ctx) + " -c " + config;
    if (ctx.config.runtime) cmd += " -r " + ctx.config.runtime;
    if (ctx.config.framework) cmd += " -f " + ctx.config.framework;
    if (ctx.config.version) cmd += " -p:Version=" + ctx.config.version;
    if (ctx.config.no_restore !== "false") cmd += " --no-restore";

    ctx.log("构建项目: " + cmd);
    var result = exec(cmd);
    if (result.error) {
        ctx.fail("dotnet build 失败: " + result.error + "\n" + result.output);
        return;
    }
    ctx.log(result.output);
    ctx.output("configuration", config);
    ctx.log("构建成功");
});

// dotnet-test: 测试
registerStep("dotnet-test", function(ctx) {
    var config = ctx.config.configuration || "Debug";
    var cmd = "dotnet test " + dotnetProject(ctx) + " -c " + config;
    if (ctx.config.filter) cmd += ' --filter "' + ctx.config.filter + '"';
    if (ctx.config.collect_coverage === "true") {
        cmd += " --collect:\"XPlat Code Coverage\"";
    }
    if (ctx.config.logger) cmd += " --logger " + ctx.config.logger;

    ctx.log("执行测试: " + cmd);
    var result = exec(cmd);
    if (result.error) {
        ctx.fail("dotnet test 失败: " + result.error + "\n" + result.output);
        return;
    }
    ctx.log(result.output);
    ctx.log("测试完成");
});

// dotnet-publish: 发布
registerStep("dotnet-publish", function(ctx) {
    var config = ctx.config.configuration || "Release";
    var output = ctx.config.output || ctx.workspace + "/publish";
    var cmd = "dotnet publish " + dotnetProject(ctx) + " -c " + config + " -o " + output;
    if (ctx.config.runtime) cmd += " -r " + ctx.config.runtime;
    if (ctx.config.self_contained === "true") cmd += " --self-contained";
    if (ctx.config.version) cmd += " -p:Version=" + ctx.config.version;

    ctx.log("发布项目: " + cmd);
    var result = exec(cmd);
    if (result.error) {
        ctx.fail("dotnet publish 失败: " + result.error + "\n" + result.output);
        return;
    }
    ctx.log(result.output);
    ctx.output("publish_dir", output);
    ctx.log("发布成功: " + output);
});

// dotnet-pack: 打包 NuGet 包
registerStep("dotnet-pack", function(ctx) {
    var config = ctx.config.configuration || "Release";
    var output = ctx.config.output || ctx.workspace + "/nupkgs";
    var cmd = "dotnet pack " + dotnetProject(ctx) + " -c " + config + " -o " + output;
    if (ctx.config.version) cmd += " -p:PackageVersion=" + ctx.config.version;
    if (ctx.config.include_symbols === "true") cmd += " --include-symbols";

    ctx.log("打包 NuGet: " + cmd);
    var result = exec(cmd);
    if (result.error) {
        ctx.fail("dotnet pack 失败: " + result.error + "\n" + result.output);
        return;
    }
    ctx.log(result.output);
    ctx.output("nupkg_dir", output);
    ctx.log("打包完成: " + output);
});
