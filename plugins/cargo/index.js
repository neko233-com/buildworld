// Cargo 插件 — Rust 构建：编译、测试、文档、发布、clippy 检查、格式化
// 配置项: release, features, manifest, target, offline, token

function cargoBase(ctx) {
    var cmd = "cargo";
    if (ctx.config.manifest) cmd += " --manifest-path " + ctx.config.manifest;
    return cmd;
}

function cargoBuildOpts(ctx) {
    var opts = "";
    if (ctx.config.release === "true") opts += " --release";
    if (ctx.config.features) opts += " --features " + ctx.config.features;
    if (ctx.config.target) opts += " --target " + ctx.config.target;
    if (ctx.config.offline === "true") opts += " --offline";
    return opts;
}

// cargo-build: 编译
registerStep("cargo-build", function(ctx) {
    var cmd = cargoBase(ctx) + " build" + cargoBuildOpts(ctx);
    ctx.log("编译项目: " + cmd);
    var result = exec(cmd);
    if (result.error) {
        ctx.fail("cargo build 失败: " + result.error + "\n" + result.output);
        return;
    }
    ctx.log(result.output);
    var profile = ctx.config.release === "true" ? "release" : "debug";
    ctx.output("profile", profile);
    ctx.log("编译成功 (" + profile + ")");
});

// cargo-test: 测试
registerStep("cargo-test", function(ctx) {
    var cmd = cargoBase(ctx) + " test" + cargoBuildOpts(ctx);
    if (ctx.config.test_name) cmd += " " + ctx.config.test_name;
    ctx.log("执行测试: " + cmd);
    var result = exec(cmd);
    if (result.error) {
        ctx.fail("cargo test 失败: " + result.error + "\n" + result.output);
        return;
    }
    ctx.log(result.output);
    // 解析测试结果
    var match = result.output.match(/test result: (\w+)\. (\d+) passed; (\d+) failed; (\d+) ignored/);
    if (match) {
        ctx.output("passed", match[2]);
        ctx.output("failed", match[3]);
        ctx.output("ignored", match[4]);
    }
    ctx.log("测试完成");
});

// cargo-doc: 生成文档
registerStep("cargo-doc", function(ctx) {
    var cmd = cargoBase(ctx) + " doc" + cargoBuildOpts(ctx);
    if (ctx.config.no_deps === "true") cmd += " --no-deps";
    ctx.log("生成文档: " + cmd);
    var result = exec(cmd);
    if (result.error) {
        ctx.fail("cargo doc 失败: " + result.error + "\n" + result.output);
        return;
    }
    ctx.log(result.output);
    ctx.output("doc_dir", ctx.workspace + "/target/doc");
    ctx.log("文档生成完成");
});

// cargo-publish: 发布到 crates.io
registerStep("cargo-publish", function(ctx) {
    var cmd = cargoBase(ctx) + " publish";
    if (ctx.config.token) {
        // 通过环境变量传递 token
        cmd = "CARGO_REGISTRY_TOKEN=" + ctx.config.token + " " + cmd;
    }
    if (ctx.config.dry_run === "true") cmd += " --dry-run";
    if (ctx.config.offline === "true") cmd += " --offline";
    ctx.log("发布 crate: " + cmd);
    var result = exec(cmd);
    if (result.error) {
        ctx.fail("cargo publish 失败: " + result.error + "\n" + result.output);
        return;
    }
    ctx.log(result.output);
    ctx.log("发布成功");
});

// cargo-clippy: 代码静态检查
registerStep("cargo-clippy", function(ctx) {
    var cmd = cargoBase(ctx) + " clippy" + cargoBuildOpts(ctx);
    var deny = ctx.config.deny === "true";
    if (deny) cmd += " -- -D warnings";
    ctx.log("运行 clippy: " + cmd);
    var result = exec(cmd);
    if (result.error) {
        if (deny) {
            ctx.fail("clippy 发现警告/错误: " + result.error + "\n" + result.output);
            return;
        }
        ctx.log("clippy 发现警告（未拒绝）");
    }
    ctx.log(result.output);
    ctx.log("clippy 检查完成");
});

// cargo-fmt: 代码格式化
registerStep("cargo-fmt", function(ctx) {
    var cmd = "cargo fmt";
    if (ctx.config.manifest) cmd += " --manifest-path " + ctx.config.manifest;
    if (ctx.config.check === "true") cmd += " -- --check";
    ctx.log("格式化代码: " + cmd);
    var result = exec(cmd);
    if (result.error) {
        if (ctx.config.check === "true") {
            ctx.fail("代码格式不符合要求: " + result.error + "\n" + result.output);
            return;
        }
        ctx.fail("cargo fmt 失败: " + result.error + "\n" + result.output);
        return;
    }
    ctx.log(result.output);
    ctx.log("格式化完成");
});
