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

## Date and time presentation

- Human-visible calendar dates use exactly `yyyy-MM-dd`. Human-visible
  timestamps use exactly `yyyy-MM-dd HH:mm:ss,SSS`. Timezone-aware source
  timestamps are rendered in the user's local timezone; offset-less source
  timestamps retain their supplied wall-clock value.
- Web UI code must use `web/src/lib/dateTime.ts` (`formatDate` or
  `formatDateTime`). Do not render user-visible dates with locale-dependent
  `toLocaleString`, `toLocaleDateString`, `toLocaleTimeString`, ad hoc slicing,
  or abbreviated month/day formats.
- Keep machine-facing API and storage values in their documented ISO 8601 or
  RFC 3339 form. Native date input values, semantic `dateTime` attributes,
  comparisons, and original build-log timestamps are not presentation strings
  and must not be rewritten.

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
