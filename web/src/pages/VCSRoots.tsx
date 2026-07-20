import { useState } from 'react'
import { motion } from 'motion/react'
import { Braces, FolderGit2, GitBranch, KeyRound, Pencil, Plus, RefreshCw, Trash2, X } from 'lucide-react'
import { api } from '../api'
import { useApi } from '../hooks'
import { dialogs } from '../components/AppDialogs'
import { ModalDialog } from '../components/ModalDialog'
import { PageState } from '../components/PageState'
import { useI18n } from '../i18n'
import { prettyConfigSource, prettyConfigSourceSync } from '../lib/configFormat'
import { canEdit, isAdmin } from '../authz'

interface VCSRoot {
  id: number
  name: string
  type: string
  url: string
  branch: string
  credential_id?: number
  poll_interval: number
  auto_checkout: boolean
  config?: string
}

interface FormData {
  name: string
  type: string
  url: string
  branch: string
  credential_id: number | ''
  poll_interval: number
  auto_checkout: boolean
  config: string
}

const emptyForm: FormData = {
  name: '',
  type: 'git',
  url: '',
  branch: 'main',
  credential_id: '',
  poll_interval: 60,
  auto_checkout: true,
  config: '{}\n',
}

function rootTypeClass(type: string) {
  return `vcs-type ${type}`
}

