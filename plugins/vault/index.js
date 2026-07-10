// HashiCorp Vault 插件 — 密钥管理：读取、写入、删除、列举
// 配置项: addr, token, path, data, mount, format

function vaultBase(ctx) {
    var cmd = "vault";
    if (ctx.config.addr) {
        cmd = "VAULT_ADDR=" + ctx.config.addr + " " + cmd;
    }
    if (ctx.config.token) {
        cmd = "VAULT_TOKEN=" + ctx.config.token + " " + cmd;
    }
    return cmd;
}

function vaultPath(ctx) {
    var mount = ctx.config.mount || "secret";
    var path = ctx.config.path || "";
    if (!path) {
        return null;
    }
    // KV v2 路径格式: mount/data/path
    return mount + "/data/" + path;
}

// vault-read: 读取密钥
registerStep("vault-read", function(ctx) {
    var path = vaultPath(ctx);
    if (!path) {
        ctx.fail("path 配置项必填");
        return;
    }
    var cmd = vaultBase(ctx) + " kv get -format=json " + path;

    ctx.log("读取密钥: " + (ctx.config.mount || "secret") + "/" + ctx.config.path);
    var result = exec(cmd);
    if (result.error) {
        ctx.fail("vault read 失败: " + result.error + "\n" + result.output);
        return;
    }

    // 解析 JSON 输出并提取数据
    try {
        var data = JSON.parse(result.output);
        var secrets = data.data ? (data.data.data || data.data) : {};
        ctx.output("data", JSON.stringify(secrets));
        // 输出每个键值对
        for (var key in secrets) {
            ctx.output(key, secrets[key]);
        }
        ctx.log("密钥读取成功");
    } catch (e) {
        ctx.log(result.output);
        ctx.log("密钥读取完成（非 JSON 格式）");
    }
});

// vault-write: 写入密钥
registerStep("vault-write", function(ctx) {
    var path = vaultPath(ctx);
    if (!path) {
        ctx.fail("path 配置项必填");
        return;
    }
    var data = ctx.config.data || "";
    if (!data) {
        ctx.fail("data 配置项必填（格式: key=value,key2=value2）");
        return;
    }

    var cmd = vaultBase(ctx) + " kv put " + path;
    // 解析 data 为 key=value 参数
    data.split(",").forEach(function(pair) {
        var idx = pair.indexOf("=");
        if (idx > 0) {
            cmd += " " + pair.substring(0, idx).trim() + "=" + pair.substring(idx + 1).trim();
        }
    });

    ctx.log("写入密钥: " + (ctx.config.mount || "secret") + "/" + ctx.config.path);
    var result = exec(cmd);
    if (result.error) {
        ctx.fail("vault write 失败: " + result.error + "\n" + result.output);
        return;
    }
    ctx.log(result.output);
    ctx.log("密钥写入成功");
});

// vault-delete: 删除密钥
registerStep("vault-delete", function(ctx) {
    var path = vaultPath(ctx);
    if (!path) {
        ctx.fail("path 配置项必填");
        return;
    }
    var cmd = vaultBase(ctx) + " kv delete " + path;

    ctx.log("删除密钥: " + (ctx.config.mount || "secret") + "/" + ctx.config.path);
    var result = exec(cmd);
    if (result.error) {
        ctx.fail("vault delete 失败: " + result.error + "\n" + result.output);
        return;
    }
    ctx.log(result.output);
    ctx.log("密钥删除成功");
});

// vault-list: 列举密钥
registerStep("vault-list", function(ctx) {
    var mount = ctx.config.mount || "secret";
    var path = ctx.config.path || "";
    // KV v2 列举路径: mount/metadata/path
    var listPath = mount + "/metadata/" + path;
    var cmd = vaultBase(ctx) + " kv list -format=json " + listPath;

    ctx.log("列举密钥: " + mount + "/" + path);
    var result = exec(cmd);
    if (result.error) {
        ctx.fail("vault list 失败: " + result.error + "\n" + result.output);
        return;
    }

    // 解析输出
    try {
        var keys = JSON.parse(result.output);
        ctx.output("keys", JSON.stringify(keys));
        ctx.log("找到 " + keys.length + " 个密钥");
    } catch (e) {
        ctx.log(result.output);
        ctx.output("listing", result.output);
        ctx.log("列举完成");
    }
});
