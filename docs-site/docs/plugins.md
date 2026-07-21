---
sidebar_position: 5
---

# Go binary plugins

BuildWorld v1 supports only out-of-process Go binary plugins. It does not load
JavaScript or TypeScript plugin source, expose an `eval`/`exec` bridge, or allow
plugins to inject browser UI. TypeScript remains available for pipeline files;
it is a separate, statically parsed configuration format.

## Install

An administrator installs a plugin from its public HTTPS GitHub repository on
the **Plugins** page. The repository must contain `plugin-buildworld.json` at
its root. BuildWorld validates the manifest, then downloads a checksummed
release for the current platform or locally builds the declared Go package.

Go plugins are trusted native programs, not sandboxed extensions. BuildWorld
runs the binary with the service account's operating-system permissions. A
SHA-256 checksum proves that bytes match the reviewed artifact; it does not
prove the program is safe. Review and trust the repository and release before
confirming installation, and run BuildWorld with the least privileges needed.

```json
{
  "api_version": "buildworld.plugin/v1",
  "name": "acme-deploy",
  "version": "1.2.0",
  "description": "Deploy an Acme service",
  "author": "Acme",
  "entrypoint": "bin/acme-deploy",
  "package": "./cmd/acme-deploy",
  "steps": ["acme:deploy"]
}
```

Prebuilt entries add a `releases` array. Every release declares `goos`,
`goarch`, an HTTPS download URL, and `checksum_sha256`. BuildWorld also records
and verifies the checksum of locally built binaries.

## Execute a custom step

```typescript
pipeline({
  name: "deploy",
  stages: [{
    name: "Deploy",
    steps: [{
      name: "Deploy service",
      type: "acme:deploy",
      config: { environment: "staging" }
    }]
  }]
})
```

The plugin receives a JSON step context on standard input and returns logs,
environment values, outputs, or an error as JSON on standard output. It runs in
its own process and follows build cancellation.

## Lifecycle

- Disable or enable a plugin without deleting its installation.
- Reload after replacing an installed manifest or binary.
- Remove deletes the installed binary directory and its database record.
- Remote workers resolve the same approved GitHub source and verify the exact
  manifest digest before execution.

See [TypeScript pipelines](./typescript-pipelines.md) for pipeline authoring.
