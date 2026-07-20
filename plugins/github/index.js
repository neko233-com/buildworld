// GitHub plugin — commit status, release, PR comment
// Config keys: token, repo (owner/name), api_url (default: https://api.github.com)

function ghApi(ctx, method, path, body) {
    var token = ctx.config.token || "";
    var repo = ctx.config.repo || "";
    var apiUrl = ctx.config.api_url || "https://api.github.com";
    var url = apiUrl + "/repos/" + repo + path;

    var headers = {
        "Authorization": "token " + token,
        "Accept": "application/vnd.github.v3+json"
    };

    var resp = http(method, url, {
        headers: headers,
        body: body ? JSON.stringify(body) : ""
    });

    if (resp.error) {
        ctx.fail("GitHub API error: " + resp.error);
        return null;
    }
    if (resp.status >= 400) {
        ctx.fail("GitHub API " + resp.status + ": " + resp.body);
        return null;
    }
    return resp.body ? JSON.parse(resp.body) : null;
}

// github-status: set commit status (pending/success/failure/error)
registerStep("github-status", function(ctx) {
    var state = ctx.config.state || "success";
    var targetUrl = ctx.config.target_url || "";
    var description = ctx.config.description || "Build passed";
    var context = ctx.config.context || "buildworld";

    ctx.log("Setting GitHub commit status: " + state + " for " + ctx.commit);

    ghApi(ctx, "POST", "/statuses/" + ctx.commit, {
        state: state,
        target_url: targetUrl,
        description: description,
        context: context
    });

    ctx.log("Commit status set successfully");
});

// github-release: create a release
registerStep("github-release", function(ctx) {
    var tagName = ctx.config.tag || ("v" + ctx.commit.substring(0, 7));
    var name = ctx.config.name || tagName;
    var body = ctx.config.body || "Release " + tagName;

    ctx.log("Creating GitHub release: " + tagName);

    var result = ghApi(ctx, "POST", "/releases", {
        tag_name: tagName,
        name: name,
        body: body,
        draft: ctx.config.draft === "true",
        prerelease: ctx.config.prerelease === "true"
    });

    if (result && result.html_url) {
        ctx.output("release_url", result.html_url);
        ctx.log("Release created: " + result.html_url);
    }
});

// github-comment: comment on a PR
registerStep("github-comment", function(ctx) {
    var prNumber = ctx.config.pr_number || "";
    var message = ctx.config.message || "Build completed";

    if (!prNumber) {
        ctx.fail("pr_number is required");
        return;
    }

    ctx.log("Commenting on PR #" + prNumber);

    ghApi(ctx, "POST", "/issues/" + prNumber + "/comments", {
        body: message
    });

    ctx.log("PR comment posted");
});
