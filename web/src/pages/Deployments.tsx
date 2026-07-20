import { useEffect, useState } from 'react'
import { motion } from 'motion/react'
import { Boxes, Braces, CheckCircle2, PackageOpen, Pencil, Play, Plus, Rocket, Trash2, X } from 'lucide-react'
import { useI18n } from '../i18n'
import { api } from '../api'
import { useApi } from '../hooks'
import { dialogs } from '../components/AppDialogs'
import { ModalDialog } from '../components/ModalDialog'
import { PageState } from '../components/PageState'
import { prettyConfigSource, prettyConfigSourceSync } from '../lib/configFormat'
import { canEdit } from '../authz'

interface DeploymentEnv {
  id: number
  project_id: number
  name: string
  description?: string
  config: string
  last_build_id?: number
  created_at?: string
  updated_at?: string
}

const emptyForm = { name: '', type: 'dev', description: '', config: '{\n  "type": "dev"\n}\n' }

function deploymentType(config: string) {
  try { return JSON.parse(config)?.type || 'custom' } catch {
    return config.match(/^\s*type\s*:\s*['"]?([^'"\r\n]+)/m)?.[1]?.trim() || 'custom'
  }
}
async function configWithType(source: string, type: string) {
  const formatted = await prettyConfigSource(source)
  if (formatted.trimStart().startsWith('{')) {
    const value = JSON.parse(formatted)
    value.type = type
    return `${JSON.stringify(value, null, 2)}\n`
  }
  const { parseDocument } = await import('yaml')
  const document = parseDocument(formatted)
  document.set('type', type)
  return `${document.toString({ indent: 2, lineWidth: 0 }).trimEnd()}\n`
}

export default function Deployments() {
  const { t } = useI18n()
  const editable = canEdit()
  const { data: projects } = useApi<any[]>(() => api.listProjects(), [])
  const [projectID, setProjectID] = useState(0)
  const { data: environments, loading, error, reload } = useApi<DeploymentEnv[]>(() => projectID ? api.listDeploymentEnvs(projectID) : Promise.resolve([]), [projectID])
  const { data: builds } = useApi<any[]>(() => projectID ? api.listProjectBuilds(projectID) : Promise.resolve([]), [projectID])
  const [showEditor, setShowEditor] = useState(false)
  const [editing, setEditing] = useState<DeploymentEnv | null>(null)
  const [form, setForm] = useState(emptyForm)
  const [selectedBuilds, setSelectedBuilds] = useState<Record<number, number | ''>>({})
  const [busyID, setBusyID] = useState<number | null>(null)
  const [formError, setFormError] = useState('')

  useEffect(() => {
    if (!projectID && projects?.length) setProjectID(projects[0].id)
  }, [projectID, projects])

  const openCreate = () => {
    setEditing(null)
    setForm(emptyForm)
    setFormError('')
    setShowEditor(true)
  }
  const openEdit = (environment: DeploymentEnv) => {
    let config = environment.config || '{}\n'
    try { config = prettyConfigSourceSync(config) } catch { /* Keep legacy source editable. */ }
    setEditing(environment)
    setForm({ name: environment.name, description: environment.description || '', type: deploymentType(config), config })
    setFormError('')
    setShowEditor(true)
  }
  const handleFormat = async () => {
    setFormError('')
    try {
      const config = await configWithType(form.config, form.type)
      setForm(current => ({ ...current, config }))
      dialogs.notify(t('config.formatted'), 'success')
    } catch (reason: any) {
      const message = reason.message || t('config.invalid')
      setFormError(message)
      dialogs.notify(t('config.invalid'))
    }
  }
  const handleSubmit = async (event: React.FormEvent) => {
    event.preventDefault()
    setBusyID(editing?.id || -1)
    setFormError('')
    try {
      const config = await configWithType(form.config, form.type)
      const data = { project_id: projectID, name: form.name, description: form.description, config }
      if (editing) await api.updateDeploymentEnv(editing.id, data)
      else await api.createDeploymentEnv(data)
      setShowEditor(false)
      reload()
    } catch (reason: any) {
      const message = reason.message || t('deployments.saveFailed')
      setFormError(message)
      if (reason?.name !== 'ApiError') dialogs.notify(message)
    } finally {
      setBusyID(null)
    }
  }
  const handleDelete = async (environment: DeploymentEnv) => {
    if (!await dialogs.confirm(t('deployments.deleteConfirm'), { title: t('deployments.deleteTitle'), action: t('common.delete') })) return
    try {
      await api.deleteDeploymentEnv(environment.id)
      reload()
    } catch (reason: any) {
      dialogs.notify(reason.message || t('common.error'))
    }
  }
  const handleDeploy = async (environment: DeploymentEnv) => {
    const buildID = selectedBuilds[environment.id]
    if (!buildID) return
    setBusyID(environment.id)
    try {
      await api.deployBuild(environment.id, Number(buildID))
      setSelectedBuilds(current => ({ ...current, [environment.id]: '' }))
      dialogs.notify(t('deployments.deployed'), 'success')
      reload()
    } catch (reason: any) {
      dialogs.notify(reason.message || t('deployments.deployFailed'))
    } finally {
      setBusyID(null)
    }
  }

  if (loading) return <PageState />
  if (error) return <PageState error={error} onRetry={reload} />

  const list = environments || []
  const deployableBuilds = (builds || []).filter(build => build.status === 'success')
  const production = list.filter(environment => deploymentType(environment.config) === 'production').length
  const buildByID = new Map((builds || []).map(build => [build.id, build]))

  return (
    <motion.section className="operations-page deployment-workbench" initial={{ opacity: 0, y: 8 }} animate={{ opacity: 1, y: 0 }} transition={{ duration: 0.24, ease: 'easeOut' }}>
      <header className="operations-heading">
        <div><p>{list.length} {t('deployments.environments')}</p><h1>{t('deployments.title')}</h1></div>
        <div className="deployment-heading-actions"><select className="operations-filter" aria-label={t('deployments.selectProject')} value={projectID} onChange={event => setProjectID(Number(event.target.value))}><option value={0}>{t('deployments.selectProject')}</option>{(projects || []).map(project => <option key={project.id} value={project.id}>{project.name}</option>)}</select>{editable && <button className="primary-command" type="button" disabled={!projectID} onClick={openCreate}><Plus size={16} />{t('deployments.newEnv')}</button>}</div>
      </header>

      <section className="notification-summary deployment-summary">
        <article><span><PackageOpen size={15} />{t('deployments.total')}</span><strong>{list.length}</strong><small>{t('deployments.totalHelp')}</small></article>
        <article><span><Rocket size={15} />{t('deployments.production')}</span><strong>{production}</strong><small>{t('deployments.productionHelp')}</small></article>
        <article><span><CheckCircle2 size={15} />{t('deployments.deployable')}</span><strong>{deployableBuilds.length}</strong><small>{t('deployments.deployableHelp')}</small></article>
      </section>

      {!projectID ? <section className="deployment-empty"><Boxes size={25} /><h2>{t('deployments.chooseProjectTitle')}</h2><p>{t('deployments.chooseProjectHelp')}</p></section> : (
        <section className="operations-table-wrap deployment-table-wrap">
          <table className="operations-table deployment-table">
            <thead><tr><th>{t('deployments.name')}</th><th>{t('deployments.type')}</th><th>{t('deployments.description')}</th><th>{t('deployments.lastBuild')}</th><th>{t('deployments.deploy')}</th><th>{t('deployments.createdAt')}</th><th aria-label={t('projects.actions')} /></tr></thead>
            <tbody>
              {!list.length && <tr><td colSpan={7} className="operations-empty"><PackageOpen size={18} />{t('deployments.empty')}</td></tr>}
              {list.map(environment => {
                const lastBuild = environment.last_build_id ? buildByID.get(environment.last_build_id) : null
                return <tr key={environment.id}>
                  <td><button className="entity-link" type="button" disabled={!editable} onClick={() => openEdit(environment)}><PackageOpen size={16} /><span><strong>{environment.name}</strong><small>#{environment.id}</small></span></button></td>
                  <td><span className={`deployment-type ${deploymentType(environment.config)}`}>{deploymentType(environment.config)}</span></td>
                  <td className="muted-cell deployment-description">{environment.description || '-'}</td>
                  <td>{lastBuild ? <span className="build-number">#{lastBuild.number}</span> : <span className="muted-cell">{t('deployments.neverDeployed')}</span>}</td>
                  <td>{editable ? <div className="deployment-run"><select aria-label={`${environment.name} ${t('deployments.buildId')}`} value={selectedBuilds[environment.id] || ''} onChange={event => setSelectedBuilds({ ...selectedBuilds, [environment.id]: Number(event.target.value) || '' })}><option value="">{t('deployments.chooseBuild')}</option>{deployableBuilds.map(build => <option key={build.id} value={build.id}>#{build.number} · {build.branch || 'main'}</option>)}</select><button className="row-run" type="button" disabled={!selectedBuilds[environment.id] || busyID === environment.id} onClick={() => handleDeploy(environment)}><Play size={13} />{t('deployments.deploy')}</button></div> : <span className="muted-cell">-</span>}</td>
                  <td className="muted-cell">{environment.created_at ? new Date(environment.created_at).toLocaleDateString() : '-'}</td>
                  <td><div className="row-actions">{editable && <><button className="row-icon" type="button" title={t('deployments.edit')} aria-label={t('deployments.edit')} onClick={() => openEdit(environment)}><Pencil size={15} /></button><button className="row-icon danger" type="button" title={t('common.delete')} aria-label={t('common.delete')} onClick={() => handleDelete(environment)}><Trash2 size={15} /></button></>}</div></td>
                </tr>
              })}
            </tbody>
          </table>
        </section>
      )}

      {showEditor && <ModalDialog className="notification-editor deployment-editor" ariaLabel={editing ? t('deployments.edit') : t('deployments.newEnv')} busy={busyID !== null} onClose={() => setShowEditor(false)}>
          <header><div><PackageOpen size={18} /><div><h2>{editing ? t('deployments.edit') : t('deployments.newEnv')}</h2><p>{t('deployments.editorHelp')}</p></div></div><button type="button" onClick={() => setShowEditor(false)} title={t('common.close')}><X size={18} /></button></header>
          <form className="notification-editor-form" onSubmit={handleSubmit}>
            <label>{t('deployments.name')}<input required autoFocus data-dialog-initial-focus value={form.name} onChange={event => setForm({ ...form, name: event.target.value })} /></label>
            <label>{t('deployments.type')}<select value={form.type} onChange={event => setForm({ ...form, type: event.target.value })}><option value="dev">Development</option><option value="staging">Staging</option><option value="production">Production</option><option value="custom">Custom</option></select></label>
            <label className="wide">{t('deployments.description')}<input value={form.description} onChange={event => setForm({ ...form, description: event.target.value })} /></label>
            <div className="wide deployment-config-field"><div className="config-label-row"><label htmlFor="deployment-config">{t('deployments.config')} (JSON/YAML)</label><button className="format-command" type="button" onClick={handleFormat}><Braces size={13} />{t('config.format')}</button></div><textarea id="deployment-config" className="code-input" rows={11} value={form.config} onChange={event => setForm({ ...form, config: event.target.value })} /><small>{t('deployments.configHelp')}</small></div>
            {formError && <p className="form-error">{formError}</p>}
            <footer><button type="button" onClick={() => setShowEditor(false)}>{t('common.cancel')}</button><button type="submit" disabled={busyID !== null}>{busyID !== null ? t('common.loading') : t('common.save')}</button></footer>
          </form>
      </ModalDialog>}
    </motion.section>
  )
}
