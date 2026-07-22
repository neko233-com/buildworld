import { useState } from 'react'
import { Copy, Eye, EyeOff, KeyRound, LoaderCircle, Pencil, Plus, Server, ShieldCheck, Trash2, X } from 'lucide-react'
import { useApi } from '../hooks'
import { api } from '../api'
import { dialogs } from '../components/AppDialogs'
import { ModalDialog } from '../components/ModalDialog'
import { JenkinsHeaderBreadcrumb } from '../components/JenkinsPageShell'
import { PageState } from '../components/PageState'
import { useI18n } from '../i18n'
import { formatDateTime } from '../lib/dateTime'
import './ManagementPages.jenkins.css'

type CredentialType = 'ssh_key' | 'git'
interface Credential {
  id: number
  name: string
  type: CredentialType
  host: string
  username: string
  password?: string
  private_key?: string
  public_key?: string
  token?: string
  description?: string
  is_secret: boolean
  updated_at?: string
}
interface FormData {
  name: string
  type: CredentialType
  host: string
  username: string
  password: string
  private_key: string
  public_key: string
  token: string
  description: string
  is_secret: boolean
}

const credentialTypes: Array<{ value: CredentialType; label: string }> = [
  { value: 'ssh_key', label: 'SSH Key' },
  { value: 'git', label: 'Git' },
]
const emptyForm: FormData = {
  name: '', type: 'ssh_key', host: '', username: '',
  password: '', private_key: '', public_key: '', token: '', description: '', is_secret: true,
}

function typeLabel(type: CredentialType) {
  return credentialTypes.find(item => item.value === type)?.label || type
}
function maskedSecret(credential: Credential) {
  return credential.password || credential.private_key || credential.token || ''
}

