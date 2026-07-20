import { lazy, Suspense, useEffect, useMemo, useState } from 'react'
import { motion } from 'motion/react'
import { Link, Navigate, useNavigate, useParams, useSearchParams } from 'react-router-dom'
import { Activity, Braces, CalendarClock, CirclePlay, FileCode2, FileInput, FolderTree, GitBranch, Link2, LoaderCircle, Settings2, SlidersHorizontal, Trash2, Workflow, X } from 'lucide-react'
import { useI18n } from '../i18n'
import { api, type PipelineMigrationResult } from '../api'
import { useApi } from '../hooks'
import { buildStatusLabel, buildStatusTone } from '../lib/buildPresentation'
import { isTypeScriptPipelineSource, prettyConfigSource, prettyConfigSourceSync } from '../lib/configFormat'
import ProjectGitHooks from '../components/ProjectGitHooks'
import { dialogs } from '../components/AppDialogs'
import { ModalDialog } from '../components/ModalDialog'
import { PageState } from '../components/PageState'
import { canEdit } from '../authz'
import RunBuildDialog, { requiresBuildParameterInput } from '../components/RunBuildDialog'
import { formatDuration } from '../lib/durationPresentation'
import { flattenProjectGroups, projectGroupPath } from '../lib/projectGroups'
import ApprovalSettingsEditor from '../components/ApprovalSettingsEditor'
import JenkinsfileImportDialog from '../components/JenkinsfileImportDialog'

const PipelineEditor = lazy(() => import('../components/PipelineEditor'))

