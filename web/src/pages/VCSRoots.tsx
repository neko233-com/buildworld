import { useState } from 'react'
import { api } from '../api'
import { useApi } from '../hooks'

const TYPE_COLORS: Record<string, string> = {
  git: 'bg-blue-100 text-blue-800',
  svn: 'bg-orange-100 text-orange-800',
  hg: 'bg-purple-100 text-purple-800',
}

interface FormData {
  name: string
  type: string
  url: string
  branch: string
  credential_id: number | ''
  poll_interval: number
  auto_checkout: boolean
}

const emptyForm: FormData = {
  name: '',
  type: 'git',
  url: '',
  branch: 'main',
  credential_id: '',
  poll_interval: 60,
  auto_checkout: true,
}

export default function VCSRoots() {
  const { data: roots, loading, error, reload } = useApi(() => api.listVCSRoots())
  const { data: credentials } = useApi(() => api.listCredentials())
  const [showForm, setShowForm] = useState(false)
  const [form, setForm] = useState<FormData>(emptyForm)
  const [saving, setSaving] = useState(false)

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault()
    setSaving(true)
    try {
      const data: any = {
        name: form.name,
        type: form.type,
        url: form.url,
        branch: form.branch,
        poll_interval: form.poll_interval,
        auto_checkout: form.auto_checkout,
      }
      if (form.credential_id !== '') {
        data.credential_id = Number(form.credential_id)
      }
      await api.createVCSRoot(data)
      setShowForm(false)
      setForm(emptyForm)
      reload()
    } catch (err: any) {
      alert(err.message)
    } finally {
      setSaving(false)
    }
  }

  const handleDelete = async (id: number) => {
    if (!confirm('Delete this VCS root?')) return
    try {
      await api.deleteVCSRoot(id)
      reload()
    } catch (err: any) {
      alert(err.message)
    }
  }

  const getCredentialName = (credId: number) => {
    const cred = credentials?.find(c => c.id === credId)
    return cred ? cred.name : '-'
  }

  if (loading) return <div className="p-6">Loading...</div>
  if (error) return <div className="p-6 text-red-500">Error: {error}</div>

  return (
    <div>
      <div className="flex justify-between items-center mb-6">
        <h1 className="text-2xl font-bold">VCS Roots</h1>
        <button
          onClick={() => setShowForm(!showForm)}
          className="bg-blue-600 text-white px-4 py-2 rounded-lg hover:bg-blue-700 transition"
        >
          {showForm ? 'Cancel' : '+ New VCS Root'}
        </button>
      </div>

      {showForm && (
        <div className="bg-white p-6 rounded-lg shadow mb-6">
          <h2 className="text-lg font-semibold mb-4">New VCS Root</h2>
          <form onSubmit={handleSubmit} className="grid grid-cols-1 md:grid-cols-2 gap-4">
            <div>
              <label className="block text-sm font-medium text-gray-700 mb-1">Name *</label>
              <input className="w-full border rounded-lg px-3 py-2" value={form.name}
                onChange={e => setForm(p => ({ ...p, name: e.target.value }))} required />
            </div>
            <div>
              <label className="block text-sm font-medium text-gray-700 mb-1">Type *</label>
              <select className="w-full border rounded-lg px-3 py-2" value={form.type}
                onChange={e => setForm(p => ({ ...p, type: e.target.value }))}>
                <option value="git">Git</option>
                <option value="svn">SVN</option>
                <option value="hg">Mercurial (hg)</option>
              </select>
            </div>
            <div className="md:col-span-2">
              <label className="block text-sm font-medium text-gray-700 mb-1">URL *</label>
              <input className="w-full border rounded-lg px-3 py-2" placeholder="https://github.com/user/repo.git"
                value={form.url} onChange={e => setForm(p => ({ ...p, url: e.target.value }))} required />
            </div>
            <div>
              <label className="block text-sm font-medium text-gray-700 mb-1">Branch</label>
              <input className="w-full border rounded-lg px-3 py-2" value={form.branch}
                onChange={e => setForm(p => ({ ...p, branch: e.target.value }))} />
            </div>
            <div>
              <label className="block text-sm font-medium text-gray-700 mb-1">Credential</label>
              <select className="w-full border rounded-lg px-3 py-2" value={form.credential_id}
                onChange={e => setForm(p => ({ ...p, credential_id: e.target.value === '' ? '' : Number(e.target.value) }))}>
                <option value="">None</option>
                {credentials?.map(c => (
                  <option key={c.id} value={c.id}>{c.name}</option>
                ))}
              </select>
            </div>
            <div>
              <label className="block text-sm font-medium text-gray-700 mb-1">Poll Interval (seconds)</label>
              <input type="number" className="w-full border rounded-lg px-3 py-2" value={form.poll_interval}
                onChange={e => setForm(p => ({ ...p, poll_interval: Number(e.target.value) }))} min={0} />
            </div>
            <div className="flex items-end">
              <label className="flex items-center gap-2 cursor-pointer">
                <input type="checkbox" checked={form.auto_checkout}
                  onChange={e => setForm(p => ({ ...p, auto_checkout: e.target.checked }))}
                  className="w-4 h-4 rounded border-gray-300" />
                <span className="text-sm font-medium text-gray-700">Auto Checkout</span>
              </label>
            </div>
            <div className="md:col-span-2">
              <button type="submit" disabled={saving}
                className="bg-blue-600 text-white px-6 py-2 rounded-lg hover:bg-blue-700 disabled:opacity-50">
                {saving ? 'Saving...' : 'Save'}
              </button>
            </div>
          </form>
        </div>
      )}

      <div className="bg-white shadow rounded-lg overflow-hidden">
        <table className="w-full">
          <thead className="bg-gray-50">
            <tr>
              <th className="px-4 py-3 text-left text-xs font-medium text-gray-500 uppercase">Name</th>
              <th className="px-4 py-3 text-left text-xs font-medium text-gray-500 uppercase">Type</th>
              <th className="px-4 py-3 text-left text-xs font-medium text-gray-500 uppercase">URL</th>
              <th className="px-4 py-3 text-left text-xs font-medium text-gray-500 uppercase">Branch</th>
              <th className="px-4 py-3 text-left text-xs font-medium text-gray-500 uppercase">Credential</th>
              <th className="px-4 py-3 text-left text-xs font-medium text-gray-500 uppercase">Poll Interval</th>
              <th className="px-4 py-3 text-left text-xs font-medium text-gray-500 uppercase">Actions</th>
            </tr>
          </thead>
          <tbody className="divide-y">
            {(roots || []).length === 0 && (
              <tr><td colSpan={7} className="px-4 py-8 text-center text-gray-500">No VCS roots</td></tr>
            )}
            {(roots || []).map(r => (
              <tr key={r.id} className="hover:bg-gray-50">
                <td className="px-4 py-3 font-medium">{r.name}</td>
                <td className="px-4 py-3">
                  <span className={`px-2 py-1 rounded text-xs font-medium ${TYPE_COLORS[r.type] || 'bg-gray-100'}`}>
                    {r.type}
                  </span>
                </td>
                <td className="px-4 py-3 text-gray-600 font-mono text-xs truncate max-w-xs">{r.url}</td>
                <td className="px-4 py-3 text-gray-600">{r.branch || 'main'}</td>
                <td className="px-4 py-3 text-gray-600">{r.credential_id ? getCredentialName(r.credential_id) : '-'}</td>
                <td className="px-4 py-3 text-gray-600">{r.poll_interval === 0 ? 'Disabled' : `${r.poll_interval}s`}</td>
                <td className="px-4 py-3">
                  <button onClick={() => handleDelete(r.id)} className="text-red-600 hover:text-red-800 text-sm">
                    Delete
                  </button>
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>
    </div>
  )
}
