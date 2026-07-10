// SVN plugin — checkout, update, commit, info
// Config keys: url, username, password, revision, path

// svn-checkout: checkout an SVN repository
registerStep("svn-checkout", function(ctx) {
    var url = ctx.config.url || "";
    var revision = ctx.config.revision || "HEAD";
    var path = ctx.config.path || ".";
    var username = ctx.config.username || "";
    var password = ctx.config.password || "";

    if (!url) {
        ctx.fail("url is required");
        return;
    }

    var cmd = "svn checkout";
    if (username) cmd += " --username " + username;
    if (password) cmd += " --password " + password + " --non-interactive --trust-server-cert";
    if (revision) cmd += " -r " + revision;
    cmd += " " + url + " " + path;

    ctx.log("SVN checkout: " + url);
    var result = exec(cmd);
    if (result.error) {
        ctx.fail("svn checkout failed: " + result.error + "\n" + result.output);
        return;
    }
    ctx.log(result.output);
    ctx.log("SVN checkout complete");
});

// svn-update: update working copy
registerStep("svn-update", function(ctx) {
    var path = ctx.config.path || ".";
    var revision = ctx.config.revision || "";
    var username = ctx.config.username || "";
    var password = ctx.config.password || "";

    var cmd = "svn update " + path;
    if (revision) cmd += " -r " + revision;
    if (username) cmd += " --username " + username;
    if (password) cmd += " --password " + password + " --non-interactive --trust-server-cert";

    ctx.log("SVN update: " + path);
    var result = exec(cmd);
    if (result.error) {
        ctx.fail("svn update failed: " + result.error);
        return;
    }
    ctx.log(result.output);
});

// svn-commit: commit changes
registerStep("svn-commit", function(ctx) {
    var path = ctx.config.path || ".";
    var message = ctx.config.message || "buildworld233 auto-commit";
    var username = ctx.config.username || "";
    var password = ctx.config.password || "";

    var cmd = "svn commit " + path + " -m \"" + message.replace(/"/g, '\\"') + "\"";
    if (username) cmd += " --username " + username;
    if (password) cmd += " --password " + password + " --non-interactive --trust-server-cert";

    ctx.log("SVN commit: " + path);
    var result = exec(cmd);
    if (result.error) {
        ctx.fail("svn commit failed: " + result.error);
        return;
    }
    ctx.log(result.output);
});

// svn-info: get working copy info
registerStep("svn-info", function(ctx) {
    var path = ctx.config.path || ".";
    var result = exec("svn info " + path);
    if (result.error) {
        ctx.fail("svn info failed: " + result.error);
        return;
    }
    ctx.log(result.output);
    var rev = result.output.match(/Revision:\s*(\d+)/);
    if (rev) {
        ctx.output("revision", rev[1]);
    }
});
