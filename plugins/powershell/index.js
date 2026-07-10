// PowerShell plugin — enhanced PS execution with module support and output capture
// Config keys: command, script, capture_output, fail_on_error, modules, working_directory

// ps-run: run a PowerShell command (cross-platform)
registerStep("ps-run", function(ctx) {
    var command = ctx.config.command || "";
    if (!command) {
        ctx.fail("command is required");
        return;
    }

    var modules = ctx.config.modules || "";
    var failOnError = ctx.config.fail_on_error !== "false";

    var prefix = "$ErrorActionPreference = 'Stop'; ";
    if (modules) {
        modules.split(",").forEach(function(m) {
            prefix += "Import-Module " + m.trim() + " -ErrorAction Stop; ";
        });
    }

    // Escape single quotes for PowerShell -Command
    var psCommand = prefix + command;

    ctx.log("PowerShell: " + command);
    var result;
    // Detect platform: use pwsh on non-Windows, powershell on Windows
    if (navigator && navigator.platform && navigator.platform.indexOf("Win") === -1) {
        result = exec("pwsh -NoProfile -NonInteractive -Command \"" + psCommand.replace(/"/g, '\\"') + "\"");
    } else {
        result = exec("powershell -NoProfile -NonInteractive -ExecutionPolicy Bypass -Command \"" + psCommand.replace(/"/g, '\\"') + "\"");
    }

    if (result.error && failOnError) {
        ctx.fail("PowerShell failed: " + result.error + "\n" + result.output);
        return;
    }
    ctx.log(result.output);

    if (ctx.config.capture_output === "true" && result.output) {
        ctx.output("result", result.output.trim());
    }
});

// ps-script: run a PowerShell script file (.ps1)
registerStep("ps-script", function(ctx) {
    var script = ctx.config.script || "";
    if (!script) {
        ctx.fail("script is required (path to .ps1 file)");
        return;
    }

    ctx.log("PowerShell script: " + script);
    var result = exec("powershell -NoProfile -NonInteractive -ExecutionPolicy Bypass -File " + script);
    if (result.error) {
        ctx.fail("PowerShell script failed: " + result.error + "\n" + result.output);
        return;
    }
    ctx.log(result.output);
});

// ps-module: install/import a PowerShell module
registerStep("ps-module", function(ctx) {
    var name = ctx.config.name || "";
    if (!name) {
        ctx.fail("name is required");
        return;
    }

    var scope = ctx.config.scope || "CurrentUser";
    var cmd = "if (-not (Get-Module -ListAvailable -Name " + name + ")) { Install-Module -Name " + name + " -Scope " + scope + " -Force -AllowClobber }; Import-Module " + name;

    ctx.log("Installing/importing PS module: " + name);
    var result = exec("powershell -NoProfile -NonInteractive -ExecutionPolicy Bypass -Command \"" + cmd.replace(/"/g, '\\"') + "\"");
    if (result.error) {
        ctx.fail("PS module install failed: " + result.error);
        return;
    }
    ctx.log("Module " + name + " ready");
});
