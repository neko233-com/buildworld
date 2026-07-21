const hasOwn = (value, key) => Object.prototype.hasOwnProperty.call(value, key)

const withAliases = (source, aliases) => {
  const value = { ...(source ?? {}) }
  for (const [camel, snake] of aliases) {
    if (hasOwn(value, camel) && !hasOwn(value, snake)) value[snake] = value[camel]
  }
  return value
}

const pipelineAliases = [
  ['agentRequirements', 'agent_requirements'],
  ['retentionCompleted', 'retention_completed'],
  ['timeoutSec', 'timeout_sec'],
  ['allowLongRunning', 'allow_long_running'],
  ['disableConcurrent', 'disable_concurrent'],
  ['abortPrevious', 'abort_previous'],
  ['on', 'triggers'],
]

const watchAliases = [
  ['targetDir', 'target_dir'],
  ['pidFile', 'pid_file'],
  ['logFile', 'log_file'],
  ['heartbeatSeconds', 'heartbeat_seconds'],
  ['pollSeconds', 'poll_seconds'],
  ['initialLines', 'initial_lines'],
]

export const definePipeline = source => {
  const pipeline = withAliases(source, pipelineAliases)
  if (pipeline.approval) {
    pipeline.approval = withAliases(pipeline.approval, [
      ['requiredRoles', 'required_roles'],
      ['allowRequester', 'allow_requester'],
    ])
  }
  return pipeline
}

export const step = (name, type, command, options = {}) => ({
  ...withAliases(options, [['platformAdditions', 'platform_additions']]),
  name,
  type,
  command,
})

export const shell = (name, command, options) => step(name, 'shell', command, options)
export const tail = (name, command, options) => step(name, 'tail', command, options)
export const script = (name, command, options) => step(name, 'script', command, options)
export const git = (name, options) => step(name, 'git', '', options)
export const notify = (name, options) => step(name, 'notify', '', options)

export const watchService = (name, options = {}) => {
  const normalized = withAliases(options, watchAliases)
  const config = Object.fromEntries(
    Object.entries(normalized)
      .filter(([, value]) => value !== undefined && value !== null)
      .map(([key, value]) => [key, String(value)]),
  )
  return { name, type: 'service_watch', config }
}

export const stage = (name, steps, options = {}) => ({
  ...withAliases(options, [
    ['dependsOn', 'depends_on'],
    ['workingDirectory', 'working_directory'],
    ['timeoutSec', 'timeout_sec'],
  ]),
  name,
  steps: Array.isArray(steps) ? steps : [steps],
})

export const trigger = (type, config = {}) => ({ type, config: config ?? {} })

export const parameter = (name, type, options = {}) => ({
  ...withAliases(options, [['isSecret', 'is_secret']]),
  name,
  type,
})
