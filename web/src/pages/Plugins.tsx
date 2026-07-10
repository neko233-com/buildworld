import { useState } from 'react'
import { useI18n } from '../i18n'
import { api } from '../api'
import { useApi } from '../hooks'

export default function Plugins() {
  const { t } = useI18n()
  const { data: plugins, loading, error, reload } = useApi(() => api.listPlugins())
  const [filter, setFilter] = useState<'all' | 'enabled' | 'disabled'>('all')
  const [expandedId, setExpandedId] = useState<number | null>(null)

  const [showForm, setShowForm] = useState(false)
  const [form, setForm] = useState({ name: '', version: '', description: '', author: '', script: '', uiScript: '' })
  const [saving, setSaving] = useState(false)
  const [formError, setFormError] = useState('')
  const [reloadingId, setReloadingId] = useState<number | null>(null)

  const handleInstall = async (e: React.FormEvent) => {
    e.preventDefault()
    setSaving(true)
    setFormError('')
    try {
      const data: any = { name: form.name, source: 'upload' }
      if (form.version) data.version = form.version
      if (form.description) data.description = form.description
      if (form.author) data.author = form.author
      if (form.script) data.script = form.script
      if (form.uiScript) data.ui_script = form.uiScript
      await api.installPlugin(data)
      setForm({ name: '', version: '', description: '', author: '', script: '', uiScript: '' })
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
    if (!confirm(t('common.confirm'))) return
    try {
      await api.deletePlugin(id)
      reload()
    } catch (e: any) {
      alert(e.message || 'Failed to delete plugin')
    }
  }

  const handleReload = async (id: number, name: string) => {
    setReloadingId(id)
    try {
      await api.reloadPlugin(name)
      reload()
    } catch (e: any) {
      alert(e.message || 'Failed to reload plugin')
    } finally {
      setReloadingId(null)
    }
  }

  const getPluginExtCount = (plugin: any) => {
    const steps = plugin.steps?.length || plugin.registered_steps?.length || 0
    const triggers = plugin.triggers?.length || plugin.registered_triggers?.length || 0
    const uiExts = plugin.ui_extensions?.length || 0
    return { steps, triggers, uiExts }
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
          className="bg-blue-500 text-white px-4 py-2 rounded hover:bg-blue-600 transition"
        >
          {t('plugins.install')}
        </button>
      </div>

      {showForm && (
        <form onSubmit={handleInstall} className="bg-white shadow rounded-lg p-6 mb-6 space-y-4 max-w-3xl">
          <div className="grid grid-cols-2 gap-4">
            <div>
              <label className="block text-sm font-medium text-gray-700 mb-1">{t('plugins.name')} *</label>
              <input
                type="text"
                required
                value={form.name}
                onChange={e => setForm({ ...form, name: e.target.value })}
                className="w-full border rounded px-3 py-2"
                placeholder="my-plugin"
              />
            </div>
            <div>
              <label className="block text-sm font-medium text-gray-700 mb-1">{t('plugins.author')}</label>
              <input
                type="text"
                value={form.author}
                onChange={e => setForm({ ...form, author: e.target.value })}
                className="w-full border rounded px-3 py-2"
              />
            </div>
          </div>
          <div className="grid grid-cols-2 gap-4">
            <div>
              <label className="block text-sm font-medium text-gray-700 mb-1">{t('plugins.version')} ({t('common.optional')})</label>
              <input
                type="text"
                value={form.version}
                onChange={e => setForm({ ...form, version: e.target.value })}
                className="w-full border rounded px-3 py-2"
                placeholder="1.0.0"
              />
            </div>
            <div>
              <label className="block text-sm font-medium text-gray-700 mb-1">{t('plugins.description')} ({t('common.optional')})</label>
              <input
                type="text"
                value={form.description}
                onChange={e => setForm({ ...form, description: e.target.value })}
                className="w-full border rounded px-3 py-2"
              />
            </div>
          </div>
          <div>
            <label className="block text-sm font-medium text-gray-700 mb-1">{t('plugins.script')}</label>
            <textarea
              value={form.script}
              onChange={e => setForm({ ...form, script: e.target.value })}
              className="w-full border rounded px-3 py-2 font-mono text-sm"
              rows={8}
              placeholder="// plugin main script (index.js)"
            />
          </div>
          <div>
            <label className="block text-sm font-medium text-gray-700 mb-1">{t('plugins.uiScript')}</label>
            <textarea
              value={form.uiScript}
              onChange={e => setForm({ ...form, uiScript: e.target.value })}
              className="w-full border rounded px-3 py-2 font-mono text-sm"
              rows={6}
              placeholder="// UI extension script (ui.js) - use React.createElement, no JSX"
            />
            <p className="text-xs text-gray-500 mt-1">Note: ui.js uses React.createElement (no JSX). Receives (React, __BW_PLUGINS__) as parameters.</p>
          </div>
          {formError && <p className="text-red-500 text-sm">{formError}</p>}
          <div className="flex gap-2">
            <button type="submit" disabled={saving} className="bg-blue-500 text-white px-4 py-2 rounded disabled:opacity-50 hover:bg-blue-600 transition">
              {saving ? t('common.loading') : t('plugins.install')}
            </button>
            <button type="button" onClick={() => setShowForm(false)} className="bg-gray-300 text-gray-700 px-4 py-2 rounded hover:bg-gray-400 transition">
              {t('common.cancel')}
            </button>
          </div>
        </form>
      )}

      <div className="flex gap-2 mb-4">
        <button
          className={`px-4 py-2 rounded ${filter === 'all' ? 'bg-blue-500 text-white' : 'bg-gray-200 hover:bg-gray-300'} transition`}
          onClick={() => setFilter('all')}
        >
          {t('common.all')} ({list.length})
        </button>
        <button
          className={`px-4 py-2 rounded ${filter === 'enabled' ? 'bg-blue-500 text-white' : 'bg-gray-200 hover:bg-gray-300'} transition`}
          onClick={() => setFilter('enabled')}
        >
          {t('common.enable')} ({list.filter(p => p.enabled).length})
        </button>
        <button
          className={`px-4 py-2 rounded ${filter === 'disabled' ? 'bg-blue-500 text-white' : 'bg-gray-200 hover:bg-gray-300'} transition`}
          onClick={() => setFilter('disabled')}
        >
          {t('common.disable')} ({list.filter(p => !p.enabled).length})
        </button>
      </div>

      <div className="space-y-4">
        {filtered.length === 0 && (
          <div className="bg-white shadow rounded-lg p-6 text-gray-500">{t('common.noData')}</div>
        )}
        {filtered.map((plugin) => {
          const { steps, triggers, uiExts } = getPluginExtCount(plugin)
          const isExpanded = expandedId === plugin.id
          const stepList = plugin.steps || plugin.registered_steps || []
          const triggerList = plugin.triggers || plugin.registered_triggers || []
          const uiExtList = plugin.ui_extensions || []

          return (
            <div key={plugin.id} className="bg-white shadow rounded-lg overflow-hidden">
              <div className="p-4">
                <div className="flex justify-between items-start">
                  <div className="flex-1">
                    <div className="flex items-center gap-2 flex-wrap">
                      <h3 className="font-semibold text-lg">{plugin.name}</h3>
                      <span className="text-sm text-gray-500">v{plugin.version}</span>
                      <span className={`px-2 py-0.5 rounded text-xs font-medium ${
                        plugin.enabled ? 'bg-green-100 text-green-800' : 'bg-gray-100 text-gray-600'
                      }`}>
                        {plugin.enabled ? t('common.enable') : t('common.disable')}
                      </span>
                      <span className={`px-2 py-0.5 rounded text-xs font-medium ${
                        plugin.source === 'builtin'
                          ? 'bg-purple-100 text-purple-800'
                          : 'bg-blue-100 text-blue-800'
                      }`}>
                        {plugin.source === 'builtin' ? t('plugins.builtin') : t('plugins.upload')}
                      </span>
                    </div>
                    <p className="text-gray-600 mt-1 text-sm">{plugin.description}</p>
                    {plugin.source !== 'builtin' && plugin.author && (
                      <p className="text-gray-400 text-xs mt-1">{t('plugins.author')}: {plugin.author}</p>
                    )}
                    {plugin.load_error && (
                      <p className="text-red-500 text-xs mt-1">⚠ {t('plugins.loadError')}: {plugin.load_error}</p>
                    )}
                    <div className="flex items-center gap-2 mt-2 flex-wrap">
                      {steps > 0 && (
                        <span className="inline-flex items-center px-2 py-0.5 rounded-full text-xs bg-gray-100 text-gray-700">
                          {t('plugins.steps')}: {steps}
                        </span>
                      )}
                      {triggers > 0 && (
                        <span className="inline-flex items-center px-2 py-0.5 rounded-full text-xs bg-gray-100 text-gray-700">
                          {t('plugins.triggers')}: {triggers}
                        </span>
                      )}
                      {uiExts > 0 && (
                        <span className="inline-flex items-center px-2 py-0.5 rounded-full text-xs bg-cyan-100 text-cyan-800">
                          {t('plugins.uiExtensions')}: {uiExts}
                        </span>
                      )}
                    </div>
                  </div>
                  <div className="flex gap-1 ml-4 flex-shrink-0">
                    <button
                      onClick={() => handleReload(plugin.id, plugin.name)}
                      disabled={reloadingId === plugin.id}
                      className="px-2 py-1 rounded text-sm bg-gray-100 text-gray-700 hover:bg-gray-200 transition disabled:opacity-50"
                      title={t('plugins.reload')}
                    >
                      ↻
                    </button>
                    <button
                      onClick={() => handleToggle(plugin.id, plugin.enabled)}
                      className={`px-3 py-1 rounded text-sm transition ${
                        plugin.enabled
                          ? 'bg-yellow-100 text-yellow-800 hover:bg-yellow-200'
                          : 'bg-green-100 text-green-800 hover:bg-green-200'
                      }`}
                    >
                      {plugin.enabled ? t('plugins.disable') : t('plugins.enable')}
                    </button>
                    <button
                      onClick={() => handleDelete(plugin.id)}
                      className="px-3 py-1 rounded text-sm bg-red-100 text-red-800 hover:bg-red-200 transition"
                    >
                      {t('plugins.uninstall')}
                    </button>
                  </div>
                </div>

                {(stepList.length > 0 || triggerList.length > 0 || uiExtList.length > 0) && (
                  <button
                    onClick={() => setExpandedId(isExpanded ? null : plugin.id)}
                    className="text-blue-500 text-sm mt-2 hover:underline"
                  >
                    {isExpanded ? `▲ ${t('plugins.hideDetails')}` : `▼ ${t('plugins.showDetails')}`}
                  </button>
                )}
              </div>

              {isExpanded && (
                <div className="border-t px-4 py-3 bg-gray-50">
                  {stepList.length > 0 && (
                    <div className="mb-3">
                      <div className="text-sm font-medium text-gray-600 mb-1">{t('plugins.steps')}:</div>
                      <div className="flex flex-wrap gap-1">
                        {stepList.map((s: string, i: number) => (
                          <span key={i} className="px-2 py-0.5 bg-gray-200 text-gray-700 rounded text-xs font-mono">
                            {s}
                          </span>
                        ))}
                      </div>
                    </div>
                  )}
                  {triggerList.length > 0 && (
                    <div className="mb-3">
                      <div className="text-sm font-medium text-gray-600 mb-1">{t('plugins.triggers')}:</div>
                      <div className="flex flex-wrap gap-1">
                        {triggerList.map((tr: string, i: number) => (
                          <span key={i} className="px-2 py-0.5 bg-amber-100 text-amber-800 rounded text-xs font-mono">
                            {tr}
                          </span>
                        ))}
                      </div>
                    </div>
                  )}
                  {uiExtList.length > 0 && (
                    <div>
                      <div className="text-sm font-medium text-gray-600 mb-1">{t('plugins.uiExtensions')}:</div>
                      <div className="space-y-1">
                        {uiExtList.map((ext: any, i: number) => (
                          <div key={i} className="text-xs bg-white p-2 rounded border">
                            <span className="font-mono text-purple-600">{ext.point}</span>
                            {' → '}
                            <span className="font-medium">{ext.label || ext.name}</span>
                          </div>
                        ))}
                      </div>
                    </div>
                  )}
                </div>
              )}
            </div>
          )
        })}
      </div>
    </div>
  )
}
