// Telegram 插件 — 通知：发送消息和文件
// 配置项: bot_token, chat_id, message, parse_mode, file_path, caption

function telegramApi(ctx, method, payload) {
    var token = ctx.config.bot_token || "";
    if (!token) {
        ctx.fail("bot_token 配置项必填");
        return null;
    }
    var url = "https://api.telegram.org/bot" + token + "/" + method;

    var resp = http("POST", url, {
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify(payload)
    });

    if (resp.error) {
        ctx.fail("Telegram API 错误: " + resp.error);
        return null;
    }
    if (resp.status >= 400) {
        ctx.fail("Telegram API " + resp.status + ": " + resp.body);
        return null;
    }
    return JSON.parse(resp.body);
}

// telegram-notify: 发送消息
registerStep("telegram-notify", function(ctx) {
    var chatId = ctx.config.chat_id || "";
    var message = ctx.config.message || "构建完成";
    var parseMode = ctx.config.parse_mode || "Markdown";
    var disablePreview = ctx.config.disable_preview === "true";

    if (!chatId) {
        ctx.fail("chat_id 配置项必填");
        return;
    }

    // 构建消息（附带构建信息）
    var text = message;
    if (ctx.config.include_info === "true") {
        text += "\n\n*分支:* `" + (ctx.branch || "N/A") + "`\n*Commit:* `" + (ctx.commit ? ctx.commit.substring(0, 7) : "N/A") + "`";
    }

    ctx.log("发送 Telegram 消息到 " + chatId);
    var result = telegramApi(ctx, "sendMessage", {
        chat_id: chatId,
        text: text,
        parse_mode: parseMode,
        disable_web_page_preview: disablePreview
    });

    if (result && result.ok) {
        ctx.output("message_id", result.result.message_id);
        ctx.log("消息发送成功");
    }
});

// telegram-send-file: 发送文件
registerStep("telegram-send-file", function(ctx) {
    var chatId = ctx.config.chat_id || "";
    var filePath = ctx.config.file_path || "";
    var caption = ctx.config.caption || "";

    if (!chatId || !filePath) {
        ctx.fail("chat_id 和 file_path 配置项必填");
        return;
    }

    ctx.log("发送文件到 Telegram: " + filePath);

    // 使用 multipart 上传文件（通过 curl 调用）
    var token = ctx.config.bot_token || "";
    var url = "https://api.telegram.org/bot" + token + "/sendDocument";
    var cmd = 'curl -s -X POST "' + url + '"' +
        ' -F chat_id="' + chatId + '"' +
        ' -F document=@"' + filePath + '"';
    if (caption) cmd += ' -F caption="' + caption.replace(/"/g, '\\"') + '"';

    var result = exec(cmd);
    if (result.error) {
        ctx.fail("文件发送失败: " + result.error + "\n" + result.output);
        return;
    }

    // 解析响应
    try {
        var data = JSON.parse(result.output);
        if (data.ok) {
            ctx.output("document_id", data.result.document.file_id);
            ctx.log("文件发送成功");
        } else {
            ctx.fail("Telegram 返回错误: " + data.description);
        }
    } catch (e) {
        ctx.log(result.output);
        ctx.log("文件发送完成（无法解析响应）");
    }
});
