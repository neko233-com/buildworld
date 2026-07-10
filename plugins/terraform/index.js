// Terraform 插件 — 基础设施即代码：初始化、计划、应用、销毁、校验、格式化
// 配置项: dir, backend_config, var_file, vars, workspace, auto_approve, destroy

function tfDir(ctx) {
    return ctx.config.dir ? " -chdir=" + ctx.config.dir : "";
}

function tfVars(ctx) {
    var opts = "";
    if (ctx.config.var_file) opts += " -var-file=" + ctx.config.var_file;
    if (ctx.config.vars) {
        ctx.config.vars.split(",").forEach(function(v) {
            opts += " -var \"" + v.trim() + "\"";
        });
    }
    return opts;
}

// terraform-init: 初始化
registerStep("terraform-init", function(ctx) {
    var cmd = "terraform" + tfDir(ctx) + " init";
    if (ctx.config.backend_config) {
        ctx.config.backend_config.split(",").forEach(function(b) {
            cmd += " -backend-config=" + b.trim();
        });
    }
    if (ctx.config.upgrade === "true") cmd += " -upgrade";

    ctx.log("初始化 Terraform: " + cmd);
    var result = exec(cmd);
    if (result.error) {
        ctx.fail("terraform init 失败: " + result.error + "\n" + result.output);
        return;
    }
    ctx.log(result.output);
    ctx.log("初始化成功");
});

// terraform-plan: 生成执行计划
registerStep("terraform-plan", function(ctx) {
    // 切换 workspace
    if (ctx.config.workspace) {
        var wsCmd = "terraform" + tfDir(ctx) + " workspace select " + ctx.config.workspace;
        var wsResult = exec(wsCmd);
        if (wsResult.error) {
            // workspace 不存在则创建
            var newWs = exec("terraform" + tfDir(ctx) + " workspace new " + ctx.config.workspace);
            if (newWs.error) {
                ctx.fail("切换 workspace 失败: " + newWs.error);
                return;
            }
        }
    }

    var cmd = "terraform" + tfDir(ctx) + " plan" + tfVars(ctx);
    if (ctx.config.out) cmd += " -out=" + ctx.config.out;
    if (ctx.config.destroy === "true") cmd += " -destroy";

    ctx.log("生成计划: " + cmd);
    var result = exec(cmd);
    if (result.error) {
        ctx.fail("terraform plan 失败: " + result.error + "\n" + result.output);
        return;
    }
    ctx.log(result.output);
    if (ctx.config.out) ctx.output("plan_file", ctx.config.out);
    ctx.log("计划生成成功");
});

// terraform-apply: 应用变更
registerStep("terraform-apply", function(ctx) {
    var planFile = ctx.config.out || ctx.config.plan;
    var cmd = "terraform" + tfDir(ctx) + " apply";
    if (planFile) {
        cmd += " " + planFile;
    } else {
        cmd += tfVars(ctx);
        if (ctx.config.auto_approve !== "false") cmd += " -auto-approve";
    }

    ctx.log("应用变更: " + cmd);
    var result = exec(cmd);
    if (result.error) {
        ctx.fail("terraform apply 失败: " + result.error + "\n" + result.output);
        return;
    }
    ctx.log(result.output);
    ctx.log("基础设施变更完成");
});

// terraform-destroy: 销毁基础设施
registerStep("terraform-destroy", function(ctx) {
    var cmd = "terraform" + tfDir(ctx) + " destroy" + tfVars(ctx);
    if (ctx.config.auto_approve !== "false") cmd += " -auto-approve";

    ctx.log("销毁基础设施: " + cmd);
    var result = exec(cmd);
    if (result.error) {
        ctx.fail("terraform destroy 失败: " + result.error + "\n" + result.output);
        return;
    }
    ctx.log(result.output);
    ctx.log("基础设施已销毁");
});

// terraform-validate: 校验配置
registerStep("terraform-validate", function(ctx) {
    var cmd = "terraform" + tfDir(ctx) + " validate";
    ctx.log("校验配置: " + cmd);
    var result = exec(cmd);
    if (result.error) {
        ctx.fail("terraform validate 失败: " + result.error + "\n" + result.output);
        return;
    }
    ctx.log(result.output);
    ctx.log("配置校验通过");
});

// terraform-fmt: 格式化配置文件
registerStep("terraform-fmt", function(ctx) {
    var path = ctx.config.dir || ".";
    var check = ctx.config.check === "true";
    var cmd = "terraform fmt";
    if (check) cmd += " -check";
    if (ctx.config.recursive !== "false") cmd += " -recursive";
    cmd += " " + path;

    ctx.log("格式化配置: " + cmd);
    var result = exec(cmd);
    if (result.error) {
        if (check) {
            ctx.fail("格式不符合要求:\n" + result.output);
            return;
        }
        ctx.fail("terraform fmt 失败: " + result.error);
        return;
    }
    ctx.log(result.output || "所有文件格式正确");
    ctx.log("格式化完成");
});
