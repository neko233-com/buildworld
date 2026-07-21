export type PipelineValidationState = {
  source: string
  checking: boolean
  diagnosticsReady: boolean
  diagnosticErrors: number
  serverValid: boolean
  valid: boolean
  message: string
  problems: number
}

export function pendingPipelineValidation(source: string): PipelineValidationState {
  return {
    source,
    checking: false,
    diagnosticsReady: false,
    diagnosticErrors: 0,
    serverValid: false,
    valid: false,
    message: '',
    problems: 0,
  }
}

export function isPipelineValidationReady(
  validation: PipelineValidationState,
  source: string,
): boolean {
  return validation.source === source
    && validation.diagnosticsReady
    && validation.diagnosticErrors === 0
    && validation.serverValid
    && !validation.checking
    && validation.valid
}
