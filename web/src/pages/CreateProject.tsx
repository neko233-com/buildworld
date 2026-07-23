import { useMemo, useRef, useState } from 'react'
import { BookTemplate, Folder, GitBranch } from 'lucide-react'
import { useNavigate, useSearchParams } from 'react-router-dom'
import { api } from '../api'
import { JenkinsHeaderBreadcrumb } from '../components/JenkinsPageShell'
import { PageState } from '../components/PageState'
import { useApi } from '../hooks'
import { useI18n } from '../i18n'
import { resolvePipelineSourceLanguage } from '../lib/configFormat'
import './CreateProject.jenkins.css'

type NewItemType = 'pipeline' | 'folder'

type BuildTemplate = {
  id: number
  name: string
  description?: string
  config: string
  vcs_root_id?: number | null
  repo_url?: string
  default_branch?: string
}

export default function CreateProject() {
  const { t } = useI18n()
  const navigate = useNavigate()
  const [searchParams] = useSearchParams()
  const nameInputRef = useRef<HTMLInputElement>(null)
  const [name, setName] = useState('')
  const [itemType, setItemType] = useState<NewItemType>('pipeline')
  const [nameDirty, setNameDirty] = useState(false)
  const [saving, setSaving] = useState(false)
  const [submitError, setSubmitError] = useState('')
  const templateParameter = (searchParams.get('template') || '').trim()
  const parsedTemplateID = Number(templateParameter)
  const requestedTemplateID = Number.isSafeInteger(parsedTemplateID) && parsedTemplateID > 0 ? parsedTemplateID : null
  const { data, loading, error, reload } = useApi(
    () => Promise.all([
      api.listProjects(),
      api.listProjectGroups(),
      requestedTemplateID ? api.listTemplates() : Promise.resolve([]),
    ]),
    [requestedTemplateID],
  )
  const projects = data?.[0] || []
  const groups = data?.[1] || []
  const templates = (data?.[2] || []) as BuildTemplate[]
  const selectedTemplate = requestedTemplateID
    ? templates.find(template => Number(template.id) === requestedTemplateID) || null
    : null
  const templateUnavailable = Boolean(templateParameter && (!requestedTemplateID || !selectedTemplate))

  const trimmedName = name.trim()
  const duplicate = useMemo(() => {
    if (!trimmedName) return false
    const items = itemType === 'pipeline' ? projects : groups
    return items.some((item: any) => String(item.name || '').trim().toLocaleLowerCase() === trimmedName.toLocaleLowerCase())
  }, [groups, itemType, projects, trimmedName])
  const nameError = nameDirty && !trimmedName
    ? t('projects.itemNameRequired')
    : duplicate
      ? t('projects.itemNameExists')
      : ''
  const ready = Boolean(trimmedName && itemType && !duplicate && !(itemType === 'pipeline' && templateUnavailable))

  const selectType = (nextType: NewItemType) => {
    setItemType(nextType)
    setSubmitError('')
  }

  const handleSubmit = async (event: React.FormEvent<HTMLFormElement>) => {
    event.preventDefault()
    setNameDirty(true)
    setSubmitError('')
    if (!ready) {
      if (!trimmedName || duplicate) nameInputRef.current?.focus()
      return
    }

    setSaving(true)
    try {
      if (itemType === 'folder') {
        await api.createProjectGroup({ name: trimmedName, description: '', color: 'neutral' })
        navigate('/projects')
        return
      }

      const template = selectedTemplate || undefined
      const templateConfig = template?.config || ''
      const config = templateConfig.trim() ? templateConfig : ''
      const useJenkinsSCM = !config
      const pipelineFormat = useJenkinsSCM ? 'jenkinsfile' : resolvePipelineSourceLanguage(config)
      if (!useJenkinsSCM) await api.validatePipeline(config)
      const repository = template?.repo_url || ''
      const branch = template?.default_branch || 'main'
      const project = await api.createProject({
        name: trimmedName,
        description: '',
        repo_url: repository,
        repo_type: 'git',
        default_branch: branch,
        config,
        tags: [],
        pipeline_format: pipelineFormat,
        pipeline_source_mode: useJenkinsSCM ? 'scm' : 'inline',
        pipeline_scm_repo: useJenkinsSCM ? repository : '',
        pipeline_scm_branch: useJenkinsSCM ? branch : '',
        pipeline_scm_path: useJenkinsSCM ? 'Jenkinsfile' : '',
        ...(template ? {
          template_id: template.id,
          vcs_root_id: template.vcs_root_id ?? null,
        } : {}),
      })
      navigate(`/projects/${project.id}/configure`)
    } catch (reason: any) {
      setSubmitError(reason.message || t(itemType === 'folder' ? 'projectGroups.saveFailed' : 'projects.createFailed'))
    } finally {
      setSaving(false)
    }
  }

  if (loading) return <PageState />
  if (error) return <PageState error={error} onRetry={reload} />

  return <>
    <JenkinsHeaderBreadcrumb ariaLabel={t('projects.newItemBreadcrumb')} breadcrumbs={[{ label: t('projects.newItem') }]} />
    <section className="jenkins-new-item-page">
      <div className="jenkins-new-item-panel">
        <header className="jenkins-new-item-app-bar">
          <h1>{t('projects.newItem')}</h1>
        </header>

        <form className="jenkins-new-item-form" onSubmit={handleSubmit} aria-busy={saving} noValidate>
          <div className={`jenkins-new-item-form-item${nameError ? ' has-error' : ''}`}>
            <label htmlFor="jenkins-new-item-name">{t('projects.enterItemName')}</label>
            <input
              ref={nameInputRef}
              id="jenkins-new-item-name"
              name="name"
              type="text"
              autoComplete="off"
              autoFocus
              disabled={saving}
              value={name}
              aria-invalid={Boolean(nameError)}
              aria-describedby={nameError ? 'jenkins-new-item-name-error' : undefined}
              onBlur={() => setNameDirty(true)}
              onChange={event => {
                setName(event.target.value)
                setNameDirty(true)
                setSubmitError('')
              }}
            />
            <div className="jenkins-new-item-validation" aria-live="polite">
              {nameError && <p id="jenkins-new-item-name-error" role="alert">{nameError}</p>}
            </div>
          </div>

          <fieldset className="jenkins-new-item-form-item">
            <legend>{t('projects.selectItemType')}</legend>
            <div className="jenkins-new-item-choice-list">
              <div className="jenkins-new-item-choice">
                <label>
                  <span className="jenkins-new-item-choice-icon" aria-hidden="true"><GitBranch /></span>
                  <input type="radio" name="mode" value="pipeline" checked={itemType === 'pipeline'} disabled={saving} onChange={() => selectType('pipeline')} />
                  <span className="jenkins-new-item-choice-label">{t('projects.pipelineItem')}</span>
                  <span className="jenkins-new-item-choice-description">{t('projects.pipelineItemDescription')}</span>
                </label>
              </div>
              <div className="jenkins-new-item-choice">
                <label>
                  <span className="jenkins-new-item-choice-icon" aria-hidden="true"><Folder /></span>
                  <input type="radio" name="mode" value="folder" checked={itemType === 'folder'} disabled={saving} onChange={() => selectType('folder')} />
                  <span className="jenkins-new-item-choice-label">{t('projects.folderItem')}</span>
                  <span className="jenkins-new-item-choice-description">{t('projects.folderItemDescription')}</span>
                </label>
              </div>
            </div>
          </fieldset>

          {itemType === 'pipeline' && selectedTemplate && <section className="jenkins-new-item-template" aria-label={t('templates.selected')}>
            <BookTemplate size={18} aria-hidden="true" />
            <span><strong>{t('templates.selected')}: {selectedTemplate.name}</strong><small>{selectedTemplate.description || t('templates.appliedOnCreate')}</small></span>
          </section>}
          {itemType === 'pipeline' && templateUnavailable && <p className="jenkins-new-item-submit-error" role="alert">{t('templates.notFound')}</p>}

          {submitError && <p className="jenkins-new-item-submit-error" role="alert">{submitError}</p>}
          <footer className="jenkins-new-item-actions">
            <button type="submit" disabled={saving || !ready}>
              {saving ? <><span className="jenkins-new-item-spinner" aria-hidden="true" />{t('projects.creatingItem')}</> : t('projects.ok')}
            </button>
          </footer>
        </form>
      </div>
    </section>
  </>
}
