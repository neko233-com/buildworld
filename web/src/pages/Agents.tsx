import { useState } from 'react'
import { motion } from 'motion/react'
import { Activity, Copy, Gauge, Network, Plus, Radio, ServerCog, Tag, Trash2, X } from 'lucide-react'
import { useI18n } from '../i18n'
import { api } from '../api'
import { useApi } from '../hooks'
import { dialogs } from '../components/AppDialogs'
import { ModalDialog } from '../components/ModalDialog'
import { JenkinsHeaderBreadcrumb } from '../components/JenkinsPageShell'
import { PageState } from '../components/PageState'
import { isAdmin } from '../authz'
import { formatDateTimeWithWeekday } from '../lib/dateTime'
import './ManagementPages.jenkins.css'

function parseLabels(labels: string): string[] {
  if (!labels) return []
  try { const parsed = JSON.parse(labels); return Array.isArray(parsed) ? parsed : typeof parsed === 'object' ? Object.keys(parsed) : [] } catch { return labels.split(',').map(item => item.trim()).filter(Boolean) }
}
export default function Agents() {
  const { t } = useI18n()
  const breadcrumb = <JenkinsHeaderBreadcrumb breadcrumbs={[{ label: t('agents.title') }]} />
  const admin = isAdmin()
  const { data: agents, loading, error, reload } = useApi(() => api.listAgents())
  const [showForm, setShowForm] = useState(false)
  const [form, setForm] = useState({ name: '', address: '', labels: '', max_builds: 4, pool: '' })
  const [token, setToken] = useState('')
  const [saving, setSaving] = useState(false)
  const [formError, setFormError] = useState('')
  const set = (key: keyof typeof form) => (event: React.ChangeEvent<HTMLInputElement>) => setForm({ ...form, [key]: event.target.value })
  const handleRegister = async (event: React.FormEvent) => {
    event.preventDefault(); setSaving(true); setFormError('')
    try {
      const data: any = { name: form.name, max_builds: Number(form.max_builds) }
      if (form.address) data.address = form.address
		if (form.labels) data.labels = form.labels.split(',').map(label => label.trim()).filter(Boolean)
      if (form.pool) data.pool = form.pool
      const response: any = await api.registerAgent(data)
      if (response?.token) setToken(response.token)
      setForm({ name: '', address: '', labels: '', max_builds: 4, pool: '' }); setShowForm(false); reload()
    } catch (reason: any) { setFormError(reason.message || t('agents.registerFailed')) } finally { setSaving(false) }
  }
  const handleDelete = async (id: string) => {
    if (!await dialogs.confirm(t('agents.removeConfirm'), { title: t('agents.removeTitle'), action: t('agents.remove') })) return
    try { await api.deleteAgent(id); reload() } catch (reason: any) { dialogs.notify(reason.message || t('agents.removeFailed')) }
  }
  const copyToken = async () => { await navigator.clipboard?.writeText(token); dialogs.notify(t('agents.copy'), 'success') }

  if (loading) return <>{breadcrumb}<section className="jenkins-management-page"><PageState /></section></>
  if (error) return <>{breadcrumb}<section className="jenkins-management-page"><PageState error={error} onRetry={reload} /></section></>
  const list = agents || []
  const online = list.filter(agent => agent.status === 'online').length
  const busy = list.filter(agent => (agent.active_builds || 0) > 0).length
  const totalCapacity = list.reduce((total, agent) => total + (agent.max_concurrent_builds || 0), 0)
  const activeCapacity = list.reduce((total, agent) => total + (agent.active_builds || 0), 0)

  return <>{breadcrumb}<motion.section className="agent-workbench jenkins-management-page" initial={{ opacity: 0, y: 8 }} animate={{ opacity: 1, y: 0 }} transition={{ duration: 0.24, ease: 'easeOut' }}>
    <header className="operations-heading"><div><p>{online} / {list.length}</p><h1>{t('agents.title')}</h1></div>{admin && <button className="primary-command" onClick={() => { setFormError(''); setShowForm(true) }}><Plus size={16} />{t('agents.register')}</button>}</header>
    <section className="agent-metrics" aria-label="Worker capacity summary"><article><span><Network size={15} />{t('agents.total')}</span><strong>{list.length}</strong></article><article><span><Radio size={15} />{t('agents.online')}</span><strong className="positive">{online}</strong></article><article><span><Activity size={15} />{t('agents.activeWorkers')}</span><strong className={busy ? 'warning' : ''}>{busy}</strong></article><article><span><Gauge size={15} />{t('agents.capacity')}</span><strong>{activeCapacity} <small>/ {totalCapacity || '-'}</small></strong></article></section>
    {token && <section className="enrollment-token"><header><div><ServerCog size={17} /><div><h2>{t('agents.tokenOneTime')}</h2><p>{t('agents.tokenDescription')}</p></div></div><button title={t('agents.dismiss')} aria-label={t('agents.dismiss')} onClick={() => setToken('')}><X size={16} /></button></header><div><code>{token}</code><button className="secondary-command" onClick={copyToken}><Copy size={15} />{t('agents.copy')}</button></div></section>}
    <section className="operations-table-wrap agent-table-wrap"><table className="operations-table agent-table"><thead><tr><th>{t('agents.name')}</th><th>{t('agents.status')}</th><th>{t('agents.pool')}</th><th>{t('agents.labels')}</th><th>{t('agents.activeBuilds')}</th><th>{t('agents.address')}</th><th>{t('agents.heartbeat')}</th><th aria-label={t('agents.actions')} /></tr></thead><tbody>{!list.length && <tr><td colSpan={8} className="operations-empty"><ServerCog size={18} />{t('common.noData')}</td></tr>}{list.map(agent => { const labels = parseLabels(agent.labels); const capacity = agent.max_concurrent_builds || 0; const active = agent.active_builds || 0; const percent = capacity ? Math.min(100, (active / capacity) * 100) : 0; return <tr key={agent.id}><td><div className="agent-name"><ServerCog size={16} /><span><strong>{agent.name}</strong><small>{agent.id}</small></span></div></td><td><span className={`agent-status ${agent.status || 'offline'}`}><i />{agent.status === 'online' ? t('agents.online') : agent.status === 'offline' ? t('agents.offline') : agent.status}</span></td><td><span className="agent-pool">{agent.pool || t('agents.defaultPool')}</span></td><td><div className="agent-labels">{labels.length ? labels.map(label => <span key={label}><Tag size={11} />{label}</span>) : <span className="muted-cell">-</span>}</div></td><td><div className="agent-capacity"><span>{active} / {capacity || '-'}</span><i><b style={{ width: `${percent}%` }} /></i></div></td><td><code className="agent-address">{agent.address || '-'}</code></td><td className="muted-cell">{formatDateTimeWithWeekday(agent.last_heartbeat)}</td><td><div className="row-actions">{admin && <button className="row-icon danger" title={t('projects.delete')} aria-label={t('projects.delete')} onClick={() => handleDelete(agent.id)}><Trash2 size={15} /></button>}</div></td></tr> })}</tbody></table></section>
    {showForm && <ModalDialog className="agent-register-modal" ariaLabel={t('agents.register')} busy={saving} onClose={() => setShowForm(false)}><header><div><ServerCog size={18} /><div><h2>{t('agents.register')}</h2><p>{t('agents.registerDescription')}</p></div></div><button onClick={() => setShowForm(false)} title={t('common.cancel')}><X size={18} /></button></header><form onSubmit={handleRegister} className="agent-register-form"><label>{t('agents.name')}<input required data-dialog-initial-focus value={form.name} onChange={set('name')} autoFocus /></label><label>{t('agents.addressOptional')}<input value={form.address} onChange={set('address')} placeholder="192.168.1.100:7050" /></label><label>{t('agents.labelsHint')}<input value={form.labels} onChange={set('labels')} placeholder="linux, amd64" /></label><div className="agent-register-grid"><label>{t('agents.maxConcurrent')}<input type="number" min="1" value={form.max_builds} onChange={set('max_builds')} /></label><label>{t('agents.pool')}<input value={form.pool} onChange={set('pool')} placeholder={t('agents.defaultPool')} /></label></div>{formError && <p className="form-error">{formError}</p>}<footer><button type="button" onClick={() => setShowForm(false)}>{t('common.cancel')}</button><button type="submit" disabled={saving}>{saving ? t('common.loading') : t('agents.register')}</button></footer></form></ModalDialog>}
  </motion.section></>
}
