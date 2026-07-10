import { useState } from 'react'
import { useI18n } from '../i18n'
import { api } from '../api'
import { useApi } from '../hooks'

export default function Plugins() {
  const { t } = useI18n()
  const { data: plugins, loading, error, reload } = useApi(() => api.listPlugins())
  const [filter, setFilter] = useState<'all' | 'enabled' | 'disabled'>('all')

  const [showForm, setShowForm] = useState(false)
  const [form, setForm] = useState({ name: '', version: '', description: '', config: '' })
  const [saving, setSaving] = useState(false)
  const [formError, setFormError] = useState('')

  const handleInstall = async (e: React.FormEvent) => {
    e.preventDefault()
    setSaving(true)
    setFormError('')
    try {
      const data: any = { name: form.name }
      if (form.version) data.version = form.version
      if (form.description) data.description = form.description
      if (form.config) data.config = form.config
      await api.installPlugin(data)
      setForm({ name: '', version: '', description: '', config: '' })
      setShowForm(false)
      reload()
    } catch (err: any) {
      setFormError(err.message || 'Failed to install plugin')
    } finally {
      setSaving(false)
    }
  }

  const handleToggle = async (id: number, enabled: boolean) => {
    try {
      await api.togglePlugin(id, !enabled)
      reload()
    } catch (e: any) {
      alert(e.message || 'Failed to toggle plugin')
    }
  }

  const handleDelete = async (id: number) => {
    if (!confirm('Uninstall this plugin?')) return
    try {
      await api.deletePlugin(id)
      reload()
    } catch (e: any) {
      alert(e.message || 'Failed to delete plugin')
    }
  }

  if (loading) return <div className="text-gray-500">{t('common.loading')}</div>
  if (error) return <div className="text-red-500">{t('common.error')}: {error}</div>

  const list = plugins || []
  const filtered = list.filter(p => {
    if (filter === 'enabled') return p.enabled
    if (filter === 'disabled') return !p.enabled
    return true
  })

  return (
    <div>
      <div className="flex justify-between items-center mb-4">
        <h1 className="text-2xl font-bold">{t('plugins.title')}</h1>
        <button
          onClick={() => setShowForm(!showForm)}
          className="bg-blue-500 text-white px-4 py-2 rounded"
        >
          {t('plugins.install')}
        </button>
      </div>

      {/* Install form */}
      {showForm && (
        <form onSubmit={handleInstall} className="bg-white shadow rounded-lg p-6 mb-6 space-y-4 max-w-2xl">
          <div>
            <label className="block text-sm font-medium text-gray-700 mb-1">Plugin Name</label>
            <input
              type="text"
              required
              value={form.name}
              onChange={e => setForm({ ...form, name: e.target.value })}
              className="w-full border rounded px-3 py-2"
            />
          </div>
          <div className="grid grid-cols-2 gap-4">
            <div>
              <label className="block text-sm font-medium text-gray-700 mb-1">Version (optional)</label>
              <input
                type="text"
                value={form.version}
                onChange={e => setForm({ ...form, version: e.target.value })}
                className="w-full border rounded px-3 py-2"
              />
            </div>
            <div>
              <label className="block text-sm font-medium text-gray-700 mb-1">Description (optional)</label>
              <input
                type="text"
                value={form.description}
                onChange={e => setForm({ ...form, description: e.target.value })}
                className="w-full border rounded px-3 py-2"
              />
            </div>
          </div>
          <div>
            <label className="block text-sm font-medium text-gray-700 mb-1">Config (JSON, optional)</label>
            <textarea
              value={form.config}
              onChange={e => setForm({ ...form, config: e.target.value })}
              className="w-full border rounded px-3 py-2 font-mono text-sm"
              rows={4}
            />
          </div>
          {formError && <p className="text-red-500 text-sm">{formError}</p>}
          <div className="flex gap-2">
            <button type="submit" disabled={saving} className="bg-blue-500 text-white px-4 py-2 rounded disabled:opacity-50">
              {saving ? t('common.loading') : t('plugins.install')}
            </button>
            <button type="button" onClick={() => setShowForm(false)} className="bg-gray-300 text-gray-700 px-4 py-2 rounded">
              {t('common.cancel')}
            </button>
          </div>
        </form>
      )}

      {/* Filter */}
      <div className="flex gap-2 mb-4">
        <button
          className={`px-4 py-2 rounded ${filter === 'all' ? 'bg-blue-500 text-white' : 'bg-gray-200'}`}
          onClick={() => setFilter('all')}
        >
          All ({list.length})
        </button>
        <button
          className={`px-4 py-2 rounded ${filter === 'enabled' ? 'bg-blue-500 text-white' : 'bg-gray-200'}`}
          onClick={() => setFilter('enabled')}
        >
          Enabled ({list.filter(p => p.enabled).length})
        </button>
        <button
          className={`px-4 py-2 rounded ${filter === 'disabled' ? 'bg-blue-500 text-white' : 'bg-gray-200'}`}
          onClick={() => setFilter('disabled')}
        >
          Disabled ({list.filter(p => !p.enabled).length})
        </button>
      </div>

      {/* Plugin list */}
      <div className="space-y-4">
        {filtered.length === 0 && (
          <div className="bg-white shadow rounded-lg p-6 text-gray-500">{t('common.noData')}</div>
        )}
        {filtered.map((plugin) => (
          <div key={plugin.id} className="bg-white shadow rounded-lg p-4">
            <div className="flex justify-between items-start">
              <div>
                <div className="flex items-center gap-2">
                  <h3 className="font-semibold text-lg">{plugin.name}</h3>
                  <span className="text-sm text-gray-500">v{plugin.version}</span>
                  <span className={`px-2 py-1 rounded text-xs ${
                    plugin.enabled ? 'bg-green-100 text-green-800' : 'bg-gray-100 text-gray-800'
                  }`}>
                    {plugin.enabled ? 'Enabled' : 'Disabled'}
                  </span>
                </div>
                <p className="text-gray-600 mt-1">{plugin.description}</p>
              </div>
              <div className="flex gap-2">
                <button
                  onClick={() => handleToggle(plugin.id, plugin.enabled)}
                  className={`px-3 py-1 rounded text-sm ${
                    plugin.enabled
                      ? 'bg-yellow-100 text-yellow-800 hover:bg-yellow-200'
                      : 'bg-green-100 text-green-800 hover:bg-green-200'
                  }`}
                >
                  {plugin.enabled ? t('plugins.disable') : t('plugins.enable')}
                </button>
                <button
                  onClick={() => handleDelete(plugin.id)}
                  className="px-3 py-1 rounded text-sm bg-red-100 text-red-800 hover:bg-red-200"
                >
                  {t('plugins.uninstall')}
                </button>
              </div>
            </div>
          </div>
        ))}
      </div>
    </div>
  )
}
