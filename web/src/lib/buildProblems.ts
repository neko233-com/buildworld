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

const problemActions = new Set<BuildProblemAction>([
  'project_settings',
  'inspect_logs',
  'worker',
  'approval',
  'retry',
])

function record(value: unknown): Record<string, unknown> {
  return value !== null && typeof value === 'object' ? value as Record<string, unknown> : {}
}

function text(value: unknown, fallback = ''): string {
  return typeof value === 'string' ? value : fallback
}

function optionalText(value: unknown): string | undefined {
  const result = text(value).trim()
  return result || undefined
}

function nonNegativeInteger(value: unknown, fallback = 0): number {
  return typeof value === 'number' && Number.isSafeInteger(value) && value >= 0 ? value : fallback
}

/**
 * Normalizes the server response before it reaches rendering code. Unknown
 * enum values cannot become CSS class names or action routes; diagnostic text
 * remains plain strings and is rendered through React text nodes.
 */
export function normalizeBuildProblemReport(value: unknown): BuildProblemReport {
  const source = record(value)
  const rawProblems = Array.isArray(source.problems) ? source.problems : []
  const problems = rawProblems.map((value, index): BuildProblem => {
    const problem = record(value)
    const action = text(problem.suggested_action) as BuildProblemAction
    const line = nonNegativeInteger(problem.line)

    return {
      id: optionalText(problem.id) || `problem-${index + 1}`,
      code: optionalText(problem.code) || 'build_failed',
      severity: problem.severity === 'warning' ? 'warning' : 'error',
      ...(optionalText(problem.stage) ? { stage: optionalText(problem.stage) } : {}),
      ...(optionalText(problem.step) ? { step: optionalText(problem.step) } : {}),
      message: text(problem.message),
      ...(optionalText(problem.excerpt) ? { excerpt: optionalText(problem.excerpt) } : {}),
      ...(line > 0 ? { line } : {}),
      suggested_action: problemActions.has(action) ? action : 'inspect_logs',
    }
  })

  return {
    version: nonNegativeInteger(source.version, 1) || 1,
    build_status: text(source.build_status),
    summary: text(source.summary),
    failed_step_count: nonNegativeInteger(source.failed_step_count),
    problems,
  }
}
