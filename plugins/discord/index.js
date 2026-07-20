// Discord 插件 — 通知：发送消息和 Embed
// 配置项: webhook_url, username, avatar_url, message, title, description, color, fields

// discord-notify: 发送消息
registerStep("discord-notify", function(ctx) {
    var webhookUrl = ctx.config.webhook_url || "";
    var message = ctx.config.message || "构建完成";
    var username = ctx.config.username || "buildworld";
    var avatarUrl = ctx.config.avatar_url || "";

    if (!webhookUrl) {
        ctx.fail("webhook_url 配置项必填");
        return;
    }

    var payload = {
        username: username,
        content: message
    };
    if (avatarUrl) payload.avatar_url = avatarUrl;

    ctx.log("发送 Discord 消息");
    var resp = http("POST", webhookUrl, {
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify(payload)
    });

    if (resp.error) {
        ctx.fail("Discord webhook 错误: " + resp.error);
        return;
    }
    // Discord 成功返回 204 No Content
    if (resp.status !== 204 && resp.status !== 200) {
        ctx.fail("Discord 返回状态 " + resp.status + ": " + resp.body);
        return;
    }
    ctx.log("消息发送成功");
});

// discord-send-embed: 发送 Embed 消息
registerStep("discord-send-embed", function(ctx) {
    var webhookUrl = ctx.config.webhook_url || "";
    if (!webhookUrl) {
        ctx.fail("webhook_url 配置项必填");
        return;
    }

    var title = ctx.config.title || "构建通知";
    var description = ctx.config.description || "";
    var color = parseInt(ctx.config.color || "3447003", 10); // 默认蓝色
    var url = ctx.config.url || "";

    // 构建字段
    var fields = [];
    if (ctx.config.include_info === "true") {
        fields.push({ name: "分支", value: ctx.branch || "N/A", inline: true });
        fields.push({ name: "Commit", value: ctx.commit ? ctx.commit.substring(0, 7) : "N/A", inline: true });
    }
    if (ctx.config.fields) {
        ctx.config.fields.split("|").forEach(function(f) {
            var parts = f.split(":");
            if (parts.length >= 2) {
                fields.push({ name: parts[0].trim(), value: parts.slice(1).join(":").trim(), inline: true });
            }
        });
    }

    var embed = {
        title: title,
        description: description,
        color: color,
        timestamp: new Date().toISOString(),
        fields: fields
    };
    if (url) embed.url = url;
    if (ctx.config.thumbnail) embed.thumbnail = { url: ctx.config.thumbnail };
    if (ctx.config.footer) embed.footer = { text: ctx.config.footer };

    var payload = {
        username: ctx.config.username || "buildworld",
        embeds: [embed]
    };

    ctx.log("发送 Discord Embed: " + title);
    var resp = http("POST", webhookUrl, {
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify(payload)
    });

    if (resp.error) {
        ctx.fail("Discord webhook 错误: " + resp.error);
        return;
    }
    if (resp.status !== 204 && resp.status !== 200) {
        ctx.fail("Discord 返回状态 " + resp.status + ": " + resp.body);
        return;
    }
    ctx.log("Embed 发送成功");
});
