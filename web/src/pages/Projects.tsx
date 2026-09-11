import { useDeferredValue, useState } from 'react'
import { Check, CirclePlay, FolderGit2, FolderTree, GitBranch, History, LayoutDashboard, LoaderCircle, Pin, Plus, Search, Settings2, SlidersHorizontal, Star, Tag, Trash2, X } from 'lucide-react'
import { Link, useNavigate } from 'react-router-dom'
import { useI18n } from '../i18n'
import { api } from '../api'
import { useApi } from '../hooks'
import { dialogs } from '../components/AppDialogs'
import { canEdit } from '../authz'
import { PageState } from '../components/PageState'
import JenkinsPageShell from '../components/JenkinsPageShell'
import { formatDateTimeWithWeekday } from '../lib/dateTime'
import { useJenkinsBuildFlow } from './projectBuildFlow'

type ProjectTableProps = {
  caption: string
  projects: any[]
  editable: boolean
  building: number | null
  emptyLabel: string
  isParameterized: (project: any) => boolean
  onBuild: (project: any) => void
  onCustomBuild: (project: any) => void
  onDelete: (project: any) => void
  onToggleTag: (tag: string) => void
  onToggleFavorite: (project: any) => void
  onToggleQuickAccess: (project: any) => void
  flagBusy: { id: number; flag: 'favorite' | 'quick_access' } | null
  deleting: number | null
}

function ProjectTable({ caption, projects, editable, building, emptyLabel, isParameterized, onBuild, onCustomBuild, onDelete, onToggleTag, onToggleFavorite, onToggleQuickAccess, flagBusy, deleting }: ProjectTableProps) {
  const { t } = useI18n()
  return <div className="operations-table-wrap project-list-table">
    <table className="operations-table projects-table">
      <caption className="sr-only">{caption}</caption>
      <thead><tr><th>{t('projects.name')}</th><th>{t('projects.tags')}</th><th>{t('projects.repository')}</th><th>{t('projects.defaultBranch')}</th><th>{t('projects.created')}</th><th className="actions-heading">{t('projects.actions')}</th></tr></thead>
      <tbody>
        {!projects.length && <tr><td colSpan={6} className="operations-empty"><FolderGit2 size={18} />{emptyLabel}</td></tr>}
        {projects.map(project => <tr key={project.id}>
            <td><Link className="entity-link" to={`/projects/${project.id}`}><FolderGit2 size={16} /><span><strong>{project.name}</strong><small>{project.description || 'Git'}</small></span></Link></td>
            <td><div className="project-row-tags">{(project.tags || []).length ? project.tags.map((tag: string) => <button type="button" key={tag} className="project-tag" onClick={() => onToggleTag(tag)}>{tag}</button>) : <span className="muted-cell">-</span>}</div></td>
            <td><span className="repo-cell" title={project.repo_url || '-'}>{project.repo_url || '-'}</span></td>
            <td><span className="branch-cell"><GitBranch size={13} />{project.default_branch || '-'}</span></td>
            <td className="muted-cell">{formatDateTimeWithWeekday(project.created_at)}</td>
            <td><div className="row-actions">{editable && <><button type="button" className={`row-icon favorite${project.favorite ? ' active' : ''}`} title={project.favorite ? t('projects.unfavorite') : t('projects.favorite')} aria-label={`${project.favorite ? t('projects.unfavorite') : t('projects.favorite')} ${project.name}`} aria-pressed={project.favorite} aria-busy={flagBusy?.id === project.id && flagBusy?.flag === 'favorite'} disabled={flagBusy !== null} onClick={() => onToggleFavorite(project)}>{flagBusy?.id === project.id && flagBusy?.flag === 'favorite' ? <LoaderCircle className="timeline-spinner" size={16} /> : <Star size={16} />}</button><button type="button" className={`row-icon quick-access${project.quick_access ? ' active' : ''}`} title={project.quick_access ? t('projects.quickAccessRemove') : t('projects.quickAccessAdd')} aria-label={`${project.quick_access ? t('projects.quickAccessRemove') : t('projects.quickAccessAdd')} ${project.name}`} aria-pressed={project.quick_access} aria-busy={flagBusy?.id === project.id && flagBusy?.flag === 'quick_access'} disabled={flagBusy !== null} onClick={() => onToggleQuickAccess(project)}>{flagBusy?.id === project.id && flagBusy?.flag === 'quick_access' ? <LoaderCircle className="timeline-spinner" size={16} /> : <Pin size={16} />}</button></>}<Link className="row-settings" title={t('projects.openSettings')} to={`/projects/${project.id}/configure`}><Settings2 size={16} />{t('projects.settings')}</Link>{editable && <><button type="button" className="row-icon custom-build" title={project.enabled === false ? t('common.disabled') : t('builds.customBuild')} aria-label={`${t('builds.customBuild')} ${project.name}`} disabled={building !== null || deleting !== null || project.enabled === false} onClick={() => onCustomBuild(project)}><SlidersHorizontal size={16} /></button><button type="button" className="row-run" title={project.enabled === false ? t('common.disabled') : undefined} disabled={building !== null || deleting !== null || project.enabled === false} aria-busy={building === project.id} onClick={() => onBuild(project)}>{building === project.id ? <LoaderCircle className="timeline-spinner" size={16} /> : <CirclePlay size={16} />}{building === project.id ? t('projects.startingBuild') : t(isParameterized(project) ? 'builds.buildWithParameters' : 'projects.build')}</button><button type="button" className="row-icon danger" disabled={deleting !== null} aria-busy={deleting === project.id} title={t('projects.delete')} aria-label={`${t('projects.delete')} ${project.name}`} onClick={() => onDelete(project)}>{deleting === project.id ? <LoaderCircle className="timeline-spinner" size={16} /> : <Trash2 size={16} />}</button></>}</div></td>
          </tr>)}
      </tbody>
    </table>
  </div>
}

