// Docker plugin — build, tag, push images
// Config keys: image, tag, dockerfile, registry, username, password

// docker-build: build and optionally tag an image
registerStep("docker-build", function(ctx) {
    var image = ctx.config.image || "";
    var tag = ctx.config.tag || "latest";
    var dockerfile = ctx.config.dockerfile || "Dockerfile";
    var context_ = ctx.config.context || ".";

    if (!image) {
        ctx.fail("image is required");
        return;
    }

    var fullImage = image + ":" + tag;
    ctx.log("Building Docker image: " + fullImage);

    var result = exec("docker build -f " + dockerfile + " -t " + fullImage + " " + context_);
    if (result.error) {
        ctx.fail("docker build failed: " + result.error);
        return;
    }
    ctx.log(result.output);
    ctx.output("image", fullImage);
    ctx.log("Docker image built: " + fullImage);
});

// docker-push: push an image to a registry
registerStep("docker-push", function(ctx) {
    var image = ctx.config.image || "";
    var tag = ctx.config.tag || "latest";
    var registry = ctx.config.registry || "";
    var username = ctx.config.username || "";
    var password = ctx.config.password || "";

    if (!image) {
        ctx.fail("image is required");
        return;
    }

    // Login if credentials provided.
    if (registry && username) {
        ctx.log("Logging into registry: " + registry);
        var loginResult = exec("echo " + password + " | docker login " + registry + " -u " + username + " --password-stdin");
        if (loginResult.error) {
            ctx.fail("docker login failed: " + loginResult.error);
            return;
        }
    }

    var targetImage = image + ":" + tag;
    if (registry) {
        targetImage = registry + "/" + targetImage;
        ctx.log("Tagging image for registry: " + targetImage);
        var tagResult = exec("docker tag " + image + ":" + tag + " " + targetImage);
        if (tagResult.error) {
            ctx.fail("docker tag failed: " + tagResult.error);
            return;
        }
    }

    ctx.log("Pushing Docker image: " + targetImage);
    var result = exec("docker push " + targetImage);
    if (result.error) {
        ctx.fail("docker push failed: " + result.error);
        return;
    }
    ctx.log(result.output);
    ctx.log("Docker image pushed: " + targetImage);
});

// docker-run: run a container (useful for integration tests)
registerStep("docker-run", function(ctx) {
    var image = ctx.config.image || "";
    var command = ctx.config.command || "";
    var env = ctx.config.env || "";
    var ports = ctx.config.ports || "";
    var remove = ctx.config.remove === "true" ? "--rm" : "";

    if (!image) {
        ctx.fail("image is required");
        return;
    }

    var cmd = "docker run " + remove;
    if (ports) cmd += " -p " + ports;
    if (env) cmd += " -e " + env;
    cmd += " " + image;
    if (command) cmd += " " + command;

    ctx.log("Running Docker container: " + cmd);
    var result = exec(cmd);
    if (result.error) {
        ctx.fail("docker run failed: " + result.error);
        return;
    }
    ctx.log(result.output);
});
