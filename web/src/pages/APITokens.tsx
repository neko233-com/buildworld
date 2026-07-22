import { useState } from 'react'
import { Check, Clock3, Copy, KeyRound, LoaderCircle, Plus, ShieldCheck, Trash2, X } from 'lucide-react'
import { useI18n } from '../i18n'
import { api } from '../api'
import { useApi } from '../hooks'
import { dialogs } from '../components/AppDialogs'
import { ModalDialog } from '../components/ModalDialog'
import { JenkinsHeaderBreadcrumb } from '../components/JenkinsPageShell'
import { PageState } from '../components/PageState'
import './ManagementPages.jenkins.css'

interface APIToken {
  id: number
  name: string
  token_prefix: string
  scopes: string | string[]
  expires_at?: string
  last_used_at?: string
  created_at?: string
}

function tokenScopes(token: APIToken) {
  if (Array.isArray(token.scopes)) return token.scopes
  if (!token.scopes) return []
  try {
    const parsed = JSON.parse(token.scopes)
    return Array.isArray(parsed) ? parsed.map(String) : []
  } catch {
    return token.scopes.split(',').map(scope => scope.trim()).filter(Boolean)
  }
}

function isExpired(token: APIToken) {
  return Boolean(token.expires_at && new Date(token.expires_at).getTime() <= Date.now())
}

