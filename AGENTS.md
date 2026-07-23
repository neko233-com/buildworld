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

## Jenkins-inspired UI direction

- Preserve Jenkins-compatible workflows and user-visible capabilities, but do
  not pixel-copy Jenkins. New local UI work should use BuildWorld-specific
  layout, hierarchy, spacing, and operational context.
- The dashboard left rail is the deliberate compatibility exception: keep
  queue and recent-build panels aligned with Jenkins' classic pane structure,
  compact row density, collapse behavior, and status-dot semantics. Use plain
  circles instead of weather icons: green success, yellow unstable/test
  failure, red failure, gray cancelled, and blue animated while running.
- Start the dashboard project table with the project's persistent unique `ID`,
  followed by the status (`S`) column. Do not restore the Jenkins weather or
  aggregate health (`W`) column.
- Prefer named imports from `react-icons` for new general-purpose interface
  icons. Do not introduce copied Jenkins image assets when the same meaning can
  be represented by the shared icon library.
- Treat the Jenkins-style dashboard as a desktop-only operations surface. Do
  not spend implementation or verification effort on mobile breakpoints,
  mobile navigation, touch-specific layout, or mobile browser QA unless the
  user explicitly requests mobile support in the current task.
- Use one stable default dashboard table density. Do not expose S/M/L or other
  user-selectable density controls, and do not persist dashboard density in
  browser storage.
- Label the Jenkins-style project job type and its user-facing configuration
  surfaces exactly `Jenkinsfile Pipeline`. New-item creation selects this type
  by default and creates an SCM/VCS-sourced definition with path `Jenkinsfile`;
  do not generate an inline pipeline as the default. Folder remains an explicit
  alternative. Prefer a strong default over asking users to choose routine
  presentation or job-type settings.
- Long-running `service_watch` steps default to a 15-minute heartbeat. Jenkins
  migration normalizes shorter heartbeat loops to at least five minutes while
  keeping realtime appended log forwarding and process-exit checks.
- Keep an authenticated, auto-refreshing disk-usage progress bar at the top of
  the dashboard for the volume containing the `builtin` executor build-temp
  cache. Disk monitoring must not add or depend on distributed Worker UI.
- Disk usage is informational and must not block project data. Show explicit
  loading, retryable failure, warning (75%+), and critical (90%+) states.

## Browser automation

- Do not use Computer Use for browser interaction in this repository.
- Use the bundled in-app Browser plugin for browser navigation, inspection,
  authentication-preserving interaction, and rendered-flow verification.

## Plugin compatibility direction

- Prefer safe, data-only native plugin capabilities that cover Jenkins build
  steps, build wrappers, publishers, notifiers, and cleanup actions.
- Lifecycle extensions use the declared binary hooks `build.before`,
  `build.always`, `build.success`, `build.failure`, and `build.cleanup`.
- UI extensions are declarative host-rendered links at `project.action` or
  `build.action`; accept only relative BuildWorld paths or HTTPS URLs.
- Do not restore JavaScript execution, UI injection, arbitrary routes, or
  controller mutation as a shortcut for Jenkins plugin compatibility.
