import { useEffect, useState } from 'react'
import { AlertTriangle, BookTemplate, FileClock, Folder, HardDrive, KeyRound, LoaderCircle, MoreHorizontal, Pin, Play, Plus, RefreshCw, Settings2, Star, UsersRound } from 'lucide-react'
import { IoLogoGithub } from 'react-icons/io5'
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
import { formatDate, formatDateTime } from '../lib/dateTime'
import { formatDuration } from '../lib/durationPresentation'
import { sortProjectGroups } from '../lib/projectGroups'
import { useJenkinsBuildFlow } from './projectBuildFlow'

const ACTIVE_VIEW_KEY = 'buildworld.jenkins.active-view'
const ACTIVE_BUILD_STATUSES = new Set(['running', 'pending', 'pending_approval', 'queued'])
const ACTIVE_REFRESH_INTERVAL_MS = 2_000
const IDLE_REFRESH_INTERVAL_MS = 15_000
const STORAGE_REFRESH_INTERVAL_MS = 30_000
const PROJECT_URL = 'https://github.com/neko233-com/buildworld233'
const WEEKDAY_KEYS = [
  'dashboard.weekdaySunday',
  'dashboard.weekdayMonday',
  'dashboard.weekdayTuesday',
  'dashboard.weekdayWednesday',
  'dashboard.weekdayThursday',
  'dashboard.weekdayFriday',
  'dashboard.weekdaySaturday',
] as const

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
    <time dateTime={formatDate(today)}>{formatDate(today)} {t(WEEKDAY_KEYS[today.getDay()])}</time>
    <span aria-hidden="true">·</span>
    <span>{t('dashboard.projectStatement')}</span>
    <span aria-hidden="true">·</span>
    <a href={PROJECT_URL} target="_blank" rel="noreferrer">
      <IoLogoGithub aria-hidden="true" />
      <span>github.com/neko233-com/buildworld233</span>
    </a>
  </footer>
}

export default function Dashboard() {
  const { t, locale } = useI18n()
  const navigate = useNavigate()
  const editable = canEdit()
  const [activeView, setActiveView] = useState(initialActiveView)
  const [sortDirection, setSortDirection] = useState<'asc' | 'desc'>('asc')
  const [flagBusy, setFlagBusy] = useState<{ id: number; flag: 'favorite' | 'quick_access' } | null>(null)
  const [building, setBuilding] = useState<number | null>(null)
  const [groupsOpen, setGroupsOpen] = useState(false)
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
  const recentBuilds = (data?.builds || []).map(build => ({
    ...build,
    project_name: projectsByID.get(build.project_id)?.name || `#${build.project_id}`,
  }))

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
    <JenkinsHomeRail recentBuilds={recentBuilds} />
    <div className="jenkins-home-main">
      <StorageMonitor />
      <div className="jenkins-home-toolbar">
        <nav className="jenkins-view-tabs" aria-label={t('nav.dashboard')}>
          {views.map(view => <button key={view.id} type="button" className={view.id === selectedView.id ? 'active' : ''} aria-pressed={view.id === selectedView.id} onClick={() => {
            setActiveView(view.id)
            localStorage.setItem(ACTIVE_VIEW_KEY, view.id)
          }}>{view.label}</button>)}
          {editable && <button type="button" className="jenkins-view-add" aria-label={t('projectGroups.newGroup')} title={t('projectGroups.newGroup')} onClick={() => setGroupsOpen(true)}><Plus size={15} /></button>}
        </nav>
      </div>

      {activeView === 'common' ? <section className="jenkins-common-functions" aria-label={t('dashboard.commonFunctions')}>
        <header><div><h2>{t('dashboard.commonFunctions')}</h2><p>{t('dashboard.commonFunctionsHelp')}</p></div></header>
        <div>{commonLinks.map(({ to, label, description, Icon }) => <Link key={to} to={to}><span><Icon size={19} /></span><strong>{label}</strong><small>{description}</small></Link>)}</div>
      </section> : <div className="jenkins-job-table-wrap">
        <table className="jenkins-job-table">
          <thead><tr>
            <th className="jenkins-id-column"><span aria-label={t('projects.identifier')}>ID</span></th>
            <th className="jenkins-status-column"><span aria-label={t('projects.status')}>S</span></th>
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
              const status = latest ? buildStatusTone(latest.status) : 'cancelled'
              const statusLabel = latest ? buildStatusLabel(t, latest.status) : t('dashboard.noBuilds')
              const buildLabel = t(isParameterized(project) ? 'builds.buildWithParameters' : 'projects.build')
              return <tr key={project.id}>
                <td className="jenkins-project-id">{project.id}</td>
                <td><span className={`jenkins-status-orb ${status}`} role="img" aria-label={statusLabel} title={statusLabel} /></td>
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
        <Link to="/projects" className="jenkins-more" aria-label={t('nav.projects')} title={t('nav.projects')}><MoreHorizontal size={18} /></Link>
      </footer>
      <ProjectFooter />
    </div>
    {groupsOpen && <ProjectGroupsDialog groups={groups} projects={projects} onReload={reload} onClose={() => setGroupsOpen(false)} />}
  </section>
}
