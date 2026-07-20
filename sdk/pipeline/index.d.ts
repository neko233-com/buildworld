export type StepType = 'shell' | 'tail' | 'script' | 'git' | 'notify' | string
export type Step = { name: string; type: StepType; command?: string; shell?: string; config?: Record<string, string>; platformAdditions?: Record<string, string>; runtime?: string }
export type ServiceWatchOptions = { targetDir?: string; pidFile: string; logFile: string; port?: string | number; heartbeatSeconds?: number; pollSeconds?: number; initialLines?: number }
export type Stage = { name: string; steps: Step | Step[]; parallel?: boolean; dependsOn?: string[]; branches?: string[] }
export type Trigger = { type: string; config?: Record<string, string> }
export type Parameter = { name: string; type: 'string' | 'text' | 'password' | 'choice' | 'boolean' | 'number'; description?: string; default?: string | number | boolean; required?: boolean; choices?: string[]; isSecret?: boolean }
export type Pipeline = { name?: string; description?: string; environment?: Record<string, string>; parameters?: Parameter[]; stages: Stage[]; on?: Trigger[]; triggers?: Trigger[]; artifacts?: string[]; post?: Record<'always' | 'success' | 'failure' | 'cleanup', Step[]>; agentRequirements?: string[]; retentionCompleted?: number; timeoutSec?: number; allowLongRunning?: boolean; disableConcurrent?: boolean; abortPrevious?: boolean; toolchains?: Record<string, string[]> }
export declare function definePipeline(pipeline: Pipeline): Pipeline
export declare function step(name: string, type: StepType, command: string, options?: Omit<Step, 'name' | 'type' | 'command'>): Step
export declare function shell(name: string, command: string, options?: Omit<Step, 'name' | 'type' | 'command'>): Step
export declare function tail(name: string, command: string, options?: Omit<Step, 'name' | 'type' | 'command'>): Step
export declare function script(name: string, command: string, options?: Omit<Step, 'name' | 'type' | 'command'>): Step
export declare function git(name: string, options?: Omit<Step, 'name' | 'type' | 'command'>): Step
export declare function notify(name: string, options?: Omit<Step, 'name' | 'type' | 'command'>): Step
export declare function watchService(name: string, options: ServiceWatchOptions): Step
export declare function stage(name: string, steps: Step | Step[], options?: Omit<Stage, 'name' | 'steps'>): Stage
export declare function trigger(type: string, config?: Record<string, string>): Trigger
export declare function parameter(name: string, type: Parameter['type'], options?: Omit<Parameter, 'name' | 'type'>): Parameter
