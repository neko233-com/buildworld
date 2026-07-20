// Mercurial (hg) plugin — clone, pull, update, commit, push
// Config keys: url, branch, revision, path, username

// hg-clone: clone a Mercurial repository
registerStep("hg-clone", function(ctx) {
    var url = ctx.config.url || "";
    var branch = ctx.config.branch || "default";
    var path = ctx.config.path || ".";

    if (!url) {
        ctx.fail("url is required");
        return;
    }

    var cmd = "hg clone -b " + branch + " " + url + " " + path;
    ctx.log("hg clone: " + url);
    var result = exec(cmd);
    if (result.error) {
        ctx.fail("hg clone failed: " + result.error + "\n" + result.output);
        return;
    }
    ctx.log(result.output);
    ctx.log("hg clone complete");
});

// hg-pull: pull latest changes
registerStep("hg-pull", function(ctx) {
    var path = ctx.config.path || ".";
    var cmd = "hg pull -R " + path;
    if (ctx.config.branch) cmd += " -b " + ctx.config.branch;

    ctx.log("hg pull: " + path);
    var result = exec(cmd);
    if (result.error) {
        ctx.fail("hg pull failed: " + result.error);
        return;
    }
    ctx.log(result.output);
});

// hg-update: update to a revision
registerStep("hg-update", function(ctx) {
    var path = ctx.config.path || ".";
    var rev = ctx.config.revision || "tip";
    var cmd = "hg update -R " + path + " " + rev;

    ctx.log("hg update: " + path + " -> " + rev);
    var result = exec(cmd);
    if (result.error) {
        ctx.fail("hg update failed: " + result.error);
        return;
    }
    ctx.log(result.output);
});

// hg-commit: commit changes
registerStep("hg-commit", function(ctx) {
    var path = ctx.config.path || ".";
    var message = ctx.config.message || "buildworld auto-commit";
    var user = ctx.config.username || "buildworld <ci@buildworld.local>";
    var cmd = "hg commit -R " + path + " -m \"" + message.replace(/"/g, '\\"') + "\" -u \"" + user + "\"";

    ctx.log("hg commit: " + path);
    var result = exec(cmd);
    if (result.error && !result.output.indexOf("nothing changed") >= 0) {
        ctx.fail("hg commit failed: " + result.error);
        return;
    }
    ctx.log(result.output || "nothing to commit");
});

// hg-push: push changes
registerStep("hg-push", function(ctx) {
    var path = ctx.config.path || ".";
    var cmd = "hg push -R " + path;

    ctx.log("hg push: " + path);
    var result = exec(cmd);
    if (result.error) {
        ctx.fail("hg push failed: " + result.error);
        return;
    }
    ctx.log(result.output);
});
