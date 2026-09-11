import { useCallback, useState } from 'react'
import { BellRing, Braces, Check, CircleCheck, Clock3, Eye, Layers3, Mail, MessageCircle, Monitor, Pencil, Plus, Send, Trash2, Webhook, X } from 'lucide-react'
import { useI18n } from '../i18n'
import { useApi } from '../hooks'
import { dialogs } from '../components/AppDialogs'
import { ModalDialog } from '../components/ModalDialog'
import { JenkinsHeaderBreadcrumb } from '../components/JenkinsPageShell'
import { PageState } from '../components/PageState'
import { api } from '../api'
import { formatDateTimeWithWeekday } from '../lib/dateTime'
import './ManagementPages.jenkins.css'

type ChannelType = 'web' | 'email' | 'feishu' | 'webhook' | 'discord' | 'wecom' | 'telegram'
type NotificationChannel = { id: number; name: string; type: ChannelType; config: string; conditions: string; description: string; enabled: boolean; created_at: string; updated_at: string }
type ChannelForm = { name: string; type: ChannelType; description: string; enabled: boolean; statuses: string[] }
const channelTypeDefinitions: Array<{ value: ChannelType; icon: typeof BellRing }> = [{ value: 'web', icon: Monitor }, { value: 'feishu', icon: MessageCircle }, { value: 'discord', icon: MessageCircle }, { value: 'wecom', icon: MessageCircle }, { value: 'telegram', icon: Send }, { value: 'email', icon: Mail }, { value: 'webhook', icon: Webhook }]

function parseStatuses(source: string): string[] {
  try { const value = JSON.parse(source); return Array.isArray(value?.statuses) ? value.statuses : [] } catch { return [] }
}
function typeClass(type: ChannelType) { return `notification-type ${type}` }

