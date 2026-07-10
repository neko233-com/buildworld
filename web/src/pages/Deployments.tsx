import { useState } from 'react'
import { useI18n } from '../i18n'
import { api } from '../api'
import { useApi } from '../hooks'

export default function Deployments() {
  const { t } = useI18n()
  const { data: envs, loading, error, reload } = useApi(() => api.listDeploymentEnvs(), [])
  const [showForm, setShowForm] = useState(false)
  const [name, setName] = useState('')
  const [type, setType] = useState('dev')
  const [desc, setDesc] = useState('')
  const [buildIds, setBuildIds] = useState<Record<number, string>>({})
  const [busy, setBusy] = useState(false)

  const handleCreate = async () => {
    setBusy(true)
    try {
      await api.createDeploymentEnv({ name, type, description: desc })
      setName(''); setDesc(''); setShowForm(false); reload()
    } catch (e: any) { alert(e.message) }
    finally { setBusy(false) }
  }

  const handleDelete = async (id: number) => {
    if (!confirm('Delete this environment?')) return
    try { await api.deleteDeploymentEnv(id); reload() }
    catch (e: any) { alert(e.message) }
  }

  const handleDeploy = async (envId: number) => {
    const bid = buildIds[envId]
    if (!bid) return
    setBusy(true)
    try {
      await api.deployBuild(envId, Number(bid))
      setBuildIds({ ...buildIds, [envId]: '' })
      reload()
    } catch (e: any) { alert(e.message) }
    finally { setBusy(false) }
  }

  if (loading) return <div className="text-gray-500">{t('common.loading')}</div>
  if (error) return <div className="text-red-500">{t('common.error')}: {error}</div>

  return (
    <div>
      <div className="flex justify-between items-center mb-4">
        <h1 className="text-2xl font-bold">{t('deployments.title')}</h1>
        <button onClick={() => setShowForm(v => !v)} className="bg-blue-500 text-white px-4 py-2 rounded">{t('deployments.newEnv')}</button>
      </div>

      {showForm && (
        <div className="bg-white rounded-lg shadow border p-4 mb-4">
          <div className="grid grid-cols-1 md:grid-cols-3 gap-3">
            <div>
              <label className="text-sm text-gray-500">{t('deployments.name')}</label>
              <input value={name} onChange={e => setName(e.target.value)} className="w-full border rounded px-2 py-1 mt-1" />
            </div>
            <div>
              <label className="text-sm text-gray-500">{t('deployments.type')}</label>
              <select value={type} onChange={e => setType(e.target.value)} className="w-full border rounded px-2 py-1 mt-1">
                <option value="dev">dev</option>
                <option value="staging">staging</option>
                <option value="production">production</option>
              </select>
            </div>
            <div>
              <label className="text-sm text-gray-500">{t('deployments.description')}</label>
              <input value={desc} onChange={e => setDesc(e.target.value)} className="w-full border rounded px-2 py-1 mt-1" />
            </div>
          </div>
          <div className="flex gap-2 mt-3">
            <button onClick={handleCreate} disabled={busy || !name} className="bg-blue-500 text-white px-4 py-1 rounded disabled:opacity-50">{t('common.create')}</button>
            <button onClick={() => setShowForm(false)} className="border px-4 py-1 rounded">{t('common.cancel')}</button>
          </div>
        </div>
      )}

      <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-4">
        {(envs || []).length === 0 && (
          <p className="text-gray-500 col-span-full text-center py-6">{t('common.noData')}</p>
        )}
        {(envs || []).map((env: any) => (
          <div key={env.id} className="bg-white rounded-lg shadow border p-4">
            <div className="flex justify-between items-start mb-2">
              <div>
                <h3 className="font-semibold">{env.name}</h3>
                <span className="text-xs px-2 py-0.5 rounded bg-gray-100 text-gray-600">{env.type}</span>
              </div>
              <button onClick={() => handleDelete(env.id)} className="text-red-500 hover:underline text-sm">{t('common.delete')}</button>
            </div>
            {env.description && <p className="text-sm text-gray-500 mb-2">{env.description}</p>}
            <p className="text-xs text-gray-400 mb-3">{env.created_at ? new Date(env.created_at).toLocaleString() : ''}</p>

            <div className="border-t pt-3">
              <p className="text-sm font-medium mb-2">{t('deployments.deploy')}</p>
              <div className="flex gap-2">
                <input
                  type="number"
                  placeholder={t('deployments.buildId')}
                  value={buildIds[env.id] || ''}
                  onChange={e => setBuildIds({ ...buildIds, [env.id]: e.target.value })}
                  className="flex-1 border rounded px-2 py-1 text-sm"
                />
                <button onClick={() => handleDeploy(env.id)} disabled={busy || !buildIds[env.id]} className="bg-green-500 text-white px-3 py-1 rounded text-sm disabled:opacity-50">{t('deployments.deploy')}</button>
              </div>
            </div>

            {env.history && env.history.length > 0 && (
              <div className="border-t mt-3 pt-3">
                <p className="text-sm font-medium mb-1">{t('deployments.history')}</p>
                <div className="space-y-1 max-h-32 overflow-auto">
                  {env.history.slice(0, 5).map((h: any, i: number) => (
                    <div key={i} className="text-xs text-gray-500 flex justify-between gap-2">
                      <span>#{h.build_id}</span>
                      <span>{h.status}</span>
                      <span>{h.deployed_at ? new Date(h.deployed_at).toLocaleString() : ''}</span>
                    </div>
                  ))}
                </div>
              </div>
            )}
          </div>
        ))}
      </div>
    </div>
  )
}
