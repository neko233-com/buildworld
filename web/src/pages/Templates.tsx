import { useState } from 'react'
import { useNavigate } from 'react-router-dom'
import { api } from '../api'
import { useApi } from '../hooks'

const TEMPLATE_DEFAULT = `{
  "stages": [
    {"name": "Build", "steps": [{"name": "compile", "type": "shell", "command": "echo building"}]}
  ]
}`

function formatDate(s?: string): string {
  if (!s) return '-'
  return new Date(s).toLocaleDateString()
}

interface FormData {
  name: string
  description: string
  config: string
}

const emptyForm: FormData = {
  name: '',
  description: '',
  config: TEMPLATE_DEFAULT,
}

export default function Templates() {
  const navigate = useNavigate()
  const { data: templates, loading, error, reload } = useApi(() => api.listTemplates())
  const [showForm, setShowForm] = useState(false)
  const [form, setForm] = useState<FormData>(emptyForm)
  const [saving, setSaving] = useState(false)

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault()
    setSaving(true)
    try {
      await api.createTemplate(form)
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
    if (!confirm('Delete this template?')) return
    try {
      await api.deleteTemplate(id)
      reload()
    } catch (err: any) {
      alert(err.message)
    }
  }

  const handleUse = (id: number) => {
    navigate(`/projects/new?template=${id}`)
  }

  if (loading) return <div className="p-6">Loading...</div>
  if (error) return <div className="p-6 text-red-500">Error: {error}</div>

  return (
    <div>
      <div className="flex justify-between items-center mb-6">
        <h1 className="text-2xl font-bold">Build Templates</h1>
        <button
          onClick={() => setShowForm(!showForm)}
          className="bg-blue-600 text-white px-4 py-2 rounded-lg hover:bg-blue-700 transition"
        >
          {showForm ? 'Cancel' : '+ New Template'}
        </button>
      </div>

      {showForm && (
        <div className="bg-white p-6 rounded-lg shadow mb-6">
          <h2 className="text-lg font-semibold mb-4">New Template</h2>
          <form onSubmit={handleSubmit} className="space-y-4">
            <div>
              <label className="block text-sm font-medium text-gray-700 mb-1">Name *</label>
              <input className="w-full border rounded-lg px-3 py-2" value={form.name}
                onChange={e => setForm(p => ({ ...p, name: e.target.value }))} required />
            </div>
            <div>
              <label className="block text-sm font-medium text-gray-700 mb-1">Description</label>
              <input className="w-full border rounded-lg px-3 py-2" value={form.description}
                onChange={e => setForm(p => ({ ...p, description: e.target.value }))} />
            </div>
            <div>
              <label className="block text-sm font-medium text-gray-700 mb-1">Config (JSON/YAML)</label>
              <textarea className="w-full border rounded-lg px-3 py-2 font-mono text-xs" rows={10}
                value={form.config} onChange={e => setForm(p => ({ ...p, config: e.target.value }))} />
            </div>
            <div>
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
              <th className="px-4 py-3 text-left text-xs font-medium text-gray-500 uppercase">Description</th>
              <th className="px-4 py-3 text-left text-xs font-medium text-gray-500 uppercase">Created</th>
              <th className="px-4 py-3 text-left text-xs font-medium text-gray-500 uppercase">Actions</th>
            </tr>
          </thead>
          <tbody className="divide-y">
            {(templates || []).length === 0 && (
              <tr><td colSpan={4} className="px-4 py-8 text-center text-gray-500">No templates</td></tr>
            )}
            {(templates || []).map(t => (
              <tr key={t.id} className="hover:bg-gray-50">
                <td className="px-4 py-3 font-medium">{t.name}</td>
                <td className="px-4 py-3 text-gray-600">{t.description || '-'}</td>
                <td className="px-4 py-3 text-gray-500 text-sm">{formatDate(t.created_at)}</td>
                <td className="px-4 py-3">
                  <div className="flex gap-2">
                    <button onClick={() => handleUse(t.id)}
                      className="text-blue-600 hover:text-blue-800 text-sm font-medium">
                      Use
                    </button>
                    <button onClick={() => handleDelete(t.id)}
                      className="text-red-600 hover:text-red-800 text-sm">
                      Delete
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
