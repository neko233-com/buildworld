import { useEffect, useMemo, useRef, useState } from 'react'
import { Navigate, useNavigate, useParams } from 'react-router-dom'
import { Braces, FileCode2, FileInput, GitBranch, Settings2, TimerReset, Wrench } from 'lucide-react'
import { api, type PipelineMigrationResult } from '../api'
import { canEdit } from '../authz'
import { dialogs } from '../components/AppDialogs'
import JenkinsfileImportDialog from '../components/JenkinsfileImportDialog'
import { JenkinsHeaderBreadcrumb } from '../components/JenkinsPageShell'
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
type ValidationField = 'name' | 'schedule' | 'pipeline' | 'scmRepo' | 'scmPath'

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

function configureSnapshot(form: ProjectConfigureForm, scheduleEnabled: boolean, scheduleCron: string): string {
  return JSON.stringify({ form, scheduleEnabled, scheduleCron })
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
    () => Promise.all([api.listProjectGroups(), api.listVCSRoots(), api.listTemplates()]),
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
  const [savedSnapshot, setSavedSnapshot] = useState('')
  const [validationErrors, setValidationErrors] = useState<Partial<Record<ValidationField, boolean>>>({})
  const pipelineEditorRef = useRef<HTMLDivElement>(null)

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
    setSavedSnapshot(configureSnapshot(next, Boolean(cron), cron || '0 2 * * *'))
    setValidationErrors({})
  }, [project])

  const groups = useMemo(() => sortProjectGroups(supportingData?.[0] || []), [supportingData])
  const vcsRoots = supportingData?.[1] || []
  const templates = supportingData?.[2] || []
  const effectiveFormat = form?.pipeline_format === 'auto'
    ? resolvePipelineSourceLanguage(form.config)
    : form?.pipeline_format || 'yaml'
  const scheduleSupported = form?.pipeline_source_mode === 'inline' && effectiveFormat !== 'jenkinsfile'
  const scmJenkinsfileTriggers = form?.pipeline_source_mode === 'scm' && effectiveFormat === 'jenkinsfile'
  const pipelineReady = form?.pipeline_source_mode === 'scm'
    || Boolean(form && isPipelineValidationReady(pipelineValidation, form.config))
  const pipelineBlockedMessage = pipelineValidation.message || t('config.resolveErrors')
  const currentSnapshot = useMemo(
    () => form ? configureSnapshot(form, scheduleEnabled, scheduleCron) : '',
    [form, scheduleCron, scheduleEnabled],
  )
  const dirty = Boolean(savedSnapshot && currentSnapshot !== savedSnapshot)

  useEffect(() => {
    if (!dirty) return
    const warnBeforeUnload = (event: BeforeUnloadEvent) => {
      event.preventDefault()
      event.returnValue = ''
    }
    window.addEventListener('beforeunload', warnBeforeUnload)
    return () => window.removeEventListener('beforeunload', warnBeforeUnload)
  }, [dirty])

  const clearValidationError = (field: ValidationField) => {
    setValidationErrors(current => current[field] ? { ...current, [field]: false } : current)
  }

  const focusValidationError = (field: ValidationField) => {
    queueMicrotask(() => {
      const ids: Partial<Record<ValidationField, string>> = {
        name: 'jenkins-configure-name',
        schedule: 'jenkins-configure-schedule-cron',
        scmRepo: 'jenkins-configure-scm-repo',
        scmPath: 'jenkins-configure-scm-path',
      }
      if (field === 'pipeline') {
        const editor = pipelineEditorRef.current
        const target = editor?.querySelector<HTMLElement>('textarea, [role="textbox"], [contenteditable="true"]') || editor
        target?.focus()
        return
      }
      document.getElementById(ids[field] || '')?.focus()
    })
  }

  const setText = (key: keyof ProjectConfigureForm) => (
    event: React.ChangeEvent<HTMLInputElement | HTMLTextAreaElement | HTMLSelectElement>,
  ) => {
    setForm(current => current ? { ...current, [key]: event.target.value } : current)
    const fieldByKey: Partial<Record<keyof ProjectConfigureForm, ValidationField>> = {
      name: 'name',
      pipeline_scm_repo: 'scmRepo',
      pipeline_scm_path: 'scmPath',
    }
    const field = fieldByKey[key]
    if (field) clearValidationError(field)
  }

  const setID = (key: 'group_id' | 'vcs_root_id' | 'template_id') => (
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
    setValidationErrors({})
  }

  const setPipelineFormat = (event: React.ChangeEvent<HTMLSelectElement>) => {
    const format = pipelineFormats.has(event.target.value as PipelineFormat)
      ? event.target.value as PipelineFormat
      : 'auto'
    setForm(current => current ? { ...current, pipeline_format: format } : current)
    setPipelineValidation(current => form ? pendingPipelineValidation(form.config) : current)
    setFormError('')
    setValidationErrors({})
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
    setValidationErrors({})
  }

  const persist = async (intent: 'save' | 'apply') => {
    if (!form) return
    if (!form.name.trim()) {
      setValidationErrors({ name: true })
      setFormError(`${t('projects.name')}: ${t('approvals.required')}`)
      focusValidationError('name')
      return
    }
    if (scheduleSupported && scheduleEnabled && !scheduleCron.trim()) {
      setValidationErrors({ schedule: true })
      setFormError(`Cron: ${t('approvals.required')}`)
      focusValidationError('schedule')
      return
    }
    if (form.pipeline_source_mode === 'inline' && !pipelineReady) {
      setValidationErrors({ pipeline: true })
      setFormError(pipelineBlockedMessage)
      focusValidationError('pipeline')
      return
    }
    if (form.pipeline_source_mode === 'scm' && !form.pipeline_scm_repo.trim()) {
      setValidationErrors({ scmRepo: true })
      setFormError(t('projectDetail.scmPipelineRequired'))
      focusValidationError('scmRepo')
      return
    }
    if (form.pipeline_source_mode === 'scm' && !form.pipeline_scm_path.trim()) {
      setValidationErrors({ scmPath: true })
      setFormError(t('projectDetail.scmPipelineRequired'))
      focusValidationError('scmPath')
      return
    }

    setSavingIntent(intent)
    setFormError('')
    setNotice('')
    setValidationErrors({})
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
      const savedForm = { ...form, config }
      setForm(savedForm)
      setSavedSnapshot(configureSnapshot(savedForm, scheduleEnabled, scheduleCron))
      if (intent === 'save') {
        navigate(`/projects/${projectID}`)
        return
      }
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

  const handleCancel = async () => {
    if (dirty && !await dialogs.confirm(t('pipeline.discardMessage'), {
      title: t('pipeline.discardTitle'),
      action: t('pipeline.discardAction'),
    })) return
    navigate(`/projects/${projectID}`)
  }

  if (!validProjectID) return <Navigate to="/projects" replace />
  if (loading || supportingLoading) return <PageState />
  if (error || supportingError) {
    return <PageState error={error || supportingError || t('common.loadFailed')} onRetry={() => { reload(); reloadSupportingData() }} />
  }
  if (!project || !form) return null

  return <section className="jenkins-configure-page">
    <JenkinsHeaderBreadcrumb ariaLabel={t('projectDetail.breadcrumb')} breadcrumbs={[
      { label: project.name, to: `/projects/${projectID}` },
      { label: t('projectDetail.configure') },
    ]} />
    <header className="jenkins-configure-header">
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

    <div className="jenkins-configure-main">
      {notice && <p className="jenkins-configure-notice" role="status">{notice}</p>}
      {formError && <p id="jenkins-configure-form-error" className="jenkins-configure-error" role="alert">{formError}</p>}
      <form onSubmit={handleSubmit} noValidate aria-busy={Boolean(savingIntent)}>
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
            <label><span>{t('projects.name')}</span><input id="jenkins-configure-name" required readOnly={!editable} aria-invalid={validationErrors.name || undefined} aria-describedby={validationErrors.name ? 'jenkins-configure-form-error' : undefined} value={form.name} onChange={setText('name')} /></label>
            <label className="jenkins-configure-wide"><span>{t('projectDetail.description')}</span><textarea id="jenkins-configure-description" readOnly={!editable} rows={4} value={form.description} onChange={setText('description')} /></label>
            <label className="jenkins-configure-wide"><span>{t('projectDetail.repositoryUrl')}</span><input readOnly={!editable} value={form.repo_url} onChange={setText('repo_url')} /></label>
            <label><span>{t('projects.defaultBranch')}</span><input readOnly={!editable} value={form.default_branch} onChange={setText('default_branch')} /></label>
            <label><span>{t('projectGroups.title')}</span><select disabled={!editable} value={form.group_id ?? ''} onChange={setID('group_id')}><option value="">{t('projectGroups.ungrouped')}</option>{groups.map((group: any) => <option key={group.id} value={group.id}>{group.name}</option>)}</select></label>
            <label><span>{t('templates.title')}</span><select id="jenkins-configure-template" disabled={!editable} value={form.template_id ?? ''} onChange={setID('template_id')}><option value="">{t('templates.none')}</option>{templates.map((template: any) => <option key={template.id} value={template.id}>{template.name}</option>)}</select><small>{t('templates.selectionHelp')}</small></label>
            <label className="jenkins-configure-wide"><span>{t('projects.tags')}</span><input readOnly={!editable} value={form.tags} onChange={setText('tags')} placeholder={t('projectDetail.tagsPlaceholder')} /><small>{t('projects.tagsHelp')}</small></label>
          </div>
        </section>

        <section id="jenkins-configure-triggers" className="jenkins-configure-section jenkins-configure-group" aria-labelledby="jenkins-configure-triggers-title">
          <div className="jenkins-configure-group-heading">
            <TimerReset size={19} aria-hidden="true" />
            <h2 id="jenkins-configure-triggers-title">{t('builds.trigger')}</h2>
          </div>
          {scheduleSupported ? <fieldset className="jenkins-configure-trigger" disabled={!editable}>
            <label className="jenkins-configure-check"><input type="checkbox" checked={scheduleEnabled} onChange={event => setScheduleEnabled(event.target.checked)} /><span>{t('projectDetail.scheduleBuilds')}</span></label>
            {scheduleEnabled && <label><span>Cron</span><input id="jenkins-configure-schedule-cron" required aria-invalid={validationErrors.schedule || undefined} aria-describedby={validationErrors.schedule ? 'jenkins-configure-form-error' : undefined} value={scheduleCron} onChange={event => { setScheduleCron(event.target.value); clearValidationError('schedule') }} placeholder="0 2 * * *" /><small>{t('projectDetail.scheduleDescription')}</small></label>}
          </fieldset> : scmJenkinsfileTriggers ? <div className="jenkins-configure-disabled"><TimerReset size={18} /><div><strong>{t('projectDetail.triggerManagedByJenkinsfileTitle')}</strong><p>{t('projectDetail.triggerManagedByJenkinsfile')}</p></div></div> : <div className="jenkins-configure-disabled"><TimerReset size={18} /><div><strong>{t('common.disabled')}</strong><p>{t('projectDetail.triggerUnavailableForSource').replace('{source}', form.pipeline_source_mode === 'scm' ? t('projectDetail.pipelineScriptFromSCM') : 'Jenkinsfile')}</p></div></div>}
        </section>

        <section id="jenkins-configure-pipeline" className="jenkins-configure-section" aria-labelledby="jenkins-configure-pipeline-title">
          <div className="jenkins-configure-section-heading"><div><h2 id="jenkins-configure-pipeline-title">{t('projectDetail.pipeline')}</h2><p>{t('projects.buildFlowHelp')}</p></div>{editable && form.pipeline_source_mode === 'inline' && <div className="jenkins-configure-tools"><button type="button" onClick={() => setShowJenkinsImport(true)}><FileInput size={14} />{t('jenkinsImport.title')}</button><button type="button" onClick={handleFormat} disabled={formatting}><Braces size={14} />{formatting ? t('config.formatting') : effectiveFormat === 'typescript' ? t('config.validate') : t('config.format')}</button></div>}</div>
          {form.pipeline_source_mode === 'inline' ? <div ref={pipelineEditorRef} className="jenkins-configure-editor" tabIndex={-1} aria-invalid={validationErrors.pipeline || undefined} aria-describedby={validationErrors.pipeline ? 'jenkins-configure-form-error' : undefined}><PipelineSourceEditor ariaLabel={t('pipeline.sourceLabel')} value={form.config} readOnly={!editable} language={effectiveFormat} onChange={config => { setForm(current => current ? { ...current, config } : current); setPipelineValidation(pendingPipelineValidation(config)); clearValidationError('pipeline') }} onValidationChange={setPipelineValidation} statusId="jenkins-configure-pipeline-status" height={440} /></div> : <div className="jenkins-configure-disabled"><GitBranch size={18} /><div><strong>{t('projectDetail.pipelineScriptFromSCM')}</strong><p>{form.pipeline_scm_path || t('common.noData')}</p></div></div>}
        </section>

        <section id="jenkins-configure-advanced" className="jenkins-configure-section jenkins-configure-group" aria-labelledby="jenkins-configure-advanced-title">
          <div className="jenkins-configure-group-heading">
            <Wrench size={19} aria-hidden="true" />
            <h2 id="jenkins-configure-advanced-title">{t('vcsRoots.config')}</h2>
          </div>
          <div className="jenkins-configure-fields">
            <label><span>{t('projects.repositoryType')}</span><input id="jenkins-configure-repository-type" value={t('projects.gitOnly')} readOnly aria-readonly="true" /></label>
            <label><span>{t('projectDetail.definition')}</span><select disabled={!editable} value={form.pipeline_source_mode} onChange={setSourceMode}><option value="inline">{t('projectDetail.pipelineScript')}</option><option value="scm">{t('projectDetail.pipelineScriptFromSCM')}</option></select></label>
            <label><span>{t('projects.sourceFormat')}</span><select disabled={!editable} value={form.pipeline_format} onChange={setPipelineFormat}><option value="auto">{t('projectDetail.autoDetect')}</option><option value="yaml">YAML</option><option value="typescript">TypeScript</option><option value="jenkinsfile">Jenkinsfile</option></select></label>
            <label><span>{t('vcsRoots.title')}</span><select disabled={!editable} value={form.vcs_root_id ?? ''} onChange={setID('vcs_root_id')}><option value="">{t('vcsRoots.none')}</option>{vcsRoots.map((root: any) => <option key={root.id} value={root.id}>{root.name}</option>)}</select></label>
            {form.pipeline_source_mode === 'scm' && <>
              <label className="jenkins-configure-wide"><span>{t('projectDetail.repositoryUrl')}</span><input id="jenkins-configure-scm-repo" required readOnly={!editable} aria-invalid={validationErrors.scmRepo || undefined} aria-describedby={validationErrors.scmRepo ? 'jenkins-configure-form-error' : undefined} value={form.pipeline_scm_repo} onChange={setText('pipeline_scm_repo')} /></label>
              <label><span>{t('vcsRoots.branch')}</span><input readOnly={!editable} value={form.pipeline_scm_branch} onChange={setText('pipeline_scm_branch')} placeholder="main" /></label>
              <label><span>{t('projectDetail.scriptPath')}</span><input id="jenkins-configure-scm-path" required readOnly={!editable} aria-invalid={validationErrors.scmPath || undefined} aria-describedby={validationErrors.scmPath ? 'jenkins-configure-form-error' : undefined} value={form.pipeline_scm_path} onChange={setText('pipeline_scm_path')} placeholder="Jenkinsfile" /></label>
            </>}
          </div>
        </section>

        {editable && <footer className="jenkins-configure-actions">
          <span className={dirty ? 'jenkins-configure-save-state dirty' : 'jenkins-configure-save-state'} role="status">{dirty ? t('pipeline.unsaved') : t('pipeline.saved')}</span>
          <button className="jenkins-configure-save" type="submit" name="intent" value="save" disabled={Boolean(savingIntent)}>{savingIntent === 'save' ? t('projectDetail.saving') : t('common.save')}</button>
          <button type="submit" name="intent" value="apply" disabled={Boolean(savingIntent)}>{savingIntent === 'apply' ? t('projectDetail.saving') : t('projectDetail.apply')}</button>
          <button type="button" onClick={() => void handleCancel()} disabled={Boolean(savingIntent)}>{t('common.cancel')}</button>
        </footer>}
      </form>
    </div>

    {showJenkinsImport && <JenkinsfileImportDialog projectName={form.name} onApply={applyJenkinsMigration} onClose={() => setShowJenkinsImport(false)} />}
  </section>
}
