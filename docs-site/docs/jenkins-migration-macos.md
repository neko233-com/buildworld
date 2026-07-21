---
sidebar_position: 3
---

# Jenkins Migration On macOS

Keep Jenkins running during migration. BuildWorld listens on `8700`; Jenkins
normally uses `8080`, so both services can run on the same Mac Mini.

## Install BuildWorld

Run as the macOS account that owns the build tools and signing keychain:

```bash
gh auth login
gh api -H "Accept: application/vnd.github.raw+json" repos/neko233-com/buildworld233/contents/scripts/install.sh | sh
buildworld start
buildworld status
```

Open `http://127.0.0.1:8700`. The installer selects `darwin/arm64` on Apple
Silicon and `darwin/amd64` on Intel Macs, installs all three binaries plus the
web bundle, verifies the release checksum, and adds `buildworld` to zsh login
PATH. It also enables a per-user LaunchAgent unless
`BUILDWORLD_NO_AUTOSTART=1` is set.

## macOS privacy access

Jenkins and BuildWorld have separate macOS privacy identities even when both
LaunchAgents run as the same user. A Desktop Folder or Full Disk Access grant
for Jenkins' Java process is not inherited by `buildworld-server`. Pipelines
that execute files below `~/Desktop`, `~/Documents`, or `~/Downloads` can
therefore fail with `Operation not permitted`; in some launch or working-
directory cases they can appear silent while the child process starts.

Prefer referenced workspaces, files, scripts, and executables outside protected
folders, for example `~/Developer` or
`~/.local/share/buildworld/workspaces`. If BuildWorld appears under **System
Settings → Privacy & Security → Files & Folders**, enable only the required
folder there. If existing Jenkins paths must remain unchanged, no narrower
control is available, and the current macOS version permits adding the
executable, add the actual installed server binary to **Full Disk Access** and
restart BuildWorld. Full Disk Access can read all protected user data, so grant
it only to the trusted binary you installed. Its default path is:

```text
~/.local/lib/buildworld/buildworld-server
```

```bash
buildworld restart
```

If the executable cannot be added or the approval is ineffective, move every
referenced workspace, file, script, or executable out of the protected folder.
A custom `BUILDWORLD_INSTALL_DIR` changes the server path shown above.

For a legacy Jenkins step that launches `feishu-robot` from Desktop, Documents,
or Downloads, do not execute that protected binary path from BuildWorld. Even
with a temporary working directory, macOS can block the dynamic loader while it
opens the executable. Install a trusted copy in BuildWorld's private helper
directory from an interactive shell that already has access:

```sh
install -d -m 700 "$HOME/Library/Application Support/buildworld/helpers"
install -m 700 "/path/to/trusted/feishu-robot" \
  "$HOME/Library/Application Support/buildworld/helpers/feishu-robot"
```

The importer then generates this guarded invocation and preserves all original
arguments:

```sh
if [ ! -x "$HOME/Library/Application Support/buildworld/helpers/feishu-robot" ]; then
  echo "Install the trusted helper with mode 0700 before enabling this pipeline" >&2
  exit 1
fi
cd "${TMPDIR:-/tmp}"
"$HOME/Library/Application Support/buildworld/helpers/feishu-robot" game-config-refresh
```

BuildWorld never copies or updates this executable automatically. The importer
emits an explicit warning requiring the private copy and its `0700` mode before
cutover. Existing imported projects must be re-imported or edited to use this
form; a protected absolute executable path is not a valid workaround.

This approval is intentionally manual. The installer and BuildWorld never edit
the macOS TCC database or bypass a privacy denial.

## Import And Compare

Create a new BuildWorld project for each Jenkins job. In the project editor,
select **Import Jenkinsfile**, paste the job's Jenkinsfile, review warnings,
then save the generated restricted TypeScript pipeline. Do not edit or delete
the Jenkins job.

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
log show --last 15m --style compact --info \
  --predicate 'subsystem == "com.apple.TCC" OR eventMessage CONTAINS[c] "System Policy"'
```

An entry containing `deny ... file-read-data` for the pipeline executable, with
`buildworld-server` shown as the responsible process, identifies a macOS
privacy denial for that access. Investigate unrelated build failures
separately.
