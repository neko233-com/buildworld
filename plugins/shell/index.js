// Shell plugin — enhanced shell execution with output capture
// Config keys: command, capture_output, fail_on_error

// shell-cmd: run a shell command and capture output as step output
registerStep("shell-cmd", function(ctx) {
    var command = ctx.config.command || "";
    if (!command) {
        ctx.fail("command is required");
        return;
    }

    ctx.log("Executing: " + command);
    var result = exec(command);

    if (result.error) {
        ctx.log("Command output: " + result.output);
        if (ctx.config.fail_on_error !== "false") {
            ctx.fail("command failed: " + result.error);
            return;
        }
        ctx.log("Command failed but fail_on_error=false");
    } else {
        ctx.log(result.output);
    }

    // Capture first line of output as a step output variable.
    if (ctx.config.capture_output === "true" && result.output) {
        var lines = result.output.trim().split("\n");
        if (lines.length > 0) {
            ctx.output("result", lines[0].trim());
        }
    }
});

// shell-condition: run a command, pass only if exit code is 0
registerStep("shell-condition", function(ctx) {
    var command = ctx.config.command || "";
    if (!command) {
        ctx.fail("command is required");
        return;
    }

    ctx.log("Checking condition: " + command);
    var result = exec(command);

    if (result.error) {
        ctx.log("Condition not met (exit code != 0)");
        ctx.fail("condition not met");
        return;
    }

    ctx.log("Condition met");
});
