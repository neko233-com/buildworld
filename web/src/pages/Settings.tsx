import { useState } from 'react'
import { useI18n } from '../i18n'
import { api } from '../api'
import { useApi } from '../hooks'

function Field({ label, value, onChange, type = 'text' }: { label: string; value: any; onChange: (v: any) => void; type?: string }) {
  return (
    <div>
      <label className="text-sm text-gray-500">{label}</label>
      <input
        type={type}
        value={value ?? ''}
        onChange={e => onChange(type === 'number' ? Number(e.target.value) : e.target.value)}
        className="w-full border rounded px-2 py-1 mt-1"
      />
    </div>
  )
}

export default function Settings() {
  const { t } = useI18n()
  const { data: settings, loading, error, reload } = useApi(() => api.getGlobalSettings(), [])
  const { data: metrics } = useApi(() => api.getServerMetrics(), [])
  const [form, setForm] = useState<any>(null)
  const [saving, setSaving] = useState(false)

  const s = form ?? settings ?? {}
  const set = (key: string, val: any) => setForm({ ...s, [key]: val })

  const handleSave = async () => {
    setSaving(true)
    try {
      await api.updateGlobalSettings(form ?? settings)
      setForm(null)
      reload()
    } catch (e: any) {
      alert(e.message)
    } finally {
      setSaving(false)
    }
  }

  if (loading) return <div className="text-gray-500">{t('common.loading')}</div>
  if (error) return <div className="text-red-500">{t('common.error')}: {error}</div>

  const sys = metrics?.system || metrics || {}
  const uptime: number | undefined = sys.uptime_seconds ?? sys.uptime

  return (
    <div>
      <div className="flex justify-between items-center mb-4">
        <h1 className="text-2xl font-bold">{t('settings.title')}</h1>
        <button
          onClick={handleSave}
          disabled={saving || !form}
          className="bg-blue-500 text-white px-4 py-2 rounded disabled:opacity-50"
        >
          {t('common.save')}
        </button>
      </div>

      <div className="bg-white rounded-lg shadow border p-6 mb-4">
        <h2 className="text-lg font-semibold mb-3">{t('settings.serverConfig')}</h2>
        <div className="grid grid-cols-1 md:grid-cols-3 gap-3 items-end">
          <Field label={t('settings.port')} type="number" value={s.port} onChange={(v: any) => set('port', v)} />
          <Field label={t('settings.host')} value={s.host} onChange={(v: any) => set('host', v)} />
          <label className="flex items-center gap-2 text-sm pb-2">
            <input type="checkbox" checked={!!s.tls} onChange={e => set('tls', e.target.checked)} />
            {t('settings.tls')}
          </label>
        </div>
      </div>

      <div className="bg-white rounded-lg shadow border p-6 mb-4">
        <h2 className="text-lg font-semibold mb-3">{t('settings.buildDefaults')}</h2>
        <div className="grid grid-cols-1 md:grid-cols-3 gap-3">
          <Field label={t('settings.timeout')} type="number" value={s.build_timeout} onChange={(v: any) => set('build_timeout', v)} />
          <Field label={t('settings.concurrency')} type="number" value={s.build_concurrency} onChange={(v: any) => set('build_concurrency', v)} />
          <Field label={t('settings.retryPolicy')} value={s.retry_policy} onChange={(v: any) => set('retry_policy', v)} />
        </div>
      </div>

      <div className="bg-white rounded-lg shadow border p-6 mb-4">
        <h2 className="text-lg font-semibold mb-3">{t('settings.storagePaths')}</h2>
        <div className="grid grid-cols-1 md:grid-cols-2 gap-3">
          <Field label="Artifacts" value={s.artifacts_path} onChange={(v: any) => set('artifacts_path', v)} />
          <Field label="Logs" value={s.logs_path} onChange={(v: any) => set('logs_path', v)} />
        </div>
      </div>

      <div className="bg-white rounded-lg shadow border p-6">
        <h2 className="text-lg font-semibold mb-3">{t('settings.systemInfo')}</h2>
        <div className="grid grid-cols-1 md:grid-cols-2 gap-x-8">
          <div className="flex justify-between border-b py-2">
            <span className="text-gray-600">{t('settings.version')}</span>
            <span className="font-mono">{sys.version ?? '-'}</span>
          </div>
          <div className="flex justify-between border-b py-2">
            <span className="text-gray-600">{t('settings.uptime')}</span>
            <span className="font-mono">
              {typeof uptime === 'number' ? `${Math.floor(uptime / 3600)}h ${Math.floor((uptime % 3600) / 60)}m` : '-'}
            </span>
          </div>
        </div>
      </div>
    </div>
  )
}