export default function Notifications() {
  const { t } = useI18n()
  const breadcrumb = <JenkinsHeaderBreadcrumb breadcrumbs={[{ label: t('settings.notifications') }]} />
  const channelTypes = channelTypeDefinitions.map(definition => ({ ...definition, label: t(`notifications.type_${definition.value}`) }))
  const statusOptions = [
    { value: 'running', label: t('notifications.statusStarted') },
    { value: 'success', label: t('notifications.statusSuccess') },
    { value: 'failed', label: t('notifications.statusFailed') },
    { value: 'cancelled', label: t('notifications.statusCancelled') },
  ]
  const labelForType = (type: ChannelType) => channelTypes.find(item => item.value === type)?.label || type
  const labelForStatus = (status: string) => statusOptions.find(option => option.value === status)?.label || status
  const labelForDelivery = (status: string) => status === 'delivered' ? t('notifications.delivered') : status === 'failed' ? t('notifications.deliveryFailed') : status
  const labelForEvent = (eventType: string) => eventType === 'build.started' ? t('notifications.webStarted') : eventType === 'build.completed' ? t('notifications.webCompleted') : eventType
  const { data: channels, loading, error, reload } = useApi<NotificationChannel[]>(() => api.listNotificationChannels())
  const [showEditor, setShowEditor] = useState(false)
  const [editing, setEditing] = useState<NotificationChannel | null>(null)
  const [eventsChannel, setEventsChannel] = useState<NotificationChannel | null>(null)
  const [events, setEvents] = useState<any[]>([])
  const [saving, setSaving] = useState(false)
  const [formError, setFormError] = useState('')
  const [form, setForm] = useState<ChannelForm>({ name: '', type: 'feishu', description: '', enabled: true, statuses: [] })
  const [webhookURL, setWebhookURL] = useState('')
  const [telegram, setTelegram] = useState({ bot_token: '', chat_id: '' })
  const [email, setEmail] = useState({ smtp_host: '', smtp_port: 587, smtp_user: '', smtp_password: '', from: '', to: '' })
  const [webhook, setWebhook] = useState({ url: '', method: 'POST', headers: '' })

  const reset = useCallback(() => {
    setForm({ name: '', type: 'feishu', description: '', enabled: true, statuses: [] }); setWebhookURL(''); setTelegram({ bot_token: '', chat_id: '' }); setEmail({ smtp_host: '', smtp_port: 587, smtp_user: '', smtp_password: '', from: '', to: '' }); setWebhook({ url: '', method: 'POST', headers: '' }); setFormError('')
  }, [])
  const openCreate = () => { reset(); setEditing(null); setShowEditor(true) }
  const openEdit = (channel: NotificationChannel) => {
    setEditing(channel); setForm({ name: channel.name, type: channel.type, description: channel.description || '', enabled: channel.type === 'web' ? true : channel.enabled, statuses: parseStatuses(channel.conditions) }); setFormError('')
    try {
      const config = JSON.parse(channel.config)
      if (channel.type === 'email') setEmail({ smtp_host: config.smtp_host || '', smtp_port: config.smtp_port || 587, smtp_user: config.smtp_user || '', smtp_password: config.smtp_password || '', from: config.from || '', to: Array.isArray(config.to) ? config.to.join(', ') : '' })
      else if (channel.type === 'telegram') setTelegram({ bot_token: config.bot_token || '', chat_id: config.chat_id || '' })
      else if (channel.type === 'webhook') setWebhook({ url: config.url || '', method: config.method || 'POST', headers: config.headers ? JSON.stringify(config.headers, null, 2) : '' })
      else setWebhookURL(config.webhook_url || '')
    } catch { reset() }
    setShowEditor(true)
  }
  const toggleStatus = (status: string) => setForm(current => ({ ...current, statuses: current.statuses.includes(status) ? current.statuses.filter(value => value !== status) : [...current.statuses, status] }))
  const formatWebhookHeaders = () => {
    try {
      const headers = webhook.headers.trim() ? JSON.parse(webhook.headers) : {}
      if (!headers || Array.isArray(headers) || typeof headers !== 'object') throw new Error(t('config.invalid'))
      setWebhook(current => ({ ...current, headers: `${JSON.stringify(headers, null, 2)}\n` }))
      setFormError('')
      dialogs.notify(t('config.formatted'), 'success')
    } catch (reason: any) {
      const message = reason.message || t('config.invalid')
      setFormError(message)
      dialogs.notify(t('config.invalid'))
    }
  }
  const handleSubmit = async (event: React.FormEvent) => {
    event.preventDefault(); setSaving(true); setFormError('')
    try {
      let config: Record<string, unknown> = {}
      if (form.type === 'web') config = {}
      else if (form.type === 'email') config = { smtp_host: email.smtp_host, smtp_port: email.smtp_port, smtp_user: email.smtp_user, smtp_password: email.smtp_password, from: email.from, to: email.to.split(',').map(value => value.trim()).filter(Boolean) }
      else if (form.type === 'telegram') config = telegram
      else if (form.type === 'webhook') { let headers: Record<string, string> = {}; if (webhook.headers.trim()) headers = JSON.parse(webhook.headers); config = { ...webhook, headers } }
      else config = { webhook_url: webhookURL }
      const data = {
        name: form.name, type: form.type, description: form.description, enabled: form.type === 'web' ? true : form.enabled,
        conditions: JSON.stringify(form.statuses.length ? { statuses: form.statuses } : {}, null, 2),
        config: JSON.stringify(config, null, 2),
      }
      if (editing) await api.updateNotificationChannel(editing.id, data); else await api.createNotificationChannel(data)
      setShowEditor(false); reload()
    } catch (reason: any) {
      const message = reason.message || t('common.error')
      setFormError(message)
      if (reason?.name !== 'ApiError') dialogs.notify(message)
    } finally { setSaving(false) }
  }
  const handleDelete = async (channel: NotificationChannel) => { if (!await dialogs.confirm(t('notifications.removeConfirm').replace('{name}', channel.name), { title: t('notifications.removeTitle'), action: t('common.delete') })) return; try { await api.deleteNotificationChannel(channel.id); reload() } catch (reason: any) { dialogs.notify(reason.message || t('common.error')) } }
  const toggleEnabled = async (channel: NotificationChannel) => {
    if (channel.type === 'web') {
      dialogs.notify(t('notifications.requiredDescription'), 'info')
      return
    }
    try { await api.updateNotificationChannel(channel.id, { ...channel, enabled: !channel.enabled }); reload() } catch (reason: any) { dialogs.notify(reason.message || t('common.error')) }
  }
  const showEvents = async (channel: NotificationChannel) => { try { setEvents(await api.listNotificationEvents(channel.id)); setEventsChannel(channel) } catch (reason: any) { dialogs.notify(reason.message || t('common.error')) } }

  if (loading) return <>{breadcrumb}<section className="jenkins-management-page"><PageState /></section></>
  if (error) return <>{breadcrumb}<section className="jenkins-management-page"><PageState error={error} onRetry={reload} /></section></>
  const list = channels || []
  const enabled = list.filter(channel => channel.enabled).length
  return <>{breadcrumb}<section className="notification-workbench jenkins-management-page">
    <header className="operations-heading"><div><p>{enabled} / {list.length}</p><h1>{t('settings.notifications')}</h1></div><button className="primary-command" onClick={openCreate}><Plus size={16} />{t('notifications.new')}</button></header>
    <section className="notification-summary"><article><span><BellRing size={15} />{t('notifications.sharedChannels')}</span><strong>{list.length}</strong><small>{t('notifications.deliveryRegistry')}</small></article><article><span><CircleCheck size={15} />{t('notifications.enabledChannels')}</span><strong className="positive">{enabled}</strong><small>{t('notifications.buildLifecycle')}</small></article><article><span><Send size={15} />{t('notifications.supportedTargets')}</span><div className="notification-targets">{channelTypes.map(type => { const Icon = type.icon; return <span key={type.value} title={type.label}><Icon size={13} />{type.label}</span> })}</div></article></section>
    <section className="parallel-delivery-card"><div><Layers3 size={18} /><div><h2>{t('notifications.parallelTitle')}</h2><p>{t('notifications.parallelDescription')}</p></div></div><div className="parallel-channel-list">{list.filter(channel => channel.enabled).map(channel => <span key={channel.id}>{labelForType(channel.type)} · {channel.name}</span>)}</div></section>
    <section className="operations-table-wrap notification-table-wrap"><table className="operations-table notification-table"><thead><tr><th>{t('notifications.name')}</th><th>{t('notifications.type')}</th><th>{t('notifications.deliveryScope')}</th><th>{t('notifications.description')}</th><th>{t('notifications.enabled')}</th><th>{t('notifications.updatedAt')}</th><th aria-label={t('agents.actions')} /></tr></thead><tbody>{!list.length && <tr><td colSpan={7} className="operations-empty"><BellRing size={18} />{t('common.noData')}</td></tr>}{list.map(channel => { const Icon = channelTypes.find(item => item.value === channel.type)?.icon || BellRing; const statuses = parseStatuses(channel.conditions); return <tr key={channel.id}><td><div className="notification-name"><Icon size={16} /><span><strong>{channel.name}</strong><small>{labelForType(channel.type)}{channel.type === 'web' ? ` · ${t('notifications.systemDefault')}` : ''}</small></span></div></td><td><span className={typeClass(channel.type)}>{labelForType(channel.type)}</span></td><td><div className="notification-statuses">{statuses.length ? statuses.map(status => <span key={status}>{labelForStatus(status)}</span>) : <span>{t('notifications.allStates')}</span>}</div></td><td className="muted-cell notification-description">{channel.description || '-'}</td><td>{channel.type === 'web' ? <span className="channel-required" title={t('notifications.requiredDescription')}><Check size={12} />{t('notifications.requiredEnabled')}</span> : <button className={`channel-toggle ${channel.enabled ? 'on' : ''}`} onClick={() => toggleEnabled(channel)} aria-label={channel.enabled ? t('notifications.disableChannel') : t('notifications.enableChannelAction')} aria-pressed={channel.enabled}><i /></button>}</td><td className="muted-cell">{formatDateTimeWithWeekday(channel.updated_at)}</td><td><div className="row-actions"><button className="row-icon" title={t('notifications.viewDeliveries')} aria-label={t('notifications.viewDeliveries')} onClick={() => showEvents(channel)}><Eye size={15} /></button><button className="row-icon" title={t('notifications.editChannel')} aria-label={t('notifications.editChannel')} onClick={() => openEdit(channel)}><Pencil size={15} /></button>{channel.type !== 'web' && <button className="row-icon danger" title={t('common.delete')} aria-label={t('common.delete')} onClick={() => handleDelete(channel)}><Trash2 size={15} /></button>}</div></td></tr>})}</tbody></table></section>
    {showEditor && <ModalDialog className="notification-editor" ariaLabel={editing ? t('notifications.editTitle') : t('notifications.new')} busy={saving} onClose={() => setShowEditor(false)}><header><div><BellRing size={18} /><div><h2>{editing ? t('notifications.editTitle') : t('notifications.new')}</h2><p>{t('notifications.channelDescription')}</p></div></div><button type="button" onClick={() => setShowEditor(false)} title={t('common.close')} aria-label={t('common.close')}><X size={18} /></button></header><form onSubmit={handleSubmit} className="notification-editor-form"><label>{t('notifications.name')}<input required data-dialog-initial-focus readOnly={form.type === 'web'} value={form.name} onChange={event => setForm({ ...form, name: event.target.value })} autoFocus /></label><label>{t('notifications.type')}<select disabled={form.type === 'web'} value={form.type} onChange={event => setForm({ ...form, type: event.target.value as ChannelType })}>{channelTypes.filter(type => type.value !== 'web' || editing?.type === 'web').map(type => <option key={type.value} value={type.value}>{type.label}</option>)}</select></label><label className="wide">{t('notifications.description')}<input value={form.description} onChange={event => setForm({ ...form, description: event.target.value })} /></label><fieldset className="wide"><legend>{t('notifications.deliveryStatuses')}</legend><div>{statusOptions.map(option => <button type="button" key={option.value} className={form.statuses.includes(option.value) ? 'selected' : ''} onClick={() => toggleStatus(option.value)} aria-pressed={form.statuses.includes(option.value)}>{form.statuses.includes(option.value) && <Check size={13} />}{option.label}</button>)}</div><small>{t('notifications.allStatuses')}</small></fieldset>{form.type === 'email' && <div className="notification-config-grid wide"><input aria-label={t('notifications.smtpHost')} placeholder={t('notifications.smtpHost')} value={email.smtp_host} onChange={event => setEmail({ ...email, smtp_host: event.target.value })} /><input type="number" aria-label={t('notifications.smtpPort')} placeholder={t('notifications.smtpPort')} value={email.smtp_port} onChange={event => setEmail({ ...email, smtp_port: Number(event.target.value) || 587 })} /><input aria-label={t('notifications.smtpUser')} placeholder={t('notifications.smtpUser')} value={email.smtp_user} onChange={event => setEmail({ ...email, smtp_user: event.target.value })} /><input type="password" aria-label={t('notifications.smtpPassword')} placeholder={t('notifications.smtpPassword')} value={email.smtp_password} onChange={event => setEmail({ ...email, smtp_password: event.target.value })} /><input aria-label={t('notifications.fromAddress')} placeholder={t('notifications.fromAddress')} value={email.from} onChange={event => setEmail({ ...email, from: event.target.value })} /><input aria-label={t('notifications.recipients')} placeholder={t('notifications.recipients')} value={email.to} onChange={event => setEmail({ ...email, to: event.target.value })} /></div>}{form.type === 'telegram' && <div className="notification-config-grid wide"><input aria-label={t('notifications.botToken')} placeholder={t('notifications.botToken')} value={telegram.bot_token} onChange={event => setTelegram({ ...telegram, bot_token: event.target.value })} /><input aria-label={t('notifications.chatId')} placeholder={t('notifications.chatId')} value={telegram.chat_id} onChange={event => setTelegram({ ...telegram, chat_id: event.target.value })} /></div>}{(form.type === 'feishu' || form.type === 'discord' || form.type === 'wecom') && <label className="wide">{labelForType(form.type)} {t('notifications.webhookUrl')}<input required value={webhookURL} onChange={event => setWebhookURL(event.target.value)} placeholder="https://" /></label>}{form.type === 'webhook' && <div className="notification-config-grid wide"><input aria-label={t('notifications.webhookUrl')} placeholder={t('notifications.webhookUrl')} value={webhook.url} onChange={event => setWebhook({ ...webhook, url: event.target.value })} /><select aria-label={t('notifications.httpMethod')} value={webhook.method} onChange={event => setWebhook({ ...webhook, method: event.target.value })}><option>POST</option><option>GET</option><option>PUT</option></select><div className="wide config-editor-label"><span><label htmlFor="notification-headers-json">{t('notifications.headersJson')}</label><button type="button" className="format-command" onClick={formatWebhookHeaders}><Braces size={13} />{t('config.format')}</button></span><textarea id="notification-headers-json" aria-label={t('notifications.headersJson')} placeholder="{\n  &quot;Authorization&quot;: &quot;Bearer ...&quot;\n}" value={webhook.headers} onChange={event => setWebhook({ ...webhook, headers: event.target.value })} /></div></div>}<label className={`channel-enabled wide ${form.type === 'web' ? 'required' : ''}`}><input type="checkbox" checked={form.type === 'web' ? true : form.enabled} disabled={form.type === 'web'} onChange={event => setForm({ ...form, enabled: event.target.checked })} /><span>{form.type === 'web' ? t('notifications.requiredEnabled') : t('notifications.enableChannel')}</span>{form.type === 'web' && <small>{t('notifications.requiredDescription')}</small>}</label>{formError && <p className="form-error">{formError}</p>}<footer><button type="button" onClick={() => setShowEditor(false)}>{t('common.cancel')}</button><button type="submit" disabled={saving}>{saving ? t('common.loading') : t('common.save')}</button></footer></form></ModalDialog>}
    {eventsChannel && <ModalDialog className="notification-events-modal" ariaLabel={t('notifications.deliveryHistory')} closeOnBackdrop onClose={() => setEventsChannel(null)}><header><div><Clock3 size={18} /><div><h2>{eventsChannel.name}</h2><p>{t('notifications.eventsDescription')}</p></div></div><button onClick={() => setEventsChannel(null)} title={t('common.close')} aria-label={t('common.close')}><X size={18} /></button></header><div className="notification-events-list">{events.length ? events.map(event => <article key={event.id} className={`notification-event ${event.status}`}><div><strong>{labelForEvent(event.event_type)}</strong><small>{formatDateTimeWithWeekday(event.created_at)}</small></div><span>{labelForDelivery(event.status)}</span>{event.error_message && <p>{event.error_message}</p>}</article>) : <p className="detail-empty">{t('notifications.noDeliveryAttempts')}</p>}</div><footer><button data-dialog-initial-focus onClick={() => setEventsChannel(null)}>{t('common.close')}</button></footer></ModalDialog>}
  </section></>
}
