import { useEffect, useState } from 'react'
import { AlertTriangle, ArrowDown, ArrowUp, Folder, GripVertical, HardDrive, LoaderCircle, MoreHorizontal, Play, Plus, RefreshCw, Star, Trash2, X } from 'lucide-react'
import { IoLogoGithub } from 'react-icons/io5'
import { Link, useNavigate } from 'react-router-dom'
import { api } from '../api'
import { canEdit } from '../authz'
import { dialogs } from '../components/AppDialogs'
import JenkinsHomeRail from '../components/JenkinsHomeRail'
import { ModalDialog } from '../components/ModalDialog'
import { PageState } from '../components/PageState'
import ProjectGroupsDialog from '../components/ProjectGroupsDialog'
import { useApi } from '../hooks'
import { useI18n } from '../i18n'
import { buildStatusLabel, buildStatusTone } from '../lib/buildPresentation'
import { formatDateTimeWithWeekday } from '../lib/dateTime'
import { formatDuration } from '../lib/durationPresentation'
import { sortProjectGroups, type ProjectGroup } from '../lib/projectGroups'
import { useJenkinsBuildFlow } from './projectBuildFlow'

const ACTIVE_VIEW_KEY = 'buildworld.jenkins.active-view'
const ACTIVE_BUILD_STATUSES = new Set(['running', 'pending', 'pending_approval', 'queued'])
const ACTIVE_REFRESH_INTERVAL_MS = 2_000
const IDLE_REFRESH_INTERVAL_MS = 15_000
const STORAGE_REFRESH_INTERVAL_MS = 30_000
const PROJECT_URL = 'https://github.com/neko233-com/buildworld'
type GroupContextMenu = {
  group: ProjectGroup
  x: number
  y: number
}

function initialActiveView() {
  if (typeof localStorage === 'undefined') return 'all'
  return localStorage.getItem(ACTIVE_VIEW_KEY) || 'all'
}

function BuildReference({ build, emptyLabel }: { build?: any; emptyLabel: string }) {
  if (!build) return <span className="jenkins-empty-value">{emptyLabel}</span>
  return <span className="jenkins-build-reference">
    <Link to={`/builds/${build.id}`}>#{build.number}</Link>
    <small>{formatDateTimeWithWeekday(build.started_at)}</small>
  </span>
}

function LatestBuildStatus({ build, emptyLabel }: { build?: any; emptyLabel: string }) {
  const { t } = useI18n()
  if (!build) return <span className="jenkins-latest-build-empty jenkins-empty-value">{emptyLabel}</span>
  const label = buildStatusLabel(t, build.status)
  const tone = buildStatusTone(build.status)
  return <span className={`jenkins-latest-build ${tone}`} aria-label={`${t('dashboard.latestBuildStatus')}: ${label}`}>
    <span className={`jenkins-status-orb ${tone}`} role="img" aria-label={label} title={label} />
    <span className="jenkins-latest-build-info">
      <Link to={`/builds/${build.id}`}>#{build.number}</Link>
      <small>{label}</small>
    </span>
  </span>
}

function formatBytes(value: number, locale: string) {
  if (!Number.isFinite(value) || value <= 0) return '0 B'
  const units = ['B', 'KiB', 'MiB', 'GiB', 'TiB']
  const unitIndex = Math.min(Math.floor(Math.log(value) / Math.log(1024)), units.length - 1)
  const amount = value / 1024 ** unitIndex
  return `${new Intl.NumberFormat(locale, { maximumFractionDigits: amount >= 100 ? 0 : 1 }).format(amount)} ${units[unitIndex]}`
}

