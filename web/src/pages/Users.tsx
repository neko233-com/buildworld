import { useState } from 'react'
import { KeyRound, Pencil, Plus, ShieldCheck, Trash2, UserRound, UsersRound, X } from 'lucide-react'
import { useI18n } from '../i18n'
import { api } from '../api'
import { useApi } from '../hooks'
import { dialogs } from '../components/AppDialogs'
import { JenkinsHeaderBreadcrumb } from '../components/JenkinsPageShell'
import { ModalDialog } from '../components/ModalDialog'
import { PageState } from '../components/PageState'
import { formatDateTimeWithWeekday } from '../lib/dateTime'
import './ManagementPages.jenkins.css'

interface User {
  id: number
  username: string
  email: string
  role: 'admin' | 'developer' | 'viewer'
  created_at?: string
  last_login?: string
}

const roles: User['role'][] = ['admin', 'developer', 'viewer']
const emptyForm = { username: '', email: '', password: '', role: 'viewer' as User['role'] }
function hasLoggedIn(value?: string) {
  if (!value) return false
  const date = new Date(value)
  return Number.isFinite(date.getTime()) && date.getFullYear() > 1970
}

export default function Users() {
  const { t } = useI18n()
  const breadcrumb = <JenkinsHeaderBreadcrumb breadcrumbs={[{ label: t('users.title') }]} />
  const { data: users, loading, error, reload } = useApi<User[]>(() => api.listUsers())
  const { data: me } = useApi<User>(() => api.me())
  const [showEditor, setShowEditor] = useState(false)
  const [editing, setEditing] = useState<User | null>(null)
  const [form, setForm] = useState(emptyForm)
  const [saving, setSaving] = useState(false)
  const [formError, setFormError] = useState('')

  const openCreate = () => {
    setEditing(null)
    setForm(emptyForm)
    setFormError('')
    setShowEditor(true)
  }
  const openEdit = (user: User) => {
    setEditing(user)
    setForm({ username: user.username, email: user.email, password: '', role: user.role })
    setFormError('')
    setShowEditor(true)
  }
  const handleSubmit = async (event: React.FormEvent) => {
    event.preventDefault()
    setSaving(true)
    setFormError('')
    try {
      if (editing) {
        if (editing.id !== me?.id && form.role !== editing.role) await api.updateUserRole(editing.id, form.role)
        if (form.password) await api.updateUserPassword(editing.id, form.password)
      } else {
        await api.createUser(form)
      }
      setShowEditor(false)
      reload()
    } catch (reason: any) {
      setFormError(reason.message || t('users.saveFailed'))
    } finally {
      setSaving(false)
    }
  }
  const handleDelete = async (user: User) => {
    if (me?.id === user.id) {
      dialogs.notify(t('users.cannotDeleteSelf'), 'info')
      return
    }
    if (!await dialogs.confirm(t('users.deleteConfirm'), { title: t('users.deleteTitle'), action: t('common.delete') })) return
    try {
      await api.deleteUser(user.id)
      reload()
    } catch (reason: any) {
      dialogs.notify(reason.message || t('common.error'))
    }
  }

  if (loading) return <>{breadcrumb}<section className="jenkins-management-page"><PageState /></section></>
  if (error) return <>{breadcrumb}<section className="jenkins-management-page"><PageState error={error} onRetry={reload} /></section></>

  const list = users || []
  const admins = list.filter(user => user.role === 'admin').length
  const active = list.filter(user => hasLoggedIn(user.last_login)).length

  return (
    <>{breadcrumb}<section className="operations-page users-workbench jenkins-management-page">
      <header className="operations-heading">
        <div><p>{list.length} {t('users.accounts')}</p><h1>{t('users.title')}</h1></div>
        <button className="primary-command" type="button" onClick={openCreate}><Plus size={16} />{t('users.addUser')}</button>
      </header>

      <section className="notification-summary users-summary">
        <article><span><UsersRound size={15} />{t('users.total')}</span><strong>{list.length}</strong><small>{t('users.totalHelp')}</small></article>
        <article><span><ShieldCheck size={15} />{t('users.admins')}</span><strong>{admins}</strong><small>{t('users.adminsHelp')}</small></article>
        <article><span><UserRound size={15} />{t('users.signedIn')}</span><strong>{active}</strong><small>{t('users.signedInHelp')}</small></article>
      </section>

      <section className="operations-table-wrap users-table-wrap">
        <table className="operations-table users-table">
          <thead><tr><th>{t('users.username')}</th><th>{t('users.email')}</th><th>{t('users.role')}</th><th>{t('users.lastLogin')}</th><th>{t('users.created')}</th><th aria-label={t('users.actions')} /></tr></thead>
          <tbody>
            {!list.length && <tr><td colSpan={6} className="operations-empty"><UsersRound size={18} />{t('users.empty')}</td></tr>}
            {list.map(user => <tr key={user.id}>
              <td><button className="entity-link" type="button" onClick={() => openEdit(user)}><UserRound size={16} /><span><strong>{user.username}{me?.id === user.id && <em>{t('users.you')}</em>}</strong><small>#{user.id}</small></span></button></td>
              <td className="muted-cell">{user.email || '-'}</td>
              <td><span className={`user-role ${user.role}`}>{t(`users.role_${user.role}`)}</span></td>
              <td className="muted-cell">{hasLoggedIn(user.last_login) ? formatDateTimeWithWeekday(user.last_login) : t('users.never')}</td>
              <td className="muted-cell">{formatDateTimeWithWeekday(user.created_at)}</td>
              <td><div className="row-actions"><button className="row-icon" type="button" title={t('users.edit')} aria-label={t('users.edit')} onClick={() => openEdit(user)}><Pencil size={15} /></button><button className="row-icon danger" type="button" disabled={me?.id === user.id} title={me?.id === user.id ? t('users.cannotDeleteSelf') : t('common.delete')} aria-label={t('common.delete')} onClick={() => handleDelete(user)}><Trash2 size={15} /></button></div></td>
            </tr>)}
          </tbody>
        </table>
      </section>

      {showEditor && <ModalDialog className="notification-editor user-editor" ariaLabel={editing ? t('users.edit') : t('users.addUser')} busy={saving} onClose={() => setShowEditor(false)}>
          <header><div><UserRound size={18} /><div><h2>{editing ? t('users.edit') : t('users.addUser')}</h2><p>{editing ? t('users.editHelp') : t('users.createHelp')}</p></div></div><button type="button" onClick={() => setShowEditor(false)} title={t('common.close')}><X size={18} /></button></header>
          <form className="notification-editor-form" onSubmit={handleSubmit}>
            <label>{t('users.username')}<input required data-dialog-initial-focus readOnly={Boolean(editing)} value={form.username} onChange={event => setForm({ ...form, username: event.target.value })} /></label>
            <label>{t('users.email')}<input required={!editing} readOnly={Boolean(editing)} type="email" value={form.email} onChange={event => setForm({ ...form, email: event.target.value })} /></label>
            <label>{t('users.role')}<select disabled={editing?.id === me?.id} value={form.role} onChange={event => setForm({ ...form, role: event.target.value as User['role'] })}>{roles.map(role => <option key={role} value={role}>{t(`users.role_${role}`)}</option>)}</select>{editing?.id === me?.id && <small>{t('users.selfRoleHelp')}</small>}</label>
            <label>{editing ? t('users.newPassword') : t('users.password')}<input required={!editing} minLength={6} type="password" autoComplete="new-password" value={form.password} onChange={event => setForm({ ...form, password: event.target.value })} />{editing && <small>{t('users.passwordHelp')}</small>}</label>
            {formError && <p className="form-error">{formError}</p>}
            <footer><button type="button" onClick={() => setShowEditor(false)}>{t('common.cancel')}</button><button type="submit" disabled={saving}><KeyRound size={14} />{saving ? t('common.loading') : t('common.save')}</button></footer>
          </form>
      </ModalDialog>}
    </section></>
  )
}
