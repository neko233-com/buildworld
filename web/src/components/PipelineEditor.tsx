import { useEffect, useState } from 'react'
import { motion } from 'motion/react'
import { Braces, Check, CirclePlay, Copy, FileCode2, RotateCcw } from 'lucide-react'
import { api } from '../api'
import { useI18n } from '../i18n'
import { isTypeScriptPipelineSource, prettyPipelineSource } from '../lib/configFormat'
import { isPipelineValidationReady, pendingPipelineValidation } from '../lib/pipelineValidation'
import { dialogs } from './AppDialogs'
import PipelineSourceEditor from './PipelineSourceEditor'

type PipelineProject = { id: number; name: string; config?: string }
type GlobalVariable = { id: number; name: string; value: string; is_secret?: boolean }

type PipelineEditorProps = {
  project: PipelineProject
  projects: PipelineProject[]
  globalVariables: GlobalVariable[]
  onProjectChange?: (id: number) => void
  onSave: (document: string) => Promise<void>
  onRun: () => Promise<unknown>
  embedded?: boolean
}

const initialDocument = `import { definePipeline, shell, stage } from '@buildworld/pipeline'

export default definePipeline({
  name: 'Build',
  stages: [
    stage('Build', shell('Compile', 'echo building')),
  ],
})
`

