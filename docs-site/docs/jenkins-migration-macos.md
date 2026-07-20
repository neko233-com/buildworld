---
sidebar_position: 3
---

# Jenkins Migration On macOS

Keep Jenkins running during migration. BuildWorld listens on `8700`; Jenkins
normally uses `8080`, so both services can run on the same Mac Mini.

## Install BuildWorld

Run as the macOS account that owns the build tools and signing keychain:

```bash
curl -fsSL https://raw.githubusercontent.com/neko233-com/buildworld233/main/scripts/install.sh | sh
buildworld start
buildworld status
```

Open `http://127.0.0.1:8700`. The installer selects `darwin/arm64` on Apple
Silicon and `darwin/amd64` on Intel Macs, installs all three binaries plus the
web bundle, verifies the release checksum, and adds `buildworld` to zsh login
PATH. It also enables a per-user LaunchAgent unless
`BUILDWORLD_NO_AUTOSTART=1` is set.

## Import And Compare

Create a new BuildWorld project for each Jenkins job. In the project editor,
select **Import Jenkinsfile**, paste the job's Jenkinsfile, review warnings,
then save. Do not edit or delete the Jenkins job.

Run the Jenkins job and its BuildWorld copy against the same commit. Compare:

- selected agent labels, tool paths, and macOS signing/keychain access;
- environment values, especially absolute paths and credentials;
- stage logs, artifacts, test reports, notifications, and deployed output.

Imported pipelines retain shell commands, stages, plain environment values,
agent labels, `dir(...)`, supported `if (fileExists(...))` blocks, and Jenkins
`$WORKSPACE` behavior. BuildWorld maps it to an isolated per-build workspace.
The conventional `tail -f` plus PID heartbeat deployment monitor is converted
to BuildWorld's native service observer, so the build stays running and streams
logs without preserving an unbounded shell loop.
Warnings mean manual configuration is required; common examples are Jenkins
credentials, `when`, shared-library calls, and `post` actions.

## Cut Over

Keep both jobs scheduled only after preventing duplicate deployment. During
comparison, make BuildWorld manual-only or point it at a staging target. After
several matching runs, disable the Jenkins trigger first, enable the matching
BuildWorld trigger, and retain Jenkins configuration for rollback.

To inspect macOS background-service failures:

```bash
buildworld status
tail -n 200 "$HOME/Library/Application Support/buildworld/server.log"
```
