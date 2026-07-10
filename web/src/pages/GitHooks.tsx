import { useState, useEffect, useCallback } from 'react'
import { useI18n } from '../i18n'
import { api } from '../api'

interface GitHook {
  id: number
  project_id: number
  event: string
  branch_filter: string
  secret: string
  params: string
  enabled: boolean
  created_at: string
}

interface Project {
  id: number
  name: string
}

const EVENTS = ['push', 'tag_push', 'pull_request', 'merge_request']

export default function GitHooks() {
  const { t } = useI18n()
  const [projects, setProjects] = useState<Project[]>([])
  const [selectedProject, setSelectedProject] = useState<number | null>(null)
  const [hooks, setHooks] = useState<GitHook[]>([])
  const [loading, setLoading] = useState(false)
  const [showModal, setShowModal] = useState(false)
  const [editing, setEditing] = useState<GitHook | null>(null)
  const [form, setForm] = useState({
    event: 'push',
    branch_filter: '*',
    secret: '',
    params: '{}',
    enabled: true,
  })

  useEffect(() => {
    loadProjects()
  }, [])

  const loadProjects = async () => {
    try {
      const projs = await api.listProjects()
      setProjects(projs || [])
      if (projs && projs.length > 0) {
        setSelectedProject(projs[0].id)
      }
    } catch (e) {
      console.error(e)
    }
  }

  const loadHooks = useCallback(async (projectId: number) => {
    setLoading(true)
    try {
      const result = await api.listGitHooks(projectId)
      setHooks(result || [])
    } catch (e) {
      setHooks([])
    } finally {
      setLoading(false)
    }
  }, [])

  useEffect(() => {
    if (selectedProject) {
      loadHooks(selectedProject)
    }
  }, [selectedProject, loadHooks])

  const resetForm = () => {
    setForm({
      event: 'push',
      branch_filter: '*',
      secret: '',
      params: '{}',
      enabled: true,
    })
  }

  const openCreate = () => {
    resetForm()
    setEditing(null)
    setShowModal(true)
  }

  const openEdit = (hook: GitHook) => {
    setEditing(hook)
    setForm({
      event: hook.event,
      branch_filter: hook.branch_filter,
      secret: hook.secret,
      params: hook.params,
      enabled: hook.enabled,
    })
    setShowModal(true)
  }

  const handleSubmit = async () => {
    if (!selectedProject) return
    const data = { ...form }

    try {
      if (editing) {
        await api.updateGitHook(editing.id, data)
      } else {
        await api.createGitHook(selectedProject, data)
      }
      setShowModal(false)
      loadHooks(selectedProject)
    } catch (e: any) {
      alert(e.message || 'Error')
    }
  }

  const handleDelete = async (id: number) => {
    if (!confirm(t('common.confirm'))) return
    try {
      await api.deleteGitHook(id)
      if (selectedProject) loadHooks(selectedProject)
    } catch (e: any) {
      alert(e.message || 'Error')
    }
  }

  const handleToggle = async (hook: GitHook) => {
    try {
      await api.updateGitHook(hook.id, { ...hook, enabled: !hook.enabled })
      if (selectedProject) loadHooks(selectedProject)
    } catch (e: any) {
      alert(e.message || 'Error')
    }
  }

  const getWebhookUrl = (projectId: number) => {
    const base = window.location.origin
    return `${base}/api/webhooks/${projectId}`
  }

  const copyToClipboard = (text: string) => {
    navigator.clipboard.writeText(text)
  }

  const getEventColor = (event: string) => {
    const colors: Record<string, string> = {
      push: 'bg-blue-100 text-blue-800',
      tag_push: 'bg-purple-100 text-purple-800',
      pull_request: 'bg-green-100 text-green-800',
      merge_request: 'bg-yellow-100 text-yellow-800',
    }
    return colors[event] || 'bg-gray-100 text-gray-800'
  }

  const getProjectName = (id: number) => {
    return projects.find(p => p.id === id)?.name || `#${id}`
  }

  return (
    <div className="space-y-4">
      <div className="flex justify-between items-center">
        <h1 className="text-2xl font-bold">{t('gitHooks.title')}</h1>
      </div>

      <div className="bg-white rounded-lg shadow border p-4">
        <div className="flex flex-col sm:flex-row gap-4 items-start sm:items-center justify-between">
          <div className="flex items-center gap-4">
            <div>
              <label className="block text-sm font-medium text-gray-700 mb-1">Project</label>
              <select
                value={selectedProject || ''}
                onChange={(e) => setSelectedProject(parseInt(e.target.value) || null)}
                className="border rounded px-3 py-2 min-w-64"
              >
                {projects.map(p => (
                  <option key={p.id} value={p.id}>{p.name}</option>
                ))}
              </select>
            </div>
          </div>
          <button
            onClick={openCreate}
            disabled={!selectedProject}
            className="px-4 py-2 bg-blue-600 text-white rounded hover:bg-blue-700 disabled:opacity-50"
          >
            + {t('common.create')} Hook
          </button>
        </div>

        {selectedProject && (
          <div className="mt-4 p-3 bg-gray-50 rounded border">
            <div className="flex items-center gap-2">
              <span className="text-sm text-gray-600">{t('gitHooks.webhookUrl')}:</span>
              <code className="flex-1 text-sm bg-white px-2 py-1 rounded border font-mono truncate">
                {getWebhookUrl(selectedProject)}
              </code>
              <button
                onClick={() => copyToClipboard(getWebhookUrl(selectedProject))}
                className="px-2 py-1 text-sm text-blue-600 hover:bg-blue-50 rounded"
              >
                {t('common.copy')}
              </button>
            </div>
          </div>
        )}
      </div>

      <div className="bg-white rounded-lg shadow border overflow-hidden">
        <table className="w-full">
          <thead className="bg-gray-50">
            <tr>
              <th className="px-4 py-3 text-left font-medium text-gray-700">ID</th>
              <th className="px-4 py-3 text-left font-medium text-gray-700">{t('gitHooks.event')}</th>
              <th className="px-4 py-3 text-left font-medium text-gray-700">{t('gitHooks.branch')}</th>
              <th className="px-4 py-3 text-left font-medium text-gray-700">{t('common.enable')}</th>
              <th className="px-4 py-3 text-left font-medium text-gray-700">Created</th>
              <th className="px-4 py-3 text-left font-medium text-gray-700">{t('credentials.actions')}</th>
            </tr>
          </thead>
          <tbody>
            {loading ? (
              <tr>
                <td colSpan={6} className="px-4 py-8 text-center text-gray-500">{t('common.loading')}</td>
              </tr>
            ) : hooks.length === 0 ? (
              <tr>
                <td colSpan={6} className="px-4 py-8 text-center text-gray-500">{t('common.noData')}</td>
              </tr>
            ) : (
              hooks.map(hook => (
                <tr key={hook.id} className="border-t hover:bg-gray-50">
                  <td className="px-4 py-3 font-mono text-gray-500">#{hook.id}</td>
                  <td className="px-4 py-3">
                    <span className={`inline-flex px-2 py-1 rounded text-xs font-medium ${getEventColor(hook.event)}`}>
                      {hook.event}
                    </span>
                  </td>
                  <td className="px-4 py-3 font-mono text-sm">{hook.branch_filter}</td>
                  <td className="px-4 py-3">
                    <button
                      onClick={() => handleToggle(hook)}
                      className={`relative inline-flex h-6 w-11 items-center rounded-full transition-colors ${
                        hook.enabled ? 'bg-green-500' : 'bg-gray-300'
                      }`}
                    >
                      <span
                        className={`inline-block h-4 w-4 transform rounded-full bg-white transition-transform ${
                          hook.enabled ? 'translate-x-6' : 'translate-x-1'
                        }`}
                      />
                    </button>
                  </td>
                  <td className="px-4 py-3 text-gray-500 text-sm">
                    {new Date(hook.created_at).toLocaleString()}
                  </td>
                  <td className="px-4 py-3">
                    <div className="flex gap-2">
                      <button onClick={() => openEdit(hook)} className="text-blue-600 hover:text-blue-800 text-sm">
                        Edit
                      </button>
                      <button onClick={() => handleDelete(hook.id)} className="text-red-600 hover:text-red-800 text-sm">
                        {t('common.delete')}
                      </button>
                    </div>
                  </td>
                </tr>
              ))
            )}
          </tbody>
        </table>
      </div>

      {showModal && (
        <div className="fixed inset-0 bg-black/50 flex items-center justify-center z-50">
          <div className="bg-white rounded-lg shadow-xl w-full max-w-lg p-6 max-h-[90vh] overflow-y-auto">
            <h2 className="text-xl font-bold mb-4">
              {editing ? `Edit Hook #${editing.id}` : `${t('common.create')} Hook - ${getProjectName(selectedProject!)}`}
            </h2>
            <div className="space-y-4">
              <div>
                <label className="block text-sm font-medium text-gray-700 mb-1">{t('gitHooks.event')}</label>
                <select
                  value={form.event}
                  onChange={(e) => setForm({ ...form, event: e.target.value })}
                  className="w-full border rounded px-3 py-2"
                >
                  {EVENTS.map(ev => (
                    <option key={ev} value={ev}>{ev}</option>
                  ))}
                </select>
              </div>
              <div>
                <label className="block text-sm font-medium text-gray-700 mb-1">{t('gitHooks.branch')}</label>
                <input
                  type="text"
                  value={form.branch_filter}
                  onChange={(e) => setForm({ ...form, branch_filter: e.target.value })}
                  placeholder="*"
                  className="w-full border rounded px-3 py-2 font-mono"
                />
                <p className="text-xs text-gray-500 mt-1">Use * for all branches, or prefix like refs/heads/main</p>
              </div>
              <div>
                <label className="block text-sm font-medium text-gray-700 mb-1">{t('gitHooks.secret')}</label>
                <input
                  type="text"
                  value={form.secret}
                  onChange={(e) => setForm({ ...form, secret: e.target.value })}
                  placeholder="Optional secret token"
                  className="w-full border rounded px-3 py-2 font-mono"
                />
              </div>
              <div>
                <label className="block text-sm font-medium text-gray-700 mb-1">{t('gitHooks.params')}</label>
                <textarea
                  value={form.params}
                  onChange={(e) => setForm({ ...form, params: e.target.value })}
                  placeholder='{"env": {"KEY": "value"}}'
                  className="w-full border rounded px-3 py-2 font-mono h-24"
                />
                <p className="text-xs text-gray-500 mt-1">JSON object of build parameters</p>
              </div>
              <div className="flex items-center">
                <input
                  type="checkbox"
                  checked={form.enabled}
                  onChange={(e) => setForm({ ...form, enabled: e.target.checked })}
                  className="mr-2 h-4 w-4"
                />
                <label className="text-sm">{t('common.enable')}</label>
              </div>
            </div>
            <div className="flex justify-end gap-2 mt-6">
              <button
                onClick={() => setShowModal(false)}
                className="px-4 py-2 text-gray-600 hover:bg-gray-100 rounded"
              >
                {t('common.cancel')}
              </button>
              <button
                onClick={handleSubmit}
                className="px-4 py-2 bg-blue-600 text-white rounded hover:bg-blue-700"
              >
                {t('common.save')}
              </button>
            </div>
          </div>
        </div>
      )}
    </div>
  )
}
