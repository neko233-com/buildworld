import { useRef, useState } from 'react'
import { BookTemplate, Braces, LoaderCircle, Pencil, Play, Plus, Trash2, X } from 'lucide-react'
import { useNavigate } from 'react-router-dom'
import { api } from '../api'
import { canEdit } from '../authz'
import { dialogs } from '../components/AppDialogs'
import { JenkinsHeaderBreadcrumb } from '../components/JenkinsPageShell'
import { ModalDialog } from '../components/ModalDialog'
import { PageState } from '../components/PageState'
import PipelineSourceEditor from '../components/PipelineSourceEditor'
import { useApi } from '../hooks'
import { useI18n } from '../i18n'
import { isTypeScriptPipelineSource, prettyPipelineSource, prettyPipelineSourceSync } from '../lib/configFormat'
import { formatDateWithWeekday } from '../lib/dateTime'
import {
  isPipelineValidationReady,
  pendingPipelineValidation,
} from '../lib/pipelineValidation'
import './ManagementPages.jenkins.css'
import './Templates.jenkins.css'

export type BuildTemplate = {
  id: number
  name: string
  description: string
  config: string
  created_at?: string
  updated_at?: string
  vcs_root_id?: number | null
}

const templateDefault = `import { definePipeline, shell, stage } from '@buildworld/pipeline'

export default definePipeline({
  stages: [
    stage('Build', [
      shell('Compile', 'echo building'),
    ]),
  ],
})
`

const emptyForm = () => ({ name: '', description: '', config: templateDefault })

