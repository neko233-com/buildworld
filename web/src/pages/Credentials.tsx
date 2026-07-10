import { useState } from 'react'
import { useApi } from '../hooks'
import { api } from '../api'

const CRED_TYPES = [
  { value: '', label: 'All Types' },
  { value: 'ssh_key', label: 'SSH Key' },
  { value: 'git', label: 'Git' },
  { value: 'svn', label: 'SVN' },
  { value: 'hg', label: 'Mercurial (hg)' },
]

const TYPE_COLORS: Record<string, string> = {
  ssh_key: 'bg-purple-100 text-purple-800',
  git: 'bg-blue-100 text-blue-800',
  svn: 'bg-orange-100 text-orange-800',
  hg: 'bg-teal-100 text-teal-800',
}

interface FormData {
  name: string
  type: string
  host: string
  username: string
  password: string
  private_key: string
  public_key: string
  token: string
  description: string
}

const emptyForm: FormData = {
  name: '', type: 'ssh_key', host: '', username: '',
  password: '', private_key: '', public_key: '', token: '', description: '',
}

export default function Credentials() {
  const [filterType, setFilterType] = useState('')
  const { data: creds, loading, error, reload } = useApi(
    () => api.listCredentials(filterType || undefined),
    [filterType]
  )
  const [showForm, setShowForm] = useState(false)
  const [form, setForm] = useState<FormData>(emptyForm)
  const [saving, setSaving] = useState(false)
  const [revealed, setRevealed] = useState<Record<number, boolean>>({})

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault()
    setSaving(true)
    try {
      await api.createCredential(form)
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
    if (!confirm('Delete this credential?')) return
    try {
      await api.deleteCredential(id)
      reload()
    } catch (err: any) {
      alert(err.message)
    }
  }

  const toggleReveal = async (id: number) => {
    if (revealed[id]) {
      setRevealed(p => ({ ...p, [id]: false }))
      reload()
      return
    }
    try {
      await api.getCredential(id, true)
      setRevealed(p => ({ ...p, [id]: true }))
      reload()
    } catch (err: any) {
      alert(err.message)
    }
  }

  const renderSecret = (value: string, isRevealed: boolean) => {
    if (!value) return <span className="text-gray-400">-</span>
    if (isRevealed) {
      return (
        <code className="text-xs bg-gray-100 px-1 rounded break-all max-w-xs block truncate">{value.slice(0, 60)}{value.length > 60 ? '...' : ''}</code>
      )
    }
    return <span className="text-gray-500 font-mono">********</span>
  }

  if (loading) return <div className="p-6">Loading...</div>
  if (error) return <div className="p-6 text-red-500">Error: {error}</div>

  return (
    <div>
      <div className="flex justify-between items-center mb-6">
        <h1 className="text-2xl font-bold">Credentials</h1>
        <button
          onClick={() => setShowForm(!showForm)}
          className="bg-blue-600 text-white px-4 py-2 rounded-lg hover:bg-blue-700 transition"
        >
          {showForm ? 'Cancel' : '+ Add Credential'}
        </button>
      </div>

      {showForm && (
        <div className="bg-white p-6 rounded-lg shadow mb-6">
          <h2 className="text-lg font-semibold mb-4">New Credential</h2>
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
                <option value="ssh_key">SSH Key</option>
                <option value="git">Git</option>
                <option value="svn">SVN</option>
                <option value="hg">Mercurial (hg)</option>
              </select>
            </div>
            <div>
              <label className="block text-sm font-medium text-gray-700 mb-1">Host</label>
              <input className="w-full border rounded-lg px-3 py-2" placeholder="github.com" value={form.host}
                onChange={e => setForm(p => ({ ...p, host: e.target.value }))} />
            </div>
            <div>
              <label className="block text-sm font-medium text-gray-700 mb-1">Username</label>
              <input className="w-full border rounded-lg px-3 py-2" value={form.username}
                onChange={e => setForm(p => ({ ...p, username: e.target.value }))} />
            </div>
            {form.type === 'ssh_key' ? (
              <>
                <div className="md:col-span-2">
                  <label className="block text-sm font-medium text-gray-700 mb-1">Private Key (PEM)</label>
                  <textarea className="w-full border rounded-lg px-3 py-2 font-mono text-xs" rows={4}
                    placeholder="-----BEGIN RSA PRIVATE KEY-----" value={form.private_key}
                    onChange={e => setForm(p => ({ ...p, private_key: e.target.value }))} />
                </div>
                <div className="md:col-span-2">
                  <label className="block text-sm font-medium text-gray-700 mb-1">Public Key</label>
                  <textarea className="w-full border rounded-lg px-3 py-2 font-mono text-xs" rows={2}
                    placeholder="ssh-rsa AAAA..." value={form.public_key}
                    onChange={e => setForm(p => ({ ...p, public_key: e.target.value }))} />
                </div>
              </>
            ) : (
              <>
                <div>
                  <label className="block text-sm font-medium text-gray-700 mb-1">Password</label>
                  <input type="password" className="w-full border rounded-lg px-3 py-2" value={form.password}
                    onChange={e => setForm(p => ({ ...p, password: e.target.value }))} />
                </div>
                <div>
                  <label className="block text-sm font-medium text-gray-700 mb-1">Access Token</label>
                  <input type="password" className="w-full border rounded-lg px-3 py-2" value={form.token}
                    onChange={e => setForm(p => ({ ...p, token: e.target.value }))} />
                </div>
              </>
            )}
            <div className="md:col-span-2">
              <label className="block text-sm font-medium text-gray-700 mb-1">Description</label>
              <input className="w-full border rounded-lg px-3 py-2" value={form.description}
                onChange={e => setForm(p => ({ ...p, description: e.target.value }))} />
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

      <div className="mb-4">
        <select className="border rounded-lg px-3 py-2" value={filterType} onChange={e => setFilterType(e.target.value)}>
          {CRED_TYPES.map(t => <option key={t.value} value={t.value}>{t.label}</option>)}
        </select>
      </div>

      <div className="bg-white shadow rounded-lg overflow-hidden">
        <table className="w-full">
          <thead className="bg-gray-50">
            <tr>
              <th className="px-4 py-3 text-left text-xs font-medium text-gray-500 uppercase">Name</th>
              <th className="px-4 py-3 text-left text-xs font-medium text-gray-500 uppercase">Type</th>
              <th className="px-4 py-3 text-left text-xs font-medium text-gray-500 uppercase">Host</th>
              <th className="px-4 py-3 text-left text-xs font-medium text-gray-500 uppercase">Username</th>
              <th className="px-4 py-3 text-left text-xs font-medium text-gray-500 uppercase">Secret</th>
              <th className="px-4 py-3 text-left text-xs font-medium text-gray-500 uppercase">Actions</th>
            </tr>
          </thead>
          <tbody className="divide-y">
            {creds?.length === 0 && (
              <tr><td colSpan={6} className="px-4 py-8 text-center text-gray-500">No credentials</td></tr>
            )}
            {creds?.map(c => (
              <tr key={c.id} className="hover:bg-gray-50">
                <td className="px-4 py-3 font-medium">{c.name}</td>
                <td className="px-4 py-3">
                  <span className={`px-2 py-1 rounded text-xs font-medium ${TYPE_COLORS[c.type] || 'bg-gray-100'}`}>
                    {c.type}
                  </span>
                </td>
                <td className="px-4 py-3 text-gray-600">{c.host || '-'}</td>
                <td className="px-4 py-3 text-gray-600">{c.username || '-'}</td>
                <td className="px-4 py-3">
                  <div className="flex items-center gap-2">
                    {renderSecret(c.password || c.private_key || c.token, revealed[c.id])}
                    <button onClick={() => toggleReveal(c.id)} className="text-blue-600 text-xs hover:underline">
                      {revealed[c.id] ? 'Hide' : 'Reveal'}
                    </button>
                  </div>
                </td>
                <td className="px-4 py-3">
                  <button onClick={() => handleDelete(c.id)} className="text-red-600 hover:text-red-800 text-sm">
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
