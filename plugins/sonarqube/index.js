// SonarQube 插件 — 代码质量：扫描、质量门、覆盖率查询
// 配置项: host, token, project_key, project_name, sources, scanner_opts, branch

// sonar-scanner: 执行代码扫描
registerStep("sonar-scanner", function(ctx) {
    var host = ctx.config.host || "";
    var token = ctx.config.token || "";
    var projectKey = ctx.config.project_key || "";
    var sources = ctx.config.sources || ".";

    if (!host || !token) {
        ctx.fail("host 和 token 配置项必填");
        return;
    }

    var cmd = "sonar-scanner";
    cmd += ' -Dsonar.host.url=' + host;
    cmd += ' -Dsonar.login=' + token;
    if (projectKey) cmd += ' -Dsonar.projectKey=' + projectKey;
    if (ctx.config.project_name) cmd += ' -Dsonar.projectName="' + ctx.config.project_name + '"';
    cmd += ' -Dsonar.sources=' + sources;
    if (ctx.branch) cmd += ' -Dsonar.branch.name=' + ctx.branch;
    if (ctx.config.exclusions) cmd += ' -Dsonar.exclusions=' + ctx.config.exclusions;

    ctx.log("执行 SonarQube 扫描: " + projectKey);
    var result = exec(cmd);
    if (result.error) {
        ctx.fail("sonar-scanner 失败: " + result.error + "\n" + result.output);
        return;
    }
    ctx.log(result.output);
    ctx.output("project_key", projectKey);
    ctx.log("扫描完成");
});

// sonar-quality-gate: 查询质量门状态
registerStep("sonar-quality-gate", function(ctx) {
    var host = ctx.config.host || "";
    var token = ctx.config.token || "";
    var projectKey = ctx.config.project_key || "";

    if (!host || !projectKey) {
        ctx.fail("host 和 project_key 配置项必填");
        return;
    }

    var url = host + "/api/qualitygates/project_status?projectKey=" + encodeURIComponent(projectKey);
    if (ctx.branch) url += "&branch=" + encodeURIComponent(ctx.branch);

    ctx.log("查询质量门状态: " + projectKey);
    var resp = http("GET", url, {
        headers: { "Authorization": "Bearer " + token }
    });

    if (resp.error) {
        ctx.fail("质量门查询失败: " + resp.error);
        return;
    }
    if (resp.status >= 400) {
        ctx.fail("SonarQube API 错误: " + resp.status + " " + resp.body);
        return;
    }

    var data = JSON.parse(resp.body);
    var status = data.projectStatus ? data.projectStatus.status : "UNKNOWN";
    ctx.output("status", status);
    ctx.log("质量门状态: " + status);

    if (status !== "OK") {
        // 输出失败条件
        if (data.projectStatus && data.projectStatus.conditions) {
            data.projectStatus.conditions.forEach(function(c) {
                if (c.status !== "OK") {
                    ctx.log("未通过条件: " + c.metricKey + " = " + c.actualValue + " (阈值: " + c.comparator + c.errorThreshold + ")");
                }
            });
        }
        ctx.fail("质量门未通过: " + status);
    }
});

// sonar-coverage: 查询覆盖率
registerStep("sonar-coverage", function(ctx) {
    var host = ctx.config.host || "";
    var token = ctx.config.token || "";
    var projectKey = ctx.config.project_key || "";

    if (!host || !projectKey) {
        ctx.fail("host 和 project_key 配置项必填");
        return;
    }

    var metrics = ctx.config.metrics || "coverage,lines_to_cover,uncovered_lines,bugs,vulnerabilities,code_smells";
    var url = host + "/api/measures/component?component=" + encodeURIComponent(projectKey) +
        "&metricKeys=" + metrics;

    ctx.log("查询代码度量: " + projectKey);
    var resp = http("GET", url, {
        headers: { "Authorization": "Bearer " + token }
    });

    if (resp.error) {
        ctx.fail("度量查询失败: " + resp.error);
        return;
    }
    if (resp.status >= 400) {
        ctx.fail("SonarQube API 错误: " + resp.status + " " + resp.body);
        return;
    }

    var data = JSON.parse(resp.body);
    if (data.component && data.component.measures) {
        data.component.measures.forEach(function(m) {
            ctx.output(m.metric, m.value);
            ctx.log(m.metric + ": " + m.value);
        });
    }
    ctx.log("度量查询完成");
});
