export type BuildProblemAction = 'project_settings' | 'inspect_logs' | 'worker' | 'approval' | 'retry'

export interface BuildProblem {
  id: string
  code: string
  severity: 'error' | 'warning'
  stage?: string
  step?: string
  message: string
  excerpt?: string
  line?: number
  suggested_action: BuildProblemAction
}

export interface BuildProblemReport {
  version: number
  build_status: string
  summary: string
  failed_step_count: number
  problems: BuildProblem[]
}
