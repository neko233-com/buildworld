import { motion } from 'motion/react'
import { useState } from 'react'
import { Check, ChevronRight, CirclePlay, Folder, FolderGit2, FolderOpen, FolderTree, GitBranch, History, LayoutDashboard, LoaderCircle, Pin, Plus, Settings2, SlidersHorizontal, Star, Tag, Trash2, X } from 'lucide-react'
import { Link, useNavigate } from 'react-router-dom'
import { useI18n } from '../i18n'
import { api } from '../api'
import { useApi } from '../hooks'
import { dialogs } from '../components/AppDialogs'
import { canEdit } from '../authz'
import { PageState } from '../components/PageState'
import ProjectGroupsDialog from '../components/ProjectGroupsDialog'
import JenkinsPageShell from '../components/JenkinsPageShell'
import { normalizeProjectGroupColor, sortProjectGroups, type ProjectGroup } from '../lib/projectGroups'
import { useJenkinsBuildFlow } from './projectBuildFlow'

const PROJECT_FOLDER_STATE_KEY = 'buildworld.projects.folders'
const PROJECT_FOLDER_STATE_VERSION = 2

type ProjectTableProps = {
  caption: string
  projects: any[]
  groupsByID: Map<number, ProjectGroup>
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

function readExpandedProjectFolders(): Set<string> {
  if (typeof window === 'undefined') return new Set()
  try {
    const stored = JSON.parse(window.localStorage.getItem(PROJECT_FOLDER_STATE_KEY) || 'null')
    if (stored?.version !== PROJECT_FOLDER_STATE_VERSION || !Array.isArray(stored.expanded)) return new Set()
    return new Set(stored.expanded.filter((value: unknown): value is string => typeof value === 'string'))
  } catch {
    return new Set()
  }
}

function persistExpandedProjectFolders(expanded: Set<string>) {
  try {
    window.localStorage.setItem(PROJECT_FOLDER_STATE_KEY, JSON.stringify({
      version: PROJECT_FOLDER_STATE_VERSION,
      expanded: [...expanded],
    }))
  } catch {
    // Storage can be disabled. The in-memory accordion remains fully usable.
  }
}

function ProjectTable({ caption, projects, groupsByID, editable, building, emptyLabel, isParameterized, onBuild, onCustomBuild, onDelete, onToggleTag, onToggleFavorite, onToggleQuickAccess, flagBusy, deleting }: ProjectTableProps) {
  const { t } = useI18n()
  return <div className="operations-table-wrap project-folder-table">
    <table className="operations-table projects-table">
      <caption className="sr-only">{caption}</caption>
      <thead><tr><th>{t('projects.name')}</th><th>{t('projectGroups.title')}</th><th>{t('projects.tags')}</th><th>{t('projects.repository')}</th><th>{t('projects.defaultBranch')}</th><th>{t('projects.created')}</th><th className="actions-heading">{t('projects.actions')}</th></tr></thead>
      <tbody>
        {!projects.length && <tr><td colSpan={7} className="operations-empty"><FolderGit2 size={18} />{emptyLabel}</td></tr>}
        {projects.map(project => {
          const group = groupsByID.get(project.group_id)
          return <tr key={project.id}>
            <td><Link className="entity-link" to={`/projects/${project.id}`}><FolderGit2 size={16} /><span><strong>{project.name}</strong><small>{project.description || 'Git'}</small></span></Link></td>
            <td>{group ? <span className="project-group-chip" data-group-color={normalizeProjectGroupColor(group.color)} title={group.name}><FolderTree size={13} />{group.name}</span> : <span className="muted-cell">{t('projectGroups.ungrouped')}</span>}</td>
            <td><div className="project-row-tags">{(project.tags || []).length ? project.tags.map((tag: string) => <button type="button" key={tag} className="project-tag" onClick={() => onToggleTag(tag)}>{tag}</button>) : <span className="muted-cell">-</span>}</div></td>
            <td><span className="repo-cell" title={project.repo_url || '-'}>{project.repo_url || '-'}</span></td>
            <td><span className="branch-cell"><GitBranch size={13} />{project.default_branch || '-'}</span></td>
            <td className="muted-cell">{project.created_at ? new Date(project.created_at).toLocaleString() : '-'}</td>
            <td><div className="row-actions">{editable && <><button type="button" className={`row-icon favorite${project.favorite ? ' active' : ''}`} title={project.favorite ? t('projects.unfavorite') : t('projects.favorite')} aria-label={`${project.favorite ? t('projects.unfavorite') : t('projects.favorite')} ${project.name}`} aria-pressed={project.favorite} aria-busy={flagBusy?.id === project.id && flagBusy?.flag === 'favorite'} disabled={flagBusy !== null} onClick={() => onToggleFavorite(project)}>{flagBusy?.id === project.id && flagBusy?.flag === 'favorite' ? <LoaderCircle className="timeline-spinner" size={16} /> : <Star size={16} />}</button><button type="button" className={`row-icon quick-access${project.quick_access ? ' active' : ''}`} title={project.quick_access ? t('projects.quickAccessRemove') : t('projects.quickAccessAdd')} aria-label={`${project.quick_access ? t('projects.quickAccessRemove') : t('projects.quickAccessAdd')} ${project.name}`} aria-pressed={project.quick_access} aria-busy={flagBusy?.id === project.id && flagBusy?.flag === 'quick_access'} disabled={flagBusy !== null} onClick={() => onToggleQuickAccess(project)}>{flagBusy?.id === project.id && flagBusy?.flag === 'quick_access' ? <LoaderCircle className="timeline-spinner" size={16} /> : <Pin size={16} />}</button></>}<Link className="row-settings" title={t('projects.openSettings')} to={`/projects/${project.id}/configure`}><Settings2 size={16} />{t('projects.settings')}</Link>{editable && <><button type="button" className="row-icon custom-build" title={project.enabled === false ? t('common.disabled') : t('builds.customBuild')} aria-label={`${t('builds.customBuild')} ${project.name}`} disabled={building !== null || deleting !== null || project.enabled === false} onClick={() => onCustomBuild(project)}><SlidersHorizontal size={16} /></button><button type="button" className="row-run" title={project.enabled === false ? t('common.disabled') : undefined} disabled={building !== null || deleting !== null || project.enabled === false} aria-busy={building === project.id} onClick={() => onBuild(project)}>{building === project.id ? <LoaderCircle className="timeline-spinner" size={16} /> : <CirclePlay size={16} />}{building === project.id ? t('projects.startingBuild') : t(isParameterized(project) ? 'builds.buildWithParameters' : 'projects.build')}</button><button type="button" className="row-icon danger" disabled={deleting !== null} aria-busy={deleting === project.id} title={t('projects.delete')} aria-label={`${t('projects.delete')} ${project.name}`} onClick={() => onDelete(project)}>{deleting === project.id ? <LoaderCircle className="timeline-spinner" size={16} /> : <Trash2 size={16} />}</button></>}</div></td>
          </tr>
        })}
      </tbody>
    </table>
  </div>
}

export default function Projects() {
  const { t } = useI18n()
  const navigate = useNavigate()
  const { data: projects, loading, error, reload: reloadProjects } = useApi(() => api.listProjects())
  const { data: projectGroups, loading: groupsLoading, error: groupsError, reload: reloadGroups } = useApi(() => api.listProjectGroups())
  const [selectedTags, setSelectedTags] = useState<string[]>([])
  const [expandedFolders, setExpandedFolders] = useState(readExpandedProjectFolders)
  const [showGroups, setShowGroups] = useState(false)
  const [building, setBuilding] = useState<number | null>(null)
  const [flagBusy, setFlagBusy] = useState<{ id: number; flag: 'favorite' | 'quick_access' } | null>(null)
  const [deleting, setDeleting] = useState<number | null>(null)
  const editable = canEdit()
  const buildProjects = projects || []
  const { isParameterized, start } = useJenkinsBuildFlow(editable ? buildProjects : [], navigate)

  const toggleFolder = (key: string) => {
    setExpandedFolders(current => {
      const next = new Set(current)
      if (next.has(key)) next.delete(key)
      else next.add(key)
      persistExpandedProjectFolders(next)
      return next
    })
  }

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

  if (loading || groupsLoading) return <PageState />
  if (error || groupsError) return <PageState error={error || groupsError} onRetry={() => { reloadProjects(); reloadGroups() }} />

  const list = projects || []
  const groups = projectGroups || []
  const orderedGroups = sortProjectGroups(groups)
  const groupsByID = new Map(orderedGroups.map(group => [group.id, group]))
  const tags = [...new Map<string, string>(list.flatMap(project => (project.tags || []).map((tag: string): [string, string] => [tag.toLowerCase(), tag]))).values()].sort((a, b) => a.localeCompare(b))
  const filtered = list.filter(project => {
    const tagMatch = !selectedTags.length || selectedTags.every(tag => (project.tags || []).some((value: string) => value.toLowerCase() === tag.toLowerCase()))
    return tagMatch
  })
  const projectsByGroup = new Map<number, any[]>(orderedGroups.map(group => [group.id, []]))
  const ungroupedProjects: any[] = []
  for (const project of filtered) {
    const bucket = projectsByGroup.get(project.group_id)
    if (bucket) bucket.push(project)
    else ungroupedProjects.push(project)
  }
  const toggleTag = (tag: string) => setSelectedTags(current => current.some(value => value.toLowerCase() === tag.toLowerCase()) ? current.filter(value => value.toLowerCase() !== tag.toLowerCase()) : [...current, tag])
  return (
    <motion.div initial={{ opacity: 0 }} animate={{ opacity: 1 }} transition={{ duration: 0.18 }}>
      <JenkinsPageShell
        className="jenkins-projects-page"
        breadcrumbs={[{ label: t('projects.title') }]}
        sidepanelLabel={t('shell.quickAccess')}
        sidepanel={<nav className="jenkins-context-task-list">
          {editable && <Link to="/projects/new"><Plus size={20} />{t('projects.newProject')}</Link>}
          <Link to="/"><LayoutDashboard size={20} />{t('nav.dashboard')}</Link>
          <Link to="/builds"><History size={20} />{t('nav.builds')}</Link>
          <Link to="/projects" className="active" aria-current="page"><FolderTree size={20} />{t('nav.projects')}</Link>
          {editable && <button type="button" onClick={() => setShowGroups(true)}><Settings2 size={20} />{t('projectGroups.manage')}</button>}
        </nav>}
      >
      <div className="operations-page jenkins-projects-content">
      <header className="jenkins-page-heading">
        <div className="operations-heading-main">
          <div className="projects-title-block"><p className="jenkins-page-heading-count">{filtered.length}{selectedTags.length ? ` / ${list.length}` : ''}</p><h1>{t('projects.title')}</h1></div>
          {tags.length > 0 && <div className="project-tag-filter project-tag-filter-inline" aria-label={t('projects.filterTags')}><span><Tag size={14} />{t('projects.tags')}</span><button type="button" className={!selectedTags.length ? 'active' : ''} aria-pressed={!selectedTags.length} onClick={() => setSelectedTags([])}>{t('projects.allProjects')}</button>{tags.map(tag => <button type="button" key={tag} className={selectedTags.some(value => value.toLowerCase() === tag.toLowerCase()) ? 'active' : ''} aria-pressed={selectedTags.some(value => value.toLowerCase() === tag.toLowerCase())} onClick={() => toggleTag(tag)}>{selectedTags.some(value => value.toLowerCase() === tag.toLowerCase()) && <Check size={13} />}{tag}</button>)}{selectedTags.length > 0 && <button type="button" className="clear-tags" onClick={() => setSelectedTags([])} title={t('projects.clearTagFilters')} aria-label={t('projects.clearTagFilters')}><X size={13} /></button>}</div>}
        </div>
        {editable && <div className="jenkins-page-actions"><button type="button" className="secondary-command" onClick={() => setShowGroups(true)}><FolderTree size={16} />{t('projectGroups.manage')}</button><Link className="primary-command" to="/projects/new"><Plus size={16} />{t('projects.newProject')}</Link></div>}
      </header>
      <div className="project-directory-list">
        {orderedGroups.map(group => {
          const key = `group:${group.id}`
          const expanded = expandedFolders.has(key)
          const folderProjects = projectsByGroup.get(group.id) || []
          const contentID = `project-folder-${group.id}`
          const label = group.name
          return <section className="project-folder" data-group-color={normalizeProjectGroupColor(group.color)} key={group.id}>
            <header className="project-folder-heading">
              <button type="button" aria-expanded={expanded} aria-controls={contentID} onClick={() => toggleFolder(key)}>
                <ChevronRight className="project-folder-chevron" size={16} aria-hidden="true" />
                <span className="project-folder-icon" aria-hidden="true">{expanded ? <FolderOpen size={17} /> : <Folder size={17} />}</span>
                <span className="project-folder-title"><strong>{label}</strong><small>{group.description || t('projectGroups.folderHelp')}</small></span>
                <span className="project-folder-count">{t('projectGroups.projectCount').replace('{count}', String(folderProjects.length))}</span>
              </button>
            </header>
            <div id={contentID} className="project-folder-content" hidden={!expanded}>
              {expanded ? <ProjectTable caption={label} projects={folderProjects} groupsByID={groupsByID} editable={editable} building={building} emptyLabel={selectedTags.length ? t('projects.noTagMatches') : t('projectGroups.emptyFolder')} isParameterized={isParameterized} onBuild={handleBuild} onCustomBuild={handleCustomBuild} onDelete={handleDelete} onToggleTag={toggleTag} onToggleFavorite={handleToggleFavorite} onToggleQuickAccess={handleToggleQuickAccess} flagBusy={flagBusy} deleting={deleting} /> : null}
            </div>
          </section>
        })}
        {(() => {
          const key = 'ungrouped'
          const expanded = expandedFolders.has(key)
          const contentID = 'project-folder-ungrouped'
          return <section className="project-folder" data-group-color="neutral">
            <header className="project-folder-heading">
              <button type="button" aria-expanded={expanded} aria-controls={contentID} onClick={() => toggleFolder(key)}>
                <ChevronRight className="project-folder-chevron" size={16} aria-hidden="true" />
                <span className="project-folder-icon" aria-hidden="true">{expanded ? <FolderOpen size={17} /> : <Folder size={17} />}</span>
                <span className="project-folder-title"><strong>{t('projectGroups.ungrouped')}</strong><small>{t('projectGroups.ungroupedHelp')}</small></span>
                <span className="project-folder-count">{t('projectGroups.projectCount').replace('{count}', String(ungroupedProjects.length))}</span>
              </button>
            </header>
            <div id={contentID} className="project-folder-content" hidden={!expanded}>
              {expanded ? <ProjectTable caption={t('projectGroups.ungrouped')} projects={ungroupedProjects} groupsByID={groupsByID} editable={editable} building={building} emptyLabel={selectedTags.length ? t('projects.noTagMatches') : t('projectGroups.emptyFolder')} isParameterized={isParameterized} onBuild={handleBuild} onCustomBuild={handleCustomBuild} onDelete={handleDelete} onToggleTag={toggleTag} onToggleFavorite={handleToggleFavorite} onToggleQuickAccess={handleToggleQuickAccess} flagBusy={flagBusy} deleting={deleting} /> : null}
            </div>
          </section>
        })()}
      </div>
      </div>
      </JenkinsPageShell>
      {showGroups && <ProjectGroupsDialog groups={groups} projects={list} onReload={() => { reloadGroups(); reloadProjects() }} onClose={() => setShowGroups(false)} />}
    </motion.div>
  )
}
