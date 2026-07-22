import { lazy, Suspense, useRef } from 'react'
import { resolvePipelineSourceLanguage } from '../lib/configFormat'

const PipelineCodeEditor = lazy(() => import('./PipelineCodeEditor'))

type Props = {
  value: string
  onChange: (value: string) => void
  readOnly?: boolean
  ariaLabel: string
  height?: number | string
  statusId?: string
  language?: 'typescript' | 'yaml' | 'jenkinsfile'
  onValidationChange?: (state: import('../lib/pipelineValidation').PipelineValidationState) => void
}

export default function PipelineSourceEditor(props: Props) {
  const detected = useRef(resolvePipelineSourceLanguage(props.value))
  detected.current = resolvePipelineSourceLanguage(props.value, detected.current)
  const language = props.language ?? detected.current
  return (
    <Suspense fallback={<div className="pipeline-source-input">Loading editor…</div>}>
      <PipelineCodeEditor {...props} language={language} />
    </Suspense>
  )
}
