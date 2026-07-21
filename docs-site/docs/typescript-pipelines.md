---
sidebar_position: 5
---

# TypeScript Pipelines

Use the typed, declarative API when a pipeline needs the readability of code
without granting configuration arbitrary process or network access. The API
package lives at the repository path `sdk/pipeline`; its module name is
`@buildworld/pipeline`.

```ts
import { definePipeline, shell, stage, watchService } from '@buildworld/pipeline'

export default definePipeline({
  name: 'game-server',
  allowLongRunning: true,
  stages: [
    stage('Start', shell('Launch', './start-server.sh')),
    stage('Observe service', watchService('Follow server log', {
      targetDir: '/srv/game-server',
      pidFile: 'game-server.pid.txt',
      logFile: 'game-server.log',
      port: 8700,
      heartbeatSeconds: 30,
    })),
  ],
})
```

`watchService` is the native equivalent of the Jenkins `tail -f` plus PID
heartbeat loop:

- the build remains `running` and the log viewer receives appended lines in
  real time;
- BuildWorld follows logs independently from PID polling, checks the PID, and
  emits a heartbeat every 30 seconds;
- if the service exits, the build fails and includes the final 50 log lines;
- stopping the build stops observation only; it never kills the deployed
  service.

The TypeScript source may contain comments and normal object literals. It may
only import `@buildworld/pipeline`; `eval`, loops, functions, Node APIs, file
access, networking, and process APIs are rejected. This keeps the configured
pipeline auditable while shell commands remain explicit `shell(...)` steps.

For editor completion, install the included package into the pipeline project
or point TypeScript path mapping at `sdk/pipeline`. The declaration file is the
single API contract for both people and AI agents.
