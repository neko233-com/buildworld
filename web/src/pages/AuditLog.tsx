import { useMemo, useState, type FormEvent } from 'react'
import { Activity, ChevronLeft, ChevronRight, ClipboardList, RefreshCw, Search, UserRound } from 'lucide-react'
import { useI18n } from '../i18n'
import { PageState } from '../components/PageState'
import { JenkinsHeaderBreadcrumb } from '../components/JenkinsPageShell'
import { api } from '../api'
import { useApi } from '../hooks'
import { formatDateTime } from '../lib/dateTime'
import './ManagementPages.jenkins.css'

const PAGE_SIZE = 50
const EMPTY_ROWS: AuditEntry[] = []

interface AuditEntry {
  id: number
  user_id?: number
  username?: string
  user?: string
  action: string
  resource_type?: string
  resource_id?: string
  detail?: string
  details?: string | Record<string, unknown>
  ip?: string
  created_at?: string
}

function entryDetail(entry: AuditEntry) {
  const value = entry.detail ?? entry.details
  if (!value) return '-'
  return typeof value === 'string' ? value : JSON.stringify(value)
}

export default function AuditLog() {
  const { t } = useI18n()
  const breadcrumb = <JenkinsHeaderBreadcrumb breadcrumbs={[{ label: t('auditLog.title') }]} />
  const [actionFilter, setActionFilter] = useState('')
  const [userFilter, setUserFilter] = useState('')
  const [applied, setApplied] = useState({ action: '', actor: '' })
  const [refreshing, setRefreshing] = useState(false)
  const [cursors, setCursors] = useState<Array<number | undefined>>([undefined])
  const beforeID = cursors.at(-1)
  const { data: logs, loading, error, reload } = useApi<AuditEntry[]>(
    () => api.listAuditLogs({ limit: PAGE_SIZE + 1, beforeID, ...applied }),
    [beforeID, applied.action, applied.actor],
  )
  const rows = useMemo(() => logs?.slice(0, PAGE_SIZE) || EMPTY_ROWS, [logs])
  const pageLabel = t('auditLog.page').replace('{page}', String(cursors.length))
  const { actions, actors } = useMemo(() => ({
    actions: [...new Set(rows.map(entry => entry.action).filter(Boolean))].sort(),
    actors: new Set(rows.map(entry => entry.username || entry.user || entry.user_id).filter(Boolean)).size,
  }), [rows])
  const hasNext = (logs?.length ?? 0) > PAGE_SIZE
  const busy = loading || refreshing
  const refresh = async () => {
    if (busy) return
    setRefreshing(true)
    try {
      await reload()
    } finally {
      setRefreshing(false)
    }
  }

  const search = (event: FormEvent<HTMLFormElement>) => {
    event.preventDefault()
    if (busy) return
    const action = actionFilter.trim()
    const actor = userFilter.trim()
    if (!beforeID && action === applied.action && actor === applied.actor) {
      void refresh()
    } else {
      setApplied({ action, actor })
      setCursors([undefined])
    }
  }

  return (
    <>{breadcrumb}<section className="operations-page audit-workbench jenkins-management-page">
      <header className="operations-heading">
        <div><p>{pageLabel}{logs && ` · ${rows.length} ${t('auditLog.events')}`}</p><h1>{t('auditLog.title')}</h1></div>
        <button className="secondary-command" type="button" onClick={refresh} disabled={busy} aria-busy={refreshing}><RefreshCw className={refreshing ? 'timeline-spinner' : ''} size={14} />{refreshing ? t('common.loading') : t('auditLog.refresh')}</button>
      </header>

      <section className="notification-summary audit-summary">
        <article><span><ClipboardList size={15} />{t('auditLog.pageEvents')}</span><strong>{logs ? rows.length : '—'}</strong><small>{pageLabel}</small></article>
        <article><span><Activity size={15} />{t('auditLog.actions')}</span><strong>{logs ? actions.length : '—'}</strong><small>{t('auditLog.actionsHelp')}</small></article>
        <article><span><UserRound size={15} />{t('auditLog.actors')}</span><strong>{logs ? actors : '—'}</strong><small>{t('auditLog.actorsHelp')}</small></article>
      </section>

      <form className="audit-filters" onSubmit={search}>
        <label><span>{t('auditLog.filterAction')}</span><div><Search size={14} /><input value={actionFilter} onChange={event => setActionFilter(event.target.value)} maxLength={128} list="audit-actions" placeholder={t('auditLog.actionSearch')} /></div></label>
        <datalist id="audit-actions">{actions.map(action => <option key={action} value={action} />)}</datalist>
        <label><span>{t('auditLog.filterUser')}</span><div><Search size={14} /><input value={userFilter} onChange={event => setUserFilter(event.target.value)} maxLength={128} placeholder={t('auditLog.userSearch')} /></div></label>
        <button className="secondary-command" type="submit" disabled={busy}>{t('builds.applyFilters')}</button>
      </form>

      {loading && <PageState />}
      {error && <div className="build-history-inline-error" role="alert"><span>{error}</span><button type="button" onClick={refresh} disabled={busy}>{t('common.retry')}</button></div>}
      {logs && <section className="operations-table-wrap audit-table-wrap" aria-busy={refreshing}>
        <table className="operations-table audit-table">
          <thead><tr><th>{t('auditLog.time')}</th><th>{t('auditLog.user')}</th><th>{t('auditLog.action')}</th><th>{t('auditLog.resourceType')}</th><th>{t('auditLog.resourceId')}</th><th>{t('auditLog.details')}</th><th>IP</th></tr></thead>
          <tbody>
            {!rows.length && <tr><td colSpan={7} className="operations-empty"><ClipboardList size={18} />{t('auditLog.empty')}</td></tr>}
            {rows.map(entry => {
              const detail = entryDetail(entry)
              return <tr key={entry.id}>
              <td className="muted-cell audit-time">{formatDateTime(entry.created_at)}</td>
              <td><span className="audit-actor"><UserRound size={13} /><span>{entry.username || entry.user || (entry.user_id ? `#${entry.user_id}` : '-')}</span></span></td>
              <td><span className={`audit-action ${entry.action}`}>{entry.action}</span></td>
              <td className="muted-cell">{entry.resource_type || '-'}</td>
              <td><code>{entry.resource_id || '-'}</code></td>
              <td><span className="audit-detail" title={detail}>{detail}</span></td>
              <td><code className="audit-ip">{entry.ip || '-'}</code></td>
            </tr>
            })}
          </tbody>
        </table>
      </section>}
      <footer className="build-history-pagination" aria-label={t('auditLog.title')}>
        <span role="status">{pageLabel}</span>
        <div>
          <button type="button" aria-label={t('builds.previousPage')} disabled={busy || cursors.length === 1} onClick={() => setCursors(stack => stack.slice(0, -1))}><ChevronLeft size={15} /></button>
          <button type="button" aria-label={t('builds.nextPage')} disabled={!hasNext || busy} onClick={() => { const lastID = rows.at(-1)?.id; if (lastID) setCursors(stack => [...stack, lastID]) }}><ChevronRight size={15} /></button>
        </div>
      </footer>
    </section></>
  )
}