export default function Projects() {
  const { t } = useI18n()
  const navigate = useNavigate()
  const { data: projects, loading, error, reload: reloadProjects } = useApi(() => api.listProjects())
  const [selectedTags, setSelectedTags] = useState<string[]>([])
  const [searchInput, setSearchInput] = useState('')
  const [building, setBuilding] = useState<number | null>(null)
  const [flagBusy, setFlagBusy] = useState<{ id: number; flag: 'favorite' | 'quick_access' } | null>(null)
  const [deleting, setDeleting] = useState<number | null>(null)
  const editable = canEdit()
  const buildProjects = projects || []
  const { isParameterized, start } = useJenkinsBuildFlow(editable ? buildProjects : [], navigate)
  const deferredSearch = useDeferredValue(searchInput.trim().toLocaleLowerCase())

  const handleBuild = async (project: any) => {
    setBuilding(project.id)
    try {
      await start(project)
    } catch (e: any) {
      dialogs.notify(e.message || t('projects.buildFailed'))
    } finally {
      setBuilding(null)
    }
  }

  const handleCustomBuild = (project: any) => navigate(`/projects/${project.id}/build`)

  const handleDelete = async (project: any) => {
    if (!await dialogs.confirm(`${project.name}\n\n${t('projects.deleteConfirm')}`, { title: t('projects.removeTitle'), action: t('projects.delete') })) return
    setDeleting(project.id)
    try {
      await api.deleteProject(project.id)
      reloadProjects()
    } catch (e: any) {
      dialogs.notify(e.message || t('projects.removeFailed'))
    } finally {
      setDeleting(null)
    }
  }

  const handleToggleFavorite = async (project: any) => {
    setFlagBusy({ id: project.id, flag: 'favorite' })
    try {
      await api.setProjectFlags(project.id, !project.favorite, project.quick_access)
      reloadProjects()
    } catch (e: any) {
      dialogs.notify(e.message || t('projects.favoriteFailed') || '操作失败')
    } finally {
      setFlagBusy(null)
    }
  }

  const handleToggleQuickAccess = async (project: any) => {
    setFlagBusy({ id: project.id, flag: 'quick_access' })
    try {
      await api.setProjectFlags(project.id, project.favorite, !project.quick_access)
      reloadProjects()
    } catch (e: any) {
      dialogs.notify(e.message || '操作失败')
    } finally {
      setFlagBusy(null)
    }
  }

  if (loading) return <PageState />
  if (error) return <PageState error={error} onRetry={reloadProjects} />

  const list = projects || []
  const tags = [...new Map<string, string>(list.flatMap(project => (project.tags || []).map((tag: string): [string, string] => [tag.toLowerCase(), tag]))).values()].sort((a, b) => a.localeCompare(b))
  const searchTokens = deferredSearch.split(/\s+/).filter(Boolean)
  const filtered = list.filter(project => {
    const tagMatch = !selectedTags.length || selectedTags.every(tag => (project.tags || []).some((value: string) => value.toLowerCase() === tag.toLowerCase()))
    if (!tagMatch) return false
    if (!searchTokens.length) return true
    const searchable = [project.id, project.name, project.description, project.repo_type, project.repo_url, project.default_branch, ...(project.tags || [])].filter(value => value !== null && value !== undefined).join(' ').toLocaleLowerCase()
    return searchTokens.every(token => searchable.includes(token))
  })
  const toggleTag = (tag: string) => setSelectedTags(current => current.some(value => value.toLowerCase() === tag.toLowerCase()) ? current.filter(value => value.toLowerCase() !== tag.toLowerCase()) : [...current, tag])
  return (
    <div>
      <JenkinsPageShell
        className="jenkins-projects-page"
        breadcrumbs={[{ label: t('projects.title') }]}
        sidepanelLabel={t('shell.quickAccess')}
        sidepanel={<nav className="jenkins-context-task-list">
          {editable && <Link to="/projects/new"><Plus size={20} />{t('projects.newProject')}</Link>}
          <Link to="/"><LayoutDashboard size={20} />{t('nav.dashboard')}</Link>
          <Link to="/builds"><History size={20} />{t('nav.builds')}</Link>
          <Link to="/projects" className="active" aria-current="page"><FolderTree size={20} />{t('nav.projects')}</Link>
        </nav>}
      >
      <div className="operations-page jenkins-projects-content">
      <header className="jenkins-page-heading">
        <div className="operations-heading-main">
          <div className="projects-title-block"><p className="jenkins-page-heading-count">{filtered.length}{selectedTags.length || searchInput.trim() ? ` / ${list.length}` : ''}</p><h1>{t('projects.title')}</h1></div>
          <label className="project-list-search"><Search size={15} aria-hidden="true" /><span className="sr-only">{t('projects.searchPlaceholder')}</span><input value={searchInput} onChange={event => setSearchInput(event.target.value)} placeholder={t('projects.searchPlaceholder')} /><button type="button" aria-label={t('projects.clearSearch')} title={t('projects.clearSearch')} onClick={() => setSearchInput('')} disabled={!searchInput}><X size={14} /></button></label>
          {tags.length > 0 && <div className="project-tag-filter project-tag-filter-inline" aria-label={t('projects.filterTags')}><span><Tag size={14} />{t('projects.tags')}</span><button type="button" className={!selectedTags.length ? 'active' : ''} aria-pressed={!selectedTags.length} onClick={() => setSelectedTags([])}>{t('projects.allProjects')}</button>{tags.map(tag => <button type="button" key={tag} className={selectedTags.some(value => value.toLowerCase() === tag.toLowerCase()) ? 'active' : ''} aria-pressed={selectedTags.some(value => value.toLowerCase() === tag.toLowerCase())} onClick={() => toggleTag(tag)}>{selectedTags.some(value => value.toLowerCase() === tag.toLowerCase()) && <Check size={13} />}{tag}</button>)}{selectedTags.length > 0 && <button type="button" className="clear-tags" onClick={() => setSelectedTags([])} title={t('projects.clearTagFilters')} aria-label={t('projects.clearTagFilters')}><X size={13} /></button>}</div>}
        </div>
        {editable && <div className="jenkins-page-actions"><Link className="primary-command" to="/projects/new"><Plus size={16} />{t('projects.newProject')}</Link></div>}
      </header>
      <ProjectTable caption={t('projects.title')} projects={filtered} editable={editable} building={building} emptyLabel={searchTokens.length ? t('projects.noSearchMatches') : selectedTags.length ? t('projects.noTagMatches') : t('common.noData')} isParameterized={isParameterized} onBuild={handleBuild} onCustomBuild={handleCustomBuild} onDelete={handleDelete} onToggleTag={toggleTag} onToggleFavorite={handleToggleFavorite} onToggleQuickAccess={handleToggleQuickAccess} flagBusy={flagBusy} deleting={deleting} />
      </div>
      </JenkinsPageShell>
    </div>
  )
}
