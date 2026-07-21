export type StepType = 'shell' | 'tail' | 'script' | 'git' | 'notify' | 'service_watch' | string
export type PostCondition = 'always' | 'success' | 'failure' | 'cleanup'
export type ApprovalRole = 'admin' | 'developer'

export type ApprovalPolicy = {
  version?: number
  strategy?: 'none' | 'single'
  requiredRoles?: readonly ApprovalRole[]
  required_roles?: readonly ApprovalRole[]
  allowRequester?: boolean
  allow_requester?: boolean
  prompt?: string
}

export type Step = {
  name: string
  type: StepType
  command?: string
  shell?: string
  config?: Readonly<Record<string, string>>
  platformAdditions?: Readonly<Record<string, string>>
  platform_additions?: Readonly<Record<string, string>>
  runtime?: string
  if?: string
}

type ServiceWatchPidFile = {
  pidFile: string
  pid_file?: string
} | {
  pidFile?: string
  pid_file: string
}

type ServiceWatchLogFile = {
  logFile: string
  log_file?: string
} | {
  logFile?: string
  log_file: string
}

export type ServiceWatchOptions = ServiceWatchPidFile & ServiceWatchLogFile & {
  targetDir?: string
  target_dir?: string
  port?: string | number
  /** Integer seconds from 5 through 86400. Defaults to 30. */
  heartbeatSeconds?: number
  heartbeat_seconds?: number
  /**
   * PID liveness-check interval in whole seconds, 1 through 3600.
   * Log bytes are followed independently at near-real-time cadence.
   * The PID check defaults to 0.5 seconds when omitted.
   */
  pollSeconds?: number
  poll_seconds?: number
  /** History lines to print on attachment, 0 through 10000. Zero skips history. */
  initialLines?: number
  initial_lines?: number
}

export type Stage = {
  id?: string
  name: string
  steps: readonly Step[]
  parallel?: boolean
  dependsOn?: readonly string[]
  depends_on?: readonly string[]
  branches?: readonly string[]
  if?: string
  environment?: Readonly<Record<string, string>> | null
  workingDirectory?: string
  working_directory?: string
  timeoutSec?: number
  timeout_sec?: number
}

export type Trigger = {
  type: string
  config?: Readonly<Record<string, string>> | null
}

export type Parameter = {
  name: string
  type: 'string' | 'text' | 'password' | 'choice' | 'boolean' | 'number'
  description?: string
  default?: string | number | boolean | null
  required?: boolean
  choices?: readonly string[]
  isSecret?: boolean
  is_secret?: boolean
}

export type Pipeline = {
  name?: string
  description?: string
  environment?: Readonly<Record<string, string>> | null
  parameters?: readonly Parameter[] | null
  approval?: ApprovalPolicy | null
  stages: readonly Stage[]
  on?: readonly Trigger[] | null
  triggers?: readonly Trigger[] | null
  artifacts?: readonly string[] | null
  post?: Partial<Record<PostCondition, readonly Step[]>> | null
  agentRequirements?: readonly string[] | null
  agent_requirements?: readonly string[] | null
  retentionCompleted?: number
  retention_completed?: number
  timeoutSec?: number
  timeout_sec?: number
  allowLongRunning?: boolean
  allow_long_running?: boolean
  disableConcurrent?: boolean
  disable_concurrent?: boolean
  abortPrevious?: boolean
  abort_previous?: boolean
  toolchains?: Readonly<Record<string, readonly string[]>> | null
}

export declare function definePipeline<const T extends Pipeline>(pipeline: T): T & Pipeline
export declare function step(name: string, type: StepType, command: string, options?: Omit<Step, 'name' | 'type' | 'command'>): Step
export declare function shell(name: string, command: string, options?: Omit<Step, 'name' | 'type' | 'command'>): Step
export declare function tail(name: string, command: string, options?: Omit<Step, 'name' | 'type' | 'command'>): Step
export declare function script(name: string, command: string, options?: Omit<Step, 'name' | 'type' | 'command'>): Step
export declare function git(name: string, options?: Omit<Step, 'name' | 'type' | 'command'>): Step
export declare function notify(name: string, options?: Omit<Step, 'name' | 'type' | 'command'>): Step
/** Follows a PID/log as a long-running step. Explicit pipeline/stage timeouts still apply. */
export declare function watchService(name: string, options: ServiceWatchOptions): Step
export declare function stage(name: string, steps: Step | readonly Step[], options?: Omit<Stage, 'name' | 'steps'>): Stage
export declare function trigger(type: string, config?: Readonly<Record<string, string>> | null): Trigger
export declare function parameter(name: string, type: Parameter['type'], options?: Omit<Parameter, 'name' | 'type'>): Parameter
