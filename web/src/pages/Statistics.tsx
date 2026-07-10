import { useState } from 'react'
import { useI18n } from '../i18n'
import { api } from '../api'
import { useApi } from '../hooks'

function fmtDuration(ms?: number): string {
  if (!ms) return '-'
  if (ms < 60000) return `${(ms / 1000).toFixed(1)}s`
  return `${(ms / 60000).toFixed(1)}m`
}

function heatColor(count: number, max: number): string {
  if (count === 0 || max === 0) return 'bg-gray-100'
  const ratio = count / max
  if (ratio > 0.75) return 'bg-green-600'
  if (ratio > 0.5) return 'bg-green-500'
  if (ratio > 0.25) return 'bg-green-300'
  return 'bg-green-200'
}

export default function Statistics() {
  const { t } = useI18n()
  const [days, setDays] = useState(7)
  const { data: stats, loading, error } = useApi(() => api.getDashboardStats(), [])

  if (loading) return <div className="text-gray-500">{t('common.loading')}</div>
  if (error) return <div className="text-red-500">{t('common.error')}: {error}</div>

  const sr = stats?.success_rate ?? 0
  const fr = stats?.failure_rate ?? 0
  const avg = stats?.avg_duration_ms ?? 0
  const total = stats?.total_builds ?? 0
  const trend: any[] = stats?.trend ?? []
  const view = trend.slice(-days)
  const maxCount = Math.max(1, ...view.map((d: any) => d.total || d.count || 0))

  return (
    <div>
      <div className="flex justify-between items-center mb-4">
        <h1 className="text-2xl font-bold">{t('statistics.title')}</h1>
        <div className="flex gap-2">
          <button onClick={() => setDays(7)} className={`px-3 py-1 rounded text-sm ${days === 7 ? 'bg-blue-500 text-white' : 'border'}`}>{t('statistics.last7Days')}</button>
          <button onClick={() => setDays(30)} className={`px-3 py-1 rounded text-sm ${days === 30 ? 'bg-blue-500 text-white' : 'border'}`}>{t('statistics.last30Days')}</button>
        </div>
      </div>

      <div className="grid grid-cols-2 md:grid-cols-4 gap-4 mb-6">
        <div className="bg-white p-4 rounded-lg shadow border">
          <p className="text-sm text-gray-500">{t('statistics.successRate')}</p>
          <p className="text-2xl font-bold text-green-600">{sr}%</p>
        </div>
        <div className="bg-white p-4 rounded-lg shadow border">
          <p className="text-sm text-gray-500">{t('statistics.failureRate')}</p>
          <p className="text-2xl font-bold text-red-600">{fr}%</p>
        </div>
        <div className="bg-white p-4 rounded-lg shadow border">
          <p className="text-sm text-gray-500">{t('statistics.avgDuration')}</p>
          <p className="text-2xl font-bold">{fmtDuration(avg)}</p>
        </div>
        <div className="bg-white p-4 rounded-lg shadow border">
          <p className="text-sm text-gray-500">{t('statistics.totalBuilds')}</p>
          <p className="text-2xl font-bold">{total}</p>
        </div>
      </div>

      <div className="bg-white rounded-lg shadow border p-4 mb-6">
        <h2 className="text-lg font-semibold mb-3">{t('statistics.trend')}</h2>
        {view.length === 0 ? (
          <p className="text-gray-500 text-center py-6">{t('common.noData')}</p>
        ) : (
          <div className="flex items-end gap-1 h-40">
            {view.map((d: any, i: number) => {
              const count = d.total || d.count || 0
              const succ = d.success || 0
              const fail = d.failed || d.fail || 0
              const heightPct = maxCount ? (count / maxCount) * 100 : 0
              return (
                <div key={i} className="flex-1 flex flex-col items-center min-w-0">
                  <div className="w-full flex flex-col justify-end h-32">
                    <div className="w-full bg-red-400 rounded-t" style={{ height: `${count ? (fail / count) * heightPct : 0}%` }} />
                    <div className="w-full bg-green-500" style={{ height: `${count ? (succ / count) * heightPct : 0}%` }} />
                  </div>
                  <span className="text-xs text-gray-400 mt-1 truncate w-full text-center">{(d.date || '').slice(5)}</span>
                  <span className="text-xs text-gray-600">{count}</span>
                </div>
              )
            })}
          </div>
        )}
      </div>

      <div className="bg-white rounded-lg shadow border p-4">
        <h2 className="text-lg font-semibold mb-3">{t('statistics.heatmap')}</h2>
        {view.length === 0 ? (
          <p className="text-gray-500 text-center py-6">{t('common.noData')}</p>
        ) : (
          <div className="flex flex-wrap gap-1">
            {view.map((d: any, i: number) => {
              const count = d.total || d.count || 0
              return (
                <div key={i} className={`w-7 h-7 rounded-sm ${heatColor(count, maxCount)}`} title={`${d.date || ''}: ${count}`} />
              )
            })}
          </div>
        )}
      </div>
    </div>
  )
}
