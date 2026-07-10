// S3 插件 — 对象存储：上传、下载、删除、同步、列举
// 配置项: bucket, key, path, region, endpoint, acl, recursive, exclude, include

function s3Base(ctx) {
    var cmd = "aws s3";
    if (ctx.config.region) cmd += " --region " + ctx.config.region;
    if (ctx.config.endpoint) cmd += " --endpoint-url " + ctx.config.endpoint;
    return cmd;
}

function s3Url(bucket, key) {
    return "s3://" + bucket + "/" + (key || "");
}

// s3-upload: 上传文件到 S3
registerStep("s3-upload", function(ctx) {
    var bucket = ctx.config.bucket || "";
    var key = ctx.config.key || "";
    var path = ctx.config.path || "";
    if (!bucket || !path) {
        ctx.fail("bucket 和 path 配置项必填");
        return;
    }
    var cmd = s3Base(ctx) + " cp " + path + " " + s3Url(bucket, key);
    if (ctx.config.acl) cmd += " --acl " + ctx.config.acl;
    if (ctx.config.storage_class) cmd += " --storage-class " + ctx.config.storage_class;
    if (ctx.config.metadata) cmd += ' --metadata "' + ctx.config.metadata + '"';

    ctx.log("上传到 S3: " + cmd);
    var result = exec(cmd);
    if (result.error) {
        ctx.fail("s3 upload 失败: " + result.error + "\n" + result.output);
        return;
    }
    ctx.log(result.output);
    ctx.output("url", s3Url(bucket, key));
    ctx.log("上传成功");
});

// s3-download: 从 S3 下载文件
registerStep("s3-download", function(ctx) {
    var bucket = ctx.config.bucket || "";
    var key = ctx.config.key || "";
    var path = ctx.config.path || "";
    if (!bucket || !path) {
        ctx.fail("bucket 和 path 配置项必填");
        return;
    }
    var cmd = s3Base(ctx) + " cp " + s3Url(bucket, key) + " " + path;
    if (ctx.config.recursive === "true") cmd += " --recursive";

    ctx.log("从 S3 下载: " + cmd);
    var result = exec(cmd);
    if (result.error) {
        ctx.fail("s3 download 失败: " + result.error + "\n" + result.output);
        return;
    }
    ctx.log(result.output);
    ctx.output("path", path);
    ctx.log("下载成功");
});

// s3-delete: 删除 S3 对象
registerStep("s3-delete", function(ctx) {
    var bucket = ctx.config.bucket || "";
    var key = ctx.config.key || "";
    if (!bucket) {
        ctx.fail("bucket 配置项必填");
        return;
    }
    var cmd = s3Base(ctx) + " rm " + s3Url(bucket, key);
    if (ctx.config.recursive === "true") cmd += " --recursive";
    if (ctx.config.exclude) cmd += ' --exclude "' + ctx.config.exclude + '"';
    if (ctx.config.include) cmd += ' --include "' + ctx.config.include + '"';

    ctx.log("删除 S3 对象: " + cmd);
    var result = exec(cmd);
    if (result.error) {
        ctx.fail("s3 delete 失败: " + result.error + "\n" + result.output);
        return;
    }
    ctx.log(result.output);
    ctx.log("删除成功");
});

// s3-sync: 同步目录到 S3
registerStep("s3-sync", function(ctx) {
    var bucket = ctx.config.bucket || "";
    var path = ctx.config.path || "";
    var direction = ctx.config.direction || "up"; // up: 本地->S3, down: S3->本地
    if (!bucket || !path) {
        ctx.fail("bucket 和 path 配置项必填");
        return;
    }
    var cmd = s3Base(ctx) + " sync ";
    if (direction === "down") {
        cmd += s3Url(bucket, ctx.config.key) + " " + path;
    } else {
        cmd += path + " " + s3Url(bucket, ctx.config.key);
    }
    if (ctx.config.acl) cmd += " --acl " + ctx.config.acl;
    if (ctx.config.delete === "true") cmd += " --delete";
    if (ctx.config.exclude) cmd += ' --exclude "' + ctx.config.exclude + '"';
    if (ctx.config.include) cmd += ' --include "' + ctx.config.include + '"';

    ctx.log("同步 S3 (" + direction + "): " + cmd);
    var result = exec(cmd);
    if (result.error) {
        ctx.fail("s3 sync 失败: " + result.error + "\n" + result.output);
        return;
    }
    ctx.log(result.output);
    ctx.log("同步成功");
});

// s3-list: 列举 S3 对象
registerStep("s3-list", function(ctx) {
    var bucket = ctx.config.bucket || "";
    if (!bucket) {
        ctx.fail("bucket 配置项必填");
        return;
    }
    var cmd = s3Base(ctx) + " ls " + s3Url(bucket, ctx.config.key);
    if (ctx.config.recursive === "true") cmd += " --recursive";
    if (ctx.config.human_readable === "true") cmd += " --human-readable";
    if (ctx.config.summarize === "true") cmd += " --summarize";

    ctx.log("列举 S3 对象: " + cmd);
    var result = exec(cmd);
    if (result.error) {
        ctx.fail("s3 list 失败: " + result.error + "\n" + result.output);
        return;
    }
    ctx.log(result.output);
    ctx.output("listing", result.output);
    ctx.log("列举完成");
});
