// Webhook 插件 — 通用 Webhook：发送和接收
// 配置项: url, method, headers, body, content_type, secret, event, expect_status

// webhook-send: 发送 webhook 请求
registerStep("webhook-send", function(ctx) {
    var url = ctx.config.url || "";
    var method = (ctx.config.method || "POST").toUpperCase();
    var body = ctx.config.body || "";
    var contentType = ctx.config.content_type || "application/json";
    var secret = ctx.config.secret || "";

    if (!url) {
        ctx.fail("url 配置项必填");
        return;
    }

    // 构建请求头
    var headers = {};
    headers["Content-Type"] = contentType;
    if (ctx.config.headers) {
        ctx.config.headers.split(",").forEach(function(h) {
            var parts = h.split(":");
            if (parts.length >= 2) {
                headers[parts[0].trim()] = parts.slice(1).join(":").trim();
            }
        });
    }

    // 附带构建上下文信息
    if (contentType === "application/json" && !body) {
        var payload = {
            event: ctx.config.event || "build",
            branch: ctx.branch,
            commit: ctx.commit,
            workspace: ctx.workspace,
            timestamp: new Date().toISOString()
        };
        if (ctx.config.message) payload.message = ctx.config.message;
        body = JSON.stringify(payload);
    }

    // 若配置了 secret，添加 X-Webhook-Secret 头
    if (secret) {
        headers["X-Webhook-Secret"] = secret;
    }

    ctx.log("发送 webhook: " + method + " " + url);
    var resp = http(method, url, {
        headers: headers,
        body: body,
        timeout: ctx.config.timeout ? parseFloat(ctx.config.timeout) : 30
    });

    if (resp.error) {
        ctx.fail("webhook 发送失败: " + resp.error);
        return;
    }

    ctx.log("响应状态: " + resp.status);
    ctx.output("status", resp.status);
    ctx.output("body", resp.body);

    // 校验期望状态码
    var expectStatus = ctx.config.expect_status || "";
    if (expectStatus) {
        var expected = parseInt(expectStatus, 10);
        if (resp.status !== expected) {
            ctx.fail("webhook 状态码不匹配: 期望 " + expected + ", 实际 " + resp.status);
            return;
        }
    } else if (resp.status >= 400) {
        ctx.fail("webhook 返回错误状态: " + resp.status + " " + resp.body);
        return;
    }

    ctx.log("webhook 发送成功");
});

// webhook-receive: 验证并处理接收到的 webhook（通过轮询端点拉取）
registerStep("webhook-receive", function(ctx) {
    var url = ctx.config.url || "";
    var secret = ctx.config.secret || "";
    var event = ctx.config.event || "";
    var timeout = ctx.config.timeout ? parseFloat(ctx.config.timeout) : 30;

    if (!url) {
        ctx.fail("url 配置项必填");
        return;
    }

    ctx.log("拉取 webhook: GET " + url);
    var headers = {};
    if (secret) headers["X-Webhook-Secret"] = secret;

    var resp = http("GET", url, {
        headers: headers,
        timeout: timeout
    });

    if (resp.error) {
        ctx.fail("webhook 拉取失败: " + resp.error);
        return;
    }
    if (resp.status >= 400) {
        ctx.fail("webhook 端点返回错误: " + resp.status + " " + resp.body);
        return;
    }

    ctx.output("status", resp.status);
    ctx.output("body", resp.body);

    // 解析并校验事件类型
    if (resp.body) {
        try {
            var payload = JSON.parse(resp.body);
            var payloadEvent = payload.event || payload.type || "";
            if (event && payloadEvent && payloadEvent !== event) {
                ctx.fail("事件类型不匹配: 期望 " + event + ", 实际 " + payloadEvent);
                return;
            }
            ctx.output("event", payloadEvent);
            ctx.log("收到事件: " + payloadEvent);
        } catch (e) {
            ctx.log("响应非 JSON 格式，直接输出");
        }
    }

    ctx.log("webhook 接收成功");
});
