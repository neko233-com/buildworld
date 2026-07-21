import { useState } from 'react'
import { AlertTriangle, ArrowRight, Braces, FileInput, X } from 'lucide-react'
import { api, type PipelineMigrationResult } from '../api'
import { useI18n } from '../i18n'
import { ModalDialog } from './ModalDialog'

type JenkinsfileImportDialogProps = {
  projectName?: string
  onApply: (result: PipelineMigrationResult) => void
  onClose: () => void
}

export default function JenkinsfileImportDialog({ projectName, onApply, onClose }: JenkinsfileImportDialogProps) {
  const { t } = useI18n()
  const [source, setSource] = useState('')
  const [result, setResult] = useState<PipelineMigrationResult | null>(null)
  const [converting, setConverting] = useState(false)
  const [error, setError] = useState('')
  const warningMessage = (warning: PipelineMigrationResult['warnings'][number]) => {
    if (warning.code === 'post_review_required') return t('jenkinsImport.postWarning')
    if (warning.code === 'options_review_required') return t('jenkinsImport.optionsWarning')
    if (warning.code === 'macos_protected_directory') return t('jenkinsImport.macosProtectedDirectoryWarning')
    return warning.message
  }

  const convert = async () => {
    if (!source.trim()) {
      setError(t('jenkinsImport.sourceRequired'))
      return
    }
    setConverting(true)
    setError('')
    try {
      setResult(await api.migratePipeline('jenkinsfile', source, projectName))
    } catch (reason: any) {
      setResult(null)
      setError(reason.message || t('jenkinsImport.failed'))
    } finally {
      setConverting(false)
    }
  }

  const apply = () => {
    if (!result) return
    onApply(result)
    onClose()
  }

  return <ModalDialog ariaLabel={t('jenkinsImport.title')} className="jenkins-import-dialog" busy={converting} onClose={onClose}>
    <header><div><FileInput size={19} /><div><h2>{t('jenkinsImport.title')}</h2><p>{t('jenkinsImport.description')}</p></div></div><button type="button" onClick={onClose} disabled={converting} title={t('common.cancel')} aria-label={t('common.cancel')}><X size={18} /></button></header>
    <div className="jenkins-import-body">
      <section className="jenkins-source-panel">
        <div className="jenkins-panel-heading"><span>Jenkinsfile</span><small>{t('jenkinsImport.pasteHelp')}</small></div>
        <textarea
          data-dialog-initial-focus
          aria-label={t('jenkinsImport.sourceLabel')}
          value={source}
          onChange={event => { setSource(event.target.value); setResult(null); setError('') }}
          placeholder={'pipeline {\n    agent any\n    stages { ... }\n}'}
          spellCheck="false"
        />
      </section>
      <div className="jenkins-conversion-divider" aria-hidden="true"><ArrowRight size={17} /></div>
      <section className="jenkins-preview-panel">
        <div className="jenkins-panel-heading"><span><Braces size={14} />{t('jenkinsImport.preview')}</span>{result && <small>{t('jenkinsImport.summary').replace('{stages}', String(result.summary.stage_count)).replace('{environment}', String(result.summary.environment_count))}</small>}</div>
        {result ? <textarea aria-label={t('jenkinsImport.previewLabel')} value={result.config} readOnly spellCheck="false" /> : <div className="jenkins-preview-empty"><Braces size={22} /><strong>{t('jenkinsImport.previewEmpty')}</strong><span>{t('jenkinsImport.previewEmptyHelp')}</span></div>}
      </section>
      {result?.warnings.length ? <aside className="jenkins-import-warnings"><AlertTriangle size={16} /><div><strong>{t('jenkinsImport.reviewTitle')}</strong><ul>{result.warnings.map(warning => <li key={`${warning.code}-${warning.message}`}>{warningMessage(warning)}</li>)}</ul></div></aside> : null}
      {error && <p className="form-error jenkins-import-error" role="alert">{error}</p>}
    </div>
    <footer><button type="button" onClick={onClose} disabled={converting}>{t('common.cancel')}</button><button type="button" className="secondary-command" onClick={convert} disabled={converting || !source.trim()}>{converting ? t('jenkinsImport.converting') : t('jenkinsImport.convert')}</button><button type="button" className="primary-command" onClick={apply} disabled={!result || converting}>{t('jenkinsImport.apply')}</button></footer>
  </ModalDialog>
}
