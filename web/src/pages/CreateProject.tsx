import { useEffect, useMemo, useState } from 'react'
import { motion } from 'motion/react'
import { useNavigate } from 'react-router-dom'
import { Braces, Check, Code2, FileCode2, FileInput, FolderGit2, Plus, Tag, X } from 'lucide-react'
import { useI18n } from '../i18n'
import { api, type PipelineMigrationResult } from '../api'
import { useApi } from '../hooks'
import { isTypeScriptPipelineSource, prettyPipelineSource } from '../lib/configFormat'
import { sortProjectGroups } from '../lib/projectGroups'
import {
  isPipelineValidationReady,
  pendingPipelineValidation,
} from '../lib/pipelineValidation'
import ApprovalSettingsEditor from '../components/ApprovalSettingsEditor'
import JenkinsfileImportDialog from '../components/JenkinsfileImportDialog'
import PipelineSourceEditor from '../components/PipelineSourceEditor'
import { dialogs } from '../components/AppDialogs'

const SAMPLE_YAML = `name: Build and Deploy
on: [push, manual]
env:
  NODE_ENV: production
jobs:
  build:
    runs-on: local
    steps:
      - uses: actions/checkout@v4
      - name: Install
        run: npm install
        shell: bash
      - name: Build
        run: npm run build
        shell: bash
  test:
    needs: build
    runs-on: local
    steps:
      - name: Test
        run: npm test
        shell: bash
`

const SAMPLE_TYPESCRIPT = `import { definePipeline, shell, stage, watchService } from '@buildworld/pipeline'

export default definePipeline({
  name: 'Build and Deploy',
  stages: [
    stage('Build', shell('Compile', 'npm run build')),
    // After your start stage writes a PID and log file, this keeps the build
    // running automatically and streams new output like Jenkins tail -f.
    // Set initialLines: 0 to skip existing history. An explicit timeoutSec
    // still limits the observer.
    // stage('Observe', watchService('Service log', { targetDir: '/srv/app', pidFile: 'server.pid', logFile: 'server.log', port: 8700, initialLines: 0 })),
  ],
})
`