export default function Credentials() {
  const { t } = useI18n()
  const [filterType, setFilterType] = useState('')
  const { data: credentials, loading, error, reload } = useApi<Credential[]>(() => api.listCredentials(filterType || undefined), [filterType])
  const [showEditor, setShowEditor] = useState(false)
  const [editing, setEditing] = useState<Credential | null>(null)
  const [form, setForm] = useState<FormData>(emptyForm)
  const [saving, setSaving] = useState(false)
  const [deletingID, setDeletingID] = useState<number | null>(null)
  const [revealingID, setRevealingID] = useState<number | null>(null)
  const [formError, setFormError] = useState('')
  const [revealed, setRevealed] = useState<Record<number, string>>({})
  const breadcrumb = <JenkinsHeaderBreadcrumb breadcrumbs={[{ label: t('credentials.title') }]} />

  const openCreate = () => {
    setEditing(null)
    setForm(emptyForm)
    setFormError('')
    setShowEditor(true)
  }
  const openEdit = (credential: Credential) => {
    setEditing(credential)
    setForm({
      name: credential.name,
      type: credential.type,
      host: credential.host || '',
      username: credential.username || '',
      password: credential.password || '',
      private_key: credential.private_key || '',
      public_key: credential.public_key || '',
      token: credential.token || '',
      description: credential.description || '',
      is_secret: credential.is_secret,
    })
    setFormError('')
    setShowEditor(true)
  }
  const handleSubmit = async (event: React.FormEvent) => {
    event.preventDefault()
    if (saving) return
    setSaving(true)
    setFormError('')
    try {
      if (editing) await api.updateCredential(editing.id, form)
      else await api.createCredential(form)
      setShowEditor(false)
      setRevealed({})
      reload()
    } catch (reason: any) {
      setFormError(reason.message || t('credentials.saveFailed'))
    } finally {
      setSaving(false)
    }
  }
  const handleDelete = async (credential: Credential) => {
    if (deletingID !== null) return
    if (!await dialogs.confirm(t('credentials.deleteConfirmNamed').replace('{name}', credential.name), { title: t('credentials.deleteTitle'), action: t('common.delete') })) return
    setDeletingID(credential.id)
    try {
      await api.deleteCredential(credential.id)
      setRevealed(current => {
        const next = { ...current }
        delete next[credential.id]
        return next
      })
      reload()
    } catch (reason: any) {
      dialogs.notify(reason.message || t('common.error'))
    } finally {
      setDeletingID(null)
    }
  }
  const toggleReveal = async (credential: Credential) => {
    if (revealingID !== null || deletingID !== null) return
    if (revealed[credential.id] !== undefined) {
      setRevealed(current => {
        const next = { ...current }
        delete next[credential.id]
        return next
      })
      return
    }
    setRevealingID(credential.id)
    try {
      const full = await api.getCredential(credential.id, true)
      setRevealed(current => ({ ...current, [credential.id]: full.password || full.private_key || full.token || '' }))
    } catch (reason: any) {
      dialogs.notify(reason.message || t('credentials.revealFailed'))
    } finally {
      setRevealingID(null)
    }
  }
  const copySecret = async (secret: string) => {
    try {
      await navigator.clipboard.writeText(secret)
      dialogs.notify(t('credentials.copied'), 'success')
    } catch {
      dialogs.notify(t('common.copyFailed'))
    }
  }

  if (loading) return <>{breadcrumb}<section className="jenkins-management-page"><PageState /></section></>
  if (error) return <>{breadcrumb}<section className="jenkins-management-page"><PageState error={error} onRetry={reload} /></section></>

  const list = credentials || []
  const hosts = new Set(list.map(credential => credential.host).filter(Boolean)).size
  const sshKeys = list.filter(credential => credential.type === 'ssh_key').length

  return <>{breadcrumb}
    <section className="operations-page credential-workbench jenkins-management-page">
      <header className="operations-heading">
        <div><p>{list.length} {t('credentials.registered')}</p><h1>{t('credentials.title')}</h1></div>
        <div className="credential-heading-actions"><select className="operations-filter" disabled={deletingID !== null} aria-label={t('credentials.filter')} value={filterType} onChange={event => setFilterType(event.target.value)}><option value="">{t('credentials.allTypes')}</option>{credentialTypes.map(type => <option key={type.value} value={type.value}>{type.label}</option>)}</select><button className="primary-command" type="button" disabled={deletingID !== null} onClick={openCreate}><Plus size={16} />{t('credentials.new')}</button></div>
      </header>

      <section className="notification-summary credential-summary">
        <article><span><KeyRound size={15} />{t('credentials.total')}</span><strong>{list.length}</strong><small>{t('credentials.totalHelp')}</small></article>
        <article><span><Server size={15} />{t('credentials.hosts')}</span><strong>{hosts}</strong><small>{t('credentials.hostsHelp')}</small></article>
        <article><span><ShieldCheck size={15} />{t('credentials.sshKeys')}</span><strong>{sshKeys}</strong><small>{t('credentials.sshKeysHelp')}</small></article>
      </section>

      <section className="operations-table-wrap credential-table-wrap">
        <table className="operations-table credential-table">
          <thead><tr><th>{t('credentials.name')}</th><th>{t('credentials.type')}</th><th>{t('credentials.host')}</th><th>{t('credentials.username')}</th><th>{t('credentials.secret')}</th><th>{t('credentials.updated')}</th><th aria-label={t('projects.actions')} /></tr></thead>
          <tbody>
            {!list.length && <tr><td colSpan={7} className="operations-empty"><KeyRound size={18} />{t('credentials.empty')}</td></tr>}
            {list.map(credential => {
              const hasSecret = Boolean(maskedSecret(credential))
              const secret = revealed[credential.id]
              const deleting = deletingID === credential.id
              const revealing = revealingID === credential.id
              return <tr key={credential.id} aria-busy={deleting || revealing || undefined}>
                <td><button className="entity-link" type="button" disabled={deletingID !== null} onClick={() => openEdit(credential)}><KeyRound size={16} /><span><strong>{credential.name}</strong><small>{credential.description || t('credentials.noDescription')}</small></span></button></td>
                <td><span className={`credential-type ${credential.type}`}>{typeLabel(credential.type)}</span></td>
                <td><code className="credential-host">{credential.host || '*'}</code></td>
                <td className="muted-cell">{credential.username || '-'}</td>
                <td>{!hasSecret ? <span className="muted-cell">{t('credentials.noSecret')}</span> : <div className="credential-secret">{secret !== undefined ? <code>{secret || t('credentials.emptySecret')}</code> : <span>••••••••</span>}<button className="row-icon" type="button" disabled={revealingID !== null || deletingID !== null} aria-busy={revealing || undefined} title={secret !== undefined ? t('credentials.hide') : t('credentials.reveal')} aria-label={`${secret !== undefined ? t('credentials.hide') : t('credentials.reveal')}: ${credential.name}`} onClick={() => void toggleReveal(credential)}>{revealing ? <LoaderCircle className="timeline-spinner" size={14} /> : secret !== undefined ? <EyeOff size={14} /> : <Eye size={14} />}</button>{secret !== undefined && secret && <button className="row-icon" type="button" disabled={deletingID !== null} title={t('common.copy')} aria-label={`${t('common.copy')}: ${credential.name}`} onClick={() => void copySecret(secret)}><Copy size={14} /></button>}</div>}</td>
                <td className="muted-cell">{formatDateTime(credential.updated_at)}</td>
                <td><div className="row-actions"><button className="row-icon" type="button" disabled={deletingID !== null} title={t('credentials.edit')} aria-label={`${t('credentials.edit')}: ${credential.name}`} onClick={() => openEdit(credential)}><Pencil size={15} /></button><button className="row-icon danger" type="button" disabled={deletingID !== null || revealingID !== null} aria-busy={deleting || undefined} title={t('common.delete')} aria-label={`${t('common.delete')}: ${credential.name}`} onClick={() => void handleDelete(credential)}>{deleting ? <LoaderCircle className="timeline-spinner" size={15} /> : <Trash2 size={15} />}</button></div></td>
              </tr>
            })}
          </tbody>
        </table>
      </section>

      {showEditor && <ModalDialog className="notification-editor credential-editor" ariaLabel={editing ? t('credentials.edit') : t('credentials.new')} busy={saving} onClose={() => setShowEditor(false)}>
          <header><div><KeyRound size={18} /><div><h2>{editing ? t('credentials.edit') : t('credentials.new')}</h2><p>{t('credentials.editorHelp')}</p></div></div><button type="button" disabled={saving} onClick={() => setShowEditor(false)} title={t('common.close')}><X size={18} /></button></header>
          <form className="notification-editor-form" onSubmit={handleSubmit}>
            <label>{t('credentials.name')}<input required autoFocus data-dialog-initial-focus value={form.name} onChange={event => setForm({ ...form, name: event.target.value })} /></label>
            <label>{t('credentials.type')}<select value={form.type} onChange={event => setForm({ ...form, type: event.target.value as CredentialType })}>{credentialTypes.map(type => <option key={type.value} value={type.value}>{type.label}</option>)}</select></label>
            <label>{t('credentials.host')}<input placeholder="github.com" value={form.host} onChange={event => setForm({ ...form, host: event.target.value })} /></label>
            <label>{t('credentials.username')}<input autoComplete="off" value={form.username} onChange={event => setForm({ ...form, username: event.target.value })} /></label>
            {form.type === 'ssh_key' ? <>
              <label className="wide">{t('credentials.privateKey')}<textarea className="credential-key-input" rows={6} placeholder="-----BEGIN OPENSSH PRIVATE KEY-----" value={form.private_key} onChange={event => setForm({ ...form, private_key: event.target.value })} /></label>
              <label className="wide">{t('credentials.publicKey')}<textarea className="credential-key-input" rows={3} placeholder="ssh-ed25519 AAAA..." value={form.public_key} onChange={event => setForm({ ...form, public_key: event.target.value })} /></label>
            </> : <>
              <label>{t('credentials.password')}<input type="password" autoComplete="new-password" value={form.password} onChange={event => setForm({ ...form, password: event.target.value })} /></label>
              <label>{t('credentials.token')}<input type="password" autoComplete="new-password" value={form.token} onChange={event => setForm({ ...form, token: event.target.value })} /></label>
            </>}
            <label className="wide">{t('credentials.description')}<input value={form.description} onChange={event => setForm({ ...form, description: event.target.value })} /></label>
            {formError && <p className="form-error" role="alert">{formError}</p>}
            <footer><button type="button" disabled={saving} onClick={() => setShowEditor(false)}>{t('common.cancel')}</button><button type="submit" disabled={saving}>{saving ? <><LoaderCircle className="timeline-spinner" size={14} />{t('common.loading')}</> : t('common.save')}</button></footer>
          </form>
      </ModalDialog>}
    </section>
  </>
}
