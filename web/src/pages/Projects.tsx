import { motion } from 'motion/react'
import { useState } from 'react'
import { Check, CirclePlay, FolderGit2, FolderTree, GitBranch, LoaderCircle, Plus, Settings2, SlidersHorizontal, Tag, Trash2, X } from 'lucide-react'
import { Link, useNavigate } from 'react-router-dom'
import { useI18n } from '../i18n'
import { api } from '../api'
import { useApi } from '../hooks'
import { dialogs } from '../components/AppDialogs'
import { canEdit } from '../authz'
import { PageState } from '../components/PageState'
import RunBuildDialog, { requiresBuildParameterInput } from '../components/RunBuildDialog'
import ProjectGroupsDialog from '../components/ProjectGroupsDialog'
import { descendantProjectGroupIDs, flattenProjectGroups, projectGroupPath } from '../lib/projectGroups'

export default function Projects() {
  const { t } = useI18n()
  const navigate = useNavigate()
  const { data: projects, loading, error, reload: reloadProjects } = useApi(() => api.listProjects())
  const { data: projectGroups, loading: groupsLoading, error: groupsError, reload: reloadGroups } = useApi(() => api.listProjectGroups())
  const [selectedTags, setSelectedTags] = useState<string[]>([])
  const [selectedGroup, setSelectedGroup] = useState<'all' | 'ungrouped' | number>('all')
  const [showGroups, setShowGroups] = useState(false)
  const [building, setBuilding] = useState<number | null>(null)
  const [loadingDefinition, setLoadingDefinition] = useState<number | null>(null)
  const [customProject, setCustomProject] = useState<any>(null)
  const editable = canEdit()

  const loadProjectDefinition = (project: any) =>
    typeof project.config === 'string' ? Promise.resolve(project) : api.getProject(project.id)

  const handleBuild = async (project: any) => {
    setBuilding(project.id)
    try {
      const definition = await loadProjectDefinition(project)
      if (requiresBuildParameterInput(definition.config)) {
        setCustomProject(definition)
        return
      }
      const build = await api.triggerBuild(project.id)
      navigate(`/builds/${build.id}`)
    } catch (e: any) {
      dialogs.notify(e.message || t('projects.buildFailed'))
    } finally {
      setBuilding(null)
    }
  }

  const handleCustomBuild = async (project: any) => {
    setLoadingDefinition(project.id)
    try {
      setCustomProject(await loadProjectDefinition(project))
    } catch (e: any) {
      dialogs.notify(e.message || t('common.loadFailed'))
    } finally {
      setLoadingDefinition(null)
    }
  }

  const handleDelete = async (id: number) => {
    if (!await dialogs.confirm(t('projects.deleteConfirm'), { title: t('projects.removeTitle'), action: t('projects.delete') })) return
    try {
      await api.deleteProject(id)
      reloadProjects()
    } catch (e: any) {
      dialogs.notify(e.message || t('projects.removeFailed'))
    }
  }

  if (loading || groupsLoading) return <PageState />
  if (error || groupsError) return <PageState error={error || groupsError} onRetry={() => { reloadProjects(); reloadGroups() }} />

  const list = projects || []
  const groups = projectGroups || []
  const flattenedGroups = flattenProjectGroups(groups)
  const tags = [...new Map<string, string>(list.flatMap(project => (project.tags || []).map((tag: string): [string, string] => [tag.toLowerCase(), tag]))).values()].sort((a, b) => a.localeCompare(b))
  const groupIDs = typeof selectedGroup === 'number' ? descendantProjectGroupIDs(groups, selectedGroup) : null
  const filtered = list.filter(project => {
    const tagMatch = !selectedTags.length || selectedTags.every(tag => (project.tags || []).some((value: string) => value.toLowerCase() === tag.toLowerCase()))
    const groupMatch = selectedGroup === 'all' || (selectedGroup === 'ungrouped' ? project.group_id == null : groupIDs?.has(project.group_id))
    return tagMatch && groupMatch
  })
  const toggleTag = (tag: string) => setSelectedTags(current => current.some(value => value.toLowerCase() === tag.toLowerCase()) ? current.filter(value => value.toLowerCase() !== tag.toLowerCase()) : [...current, tag])
  return (
    <motion.section className="operations-page" initial={{ opacity: 0, y: 8 }} animate={{ opacity: 1, y: 0 }} transition={{ duration: 0.24, ease: 'easeOut' }}>
      <header className="operations-heading">
        <div className="operations-heading-main">
          <div className="projects-title-block"><p>{filtered.length}{selectedTags.length || selectedGroup !== 'all' ? ` / ${list.length}` : ''}</p><h1>{t('projects.title')}</h1></div>
          {tags.length > 0 && <div className="project-tag-filter project-tag-filter-inline" aria-label={t('projects.filterTags')}><span><Tag size={14} />{t('projects.tags')}</span><button type="button" className={!selectedTags.length ? 'active' : ''} aria-pressed={!selectedTags.length} onClick={() => setSelectedTags([])}>{t('projects.allProjects')}</button>{tags.map(tag => <button type="button" key={tag} className={selectedTags.some(value => value.toLowerCase() === tag.toLowerCase()) ? 'active' : ''} aria-pressed={selectedTags.some(value => value.toLowerCase() === tag.toLowerCase())} onClick={() => toggleTag(tag)}>{selectedTags.some(value => value.toLowerCase() === tag.toLowerCase()) && <Check size={13} />}{tag}</button>)}{selectedTags.length > 0 && <button type="button" className="clear-tags" onClick={() => setSelectedTags([])} title={t('projects.clearTagFilters')} aria-label={t('projects.clearTagFilters')}><X size={13} /></button>}</div>}
        </div>
        {editable && <div className="operations-heading-actions"><button type="button" className="secondary-command" onClick={() => setShowGroups(true)}><FolderTree size={16} />{t('projectGroups.manage')}</button><Link className="primary-command" to="/projects/new"><Plus size={16} />{t('projects.newProject')}</Link></div>}
      </header>
      <div className="project-group-filter" aria-label={t('projectGroups.filter')}>
        <span><FolderTree size={14} />{t('projectGroups.title')}</span>
        <button type="button" className={selectedGroup === 'all' ? 'active' : ''} aria-pressed={selectedGroup === 'all'} onClick={() => setSelectedGroup('all')}>{t('projectGroups.all')}</button>
        <button type="button" className={selectedGroup === 'ungrouped' ? 'active' : ''} aria-pressed={selectedGroup === 'ungrouped'} onClick={() => setSelectedGroup('ungrouped')}>{t('projectGroups.ungrouped')}</button>
        {flattenedGroups.map(group => <button type="button" key={group.id} className={selectedGroup === group.id ? 'active' : ''} aria-pressed={selectedGroup === group.id} onClick={() => setSelectedGroup(group.id)} title={group.path}>{'\u00a0'.repeat(group.depth * 2)}{group.name}</button>)}
      </div>
      <section className="operations-table-wrap">
        <table className="operations-table projects-table">
          <thead><tr><th>{t('projects.name')}</th><th>{t('projectGroups.title')}</th><th>{t('projects.tags')}</th><th>{t('projects.repository')}</th><th>{t('projects.defaultBranch')}</th><th>{t('projects.created')}</th><th className="actions-heading">{t('projects.actions')}</th></tr></thead>
          <tbody>
            {!filtered.length && <tr><td colSpan={7} className="operations-empty"><FolderGit2 size={18} />{selectedTags.length || selectedGroup !== 'all' ? t('projects.noTagMatches') : t('common.noData')}</td></tr>}
            {filtered.map((project) => <tr key={project.id}>
              <td><Link className="entity-link" to={`/projects/${project.id}`}><FolderGit2 size={16} /><span><strong>{project.name}</strong><small>{project.description || project.repo_type || '-'}</small></span></Link></td>
              <td>{project.group_id != null ? <button type="button" className="project-group-chip" onClick={() => setSelectedGroup(project.group_id)} title={projectGroupPath(groups, project.group_id)}><FolderTree size={13} />{flattenedGroups.find(group => group.id === project.group_id)?.name || `#${project.group_id}`}</button> : <span className="muted-cell">{t('projectGroups.ungrouped')}</span>}</td>
              <td><div className="project-row-tags">{(project.tags || []).length ? project.tags.map((tag: string) => <button type="button" key={tag} className="project-tag" onClick={() => toggleTag(tag)}>{tag}</button>) : <span className="muted-cell">-</span>}</div></td>
              <td><span className="repo-cell" title={project.repo_url || '-'}>{project.repo_url || '-'}</span></td>
              <td><span className="branch-cell"><GitBranch size={13} />{project.default_branch || '-'}</span></td>
              <td className="muted-cell">{project.created_at ? new Date(project.created_at).toLocaleString() : '-'}</td>
              <td><div className="row-actions"><Link className="row-settings" title={t('projects.openSettings')} to={`/projects/${project.id}?view=settings`}><Settings2 size={15} />{t('projects.settings')}</Link>{editable && <><button type="button" className="row-icon custom-build" title={t('builds.customBuild')} aria-label={`${t('builds.customBuild')} ${project.name}`} disabled={building !== null || loadingDefinition !== null} aria-busy={loadingDefinition === project.id} onClick={() => handleCustomBuild(project)}>{loadingDefinition === project.id ? <LoaderCircle className="timeline-spinner" size={15} /> : <SlidersHorizontal size={15} />}</button><button type="button" className="row-run" disabled={building !== null || loadingDefinition !== null} aria-busy={building === project.id} onClick={() => handleBuild(project)}>{building === project.id ? <LoaderCircle className="timeline-spinner" size={15} /> : <CirclePlay size={15} />}{building === project.id ? t('projects.startingBuild') : t('projects.build')}</button><button type="button" className="row-icon danger" title={t('projects.delete')} aria-label={t('projects.delete')} onClick={() => handleDelete(project.id)}><Trash2 size={15} /></button></>}</div></td>
            </tr>)}
          </tbody>
        </table>
      </section>
      {customProject && <RunBuildDialog project={customProject} onClose={() => setCustomProject(null)} onQueued={build => { setCustomProject(null); navigate(`/builds/${build.id}`) }} />}
      {showGroups && <ProjectGroupsDialog groups={groups} projects={list} onReload={() => { reloadGroups(); reloadProjects() }} onClose={() => setShowGroups(false)} />}
    </motion.section>
  )
}
