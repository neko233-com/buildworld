import { useState, useEffect } from 'react'
import { useI18n } from '../i18n'
import { api } from '../api'
import { useApi } from '../hooks'
import { compileToJS, TS_TEMPLATE, TSX_TEMPLATE, type ScriptLang } from '../ts-compiler'

interface FormState {
  name: string
  version: string
  description: string
  author: string
  script: string
  uiScript: string
  scriptLang: ScriptLang
  uiScriptLang: ScriptLang | 'none'
}

interface EditState {
  pluginName: string
  script: string
  uiScript: string
  scriptLang: ScriptLang
  uiScriptLang: ScriptLang | 'none'
}

export default function Plugins() {
  const { t } = useI18n()
  const { data: plugins, loading, error, reload } = useApi(() => api.listPlugins())
  const [filter, setFilter] = useState<'all' | 'enabled' | 'disabled'>('all')
  const [expandedId, setExpandedId] = useState<number | null>(null)

  const [showForm, setShowForm] = useState(false)
  const [form, setForm] = useState<FormState>({
    name: '', version: '', description: '', author: '', script: '', uiScript: '',
    scriptLang: 'js', uiScriptLang: 'none'
  })
  const [saving, setSaving] = useState(false)
  const [formError, setFormError] = useState('')
  const [formCompileStatus, setFormCompileStatus] = useState<{ script?: { ok: boolean; error?: string; size?: number }; ui?: { ok: boolean; error?: string; size?: number } }>({})
  const [reloadingId, setReloadingId] = useState<number | null>(null)

  const [showEdit, setShowEdit] = useState(false)
  const [edit, setEdit] = useState<EditState>({ pluginName: '', script: '', uiScript: '', scriptLang: 'js', uiScriptLang: 'none' })
  const [editSaving, setEditSaving] = useState(false)
  const [editError, setEditError] = useState('')
  const [editCompileStatus, setEditCompileStatus] = useState<{ script?: { ok: boolean; error?: string; size?: number }; ui?: { ok: boolean; error?: string; size?: number } }>({})
  const [editLoading, setEditLoading] = useState(false)

  useEffect(() => {
    if (!form.script.trim()) {
      setFormCompileStatus(s => ({ ...s, script: undefined }))
      return
    }
    const result = compileToJS(form.script, form.scriptLang)
    if (result.error) {
      setFormCompileStatus(s => ({ ...s, script: { ok: false, error: result.error } }))
    } else {
      setFormCompileStatus(s => ({ ...s, script: { ok: true, size: result.code.length } }))
    }
  }, [form.script, form.scriptLang])

  useEffect(() => {
    if (!form.uiScript.trim() || form.uiScriptLang === 'none') {
      setFormCompileStatus(s => ({ ...s, ui: undefined }))
      return
    }
    const result = compileToJS(form.uiScript, form.uiScriptLang as ScriptLang)
    if (result.error) {
      setFormCompileStatus(s => ({ ...s, ui: { ok: false, error: result.error } }))
    } else {
      setFormCompileStatus(s => ({ ...s, ui: { ok: true, size: result.code.length } }))
    }
  }, [form.uiScript, form.uiScriptLang])

  useEffect(() => {
    if (!edit.script.trim()) {
      setEditCompileStatus(s => ({ ...s, script: undefined }))
      return
    }
    const result = compileToJS(edit.script, edit.scriptLang)
    if (result.error) {
      setEditCompileStatus(s => ({ ...s, script: { ok: false, error: result.error } }))
    } else {
      setEditCompileStatus(s => ({ ...s, script: { ok: true, size: result.code.length } }))
    }
  }, [edit.script, edit.scriptLang])

  useEffect(() => {
    if (!edit.uiScript.trim() || edit.uiScriptLang === 'none') {
      setEditCompileStatus(s => ({ ...s, ui: undefined }))
      return
    }
    const result = compileToJS(edit.uiScript, edit.uiScriptLang as ScriptLang)
    if (result.error) {
      setEditCompileStatus(s => ({ ...s, ui: { ok: false, error: result.error } }))
    } else {
      setEditCompileStatus(s => ({ ...s, ui: { ok: true, size: result.code.length } }))
    }
  }, [edit.uiScript, edit.uiScriptLang])

  const getScriptLabel = (lang: ScriptLang) => {
    const ext = lang === 'js' ? 'js' : lang === 'ts' ? 'ts' : 'tsx'
    return `${t('plugins.script').split(' ')[0]} (index.${ext})`
  }

  const getUIScriptLabel = (lang: ScriptLang | 'none') => {
    if (lang === 'none') return t('plugins.uiScriptNone')
    const ext = lang === 'js' ? 'js' : lang === 'tsx' ? 'tsx' : 'tsx'
    return `${t('plugins.uiScript').split(' ')[0]} (ui.${ext})`
  }

  const getScriptPlaceholder = (lang: ScriptLang) => {
    if (lang === 'ts') return '// plugin main script (index.ts) - TypeScript supported'
    if (lang === 'tsx') return '// plugin main script (index.tsx)'
    return '// plugin main script (index.js)'
  }

  const getUIScriptPlaceholder = (lang: ScriptLang | 'none') => {
    if (lang === 'tsx') return '// UI extension script (ui.tsx) - JSX supported'
    return '// UI extension script (ui.js) - use React.createElement'
  }

  const handleInstall = async (e: React.FormEvent) => {
    e.preventDefault()
    setSaving(true)
    setFormError('')
    try {
      let compiledScript = form.script
      if (form.scriptLang !== 'js' && form.script.trim()) {
        const result = compileToJS(form.script, form.scriptLang)
        if (result.error) {
          setFormError(`${t('plugins.compileError')}: ${result.error}`)
          setSaving(false)
          return
        }
        compiledScript = result.code
      }

      let compiledUIScript = form.uiScript
      if (form.uiScriptLang !== 'none' && form.uiScriptLang !== 'js' && form.uiScript.trim()) {
        const result = compileToJS(form.uiScript, form.uiScriptLang as ScriptLang)
        if (result.error) {
          setFormError(`${t('plugins.compileError')} (UI): ${result.error}`)
          setSaving(false)
          return
        }
        compiledUIScript = result.code
      }

      const data: any = { name: form.name, source: 'upload', script_lang: form.scriptLang }
      if (form.version) data.version = form.version
      if (form.description) data.description = form.description
      if (form.author) data.author = form.author
      if (form.script) {
        data.script = compiledScript
        data.source_script = form.script
      }
      if (form.uiScript && form.uiScriptLang !== 'none') {
        data.ui_script = compiledUIScript
        data.source_ui_script = form.uiScript
        data.ui_script_lang = form.uiScriptLang
      }
      await api.installPlugin(data)
      setForm({ name: '', version: '', description: '', author: '', script: '', uiScript: '', scriptLang: 'js', uiScriptLang: 'none' })
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

  const handleEdit = async (plugin: any) => {
    if (plugin.source === 'builtin') {
      alert(t('plugins.builtinNoEdit'))
      return
    }
    setEditLoading(true)
    setEditError('')
    setShowEdit(true)
    try {
      const source = await api.getPluginSource(plugin.name)
      setEdit({
        pluginName: plugin.name,
        script: source.source_script || source.script || '',
        uiScript: source.source_ui_script || source.ui_script || '',
        scriptLang: (source.script_lang as ScriptLang) || 'js',
        uiScriptLang: (source.ui_script_lang as ScriptLang | 'none') || (source.ui_script ? 'js' : 'none')
      })
    } catch (e: any) {
      setEditError(e.message || 'Failed to load source')
      setEdit({
        pluginName: plugin.name,
        script: '',
        uiScript: '',
        scriptLang: 'js',
        uiScriptLang: 'none'
      })
    } finally {
      setEditLoading(false)
    }
  }

  const handleSaveEdit = async () => {
    setEditSaving(true)
    setEditError('')
    try {
      let compiledScript = edit.script
      if (edit.scriptLang !== 'js' && edit.script.trim()) {
        const result = compileToJS(edit.script, edit.scriptLang)
        if (result.error) {
          setEditError(`${t('plugins.compileError')}: ${result.error}`)
          setEditSaving(false)
          return
        }
        compiledScript = result.code
      }

      let compiledUIScript = edit.uiScript
      if (edit.uiScriptLang !== 'none' && edit.uiScriptLang !== 'js' && edit.uiScript.trim()) {
        const result = compileToJS(edit.uiScript, edit.uiScriptLang as ScriptLang)
        if (result.error) {
          setEditError(`${t('plugins.compileError')} (UI): ${result.error}`)
          setEditSaving(false)
          return
        }
        compiledUIScript = result.code
      }

      const data: any = { script_lang: edit.scriptLang }
      if (edit.script) {
        data.script = compiledScript
        data.source_script = edit.script
      }
      if (edit.uiScript && edit.uiScriptLang !== 'none') {
        data.ui_script = compiledUIScript
        data.source_ui_script = edit.uiScript
        data.ui_script_lang = edit.uiScriptLang
      } else {
        data.ui_script = ''
        data.source_ui_script = ''
        data.ui_script_lang = 'none'
      }
      await api.updatePluginSource(edit.pluginName, data)
      setShowEdit(false)
      reload()
    } catch (err: any) {
      setEditError(err.message || 'Failed to save')
    } finally {
      setEditSaving(false)
    }
  }

  const insertTemplate = (isUI: boolean) => {
    if (isUI) {
      setForm({ ...form, uiScript: TSX_TEMPLATE, uiScriptLang: 'tsx' })
    } else {
      setForm({ ...form, script: TS_TEMPLATE, scriptLang: 'ts' })
    }
  }

  const insertEditTemplate = (isUI: boolean) => {
    if (isUI) {
      setEdit({ ...edit, uiScript: TSX_TEMPLATE, uiScriptLang: 'tsx' })
    } else {
      setEdit({ ...edit, script: TS_TEMPLATE, scriptLang: 'ts' })
    }
  }

  const getPluginExtCount = (plugin: any) => {
    const steps = plugin.steps?.length || plugin.registered_steps?.length || 0
    const triggers = plugin.triggers?.length || plugin.registered_triggers?.length || 0
    const uiExts = plugin.ui_extensions?.length || 0
    return { steps, triggers, uiExts }
  }

  const renderCompileStatus = (status: { ok: boolean; error?: string; size?: number } | undefined) => {
    if (!status) return null
    if (!status.ok) {
      return <p className="text-red-400 text-xs mt-1 bg-red-900/30 p-2 rounded border border-red-800">❌ {t('plugins.compileError')}: {status.error}</p>
    }
    return <p className="text-green-400 text-xs mt-1">✓ {t('plugins.compileOk')} ({t('plugins.compiledSize')}: {status.size} bytes)</p>
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
        <form onSubmit={handleInstall} className="bg-white shadow rounded-lg p-6 mb-6 space-y-4 max-w-4xl">
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
            <div className="grid grid-cols-2 gap-4 mb-2">
              <div>
                <label className="block text-sm font-medium text-gray-700 mb-1">{t('plugins.scriptLang')}</label>
                <select
                  value={form.scriptLang}
                  onChange={e => setForm({ ...form, scriptLang: e.target.value as ScriptLang })}
                  className="w-full border rounded px-3 py-2"
                >
                  <option value="js">{t('plugins.langJs')}</option>
                  <option value="ts">{t('plugins.langTs')}</option>
                </select>
              </div>
              <div className="flex items-end">
                <button
                  type="button"
                  onClick={() => insertTemplate(false)}
                  className="text-sm px-3 py-2 bg-gray-100 hover:bg-gray-200 rounded border"
                >
                  {t('plugins.insertTemplate')}
                </button>
              </div>
            </div>
            <label className="block text-sm font-medium text-gray-700 mb-1">{getScriptLabel(form.scriptLang)}</label>
            <textarea
              value={form.script}
              onChange={e => setForm({ ...form, script: e.target.value })}
              className="w-full rounded px-3 py-2 font-mono text-sm bg-gray-900 text-green-300 border border-gray-700 min-h-[200px]"
              rows={10}
              placeholder={getScriptPlaceholder(form.scriptLang)}
              spellCheck={false}
            />
            {renderCompileStatus(formCompileStatus.script)}
          </div>
          <div>
            <div className="grid grid-cols-2 gap-4 mb-2">
              <div>
                <label className="block text-sm font-medium text-gray-700 mb-1">{t('plugins.uiLang')}</label>
                <select
                  value={form.uiScriptLang}
                  onChange={e => setForm({ ...form, uiScriptLang: e.target.value as ScriptLang | 'none' })}
                  className="w-full border rounded px-3 py-2"
                >
                  <option value="none">{t('plugins.langNone')}</option>
                  <option value="js">{t('plugins.langJs')}</option>
                  <option value="tsx">{t('plugins.langTsx')}</option>
                </select>
              </div>
              <div className="flex items-end">
                {form.uiScriptLang !== 'none' && (
                  <button
                    type="button"
                    onClick={() => insertTemplate(true)}
                    className="text-sm px-3 py-2 bg-gray-100 hover:bg-gray-200 rounded border"
                  >
                    {t('plugins.insertTemplate')}
                  </button>
                )}
              </div>
            </div>
            {form.uiScriptLang !== 'none' && (
              <>
                <label className="block text-sm font-medium text-gray-700 mb-1">{getUIScriptLabel(form.uiScriptLang)}</label>
                <textarea
                  value={form.uiScript}
                  onChange={e => setForm({ ...form, uiScript: e.target.value })}
                  className="w-full rounded px-3 py-2 font-mono text-sm bg-gray-900 text-green-300 border border-gray-700 min-h-[200px]"
                  rows={8}
                  placeholder={getUIScriptPlaceholder(form.uiScriptLang)}
                  spellCheck={false}
                />
                <p className="text-xs text-gray-500 mt-1">Note: ui script receives (React, __BW_PLUGINS__) as parameters.</p>
                {renderCompileStatus(formCompileStatus.ui)}
              </>
            )}
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
          const isBuiltin = plugin.source === 'builtin'

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
                        isBuiltin
                          ? 'bg-purple-100 text-purple-800'
                          : 'bg-blue-100 text-blue-800'
                      }`}>
                        {isBuiltin ? t('plugins.builtin') : t('plugins.upload')}
                      </span>
                      {plugin.script_lang && plugin.script_lang !== 'js' && (
                        <span className="px-2 py-0.5 rounded text-xs font-medium bg-orange-100 text-orange-800">
                          {plugin.script_lang.toUpperCase()}
                        </span>
                      )}
                    </div>
                    <p className="text-gray-600 mt-1 text-sm">{plugin.description}</p>
                    {!isBuiltin && plugin.author && (
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
                      onClick={() => handleEdit(plugin)}
                      disabled={isBuiltin}
                      className={`px-2 py-1 rounded text-sm transition ${
                        isBuiltin
                          ? 'bg-gray-100 text-gray-400 cursor-not-allowed'
                          : 'bg-gray-100 text-gray-700 hover:bg-gray-200'
                      }`}
                      title={isBuiltin ? t('plugins.builtinNoEdit') : t('plugins.editSource')}
                    >
                      ✏️
                    </button>
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
                    {!isBuiltin && (
                      <button
                        onClick={() => handleDelete(plugin.id)}
                        className="px-3 py-1 rounded text-sm bg-red-100 text-red-800 hover:bg-red-200 transition"
                      >
                        {t('plugins.uninstall')}
                      </button>
                    )}
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

      {showEdit && (
        <div className="fixed inset-0 bg-black/50 flex items-center justify-center z-50 p-4">
          <div className="bg-white rounded-lg shadow-xl w-full max-w-4xl max-h-[90vh] overflow-y-auto">
            <div className="p-6 border-b">
              <div className="flex justify-between items-center">
                <h2 className="text-xl font-bold">{t('plugins.editSource')}: {edit.pluginName}</h2>
                <button
                  onClick={() => setShowEdit(false)}
                  className="text-gray-500 hover:text-gray-700 text-2xl"
                >
                  ×
                </button>
              </div>
            </div>
            <div className="p-6 space-y-4">
              {editLoading ? (
                <div className="text-gray-500 py-8 text-center">{t('common.loading')}</div>
              ) : (
                <>
                  <div>
                    <div className="grid grid-cols-2 gap-4 mb-2">
                      <div>
                        <label className="block text-sm font-medium text-gray-700 mb-1">{t('plugins.scriptLang')}</label>
                        <select
                          value={edit.scriptLang}
                          onChange={e => setEdit({ ...edit, scriptLang: e.target.value as ScriptLang })}
                          className="w-full border rounded px-3 py-2"
                        >
                          <option value="js">{t('plugins.langJs')}</option>
                          <option value="ts">{t('plugins.langTs')}</option>
                        </select>
                      </div>
                      <div className="flex items-end">
                        <button
                          type="button"
                          onClick={() => insertEditTemplate(false)}
                          className="text-sm px-3 py-2 bg-gray-100 hover:bg-gray-200 rounded border"
                        >
                          {t('plugins.insertTemplate')}
                        </button>
                      </div>
                    </div>
                    <label className="block text-sm font-medium text-gray-700 mb-1">{getScriptLabel(edit.scriptLang)}</label>
                    <textarea
                      value={edit.script}
                      onChange={e => setEdit({ ...edit, script: e.target.value })}
                      className="w-full rounded px-3 py-2 font-mono text-sm bg-gray-900 text-green-300 border border-gray-700 min-h-[200px]"
                      rows={12}
                      spellCheck={false}
                    />
                    {renderCompileStatus(editCompileStatus.script)}
                  </div>
                  <div>
                    <div className="grid grid-cols-2 gap-4 mb-2">
                      <div>
                        <label className="block text-sm font-medium text-gray-700 mb-1">{t('plugins.uiLang')}</label>
                        <select
                          value={edit.uiScriptLang}
                          onChange={e => setEdit({ ...edit, uiScriptLang: e.target.value as ScriptLang | 'none' })}
                          className="w-full border rounded px-3 py-2"
                        >
                          <option value="none">{t('plugins.langNone')}</option>
                          <option value="js">{t('plugins.langJs')}</option>
                          <option value="tsx">{t('plugins.langTsx')}</option>
                        </select>
                      </div>
                      <div className="flex items-end">
                        {edit.uiScriptLang !== 'none' && (
                          <button
                            type="button"
                            onClick={() => insertEditTemplate(true)}
                            className="text-sm px-3 py-2 bg-gray-100 hover:bg-gray-200 rounded border"
                          >
                            {t('plugins.insertTemplate')}
                          </button>
                        )}
                      </div>
                    </div>
                    {edit.uiScriptLang !== 'none' && (
                      <>
                        <label className="block text-sm font-medium text-gray-700 mb-1">{getUIScriptLabel(edit.uiScriptLang)}</label>
                        <textarea
                          value={edit.uiScript}
                          onChange={e => setEdit({ ...edit, uiScript: e.target.value })}
                          className="w-full rounded px-3 py-2 font-mono text-sm bg-gray-900 text-green-300 border border-gray-700 min-h-[200px]"
                          rows={10}
                          spellCheck={false}
                        />
                        <p className="text-xs text-gray-500 mt-1">Note: ui script receives (React, __BW_PLUGINS__) as parameters.</p>
                        {renderCompileStatus(editCompileStatus.ui)}
                      </>
                    )}
                  </div>
                  {editError && <p className="text-red-500 text-sm">{editError}</p>}
                </>
              )}
            </div>
            <div className="p-6 border-t flex gap-2 justify-end">
              <button
                onClick={() => setShowEdit(false)}
                className="bg-gray-300 text-gray-700 px-4 py-2 rounded hover:bg-gray-400 transition"
              >
                {t('common.cancel')}
              </button>
              <button
                onClick={handleSaveEdit}
                disabled={editSaving || editLoading}
                className="bg-blue-500 text-white px-4 py-2 rounded disabled:opacity-50 hover:bg-blue-600 transition"
              >
                {editSaving ? t('common.loading') : t('common.save')}
              </button>
            </div>
          </div>
        </div>
      )}
    </div>
  )
}
