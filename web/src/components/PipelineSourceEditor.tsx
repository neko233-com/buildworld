import { lazy, Suspense, useEffect, useRef, useState } from 'react'
import { api } from '../api'
import { useI18n } from '../i18n'
import { resolvePipelineSourceLanguage } from '../lib/configFormat'
import { pendingPipelineValidation, type PipelineValidationState } from '../lib/pipelineValidation'

const PipelineCodeEditor = lazy(() => import('./PipelineCodeEditor'))

type Props = {
  value: string
  onChange: (value: string) => void
  readOnly?: boolean
  ariaLabel: string
  height?: number | string
  statusId?: string
  onValidationChange?: (state: PipelineValidationState) => void
}

function YAMLPipelineEditor({ value, onChange, readOnly, ariaLabel, height = 420, statusId, onValidationChange }: Props) {
  const { t } = useI18n()
  const sequence = useRef(0)
  const callback = useRef(onValidationChange)
  const [validation, setValidation] = useState(() => pendingPipelineValidation(value))
  callback.current = onValidationChange

  useEffect(() => {
    const current = ++sequence.current
    const publish = (state: PipelineValidationState) => {
      if (current !== sequence.current) return
      setValidation(state)
      callback.current?.(state)
    }
    if (!value.trim()) {
      publish({ ...pendingPipelineValidation(value), diagnosticsReady: true, message: t('config.invalid'), problems: 1 })
      return
    }
    publish({ ...pendingPipelineValidation(value), checking: true, diagnosticsReady: true, message: t('config.validating') })
    const timer = window.setTimeout(async () => {
      try {
        await api.validatePipeline(value)
        publish({ source: value, checking: false, diagnosticsReady: true, diagnosticErrors: 0, serverValid: true, valid: true, message: t('config.validated'), problems: 0 })
      } catch (error: any) {
        publish({ source: value, checking: false, diagnosticsReady: true, diagnosticErrors: 0, serverValid: false, valid: false, message: error?.message || t('config.invalid'), problems: 1 })
      }
    }, 450)
    return () => window.clearTimeout(timer)
  }, [t, value])

  useEffect(() => () => { sequence.current += 1 }, [])

  return <div className="pipeline-monaco-editor pipeline-yaml-editor" aria-label={ariaLabel} aria-describedby={statusId} aria-invalid={!validation.checking && !validation.valid} aria-busy={validation.checking}>
    <textarea className="pipeline-source-input" style={{ height }} readOnly={readOnly} aria-label={ariaLabel} value={value} onChange={event => onChange(event.target.value)} spellCheck="false" />
    <div className="pipeline-monaco-status" id={statusId} role="status" aria-live="polite">
      <span className={validation.valid ? 'valid' : validation.checking ? 'checking' : validation.problems ? 'invalid' : ''}>{validation.message || t('config.validating')}</span>
      <span>YAML</span><span>UTF-8</span><span>{validation.problems} {t('config.problems')}</span>
    </div>
  </div>
}

export default function PipelineSourceEditor(props: Props) {
  const language = useRef(resolvePipelineSourceLanguage(props.value))
  language.current = resolvePipelineSourceLanguage(props.value, language.current)
  if (language.current === 'yaml') return <YAMLPipelineEditor {...props} />
  return <Suspense fallback={<div className="pipeline-source-input">Loading editor…</div>}><PipelineCodeEditor {...props} /></Suspense>
}
