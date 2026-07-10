import { useState } from 'react'
import { useI18n } from '../i18n'
import { api } from '../api'
import { useApi } from '../hooks'

export default function AuditLog() {
  const { t } = useI18n()
  const [actionFilter, setActionFilter] = useState('')
  const [userFilter, setUserFilter] = useState('')
  const { data: logs, loading, error } = useApi(() => api.listAuditLogs(), [])

  if (loading) return <div className="text-gray-500">{t('common.loading')}</div>
  if (error) return <div className="text-red-500">{t('common.error')}: {error}</div>

  const rows = (logs || []).filter((l: any) =>
    (!actionFilter || l.action === actionFilter) &&
    (!userFilter || String(l.user_id ?? l.user ?? '').toLowerCase().includes(userFilter.toLowerCase()))
  )
  const actions = Array.from(new Set((logs || []).map((l: any) => l.action).filter(Boolean)))

  return (
    <div>
      <h1 className="text-2xl font-bold mb-4">{t('auditLog.title')}</h1>

      <div className="bg-white rounded-lg shadow border p-3 mb-4 flex gap-4 items-center">
        <div>
          <label className="text-sm text-gray-500 mr-2">{t('auditLog.filterAction')}</label>
          <select value={actionFilter} onChange={e => setActionFilter(e.target.value)} className="border rounded px-2 py-1 text-sm">
            <option value="">{t('auditLog.all')}</option>
            {actions.map(a => <option key={a} value={a}>{a}</option>)}
          </select>
        </div>
        <div>
          <label className="text-sm text-gray-500 mr-2">{t('auditLog.filterUser')}</label>
          <input value={userFilter} onChange={e => setUserFilter(e.target.value)} placeholder={t('common.search')} className="border rounded px-2 py-1 text-sm" />
        </div>
      </div>

      <div className="bg-white shadow rounded-lg">
        <table className="min-w-full">
          <thead>
            <tr className="border-b">
              <th className="px-4 py-3 text-left text-sm font-medium text-gray-500">{t('auditLog.time')}</th>
              <th className="px-4 py-3 text-left text-sm font-medium text-gray-500">{t('auditLog.user')}</th>
              <th className="px-4 py-3 text-left text-sm font-medium text-gray-500">{t('auditLog.action')}</th>
              <th className="px-4 py-3 text-left text-sm font-medium text-gray-500">{t('auditLog.resourceType')}</th>
              <th className="px-4 py-3 text-left text-sm font-medium text-gray-500">{t('auditLog.resourceId')}</th>
              <th className="px-4 py-3 text-left text-sm font-medium text-gray-500">{t('auditLog.details')}</th>
            </tr>
          </thead>
          <tbody>
            {rows.length === 0 && (
              <tr><td colSpan={6} className="px-4 py-4 text-gray-500">{t('common.noData')}</td></tr>
            )}
            {rows.map((l: any) => (
              <tr key={l.id} className="border-b">
                <td className="px-4 py-3 text-sm text-gray-500">{l.created_at ? new Date(l.created_at).toLocaleString() : '-'}</td>
                <td className="px-4 py-3 text-sm">{l.user_id ?? l.user ?? '-'}</td>
                <td className="px-4 py-3 text-sm"><span className="px-2 py-0.5 rounded bg-blue-100 text-blue-800">{l.action}</span></td>
                <td className="px-4 py-3 text-sm text-gray-500">{l.resource_type || '-'}</td>
                <td className="px-4 py-3 text-sm font-mono text-gray-500">{l.resource_id ?? '-'}</td>
                <td className="px-4 py-3 text-sm text-gray-500 max-w-xs truncate">{l.details ? (typeof l.details === 'string' ? l.details : JSON.stringify(l.details)) : '-'}</td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>
    </div>
  )
}
