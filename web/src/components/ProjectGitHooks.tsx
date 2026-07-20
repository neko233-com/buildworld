import { useCallback, useEffect, useState } from 'react'
import { Braces, Copy, GitBranch, Pencil, Plus, Radio, Trash2, X } from 'lucide-react'
import { api } from '../api'
import { useI18n } from '../i18n'
import { dialogs } from './AppDialogs'
import { ModalDialog } from './ModalDialog'
import { prettyConfigSource } from '../lib/configFormat'
import { canEdit } from '../authz'

interface GitHook {
  id: number
  project_id: number
  name: string
  event: string
  branch: string
  secret: string
  enabled: boolean
  build_params: string
  description: string
  created_at: string
  updated_at: string
}

const EVENTS = ['push', 'tag_push', 'pull_request', 'merge_request']
const emptyForm = {
  name: '',
  event: 'push',
  branch: '*',
  secret: '',
  enabled: true,
  build_params: '{\n  \n}\n',
  description: '',
}

export default function ProjectGitHooks({ projectId }: { projectId: number }) {
  const { t } = useI18n()
  const editable = canEdit()
  const [hooks, setHooks] = useState<GitHook[]>([])
  const [loading, setLoading] = useState(true)
  const [editorOpen, setEditorOpen] = useState(false)
  const [editingId, setEditingId] = useState<number | null>(null)
  const [form, setForm] = useState(emptyForm)
  const [formError, setFormError] = useState('')
  const [saving, setSaving] = useState(false)

  const loadHooks = useCallback(async () => {
    setLoading(true)
    try {
      setHooks((await api.listGitHooks(projectId)) || [])
    } catch (reason: any) {
      dialogs.notify(reason.message || t('gitHooks.loadFailed'))
      setHooks([])
    } finally {
      setLoading(false)
    }
  }, [projectId, t])

  useEffect(() => { loadHooks() }, [loadHooks])

  const openCreate = () => {
    setEditingId(null)
    setForm(emptyForm)
    setFormError('')
    setEditorOpen(true)
  }

  const openEdit = (hook: GitHook) => {
    setEditingId(hook.id)
    setForm({
      name: hook.name,
      event: hook.event,
      branch: hook.branch || '*',
      secret: hook.secret || '',
      enabled: hook.enabled,
      build_params: hook.build_params || '{}',
      description: hook.description || '',
    })
    setFormError('')
    setEditorOpen(true)
  }

  const formatParams = async () => {
    try {
      const build_params = await prettyConfigSource(form.build_params || '{}')
      setForm(current => ({ ...current, build_params }))
      setFormError('')
      dialogs.notify(t('config.formatted'), 'success')
    } catch (reason: any) {
      const message = reason.message || t('config.invalid')
      setFormError(message)
      dialogs.notify(t('config.invalid'))
    }
  }

  const saveHook = async (event: React.FormEvent) => {
    event.preventDefault()
    setSaving(true)
    setFormError('')
    try {
      const build_params = await prettyConfigSource(form.build_params || '{}')
      const payload = { ...form, build_params }
      if (editingId) await api.updateGitHook(editingId, payload)
      else await api.createGitHook(projectId, payload)
      setEditorOpen(false)
      await loadHooks()
    } catch (reason: any) {
      const message = reason.message || t('gitHooks.saveFailed')
      setFormError(message)
      if (reason?.name !== 'ApiError') dialogs.notify(message)
    } finally {
      setSaving(false)
    }
  }

  const toggleHook = async (hook: GitHook) => {
    try {
      await api.updateGitHook(hook.id, { ...hook, enabled: !hook.enabled })
      await loadHooks()
    } catch (reason: any) {
      dialogs.notify(reason.message || t('gitHooks.saveFailed'))
    }
  }

  const deleteHook = async (hook: GitHook) => {
    if (!await dialogs.confirm(t('gitHooks.deleteConfirm'), { title: t('gitHooks.deleteTitle'), action: t('common.delete') })) return
    try {
      await api.deleteGitHook(hook.id)
      await loadHooks()
    } catch (reason: any) {
      dialogs.notify(reason.message || t('gitHooks.deleteFailed'))
    }
  }

  const webhookURL = `${window.location.origin}/api/webhooks/${projectId}`
  const copyWebhookURL = async () => {
    await navigator.clipboard.writeText(webhookURL)
    dialogs.notify(t('gitHooks.copied'), 'success')
  }

  return <section className="project-hook-settings">
    <header>
      <div><Radio size={17} /><div><h2>{t('gitHooks.projectTitle')}</h2><p>{t('gitHooks.projectHelp')}</p></div></div>
      {editable && <button type="button" className="secondary-command" onClick={openCreate}><Plus size={15} />{t('gitHooks.new')}</button>}
    </header>
    <div className="hook-webhook-url"><span>{t('gitHooks.webhookUrl')}</span><code>{webhookURL}</code><button type="button" className="secondary-command" onClick={copyWebhookURL}><Copy size={14} />{t('common.copy')}</button></div>
    <div className="operations-table-wrap project-hook-table-wrap">
      <table className="operations-table project-hook-table">
        <thead><tr><th>{t('gitHooks.name')}</th><th>{t('gitHooks.event')}</th><th>{t('gitHooks.branch')}</th><th>{t('common.enable')}</th><th aria-label={t('projects.actions')} /></tr></thead>
        <tbody>
          {!loading && !hooks.length && <tr><td colSpan={5} className="operations-empty"><GitBranch size={17} />{t('gitHooks.empty')}</td></tr>}
          {loading && <tr><td colSpan={5} className="operations-empty">{t('common.loading')}</td></tr>}
          {hooks.map(hook => <tr key={hook.id}>
            <td><strong>{hook.name}</strong>{hook.description && <small className="hook-description">{hook.description}</small>}</td>
            <td><span className="hook-event">{hook.event}</span></td>
            <td><code>{hook.branch || '*'}</code></td>
            <td><button type="button" disabled={!editable} className={`channel-toggle ${hook.enabled ? 'on' : ''}`} aria-pressed={hook.enabled} aria-label={t('gitHooks.toggle')} onClick={() => toggleHook(hook)}><i /></button></td>
            <td><div className="row-actions">{editable && <><button type="button" className="row-icon" title={t('projects.edit')} aria-label={t('projects.edit')} onClick={() => openEdit(hook)}><Pencil size={14} /></button><button type="button" className="row-icon danger" title={t('common.delete')} aria-label={t('common.delete')} onClick={() => deleteHook(hook)}><Trash2 size={14} /></button></>}</div></td>
          </tr>)}
        </tbody>
      </table>
    </div>

    {editorOpen && <ModalDialog className="hook-editor" ariaLabel={editingId ? t('gitHooks.edit') : t('gitHooks.new')} busy={saving} onClose={() => setEditorOpen(false)}>
      <header><div><GitBranch size={18} /><div><h2>{editingId ? t('gitHooks.edit') : t('gitHooks.new')}</h2><p>{t('gitHooks.editorHelp')}</p></div></div><button type="button" onClick={() => setEditorOpen(false)} title={t('common.close')} aria-label={t('common.close')}><X size={18} /></button></header>
      <form className="hook-editor-form" onSubmit={saveHook}>
        <label>{t('gitHooks.name')}<input required data-dialog-initial-focus value={form.name} onChange={event => setForm(current => ({ ...current, name: event.target.value }))} autoFocus /></label>
        <label>{t('gitHooks.event')}<select value={form.event} onChange={event => setForm(current => ({ ...current, event: event.target.value }))}>{EVENTS.map(value => <option key={value} value={value}>{value}</option>)}</select></label>
        <label>{t('gitHooks.branch')}<input value={form.branch} onChange={event => setForm(current => ({ ...current, branch: event.target.value }))} placeholder="*" /></label>
        <label>{t('gitHooks.secret')}<input value={form.secret} onChange={event => setForm(current => ({ ...current, secret: event.target.value }))} autoComplete="off" /></label>
        <label className="wide">{t('gitHooks.description')}<input value={form.description} onChange={event => setForm(current => ({ ...current, description: event.target.value }))} /></label>
        <label className="wide config-editor-label"><span>{t('gitHooks.params')}<button type="button" className="format-command" onClick={formatParams}><Braces size={13} />{t('config.format')}</button></span><textarea className="code-input" value={form.build_params} onChange={event => setForm(current => ({ ...current, build_params: event.target.value }))} rows={8} spellCheck="false" /></label>
        <label className="hook-enabled"><input type="checkbox" checked={form.enabled} onChange={event => setForm(current => ({ ...current, enabled: event.target.checked }))} />{t('common.enable')}</label>
        {formError && <p className="form-error">{formError}</p>}
        <footer><button type="button" onClick={() => setEditorOpen(false)}>{t('common.cancel')}</button><button type="submit" disabled={saving}>{saving ? t('common.loading') : t('common.save')}</button></footer>
      </form>
    </ModalDialog>}
  </section>
}
