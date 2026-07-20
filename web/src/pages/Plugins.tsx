import { useMemo, useState } from 'react'
import { Download, LoaderCircle, LockKeyhole, PackageOpen, RefreshCw, ShieldCheck, Trash2, X } from 'lucide-react'
import { api } from '../api'
import { useApi } from '../hooks'
import { useI18n } from '../i18n'
import { isAdmin } from '../authz'
import { ModalDialog } from '../components/ModalDialog'
import { PageState } from '../components/PageState'

type Filter = 'all' | 'enabled' | 'disabled'

function stepNames(plugin: any): string[] {
  const steps = plugin.steps || plugin.registered_steps || []
  if (Array.isArray(steps)) return steps
  if (typeof steps === 'string') {
    try {
      const parsed = JSON.parse(steps)
      return Array.isArray(parsed) ? parsed : []
    } catch {
      return []
    }
  }
  return []
}

export default function Plugins() {
  const { t } = useI18n()
  const admin = isAdmin()
  const p = (key: string) => t(`pluginRegistry.${key}`)
  const { data: plugins, loading, error, reload } = useApi(() => api.listPlugins())
  const [source, setSource] = useState('')
  const [filter, setFilter] = useState<Filter>('all')
  const [installing, setInstalling] = useState(false)
  const [reloading, setReloading] = useState<string | null>(null)
  const [pendingDelete, setPendingDelete] = useState<any>(null)
  const [notice, setNotice] = useState('')
  const [busyDelete, setBusyDelete] = useState(false)

  const list = plugins || []
  const visible = useMemo(() => list.filter(plugin => filter === 'all' || (filter === 'enabled' ? plugin.enabled : !plugin.enabled)), [filter, list])

  const installFromGitHub = async (event: React.FormEvent) => {
    event.preventDefault()
    setInstalling(true)
    setNotice('')
    try {
      const manifest = await api.installGitHubPlugin(source)
      setSource('')
      setNotice(`${manifest.name}@${manifest.version} ${p('installedNotice')}.`)
      reload()
    } catch (err: any) {
      setNotice(err.message || p('installFailed'))
    } finally {
      setInstalling(false)
    }
  }

  const togglePlugin = async (plugin: any) => {
    try {
      await api.togglePlugin(plugin.id, !plugin.enabled)
      reload()
    } catch (err: any) {
      setNotice(err.message || p('toggleFailed'))
    }
  }

  const reloadPlugin = async (plugin: any) => {
    setReloading(plugin.name)
    try {
      await api.reloadPlugin(plugin.name)
      setNotice(`${plugin.name} ${p('reloaded')}`)
      reload()
    } catch (err: any) {
      setNotice(err.message || p('reloadFailed'))
    } finally {
      setReloading(null)
    }
  }

  const deletePlugin = async () => {
    if (!pendingDelete) return
    setBusyDelete(true)
    try {
      await api.deletePlugin(pendingDelete.id)
      setNotice(`${pendingDelete.name} ${p('removed')}`)
      setPendingDelete(null)
      reload()
    } catch (err: any) {
      setNotice(err.message || p('removeFailed'))
      setPendingDelete(null)
    } finally {
      setBusyDelete(false)
    }
  }

  if (loading) return <PageState />
  if (error) return <PageState error={error} onRetry={reload} />

  return (
    <main className="plugin-registry">
      <header className="plugin-registry-heading">
        <div>
          <h1>{p('title')}</h1>
          <p>{p('description')}</p>
        </div>
        <div className="plugin-count"><PackageOpen size={16} /> {list.length} {p('installed')}</div>
      </header>

      {admin && <><form className="plugin-source-form" onSubmit={installFromGitHub}>
        <div className="plugin-source-icon"><Download size={18} /></div>
        <label>
          <span>{p('repositoryUrl')}</span>
          <input required type="url" value={source} onChange={event => setSource(event.target.value)} placeholder={p('repositoryPlaceholder')} />
        </label>
        <button disabled={installing} type="submit">{installing ? <LoaderCircle className="spin" size={16} /> : <Download size={16} />}{installing ? p('building') : p('install')}</button>
      </form>
      <p className="plugin-source-note"><ShieldCheck size={14} /> {p('installHelp')}</p></>}

      {notice && <div className="plugin-notice" role="status"><span>{notice}</span><button type="button" title={t('common.dismiss')} aria-label={t('common.dismiss')} onClick={() => setNotice('')}><X size={15} /></button></div>}

      <nav className="plugin-filter" aria-label={p('filtersLabel')}>
        {(['all', 'enabled', 'disabled'] as Filter[]).map(value => <button type="button" key={value} className={filter === value ? 'active' : ''} aria-pressed={filter === value} onClick={() => setFilter(value)}>{p(value)} <span>{value === 'all' ? list.length : list.filter(plugin => value === 'enabled' ? plugin.enabled : !plugin.enabled).length}</span></button>)}
      </nav>

      <section className="plugin-table-wrap" aria-label={p('installedList')}>
        <table className="plugin-table">
          <thead><tr><th>{t('plugins.name')}</th><th>{p('version')}</th><th>{p('capabilities')}</th><th>{p('integrity')}</th><th>{p('status')}</th><th aria-label={t('projects.actions')} /></tr></thead>
          <tbody>
            {visible.map(plugin => {
              const steps = stepNames(plugin)
              const isReloading = reloading === plugin.name
              const isBuiltin = plugin.source === 'builtin'
              return <tr key={plugin.id ?? plugin.name}>
                <td><strong>{plugin.name}</strong><small>{plugin.description || p('noDescription')}</small></td>
                <td><code>{plugin.version || p('unversioned')}</code></td>
                <td>{steps.length ? <div className="plugin-capabilities">{steps.map((step: string) => <code key={step}>{step}</code>)}</div> : <span className="muted">{p('noSteps')}</span>}</td>
                <td><span className={isBuiltin ? 'integrity builtin' : 'integrity'}><ShieldCheck size={14} />{isBuiltin ? p('builtin') : p('checksum')}</span></td>
                <td><button type="button" disabled={!admin} aria-label={`${plugin.name}: ${plugin.enabled ? p('enabled') : p('disabled')}`} aria-pressed={Boolean(plugin.enabled)} className={`plugin-status ${plugin.enabled ? 'enabled' : ''}`} onClick={() => togglePlugin(plugin)}>{plugin.enabled ? p('enabled') : p('disabled')}</button></td>
                <td><div className="plugin-actions">{admin && <><button type="button" title={`${p('reload')} ${plugin.name}`} aria-label={`${p('reload')} ${plugin.name}`} disabled={isReloading} onClick={() => reloadPlugin(plugin)}>{isReloading ? <LoaderCircle className="spin" size={16} /> : <RefreshCw size={16} />}</button>{isBuiltin ? <button type="button" className="protected" disabled title={p('builtinProtected')} aria-label={`${plugin.name}: ${p('builtinProtected')}`}><LockKeyhole size={15} /></button> : <button type="button" title={`${p('remove')} ${plugin.name}`} aria-label={`${p('remove')} ${plugin.name}`} className="remove" onClick={() => setPendingDelete(plugin)}><Trash2 size={16} /></button>}</>}</div></td>
              </tr>
            })}
            {!visible.length && <tr><td className="plugin-empty" colSpan={6}>{p('noMatches')}</td></tr>}
          </tbody>
        </table>
      </section>

      {pendingDelete && <ModalDialog ariaLabel={p('removeTitle')} busy={busyDelete} onClose={() => setPendingDelete(null)}><header><div><Trash2 size={18} /><div><h2>{p('removeTitle')} {pendingDelete.name}</h2><p>{p('removeDescription')}</p></div></div><button type="button" onClick={() => setPendingDelete(null)} title={t('common.close')} aria-label={t('common.close')}><X size={18} /></button></header><footer><button type="button" data-dialog-initial-focus disabled={busyDelete} onClick={() => setPendingDelete(null)}>{t('common.cancel')}</button><button type="button" className="danger-action" disabled={busyDelete} onClick={deletePlugin}>{busyDelete ? p('removing') : p('remove')}</button></footer></ModalDialog>}
    </main>
  )
}
