import assert from 'node:assert/strict'
import test from 'node:test'

import {
  definePipeline,
  parameter,
  shell,
  stage,
  trigger,
  watchService,
} from './index.js'

test('helpers normalize camelCase exactly like the server parser', () => {
  const source = {
    stages: [],
    agentRequirements: ['macos'],
    retentionCompleted: 12,
    timeoutSec: 60,
    allowLongRunning: false,
    disableConcurrent: true,
    abortPrevious: true,
    on: [{ type: 'manual', config: {} }],
    approval: {
      strategy: 'single',
      requiredRoles: ['admin'],
      allowRequester: false,
    },
  }

  const pipeline = definePipeline(source)
  assert.deepEqual(pipeline.agent_requirements, ['macos'])
  assert.equal(pipeline.retention_completed, 12)
  assert.equal(pipeline.timeout_sec, 60)
  assert.equal(pipeline.allow_long_running, false)
  assert.equal(pipeline.disable_concurrent, true)
  assert.equal(pipeline.abort_previous, true)
  assert.equal(pipeline.triggers, source.on)
  assert.deepEqual(pipeline.approval.required_roles, ['admin'])
  assert.equal(pipeline.approval.allow_requester, false)
  assert.equal('agent_requirements' in source, false)
  assert.equal('required_roles' in source.approval, false)
})

test('step, stage, parameter, and service watch aliases are canonicalized', () => {
  const run = shell('run', 'npm test', {
    if: 'success()',
    platformAdditions: { macos: './sign.sh' },
  })
  const verify = stage('Verify', run, {
    dependsOn: ['Checkout'],
    workingDirectory: '/srv/app',
    timeoutSec: 60,
  })
  const secret = parameter('token', 'password', { isSecret: true })
  const watch = watchService('logs', {
    targetDir: '/srv/app',
    pidFile: 'server.pid',
    logFile: 'server.log',
    heartbeatSeconds: 30,
    pollSeconds: 2,
    initialLines: 20,
    port: 8700,
  })

  assert.deepEqual(run.platform_additions, { macos: './sign.sh' })
  assert.equal(run.if, 'success()')
  assert.deepEqual(verify.depends_on, ['Checkout'])
  assert.equal(verify.working_directory, '/srv/app')
  assert.equal(verify.timeout_sec, 60)
  assert.deepEqual(verify.steps, [run])
  assert.equal(secret.is_secret, true)
  assert.equal(watch.config.target_dir, '/srv/app')
  assert.equal(watch.config.pid_file, 'server.pid')
  assert.equal(watch.config.log_file, 'server.log')
  assert.equal(watch.config.heartbeat_seconds, '30')
  assert.equal(watch.config.poll_seconds, '2')
  assert.equal(watch.config.initial_lines, '20')
  assert.equal(watch.config.port, '8700')
})

test('canonical snake_case options are preserved and camel aliases never override them', () => {
  const verify = stage('Verify', shell('run', 'npm test'), {
    workingDirectory: '/camel',
    working_directory: '/canonical',
    timeoutSec: 30,
    timeout_sec: 90,
  })
  const watch = watchService('logs', {
    target_dir: '/srv/app',
    pid_file: 'server.pid',
    log_file: 'server.log',
    heartbeat_seconds: 15,
    poll_seconds: 1,
    initial_lines: 100,
  })

  assert.equal(verify.working_directory, '/canonical')
  assert.equal(verify.timeout_sec, 90)
  assert.equal(watch.config.target_dir, '/srv/app')
  assert.equal(watch.config.pid_file, 'server.pid')
  assert.equal(watch.config.log_file, 'server.log')
  assert.equal(watch.config.heartbeat_seconds, '15')
  assert.equal(watch.config.poll_seconds, '1')
  assert.equal(watch.config.initial_lines, '100')
  assert.deepEqual(trigger('manual', null).config, {})
})
