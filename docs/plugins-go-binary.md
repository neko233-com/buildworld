# Go Binary Plugins

Buildworld does not require a plugin market. A public GitHub URL is a plugin
source when it contains `plugin-buildworld.json`. The server clones a shallow
copy, validates the manifest, and either builds the declared Go package or
downloads the matching prebuilt release binary before launching only its
declared step types.

The manifest uses `buildworld.plugin/v1`. It defines name, version, binary
entrypoint, package, and capabilities. Installation is idempotent: sharing a
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

Official independent templates in this workspace:

- `buildworld-plugin-lib-go/` - stable Go contract library.
- `buildworld-plugin-echo/` - minimal executable step plugin.
- `buildworld-plugin-notify-template/` - provider notification template.
