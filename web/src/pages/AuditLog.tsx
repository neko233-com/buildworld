import { useState } from 'react'
import { motion } from 'motion/react'
import { Activity, ClipboardList, RefreshCw, Search, UserRound } from 'lucide-react'
import { useI18n } from '../i18n'
import { PageState } from '../components/PageState'
import { JenkinsHeaderBreadcrumb } from '../components/JenkinsPageShell'
import { api } from '../api'
import { useApi } from '../hooks'
import './ManagementPages.jenkins.css'

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
  const { data: logs, loading, error, reload } = useApi<AuditEntry[]>(() => api.listAuditLogs(), [])

  if (loading) return <>{breadcrumb}<section className="jenkins-management-page"><PageState /></section></>
  if (error) return <>{breadcrumb}<section className="jenkins-management-page"><PageState error={error} onRetry={reload} /></section></>

  const list = logs || []
  const query = userFilter.trim().toLowerCase()
  const rows = list.filter(entry =>
    (!actionFilter || entry.action === actionFilter) &&
    (!query || `${entry.username || entry.user || ''} ${entry.user_id || ''}`.toLowerCase().includes(query))
  )
  const actions = Array.from(new Set(list.map(entry => entry.action).filter(Boolean))).sort()
  const actors = new Set(list.map(entry => entry.username || entry.user || entry.user_id).filter(Boolean)).size

  return (
    <>{breadcrumb}<motion.section className="operations-page audit-workbench jenkins-management-page" initial={{ opacity: 0, y: 8 }} animate={{ opacity: 1, y: 0 }} transition={{ duration: 0.24, ease: 'easeOut' }}>
      <header className="operations-heading">
        <div><p>{rows.length} / {list.length} {t('auditLog.events')}</p><h1>{t('auditLog.title')}</h1></div>
        <button className="secondary-command" type="button" onClick={reload}><RefreshCw size={14} />{t('auditLog.refresh')}</button>
      </header>

      <section className="notification-summary audit-summary">
        <article><span><ClipboardList size={15} />{t('auditLog.total')}</span><strong>{list.length}</strong><small>{t('auditLog.totalHelp')}</small></article>
        <article><span><Activity size={15} />{t('auditLog.actions')}</span><strong>{actions.length}</strong><small>{t('auditLog.actionsHelp')}</small></article>
        <article><span><UserRound size={15} />{t('auditLog.actors')}</span><strong>{actors}</strong><small>{t('auditLog.actorsHelp')}</small></article>
      </section>

      <div className="audit-filters">
        <label>{t('auditLog.filterAction')}<select value={actionFilter} onChange={event => setActionFilter(event.target.value)}><option value="">{t('auditLog.all')}</option>{actions.map(action => <option key={action} value={action}>{action}</option>)}</select></label>
        <label><span>{t('auditLog.filterUser')}</span><div><Search size={14} /><input value={userFilter} onChange={event => setUserFilter(event.target.value)} placeholder={t('auditLog.userSearch')} /></div></label>
      </div>

      <section className="operations-table-wrap audit-table-wrap">
        <table className="operations-table audit-table">
          <thead><tr><th>{t('auditLog.time')}</th><th>{t('auditLog.user')}</th><th>{t('auditLog.action')}</th><th>{t('auditLog.resourceType')}</th><th>{t('auditLog.resourceId')}</th><th>{t('auditLog.details')}</th><th>IP</th></tr></thead>
          <tbody>
            {!rows.length && <tr><td colSpan={7} className="operations-empty"><ClipboardList size={18} />{t('auditLog.empty')}</td></tr>}
            {rows.map(entry => <tr key={entry.id}>
              <td className="muted-cell audit-time">{entry.created_at ? new Date(entry.created_at).toLocaleString() : '-'}</td>
              <td><span className="audit-actor"><UserRound size={13} /><span>{entry.username || entry.user || `#${entry.user_id || '-'}`}</span></span></td>
              <td><span className={`audit-action ${entry.action}`}>{entry.action}</span></td>
              <td className="muted-cell">{entry.resource_type || '-'}</td>
              <td><code>{entry.resource_id || '-'}</code></td>
              <td><span className="audit-detail" title={entryDetail(entry)}>{entryDetail(entry)}</span></td>
              <td><code className="audit-ip">{entry.ip || '-'}</code></td>
            </tr>)}
          </tbody>
        </table>
      </section>
    </motion.section></>
  )
}
