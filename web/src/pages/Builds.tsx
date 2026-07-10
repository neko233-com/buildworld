import { useState } from 'react'
import { useNavigate } from 'react-router-dom'
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

function formatDuration(ms?: number): string {
  if (!ms) return '-'
  return `${(ms / 1000).toFixed(1)}s`
}

export default function Builds() {
  const { t } = useI18n()
  const navigate = useNavigate()
  const { data: builds, loading, error, reload } = useApi(() => api.listBuilds(100))
  const [pinning, setPinning] = useState<number | null>(null)
  const [retrying, setRetrying] = useState<number | null>(null)

  const handleRetry = async (e: React.MouseEvent, buildId: number) => {
    e.stopPropagation()
    setRetrying(buildId)
    try {
      await api.retryBuild(buildId)
      reload()
    } catch (err: any) {
      alert(err.message)
    } finally {
      setRetrying(null)
    }
  }

  const handlePin = async (e: React.MouseEvent, buildId: number, currentPinned: boolean) => {
    e.stopPropagation()
    setPinning(buildId)
    try {
      await api.pinBuild(buildId, !currentPinned)
      reload()
    } catch (err: any) {
      alert(err.message)
    } finally {
      setPinning(null)
    }
  }

  if (loading) return <div className="text-gray-500">{t('common.loading')}</div>
  if (error) return <div className="text-red-500">{t('common.error')}: {error}</div>

  return (
    <div>
      <h1 className="text-2xl font-bold mb-4">{t('builds.title')}</h1>

      <div className="bg-white shadow rounded-lg">
        <table className="min-w-full">
          <thead>
            <tr className="border-b">
              <th className="px-6 py-3 text-left text-sm font-medium text-gray-500">{t('builds.buildNumber')}</th>
              <th className="px-6 py-3 text-left text-sm font-medium text-gray-500">Project ID</th>
              <th className="px-6 py-3 text-left text-sm font-medium text-gray-500">{t('builds.status')}</th>
              <th className="px-6 py-3 text-left text-sm font-medium text-gray-500">{t('builds.trigger')}</th>
              <th className="px-6 py-3 text-left text-sm font-medium text-gray-500">{t('builds.branch')}</th>
              <th className="px-6 py-3 text-left text-sm font-medium text-gray-500">{t('builds.commit')}</th>
              <th className="px-6 py-3 text-left text-sm font-medium text-gray-500">{t('builds.duration')}</th>
              <th className="px-6 py-3 text-left text-sm font-medium text-gray-500">{t('projects.actions')}</th>
            </tr>
          </thead>
          <tbody>
            {(builds || []).length === 0 && (
              <tr><td colSpan={8} className="px-6 py-4 text-gray-500">{t('common.noData')}</td></tr>
            )}
            {(builds || []).map((build) => (
              <tr
                key={build.id}
                className="border-b hover:bg-gray-50 cursor-pointer"
                onClick={() => navigate(`/builds/${build.id}`)}
              >
                <td className="px-6 py-4 font-medium">
                  <span className="flex items-center gap-1">
                    {build.pinned && <span title="Pinned">📌</span>}
                    #{build.number}
                    {build.retried_from && (
                      <span className="text-gray-400 text-xs font-normal ml-1">
                        (retry of{' '}
                        <button
                          onClick={(e) => { e.stopPropagation(); navigate(`/builds/${build.retried_from}`) }}
                          className="text-blue-500 hover:underline"
                        >
                          #{build.retried_from}
                        </button>)
                      </span>
                    )}
                  </span>
                </td>
                <td className="px-6 py-4 text-gray-500">{build.project_id}</td>
                <td className="px-6 py-4">
                  <span className={`px-2 py-1 rounded text-sm ${statusColors[build.status] || 'bg-gray-100 text-gray-800'}`}>
                    {build.status}
                  </span>
                </td>
                <td className="px-6 py-4 text-gray-500">{build.trigger}</td>
                <td className="px-6 py-4 text-gray-500">{build.branch}</td>
                <td className="px-6 py-4 text-gray-500 font-mono text-xs">{build.commit_sha?.slice(0, 8) || '-'}</td>
                <td className="px-6 py-4 text-gray-500">{formatDuration(build.duration_ms)}</td>
                <td className="px-6 py-4">
                  <div className="flex items-center gap-2" onClick={e => e.stopPropagation()}>
                    <button
                      onClick={(e) => handleRetry(e, build.id)}
                      disabled={retrying === build.id || build.status === 'running' || build.status === 'pending'}
                      className="px-2 py-1 text-sm border rounded hover:bg-gray-50 disabled:opacity-40"
                      title="Retry build"
                    >
                      🔄
                    </button>
                    <button
                      onClick={(e) => handlePin(e, build.id, !!build.pinned)}
                      disabled={pinning === build.id}
                      className={`px-2 py-1 text-sm border rounded hover:bg-gray-50 disabled:opacity-40 ${build.pinned ? 'bg-yellow-50 border-yellow-300' : ''}`}
                      title={build.pinned ? 'Unpin' : 'Pin'}
                    >
                      📌
                    </button>
                    <button
                      onClick={() => navigate(`/builds/${build.id}`)}
                      className="text-blue-500 hover:underline text-sm"
                    >
                      {t('builds.logs')}
                    </button>
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
