# Go Binary Plugins

Buildworld does not require a plugin market. A public GitHub URL is a plugin
source when it contains `plugin-buildworld.json`. The server clones a shallow
copy, validates the manifest, and either builds the declared Go package or
downloads the matching prebuilt release binary before launching only its
declared step types.

These plugins are trusted native programs and are not sandboxed. They run with
the BuildWorld service account's operating-system permissions. A SHA-256 check
proves artifact integrity, not safety; review and trust the source/release
before installing and keep the service account least-privileged.

The manifest uses `buildworld.plugin/v1`. It defines name, version, binary
entrypoint, package, and capabilities. A capability may be a custom pipeline
step or a Jenkins-style lifecycle hook. Installation is idempotent: sharing a
GitHub URL will reuse an already installed matching plugin name and version.
After a source build, Buildworld writes the binary SHA-256 into the installed
manifest and verifies it at every load. A prebuilt release is platform-specific
and must declare its own checksum. Buildworld rejects a release list without a
matching `goos`/`goarch` asset, a release without SHA-256, or a binary above
256 MiB.

~~~~json
{
  "api_version": "buildworld.plugin/v1",
  "name": "acme-deploy",
  "version": "1.2.0",
  "entrypoint": "bin/acme-deploy",
  "steps": ["acme:deploy"],
  "hooks": ["build.before", "build.success", "build.failure"],
  "ui": [
    {
      "location": "build.action",
      "label": "Open deployment",
      "url": "https://deployments.example.test/builds/{buildId}",
      "open_in_new_tab": true
    }
  ],
  "releases": [
    {
      "goos": "windows",
      "goarch": "amd64",
      "url": "https://github.com/acme/buildworld-plugin-deploy/releases/download/v1.2.0/acme-deploy-windows-amd64.exe",
      "checksum_sha256": "<sha256>"
    }
  ]
}
~~~~

At least one `steps` or `hooks` entry is required. Supported hooks are:

- `build.before`: build wrapper/setup before the first stage; hook failure
  fails the build.
- `build.always`: publisher/notifier invoked for every completed outcome.
- `build.success`: success-only publisher.
- `build.failure`: failure-only publisher.
- `build.cleanup`: final cleanup after all other post-build actions.

Multiple enabled plugins may subscribe to the same hook. BuildWorld invokes
them deterministically by plugin name. Hook responses use the same
`logs`/`env`/`outputs` data-only response as custom steps. The request operation
is `hook`, includes the hook name, project/build identity, outcome, workspace,
branch, commit, and a read-only environment snapshot. BuildWorld keeps log
masking active for hook output.

These lifecycle hooks cover the safe native equivalents of common Jenkins
build wrappers, publishers, notifiers, and cleanup plugins. Plugins still
cannot inject server-side JavaScript, arbitrary UI code, routes, credential
pages, or unreviewed controller code.

## Declarative UI actions

Plugins may add host-rendered action links to `project.action` and
`build.action`. URLs may be relative BuildWorld paths or HTTPS links and can
use `{projectId}`, `{buildId}`, and `{buildNumber}` placeholders. BuildWorld
renders one fixed extension icon and controls navigation behavior; the plugin
cannot provide markup, scripts, styles, or executable browser code.

JavaScript and TypeScript plugin runtimes are not supported. TypeScript
pipeline files are configuration parsed by the pipeline parser and are not a
plugin execution mechanism.