export default function VCSRoots() {
  const { t } = useI18n()
  const editable = canEdit()
  const admin = isAdmin()
  const { data: roots, loading, error, reload } = useApi<VCSRoot[]>(() => api.listVCSRoots())
  const { data: credentials } = useApi<any[]>(() => admin ? api.listCredentials() : Promise.resolve([]), [admin])
  const [editing, setEditing] = useState<VCSRoot | null>(null)
  const [showEditor, setShowEditor] = useState(false)
  const [form, setForm] = useState<FormData>(emptyForm)
  const [saving, setSaving] = useState(false)
  const [formError, setFormError] = useState('')

  const openCreate = () => {
    setEditing(null)
    setForm(emptyForm)
    setFormError('')
    setShowEditor(true)
  }

  const openEdit = (root: VCSRoot) => {
    let config = root.config || '{}\n'
    try { config = prettyConfigSourceSync(config) } catch { /* Preserve invalid legacy source for repair. */ }
    setEditing(root)
    setForm({
      name: root.name,
      type: root.type || 'git',
      url: root.url || '',
      branch: root.branch || 'main',
      credential_id: root.credential_id || '',
      poll_interval: root.poll_interval || 0,
      auto_checkout: root.auto_checkout,
      config,
    })
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
      const data = {
        ...form,
        config,
        credential_id: form.credential_id === '' ? null : Number(form.credential_id),
        poll_interval: Math.max(0, form.poll_interval),
      }
      if (editing) await api.updateVCSRoot(editing.id, data)
      else await api.createVCSRoot(data)
      setShowEditor(false)
      reload()
    } catch (reason: any) {
      const message = reason.message || t('vcsRoots.saveFailed')
      setFormError(message)
      if (reason?.name !== 'ApiError') dialogs.notify(message)
    } finally {
      setSaving(false)
    }
  }

  const handleDelete = async (root: VCSRoot) => {
    if (!await dialogs.confirm(t('vcsRoots.deleteConfirm'), { title: t('vcsRoots.deleteTitle'), action: t('common.delete') })) return
    try {
      await api.deleteVCSRoot(root.id)
      reload()
    } catch (reason: any) {
      dialogs.notify(reason.message || t('common.error'))
    }
  }

  const getCredentialName = (credentialID?: number) => credentials?.find(credential => credential.id === credentialID)?.name || '-'

  if (loading) return <PageState />
  if (error) return <PageState error={error} onRetry={reload} />

  const list = roots || []
  const automatic = list.filter(root => root.auto_checkout).length
  const polling = list.filter(root => root.poll_interval > 0).length

  return (
    <motion.section className="operations-page vcs-workbench" initial={{ opacity: 0, y: 8 }} animate={{ opacity: 1, y: 0 }} transition={{ duration: 0.24, ease: 'easeOut' }}>
      <header className="operations-heading">
        <div><p>{list.length} {t('vcsRoots.registered')}</p><h1>{t('vcsRoots.title')}</h1></div>
        {editable && <button className="primary-command" type="button" onClick={openCreate}><Plus size={16} />{t('vcsRoots.new')}</button>}
      </header>

      <section className="notification-summary vcs-summary">
        <article><span><FolderGit2 size={15} />{t('vcsRoots.total')}</span><strong>{list.length}</strong><small>{t('vcsRoots.totalHelp')}</small></article>
        <article><span><GitBranch size={15} />{t('vcsRoots.automatic')}</span><strong className="positive">{automatic}</strong><small>{t('vcsRoots.automaticHelp')}</small></article>
        <article><span><RefreshCw size={15} />{t('vcsRoots.polling')}</span><strong>{polling}</strong><small>{t('vcsRoots.pollingHelp')}</small></article>
      </section>

      <section className="operations-table-wrap vcs-table-wrap">
        <table className="operations-table vcs-table">
          <thead><tr><th>{t('vcsRoots.name')}</th><th>{t('vcsRoots.type')}</th><th>{t('vcsRoots.url')}</th><th>{t('vcsRoots.branch')}</th><th>{t('vcsRoots.credential')}</th><th>{t('vcsRoots.pollInterval')}</th><th aria-label={t('projects.actions')} /></tr></thead>
          <tbody>
            {!list.length && <tr><td colSpan={7} className="operations-empty"><FolderGit2 size={18} />{t('vcsRoots.empty')}</td></tr>}
            {list.map(root => (
              <tr key={root.id}>
                <td><button className="entity-link" type="button" disabled={!editable} onClick={() => openEdit(root)}><FolderGit2 size={16} /><span><strong>{root.name}</strong><small>{root.auto_checkout ? t('vcsRoots.autoCheckout') : t('vcsRoots.manualCheckout')}</small></span></button></td>
                <td><span className={rootTypeClass(root.type)}>{root.type}</span></td>
                <td><code className="repo-cell" title={root.url}>{root.url || '-'}</code></td>
                <td><span className="branch-cell"><GitBranch size={13} />{root.branch || 'main'}</span></td>
                <td>{root.credential_id ? <span className="vcs-credential"><KeyRound size={13} />{getCredentialName(root.credential_id)}</span> : <span className="muted-cell">{t('vcsRoots.none')}</span>}</td>
                <td className="muted-cell">{root.poll_interval > 0 ? `${root.poll_interval}${t('vcsRoots.secondsShort')}` : t('vcsRoots.disabled')}</td>
                <td><div className="row-actions">{editable && <><button className="row-icon" type="button" title={t('vcsRoots.edit')} aria-label={t('vcsRoots.edit')} onClick={() => openEdit(root)}><Pencil size={15} /></button><button className="row-icon danger" type="button" title={t('common.delete')} aria-label={t('common.delete')} onClick={() => handleDelete(root)}><Trash2 size={15} /></button></>}</div></td>
              </tr>
            ))}
          </tbody>
        </table>
      </section>

      {showEditor && <ModalDialog className="notification-editor vcs-editor" ariaLabel={editing ? t('vcsRoots.edit') : t('vcsRoots.new')} busy={saving} onClose={() => setShowEditor(false)}>
          <header><div><FolderGit2 size={18} /><div><h2>{editing ? t('vcsRoots.edit') : t('vcsRoots.new')}</h2><p>{t('vcsRoots.editorHelp')}</p></div></div><button type="button" onClick={() => setShowEditor(false)} title={t('common.close')}><X size={18} /></button></header>
          <form className="notification-editor-form" onSubmit={handleSubmit}>
            <label>{t('vcsRoots.name')}<input required autoFocus data-dialog-initial-focus value={form.name} onChange={event => setForm({ ...form, name: event.target.value })} /></label>
            <label>{t('vcsRoots.type')}<select value={form.type} onChange={event => setForm({ ...form, type: event.target.value })}><option value="git">Git</option><option value="svn">Subversion</option><option value="hg">Mercurial</option></select></label>
            <label className="wide">{t('vcsRoots.url')}<input required placeholder="https://github.com/team/repository.git or git@github.com:team/repository.git" value={form.url} onChange={event => setForm({ ...form, url: event.target.value })} /></label>
            <label>{t('vcsRoots.branch')}<input value={form.branch} onChange={event => setForm({ ...form, branch: event.target.value })} /></label>
            <label>{t('vcsRoots.credential')}<select value={form.credential_id} onChange={event => setForm({ ...form, credential_id: event.target.value === '' ? '' : Number(event.target.value) })}><option value="">{t('vcsRoots.none')}</option>{(credentials || []).map(credential => <option key={credential.id} value={credential.id}>{credential.name}</option>)}</select></label>
            <label>{t('vcsRoots.pollInterval')}<input type="number" min={0} value={form.poll_interval} onChange={event => setForm({ ...form, poll_interval: Number(event.target.value) || 0 })} /></label>
            <label className="channel-enabled vcs-auto-checkout"><input type="checkbox" checked={form.auto_checkout} onChange={event => setForm({ ...form, auto_checkout: event.target.checked })} />{t('vcsRoots.autoCheckout')}</label>
            <div className="wide vcs-config-field"><div className="config-label-row"><label htmlFor="vcs-config">{t('vcsRoots.config')} (JSON/YAML)</label><button className="format-command" type="button" onClick={handleFormat}><Braces size={13} />{t('config.format')}</button></div><textarea id="vcs-config" className="code-input" rows={7} value={form.config} onChange={event => setForm({ ...form, config: event.target.value })} /></div>
            {formError && <p className="form-error">{formError}</p>}
            <footer><button type="button" onClick={() => setShowEditor(false)}>{t('common.cancel')}</button><button type="submit" disabled={saving}>{saving ? t('common.loading') : t('common.save')}</button></footer>
          </form>
      </ModalDialog>}
    </motion.section>
  )
}