export default function CreateProject() {
  const { t } = useI18n()
  const navigate = useNavigate()
  const [format, setFormat] = useState<'yaml' | 'ts' | 'jenkinsfile'>('yaml')
  const [form, setForm] = useState({ name: '', description: '', repo_url: '', default_branch: 'main', config: SAMPLE_YAML, vcs_root_id: '' as number | '', group_id: '' as number | '', tags: [] as string[] })
  const [tagDraft, setTagDraft] = useState('')
  const [saving, setSaving] = useState(false)
  const [formatting, setFormatting] = useState(false)
  const [showJenkinsImport, setShowJenkinsImport] = useState(false)
  const [error, setError] = useState('')
  const [pipelineValidation, setPipelineValidation] = useState(() => pendingPipelineValidation(SAMPLE_YAML))
  const { data: vcsRoots } = useApi(() => api.listVCSRoots())
  const { data: projects } = useApi(() => api.listProjects())
  const { data: projectGroups } = useApi(() => api.listProjectGroups())
  const knownTags = useMemo<string[]>(() => [...new Map<string, string>((projects || []).flatMap(project => (project.tags || []).map((tag: string): [string, string] => [tag.toLowerCase(), tag]))).values()].sort((a, b) => a.localeCompare(b)), [projects])
  const groups = useMemo(() => sortProjectGroups(projectGroups || []), [projectGroups])
  const pipelineReady = isPipelineValidationReady(pipelineValidation, form.config)
  const pipelineBlockedMessage = pipelineValidation.message || t('config.resolveErrors')

  useEffect(() => {
    if (form.vcs_root_id === '' || form.vcs_root_id === undefined) return
    const root = vcsRoots?.find((item: any) => item.id === Number(form.vcs_root_id))
    if (root) setForm(previous => ({ ...previous, repo_url: root.url || previous.repo_url, default_branch: root.branch || previous.default_branch }))
  }, [form.vcs_root_id, vcsRoots])

  const set = (key: Exclude<keyof typeof form, 'tags'>) => (event: React.ChangeEvent<HTMLInputElement | HTMLTextAreaElement | HTMLSelectElement>) => {
    const value = event.target.value
    setForm(previous => ({ ...previous, [key]: key === 'vcs_root_id' || key === 'group_id' ? value === '' ? '' : Number(value) : value }))
  }
  const addTag = (value = tagDraft) => {
    const tag = value.trim().replace(/,$/, '')
    if (tag && !form.tags.some(item => item.toLowerCase() === tag.toLowerCase())) setForm(previous => ({ ...previous, tags: [...previous.tags, tag] }))
    setTagDraft('')
  }
  const toggleTag = (tag: string) => setForm(previous => ({ ...previous, tags: previous.tags.some(item => item.toLowerCase() === tag.toLowerCase()) ? previous.tags.filter(item => item.toLowerCase() !== tag.toLowerCase()) : [...previous.tags, tag] }))
  const switchFormat = (nextFormat: 'yaml' | 'ts') => {
    setFormat(nextFormat)
    const config = nextFormat === 'ts' ? SAMPLE_TYPESCRIPT : SAMPLE_YAML
    setPipelineValidation(pendingPipelineValidation(config))
    setForm(previous => ({ ...previous, config }))
  }
  const handleFormatConfig = async () => {
    setFormatting(true); setError('')
    try {
      const config = format === 'ts' ? form.config : await prettyPipelineSource(form.config)
      await api.validatePipeline(config)
      setForm(previous => ({ ...previous, config }))
      if (format === 'ts' && (!pipelineValidation.diagnosticsReady || pipelineValidation.diagnosticErrors > 0 || pipelineValidation.source !== config)) {
        throw new Error(pipelineBlockedMessage)
      }
      dialogs.notify(format === 'ts' ? t('config.validated') : t('config.formatted'), 'success')
    } catch (reason: any) {
      const message = reason.message || t('config.invalid')
      setError(message)
      dialogs.notify(t('config.invalid'))
    } finally {
      setFormatting(false)
    }
  }
  const applyJenkinsMigration = (result: PipelineMigrationResult) => {
    setFormat(isTypeScriptPipelineSource(result.config) ? 'ts' : 'yaml')
    setPipelineValidation(pendingPipelineValidation(result.config))
    setError('')
    setForm(previous => ({
      ...previous,
      config: result.config,
      repo_url: previous.repo_url || result.hints.repository_url || '',
      default_branch: result.hints.default_branch || previous.default_branch,
    }))
  }
  const handleSubmit = async (event: React.FormEvent) => {
    event.preventDefault()
    if (!pipelineReady) {
      setError(pipelineBlockedMessage)
      return
    }
    setSaving(true); setError('')
    try {
      const config = await prettyPipelineSource(form.config)
      await api.validatePipeline(config)
      const data: any = {
        name: form.name,
        description: form.description,
        repo_url: form.repo_url,
        repo_type: 'git',
        default_branch: form.default_branch,
        config,
        tags: form.tags,
        pipeline_format: format === 'ts' ? 'typescript' : format,
        pipeline_source_mode: 'inline',
        pipeline_scm_repo: '',
        pipeline_scm_branch: '',
        pipeline_scm_path: '',
      }
      if (form.vcs_root_id !== '') data.vcs_root_id = Number(form.vcs_root_id)
      if (form.group_id !== '') data.group_id = Number(form.group_id)
      const project = await api.createProject(data)
      navigate(`/projects/${project.id}/configure`)
    } catch (reason: any) {
      const message = reason.message || t('projects.createFailed')
      setError(message)
      if (reason?.name !== 'ApiError') dialogs.notify(message)
    } finally { setSaving(false) }
  }

  return <motion.section className="project-create-page" initial={{ opacity: 0, y: 8 }} animate={{ opacity: 1, y: 0 }} transition={{ duration: 0.24, ease: 'easeOut' }}>
    <header className="detail-heading"><div><div className="detail-kicker">{t('projects.title')} <span>/</span> {t('common.create')}</div><div className="detail-title-row"><h1>{t('projects.newProject')}</h1></div><p className="detail-description">{t('projects.createDescription')}</p></div></header>
    <form onSubmit={handleSubmit} className="project-create-form">
      <section className="project-create-section"><header><div><FolderGit2 size={17} /><div><h2>{t('projectDetail.project')}</h2><p>{t('projects.identityHelp')}</p></div></div></header><div className="project-create-grid"><label><span>{t('projects.name')}</span><input required value={form.name} onChange={set('name')} placeholder="my-project" autoFocus /></label><label><span>{t('projects.defaultBranch')}</span><input value={form.default_branch} onChange={set('default_branch')} /></label><label className="wide"><span>{t('projectDetail.description')}</span><textarea value={form.description} onChange={set('description')} placeholder={t('projects.descriptionPlaceholder')} rows={2} /></label><label><span>{t('vcsRoots.title')}</span><select value={form.vcs_root_id} onChange={set('vcs_root_id')}><option value="">{t('projects.manualRepository')}</option>{vcsRoots?.map((root: any) => <option key={root.id} value={root.id}>{root.name}</option>)}</select></label><label><span>{t('projectGroups.title')}</span><select value={form.group_id} onChange={set('group_id')}><option value="">{t('projectGroups.ungrouped')}</option>{groups.map(group => <option key={group.id} value={group.id}>{group.name}</option>)}</select></label><label><span>{t('projects.repositoryType')}</span><input value={t('projects.gitOnly')} readOnly aria-readonly="true" /></label><label className="wide"><span>{t('projectDetail.repositoryUrl')}</span><input value={form.repo_url} onChange={set('repo_url')} placeholder="https://github.com/organization/repository.git" /></label></div></section>

      <section className="project-create-section"><header><div><Tag size={17} /><div><h2>{t('projects.tags')}</h2><p>{t('projects.tagsHelp')}</p></div></div></header><div className="tag-composer"><div className="known-tag-list">{knownTags.length ? knownTags.map(tag => <button type="button" key={tag} className={form.tags.some(item => item.toLowerCase() === tag.toLowerCase()) ? 'selected' : ''} onClick={() => toggleTag(tag)} aria-pressed={form.tags.some(item => item.toLowerCase() === tag.toLowerCase())}>{form.tags.some(item => item.toLowerCase() === tag.toLowerCase()) && <Check size={13} />}{tag}</button>) : <span>{t('projects.noExistingTags')}</span>}</div><div className="tag-input-row"><input value={tagDraft} onChange={event => setTagDraft(event.target.value)} onKeyDown={event => { if (event.key === 'Enter' || event.key === ',') { event.preventDefault(); addTag() } }} placeholder={t('projects.addTag')} aria-label={t('projects.addTag')} /><button type="button" className="secondary-command" onClick={() => addTag()}><Plus size={15} />{t('common.create')}</button></div>{form.tags.length > 0 && <div className="selected-tag-list">{form.tags.map(tag => <button type="button" key={tag} onClick={() => toggleTag(tag)}>{tag}<X size={13} /></button>)}</div>}</div></section>

      <section className="project-create-section"><header><div><FileCode2 size={17} /><div><h2>{t('projectDetail.pipelineConfig')}</h2><p>{t('projects.buildFlowHelp')}</p></div></div></header>{format === 'yaml' && <ApprovalSettingsEditor source={form.config} onChange={config => { setPipelineValidation(pendingPipelineValidation(config)); setForm(previous => ({ ...previous, config })) }} onError={setError} />}<div className="definition-toolbar"><span>{t('projects.sourceFormat')}</span><div className="definition-toolbar-actions"><div role="group" aria-label={t('projects.sourceFormat')}><button type="button" className={format === 'yaml' ? 'selected' : ''} onClick={() => switchFormat('yaml')}><FileCode2 size={14} />YAML</button><button type="button" className={format === 'ts' ? 'selected' : ''} onClick={() => switchFormat('ts')}><Code2 size={14} />TypeScript</button></div><button type="button" className="format-command" onClick={() => setShowJenkinsImport(true)}><FileInput size={13} />{t('jenkinsImport.title')}</button><button type="button" className="format-command" onClick={handleFormatConfig} disabled={formatting}><Braces size={13} />{formatting ? t('config.formatting') : format === 'ts' ? t('config.validate') : t('config.format')}</button></div></div><PipelineSourceEditor ariaLabel={t('pipeline.sourceLabel')} value={form.config} onChange={config => { setPipelineValidation(pendingPipelineValidation(config)); setForm(previous => ({ ...previous, config })) }} onValidationChange={setPipelineValidation} statusId="create-project-pipeline-status" height={430} /></section>
      {error && <p className="form-error">{error}</p>}
      <footer className="project-create-actions"><button type="button" className="secondary-command" onClick={() => navigate('/projects')}>{t('common.cancel')}</button><button type="submit" className="primary-command" disabled={saving || !pipelineReady} aria-describedby={!pipelineReady ? 'create-project-pipeline-status' : undefined} title={!pipelineReady ? pipelineBlockedMessage : undefined}>{saving ? t('common.loading') : t('projects.newProject')}</button></footer>
    </form>
    {showJenkinsImport && <JenkinsfileImportDialog projectName={form.name} onApply={applyJenkinsMigration} onClose={() => setShowJenkinsImport(false)} />}
  </motion.section>
}