export default function Templates() {
  const { t } = useI18n()
  const editable = canEdit()
  const navigate = useNavigate()
  const nameInputRef = useRef<HTMLInputElement>(null)
  const { data, loading, error, reload } = useApi(
    () => Promise.all([api.listTemplates(), api.listProjects()]),
    [],
  )
  const [showEditor, setShowEditor] = useState(false)
  const [editing, setEditing] = useState<BuildTemplate | null>(null)
  const [form, setForm] = useState(emptyForm)
  const [saving, setSaving] = useState(false)
  const [formatting, setFormatting] = useState(false)
  const [deletingID, setDeletingID] = useState<number | null>(null)
  const [formError, setFormError] = useState('')
  const [actionError, setActionError] = useState('')
  const [pipelineValidation, setPipelineValidation] = useState(() => pendingPipelineValidation(templateDefault))
  const typeScriptConfig = isTypeScriptPipelineSource(form.config)
  const pipelineReady = isPipelineValidationReady(pipelineValidation, form.config)
  const pipelineBlockedMessage = pipelineValidation.message || t('config.resolveErrors')
  const breadcrumb = <JenkinsHeaderBreadcrumb breadcrumbs={[{ label: t('templates.title') }]} />

  const openCreate = () => {
    const next = emptyForm()
    setEditing(null)
    setForm(next)
    setFormError('')
    setPipelineValidation(pendingPipelineValidation(next.config))
    setShowEditor(true)
  }

  const openEdit = (template: BuildTemplate) => {
    let config = template.config || templateDefault
    try { config = prettyPipelineSourceSync(config) } catch { /* Live validation describes unsupported source. */ }
    setEditing(template)
    setForm({ name: template.name, description: template.description || '', config })
    setFormError('')
    setPipelineValidation(pendingPipelineValidation(config))
    setShowEditor(true)
  }

  const handleFormat = async () => {
    setFormatting(true)
    setFormError('')
    try {
      const config = typeScriptConfig ? form.config : await prettyPipelineSource(form.config)
      await api.validatePipeline(config)
      if (typeScriptConfig && (!pipelineValidation.diagnosticsReady || pipelineValidation.diagnosticErrors > 0 || pipelineValidation.source !== config)) {
        throw new Error(pipelineBlockedMessage)
      }
      setForm(current => ({ ...current, config }))
      dialogs.notify(typeScriptConfig ? t('config.validated') : t('config.formatted'), 'success')
    } catch (reason: any) {
      const message = reason.message || t('config.invalid')
      setFormError(message)
    } finally {
      setFormatting(false)
    }
  }

  const handleSubmit = async (event: React.FormEvent) => {
    event.preventDefault()
    const name = form.name.trim()
    if (!name) {
      setFormError(t('templates.nameRequired'))
      nameInputRef.current?.focus()
      return
    }
    if (!pipelineReady) {
      setFormError(pipelineBlockedMessage)
      return
    }

    setSaving(true)
    setFormError('')
    try {
      const config = await prettyPipelineSource(form.config)
      await api.validatePipeline(config)
      const payload = { name, description: form.description.trim(), config }
      if (editing) await api.updateTemplate(editing.id, payload)
      else await api.createTemplate(payload)
      setShowEditor(false)
      dialogs.notify(t(editing ? 'templates.updated' : 'templates.createdSuccess'), 'success')
      reload()
    } catch (reason: any) {
      setFormError(reason.message || t('templates.saveFailed'))
    } finally {
      setSaving(false)
    }
  }

  const handleDelete = async (template: BuildTemplate) => {
    if (!await dialogs.confirm(t('templates.deleteConfirm').replace('{name}', template.name), {
      title: t('templates.removeTitle'),
      action: t('common.delete'),
    })) return

    setDeletingID(template.id)
    setActionError('')
    try {
      await api.deleteTemplate(template.id)
      dialogs.notify(t('templates.deleted').replace('{name}', template.name), 'success')
      reload()
    } catch (reason: any) {
      setActionError(reason.message || t('templates.deleteFailed'))
    } finally {
      setDeletingID(null)
    }
  }

  if (loading) return <>{breadcrumb}<section className="jenkins-management-page"><PageState /></section></>
  if (error) return <>{breadcrumb}<section className="jenkins-management-page"><PageState error={error} onRetry={reload} /></section></>

  const list = (data?.[0] || []) as BuildTemplate[]
  const projects = data?.[1] || []
  const references = projects.filter((project: any) => project.template_id).length

  return <>{breadcrumb}<section className="operations-page templates-workbench jenkins-management-page">
    <header className="operations-heading">
      <div><p>{list.length} {t('templates.registered')} · {references} {t('templates.projectsUsing')}</p><h1>{t('templates.title')}</h1></div>
      {editable && <button className="primary-command" type="button" onClick={openCreate}><Plus size={16} />{t('templates.new')}</button>}
    </header>

    {actionError && <p className="templates-action-error" role="alert">{actionError}</p>}

    <section className="operations-table-wrap templates-table-wrap" aria-label={t('templates.title')}>
      <table className="operations-table templates-table">
        <thead><tr><th>{t('templates.name')}</th><th>{t('templates.description')}</th><th>{t('templates.config')}</th><th>{t('templates.created')}</th><th aria-label={t('projects.actions')} /></tr></thead>
        <tbody>
          {!list.length && <tr><td colSpan={5} className="operations-empty"><BookTemplate size={18} />{t('templates.empty')}</td></tr>}
          {list.map(template => {
            const deleting = deletingID === template.id
            return <tr key={template.id} aria-busy={deleting || undefined}>
              <td>{editable
                ? <button className="entity-link" type="button" disabled={Boolean(deletingID)} onClick={() => openEdit(template)}><BookTemplate size={16} /><span><strong>{template.name}</strong><small>#{template.id}</small></span></button>
                : <span className="templates-name"><BookTemplate size={16} /><span><strong>{template.name}</strong><small>#{template.id}</small></span></span>}</td>
              <td className="muted-cell template-description">{template.description || '-'}</td>
              <td><span className="template-config-kind"><Braces size={13} />{isTypeScriptPipelineSource(template.config || '') ? 'TypeScript' : 'YAML'}</span></td>
              <td className="muted-cell">{formatDateWithWeekday(template.created_at)}</td>
              <td><div className="row-actions">{editable && <>
                <button className="row-run" type="button" disabled={Boolean(deletingID)} onClick={() => navigate(`/projects/new?template=${template.id}`)}><Play size={13} />{t('templates.use')}</button>
                <button className="row-icon" type="button" disabled={Boolean(deletingID)} title={t('templates.edit')} aria-label={`${t('templates.edit')}: ${template.name}`} onClick={() => openEdit(template)}><Pencil size={15} /></button>
                <button className="row-icon danger" type="button" disabled={Boolean(deletingID)} title={t('common.delete')} aria-label={`${t('common.delete')}: ${template.name}`} onClick={() => void handleDelete(template)}>{deleting ? <LoaderCircle className="timeline-spinner" size={15} /> : <Trash2 size={15} />}</button>
              </>}</div></td>
            </tr>
          })}
        </tbody>
      </table>
    </section>

    {showEditor && <ModalDialog className="notification-editor template-editor" ariaLabel={editing ? t('templates.edit') : t('templates.new')} busy={saving || formatting} onClose={() => setShowEditor(false)}>
      <header><div><BookTemplate size={18} /><div><h2>{editing ? t('templates.edit') : t('templates.new')}</h2><p>{t('templates.editorHelp')}</p></div></div><button type="button" disabled={saving || formatting} onClick={() => setShowEditor(false)} title={t('common.close')}><X size={18} /></button></header>
      <form className="notification-editor-form" onSubmit={handleSubmit} noValidate>
        <label>{t('templates.name')}<input ref={nameInputRef} required autoFocus data-dialog-initial-focus aria-invalid={Boolean(formError && !form.name.trim()) || undefined} value={form.name} onChange={event => { setForm({ ...form, name: event.target.value }); setFormError('') }} /></label>
        <label>{t('templates.description')}<input value={form.description} onChange={event => setForm({ ...form, description: event.target.value })} /></label>
        <div className="wide template-config-field"><div className="config-label-row"><label id="template-config-label">{t('templates.config')} (TypeScript / YAML)</label><button className="format-command" type="button" disabled={saving || formatting} onClick={() => void handleFormat()}><Braces size={13} />{formatting ? t('config.formatting') : typeScriptConfig ? t('config.validate') : t('config.format')}</button></div><PipelineSourceEditor ariaLabel={t('templates.config')} value={form.config} readOnly={saving} onChange={config => { setPipelineValidation(pendingPipelineValidation(config)); setForm(current => ({ ...current, config })); setFormError('') }} onValidationChange={setPipelineValidation} statusId="template-pipeline-status" height={420} /><small>{t('templates.configHelp')}</small></div>
        {formError && <p className="form-error" role="alert">{formError}</p>}
        <footer><button type="button" disabled={saving || formatting} onClick={() => setShowEditor(false)}>{t('common.cancel')}</button><button type="submit" disabled={saving || formatting || !pipelineReady} aria-describedby={!pipelineReady ? 'template-pipeline-status' : undefined} title={!pipelineReady ? pipelineBlockedMessage : undefined}>{saving ? <><LoaderCircle className="timeline-spinner" size={14} />{t('common.loading')}</> : t('common.save')}</button></footer>
      </form>
    </ModalDialog>}
  </section></>
}
