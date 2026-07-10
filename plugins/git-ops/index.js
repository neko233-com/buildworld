// Git 操作插件 — 克隆、拉取、推送、标签、合并、cherry-pick、发布
// 配置项: url, branch, path, tag, message, remote, commit, force, token, repo, api_url

// git-clone: 克隆仓库
registerStep("git-clone", function(ctx) {
    var url = ctx.config.url || "";
    var branch = ctx.config.branch || "";
    var path = ctx.config.path || "";
    var depth = ctx.config.depth || "";

    if (!url) {
        ctx.fail("url 配置项必填");
        return;
    }
    var cmd = "git clone";
    if (branch) cmd += " -b " + branch;
    if (depth) cmd += " --depth " + depth;
    cmd += " " + url;
    if (path) cmd += " " + path;

    ctx.log("克隆仓库: " + cmd);
    var result = exec(cmd);
    if (result.error) {
        ctx.fail("git clone 失败: " + result.error + "\n" + result.output);
        return;
    }
    ctx.log(result.output);
    ctx.log("克隆成功");
});

// git-pull: 拉取更新
registerStep("git-pull", function(ctx) {
    var remote = ctx.config.remote || "origin";
    var branch = ctx.config.branch || "";
    var cmd = "git pull " + remote;
    if (branch) cmd += " " + branch;
    if (ctx.config.rebase === "true") cmd += " --rebase";
    if (ctx.config.no_ff === "true") cmd += " --no-ff";

    ctx.log("拉取更新: " + cmd);
    var result = exec(cmd);
    if (result.error) {
        ctx.fail("git pull 失败: " + result.error + "\n" + result.output);
        return;
    }
    ctx.log(result.output);
    ctx.log("拉取成功");
});

// git-push: 推送变更
registerStep("git-push", function(ctx) {
    var remote = ctx.config.remote || "origin";
    var branch = ctx.config.branch || ctx.branch || "HEAD";
    var tags = ctx.config.tags === "true";
    var force = ctx.config.force === "true";
    var cmd = "git push " + remote + " " + branch;
    if (force) cmd += " --force-with-lease";
    if (tags) cmd = "git push " + remote + " --tags";

    ctx.log("推送变更: " + cmd);
    var result = exec(cmd);
    if (result.error) {
        ctx.fail("git push 失败: " + result.error + "\n" + result.output);
        return;
    }
    ctx.log(result.output);
    ctx.log("推送成功");
});

// git-tag: 创建标签
registerStep("git-tag", function(ctx) {
    var tag = ctx.config.tag || "";
    var message = ctx.config.message || "Release " + tag;
    if (!tag) {
        ctx.fail("tag 配置项必填");
        return;
    }
    var cmd = "git tag -a " + tag + ' -m "' + message.replace(/"/g, '\\"') + '"';
    if (ctx.config.commit) cmd += " " + ctx.config.commit;

    ctx.log("创建标签: " + cmd);
    var result = exec(cmd);
    if (result.error) {
        ctx.fail("git tag 失败: " + result.error + "\n" + result.output);
        return;
    }
    ctx.log(result.output);
    ctx.output("tag", tag);
    ctx.log("标签创建成功: " + tag);
});

// git-merge: 合并分支
registerStep("git-merge", function(ctx) {
    var branch = ctx.config.branch || "";
    if (!branch) {
        ctx.fail("branch 配置项必填");
        return;
    }
    var cmd = "git merge " + branch;
    if (ctx.config.no_ff === "true") cmd += " --no-ff";
    if (ctx.config.squash === "true") cmd += " --squash";
    if (ctx.config.message) cmd += ' -m "' + ctx.config.message.replace(/"/g, '\\"') + '"';

    ctx.log("合并分支: " + cmd);
    var result = exec(cmd);
    if (result.error) {
        ctx.fail("git merge 失败: " + result.error + "\n" + result.output);
        return;
    }
    ctx.log(result.output);
    ctx.log("合并成功");
});

// git-cherry-pick: cherry-pick 指定 commit
registerStep("git-cherry-pick", function(ctx) {
    var commit = ctx.config.commit || "";
    if (!commit) {
        ctx.fail("commit 配置项必填");
        return;
    }
    var cmd = "git cherry-pick " + commit;
    if (ctx.config.no_commit === "true") cmd += " --no-commit";

    ctx.log("cherry-pick: " + cmd);
    var result = exec(cmd);
    if (result.error) {
        ctx.fail("git cherry-pick 失败: " + result.error + "\n" + result.output);
        return;
    }
    ctx.log(result.output);
    ctx.log("cherry-pick 成功");
});

// git-release: 通过 GitHub/Gitea API 创建 Release
registerStep("git-release", function(ctx) {
    var token = ctx.config.token || "";
    var repo = ctx.config.repo || "";
    var apiUrl = ctx.config.api_url || "https://api.github.com";
    var tagName = ctx.config.tag || ("v" + ctx.commit.substring(0, 7));
    var name = ctx.config.name || tagName;
    var body = ctx.config.body || "Release " + tagName;

    if (!token || !repo) {
        ctx.fail("token 和 repo 配置项必填");
        return;
    }

    ctx.log("创建 Release: " + tagName);
    var resp = http("POST", apiUrl + "/repos/" + repo + "/releases", {
        headers: {
            "Authorization": "token " + token,
            "Accept": "application/vnd.github.v3+json"
        },
        body: JSON.stringify({
            tag_name: tagName,
            name: name,
            body: body,
            draft: ctx.config.draft === "true",
            prerelease: ctx.config.prerelease === "true"
        })
    });

    if (resp.error) {
        ctx.fail("创建 release 失败: " + resp.error);
        return;
    }
    if (resp.status >= 400) {
        ctx.fail("API 错误 " + resp.status + ": " + resp.body);
        return;
    }

    var data = JSON.parse(resp.body);
    if (data.html_url) {
        ctx.output("release_url", data.html_url);
        ctx.output("release_id", data.id);
        ctx.log("Release 创建成功: " + data.html_url);
    }
});
