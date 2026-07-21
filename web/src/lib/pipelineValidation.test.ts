import { describe, expect, it } from 'vitest'
import {
  isPipelineValidationReady,
  pendingPipelineValidation,
  type PipelineValidationState,
} from './pipelineValidation'

function validated(overrides: Partial<PipelineValidationState> = {}): PipelineValidationState {
  return {
    ...pendingPipelineValidation('pipeline source'),
    diagnosticsReady: true,
    serverValid: true,
    valid: true,
    ...overrides,
  }
}

describe('isPipelineValidationReady', () => {
  it('requires Monaco diagnostics and server validation for the current source', () => {
    expect(isPipelineValidationReady(validated(), 'pipeline source')).toBe(true)
    expect(isPipelineValidationReady(validated({ diagnosticsReady: false }), 'pipeline source')).toBe(false)
    expect(isPipelineValidationReady(validated({ serverValid: false }), 'pipeline source')).toBe(false)
    expect(isPipelineValidationReady(validated({ checking: true }), 'pipeline source')).toBe(false)
    expect(isPipelineValidationReady(validated(), 'changed source')).toBe(false)
  })

  it('cannot be bypassed by setting valid when Monaco still has an Error', () => {
    expect(isPipelineValidationReady(validated({
      diagnosticErrors: 1,
      problems: 1,
      valid: true,
    }), 'pipeline source')).toBe(false)
  })
})
