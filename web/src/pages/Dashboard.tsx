import { useI18n } from '../i18n'
import { api } from '../api'
import { useApi } from '../hooks'

const statusColors: Record<string, string> = {
  success: 'bg-green-100 text-green-800',
  failed: 'bg-red-100 text-red-800',
  running: 'bg-blue-100 text-blue-800',
  pending: 'bg-gray-100 text-gray-800',
  cancelled: 'bg-yellow-100 text-yellow-800',
}

function StatusBadge({ status }: { status: string }) {
  return (
    <span className={`px-2 py-1 rounded text-sm ${statusColors[status] || 'bg-gray-100 text-gray-800'}`}>
      {status}
    </span>
  )
}

export default function Dashboard() {
  const { t } = useI18n()
  const { data: projects, loading: lp, error: ep } = useApi(() => api.listProjects())
  const { data: builds, loading: lb, error: eb } = useApi(() => api.listBuilds(10))
  const { data: agents, loading: la, error: ea } = useApi(() => api.listAgents())

  const loading = lp || lb || la
  const error = ep || eb || ea

  if (loading) return <div className="text-gray-500">{t('common.loading')}</div>
  if (error) return <div className="text-red-500">{t('common.error')}: {error}</div>

  const projectMap = new Map((projects || []).map(p => [p.id, p]))
  const runningBuilds = (builds || []).filter(b => b.status === 'running' || b.status === 'pending').length
  const onlineAgents = (agents || []).filter(a => a.status === 'online').length

  return (
    <div>
      <h1 className="text-2xl font-bold mb-4">{t('dashboard.title')}</h1>

      <div className="grid grid-cols-1 md:grid-cols-3 gap-4 mb-6">
        <div className="bg-white p-6 rounded-lg shadow">
          <h2 className="text-lg font-semibold">{t('dashboard.projects')}</h2>
          <p className="text-3xl font-bold text-blue-600">{projects?.length || 0}</p>
        </div>
        <div className="bg-white p-6 rounded-lg shadow">
          <h2 className="text-lg font-semibold">{t('dashboard.activeBuilds')}</h2>
          <p className="text-3xl font-bold text-green-600">{runningBuilds}</p>
        </div>
        <div className="bg-white p-6 rounded-lg shadow">
          <h2 className="text-lg font-semibold">{t('dashboard.workers')}</h2>
          <p className="text-3xl font-bold text-purple-600">{onlineAgents}</p>
        </div>
      </div>

      <div className="bg-white shadow rounded-lg p-6">
        <h2 className="text-lg font-semibold mb-4">{t('dashboard.recentBuilds')}</h2>
        <div className="space-y-3">
          {(builds || []).length === 0 && <p className="text-gray-500">{t('common.noData')}</p>}
          {(builds || []).map((build) => {
            const project = projectMap.get(build.project_id)
            return (
              <div key={build.id} className="flex items-center justify-between p-3 bg-gray-50 rounded">
                <div>
                  <span className="font-medium">{project?.name || `#${build.project_id}`} #{build.number}</span>
                  <span className="text-gray-500 ml-2">{build.branch}</span>
                  <span className="text-gray-500 ml-2">{build.started_at ? new Date(build.started_at).toLocaleString() : ''}</span>
                </div>
                <StatusBadge status={build.status} />
              </div>
            )
          })}
        </div>
      </div>
    </div>
  )
}
