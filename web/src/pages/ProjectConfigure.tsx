import { useEffect, useMemo, useState } from 'react'
import { Link, Navigate, useNavigate, useParams } from 'react-router-dom'
import { Braces, FileCode2, FileInput, GitBranch, Settings2, TimerReset, Wrench } from 'lucide-react'
import { api, type PipelineMigrationResult } from '../api'
import { canEdit } from '../authz'
import JenkinsfileImportDialog from '../components/JenkinsfileImportDialog'
import { PageState } from '../components/PageState'
import PipelineSourceEditor from '../components/PipelineSourceEditor'
import { useApi } from '../hooks'
import { useI18n } from '../i18n'
import { isTypeScriptPipelineSource, prettyPipelineSource, resolvePipelineSourceLanguage } from '../lib/configFormat'
import { isPipelineValidationReady, pendingPipelineValidation } from '../lib/pipelineValidation'
import { readScheduleFromPipeline, removeScheduleFromPipeline, writeScheduleToPipeline } from '../lib/pipelineSchedule'
import { sortProjectGroups } from '../lib/projectGroups'
import './ProjectConfigure.css'

type PipelineFormat = 'auto' | 'yaml' | 'typescript' | 'jenkinsfile'
type PipelineSourceMode = 'inline' | 'scm'

type ProjectConfigureForm = {
  enabled: boolean
  name: string
  description: string
  repo_url: string
  default_branch: string
  group_id: number | null
  tags: string
  config: string
  vcs_root_id: number | null
  template_id: number | null
  pipeline_format: PipelineFormat
  pipeline_source_mode: PipelineSourceMode
  pipeline_scm_repo: string
  pipeline_scm_branch: string
  pipeline_scm_path: string
}

const pipelineFormats = new Set<PipelineFormat>(['auto', 'yaml', 'typescript', 'jenkinsfile'])

function normalizePipelineFormat(project: any): PipelineFormat {
  const stored = String(project.pipeline_format || '').toLowerCase() as PipelineFormat
  if (pipelineFormats.has(stored)) return stored
  return resolvePipelineSourceLanguage(project.config || '')
}

function projectToForm(project: any): ProjectConfigureForm {
  return {
    enabled: project.enabled !== false,
    name: project.name || '',
    description: project.description || '',
    repo_url: project.repo_url || '',
    default_branch: project.default_branch || 'main',
    group_id: project.group_id ?? null,
    tags: (project.tags || []).join(', '),
    config: project.config || '',
    vcs_root_id: project.vcs_root_id ?? null,
    template_id: project.template_id ?? null,
    pipeline_format: normalizePipelineFormat(project),
    pipeline_source_mode: project.pipeline_source_mode === 'scm' ? 'scm' : 'inline',
    pipeline_scm_repo: project.pipeline_scm_repo || '',
    pipeline_scm_branch: project.pipeline_scm_branch || '',
    pipeline_scm_path: project.pipeline_scm_path || '',
  }
}

function nullableID(value: string): number | null {
  if (!value) return null
  const parsed = Number(value)
  return Number.isSafeInteger(parsed) && parsed > 0 ? parsed : null
}

function tagsFromInput(value: string): string[] {
  return [...new Map(value.split(',').map(tag => tag.trim()).filter(Boolean).map(tag => [tag.toLowerCase(), tag])).values()]
}

