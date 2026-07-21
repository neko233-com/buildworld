import {
  definePipeline,
  notify,
  parameter,
  shell,
  stage,
  trigger,
  watchService,
  type Pipeline,
  type ServiceWatchOptions,
  type Stage,
  type Step,
} from './index.js'

const conditionalStep: Step = shell('test', 'npm test', {
  if: 'success()',
  shell: 'bash',
  runtime: 'node',
  config: { NODE_OPTIONS: '--enable-source-maps' },
  platformAdditions: { darwin: './sign.sh' },
})

const watchOptions: ServiceWatchOptions = {
  target_dir: '/srv/typed',
  pid_file: 'server.pid',
  log_file: 'server.log',
  heartbeat_seconds: 30,
  poll_seconds: 2,
  initial_lines: 50,
  port: 8700,
}

// @ts-expect-error A service observer cannot start without both PID and log files.
const missingPidFile: ServiceWatchOptions = { logFile: 'server.log' }
void missingPidFile

const typedStage: Stage = stage('Verify', [
  conditionalStep,
  watchService('follow logs', watchOptions),
], {
  id: 'verify',
  if: 'success()',
  environment: { NODE_ENV: 'test' },
  workingDirectory: '/srv/typed',
  timeoutSec: 120,
  parallel: false,
  dependsOn: ['Checkout'],
  branches: ['main'],
})

const pipeline = definePipeline({
  name: 'typed',
  approval: {
    strategy: 'single',
    requiredRoles: ['admin'],
    allowRequester: false,
  },
  parameters: [
    parameter('target', 'choice', {
      choices: ['staging', 'production'],
      default: 'staging',
    }),
    parameter('optional-note', 'text', {
      default: null,
    }),
  ],
  stages: [
    typedStage,
  ],
  on: [trigger('manual', null)],
  post: {
    failure: [notify('diagnostics', { config: { message: 'collect logs' } })],
  },
} as const satisfies Pipeline)

pipeline.stages[0]?.steps[0]?.name

definePipeline({
  name: 'migrated-nullable-fields',
  environment: null,
  parameters: null,
  approval: null,
  on: null,
  triggers: null,
  artifacts: null,
  post: null,
  agentRequirements: null,
  agent_requirements: null,
  toolchains: null,
  stages: [{
    id: 'literal-stage',
    name: 'Literal stage',
    if: 'always()',
    environment: null,
    working_directory: '/srv/literal',
    timeout_sec: 30,
    steps: [{
      name: 'Literal step',
      type: 'shell',
      command: 'echo ok',
      if: 'success()',
    }],
  }],
} as const satisfies Pipeline)
