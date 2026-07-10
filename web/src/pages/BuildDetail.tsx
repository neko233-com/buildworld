import { useEffect, useState, useRef } from 'react'
import { useParams } from 'react-router-dom'
import { useI18n } from '../i18n'
import { api } from '../api'
import { useApi } from '../hooks'

function formatDuration(ms?: number): string {
  if (!ms) return '-'
  return `${(ms / 1000).toFixed(1)}s`
}

function formatTime(s?: string): string {
  if (!s) return '-'
  return new Date(s).toLocaleString()
}

function formatSize(bytes?: number): string {
  if (!bytes || bytes === 0) return '0 B'
  if (bytes < 1024) return `${bytes} B`
  if (bytes < 1024 * 1024) return `${(bytes / 1024).toFixed(1)} KB`
  return `${(bytes / (1024 * 1024)).toFixed(1)} MB`
}

export default function BuildDetail() {
  const { t } = useI18n()
  const { id } = useParams<{ id: string }>()
  const buildId = Number(id)
  const { data: build, loading, error, reload: reloadBuild } = useApi(
    () => api.getBuild(buildId),
    [buildId]
  )
  const { data: logsResp, reload: reloadLogs } = useApi(
    () => api.getBuildLogs(buildId),
    [buildId]
  )
  const { data: artifacts, reload: reloadArtifacts } = useApi(
    () => api.listArtifacts(buildId),
    [buildId]
  )
  const [retrying, setRetrying] = useState(false)
  const [pinning, setPinning] = useState(false)
  const [uploading, setUploading] = useState(false)
  const fileInputRef = useRef<HTMLInputElement>(null)

  const isRunning = build?.status === 'running' || build?.status === 'pending'
  const isFinished = build?.status && !isRunning

  useEffect(() => {
    if (!isRunning) return
    const timer = setInterval(() => {
      reloadLogs()
      reloadBuild()
    }, 2000)
    return () => clearInterval(timer)
  }, [isRunning, reloadLogs, reloadBuild])

  const handleStop = async () => {
    try {
      await api.stopBuild(buildId)
      reloadBuild()
    } catch (e: any) {
      alert(e.message || 'Failed to stop build')
    }
  }

  const handleRetry = async () => {
    setRetrying(true)
    try {
      await api.retryBuild(buildId)
      reloadBuild()
    } catch (e: any) {
      alert(e.message || 'Failed to retry build')
    } finally {
      setRetrying(false)
    }
  }

  const handlePin = async () => {
    if (!build) return
    setPinning(true)
    try {
      await api.pinBuild(buildId, !build.pinned)
      reloadBuild()
    } catch (e: any) {
      alert(e.message || 'Failed to toggle pin')
    } finally {
      setPinning(false)
    }
  }

  const handleUpload = async (e: React.ChangeEvent<HTMLInputElement>) => {
    const file = e.target.files?.[0]
    if (!file) return
    setUploading(true)
    try {
      await api.uploadArtifact(buildId, file)
      reloadArtifacts()
    } catch (e: any) {
      alert(e.message || 'Failed to upload artifact')
    } finally {
      setUploading(false)
      if (fileInputRef.current) fileInputRef.current.value = ''
    }
  }

  if (loading) return <div className="text-gray-500">{t('common.loading')}</div>
  if (error) return <div className="text-red-500">{t('common.error')}: {error}</div>
  if (!build) return null

  return (
    <div>
      <div className="flex justify-between items-center mb-4">
        <div>
          <h1 className="text-2xl font-bold flex items-center gap-2">
            {build.pinned && <span title="Pinned">📌</span>}
            Build #{build.number}
          </h1>
          <p className="text-gray-500">Project ID: {build.project_id}</p>
        </div>
        <div className="flex gap-2">
          {isFinished && (
            <>
              <button
                onClick={handleRetry}
                disabled={retrying}
                className="border border-blue-500 text-blue-600 px-4 py-2 rounded hover:bg-blue-50 disabled:opacity-50"
              >
                🔄 {retrying ? 'Retrying...' : 'Retry Build'}
              </button>
              <button
                onClick={handlePin}
                disabled={pinning}
                className={`border px-4 py-2 rounded hover:bg-gray-50 disabled:opacity-50 ${build.pinned ? 'bg-yellow-50 border-yellow-400 text-yellow-700' : 'border-gray-300 text-gray-700'}`}
              >
                📌 {build.pinned ? 'Unpin' : 'Pin'}
              </button>
            </>
          )}
          {isRunning && (
            <button
              onClick={handleStop}
              className="bg-red-500 text-white px-4 py-2 rounded"
            >
              Stop Build
            </button>
          )}
        </div>
      </div>

      <div className="grid grid-cols-2 md:grid-cols-4 gap-4 mb-6">
        <div className="bg-white p-4 rounded-lg shadow">
          <p className="text-sm text-gray-500">{t('builds.status')}</p>
          <p className="font-semibold">{build.status}</p>
        </div>
        <div className="bg-white p-4 rounded-lg shadow">
          <p className="text-sm text-gray-500">{t('builds.duration')}</p>
          <p className="font-semibold">{formatDuration(build.duration_ms)}</p>
        </div>
        <div className="bg-white p-4 rounded-lg shadow">
          <p className="text-sm text-gray-500">{t('builds.branch')}</p>
          <p className="font-semibold">{build.branch}</p>
        </div>
        <div className="bg-white p-4 rounded-lg shadow">
          <p className="text-sm text-gray-500">{t('builds.commit')}</p>
          <p className="font-semibold font-mono text-sm">{build.commit_sha?.slice(0, 8) || '-'}</p>
        </div>
        <div className="bg-white p-4 rounded-lg shadow">
          <p className="text-sm text-gray-500">{t('builds.trigger')}</p>
          <p className="font-semibold">{build.trigger}</p>
        </div>
        <div className="bg-white p-4 rounded-lg shadow">
          <p className="text-sm text-gray-500">Started</p>
          <p className="font-semibold text-sm">{formatTime(build.started_at)}</p>
        </div>
        <div className="bg-white p-4 rounded-lg shadow">
          <p className="text-sm text-gray-500">Finished</p>
          <p className="font-semibold text-sm">{formatTime(build.finished_at)}</p>
        </div>
        {(build.agent_id || build.agent_requirements) && (
          <div className="bg-white p-4 rounded-lg shadow">
            <p className="text-sm text-gray-500">Agent</p>
            <p className="font-semibold text-sm">
              {build.agent_id ? `#${build.agent_id}` : ''}
              {build.agent_requirements && (
                <span className="text-gray-500 text-xs block truncate">
                  req: {typeof build.agent_requirements === 'string' ? build.agent_requirements : JSON.stringify(build.agent_requirements)}
                </span>
              )}
            </p>
          </div>
        )}
        {build.retried_from && (
          <div className="bg-white p-4 rounded-lg shadow">
            <p className="text-sm text-gray-500">Retried From</p>
            <p className="font-semibold text-sm">
              <a href={`/builds/${build.retried_from}`} className="text-blue-600 hover:underline">
                #{build.retried_from}
              </a>
            </p>
          </div>
        )}
      </div>

      <div className="bg-white shadow rounded-lg p-4 mb-6">
        <h2 className="text-lg font-semibold mb-3">{t('builds.logs')}</h2>
        <pre className="bg-gray-900 text-green-400 p-4 rounded font-mono text-sm overflow-auto max-h-96 whitespace-pre-wrap">
          {logsResp?.log || 'No logs available'}
        </pre>
      </div>

      <div className="bg-white shadow rounded-lg p-4">
        <div className="flex justify-between items-center mb-3">
          <h2 className="text-lg font-semibold">Artifacts</h2>
          <div>
            <input
              ref={fileInputRef}
              type="file"
              onChange={handleUpload}
              className="hidden"
              id="artifact-upload"
            />
            <label
              htmlFor="artifact-upload"
              className={`inline-block cursor-pointer px-3 py-1 text-sm border rounded hover:bg-gray-50 ${uploading ? 'opacity-50 pointer-events-none' : ''}`}
            >
              {uploading ? 'Uploading...' : '+ Upload'}
            </label>
          </div>
        </div>
        {(artifacts || []).length === 0 ? (
          <p className="text-gray-500 text-center py-6">No artifacts</p>
        ) : (
          <table className="w-full">
            <thead>
              <tr className="border-b text-left text-xs text-gray-500">
                <th className="px-3 py-2 font-medium">Name</th>
                <th className="px-3 py-2 font-medium">Size</th>
                <th className="px-3 py-2 font-medium">Downloads</th>
                <th className="px-3 py-2 font-medium"></th>
              </tr>
            </thead>
            <tbody className="divide-y">
              {(artifacts || []).map((a: any) => (
                <tr key={a.id} className="hover:bg-gray-50">
                  <td className="px-3 py-2 font-medium text-sm">{a.name}</td>
                  <td className="px-3 py-2 text-gray-500 text-sm">{formatSize(a.size)}</td>
                  <td className="px-3 py-2 text-gray-500 text-sm">{a.download_count || 0}</td>
                  <td className="px-3 py-2 text-right">
                    <a
                      href={api.artifactDownloadUrl(a.id)}
                      target="_blank"
                      rel="noopener noreferrer"
                      download={a.name}
                      className="text-blue-600 hover:underline text-sm"
                    >
                      Download
                    </a>
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        )}
      </div>
    </div>
  )
}
