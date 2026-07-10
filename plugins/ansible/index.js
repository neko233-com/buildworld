// Ansible 插件 — 配置管理：执行 playbook、安装角色、加密/解密、清单管理
// 配置项: playbook, inventory, vault_password_file, requirements, limit, tags, extra_vars

// ansible-playbook: 执行 playbook
registerStep("ansible-playbook", function(ctx) {
    var playbook = ctx.config.playbook || "";
    if (!playbook) {
        ctx.fail("playbook 配置项必填");
        return;
    }
    var cmd = "ansible-playbook " + playbook;
    if (ctx.config.inventory) cmd += " -i " + ctx.config.inventory;
    if (ctx.config.vault_password_file) cmd += " --vault-password-file " + ctx.config.vault_password_file;
    if (ctx.config.limit) cmd += " --limit " + ctx.config.limit;
    if (ctx.config.tags) cmd += " --tags " + ctx.config.tags;
    if (ctx.config.skip_tags) cmd += " --skip-tags " + ctx.config.skip_tags;
    if (ctx.config.extra_vars) {
        cmd += ' --extra-vars "' + ctx.config.extra_vars + '"';
    }
    if (ctx.config.verbose === "true") cmd += " -v";

    ctx.log("执行 playbook: " + cmd);
    var result = exec(cmd);
    if (result.error) {
        ctx.fail("ansible playbook 失败: " + result.error + "\n" + result.output);
        return;
    }
    ctx.log(result.output);
    ctx.log("playbook 执行完成");
});

// ansible-galaxy-install: 安装角色/集合
registerStep("ansible-galaxy-install", function(ctx) {
    var requirements = ctx.config.requirements || "";
    var type = ctx.config.type || "role"; // role 或 collection
    var cmd = "ansible-galaxy install";
    if (type === "collection") {
        cmd = "ansible-galaxy collection install";
    }
    if (requirements) {
        cmd += " -r " + requirements;
    } else if (ctx.config.name) {
        cmd += " " + ctx.config.name;
    } else {
        ctx.fail("requirements 或 name 配置项必填");
        return;
    }
    if (ctx.config.force === "true") cmd += " --force";

    ctx.log("安装 " + type + ": " + cmd);
    var result = exec(cmd);
    if (result.error) {
        ctx.fail("ansible-galaxy install 失败: " + result.error + "\n" + result.output);
        return;
    }
    ctx.log(result.output);
    ctx.log(type + " 安装完成");
});

// ansible-vault-encrypt: 加密文件
registerStep("ansible-vault-encrypt", function(ctx) {
    var file = ctx.config.file || "";
    if (!file) {
        ctx.fail("file 配置项必填");
        return;
    }
    var cmd = "ansible-vault encrypt " + file;
    if (ctx.config.vault_password_file) {
        cmd += " --vault-password-file " + ctx.config.vault_password_file;
    } else if (ctx.config.vault_id) {
        cmd += " --vault-id " + ctx.config.vault_id;
    }

    ctx.log("加密文件: " + cmd);
    var result = exec(cmd);
    if (result.error) {
        ctx.fail("vault encrypt 失败: " + result.error + "\n" + result.output);
        return;
    }
    ctx.log(result.output);
    ctx.log("文件已加密: " + file);
});

// ansible-vault-decrypt: 解密文件
registerStep("ansible-vault-decrypt", function(ctx) {
    var file = ctx.config.file || "";
    if (!file) {
        ctx.fail("file 配置项必填");
        return;
    }
    var cmd = "ansible-vault decrypt " + file;
    if (ctx.config.vault_password_file) {
        cmd += " --vault-password-file " + ctx.config.vault_password_file;
    } else if (ctx.config.vault_id) {
        cmd += " --vault-id " + ctx.config.vault_id;
    }
    if (ctx.config.output) cmd += " --output " + ctx.config.output;

    ctx.log("解密文件: " + cmd);
    var result = exec(cmd);
    if (result.error) {
        ctx.fail("vault decrypt 失败: " + result.error + "\n" + result.output);
        return;
    }
    ctx.log(result.output);
    ctx.log("文件已解密: " + file);
});

// ansible-inventory: 查看清单
registerStep("ansible-inventory", function(ctx) {
    var inventory = ctx.config.inventory || "";
    if (!inventory) {
        ctx.fail("inventory 配置项必填");
        return;
    }
    var cmd = "ansible-inventory -i " + inventory;
    var action = ctx.config.action || "list"; // list, graph, host
    if (action === "graph") {
        cmd += " --graph";
    } else if (action === "host" && ctx.config.host) {
        cmd += " --host " + ctx.config.host;
    } else {
        cmd += " --list";
    }

    ctx.log("查询清单: " + cmd);
    var result = exec(cmd);
    if (result.error) {
        ctx.fail("ansible-inventory 失败: " + result.error + "\n" + result.output);
        return;
    }
    ctx.log(result.output);
    ctx.output("inventory", result.output);
    ctx.log("清单查询完成");
});
