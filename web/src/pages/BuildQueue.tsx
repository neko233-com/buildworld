import { useI18n } from '../i18n'
import { api } from '../api'
import { useApi } from '../hooks'

export default function BuildQueue() {
  const { t } = useI18n()
  const { data: queue, loading, error, reload } = useApi(() => api.listBuildQueue(), [])

  const list = queue || []

  const move = async (id: number, dir: 'up' | 'down') => {
    const idx = list.findIndex((q: any) => q.id === id)
    if (idx === -1) return
    const target = dir === 'up' ? idx - 1 : idx + 1
    if (target < 0 || target >= list.length) return
    try {
      await api.reorderBuildQueue(id, list[target].priority)
      reload()
    } catch (e: any) { alert(e.message) }
  }

  const cancel = async (id: number) => {
    if (!confirm('Cancel this queued build?')) return
    try { await api.stopBuild(id); reload() }
    catch (e: any) { alert(e.message) }
  }

  if (loading) return <div className="text-gray-500">{t('common.loading')}</div>
  if (error) return <div className="text-red-500">{t('common.error')}: {error}</div>

  return (
    <div>
      <h1 className="text-2xl font-bold mb-4">{t('buildQueue.title')}</h1>
      <div className="bg-white shadow rounded-lg">
        <table className="min-w-full">
          <thead>
            <tr className="border-b">
              <th className="px-6 py-3 text-left text-sm font-medium text-gray-500">{t('buildQueue.position')}</th>
              <th className="px-6 py-3 text-left text-sm font-medium text-gray-500">{t('buildQueue.project')}</th>
              <th className="px-6 py-3 text-left text-sm font-medium text-gray-500">{t('buildQueue.branch')}</th>
              <th className="px-6 py-3 text-left text-sm font-medium text-gray-500">{t('buildQueue.priority')}</th>
              <th className="px-6 py-3 text-left text-sm font-medium text-gray-500">{t('projects.actions')}</th>
            </tr>
          </thead>
          <tbody>
            {list.length === 0 && (
              <tr><td colSpan={5} className="px-6 py-4 text-gray-500">{t('common.noData')}</td></tr>
            )}
            {list.map((q: any, i: number) => (
              <tr key={q.id} className="border-b">
                <td className="px-6 py-4 font-medium">#{i + 1}</td>
                <td className="px-6 py-4 text-sm">{q.project_id ?? q.project ?? '-'}</td>
                <td className="px-6 py-4 text-sm text-gray-500">{q.branch || '-'}</td>
                <td className="px-6 py-4 text-sm text-gray-500">{q.priority ?? 0}</td>
                <td className="px-6 py-4">
                  <div className="flex gap-2">
                    <button onClick={() => move(q.id, 'up')} disabled={i === 0} className="px-2 py-1 text-sm border rounded hover:bg-gray-50 disabled:opacity-40">↑</button>
                    <button onClick={() => move(q.id, 'down')} disabled={i === list.length - 1} className="px-2 py-1 text-sm border rounded hover:bg-gray-50 disabled:opacity-40">↓</button>
                    <button onClick={() => cancel(q.id)} className="text-red-500 hover:underline text-sm">{t('buildQueue.cancel')}</button>
                  </div>
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>
    </div>
  )
}
