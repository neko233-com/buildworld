import { describe, expect, it } from 'vitest'
import { normalizeBuildProblemReport } from './buildProblems'

describe('normalizeBuildProblemReport', () => {
  it('keeps diagnostic text while constraining values used by the UI', () => {
    const report = normalizeBuildProblemReport({
      version: 1,
      build_status: 'failed',
      summary: 'failed',
      failed_step_count: 1,
      problems: [{
        id: 'compiler',
        code: 'compile_failed',
        severity: 'error injected-class',
        stage: 'Build',
        step: 'Compile',
        message: '<img src=x onerror="alert(1)">',
        excerpt: '<script>alert(1)</script>',
        line: 9,
        suggested_action: 'javascript:alert(1)',
      }],
    })

    expect(report.problems[0]).toEqual({
      id: 'compiler',
      code: 'compile_failed',
      severity: 'error',
      stage: 'Build',
      step: 'Compile',
      message: '<img src=x onerror="alert(1)">',
      excerpt: '<script>alert(1)</script>',
      line: 9,
      suggested_action: 'inspect_logs',
    })
  })

  it('returns a stable empty report for a malformed response', () => {
    expect(normalizeBuildProblemReport(null)).toEqual({
      version: 1,
      build_status: '',
      summary: '',
      failed_step_count: 0,
      problems: [],
    })
  })
})