function StorageMonitor() {
  const { t, locale } = useI18n()
  const { data, loading, error, reload } = useApi(() => api.getStorageUsage(), [])

  useEffect(() => {
    const timer = window.setInterval(() => {
      if (document.visibilityState === 'visible') reload()
    }, STORAGE_REFRESH_INTERVAL_MS)
    return () => window.clearInterval(timer)
  }, [reload])

  if (loading) return <section className="jenkins-storage-monitor loading" aria-label={t('dashboard.diskUsage')} aria-busy="true">
    <HardDrive size={17} aria-hidden="true" />
    <span>{t('dashboard.diskLoading')}</span>
  </section>
  if (error || !data) return <section className="jenkins-storage-monitor error" role="alert">
    <AlertTriangle size={17} aria-hidden="true" />
    <span>{t('dashboard.diskUnavailable')}</span>
    <button type="button" onClick={reload}><RefreshCw size={13} aria-hidden="true" />{t('common.retry')}</button>
  </section>

  const usedPercent = Math.min(100, Math.max(0, data.used_percent))
  const tone = usedPercent >= 90 ? 'critical' : usedPercent >= 75 ? 'warning' : 'normal'
  const roundedPercent = Math.round(usedPercent)
  return <section className={`jenkins-storage-monitor ${tone}`} aria-label={t('dashboard.diskUsage')}>
    <div className="jenkins-storage-summary">
      <span><HardDrive size={17} aria-hidden="true" /><strong>{t('dashboard.diskUsage')}</strong><small>{data.executor} · {data.volume}</small></span>
      <b>{roundedPercent}%</b>
    </div>
    <progress max={100} value={usedPercent} aria-label={`${t('dashboard.diskUsed')} ${roundedPercent}%`} />
    <div className="jenkins-storage-detail">
      <span>{t('dashboard.diskUsed')} <strong>{formatBytes(data.used_bytes, locale)}</strong></span>
      <span>{t('dashboard.diskFree')} <strong>{formatBytes(data.free_bytes, locale)}</strong></span>
      <span>{t('dashboard.diskTotal')} <strong>{formatBytes(data.total_bytes, locale)}</strong></span>
    </div>
  </section>
}

function ProjectFooter() {
  const { t } = useI18n()
  const [today, setToday] = useState(() => new Date())

  useEffect(() => {
    const timer = window.setInterval(() => setToday(new Date()), 60_000)
    return () => window.clearInterval(timer)
  }, [])

  return <footer className="jenkins-project-footer">
    <time dateTime={today.toISOString()}>{formatDateTimeWithWeekday(today)}</time>
    <span aria-hidden="true">·</span>
    <span>{t('dashboard.projectStatement')}</span>
    <span aria-hidden="true">·</span>
    <a href={PROJECT_URL} target="_blank" rel="noreferrer">
      <IoLogoGithub aria-hidden="true" />
      <span>github.com/neko233-com/buildworld</span>
    </a>
  </footer>
}