export default function PipelineEditor({ project, projects, onProjectChange, onSave, onRun, embedded = false }: PipelineEditorProps) {
  const { t } = useI18n()
  const source = project.config || initialDocument
  const [document, setDocument] = useState(source)
  const [savedDocument, setSavedDocument] = useState(source)
  const [saving, setSaving] = useState(false)
  const [running, setRunning] = useState(false)
  const [formatting, setFormatting] = useState(false)
  const [message, setMessage] = useState('')
  const [validation, setValidation] = useState(() => pendingPipelineValidation(source))
  const isDirty = document !== savedDocument
  const pipelineReady = isPipelineValidationReady(validation, document)
  const blockedMessage = validation.message || t('config.resolveErrors')
  const format = isTypeScriptPipelineSource(document) ? 'TypeScript' : 'YAML'

  useEffect(() => {
    const next = project.config || initialDocument
    setDocument(next)
    setSavedDocument(next)
    setMessage('')
    setValidation(pendingPipelineValidation(next))
  }, [project.id, project.config])

  useEffect(() => {
    const warnBeforeUnload = (event: BeforeUnloadEvent) => {
      if (!isDirty) return
      event.preventDefault()
    }
    window.addEventListener('beforeunload', warnBeforeUnload)
    return () => window.removeEventListener('beforeunload', warnBeforeUnload)
  }, [isDirty])

  const saveDocument = async () => {
    if (!isDirty || saving || !pipelineReady) {
      if (!pipelineReady) setMessage(blockedMessage)
      return
    }
    setSaving(true); setMessage('')
    try {
      const formatted = await prettyPipelineSource(document)
      await api.validatePipeline(formatted)
      await onSave(formatted)
      setDocument(formatted)
      setSavedDocument(formatted)
      setMessage(t('pipeline.savedMessage'))
    } catch (error: any) {
      setMessage(error?.message || t('pipeline.saveFailed'))
    } finally { setSaving(false) }
  }

  const runPipeline = async () => {
    if (running || !pipelineReady) {
      if (!pipelineReady) setMessage(blockedMessage)
      return
    }
    setRunning(true); setMessage('')
    try {
      if (isDirty) {
        const formatted = await prettyPipelineSource(document)
        await api.validatePipeline(formatted)
        await onSave(formatted)
        setDocument(formatted)
        setSavedDocument(formatted)
      }
      await onRun()
      setMessage(t('pipeline.queued'))
    } catch (error: any) {
      setMessage(error?.message || t('pipeline.runFailed'))
    } finally { setRunning(false) }
  }

  const formatDocument = async () => {
    if (formatting) return
    setFormatting(true); setMessage('')
    try {
      const formatted = await prettyPipelineSource(document)
      await api.validatePipeline(formatted)
      setDocument(formatted)
      setMessage(t('config.formatted'))
      dialogs.notify(t('config.formatted'), 'success')
    } catch (error: any) {
      const reason = error?.message || t('config.invalid')
      setMessage(reason)
      dialogs.notify(reason)
    } finally { setFormatting(false) }
  }

  const copySource = async () => {
    try {
      await navigator.clipboard.writeText(document)
      setMessage(t('pipeline.copied'))
    } catch { dialogs.notify(t('common.copyFailed')) }
  }

  const changeProject = async (id: number) => {
    if (id === project.id) return
    if (isDirty && !await dialogs.confirm(t('pipeline.discardMessage'), { title: t('pipeline.discardTitle'), action: t('pipeline.discardAction') })) return
    onProjectChange?.(id)
  }

  return <motion.section className={`pipeline-workbench pipeline-code-workbench ${embedded ? 'pipeline-workbench-embedded' : ''}`} initial={{ opacity: 0, y: 8 }} animate={{ opacity: 1, y: 0 }} transition={{ duration: 0.22, ease: 'easeOut' }}>
    <header className="pipeline-heading">
      <div>{!embedded && <div className="crumbs">{project.name}</div>}<div className="title-row"><h1>{embedded ? t('pipeline.definition') : project.name}</h1><span className="saved-state"><Check size={14} />{isDirty ? t('pipeline.unsaved') : t('pipeline.saved')}</span></div></div>
      <div className="pipeline-actions">
        <span className="template-config-kind"><FileCode2 size={13} />{format}</span>
        <button className="icon-button" type="button" title={t('pipeline.revert')} aria-label={t('pipeline.revert')} onClick={() => { setDocument(savedDocument); setMessage('') }} disabled={!isDirty}><RotateCcw size={16} /></button>
        {!embedded && <select className="select-control project-picker" value={project.id} onChange={event => changeProject(Number(event.target.value))} aria-label={t('pipeline.selectProject')}>{projects.map(item => <option key={item.id} value={item.id}>{item.name}</option>)}</select>}
        <button className="secondary-button" type="button" onClick={formatDocument} disabled={formatting}><Braces size={15} />{formatting ? t('config.formatting') : format === 'TypeScript' ? t('config.validate') : t('config.format')}</button>
        <button className="secondary-button" type="button" onClick={copySource}><Copy size={15} />{t('common.copy')}</button>
        <button className="secondary-button" type="button" onClick={saveDocument} disabled={!isDirty || saving || !pipelineReady} aria-describedby={!pipelineReady ? 'pipeline-editor-validation-status' : undefined} title={!pipelineReady ? blockedMessage : undefined}>{saving ? t('pipeline.saving') : t('pipeline.save')}</button>
        <button className="primary-button" type="button" onClick={runPipeline} disabled={running || !pipelineReady} aria-describedby={!pipelineReady ? 'pipeline-editor-validation-status' : undefined} title={!pipelineReady ? blockedMessage : undefined}><CirclePlay size={16} />{running ? t('pipeline.running') : t('pipeline.run')}</button>
      </div>
    </header>
    <section className="pipeline-source-panel pipeline-source-only">
      <div className="panel-header"><div><FileCode2 size={16} />{t('projectDetail.pipelineConfig')}</div><span>{format === 'TypeScript' ? t('config.typescriptHelp') : t('config.yamlHelp')}</span></div>
      <div className="editor-body monaco-body"><PipelineSourceEditor ariaLabel={t('pipeline.sourceLabel')} value={document} onChange={next => { setValidation(pendingPipelineValidation(next)); setDocument(next) }} onValidationChange={setValidation} statusId="pipeline-editor-validation-status" height="100%" /></div>
    </section>
    <div className="run-strip"><span className={`run-dot ${isDirty ? 'pending' : ''}`} /><strong>{isDirty ? t('pipeline.draft') : t('pipeline.ready')}</strong><span>{project.name}</span><span className="run-stage">{format}</span><div className="run-progress"><i /></div><span className="run-time">{message || validation.message}</span></div>
  </motion.section>
}
