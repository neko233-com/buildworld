import { useState } from 'react'
import { useI18n } from '../i18n'
import { api } from '../api'
import { useApi } from '../hooks'

export default function APITokens() {
  const { t } = useI18n()
  const { data: tokens, loading, error, reload } = useApi(() => api.listAPITokens(), [])
  const [showForm, setShowForm] = useState(false)
  const [name, setName] = useState('')
  const [scopes, setScopes] = useState('')
  const [expires, setExpires] = useState('')
  const [creating, setCreating] = useState(false)
  const [newToken, setNewToken] = useState('')

  const handleCreate = async () => {
    setCreating(true)
    try {
      const data: any = { name }
      if (scopes) data.scopes = scopes.split(',').map(s => s.trim()).filter(Boolean)
      if (expires) data.expires_at = new Date(expires).toISOString()
      const resp = await api.createAPIToken(data)
      setNewToken(resp.token || resp.value || JSON.stringify(resp))
      setName(''); setScopes(''); setExpires(''); setShowForm(false)
      reload()
    } catch (e: any) {
      alert(e.message)
    } finally {
      setCreating(false)
    }
  }

  const handleDelete = async (id: number) => {
    if (!confirm('Delete this token?')) return
    try { await api.deleteAPIToken(id); reload() }
    catch (e: any) { alert(e.message) }
  }

  if (loading) return <div className="text-gray-500">{t('common.loading')}</div>
  if (error) return <div className="text-red-500">{t('common.error')}: {error}</div>

  return (
    <div>
      <div className="flex justify-between items-center mb-4">
        <h1 className="text-2xl font-bold">{t('apiTokens.title')}</h1>
        <button onClick={() => setShowForm(v => !v)} className="bg-blue-500 text-white px-4 py-2 rounded">{t('apiTokens.new')}</button>
      </div>

      {newToken && (
        <div className="bg-yellow-50 border border-yellow-300 rounded-lg p-4 mb-4">
          <p className="text-sm text-yellow-800 mb-2">{t('apiTokens.tokenOnce')}</p>
          <div className="flex gap-2">
            <code className="flex-1 bg-white border rounded px-3 py-2 font-mono text-sm break-all">{newToken}</code>
            <button onClick={() => { navigator.clipboard?.writeText(newToken); setNewToken('') }} className="bg-yellow-500 text-white px-3 py-2 rounded text-sm whitespace-nowrap">{t('common.copy')}</button>
          </div>
        </div>
      )}

      {showForm && (
        <div className="bg-white rounded-lg shadow border p-4 mb-4">
          <div className="grid grid-cols-1 md:grid-cols-3 gap-3">
            <div>
              <label className="text-sm text-gray-500">{t('apiTokens.name')}</label>
              <input value={name} onChange={e => setName(e.target.value)} className="w-full border rounded px-2 py-1 mt-1" />
            </div>
            <div>
              <label className="text-sm text-gray-500">{t('apiTokens.scopes')}</label>
              <input value={scopes} onChange={e => setScopes(e.target.value)} placeholder="read,write" className="w-full border rounded px-2 py-1 mt-1" />
            </div>
            <div>
              <label className="text-sm text-gray-500">{t('apiTokens.expiresAt')}</label>
              <input type="datetime-local" value={expires} onChange={e => setExpires(e.target.value)} className="w-full border rounded px-2 py-1 mt-1" />
            </div>
          </div>
          <div className="flex gap-2 mt-3">
            <button onClick={handleCreate} disabled={creating || !name} className="bg-blue-500 text-white px-4 py-1 rounded disabled:opacity-50">{t('apiTokens.create')}</button>
            <button onClick={() => setShowForm(false)} className="border px-4 py-1 rounded">{t('common.cancel')}</button>
          </div>
        </div>
      )}

      <div className="bg-white shadow rounded-lg">
        <table className="min-w-full">
          <thead>
            <tr className="border-b">
              <th className="px-6 py-3 text-left text-sm font-medium text-gray-500">{t('apiTokens.name')}</th>
              <th className="px-6 py-3 text-left text-sm font-medium text-gray-500">{t('apiTokens.prefix')}</th>
              <th className="px-6 py-3 text-left text-sm font-medium text-gray-500">{t('apiTokens.scopes')}</th>
              <th className="px-6 py-3 text-left text-sm font-medium text-gray-500">{t('apiTokens.expiresAt')}</th>
              <th className="px-6 py-3 text-left text-sm font-medium text-gray-500">{t('apiTokens.createdAt')}</th>
              <th className="px-6 py-3 text-left text-sm font-medium text-gray-500">{t('projects.actions')}</th>
            </tr>
          </thead>
          <tbody>
            {(tokens || []).length === 0 && (
              <tr><td colSpan={6} className="px-6 py-4 text-gray-500">{t('common.noData')}</td></tr>
            )}
            {(tokens || []).map((tk: any) => (
              <tr key={tk.id} className="border-b">
                <td className="px-6 py-4 font-medium">{tk.name}</td>
                <td className="px-6 py-4 font-mono text-sm text-gray-500">{tk.prefix}…</td>
                <td className="px-6 py-4 text-sm text-gray-500">{Array.isArray(tk.scopes) ? tk.scopes.join(', ') : (tk.scopes || '-')}</td>
                <td className="px-6 py-4 text-sm text-gray-500">{tk.expires_at ? new Date(tk.expires_at).toLocaleString() : t('apiTokens.noExpiry')}</td>
                <td className="px-6 py-4 text-sm text-gray-500">{tk.created_at ? new Date(tk.created_at).toLocaleString() : '-'}</td>
                <td className="px-6 py-4">
                  <button onClick={() => handleDelete(tk.id)} className="text-red-500 hover:underline text-sm">{t('common.delete')}</button>
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>
    </div>
  )
}