export default function ProjectConfigure() {
  const { t } = useI18n()
  const navigate = useNavigate()
  const { id } = useParams<{ id: string }>()
  const projectID = Number(id)
  const validProjectID = Number.isSafeInteger(projectID) && projectID > 0
  const editable = canEdit()
  const { data: project, loading, error, reload } = useApi(
    () => validProjectID ? api.getProject(projectID) : Promise.resolve(null),
    [projectID, validProjectID],
  )
  const {
    data: supportingData,
    loading: supportingLoading,
    error: supportingError,
    reload: reloadSupportingData,
  } = useApi(
    () => Promise.all([api.listProjectGroups(), api.listVCSRoots()]),
    [],
  )
  const [form, setForm] = useState<ProjectConfigureForm | null>(null)
  const [scheduleEnabled, setScheduleEnabled] = useState(false)
  const [scheduleCron, setScheduleCron] = useState('0 2 * * *')
  const [pipelineValidation, setPipelineValidation] = useState(() => pendingPipelineValidation(''))
  const [savingIntent, setSavingIntent] = useState<'save' | 'apply' | null>(null)
  const [formatting, setFormatting] = useState(false)
  const [showJenkinsImport, setShowJenkinsImport] = useState(false)
  const [formError, setFormError] = useState('')
  const [notice, setNotice] = useState('')

  useEffect(() => {
    if (!project) return
    const next = projectToForm(project)
    const cron = next.pipeline_source_mode === 'inline' && next.pipeline_format !== 'jenkinsfile'
      ? readScheduleFromPipeline(next.config)
      : ''
    setForm(next)
    setScheduleEnabled(Boolean(cron))
    setScheduleCron(cron || '0 2 * * *')
    setPipelineValidation(pendingPipelineValidation(next.config))
  }, [project])

  const groups = useMemo(() => sortProjectGroups(supportingData?.[0] || []), [supportingData])
  const vcsRoots = supportingData?.[1] || []
  const effectiveFormat = form?.pipeline_format === 'auto'
    ? resolvePipelineSourceLanguage(form.config)
    : form?.pipeline_format || 'yaml'
  const scheduleSupported = form?.pipeline_source_mode === 'inline' && effectiveFormat !== 'jenkinsfile'
  const pipelineReady = form?.pipeline_source_mode === 'scm'
    || Boolean(form && isPipelineValidationReady(pipelineValidation, form.config))
  const pipelineBlockedMessage = pipelineValidation.message || t('config.resolveErrors')

  const setText = (key: keyof ProjectConfigureForm) => (
    event: React.ChangeEvent<HTMLInputElement | HTMLTextAreaElement | HTMLSelectElement>,
  ) => setForm(current => current ? { ...current, [key]: event.target.value } : current)

  const setID = (key: 'group_id' | 'vcs_root_id') => (
    event: React.ChangeEvent<HTMLSelectElement>,
  ) => setForm(current => current ? { ...current, [key]: nullableID(event.target.value) } : current)

  const setSourceMode = (event: React.ChangeEvent<HTMLSelectElement>) => {
    const mode = event.target.value === 'scm' ? 'scm' : 'inline'
    setForm(current => current ? {
      ...current,
      pipeline_source_mode: mode,
      pipeline_scm_repo: mode === 'scm' && !current.pipeline_scm_repo ? current.repo_url : current.pipeline_scm_repo,
      pipeline_scm_branch: mode === 'scm' && !current.pipeline_scm_branch ? current.default_branch : current.pipeline_scm_branch,
      pipeline_scm_path: mode === 'scm' && !current.pipeline_scm_path
        ? current.pipeline_format === 'jenkinsfile' ? 'Jenkinsfile' : 'buildworld.yaml'
        : current.pipeline_scm_path,
    } : current)
    setFormError('')
  }

  const setPipelineFormat = (event: React.ChangeEvent<HTMLSelectElement>) => {
    const format = pipelineFormats.has(event.target.value as PipelineFormat)
      ? event.target.value as PipelineFormat
      : 'auto'
    setForm(current => current ? { ...current, pipeline_format: format } : current)
    setPipelineValidation(current => form ? pendingPipelineValidation(form.config) : current)
    setFormError('')
  }

  const handleFormat = async () => {
    if (!form || form.pipeline_source_mode === 'scm') return
    setFormatting(true)
    setFormError('')
    try {
      const config = await prettyPipelineSource(form.config)
      await api.validatePipeline(config)
      setForm(current => current ? { ...current, config } : current)
      setPipelineValidation(pendingPipelineValidation(config))
      setNotice(isTypeScriptPipelineSource(config) ? t('config.validated') : t('config.formatted'))
    } catch (reason: any) {
      setFormError(reason.message || t('config.invalid'))
    } finally {
      setFormatting(false)
    }
  }

  const applyJenkinsMigration = (result: PipelineMigrationResult) => {
    const cron = readScheduleFromPipeline(result.config)
    setForm(current => current ? {
      ...current,
      config: result.config,
      repo_url: current.repo_url || result.hints.repository_url || '',
      default_branch: result.hints.default_branch || current.default_branch,
      pipeline_format: resolvePipelineSourceLanguage(result.config),
      pipeline_source_mode: 'inline',
    } : current)
    setScheduleEnabled(Boolean(cron))
    setScheduleCron(cron || '0 2 * * *')
    setPipelineValidation(pendingPipelineValidation(result.config))
    setFormError('')
    setNotice(t('jenkinsImport.applied'))
  }

  const persist = async (intent: 'save' | 'apply') => {
    if (!form) return
    if (form.pipeline_source_mode === 'inline' && !pipelineReady) {
      setFormError(pipelineBlockedMessage)
      return
    }
    if (form.pipeline_source_mode === 'scm' && (!form.pipeline_scm_repo.trim() || !form.pipeline_scm_path.trim())) {
      setFormError(t('projectDetail.scmPipelineRequired'))
      return
    }

    setSavingIntent(intent)
    setFormError('')
    setNotice('')
    try {
      let config = form.config
      if (form.pipeline_source_mode === 'inline') {
        if (scheduleSupported) {
          config = scheduleEnabled
            ? writeScheduleToPipeline(config, scheduleCron.trim())
            : removeScheduleFromPipeline(config)
        }
        config = await prettyPipelineSource(config)
        await api.validatePipeline(config)
      }
      const payload = {
        enabled: form.enabled,
        name: form.name,
        description: form.description,
        repo_url: form.repo_url,
        repo_type: 'git',
        default_branch: form.default_branch,
        vcs_root_id: form.vcs_root_id,
        template_id: form.template_id,
        group_id: form.group_id,
        tags: tagsFromInput(form.tags),
        config,
        pipeline_format: form.pipeline_format,
        pipeline_source_mode: form.pipeline_source_mode,
        pipeline_scm_repo: form.pipeline_scm_repo,
        pipeline_scm_branch: form.pipeline_scm_branch,
        pipeline_scm_path: form.pipeline_scm_path,
      }
      await api.updateProject(projectID, payload)
      if (intent === 'save') {
        navigate(`/projects/${projectID}`)
        return
      }
      setForm(current => current ? { ...current, config } : current)
      setPipelineValidation(pendingPipelineValidation(config))
      setNotice(t('projectDetail.saved'))
      reload()
    } catch (reason: any) {
      setFormError(reason.message || t('projectDetail.updateFailed'))
    } finally {
      setSavingIntent(null)
    }
  }

  const handleSubmit = (event: React.FormEvent<HTMLFormElement>) => {
    event.preventDefault()
    const submitter = (event.nativeEvent as SubmitEvent).submitter as HTMLButtonElement | null
    void persist(submitter?.value === 'apply' ? 'apply' : 'save')
  }

  if (!validProjectID) return <Navigate to="/projects" replace />
  if (loading || supportingLoading) return <PageState />
  if (error || supportingError) {
    return <PageState error={error || supportingError || t('common.loadFailed')} onRetry={() => { reload(); reloadSupportingData() }} />
  }
  if (!project || !form) return null

  return <section className="jenkins-configure-page">
    <header className="jenkins-configure-header">
      <nav aria-label={t('projectDetail.breadcrumb')}>
        <Link to="/">BuildWorld</Link><span>/</span><Link to={`/projects/${projectID}`}>{project.name}</Link><span>/</span><strong>{t('projectDetail.configure')}</strong>
      </nav>
      <h1>{t('projectDetail.configure')}</h1>
    </header>

    <aside className="jenkins-configure-left">
      <nav className="jenkins-configure-nav" aria-label={t('projectDetail.configure')}>
        <a href="#jenkins-configure-general"><Settings2 size={16} />{t('settings.general')}</a>
        <a href="#jenkins-configure-triggers"><TimerReset size={16} />{t('builds.trigger')}</a>
        <a href="#jenkins-configure-pipeline"><FileCode2 size={16} />{t('projectDetail.pipeline')}</a>
        <a href="#jenkins-configure-advanced"><Wrench size={16} />{t('vcsRoots.config')}</a>
      </nav>
    </aside>

    <main className="jenkins-configure-main">
      {notice && <p className="jenkins-configure-notice" role="status">{notice}</p>}
      {formError && <p className="jenkins-configure-error" role="alert">{formError}</p>}
      <form onSubmit={handleSubmit}>
        <section id="jenkins-configure-general" className="jenkins-configure-section" aria-labelledby="jenkins-configure-general-title">
          <div className="jenkins-configure-general-heading">
            <h2 id="jenkins-configure-general-title">{t('settings.general')}</h2>
            <label className="jenkins-configure-enabled">
              <span>{t('common.enable')}</span>
              <input type="checkbox" role="switch" checked={form.enabled} disabled={!editable} onChange={event => setForm(current => current ? { ...current, enabled: event.target.checked } : current)} />
              <i aria-hidden="true" />
            </label>
          </div>
          <div className="jenkins-configure-fields">
            <label><span>{t('projects.name')}</span><input id="jenkins-configure-name" required readOnly={!editable} value={form.name} onChange={setText('name')} /></label>
            <label className="jenkins-configure-wide"><span>{t('projectDetail.description')}</span><textarea id="jenkins-configure-description" readOnly={!editable} rows={4} value={form.description} onChange={setText('description')} /></label>
            <label className="jenkins-configure-wide"><span>{t('projectDetail.repositoryUrl')}</span><input readOnly={!editable} value={form.repo_url} onChange={setText('repo_url')} /></label>
            <label><span>{t('projects.defaultBranch')}</span><input readOnly={!editable} value={form.default_branch} onChange={setText('default_branch')} /></label>
            <label><span>{t('projectGroups.title')}</span><select disabled={!editable} value={form.group_id ?? ''} onChange={setID('group_id')}><option value="">{t('projectGroups.ungrouped')}</option>{groups.map((group: any) => <option key={group.id} value={group.id}>{group.name}</option>)}</select></label>
            <label className="jenkins-configure-wide"><span>{t('projects.tags')}</span><input readOnly={!editable} value={form.tags} onChange={setText('tags')} placeholder={t('projectDetail.tagsPlaceholder')} /><small>{t('projects.tagsHelp')}</small></label>
          </div>
        </section>

        <section id="jenkins-configure-triggers" className="jenkins-configure-section" aria-labelledby="jenkins-configure-triggers-title">
          <h2 id="jenkins-configure-triggers-title">{t('builds.trigger')}</h2>
          {scheduleSupported ? <fieldset className="jenkins-configure-trigger" disabled={!editable}>
            <label className="jenkins-configure-check"><input type="checkbox" checked={scheduleEnabled} onChange={event => setScheduleEnabled(event.target.checked)} /><span>{t('projectDetail.scheduleBuilds')}</span></label>
            {scheduleEnabled && <label><span>Cron</span><input required value={scheduleCron} onChange={event => setScheduleCron(event.target.value)} placeholder="0 2 * * *" /><small>{t('projectDetail.scheduleDescription')}</small></label>}
          </fieldset> : <div className="jenkins-configure-disabled"><TimerReset size={18} /><div><strong>{t('common.disabled')}</strong><p>{t('projectDetail.triggerUnavailableForSource').replace('{source}', form.pipeline_source_mode === 'scm' ? t('projectDetail.pipelineScriptFromSCM') : 'Jenkinsfile')}</p></div></div>}
        </section>

        <section id="jenkins-configure-pipeline" className="jenkins-configure-section" aria-labelledby="jenkins-configure-pipeline-title">
          <div className="jenkins-configure-section-heading"><div><h2 id="jenkins-configure-pipeline-title">{t('projectDetail.pipeline')}</h2><p>{t('projects.buildFlowHelp')}</p></div>{editable && form.pipeline_source_mode === 'inline' && <div className="jenkins-configure-tools"><button type="button" onClick={() => setShowJenkinsImport(true)}><FileInput size={14} />{t('jenkinsImport.title')}</button><button type="button" onClick={handleFormat} disabled={formatting}><Braces size={14} />{formatting ? t('config.formatting') : effectiveFormat === 'typescript' ? t('config.validate') : t('config.format')}</button></div>}</div>
          {form.pipeline_source_mode === 'inline' ? <div className="jenkins-configure-editor"><PipelineSourceEditor ariaLabel={t('pipeline.sourceLabel')} value={form.config} readOnly={!editable} language={effectiveFormat} onChange={config => { setForm(current => current ? { ...current, config } : current); setPipelineValidation(pendingPipelineValidation(config)) }} onValidationChange={setPipelineValidation} statusId="jenkins-configure-pipeline-status" height={440} /></div> : <div className="jenkins-configure-disabled"><GitBranch size={18} /><div><strong>{t('projectDetail.pipelineScriptFromSCM')}</strong><p>{form.pipeline_scm_path || t('common.noData')}</p></div></div>}
        </section>

        <section id="jenkins-configure-advanced" className="jenkins-configure-section" aria-labelledby="jenkins-configure-advanced-title">
          <h2 id="jenkins-configure-advanced-title">{t('vcsRoots.config')}</h2>
          <div className="jenkins-configure-fields">
            <label><span>{t('projects.repositoryType')}</span><input id="jenkins-configure-repository-type" value={t('projects.gitOnly')} readOnly aria-readonly="true" /></label>
            <label><span>{t('projectDetail.definition')}</span><select disabled={!editable} value={form.pipeline_source_mode} onChange={setSourceMode}><option value="inline">{t('projectDetail.pipelineScript')}</option><option value="scm">{t('projectDetail.pipelineScriptFromSCM')}</option></select></label>
            <label><span>{t('projects.sourceFormat')}</span><select disabled={!editable} value={form.pipeline_format} onChange={setPipelineFormat}><option value="auto">{t('projectDetail.autoDetect')}</option><option value="yaml">YAML</option><option value="typescript">TypeScript</option><option value="jenkinsfile">Jenkinsfile</option></select></label>
            <label><span>{t('vcsRoots.title')}</span><select disabled={!editable} value={form.vcs_root_id ?? ''} onChange={setID('vcs_root_id')}><option value="">{t('vcsRoots.none')}</option>{vcsRoots.map((root: any) => <option key={root.id} value={root.id}>{root.name}</option>)}</select></label>
            {form.pipeline_source_mode === 'scm' && <>
              <label className="jenkins-configure-wide"><span>{t('projectDetail.repositoryUrl')}</span><input required readOnly={!editable} value={form.pipeline_scm_repo} onChange={setText('pipeline_scm_repo')} /></label>
              <label><span>{t('vcsRoots.branch')}</span><input readOnly={!editable} value={form.pipeline_scm_branch} onChange={setText('pipeline_scm_branch')} placeholder="main" /></label>
              <label><span>{t('projectDetail.scriptPath')}</span><input required readOnly={!editable} value={form.pipeline_scm_path} onChange={setText('pipeline_scm_path')} placeholder="Jenkinsfile" /></label>
            </>}
          </div>
        </section>

        {editable && <footer className="jenkins-configure-actions">
          <button className="jenkins-configure-save" type="submit" name="intent" value="save" disabled={Boolean(savingIntent) || !pipelineReady} title={!pipelineReady ? pipelineBlockedMessage : undefined}>{savingIntent === 'save' ? t('projectDetail.saving') : t('common.save')}</button>
          <button type="submit" name="intent" value="apply" disabled={Boolean(savingIntent) || !pipelineReady} title={!pipelineReady ? pipelineBlockedMessage : undefined}>{savingIntent === 'apply' ? t('projectDetail.saving') : t('projectDetail.apply')}</button>
        </footer>}
      </form>
    </main>

    {showJenkinsImport && <JenkinsfileImportDialog projectName={form.name} onApply={applyJenkinsMigration} onClose={() => setShowJenkinsImport(false)} />}
  </section>
}
