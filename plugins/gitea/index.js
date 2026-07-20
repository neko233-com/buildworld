// Gitea plugin — commit status, release (Gitea API is GitHub-compatible)
// Config keys: token, repo (owner/name), api_url (e.g. https://gitea.example.com/api/v1)

function giteaApi(ctx, method, path, body) {
    var token = ctx.config.token || "";
    var repo = ctx.config.repo || "";
    var apiUrl = ctx.config.api_url || "http://localhost:3000/api/v1";
    var url = apiUrl + "/repos/" + repo + path;

    var headers = {
        "Authorization": "token " + token,
        "Accept": "application/json"
    };

    var resp = http(method, url, {
        headers: headers,
        body: body ? JSON.stringify(body) : ""
    });

    if (resp.error) {
        ctx.fail("Gitea API error: " + resp.error);
        return null;
    }
    if (resp.status >= 400) {
        ctx.fail("Gitea API " + resp.status + ": " + resp.body);
        return null;
    }
    return resp.body ? JSON.parse(resp.body) : null;
}

// gitea-status: set commit status
registerStep("gitea-status", function(ctx) {
    var state = ctx.config.state || "success";
    var targetUrl = ctx.config.target_url || "";
    var description = ctx.config.description || "Build passed";
    var context = ctx.config.context || "buildworld";

    ctx.log("Setting Gitea commit status: " + state + " for " + ctx.commit);

    giteaApi(ctx, "POST", "/statuses/" + ctx.commit, {
        state: state,
        target_url: targetUrl,
        description: description,
        context: context
    });

    ctx.log("Commit status set successfully");
});

// gitea-release: create a release
registerStep("gitea-release", function(ctx) {
    var tagName = ctx.config.tag || ("v" + ctx.commit.substring(0, 7));
    var name = ctx.config.name || tagName;
    var body = ctx.config.body || "Release " + tagName;

    ctx.log("Creating Gitea release: " + tagName);

    var result = giteaApi(ctx, "POST", "/releases", {
        tag_name: tagName,
        name: name,
        body: body,
        draft: ctx.config.draft === "true",
        prerelease: ctx.config.prerelease === "true"
    });

    if (result && result.url) {
        ctx.output("release_url", result.url);
        ctx.log("Release created: " + result.url);
    }
});
