// Slack plugin — send notifications via Incoming Webhook
// Config keys: webhook_url, channel, username, icon_emoji

// slack-notify: send a message to Slack
registerStep("slack-notify", function(ctx) {
    var webhookUrl = ctx.config.webhook_url || "";
    var message = ctx.config.message || "Build completed";
    var channel = ctx.config.channel || "";
    var username = ctx.config.username || "buildworld233";
    var iconEmoji = ctx.config.icon_emoji || ":hammer_and_wrench:";
    var color = ctx.config.color || "good";

    if (!webhookUrl) {
        ctx.fail("webhook_url is required");
        return;
    }

    ctx.log("Sending Slack notification: " + message);

    var payload = {
        username: username,
        icon_emoji: iconEmoji,
        text: message
    };

    if (channel) {
        payload.channel = channel;
    }

    if (ctx.config.attachment === "true") {
        payload.attachments = [{
            color: color,
            text: message,
            fields: [
                { title: "Branch", value: ctx.branch, short: true },
                { title: "Commit", value: ctx.commit.substring(0, 7), short: true }
            ]
        }];
        payload.text = "";
    }

    var resp = http("POST", webhookUrl, {
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify(payload)
    });

    if (resp.error) {
        ctx.fail("Slack API error: " + resp.error);
        return;
    }
    if (resp.status !== 200) {
        ctx.fail("Slack returned status " + resp.status + ": " + resp.body);
        return;
    }

    ctx.log("Slack notification sent successfully");
});
