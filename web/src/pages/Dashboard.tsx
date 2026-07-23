import { useEffect, useState } from 'react'
import { BookTemplate, Cloud, CloudRain, CloudSun, FileClock, Folder, KeyRound, LoaderCircle, MoreHorizontal, Pin, Play, Plus, Settings2, Star, Sun, UsersRound } from 'lucide-react'
import { Link, useNavigate } from 'react-router-dom'
import { api } from '../api'
import { canEdit } from '../authz'
import { dialogs } from '../components/AppDialogs'
import JenkinsHomeRail from '../components/JenkinsHomeRail'
import { PageState } from '../components/PageState'
import ProjectGroupsDialog from '../components/ProjectGroupsDialog'
import { useApi } from '../hooks'
import { useI18n } from '../i18n'
import { buildStatusLabel, buildStatusTone } from '../lib/buildPresentation'
import { formatDateTime } from '../lib/dateTime'
import { formatDuration } from '../lib/durationPresentation'
import { sortProjectGroups } from '../lib/projectGroups'
import { useJenkinsBuildFlow } from './projectBuildFlow'

type IconSize = 'small' | 'medium' | 'large'

const ICON_SIZE_KEY = 'buildworld.jenkins.icon-size'
const ACTIVE_VIEW_KEY = 'buildworld.jenkins.active-view'
const ACTIVE_BUILD_STATUSES = new Set(['running', 'pending', 'pending_approval', 'queued'])
const ACTIVE_REFRESH_INTERVAL_MS = 2_000
const IDLE_REFRESH_INTERVAL_MS = 15_000

function initialIconSize(): IconSize {
  const stored = typeof localStorage === 'undefined' ? null : localStorage.getItem(ICON_SIZE_KEY)
  return stored === 'small' || stored === 'large' ? stored : 'medium'
}

function initialActiveView() {
  if (typeof localStorage === 'undefined') return 'all'
  return localStorage.getItem(ACTIVE_VIEW_KEY) || 'all'
}

function BuildReference({ build, emptyLabel }: { build?: any; emptyLabel: string }) {
  if (!build) return <span className="jenkins-empty-value">{emptyLabel}</span>
  return <span className="jenkins-build-reference">
    <Link to={`/builds/${build.id}`}>#{build.number}</Link>
    <small>{formatDateTime(build.started_at)}</small>
  </span>
}

function Health({ builds, label }: { builds: any[]; label: string }) {
  const completed = builds.filter(build => !['running', 'pending', 'queued', 'pending_approval'].includes(build.status)).slice(0, 5)
  if (!completed.length) return <span className="jenkins-health none" role="img" aria-label={label}><Cloud size={24} /></span>
  const successRate = completed.filter(build => build.status === 'success').length / completed.length
  const accessibleLabel = `${label}: ${Math.round(successRate * 100)}%`
  if (successRate === 1) return <span className="jenkins-health excellent" role="img" aria-label={accessibleLabel}><Sun size={24} /></span>
  if (successRate >= 0.6) return <span className="jenkins-health fair" role="img" aria-label={accessibleLabel}><CloudSun size={24} /></span>
  return <span className="jenkins-health poor" role="img" aria-label={accessibleLabel}><CloudRain size={24} /></span>
}

