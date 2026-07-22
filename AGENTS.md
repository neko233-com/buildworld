# BuildWorld contributor instructions

Read and follow [Rule.md](Rule.md) before modifying this repository. It is the
authoritative engineering and verification policy for all automated agents.

Release work must preserve the installer contract in Rule.md: all supported
platforms receive a locally built bundle and one-click installers keep the CLI
on the user environment PATH. GitHub Actions is not used for CI,
documentation, notification, packaging, or upload work; run those steps
locally.

Before creating or uploading any release bundle, complete all applicable test
suites and required production checks. Packaging and upload are forbidden while
verification is incomplete or failing.

## Distributed Worker scope

- BuildWorld's default and primary executor is the embedded `builtin` executor.
  Unless the user explicitly requests distributed or remote Worker work in the
  current task, do not add, expose, configure, register, start, dispatch to, or
  use distributed Workers, including for project verification.
- Jenkins-style UI keeps running and waiting jobs under **Builds in Progress**.
  Do not add a separate distributed Worker navigation item, panel, or metric.
  When executor state is needed, show `builtin` by default.
- Generated and migrated pipelines must not add `agentRequirements`,
  `agent_requirements`, or non-local `runs-on` values unless the user explicitly
  requests remote execution.
- Preserve the existing remote Worker page and hidden route, APIs, protocol,
  database migrations, SDK fields, implementation, regression coverage, and
  release binaries. Do not remove or extend them without an explicit request.
  The Worker release contract in `Rule.md` remains mandatory.
