import { useEffect, useMemo, useState } from 'react'
import { motion } from 'motion/react'
import { useNavigate, useSearchParams } from 'react-router-dom'
import { Braces, Check, Code2, FileCode2, FileInput, FolderGit2, Plus, Tag, X } from 'lucide-react'
import { useI18n } from '../i18n'
import { api, type PipelineMigrationResult } from '../api'
import { useApi } from '../hooks'
import { isTypeScriptPipelineSource, prettyConfigSource, prettyConfigSourceSync } from '../lib/configFormat'
import { flattenProjectGroups } from '../lib/projectGroups'
import ApprovalSettingsEditor from '../components/ApprovalSettingsEditor'
import JenkinsfileImportDialog from '../components/JenkinsfileImportDialog'
import { dialogs } from '../components/AppDialogs'

const SAMPLE_JSON = `{
  "stages": [
    {
      "name": "Build",
      "steps": [
        {
          "name": "compile",
          "type": "shell",
          "command": "echo building"
        }
      ]
    }
  ]
}
`

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
    // running and streams output just like Jenkins tail -f.
    // stage('Observe', watchService('Service log', { targetDir: '/srv/app', pidFile: 'server.pid', logFile: 'server.log', port: 8700 })),
  ],
})
`

export default function CreateProject() {
  const { t } = useI18n()
  const navigate = useNavigate()
  const [searchParams] = useSearchParams()
  const [format, setFormat] = useState<'json' | 'yaml' | 'ts'>('yaml')
  const [form, setForm] = useState({ name: '', description: '', repo_url: '', repo_type: 'git', default_branch: 'main', config: SAMPLE_YAML, vcs_root_id: '' as number | '', template_id: '' as number | '', group_id: '' as number | '', tags: [] as string[] })
  const [tagDraft, setTagDraft] = useState('')
  const [saving, setSaving] = useState(false)
  const [formatting, setFormatting] = useState(false)
  const [showJenkinsImport, setShowJenkinsImport] = useState(false)
  const [error, setError] = useState('')
  const { data: vcsRoots } = useApi(() => api.listVCSRoots())
  const { data: templates } = useApi(() => api.listTemplates())
  const { data: projects } = useApi(() => api.listProjects())
  const { data: projectGroups } = useApi(() => api.listProjectGroups())
  const knownTags = useMemo<string[]>(() => [...new Map<string, string>((projects || []).flatMap(project => (project.tags || []).map((tag: string): [string, string] => [tag.toLowerCase(), tag]))).values()].sort((a, b) => a.localeCompare(b)), [projects])
  const groups = useMemo(() => flattenProjectGroups(projectGroups || []), [projectGroups])

  useEffect(() => {
    const templateParam = searchParams.get('template')
    if (templateParam) setForm(previous => ({ ...previous, template_id: Number(templateParam) }))
  }, [searchParams])

  useEffect(() => {
    if (form.template_id === '' || form.template_id === undefined) return
    const template = templates?.find((item: any) => item.id === Number(form.template_id))
    if (!template?.config) return
    let config = typeof template.config === 'string' ? template.config : JSON.stringify(template.config, null, 2)
    try { config = prettyConfigSourceSync(config) } catch { /* Validate on format or submit. */ }
    setFormat(isTypeScriptPipelineSource(config) ? 'ts' : config.trim().startsWith('{') ? 'json' : 'yaml')
    setForm(previous => ({ ...previous, config }))
  }, [form.template_id, templates])

  useEffect(() => {
    if (form.vcs_root_id === '' || form.vcs_root_id === undefined) return
    const root = vcsRoots?.find((item: any) => item.id === Number(form.vcs_root_id))
    if (root) setForm(previous => ({ ...previous, repo_url: root.url || previous.repo_url, repo_type: root.type || previous.repo_type, default_branch: root.branch || previous.default_branch }))
  }, [form.vcs_root_id, vcsRoots])

  const set = (key: Exclude<keyof typeof form, 'tags'>) => (event: React.ChangeEvent<HTMLInputElement | HTMLTextAreaElement | HTMLSelectElement>) => {
    const value = event.target.value
    setForm(previous => ({ ...previous, [key]: key === 'vcs_root_id' || key === 'template_id' || key === 'group_id' ? value === '' ? '' : Number(value) : value }))
  }
  const addTag = (value = tagDraft) => {
    const tag = value.trim().replace(/,$/, '')
    if (tag && !form.tags.some(item => item.toLowerCase() === tag.toLowerCase())) setForm(previous => ({ ...previous, tags: [...previous.tags, tag] }))
    setTagDraft('')
  }
  const toggleTag = (tag: string) => setForm(previous => ({ ...previous, tags: previous.tags.some(item => item.toLowerCase() === tag.toLowerCase()) ? previous.tags.filter(item => item.toLowerCase() !== tag.toLowerCase()) : [...previous.tags, tag] }))
  const switchFormat = (nextFormat: 'json' | 'yaml' | 'ts') => { setFormat(nextFormat); setForm(previous => ({ ...previous, config: nextFormat === 'json' ? SAMPLE_JSON : nextFormat === 'ts' ? SAMPLE_TYPESCRIPT : SAMPLE_YAML, template_id: '' })) }
  const handleFormatConfig = async () => {
    setFormatting(true); setError('')
    try {
      const config = await prettyConfigSource(form.config)
      setForm(previous => ({ ...previous, config }))
      dialogs.notify(t('config.formatted'), 'success')
    } catch (reason: any) {
      const message = reason.message || t('config.invalid')
      setError(message)
      dialogs.notify(t('config.invalid'))
    } finally {
      setFormatting(false)
    }
  }
  const applyJenkinsMigration = (result: PipelineMigrationResult) => {
    setFormat('json')
    setError('')
    setForm(previous => ({
      ...previous,
      config: result.config,
      repo_url: previous.repo_url || result.hints.repository_url || '',
      default_branch: result.hints.default_branch || previous.default_branch,
      template_id: '',
    }))
  }
  const handleSubmit = async (event: React.FormEvent) => {
    event.preventDefault()
    setSaving(true); setError('')
    try {
      const config = await prettyConfigSource(form.config)
      const data: any = { name: form.name, description: form.description, repo_url: form.repo_url, repo_type: form.repo_type, default_branch: form.default_branch, config, tags: form.tags }
      if (form.vcs_root_id !== '') data.vcs_root_id = Number(form.vcs_root_id)
      if (form.template_id !== '') data.template_id = Number(form.template_id)
      if (form.group_id !== '') data.group_id = Number(form.group_id)
      const project = await api.createProject(data)
      navigate(`/projects/${project.id}?view=settings`)
    } catch (reason: any) {
      const message = reason.message || t('projects.createFailed')
      setError(message)
      if (reason?.name !== 'ApiError') dialogs.notify(message)
    } finally { setSaving(false) }
  }

  return <motion.section className="project-create-page" initial={{ opacity: 0, y: 8 }} animate={{ opacity: 1, y: 0 }} transition={{ duration: 0.24, ease: 'easeOut' }}>
    <header className="detail-heading"><div><div className="detail-kicker">{t('projects.title')} <span>/</span> {t('common.create')}</div><div className="detail-title-row"><h1>{t('projects.newProject')}</h1></div><p className="detail-description">{t('projects.createDescription')}</p></div></header>
    <form onSubmit={handleSubmit} className="project-create-form">
      <section className="project-create-section"><header><div><FolderGit2 size={17} /><div><h2>{t('projectDetail.project')}</h2><p>{t('projects.identityHelp')}</p></div></div></header><div className="project-create-grid"><label><span>{t('projects.name')}</span><input required value={form.name} onChange={set('name')} placeholder="my-project" autoFocus /></label><label><span>{t('projects.defaultBranch')}</span><input value={form.default_branch} onChange={set('default_branch')} /></label><label className="wide"><span>{t('projectDetail.description')}</span><textarea value={form.description} onChange={set('description')} placeholder={t('projects.descriptionPlaceholder')} rows={2} /></label><label><span>VCS Root</span><select value={form.vcs_root_id} onChange={set('vcs_root_id')}><option value="">{t('projects.manualRepository')}</option>{vcsRoots?.map((root: any) => <option key={root.id} value={root.id}>{root.name}</option>)}</select></label><label><span>{t('projectGroups.title')}</span><select value={form.group_id} onChange={set('group_id')}><option value="">{t('projectGroups.ungrouped')}</option>{groups.map(group => <option key={group.id} value={group.id}>{'\u00a0\u00a0'.repeat(group.depth)}{group.path}</option>)}</select></label><label><span>{t('projects.repositoryType')}</span><select value={form.repo_type} onChange={set('repo_type')}><option value="git">Git</option><option value="svn">SVN</option><option value="hg">Mercurial</option></select></label><label className="wide"><span>{t('projectDetail.repositoryUrl')}</span><input value={form.repo_url} onChange={set('repo_url')} placeholder="https://github.com/organization/repository" /></label></div></section>

      <section className="project-create-section"><header><div><Tag size={17} /><div><h2>{t('projects.tags')}</h2><p>{t('projects.tagsHelp')}</p></div></div></header><div className="tag-composer"><div className="known-tag-list">{knownTags.length ? knownTags.map(tag => <button type="button" key={tag} className={form.tags.some(item => item.toLowerCase() === tag.toLowerCase()) ? 'selected' : ''} onClick={() => toggleTag(tag)} aria-pressed={form.tags.some(item => item.toLowerCase() === tag.toLowerCase())}>{form.tags.some(item => item.toLowerCase() === tag.toLowerCase()) && <Check size={13} />}{tag}</button>) : <span>{t('projects.noExistingTags')}</span>}</div><div className="tag-input-row"><input value={tagDraft} onChange={event => setTagDraft(event.target.value)} onKeyDown={event => { if (event.key === 'Enter' || event.key === ',') { event.preventDefault(); addTag() } }} placeholder={t('projects.addTag')} aria-label={t('projects.addTag')} /><button type="button" className="secondary-command" onClick={() => addTag()}><Plus size={15} />{t('common.create')}</button></div>{form.tags.length > 0 && <div className="selected-tag-list">{form.tags.map(tag => <button type="button" key={tag} onClick={() => toggleTag(tag)}>{tag}<X size={13} /></button>)}</div>}</div></section>

      <section className="project-create-section"><header><div><FileCode2 size={17} /><div><h2>{t('projectDetail.pipelineConfig')}</h2><p>{t('projects.buildFlowHelp')}</p></div></div><label className="template-picker"><span>{t('projects.template')}</span><select value={form.template_id} onChange={set('template_id')}><option value="">{t('projects.noTemplate')}</option>{templates?.map((template: any) => <option key={template.id} value={template.id}>{template.name}</option>)}</select></label></header>{format !== 'ts' && <ApprovalSettingsEditor source={form.config} onChange={config => setForm(previous => ({ ...previous, config }))} onError={setError} />}<div className="definition-toolbar"><span>{t('projects.sourceFormat')}</span><div className="definition-toolbar-actions"><div role="group" aria-label={t('projects.sourceFormat')}><button type="button" className={format === 'yaml' ? 'selected' : ''} onClick={() => switchFormat('yaml')}><FileCode2 size={14} />YAML</button><button type="button" className={format === 'json' ? 'selected' : ''} onClick={() => switchFormat('json')}><Code2 size={14} />JSON</button><button type="button" className={format === 'ts' ? 'selected' : ''} onClick={() => switchFormat('ts')}><Code2 size={14} />TS</button></div><button type="button" className="format-command" onClick={() => setShowJenkinsImport(true)}><FileInput size={13} />{t('jenkinsImport.title')}</button><button type="button" className="format-command" onClick={handleFormatConfig} disabled={formatting}><Braces size={13} />{formatting ? t('config.formatting') : t('config.format')}</button></div></div><textarea className="pipeline-source-input" aria-label={t('pipeline.sourceLabel')} value={form.config} onChange={set('config')} spellCheck="false" rows={18} /></section>
      {error && <p className="form-error">{error}</p>}
      <footer className="project-create-actions"><button type="button" className="secondary-command" onClick={() => navigate('/projects')}>{t('common.cancel')}</button><button type="submit" className="primary-command" disabled={saving}>{saving ? t('common.loading') : t('projects.newProject')}</button></footer>
    </form>
    {showJenkinsImport && <JenkinsfileImportDialog projectName={form.name} onApply={applyJenkinsMigration} onClose={() => setShowJenkinsImport(false)} />}
  </motion.section>
}