export default function Dashboard() {
  const { t, locale } = useI18n()
  const navigate = useNavigate()
  const editable = canEdit()
  const [activeView, setActiveView] = useState(initialActiveView)
  const [sortDirection, setSortDirection] = useState<'asc' | 'desc'>('asc')
  const [iconSize, setIconSize] = useState<IconSize>(initialIconSize)
  const [flagBusy, setFlagBusy] = useState<{ id: number; flag: 'favorite' | 'quick_access' } | null>(null)
  const [building, setBuilding] = useState<number | null>(null)
  const [groupsOpen, setGroupsOpen] = useState(false)
  const { data, loading, error, reload } = useApi(async () => {
    const [projects, overviews, groups] = await Promise.all([
      api.listProjects(),
      api.listProjectBuildOverviews(),
      api.listProjectGroups(),
    ])
    return { projects, overviews, groups }
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
    const validViews = new Set(['all', 'favorites', 'quick', 'common', ...(data.groups || []).map(group => `group-${group.id}`)])
    if (validViews.has(activeView)) return
    setActiveView('all')
    localStorage.setItem(ACTIVE_VIEW_KEY, 'all')
  }, [activeView, data])

  if (loading) return <PageState />
  if (error) return <PageState error={error} onRetry={reload} />

  const projects = data?.projects || []
  const groups = sortProjectGroups(data?.groups || [])
  const overviewsByProject = new Map((data?.overviews || []).map(overview => [overview.project_id, overview]))
  const projectsByID = new Map(projects.map(project => [project.id, project]))
  const recentProjectBuilds = (data?.overviews || [])
    .flatMap(overview => overview.latest ? [{
      ...overview.latest,
      project_id: overview.project_id,
      project_name: projectsByID.get(overview.project_id)?.name || `#${overview.project_id}`,
    }] : [])

  const views = [
    { id: 'all', label: t('common.all'), matches: () => true },
    { id: 'favorites', label: t('dashboard.favorites'), matches: (project: any) => Boolean(project.favorite) },
    { id: 'quick', label: t('shell.quickAccess'), matches: (project: any) => Boolean(project.quick_access) },
    { id: 'common', label: t('dashboard.commonFunctions'), matches: () => false },
    ...groups.map(group => ({ id: `group-${group.id}`, label: group.name, matches: (project: any) => project.group_id === group.id })),
  ]
  const selectedView = views.find(view => view.id === activeView) || views[0]
  const visibleProjects = projects
    .filter(selectedView.matches)
    .sort((left: any, right: any) => left.name.localeCompare(right.name, locale) * (sortDirection === 'asc' ? 1 : -1))
  const commonLinks = [
    { to: '/api-tokens', label: t('nav.apiTokens'), description: t('dashboard.commonApiTokens'), Icon: KeyRound },
    { to: '/builds', label: t('nav.builds'), description: t('dashboard.commonBuilds'), Icon: FileClock },
    { to: '/templates', label: t('nav.templates'), description: t('dashboard.commonTemplates'), Icon: BookTemplate },
    ...(editable ? [{ to: '/settings', label: t('nav.settings'), description: t('dashboard.commonSettings'), Icon: Settings2 }] : []),
    ...(editable ? [{ to: '/users', label: t('nav.users'), description: t('dashboard.commonUsers'), Icon: UsersRound }] : []),
  ]

  const setSize = (size: IconSize) => {
    setIconSize(size)
    localStorage.setItem(ICON_SIZE_KEY, size)
  }

  const toggleFlag = async (project: any, flag: 'favorite' | 'quick_access') => {
    setFlagBusy({ id: project.id, flag })
    try {
      await api.setProjectFlags(
        project.id,
        flag === 'favorite' ? !project.favorite : project.favorite,
        flag === 'quick_access' ? !project.quick_access : project.quick_access,
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

  return <section className="jenkins-home">
    <h1 className="sr-only">{t('nav.dashboard')}</h1>
    <JenkinsHomeRail recentBuilds={recentProjectBuilds} />
    <div className="jenkins-home-main">
      <nav className="jenkins-view-tabs" aria-label={t('nav.dashboard')}>
        {views.map(view => <button key={view.id} type="button" className={view.id === selectedView.id ? 'active' : ''} aria-pressed={view.id === selectedView.id} onClick={() => {
          setActiveView(view.id)
          localStorage.setItem(ACTIVE_VIEW_KEY, view.id)
        }}>{view.label}</button>)}
        {editable && <button type="button" className="jenkins-view-add" aria-label={t('projectGroups.newGroup')} title={t('projectGroups.newGroup')} onClick={() => setGroupsOpen(true)}><Plus size={15} /></button>}
      </nav>

      {activeView === 'common' ? <section className="jenkins-common-functions" aria-label={t('dashboard.commonFunctions')}>
        <header><div><h2>{t('dashboard.commonFunctions')}</h2><p>{t('dashboard.commonFunctionsHelp')}</p></div></header>
        <div>{commonLinks.map(({ to, label, description, Icon }) => <Link key={to} to={to}><span><Icon size={19} /></span><strong>{label}</strong><small>{description}</small></Link>)}</div>
      </section> : <div className="jenkins-job-table-wrap">
        <table className={`jenkins-job-table icon-${iconSize}`}>
          <thead><tr>
            <th className="jenkins-status-column"><span aria-label={t('projects.status')}>S</span></th>
            <th className="jenkins-health-column"><span aria-label={t('statistics.successRate')}>W</span></th>
            <th className="jenkins-name-column" aria-sort={sortDirection === 'asc' ? 'ascending' : 'descending'}><button type="button" onClick={() => setSortDirection(value => value === 'asc' ? 'desc' : 'asc')}>{t('projects.name')} <span aria-hidden="true">{sortDirection === 'asc' ? '\u2193' : '\u2191'}</span></button></th>
            <th>{t('projectDetail.lastSuccessfulBuild')}</th>
            <th>{t('projectDetail.lastFailedBuild')}</th>
            <th>{t('builds.duration')}</th>
            <th aria-label={t('projects.actions')} />
          </tr></thead>
          <tbody>
            {!visibleProjects.length && <tr><td className="jenkins-job-empty" colSpan={7}><Folder size={20} /><span>{t('common.noData')}</span></td></tr>}
            {visibleProjects.map((project: any) => {
              const overview = overviewsByProject.get(project.id)
              const latest = overview?.latest
              const lastSuccess = overview?.last_success
              const lastFailure = overview?.last_failure
              const recentStatuses = (overview?.recent_statuses || []).map(status => ({ status }))
              const status = latest ? buildStatusTone(latest.status) : 'cancelled'
              const statusLabel = latest ? buildStatusLabel(t, latest.status) : t('dashboard.noBuilds')
              const buildLabel = t(isParameterized(project) ? 'builds.buildWithParameters' : 'projects.build')
              return <tr key={project.id}>
                <td><span className={`jenkins-status-orb ${status}`} role="img" aria-label={statusLabel} title={statusLabel} /></td>
                <td><Health builds={recentStatuses} label={`${project.name} ${t('statistics.successRate')}`} /></td>
                <td><Link className="jenkins-job-name" to={`/projects/${project.id}`}><span><strong>{project.name}</strong>{project.default_branch && <small>{project.default_branch}</small>}</span></Link></td>
                <td><BuildReference build={lastSuccess} emptyLabel={t('projectDetail.none')} /></td>
                <td><BuildReference build={lastFailure} emptyLabel={t('projectDetail.none')} /></td>
                <td className="jenkins-duration">{formatDuration(latest?.duration_ms)}</td>
                <td><div className="jenkins-job-actions">
                  <button type="button" className={project.favorite ? 'active' : ''} disabled={flagBusy !== null} aria-busy={flagBusy?.id === project.id && flagBusy?.flag === 'favorite'} aria-label={`${project.favorite ? t('projects.unfavorite') : t('projects.favorite')} ${project.name}`} title={project.favorite ? t('projects.unfavorite') : t('projects.favorite')} onClick={() => toggleFlag(project, 'favorite')}>{flagBusy?.id === project.id && flagBusy?.flag === 'favorite' ? <LoaderCircle className="timeline-spinner" size={15} /> : <Star size={15} />}</button>
                  <button type="button" className={project.quick_access ? 'active' : ''} disabled={flagBusy !== null} aria-busy={flagBusy?.id === project.id && flagBusy?.flag === 'quick_access'} aria-label={`${project.quick_access ? t('projects.quickAccessRemove') : t('projects.quickAccessAdd')} ${project.name}`} title={project.quick_access ? t('projects.quickAccessRemove') : t('projects.quickAccessAdd')} onClick={() => toggleFlag(project, 'quick_access')}>{flagBusy?.id === project.id && flagBusy?.flag === 'quick_access' ? <LoaderCircle className="timeline-spinner" size={15} /> : <Pin size={15} />}</button>
                  {editable && <button type="button" disabled={building !== null || project.enabled === false} aria-busy={building === project.id} aria-label={`${buildLabel} ${project.name}`} title={project.enabled === false ? t('common.disabled') : buildLabel} onClick={() => handleBuild(project)}>{building === project.id ? <LoaderCircle className="timeline-spinner" size={15} /> : <Play size={15} />}</button>}
                </div></td>
              </tr>
            })}
          </tbody>
        </table>
      </div>}

      <footer className="jenkins-table-footer">
        <div className="jenkins-icon-size" aria-label={t('builds.size')}><span>{t('builds.size')}:</span>{(['small', 'medium', 'large'] as IconSize[]).map((size, index) => <button type="button" key={size} className={iconSize === size ? 'active' : ''} aria-pressed={iconSize === size} onClick={() => setSize(size)}>{['S', 'M', 'L'][index]}</button>)}</div>
        <Link to="/projects" className="jenkins-more" aria-label={t('nav.projects')} title={t('nav.projects')}><MoreHorizontal size={18} /></Link>
      </footer>
    </div>
    {groupsOpen && <ProjectGroupsDialog groups={groups} projects={projects} onReload={reload} onClose={() => setGroupsOpen(false)} />}
  </section>
}