export default function APITokens() {
  const { t } = useI18n()
  const { data: tokens, loading, error, reload } = useApi<APIToken[]>(() => api.listAPITokens(), [])
  const [showEditor, setShowEditor] = useState(false)
  const [name, setName] = useState('')
  const [scopes, setScopes] = useState('')
  const [expires, setExpires] = useState('')
  const [creating, setCreating] = useState(false)
  const [deletingID, setDeletingID] = useState<number | null>(null)
  const [formError, setFormError] = useState('')
  const [newToken, setNewToken] = useState('')
  const [copied, setCopied] = useState(false)
  const breadcrumb = <JenkinsHeaderBreadcrumb breadcrumbs={[{ label: t('apiTokens.title') }]} />

  const openCreate = () => {
    setName('')
    setScopes('')
    setExpires('')
    setFormError('')
    setShowEditor(true)
  }

  const handleCreate = async (event: React.FormEvent) => {
    event.preventDefault()
    if (creating) return
    setCreating(true)
    setFormError('')
    try {
      const payload: { name: string; scopes?: string[]; expires_at?: string } = { name: name.trim() }
      const parsedScopes = scopes.split(',').map(scope => scope.trim()).filter(Boolean)
      if (parsedScopes.length) payload.scopes = parsedScopes
      if (expires) payload.expires_at = new Date(expires).toISOString()
      const response = await api.createAPIToken(payload)
      setNewToken(response.token || '')
      setCopied(false)
      setShowEditor(false)
      reload()
    } catch (reason: any) {
      setFormError(reason.message || t('common.error'))
    } finally {
      setCreating(false)
    }
  }

  const handleDelete = async (token: APIToken) => {
    if (deletingID !== null) return
    if (!await dialogs.confirm(t('apiTokens.removeConfirmNamed').replace('{name}', token.name), {
      title: t('apiTokens.removeTitle'),
      action: t('apiTokens.remove'),
    })) return
    setDeletingID(token.id)
    try {
      await api.deleteAPIToken(token.id)
      setNewToken('')
      setCopied(false)
      reload()
    } catch (reason: any) {
      dialogs.notify(reason.message || t('common.error'))
    } finally {
      setDeletingID(null)
    }
  }

  const handleCopy = async () => {
    try {
      if (!navigator.clipboard) throw new Error()
      await navigator.clipboard.writeText(newToken)
      setCopied(true)
      dialogs.notify(t('apiTokens.copied'), 'success')
    } catch {
      dialogs.notify(t('common.copyFailed'))
    }
  }

  if (loading) return <>{breadcrumb}<section className="jenkins-management-page"><PageState /></section></>
  if (error) return <>{breadcrumb}<section className="jenkins-management-page"><PageState error={error} onRetry={reload} /></section></>

  const list = tokens || []
  const active = list.filter(token => !isExpired(token)).length
  const used = list.filter(token => token.last_used_at).length

  return <>{breadcrumb}
    <section className="operations-page api-token-workbench jenkins-management-page">
      <header className="operations-heading">
        <div><p>{list.length} {t('apiTokens.registered')}</p><h1>{t('apiTokens.title')}</h1></div>
        <button className="primary-command" type="button" disabled={deletingID !== null} onClick={openCreate}><Plus size={16} />{t('apiTokens.new')}</button>
      </header>

      <section className="notification-summary api-token-summary">
        <article><span><KeyRound size={15} />{t('apiTokens.total')}</span><strong>{list.length}</strong><small>{t('apiTokens.totalHelp')}</small></article>
        <article><span><ShieldCheck size={15} />{t('apiTokens.active')}</span><strong>{active}</strong><small>{t('apiTokens.activeHelp')}</small></article>
        <article><span><Clock3 size={15} />{t('apiTokens.used')}</span><strong>{used}</strong><small>{t('apiTokens.usedHelp')}</small></article>
      </section>

      {newToken && <section className="api-token-once" aria-live="polite">
        <div><ShieldCheck size={18} /><span><strong>{t('apiTokens.created')}</strong><small>{t('apiTokens.tokenOnce')}</small></span></div>
        <code>{newToken}</code>
        <div className="api-token-once-actions">
          <button className="secondary-command" type="button" onClick={handleCopy}>{copied ? <Check size={14} /> : <Copy size={14} />}{copied ? t('apiTokens.copied') : t('common.copy')}</button>
          <button className="row-icon" type="button" onClick={() => setNewToken('')} title={t('common.dismiss')} aria-label={t('common.dismiss')}><X size={16} /></button>
        </div>
      </section>}

      <section className="operations-table-wrap api-token-table-wrap">
        <table className="operations-table api-token-table">
          <thead><tr><th>{t('apiTokens.name')}</th><th>{t('apiTokens.prefix')}</th><th>{t('apiTokens.scopes')}</th><th>{t('apiTokens.status')}</th><th>{t('apiTokens.lastUsed')}</th><th>{t('apiTokens.createdAt')}</th><th aria-label={t('projects.actions')} /></tr></thead>
          <tbody>
            {!list.length && <tr><td colSpan={7} className="operations-empty"><KeyRound size={18} />{t('apiTokens.empty')}</td></tr>}
            {list.map(token => {
              const parsedScopes = tokenScopes(token)
              const expired = isExpired(token)
              const deleting = deletingID === token.id
              return <tr key={token.id} aria-busy={deleting || undefined}>
                <td><span className="api-token-name"><KeyRound size={15} /><strong>{token.name}</strong></span></td>
                <td><code className="api-token-prefix">{token.token_prefix || '-'}{token.token_prefix ? '…' : ''}</code></td>
                <td><span className="api-token-scopes">{parsedScopes.length ? parsedScopes.map(scope => <span key={scope}>{scope}</span>) : <span>{t('apiTokens.allScopes')}</span>}</span></td>
                <td><span className={`api-token-status ${expired ? 'expired' : 'active'}`}>{expired ? t('apiTokens.expired') : token.expires_at ? `${t('apiTokens.activeUntil')} ${new Date(token.expires_at).toLocaleDateString()}` : t('apiTokens.noExpiry')}</span></td>
                <td className="muted-cell">{token.last_used_at ? new Date(token.last_used_at).toLocaleString() : t('apiTokens.neverUsed')}</td>
                <td className="muted-cell">{token.created_at ? new Date(token.created_at).toLocaleString() : '-'}</td>
                <td><button className="row-icon danger" type="button" disabled={deletingID !== null} aria-busy={deleting || undefined} onClick={() => void handleDelete(token)} title={t('apiTokens.remove')} aria-label={`${t('apiTokens.remove')}: ${token.name}`}>{deleting ? <LoaderCircle className="timeline-spinner" size={15} /> : <Trash2 size={15} />}</button></td>
              </tr>
            })}
          </tbody>
        </table>
      </section>

      {showEditor && <ModalDialog className="notification-editor api-token-editor" ariaLabel={t('apiTokens.new')} busy={creating} onClose={() => setShowEditor(false)}>
          <header><div><KeyRound size={18} /><div><h2>{t('apiTokens.new')}</h2><p>{t('apiTokens.editorHelp')}</p></div></div><button type="button" disabled={creating} onClick={() => setShowEditor(false)} title={t('common.close')}><X size={18} /></button></header>
          <form className="notification-editor-form" onSubmit={handleCreate}>
            <label className="wide">{t('apiTokens.name')}<input required autoFocus data-dialog-initial-focus value={name} onChange={event => setName(event.target.value)} placeholder={t('apiTokens.namePlaceholder')} /></label>
            <label className="wide">{t('apiTokens.scopes')}<input value={scopes} onChange={event => setScopes(event.target.value)} placeholder="build:trigger" /><small>{t('apiTokens.scopesHelp')}</small></label>
            <label className="wide">{t('apiTokens.expiresAt')}<input type="datetime-local" value={expires} onChange={event => setExpires(event.target.value)} /><small>{t('apiTokens.expiryHelp')}</small></label>
            {formError && <p className="form-error" role="alert">{formError}</p>}
            <footer><button type="button" disabled={creating} onClick={() => setShowEditor(false)}>{t('common.cancel')}</button><button type="submit" disabled={creating || !name.trim()}>{creating ? <><LoaderCircle className="timeline-spinner" size={14} />{t('common.loading')}</> : t('apiTokens.create')}</button></footer>
          </form>
      </ModalDialog>}
    </section>
  </>
}