export default function ProjectDetail() {
  const { t } = useI18n()
  const navigate = useNavigate()
  const [searchParams, setSearchParams] = useSearchParams()
  const { id } = useParams<{ id: string }>()
  const projectId = Number(id)
  const validProjectId = Number.isSafeInteger(projectId) && projectId > 0
  const requestedView = searchParams.get('view')
  const [activeTab, setActiveTab] = useState<'overview' | 'builds' | 'build' | 'settings'>(requestedView === 'builds' || requestedView === 'build' || requestedView === 'settings' ? requestedView : 'overview')
  const { data: project, loading, error, reload } = useApi(() => validProjectId ? api.getProject(projectId) : Promise.resolve(null), [projectId, validProjectId])
  const { data: builds } = useApi(() => validProjectId ? api.listProjectBuilds(projectId) : Promise.resolve([]), [projectId, validProjectId])
  const { data: projectGroups } = useApi(() => api.listProjectGroups())
  const { data: globalVariables } = useApi(() => activeTab === 'build' ? api.listEnvVars('global') : Promise.resolve([]), [activeTab])
  const groups = useMemo(() => flattenProjectGroups(projectGroups || []), [projectGroups])
  const [form, setForm] = useState<any>(null)
  const [saving, setSaving] = useState(false)
  const [formError, setFormError] = useState('')
  const [showSchedule, setShowSchedule] = useState(false)
  const [schedule, setSchedule] = useState({ mode: 'daily', hour: '2', minute: '0' })
  const [deleting, setDeleting] = useState(false)
  const [notice, setNotice] = useState('')
  const [building, setBuilding] = useState(false)
  const [formatting, setFormatting] = useState(false)
  const [showJenkinsImport, setShowJenkinsImport] = useState(false)
  const [showCustomBuild, setShowCustomBuild] = useState(false)
  const editable = canEdit()
  const typeScriptPipeline = isTypeScriptPipelineSource(form?.config || project?.config || '')

  useEffect(() => {
    if (!project) return
    let config = project.config || ''
    try { config = prettyConfigSourceSync(config) } catch { /* Surface validation when the user formats or saves. */ }
    setForm({ name: project.name, description: project.description || '', repo_url: project.repo_url || '', repo_type: project.repo_type, default_branch: project.default_branch, vcs_root_id: project.vcs_root_id ?? null, template_id: project.template_id ?? null, group_id: project.group_id ?? null, tags: (project.tags || []).join(', '), config })
  }, [project])

  useEffect(() => {
    const next = searchParams.get('view')
    if (next === 'overview' || next === 'builds' || next === 'build' || next === 'settings') setActiveTab(next)
  }, [searchParams])

  const set = (key: string) => (event: React.ChangeEvent<HTMLInputElement | HTMLTextAreaElement | HTMLSelectElement>) => setForm((current: any) => ({ ...current, [key]: key === 'group_id' ? event.target.value === '' ? null : Number(event.target.value) : event.target.value }))
  const showView = (view: 'overview' | 'builds' | 'build' | 'settings') => {
    setActiveTab(view)
    setSearchParams(view === 'overview' ? {} : { view })
  }
  const handleBuildNow = async () => {
    if (requiresBuildParameterInput(project?.config)) {
      setShowCustomBuild(true)
      return
    }
    setBuilding(true)
    try {
      const build = await api.triggerBuild(projectId)
      navigate(`/builds/${build.id}`)
    } catch (reason: any) {
      setNotice(reason.message || t('projectDetail.buildFailed'))
      setBuilding(false)
    }
  }
  const handleFormatConfig = async () => {
    setFormatting(true); setFormError('')
    try {
      const config = await prettyConfigSource(form.config)
      setForm((current: any) => ({ ...current, config }))
      setNotice(t('config.formatted'))
      dialogs.notify(t('config.formatted'), 'success')
    } catch (reason: any) {
      const message = reason.message || t('config.invalid')
      setFormError(message)
      dialogs.notify(t('config.invalid'))
    } finally {
      setFormatting(false)
    }
  }
  const handleSave = async (event: React.FormEvent) => {
    event.preventDefault(); setSaving(true); setFormError('')
    try {
      const config = await prettyConfigSource(form.config)
      const next = { ...form, config }
      await api.updateProject(projectId, { ...next, tags: form.tags.split(',').map((tag: string) => tag.trim()).filter(Boolean) })
      setForm(next)
      reload()
      setNotice(t('projectDetail.saved'))
    } catch (reason: any) {
      const message = reason.message || t('projectDetail.updateFailed')
      setFormError(message)
      if (reason?.name !== 'ApiError') dialogs.notify(message)
    } finally {
      setSaving(false)
    }
  }
  const applyJenkinsMigration = (result: PipelineMigrationResult) => {
    setForm((current: any) => ({
      ...current,
      config: result.config,
      repo_url: current.repo_url || result.hints.repository_url || '',
      default_branch: result.hints.default_branch || current.default_branch,
      template_id: null,
    }))
    setFormError('')
    setNotice(t('jenkinsImport.applied'))
  }
  const savePipeline = async (document: string) => {
    const next = { ...form, config: document }
    await api.updateProject(projectId, { ...next, tags: form.tags.split(',').map((tag: string) => tag.trim()).filter(Boolean) })
    setForm(next)
    reload()
    setNotice(t('projectDetail.saved'))
  }
  const runPipeline = async () => {
    const build = await api.triggerBuild(projectId)
    navigate(`/builds/${build.id}`)
    return build
  }
  const handleDelete = async () => {
    if (!await dialogs.confirm(t('projectDetail.deleteDescription'), { title: t('projectDetail.deleteTitle'), action: t('projectDetail.deleteTitle') })) return
    setDeleting(true)
    try { await api.deleteProject(projectId); navigate('/projects') } catch (reason: any) { setNotice(reason.message || t('projectDetail.deleteFailed')); setDeleting(false) }
  }
  const saveSchedule = async () => {
    if (!form) return
    const cron = schedule.mode === 'hourly' ? `${schedule.minute} * * * *` : schedule.mode === 'weekly' ? `${schedule.minute} ${schedule.hour} * * 1` : `${schedule.minute} ${schedule.hour} * * *`
    let config: string
    try {
      const { writeScheduleToPipeline } = await import('../lib/pipelineSchedule')
      config = writeScheduleToPipeline(form.config || '', cron)
    } catch (reason: any) {
      const message = reason.message || t('projectDetail.invalidPipeline')
      setFormError(message)
      dialogs.notify(message)
      return
    }
    const next = { ...form, config }
    setSaving(true); setFormError('')
    try { await api.updateProject(projectId, { ...next, tags: form.tags.split(',').map((tag: string) => tag.trim()).filter(Boolean) }); setForm(next); reload(); setShowSchedule(false); setNotice(t('projectDetail.scheduleSaved')) } catch (reason: any) { setFormError(reason.message || t('projectDetail.saveScheduleFailed')) } finally { setSaving(false) }
  }

  if (!validProjectId) return <Navigate to="/projects" replace />
  if (loading) return <PageState />
  if (error) return <PageState error={error} onRetry={reload} />
  if (!project) return null

  const buildList = builds || []
  return <motion.section className="detail-page project-detail-page" initial={{ opacity: 0, y: 8 }} animate={{ opacity: 1, y: 0 }} transition={{ duration: 0.24, ease: 'easeOut' }}>
    <header className="detail-heading"><div><div className="detail-kicker">{t('projectDetail.project')} <span>/</span> {project.repo_type || t('projectDetail.repositoryKind')}</div><div className="detail-title-row"><h1>{project.name}</h1></div><p className="detail-description">{project.description || t('projectDetail.noDescription')}</p></div><div className="detail-actions"><button className="secondary-command" onClick={() => showView('settings')}><Settings2 size={15} />{t('projects.settings')}</button>{editable && <><button className="secondary-command" onClick={() => setShowSchedule(true)}><CalendarClock size={15} />{t('projectDetail.schedule')}</button><button className="secondary-command" onClick={() => setShowCustomBuild(true)}><SlidersHorizontal size={15} />{t('builds.customBuild')}</button><button className="primary-command" onClick={handleBuildNow} disabled={building} aria-busy={building}>{building ? <LoaderCircle className="timeline-spinner" size={15} /> : <CirclePlay size={15} />}{building ? t('projects.startingBuild') : t('projects.build')}</button></>}</div></header>
    {notice && <div className="detail-notice" role="status"><span>{notice}</span><button type="button" onClick={() => setNotice('')} title={t('common.dismiss')} aria-label={t('common.dismiss')}><X size={15} /></button></div>}

    <section className="project-summary" aria-label={t('projectDetail.summary')}><article><span><Link2 size={14} />{t('projectDetail.repository')}</span><strong title={project.repo_url}>{project.repo_url || '-'}</strong></article><article><span><GitBranch size={14} />{t('builds.branch')}</span><strong>{project.default_branch || '-'}</strong></article><article><span><FolderTree size={14} />{t('projectGroups.title')}</span><strong>{projectGroupPath(projectGroups || [], project.group_id) || t('projectGroups.ungrouped')}</strong></article><article><span><Activity size={14} />{t('projectDetail.recentBuilds')}</span><strong>{buildList.length}</strong></article></section>

    <section className="detail-panel project-workbench"><nav className="detail-tabs" aria-label={t('projectDetail.views')}><button className={activeTab === 'overview' ? 'active' : ''} onClick={() => showView('overview')}><FileCode2 size={15} />{t('projectDetail.overview')}</button><button className={activeTab === 'builds' ? 'active' : ''} onClick={() => showView('builds')}><Activity size={15} />{t('builds.title')}<span>{buildList.length}</span></button><button className={activeTab === 'build' ? 'active' : ''} onClick={() => showView('build')}><Workflow size={15} />{t('projectDetail.buildSteps')}</button><button className={activeTab === 'settings' ? 'active' : ''} onClick={() => showView('settings')}><Settings2 size={15} />{t('projects.settings')}</button></nav>
      <div className="detail-panel-body">
        {activeTab === 'overview' && <div className="pipeline-preview"><header><div><FileCode2 size={16} /><h2>{t('projectDetail.pipelineConfig')}</h2></div><button className="table-link" onClick={() => showView('build')}><Workflow size={14} />{t('projectDetail.openBuildSteps')}</button></header><pre>{(() => { try { return prettyConfigSourceSync(project.config || '{}') } catch { return project.config || '{}' } })()}</pre></div>}
        {activeTab === 'builds' && <div className="operations-table-wrap"><table className="operations-table"><thead><tr><th>#</th><th>{t('builds.status')}</th><th>{t('builds.duration')}</th><th>{t('builds.branch')}</th><th>{t('projectDetail.started')}</th></tr></thead><tbody>{!buildList.length && <tr><td colSpan={5} className="operations-empty">{t('common.noData')}</td></tr>}{buildList.map(build => <tr key={build.id}><td><Link className="build-number-link" to={`/builds/${build.id}`}>#{build.number}</Link></td><td><span className={`build-status ${buildStatusTone(build.status)}`}>{buildStatusLabel(t, build.status)}</span></td><td className="muted-cell">{formatDuration(build.duration_ms)}</td><td className="branch-cell"><GitBranch size={13} />{build.branch || '-'}</td><td className="muted-cell">{build.started_at ? new Date(build.started_at).toLocaleString() : '-'}</td></tr>)}</tbody></table></div>}
        {activeTab === 'build' && form && (typeScriptPipeline ? <div className="detail-empty">TypeScript pipeline uses its typed source editor; visual editing is unavailable to preserve source semantics.</div> : <Suspense fallback={<PageState />}><PipelineEditor embedded project={{ id: project.id, name: project.name, config: form.config }} projects={[project]} globalVariables={globalVariables || []} onSave={savePipeline} onRun={runPipeline} /></Suspense>)}
        {activeTab === 'settings' && form && <div className="project-settings-stack">
          <section className="project-config-section">
            <header><div><Settings2 size={17} /><div><h2>{t('projectDetail.projectSettings')}</h2><p>{t('projectDetail.projectSettingsHelp')}</p></div></div></header>
            <form onSubmit={handleSave} className="project-settings-form">
              <div><label htmlFor="project-settings-name">{t('projects.name')}</label><input id="project-settings-name" type="text" readOnly={!editable} value={form.name} onChange={set('name')} /></div>
              <div><label htmlFor="project-settings-description">{t('projectDetail.description')}</label><textarea id="project-settings-description" readOnly={!editable} value={form.description} onChange={set('description')} placeholder={t('projects.descriptionPlaceholder')} rows={3} /><small>{t('projects.descriptionPlaceholder')}</small></div>
              <div><label htmlFor="project-settings-repository">{t('projectDetail.repositoryUrl')}</label><input id="project-settings-repository" type="text" readOnly={!editable} value={form.repo_url} onChange={set('repo_url')} /></div>
              <div><label htmlFor="project-settings-branch">{t('projects.defaultBranch')}</label><input id="project-settings-branch" type="text" readOnly={!editable} value={form.default_branch} onChange={set('default_branch')} /></div>
              <div><label htmlFor="project-settings-group">{t('projectGroups.title')}</label><select id="project-settings-group" disabled={!editable} value={form.group_id ?? ''} onChange={set('group_id')}><option value="">{t('projectGroups.ungrouped')}</option>{groups.map(group => <option key={group.id} value={group.id}>{'\u00a0\u00a0'.repeat(group.depth)}{group.path}</option>)}</select><small>{t('projectGroups.assignmentHelp')}</small></div>
              <div><label htmlFor="project-settings-tags">{t('projects.tags')}</label><input id="project-settings-tags" type="text" readOnly={!editable} value={form.tags} onChange={set('tags')} placeholder={t('projectDetail.tagsPlaceholder')} /></div>
              {!typeScriptPipeline && <div className="settings-wide"><ApprovalSettingsEditor source={form.config} disabled={!editable} onChange={config => setForm((current: any) => ({ ...current, config }))} onError={setFormError} /></div>}
              <div className="settings-wide config-editor-label"><span><label htmlFor="project-settings-pipeline">{t('projectDetail.pipelineConfig')}</label>{editable && <div className="config-editor-actions">{!typeScriptPipeline && <button type="button" className="format-command" onClick={() => showView('build')}><Workflow size={13} />{t('projectDetail.visualEditor')}</button>}<button type="button" className="format-command" onClick={() => setShowJenkinsImport(true)}><FileInput size={13} />{t('jenkinsImport.title')}</button><button type="button" className="format-command" onClick={handleFormatConfig} disabled={formatting}><Braces size={13} />{formatting ? t('config.formatting') : t('config.format')}</button></div>}</span><textarea id="project-settings-pipeline" readOnly={!editable} value={form.config} onChange={set('config')} className="code-input" rows={15} spellCheck="false" /><small>{typeScriptPipeline ? 'TypeScript DSL: @buildworld/pipeline provides autocomplete and validates when a build starts.' : t('config.saveHelp')}</small></div>
              {formError && <p className="form-error">{formError}</p>}
              {editable && <footer><button type="submit" className="primary-command" disabled={saving}>{saving ? t('common.loading') : t('common.save')}</button><button type="button" className="danger-command" disabled={deleting} onClick={handleDelete}><Trash2 size={14} />{deleting ? t('common.loading') : t('projects.delete')}</button></footer>}
            </form>
          </section>
          <ProjectGitHooks projectId={projectId} />
        </div>}
      </div>
    </section>

    {showSchedule && <ModalDialog ariaLabel={t('projectDetail.scheduleBuilds')} busy={saving} onClose={() => setShowSchedule(false)}><header><div><CalendarClock size={18} /><div><h2>{t('projectDetail.scheduleBuilds')}</h2><p>{t('projectDetail.scheduleDescription')}</p></div></div><button onClick={() => setShowSchedule(false)} title={t('common.cancel')}><X size={18} /></button></header><div className="schedule-modal-body"><label>{t('projectDetail.frequency')}<select data-dialog-initial-focus value={schedule.mode} onChange={event => setSchedule({ ...schedule, mode: event.target.value })}><option value="daily">{t('projectDetail.daily')}</option><option value="weekly">{t('projectDetail.weekly')}</option><option value="hourly">{t('projectDetail.hourly')}</option></select></label><div className="schedule-time"><label>{t('projectDetail.hour')}<input type="number" min="0" max="23" value={schedule.hour} disabled={schedule.mode === 'hourly'} onChange={event => setSchedule({ ...schedule, hour: event.target.value })} /></label><label>{t('projectDetail.minute')}<input type="number" min="0" max="59" value={schedule.minute} onChange={event => setSchedule({ ...schedule, minute: event.target.value })} /></label></div><code>{schedule.mode === 'hourly' ? `${schedule.minute} * * * *` : schedule.mode === 'weekly' ? `${schedule.minute} ${schedule.hour} * * 1` : `${schedule.minute} ${schedule.hour} * * *`}</code></div><footer><button onClick={() => setShowSchedule(false)}>{t('common.cancel')}</button><button onClick={saveSchedule} disabled={saving}>{saving ? t('projectDetail.saving') : t('projectDetail.saveSchedule')}</button></footer></ModalDialog>}
    {showJenkinsImport && <JenkinsfileImportDialog projectName={form?.name || project.name} onApply={applyJenkinsMigration} onClose={() => setShowJenkinsImport(false)} />}
    {showCustomBuild && <RunBuildDialog project={project} onClose={() => setShowCustomBuild(false)} onQueued={build => { setShowCustomBuild(false); navigate(`/builds/${build.id}`) }} />}
  </motion.section>
}
