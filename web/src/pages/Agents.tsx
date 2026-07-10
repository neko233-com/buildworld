import { useState } from 'react'
import { useI18n } from '../i18n'
import { api } from '../api'
import { useApi } from '../hooks'

function parseLabels(labels: string): string[] {
  if (!labels) return []
  try {
    const parsed = JSON.parse(labels)
    if (Array.isArray(parsed)) return parsed
    if (typeof parsed === 'object') return Object.keys(parsed)
    return []
  } catch {
    return labels.split(',').map((s: string) => s.trim()).filter(Boolean)
  }
}

function formatHeartbeat(s?: string): string {
  if (!s) return '-'
  const diff = Date.now() - new Date(s).getTime()
  if (diff < 0) return s
  const secs = Math.floor(diff / 1000)
  if (secs < 60) return `${secs}s ago`
  const mins = Math.floor(secs / 60)
  if (mins < 60) return `${mins}m ago`
  return new Date(s).toLocaleString()
}

const statusColors: Record<string, string> = {
  online: 'bg-green-100 text-green-800',
  offline: 'bg-red-100 text-red-800',
  busy: 'bg-yellow-100 text-yellow-800',
}

export default function Agents() {
  const { t } = useI18n()
  const { data: agents, loading, error, reload } = useApi(() => api.listAgents())

  const [showForm, setShowForm] = useState(false)
  const [form, setForm] = useState({ name: '', address: '', labels: '', max_builds: 4, pool: '' })
  const [token, setToken] = useState('')
  const [saving, setSaving] = useState(false)
  const [formError, setFormError] = useState('')

  const set = (key: keyof typeof form) => (e: React.ChangeEvent<HTMLInputElement>) =>
    setForm({ ...form, [key]: e.target.value })

  const handleRegister = async (e: React.FormEvent) => {
    e.preventDefault()
    setSaving(true)
    setFormError('')
    try {
      const data: any = { name: form.name, max_builds: Number(form.max_builds) }
      if (form.address) data.address = form.address
      if (form.labels) data.labels = form.labels
      if (form.pool) data.pool = form.pool
      const res: any = await api.registerAgent(data)
      if (res?.token) setToken(res.token)
      setForm({ name: '', address: '', labels: '', max_builds: 4, pool: '' })
      setShowForm(false)
      reload()
    } catch (err: any) {
      setFormError(err.message || 'Failed to register agent')
    } finally {
      setSaving(false)
    }
  }

  const handleDelete = async (id: string) => {
    if (!confirm('Delete this agent?')) return
    try {
      await api.deleteAgent(id)
      reload()
    } catch (e: any) {
      alert(e.message || 'Failed to delete agent')
    }
  }

  const copyToken = () => {
    navigator.clipboard?.writeText(token)
  }

  if (loading) return <div className="text-gray-500">{t('common.loading')}</div>
  if (error) return <div className="text-red-500">{t('common.error')}: {error}</div>

  const list = agents || []
  const online = list.filter(a => a.status === 'online').length
  const offline = list.filter(a => a.status !== 'online').length

  return (
    <div>
      <div className="flex justify-between items-center mb-4">
        <h1 className="text-2xl font-bold">{t('agents.title')}</h1>
        <button
          onClick={() => setShowForm(!showForm)}
          className="bg-blue-500 text-white px-4 py-2 rounded"
        >
          + Register Agent
        </button>
      </div>

      {/* Stats */}
      <div className="grid grid-cols-1 md:grid-cols-3 gap-4 mb-6">
        <div className="bg-white p-4 rounded-lg shadow">
          <p className="text-sm text-gray-500">Total Agents</p>
          <p className="text-2xl font-bold">{list.length}</p>
        </div>
        <div className="bg-white p-4 rounded-lg shadow">
          <p className="text-sm text-gray-500">Online</p>
          <p className="text-2xl font-bold text-green-600">{online}</p>
        </div>
        <div className="bg-white p-4 rounded-lg shadow">
          <p className="text-sm text-gray-500">Offline</p>
          <p className="text-2xl font-bold text-red-600">{offline}</p>
        </div>
      </div>

      {/* Token display */}
      {token && (
        <div className="bg-yellow-50 border border-yellow-300 p-4 rounded-lg mb-6">
          <p className="font-semibold mb-2">Agent Token (copy now, shown once):</p>
          <div className="flex gap-2">
            <code className="bg-white px-2 py-1 rounded flex-1 font-mono text-sm break-all">{token}</code>
            <button onClick={copyToken} className="bg-blue-500 text-white px-3 py-1 rounded text-sm">Copy</button>
            <button onClick={() => setToken('')} className="bg-gray-300 px-3 py-1 rounded text-sm">Dismiss</button>
          </div>
        </div>
      )}

      {/* Register form */}
      {showForm && (
        <form onSubmit={handleRegister} className="bg-white shadow rounded-lg p-6 mb-6 space-y-4 max-w-2xl">
          <div>
            <label className="block text-sm font-medium text-gray-700 mb-1">Name</label>
            <input type="text" required value={form.name} onChange={set('name')} className="w-full border rounded px-3 py-2" />
          </div>
          <div>
            <label className="block text-sm font-medium text-gray-700 mb-1">Address (optional)</label>
            <input type="text" value={form.address} onChange={set('address')} placeholder="192.168.1.100:7050" className="w-full border rounded px-3 py-2" />
          </div>
          <div>
            <label className="block text-sm font-medium text-gray-700 mb-1">Labels (comma-separated)</label>
            <input type="text" value={form.labels} onChange={set('labels')} placeholder="linux,amd64" className="w-full border rounded px-3 py-2" />
          </div>
          <div>
            <label className="block text-sm font-medium text-gray-700 mb-1">Max Concurrent Builds</label>
            <input type="number" value={form.max_builds} onChange={set('max_builds')} className="w-full border rounded px-3 py-2" />
          </div>
          <div>
            <label className="block text-sm font-medium text-gray-700 mb-1">Pool</label>
            <input type="text" value={form.pool} onChange={set('pool')} placeholder="default" className="w-full border rounded px-3 py-2" />
          </div>
          {formError && <p className="text-red-500 text-sm">{formError}</p>}
          <div className="flex gap-2">
            <button type="submit" disabled={saving} className="bg-blue-500 text-white px-4 py-2 rounded disabled:opacity-50">
              {saving ? t('common.loading') : 'Register'}
            </button>
            <button type="button" onClick={() => setShowForm(false)} className="bg-gray-300 text-gray-700 px-4 py-2 rounded">
              {t('common.cancel')}
            </button>
          </div>
        </form>
      )}

      {/* Agent list */}
      <div className="bg-white shadow rounded-lg">
        <table className="min-w-full">
          <thead>
            <tr className="border-b">
              <th className="px-4 py-3 text-left text-sm font-medium text-gray-500">Name</th>
              <th className="px-4 py-3 text-left text-sm font-medium text-gray-500">Address</th>
              <th className="px-4 py-3 text-left text-sm font-medium text-gray-500">Status</th>
              <th className="px-4 py-3 text-left text-sm font-medium text-gray-500">Pool</th>
              <th className="px-4 py-3 text-left text-sm font-medium text-gray-500">Labels</th>
              <th className="px-4 py-3 text-left text-sm font-medium text-gray-500">Max Builds</th>
              <th className="px-4 py-3 text-left text-sm font-medium text-gray-500">Heartbeat</th>
              <th className="px-4 py-3 text-left text-sm font-medium text-gray-500">Actions</th>
            </tr>
          </thead>
          <tbody>
            {list.length === 0 && (
              <tr><td colSpan={8} className="px-4 py-4 text-gray-500">{t('common.noData')}</td></tr>
            )}
            {list.map((agent) => {
              const labels = parseLabels(agent.labels)
              return (
                <tr key={agent.id} className="border-b">
                  <td className="px-4 py-3 font-medium">{agent.name}</td>
                  <td className="px-4 py-3 text-gray-500 font-mono text-xs">{agent.address || '-'}</td>
                  <td className="px-4 py-3">
                    <span className={`px-2 py-1 rounded text-sm ${statusColors[agent.status] || 'bg-gray-100 text-gray-800'}`}>
                      {agent.status}
                    </span>
                  </td>
                  <td className="px-4 py-3">
                    <span className="px-2 py-1 rounded text-xs bg-gray-100 text-gray-700">
                      {agent.pool || 'default'}
                    </span>
                  </td>
                  <td className="px-4 py-3">
                    <div className="flex gap-1 flex-wrap">
                      {labels.length === 0 && <span className="text-gray-400 text-xs">-</span>}
                      {labels.map((label, i) => (
                        <span key={i} className="px-2 py-1 bg-gray-100 text-gray-600 rounded text-xs">{label}</span>
                      ))}
                    </div>
                  </td>
                  <td className="px-4 py-3">{agent.max_concurrent_builds ?? '-'}</td>
                  <td className="px-4 py-3 text-gray-500">{formatHeartbeat(agent.last_heartbeat)}</td>
                  <td className="px-4 py-3">
                    <button
                      onClick={() => handleDelete(agent.id)}
                      className="text-red-500 hover:underline text-sm"
                    >
                      {t('projects.delete')}
                    </button>
                  </td>
                </tr>
              )
            })}
          </tbody>
        </table>
      </div>
    </div>
  )
}
