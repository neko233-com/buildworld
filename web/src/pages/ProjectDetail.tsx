import { useState } from 'react'
import { useParams, useNavigate } from 'react-router-dom'
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

export default function ProjectDetail() {
  const { t } = useI18n()
  const navigate = useNavigate()
  const { id } = useParams<{ id: string }>()
  const projectId = Number(id)
  const [activeTab, setActiveTab] = useState<'overview' | 'builds' | 'settings'>('overview')

  const { data: project, loading, error, reload } = useApi(
    () => api.getProject(projectId),
    [projectId]
  )
  const { data: builds, reload: reloadBuilds } = useApi(
    () => api.listProjectBuilds(projectId),
    [projectId]
  )

  const [form, setForm] = useState<any>(null)
  const [saving, setSaving] = useState(false)
  const [formError, setFormError] = useState('')

  // Initialize form once project loads
  if (project && !form) {
    setForm({
      name: project.name,
      description: project.description,
      repo_url: project.repo_url,
      repo_type: project.repo_type,
      default_branch: project.default_branch,
      config: project.config || '',
    })
  }

  const set = (key: string) => (e: React.ChangeEvent<HTMLInputElement | HTMLTextAreaElement | HTMLSelectElement>) =>
    setForm({ ...form, [key]: e.target.value })

  const handleBuildNow = async () => {
    try {
      await api.triggerBuild(projectId)
      reloadBuilds()
    } catch (e: any) {
      alert(e.message || 'Failed to trigger build')
    }
  }

  const handleSave = async (e: React.FormEvent) => {
    e.preventDefault()
    setSaving(true)
    setFormError('')
    try {
      await api.updateProject(projectId, form)
      reload()
    } catch (err: any) {
      setFormError(err.message || 'Failed to update project')
    } finally {
      setSaving(false)
    }
  }

  const handleDelete = async () => {
    if (!confirm('Delete this project?')) return
    try {
      await api.deleteProject(projectId)
      navigate('/projects')
    } catch (e: any) {
      alert(e.message || 'Failed to delete project')
    }
  }

  if (loading) return <div className="text-gray-500">{t('common.loading')}</div>
  if (error) return <div className="text-red-500">{t('common.error')}: {error}</div>
  if (!project) return null

  return (
    <div>
      <div className="flex justify-between items-center mb-4">
        <div>
          <h1 className="text-2xl font-bold">{project.name}</h1>
          <p className="text-gray-500">{project.description}</p>
        </div>
        <div className="flex gap-2">
          <button
            onClick={handleBuildNow}
            className="bg-blue-500 text-white px-4 py-2 rounded"
          >
            {t('projects.build')}
          </button>
        </div>
      </div>

      {/* Project Info */}
      <div className="grid grid-cols-1 md:grid-cols-3 gap-4 mb-6">
        <div className="bg-white p-4 rounded-lg shadow">
          <p className="text-sm text-gray-500">Repository</p>
          <p className="font-semibold text-blue-500 truncate">{project.repo_url}</p>
        </div>
        <div className="bg-white p-4 rounded-lg shadow">
          <p className="text-sm text-gray-500">{t('builds.branch')}</p>
          <p className="font-semibold">{project.default_branch}</p>
        </div>
        <div className="bg-white p-4 rounded-lg shadow">
          <p className="text-sm text-gray-500">Description</p>
          <p className="font-semibold">{project.description}</p>
        </div>
      </div>

      {/* Tabs */}
      <div className="bg-white shadow rounded-lg">
        <div className="border-b">
          <nav className="flex">
            <button
              className={`px-4 py-2 text-sm font-medium ${
                activeTab === 'overview' ? 'border-b-2 border-blue-500 text-blue-600' : 'text-gray-500 hover:text-gray-700'
              }`}
              onClick={() => setActiveTab('overview')}
            >
              Overview
            </button>
            <button
              className={`px-4 py-2 text-sm font-medium ${
                activeTab === 'builds' ? 'border-b-2 border-blue-500 text-blue-600' : 'text-gray-500 hover:text-gray-700'
              }`}
              onClick={() => setActiveTab('builds')}
            >
              {t('builds.title')}
            </button>
            <button
              className={`px-4 py-2 text-sm font-medium ${
                activeTab === 'settings' ? 'border-b-2 border-blue-500 text-blue-600' : 'text-gray-500 hover:text-gray-700'
              }`}
              onClick={() => setActiveTab('settings')}
            >
              {t('projects.settings')}
            </button>
          </nav>
        </div>

        <div className="p-4">
          {activeTab === 'overview' && (
            <div>
              <label className="block text-sm font-medium text-gray-700 mb-1">Pipeline Configuration</label>
              <pre className="bg-gray-900 text-green-400 p-4 rounded text-sm overflow-auto whitespace-pre-wrap">
                {project.config || '{}'}
              </pre>
            </div>
          )}

          {activeTab === 'builds' && (
            <table className="min-w-full">
              <thead>
                <tr className="border-b">
                  <th className="px-4 py-2 text-left text-sm font-medium text-gray-500">#</th>
                  <th className="px-4 py-2 text-left text-sm font-medium text-gray-500">{t('builds.status')}</th>
                  <th className="px-4 py-2 text-left text-sm font-medium text-gray-500">{t('builds.duration')}</th>
                  <th className="px-4 py-2 text-left text-sm font-medium text-gray-500">{t('builds.branch')}</th>
                  <th className="px-4 py-2 text-left text-sm font-medium text-gray-500">Started</th>
                </tr>
              </thead>
              <tbody>
                {(builds || []).length === 0 && (
                  <tr><td colSpan={5} className="px-4 py-2 text-gray-500">{t('common.noData')}</td></tr>
                )}
                {(builds || []).map((build) => (
                  <tr
                    key={build.id}
                    className="border-b hover:bg-gray-50 cursor-pointer"
                    onClick={() => navigate(`/builds/${build.id}`)}
                  >
                    <td className="px-4 py-2 font-medium">#{build.number}</td>
                    <td className="px-4 py-2">
                      <span className={`px-2 py-1 rounded text-sm ${statusColors[build.status] || 'bg-gray-100 text-gray-800'}`}>
                        {build.status}
                      </span>
                    </td>
                    <td className="px-4 py-2 text-gray-500">{formatDuration(build.duration_ms)}</td>
                    <td className="px-4 py-2 text-gray-500">{build.branch}</td>
                    <td className="px-4 py-2 text-gray-500">
                      {build.started_at ? new Date(build.started_at).toLocaleString() : '-'}
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          )}

          {activeTab === 'settings' && form && (
            <form onSubmit={handleSave} className="space-y-4 max-w-2xl">
              <div>
                <label className="block text-sm font-medium text-gray-700 mb-1">{t('projects.name')}</label>
                <input type="text" value={form.name} onChange={set('name')} className="w-full border rounded px-3 py-2" />
              </div>
              <div>
                <label className="block text-sm font-medium text-gray-700 mb-1">Description</label>
                <textarea value={form.description} onChange={set('description')} className="w-full border rounded px-3 py-2" rows={3} />
              </div>
              <div>
                <label className="block text-sm font-medium text-gray-700 mb-1">Repository URL</label>
                <input type="text" value={form.repo_url} onChange={set('repo_url')} className="w-full border rounded px-3 py-2" />
              </div>
              <div>
                <label className="block text-sm font-medium text-gray-700 mb-1">Default Branch</label>
                <input type="text" value={form.default_branch} onChange={set('default_branch')} className="w-full border rounded px-3 py-2" />
              </div>
              <div>
                <label className="block text-sm font-medium text-gray-700 mb-1">Pipeline Config (JSON)</label>
                <textarea value={form.config} onChange={set('config')} className="w-full border rounded px-3 py-2 font-mono text-sm" rows={10} />
              </div>
              {formError && <p className="text-red-500 text-sm">{formError}</p>}
              <div className="flex gap-2">
                <button type="submit" disabled={saving} className="bg-blue-500 text-white px-4 py-2 rounded disabled:opacity-50">
                  {saving ? t('common.loading') : t('common.save')}
                </button>
                <button type="button" onClick={handleDelete} className="bg-red-500 text-white px-4 py-2 rounded">
                  {t('projects.delete')}
                </button>
              </div>
            </form>
          )}
        </div>
      </div>
    </div>
  )
}
