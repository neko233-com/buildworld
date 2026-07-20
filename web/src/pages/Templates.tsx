import { useState } from 'react'
import { motion } from 'motion/react'
import { BookTemplate, Braces, Boxes, Pencil, Play, Plus, Trash2, X } from 'lucide-react'
import { useNavigate } from 'react-router-dom'
import { api } from '../api'
import { useApi } from '../hooks'
import { dialogs } from '../components/AppDialogs'
import { ModalDialog } from '../components/ModalDialog'
import { PageState } from '../components/PageState'
import { useI18n } from '../i18n'
import { prettyConfigSource, prettyConfigSourceSync } from '../lib/configFormat'
import { canEdit } from '../authz'

interface Template {
  id: number
  name: string
  description: string
  config: string
  created_at?: string
  updated_at?: string
}

const templateDefault = `{
  "stages": [
    {
      "name": "Build",
      "steps": [
        {
          "name": "compile",
          "type": "shell",
          "command": "echo building"
        }
      ]
    }
  ]
}
`
const emptyForm = { name: '', description: '', config: templateDefault }

export default function Templates() {
  const { t } = useI18n()
  const editable = canEdit()
  const navigate = useNavigate()
  const { data: templates, loading, error, reload } = useApi<Template[]>(() => api.listTemplates())
  const { data: projects } = useApi<any[]>(() => api.listProjects())
  const [showEditor, setShowEditor] = useState(false)
  const [editing, setEditing] = useState<Template | null>(null)
  const [form, setForm] = useState(emptyForm)
  const [saving, setSaving] = useState(false)
  const [formError, setFormError] = useState('')

  const openCreate = () => {
    setEditing(null)
    setForm(emptyForm)
    setFormError('')
    setShowEditor(true)
  }
  const openEdit = (template: Template) => {
    let config = template.config || templateDefault
    try { config = prettyConfigSourceSync(config) } catch { /* Keep legacy source editable. */ }
    setEditing(template)
    setForm({ name: template.name, description: template.description || '', config })
    setFormError('')
    setShowEditor(true)
  }
  const handleFormat = async () => {
    setFormError('')
    try {
      const config = await prettyConfigSource(form.config)
      setForm(current => ({ ...current, config }))
      dialogs.notify(t('config.formatted'), 'success')
    } catch (reason: any) {
      const message = reason.message || t('config.invalid')
      setFormError(message)
      dialogs.notify(t('config.invalid'))
    }
  }
  const handleSubmit = async (event: React.FormEvent) => {
    event.preventDefault()
    setSaving(true)
    setFormError('')
    try {
      const config = await prettyConfigSource(form.config)
      if (editing) await api.updateTemplate(editing.id, { ...form, config })
      else await api.createTemplate({ ...form, config })
      setShowEditor(false)
      reload()
    } catch (reason: any) {
      const message = reason.message || t('templates.saveFailed')
      setFormError(message)
      if (reason?.name !== 'ApiError') dialogs.notify(message)
    } finally {
      setSaving(false)
    }
  }
  const handleDelete = async (template: Template) => {
    if (!await dialogs.confirm(t('templates.deleteConfirm'), { title: t('templates.removeTitle'), action: t('common.delete') })) return
    try {
      await api.deleteTemplate(template.id)
      reload()
    } catch (reason: any) {
      dialogs.notify(reason.message || t('common.error'))
    }
  }

  if (loading) return <PageState />
  if (error) return <PageState error={error} onRetry={reload} />

  const list = templates || []
  const references = (projects || []).filter(project => project.template_id).length

  return (
    <motion.section className="operations-page templates-workbench" initial={{ opacity: 0, y: 8 }} animate={{ opacity: 1, y: 0 }} transition={{ duration: 0.24, ease: 'easeOut' }}>
      <header className="operations-heading">
        <div><p>{list.length} {t('templates.registered')}</p><h1>{t('templates.title')}</h1></div>
        {editable && <button className="primary-command" type="button" onClick={openCreate}><Plus size={16} />{t('templates.new')}</button>}
      </header>

      <section className="notification-summary templates-summary">
        <article><span><BookTemplate size={15} />{t('templates.total')}</span><strong>{list.length}</strong><small>{t('templates.totalHelp')}</small></article>
        <article><span><Boxes size={15} />{t('templates.projectsUsing')}</span><strong>{references}</strong><small>{t('templates.projectsUsingHelp')}</small></article>
        <article><span><Braces size={15} />{t('templates.formats')}</span><div className="template-formats"><span>JSON</span><span>YAML</span></div><small>{t('templates.formatsHelp')}</small></article>
      </section>

      <section className="operations-table-wrap templates-table-wrap">
        <table className="operations-table templates-table">
          <thead><tr><th>{t('templates.name')}</th><th>{t('templates.description')}</th><th>{t('templates.config')}</th><th>{t('templates.created')}</th><th aria-label={t('projects.actions')} /></tr></thead>
          <tbody>
            {!list.length && <tr><td colSpan={5} className="operations-empty"><BookTemplate size={18} />{t('templates.empty')}</td></tr>}
            {list.map(template => <tr key={template.id}>
              <td><button className="entity-link" type="button" disabled={!editable} onClick={() => openEdit(template)}><BookTemplate size={16} /><span><strong>{template.name}</strong><small>#{template.id}</small></span></button></td>
              <td className="muted-cell template-description">{template.description || '-'}</td>
              <td><span className="template-config-kind"><Braces size={13} />{template.config?.trimStart().startsWith('{') ? 'JSON' : 'YAML'}</span></td>
              <td className="muted-cell">{template.created_at ? new Date(template.created_at).toLocaleDateString() : '-'}</td>
              <td><div className="row-actions">{editable && <><button className="row-run" type="button" onClick={() => navigate(`/projects/new?template=${template.id}`)}><Play size={13} />{t('templates.use')}</button><button className="row-icon" type="button" title={t('templates.edit')} aria-label={t('templates.edit')} onClick={() => openEdit(template)}><Pencil size={15} /></button><button className="row-icon danger" type="button" title={t('common.delete')} aria-label={t('common.delete')} onClick={() => handleDelete(template)}><Trash2 size={15} /></button></>}</div></td>
            </tr>)}
          </tbody>
        </table>
      </section>

      {showEditor && <ModalDialog className="notification-editor template-editor" ariaLabel={editing ? t('templates.edit') : t('templates.new')} busy={saving} onClose={() => setShowEditor(false)}>
          <header><div><BookTemplate size={18} /><div><h2>{editing ? t('templates.edit') : t('templates.new')}</h2><p>{t('templates.editorHelp')}</p></div></div><button type="button" onClick={() => setShowEditor(false)} title={t('common.close')}><X size={18} /></button></header>
          <form className="notification-editor-form" onSubmit={handleSubmit}>
            <label>{t('templates.name')}<input required autoFocus data-dialog-initial-focus value={form.name} onChange={event => setForm({ ...form, name: event.target.value })} /></label>
            <label>{t('templates.description')}<input value={form.description} onChange={event => setForm({ ...form, description: event.target.value })} /></label>
            <div className="wide template-config-field"><div className="config-label-row"><label htmlFor="template-config">{t('templates.config')} (JSON/YAML)</label><button className="format-command" type="button" onClick={handleFormat}><Braces size={13} />{t('config.format')}</button></div><textarea id="template-config" className="code-input" rows={16} value={form.config} onChange={event => setForm({ ...form, config: event.target.value })} /><small>{t('templates.configHelp')}</small></div>
            {formError && <p className="form-error">{formError}</p>}
            <footer><button type="button" onClick={() => setShowEditor(false)}>{t('common.cancel')}</button><button type="submit" disabled={saving}>{saving ? t('common.loading') : t('common.save')}</button></footer>
          </form>
      </ModalDialog>}
    </motion.section>
  )
}