export default function Dashboard() {
  const { t } = useI18n()
  const navigate = useNavigate()
  const editable = canEdit()
  const [activeView, setActiveView] = useState(initialActiveView)
  const [projectOrder, setProjectOrder] = useState<number[]>([])
  const [draggingID, setDraggingID] = useState<number | null>(null)
  const [reordering, setReordering] = useState(false)
  const [deleting, setDeleting] = useState<number | null>(null)
  const [flagBusy, setFlagBusy] = useState<{ id: number; flag: 'favorite' } | null>(null)
  const [building, setBuilding] = useState<number | null>(null)
  const [groupsOpen, setGroupsOpen] = useState(false)
  const [groupContextMenu, setGroupContextMenu] = useState<GroupContextMenu | null>(null)
  const [groupPendingDeletion, setGroupPendingDeletion] = useState<ProjectGroup | null>(null)
  const [deleteGroupProjects, setDeleteGroupProjects] = useState(false)
  const [deletingGroup, setDeletingGroup] = useState<number | null>(null)
  const { data, loading, error, reload } = useApi(async () => {
    const [projects, overviews, groups, builds] = await Promise.all([
      api.listProjects(),
      api.listProjectBuildOverviews(),
      api.listProjectGroups(),
      api.listBuilds(30),
    ])
    return { projects, overviews, groups, builds }
  })
  const buildProjects = data?.projects || []
  const { isParameterized, start } = useJenkinsBuildFlow(editable ? buildProjects : [], navigate)
  const hasActiveBuilds = (data?.overviews || []).some(overview => ACTIVE_BUILD_STATUSES.has(overview.latest?.status || ''))

  useEffect(() => {
    const refreshInterval = hasActiveBuilds ? ACTIVE_REFRESH_INTERVAL_MS : IDLE_REFRESH_INTERVAL_MS
    const timer = window.setInterval(() => {
      if (document.visibilityState === 'visible') reload()
    }, refreshInterval)
    return () => window.clearInterval(timer)
  }, [hasActiveBuilds, reload])

  useEffect(() => {
    if (!data) return
    setProjectOrder(data.projects.map(project => project.id))
    const validViews = new Set(['all', 'favorites', ...(data.groups || []).map(group => `group-${group.id}`)])
    if (validViews.has(activeView)) return
    setActiveView('all')
    localStorage.setItem(ACTIVE_VIEW_KEY, 'all')
  }, [activeView, data])

  useEffect(() => {
    if (!groupContextMenu) return
    const dismiss = (event: PointerEvent) => {
      if (event.target instanceof Element && event.target.closest('.jenkins-group-context-menu')) return
      setGroupContextMenu(null)
    }
    const dismissOnEscape = (event: KeyboardEvent) => {
      if (event.key === 'Escape') setGroupContextMenu(null)
    }
    window.addEventListener('pointerdown', dismiss)
    window.addEventListener('keydown', dismissOnEscape)
    return () => {
      window.removeEventListener('pointerdown', dismiss)
      window.removeEventListener('keydown', dismissOnEscape)
    }
  }, [groupContextMenu])

  if (loading) return <PageState />
  if (error) return <PageState error={error} onRetry={reload} />

  const projects = data?.projects || []
  const groups = sortProjectGroups(data?.groups || [])
  const overviewsByProject = new Map((data?.overviews || []).map(overview => [overview.project_id, overview]))
  const projectsByID = new Map(projects.map(project => [project.id, project]))
  const recentBuilds = (data?.builds || []).map(build => ({
    ...build,
    project_name: projectsByID.get(build.project_id)?.name || `#${build.project_id}`,
  }))

  const views = [
    { id: 'all', label: t('common.all'), group: undefined, favorite: false, matches: () => true },
    { id: 'favorites', label: t('dashboard.favorites'), group: undefined, favorite: true, matches: (project: any) => Boolean(project.favorite) },
    ...groups.map(group => ({ id: `group-${group.id}`, label: group.name, group, favorite: false, matches: (project: any) => project.group_id === group.id })),
  ]
  const selectedView = views.find(view => view.id === activeView) || views[0]
  const orderIndex = new Map(projectOrder.map((id, index) => [id, index]))
  const visibleProjects = projects
    .filter(selectedView.matches)
    .sort((left: any, right: any) => (orderIndex.get(left.id) ?? Number.MAX_SAFE_INTEGER) - (orderIndex.get(right.id) ?? Number.MAX_SAFE_INTEGER))

  const toggleFavorite = async (project: any) => {
    const flag = 'favorite' as const
    setFlagBusy({ id: project.id, flag })
    try {
      await api.setProjectFlags(
        project.id,
        !project.favorite,
        Boolean(project.quick_access),
      )
      reload()
    } catch (reason: any) {
      dialogs.notify(reason.message || t('common.error'))
    } finally {
      setFlagBusy(null)
    }
  }

  const handleBuild = async (project: any) => {
    setBuilding(project.id)
    try {
      await start(project)
    } catch (reason: any) {
      dialogs.notify(reason.message || t('projects.buildFailed'))
    } finally {
      setBuilding(null)
    }
  }

  const persistVisibleOrder = async (nextVisibleIDs: number[]) => {
    if (reordering) return
    const previousOrder = projectOrder.length ? projectOrder : projects.map(project => project.id)
    const visibleIDs = new Set(visibleProjects.map(project => project.id))
    let visibleIndex = 0
    const nextOrder = previousOrder.map(id => visibleIDs.has(id) ? nextVisibleIDs[visibleIndex++] : id)
    setProjectOrder(nextOrder)
    setReordering(true)
    try {
      await api.reorderProjects(nextOrder)
      await reload()
    } catch (reason: any) {
      setProjectOrder(previousOrder)
      dialogs.notify(reason.message || t('dashboard.reorderFailed'))
    } finally {
      setReordering(false)
    }
  }

  const moveProject = (projectID: number, direction: -1 | 1) => {
    const visibleIDs = visibleProjects.map(project => project.id)
    const from = visibleIDs.indexOf(projectID)
    const to = from + direction
    if (from < 0 || to < 0 || to >= visibleIDs.length) return
    const next = [...visibleIDs]
    const [moved] = next.splice(from, 1)
    next.splice(to, 0, moved)
    void persistVisibleOrder(next)
  }

  const dropProject = (targetID: number) => {
    if (draggingID === null || draggingID === targetID) return
    const visibleIDs = visibleProjects.map(project => project.id)
    const from = visibleIDs.indexOf(draggingID)
    if (from < 0 || !visibleIDs.includes(targetID)) return
    const next = visibleIDs.filter(id => id !== draggingID)
    next.unshift(draggingID)
    setDraggingID(null)
    void persistVisibleOrder(next)
  }

  const deleteProject = async (project: any) => {
    if (!await dialogs.confirm(`${project.name}\n\n${t('projects.deleteConfirm')}`, {
      title: t('projects.removeTitle'),
      action: t('projects.delete'),
    })) return
    setDeleting(project.id)
    try {
      await api.deleteProject(project.id)
      dialogs.notify(`${project.name} · ${t('projects.deleted')}`, 'success')
      await reload()
    } catch (reason: any) {
      dialogs.notify(reason.message || t('projects.removeFailed'))
    } finally {
      setDeleting(null)
    }
  }

  const openGroupDeletion = (group: ProjectGroup) => {
    setGroupContextMenu(null)
    setDeleteGroupProjects(false)
    setGroupPendingDeletion(group)
  }

  const deleteGroup = async () => {
    const group = groupPendingDeletion
    if (!group || deletingGroup !== null) return
    setDeletingGroup(group.id)
    try {
      await api.deleteProjectGroup(group.id, deleteGroupProjects)
      setActiveView('all')
      localStorage.setItem(ACTIVE_VIEW_KEY, 'all')
      setGroupPendingDeletion(null)
      dialogs.notify(t(deleteGroupProjects ? 'projectGroups.deletedWithProjects' : 'projectGroups.deleted'), 'success')
      await reload()
    } catch (reason: any) {
      dialogs.notify(reason.message || t('projectGroups.deleteFailed'))
    } finally {
      setDeletingGroup(null)
    }
  }

  return <section className="jenkins-home">
    <h1 className="sr-only">{t('nav.dashboard')}</h1>
    <JenkinsHomeRail recentBuilds={recentBuilds} />
    <div className="jenkins-home-main">
      <StorageMonitor />
      <div className="jenkins-home-toolbar">
        <nav className="jenkins-view-tabs" aria-label={t('nav.dashboard')}>
          {views.map(view => <button key={view.id} type="button" className={[view.id === selectedView.id && 'active', view.favorite && 'jenkins-view-favorites'].filter(Boolean).join(' ')} aria-pressed={view.id === selectedView.id} aria-haspopup={view.group && editable ? 'menu' : undefined} onClick={() => {
            setActiveView(view.id)
            localStorage.setItem(ACTIVE_VIEW_KEY, view.id)
          }} onContextMenu={event => {
            if (!editable || !view.group) return
            event.preventDefault()
            setGroupContextMenu({ group: view.group, x: event.clientX, y: event.clientY })
          }} onKeyDown={event => {
            if (!editable || !view.group || (event.key !== 'ContextMenu' && !(event.shiftKey && event.key === 'F10'))) return
            event.preventDefault()
            const rect = event.currentTarget.getBoundingClientRect()
            setGroupContextMenu({ group: view.group, x: rect.left, y: rect.bottom + 4 })
          }}>{view.favorite && <Star size={12} fill="currentColor" aria-hidden="true" />}{view.label}</button>)}
          {editable && <button type="button" className="jenkins-view-add" aria-label={t('projectGroups.newGroup')} title={t('projectGroups.newGroup')} onClick={() => setGroupsOpen(true)}><Plus size={15} /></button>}
        </nav>
      </div>

      <div className="jenkins-job-table-wrap">
        <table className="jenkins-job-table">
          <thead><tr>
            <th className="jenkins-id-column"><span aria-label={t('projects.identifier')}>ID</span></th>
            <th className="jenkins-status-column"><span aria-label={t('projects.status')}>S</span></th>
            <th className="jenkins-name-column">{t('projects.name')}</th>
            <th>{t('dashboard.latestBuildStatus')}</th>
            <th>{t('projectDetail.lastSuccessfulBuild')}</th>
            <th>{t('projectDetail.lastFailedBuild')}</th>
            <th>{t('builds.duration')}</th>
            <th aria-label={t('projects.actions')} />
          </tr></thead>
          <tbody>
            {!visibleProjects.length && <tr><td className="jenkins-job-empty" colSpan={8}><Folder size={20} /><span>{t('common.noData')}</span></td></tr>}
            {visibleProjects.map((project: any, projectIndex) => {
              const overview = overviewsByProject.get(project.id)
              const latest = overview?.latest
              const lastSuccess = overview?.last_success
              const lastFailure = overview?.last_failure
              const status = latest ? buildStatusTone(latest.status) : 'cancelled'
              const statusLabel = latest ? buildStatusLabel(t, latest.status) : t('dashboard.noBuilds')
              const buildLabel = t(isParameterized(project) ? 'builds.buildWithParameters' : 'projects.build')
              const visibleIndex = visibleProjects.findIndex(candidate => candidate.id === project.id)
              return <tr
                key={project.id}
                className={draggingID === project.id ? 'dragging' : ''}
                onDragOver={event => {
                  if (!editable || reordering) return
                  event.preventDefault()
                  event.dataTransfer.dropEffect = 'move'
                }}
                onDrop={event => {
                  event.preventDefault()
                  dropProject(project.id)
                }}
              >
                <td className="jenkins-project-id"><div><span className="jenkins-project-id-values"><span className="jenkins-project-sequence" title={`${t('projects.sequence')} ${projectIndex + 1}`}>{projectIndex + 1}</span><span className="jenkins-project-id-value">{project.id}</span></span>{editable && <button
                  type="button"
                  className="jenkins-project-drag"
                  draggable={!reordering}
                  disabled={reordering}
                  aria-label={`${t('dashboard.dragProject')} ${project.name}`}
                  title={t('dashboard.dragProject')}
                  onDragStart={event => {
                    setDraggingID(project.id)
                    event.dataTransfer.effectAllowed = 'move'
                    event.dataTransfer.setData('text/plain', String(project.id))
                  }}
                  onDragEnd={() => setDraggingID(null)}
                ><GripVertical size={14} /></button>}</div></td>
                <td><span className={`jenkins-status-orb ${status}`} role="img" aria-label={statusLabel} title={statusLabel} /></td>
                <td><Link className="jenkins-job-name" to={`/projects/${project.id}`}><span><strong>{project.name}</strong>{project.default_branch && project.default_branch.trim().toLowerCase() !== 'main' && <small>{project.default_branch}</small>}</span></Link></td>
                <td><LatestBuildStatus build={latest} emptyLabel={t('projectDetail.none')} /></td>
                <td><BuildReference build={lastSuccess} emptyLabel={t('projectDetail.none')} /></td>
                <td><BuildReference build={lastFailure} emptyLabel={t('projectDetail.none')} /></td>
                <td className="jenkins-duration">{formatDuration(latest?.duration_ms)}</td>
                <td><div className="jenkins-job-actions">
                  <button type="button" className={project.favorite ? 'active' : ''} disabled={flagBusy !== null} aria-busy={flagBusy?.id === project.id && flagBusy?.flag === 'favorite'} aria-label={`${project.favorite ? t('projects.unfavorite') : t('projects.favorite')} ${project.name}`} title={project.favorite ? t('projects.unfavorite') : t('projects.favorite')} onClick={() => toggleFavorite(project)}>{flagBusy?.id === project.id && flagBusy?.flag === 'favorite' ? <LoaderCircle className="timeline-spinner" size={15} /> : <Star size={15} />}</button>
                  {editable && <button type="button" disabled={reordering || visibleIndex === 0} aria-label={`${t('buildQueue.moveUp')} ${project.name}`} title={t('buildQueue.moveUp')} onClick={() => moveProject(project.id, -1)}><ArrowUp size={15} /></button>}
                  {editable && <button type="button" disabled={reordering || visibleIndex === visibleProjects.length - 1} aria-label={`${t('buildQueue.moveDown')} ${project.name}`} title={t('buildQueue.moveDown')} onClick={() => moveProject(project.id, 1)}><ArrowDown size={15} /></button>}
                  {editable && <button type="button" disabled={building !== null || project.enabled === false} aria-busy={building === project.id} aria-label={`${buildLabel} ${project.name}`} title={project.enabled === false ? t('common.disabled') : buildLabel} onClick={() => handleBuild(project)}>{building === project.id ? <LoaderCircle className="timeline-spinner" size={15} /> : <Play size={15} />}</button>}
                  {editable && <button type="button" className="danger" disabled={deleting !== null} aria-busy={deleting === project.id} aria-label={`${t('projects.delete')} ${project.name}`} title={t('projects.delete')} onClick={() => deleteProject(project)}>{deleting === project.id ? <LoaderCircle className="timeline-spinner" size={15} /> : <Trash2 size={15} />}</button>}
                </div></td>
              </tr>
            })}
          </tbody>
        </table>
      </div>

      <footer className="jenkins-table-footer">
        <Link to="/projects" className="jenkins-more" aria-label={t('nav.projects')} title={t('nav.projects')}><MoreHorizontal size={18} /></Link>
      </footer>
      <ProjectFooter />
    </div>
    {groupContextMenu && <div className="jenkins-group-context-menu" role="menu" aria-label={groupContextMenu.group.name} style={{ left: `min(${groupContextMenu.x}px, calc(100vw - 220px))`, top: `min(${groupContextMenu.y}px, calc(100dvh - 60px))` }}>
      <button type="button" role="menuitem" onClick={() => openGroupDeletion(groupContextMenu.group)}><Trash2 size={15} />{t('projectGroups.deleteGroup')}</button>
    </div>}
    {groupPendingDeletion && <ModalDialog className="project-group-delete-dialog" ariaLabel={t('projectGroups.deleteTitle')} busy={deletingGroup !== null} onClose={() => { if (deletingGroup === null) setGroupPendingDeletion(null) }}>
      <header><div><Trash2 size={18} /><div><h2>{t('projectGroups.deleteTitle')}</h2><p>{t('projectGroups.deleteConfirm').replace('{name}', groupPendingDeletion.name)}</p></div></div><button type="button" onClick={() => setGroupPendingDeletion(null)} disabled={deletingGroup !== null} title={t('common.close')} aria-label={t('common.close')}><X size={18} /></button></header>
      <div className="project-group-delete-dialog-body">
        <p>{t('projectGroups.deleteMoveToAll').replace('{count}', String(projects.filter(project => project.group_id === groupPendingDeletion.id).length))}</p>
        <label><input type="checkbox" name="delete-group-projects" checked={deleteGroupProjects} disabled={deletingGroup !== null} onChange={event => setDeleteGroupProjects(event.target.checked)} />{t('projectGroups.deleteProjectsOption').replace('{count}', String(projects.filter(project => project.group_id === groupPendingDeletion.id).length))}</label>
        {deleteGroupProjects && <p className="warning">{t('projectGroups.deleteProjectsWarning')}</p>}
      </div>
      <footer><button type="button" className="secondary-command" disabled={deletingGroup !== null} onClick={() => setGroupPendingDeletion(null)}>{t('common.cancel')}</button><button type="button" className="danger-command" disabled={deletingGroup !== null} aria-busy={deletingGroup !== null} onClick={() => void deleteGroup()}>{deletingGroup !== null ? <LoaderCircle className="timeline-spinner" size={15} /> : <Trash2 size={15} />}{t('common.delete')}</button></footer>
    </ModalDialog>}
    {groupsOpen && <ProjectGroupsDialog groups={groups} projects={projects} onReload={reload} onClose={() => setGroupsOpen(false)} />}
  </section>
}
