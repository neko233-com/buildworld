import { useState, useCallback } from 'react'
import { useI18n } from '../i18n'
import { useApi } from '../hooks'
import { api } from '../api'

type ChannelType = 'email' | 'feishu' | 'webhook'

interface NotificationChannel {
  id: number
  name: string
  type: ChannelType
  config: string
  conditions: string
  description: string
  enabled: boolean
  created_at: string
  updated_at: string
}

export default function Notifications() {
  const { t } = useI18n()
  const { data: channels, reload } = useApi<NotificationChannel[]>(() => api.listNotificationChannels())
  const [showModal, setShowModal] = useState(false)
  const [editing, setEditing] = useState<NotificationChannel | null>(null)
  const [showEvents, setShowEvents] = useState<number | null>(null)
  const [events, setEvents] = useState<any[]>([])

  const [form, setForm] = useState({
    name: '',
    type: 'webhook' as ChannelType,
    config: '',
    conditions: '',
    description: '',
    enabled: true,
  })

  const [emailConfig, setEmailConfig] = useState({
    smtp_host: '',
    smtp_port: 587,
    smtp_user: '',
    smtp_password: '',
    from: '',
    to: '',
  })

  const [feishuConfig, setFeishuConfig] = useState({
    webhook_url: '',
  })

  const [webhookConfig, setWebhookConfig] = useState({
    url: '',
    method: 'POST',
    headers: '',
  })

  const resetForm = useCallback(() => {
    setForm({ name: '', type: 'webhook', config: '', conditions: '', description: '', enabled: true })
    setEmailConfig({ smtp_host: '', smtp_port: 587, smtp_user: '', smtp_password: '', from: '', to: '' })
    setFeishuConfig({ webhook_url: '' })
    setWebhookConfig({ url: '', method: 'POST', headers: '' })
  }, [])

  const openCreate = () => {
    resetForm()
    setEditing(null)
    setShowModal(true)
  }

  const openEdit = (channel: NotificationChannel) => {
    setEditing(channel)
    setForm({
      name: channel.name,
      type: channel.type,
      config: channel.config,
      conditions: channel.conditions,
      description: channel.description,
      enabled: channel.enabled,
    })
    try {
      const cfg = JSON.parse(channel.config)
      if (channel.type === 'email') {
        setEmailConfig({
          smtp_host: cfg.smtp_host || '',
          smtp_port: cfg.smtp_port || 587,
          smtp_user: cfg.smtp_user || '',
          smtp_password: cfg.smtp_password || '',
          from: cfg.from || '',
          to: Array.isArray(cfg.to) ? cfg.to.join(',') : '',
        })
      } else if (channel.type === 'feishu') {
        setFeishuConfig({ webhook_url: cfg.webhook_url || '' })
      } else {
        setWebhookConfig({
          url: cfg.url || '',
          method: cfg.method || 'POST',
          headers: cfg.headers ? JSON.stringify(cfg.headers, null, 2) : '',
        })
      }
    } catch {
    }
    setShowModal(true)
  }

  const handleSubmit = async () => {
    let configStr = '{}'
    if (form.type === 'email') {
      configStr = JSON.stringify({
        smtp_host: emailConfig.smtp_host,
        smtp_port: emailConfig.smtp_port,
        smtp_user: emailConfig.smtp_user,
        smtp_password: emailConfig.smtp_password,
        from: emailConfig.from,
        to: emailConfig.to.split(',').map(s => s.trim()).filter(Boolean),
      })
    } else if (form.type === 'feishu') {
      configStr = JSON.stringify({ webhook_url: feishuConfig.webhook_url })
    } else {
      let headers: Record<string, string> = {}
      try { headers = JSON.parse(webhookConfig.headers) || {} } catch { }
      configStr = JSON.stringify({ url: webhookConfig.url, method: webhookConfig.method, headers })
    }

    const data = { ...form, config: configStr }
    if (editing) {
      await api.updateNotificationChannel(editing.id, data)
    } else {
      await api.createNotificationChannel(data)
    }
    setShowModal(false)
    reload()
  }

  const handleDelete = async (id: number) => {
    if (!confirm(t('common.confirm'))) return
    await api.deleteNotificationChannel(id)
    reload()
  }

  const fetchEvents = async (id: number) => {
    const evts = await api.listNotificationEvents(id)
    setEvents(evts)
    setShowEvents(id)
  }

  const getTypeLabel = (type: ChannelType) => {
    const map: Record<ChannelType, string> = { email: 'Email', feishu: 'Feishu', webhook: 'Webhook' }
    return map[type] || type
  }

  const getTypeColor = (type: ChannelType) => {
    const map: Record<ChannelType, string> = {
      email: 'bg-blue-100 text-blue-800',
      feishu: 'bg-green-100 text-green-800',
      webhook: 'bg-purple-100 text-purple-800',
    }
    return map[type] || 'bg-gray-100 text-gray-800'
  }

  return (
    <div className="space-y-4">
      <div className="flex justify-between items-center">
        <h1 className="text-2xl font-bold">{t('settings.notifications')}</h1>
        <button onClick={openCreate} className="px-4 py-2 bg-blue-600 text-white rounded hover:bg-blue-700">
          {t('common.save')}
        </button>
      </div>

      <div className="bg-white rounded-lg shadow border overflow-hidden">
        <table className="w-full">
          <thead className="bg-gray-50">
            <tr>
              <th className="px-4 py-3 text-left font-medium text-gray-700">{t('credentials.name')}</th>
              <th className="px-4 py-3 text-left font-medium text-gray-700">{t('credentials.type')}</th>
              <th className="px-4 py-3 text-left font-medium text-gray-700">{t('credentials.description')}</th>
              <th className="px-4 py-3 text-left font-medium text-gray-700">Enabled</th>
              <th className="px-4 py-3 text-left font-medium text-gray-700">Created</th>
              <th className="px-4 py-3 text-left font-medium text-gray-700">{t('credentials.actions')}</th>
            </tr>
          </thead>
          <tbody>
            {channels?.map(ch => (
              <tr key={ch.id} className="border-t hover:bg-gray-50">
                <td className="px-4 py-3 font-medium">{ch.name}</td>
                <td className="px-4 py-3">
                  <span className={`inline-flex px-2 py-1 rounded text-xs font-medium ${getTypeColor(ch.type)}`}>
                    {getTypeLabel(ch.type)}
                  </span>
                </td>
                <td className="px-4 py-3 text-gray-600">{ch.description || '-'}</td>
                <td className="px-4 py-3">
                  <span className={`inline-flex px-2 py-1 rounded text-xs font-medium ${ch.enabled ? 'bg-green-100 text-green-800' : 'bg-gray-100 text-gray-800'}`}>
                    {ch.enabled ? 'Yes' : 'No'}
                  </span>
                </td>
                <td className="px-4 py-3 text-gray-500 text-sm">{new Date(ch.created_at).toLocaleString()}</td>
                <td className="px-4 py-3">
                  <div className="flex gap-2">
                    <button onClick={() => openEdit(ch)} className="text-blue-600 hover:text-blue-800 text-sm">Edit</button>
                    <button onClick={() => fetchEvents(ch.id)} className="text-green-600 hover:text-green-800 text-sm">Events</button>
                    <button onClick={() => handleDelete(ch.id)} className="text-red-600 hover:text-red-800 text-sm">Delete</button>
                  </div>
                </td>
              </tr>
            ))}
            {!channels?.length && (
              <tr>
                <td colSpan={6} className="px-4 py-8 text-center text-gray-500">{t('common.noData')}</td>
              </tr>
            )}
          </tbody>
        </table>
      </div>

      {showEvents !== null && (
        <div className="bg-white rounded-lg shadow border p-4">
          <div className="flex justify-between items-center mb-4">
            <h2 className="text-lg font-semibold">Notification Events</h2>
            <button onClick={() => setShowEvents(null)} className="text-gray-600 hover:text-gray-800">Close</button>
          </div>
          <div className="space-y-2 max-h-64 overflow-y-auto">
            {events.map(e => (
              <div key={e.id} className={`p-3 rounded border ${e.status === 'delivered' ? 'border-green-200 bg-green-50' : e.status === 'failed' ? 'border-red-200 bg-red-50' : 'border-gray-200 bg-gray-50'}`}>
                <div className="flex justify-between items-start">
                  <div>
                    <div className="font-medium">{e.event_type}</div>
                    <div className="text-sm text-gray-600">{new Date(e.created_at).toLocaleString()}</div>
                  </div>
                  <span className={`px-2 py-1 rounded text-xs font-medium ${e.status === 'delivered' ? 'bg-green-100 text-green-800' : e.status === 'failed' ? 'bg-red-100 text-red-800' : 'bg-gray-100 text-gray-800'}`}>
                    {e.status}
                  </span>
                </div>
                {e.error_message && <div className="mt-2 text-sm text-red-600">{e.error_message}</div>}
              </div>
            ))}
            {!events.length && <div className="text-center text-gray-500 py-4">No events</div>}
          </div>
        </div>
      )}

      {showModal && (
        <div className="fixed inset-0 bg-black/50 flex items-center justify-center z-50">
          <div className="bg-white rounded-lg shadow-xl w-full max-w-lg p-6">
            <h2 className="text-xl font-bold mb-4">{editing ? 'Edit Channel' : 'New Channel'}</h2>
            <div className="space-y-4">
              <div>
                <label className="block text-sm font-medium text-gray-700 mb-1">Name</label>
                <input
                  type="text"
                  value={form.name}
                  onChange={e => setForm({ ...form, name: e.target.value })}
                  className="w-full border rounded px-3 py-2"
                  required
                />
              </div>
              <div>
                <label className="block text-sm font-medium text-gray-700 mb-1">Type</label>
                <select
                  value={form.type}
                  onChange={e => setForm({ ...form, type: e.target.value as ChannelType })}
                  className="w-full border rounded px-3 py-2"
                >
                  <option value="email">Email</option>
                  <option value="feishu">Feishu</option>
                  <option value="webhook">Webhook</option>
                </select>
              </div>
              {form.type === 'email' && (
                <div className="border rounded p-3 space-y-2">
                  <div className="grid grid-cols-2 gap-2">
                    <input placeholder="SMTP Host" value={emailConfig.smtp_host} onChange={e => setEmailConfig({ ...emailConfig, smtp_host: e.target.value })} className="border rounded px-2 py-1 text-sm" />
                    <input type="number" placeholder="SMTP Port" value={emailConfig.smtp_port} onChange={e => setEmailConfig({ ...emailConfig, smtp_port: parseInt(e.target.value) || 587 })} className="border rounded px-2 py-1 text-sm" />
                  </div>
                  <input placeholder="SMTP User" value={emailConfig.smtp_user} onChange={e => setEmailConfig({ ...emailConfig, smtp_user: e.target.value })} className="border rounded px-2 py-1 text-sm w-full" />
                  <input type="password" placeholder="SMTP Password" value={emailConfig.smtp_password} onChange={e => setEmailConfig({ ...emailConfig, smtp_password: e.target.value })} className="border rounded px-2 py-1 text-sm w-full" />
                  <input placeholder="From" value={emailConfig.from} onChange={e => setEmailConfig({ ...emailConfig, from: e.target.value })} className="border rounded px-2 py-1 text-sm w-full" />
                  <input placeholder="To (comma separated)" value={emailConfig.to} onChange={e => setEmailConfig({ ...emailConfig, to: e.target.value })} className="border rounded px-2 py-1 text-sm w-full" />
                </div>
              )}
              {form.type === 'feishu' && (
                <div className="border rounded p-3">
                  <input placeholder="Webhook URL" value={feishuConfig.webhook_url} onChange={e => setFeishuConfig({ webhook_url: e.target.value })} className="border rounded px-2 py-1 text-sm w-full" />
                </div>
              )}
              {form.type === 'webhook' && (
                <div className="border rounded p-3 space-y-2">
                  <input placeholder="URL" value={webhookConfig.url} onChange={e => setWebhookConfig({ ...webhookConfig, url: e.target.value })} className="border rounded px-2 py-1 text-sm w-full" />
                  <select value={webhookConfig.method} onChange={e => setWebhookConfig({ ...webhookConfig, method: e.target.value })} className="border rounded px-2 py-1 text-sm">
                    <option value="POST">POST</option>
                    <option value="GET">GET</option>
                    <option value="PUT">PUT</option>
                  </select>
                  <textarea placeholder='Headers (JSON, e.g. {"Authorization": "Bearer xxx"})' value={webhookConfig.headers} onChange={e => setWebhookConfig({ ...webhookConfig, headers: e.target.value })} className="border rounded px-2 py-1 text-sm w-full h-20" />
                </div>
              )}
              <div>
                <label className="block text-sm font-medium text-gray-700 mb-1">Conditions (JSON)</label>
                <textarea
                  placeholder='{"statuses": ["success", "failed"]}'
                  value={form.conditions}
                  onChange={e => setForm({ ...form, conditions: e.target.value })}
                  className="w-full border rounded px-3 py-2 h-20"
                />
              </div>
              <div>
                <label className="block text-sm font-medium text-gray-700 mb-1">Description</label>
                <input
                  type="text"
                  value={form.description}
                  onChange={e => setForm({ ...form, description: e.target.value })}
                  className="w-full border rounded px-3 py-2"
                />
              </div>
              <div className="flex items-center">
                <input
                  type="checkbox"
                  checked={form.enabled}
                  onChange={e => setForm({ ...form, enabled: e.target.checked })}
                  className="mr-2"
                />
                <label className="text-sm">Enabled</label>
              </div>
            </div>
            <div className="flex justify-end gap-2 mt-6">
              <button onClick={() => setShowModal(false)} className="px-4 py-2 text-gray-600 hover:bg-gray-100 rounded">
                {t('common.cancel')}
              </button>
              <button onClick={handleSubmit} className="px-4 py-2 bg-blue-600 text-white rounded hover:bg-blue-700">
                {t('common.save')}
              </button>
            </div>
          </div>
        </div>
      )}
    </div>
  )
}