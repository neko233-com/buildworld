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

test('definePipeline canonicalizes literal stages and steps without mutating source', () => {
  const source = {
    stages: [{
      name: 'Literal',
      steps: [{
        name: 'run',
        type: 'shell',
        command: 'npm test',
        platformAdditions: { macos: './sign.sh' },
      }],
      dependsOn: ['Checkout'],
      workingDirectory: '/camel',
      timeoutSec: 30,
    }, {
      name: 'Canonical',
      steps: [],
      dependsOn: ['Literal'],
      depends_on: ['Checkout'],
      workingDirectory: '/camel',
      working_directory: '/canonical',
      timeoutSec: 30,
      timeout_sec: 90,
    }],
    post: {
      failure: [{
        name: 'notify',
        type: 'shell',
        command: 'echo failed',
        platformAdditions: { macos: './camel.sh' },
      }, {
        name: 'cleanup',
        type: 'shell',
        command: 'echo cleanup',
        platformAdditions: { macos: './camel-cleanup.sh' },
        platform_additions: { macos: './canonical-cleanup.sh' },
      }],
    },
  }

  const pipeline = definePipeline(source)
  const literal = pipeline.stages[0]
  assert.deepEqual(literal.depends_on, ['Checkout'])
  assert.equal(literal.working_directory, '/camel')
  assert.equal(literal.timeout_sec, 30)
  assert.deepEqual(literal.steps[0].platform_additions, { macos: './sign.sh' })
  assert.equal(pipeline.stages[1].timeout_sec, 90)
  assert.deepEqual(pipeline.stages[1].depends_on, ['Checkout'])
  assert.equal(pipeline.stages[1].working_directory, '/canonical')
  assert.deepEqual(pipeline.post.failure[0].platform_additions, { macos: './camel.sh' })
  assert.deepEqual(pipeline.post.failure[1].platform_additions, { macos: './canonical-cleanup.sh' })
  assert.equal('depends_on' in source.stages[0], false)
  assert.equal('platform_additions' in source.stages[0].steps[0], false)
  assert.equal('platform_additions' in source.post.failure[0], false)
})
