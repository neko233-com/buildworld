import { useEffect, useId, useMemo, useState } from 'react'
import {
  IoAdd,
  IoAlbumsOutline,
  IoChevronDown,
  IoFolderOutline,
  IoGitBranchOutline,
  IoKeyOutline,
  IoPeopleOutline,
  IoReload,
  IoSettingsOutline,
  IoTimeOutline,
} from 'react-icons/io5'
import { Link, useNavigate } from 'react-router-dom'
import { api, type BuildQueueCapacity } from '../api'
import { canEdit } from '../authz'
import { dialogs } from './AppDialogs'
import { DISTRIBUTED_WORKERS_ENABLED } from '../featureFlags'
import { useI18n } from '../i18n'
import { buildStatusLabel, buildStatusTone } from '../lib/buildPresentation'
import { timelineProgress, type BuildTimeline } from '../lib/buildTimeline'
import { formatDateTimeWithWeekday } from '../lib/dateTime'

type JenkinsHomeRailProps = {
  editable?: boolean
  recentBuilds?: JenkinsRecentBuild[]
}

export type JenkinsRecentBuild = {
  id: number
  project_id: number
  project_name: string
  number: number
  status: string
  branch?: string | null
  started_at?: string | null
}

type QueueItem = {
  id?: number | string
  build_id?: number | string
  build_number?: number | string
  project_id?: number | string
  project_name?: string
  status?: string
}

type Agent = {
  id?: number | string
  name?: string
  status?: string
  active_builds?: number
  max_concurrent_builds?: number
}

type RunningBuildProgress = {
  percent: number
  stage: string
}

const BUILD_QUEUE_COLLAPSED_KEY = 'buildworld.jenkins.pane.buildQueue.collapsed'
const RECENT_BUILDS_COLLAPSED_KEY = 'buildworld.jenkins.pane.recentBuilds.collapsed'
const ACTIVE_BUILD_STATUSES = new Set(['running', 'pending', 'pending_approval', 'queued'])
const EMPTY_RECENT_BUILDS: JenkinsRecentBuild[] = []

function initialQueueOpen(): boolean {
  if (typeof localStorage === 'undefined') return true
  return localStorage.getItem(BUILD_QUEUE_COLLAPSED_KEY) !== 'true'
}

function initialRecentBuildsOpen(): boolean {
  if (typeof localStorage === 'undefined') return true
  return localStorage.getItem(RECENT_BUILDS_COLLAPSED_KEY) !== 'true'
}

function count(value: unknown): number {
  const parsed = Number(value)
  return Number.isFinite(parsed) && parsed > 0 ? parsed : 0
}

function errorMessage(reason: unknown, fallback: string): string {
  return reason instanceof Error && reason.message ? reason.message : fallback
}

export default function JenkinsHomeRail({ editable: editableOverride, recentBuilds = EMPTY_RECENT_BUILDS }: JenkinsHomeRailProps) {
  const { t } = useI18n()
  const navigate = useNavigate()
  const authorizedToEdit = useMemo(() => canEdit(), [])
  const editable = editableOverride ?? authorizedToEdit
  const agentsPanelId = useId()
  const recentBuildsPanelId = useId()
  const [queue, setQueue] = useState<QueueItem[] | null>(null)
  const [builtinCapacity, setBuiltinCapacity] = useState<BuildQueueCapacity>({ executor: 'builtin', max_concurrent_builds: 1 })
  const [history, setHistory] = useState<JenkinsRecentBuild[] | null>(null)
  const [agents, setAgents] = useState<Agent[] | null>(DISTRIBUTED_WORKERS_ENABLED ? null : [])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState('')
  const [reloadVersion, setReloadVersion] = useState(0)
  const [queueOpen, setQueueOpen] = useState(initialQueueOpen)
  const [recentBuildsOpen, setRecentBuildsOpen] = useState(initialRecentBuildsOpen)
  const [agentsOpen, setAgentsOpen] = useState(true)
  const [rebuilding, setRebuilding] = useState<number | null>(null)
  const [runningProgress, setRunningProgress] = useState<Record<number, RunningBuildProgress>>({})

  useEffect(() => {
    let active = true
    let inFlight = false
    let timer: number | undefined

    const clearTimer = () => {
      if (timer !== undefined) window.clearTimeout(timer)
      timer = undefined
    }

    const schedule = () => {
      clearTimer()
      if (!active || document.visibilityState !== 'visible') return
      timer = window.setTimeout(load, 5000)
    }

    const load = async () => {
      if (!active || inFlight || document.visibilityState !== 'visible') return
      inFlight = true
      setError('')
      try {
        const [nextQueue, nextCapacity, nextAgents, nextBuilds, nextProjects] = await Promise.all([
          api.listBuildQueue(),
          api.getBuildQueueCapacity().catch(() => null),
          DISTRIBUTED_WORKERS_ENABLED ? api.listAgents() : Promise.resolve([]),
          api.listBuilds(30),
          api.listProjects(),
        ])
        if (!active) return
        const normalizedQueue = Array.isArray(nextQueue) ? nextQueue : []
        const runningIDs = normalizedQueue
          .filter(item => item.status === 'running')
          .map(item => Number(item.build_id))
          .filter(Number.isFinite)
          .slice(0, 4)
        const progressEntries = await Promise.all(runningIDs.map(async buildID => {
          try {
            const timeline = await api.getBuildTimeline(buildID) as BuildTimeline
            const step = timeline.steps.find(candidate => candidate.status === 'running')
            return [buildID, { percent: timelineProgress(timeline), stage: step?.stage || step?.name || '' }] as const
          } catch {
            return null
          }
        }))
        const normalizedAgents = Array.isArray(nextAgents) ? nextAgents : []
        const projectNames = new Map((Array.isArray(nextProjects) ? nextProjects : []).map(project => [Number(project.id), String(project.name || `#${project.id}`)]))
        const normalizedHistory = (Array.isArray(nextBuilds) ? nextBuilds : []).map(build => ({
          id: Number(build.id),
          project_id: Number(build.project_id),
          project_name: projectNames.get(Number(build.project_id)) || `#${build.project_id}`,
          number: Number(build.number),
          status: String(build.status || 'pending'),
          branch: build.branch,
          started_at: build.started_at,
        }))
        setQueue(normalizedQueue)
        if (nextCapacity && count(nextCapacity.max_concurrent_builds)) {
          setBuiltinCapacity(nextCapacity)
        }
        setAgents(normalizedAgents)
        setHistory(normalizedHistory)
        setRunningProgress(Object.fromEntries(progressEntries.filter((entry): entry is readonly [number, RunningBuildProgress] => entry !== null)))
        setLoading(false)
        schedule()
      } catch (reason) {
        if (!active) return
        setError(errorMessage(reason, t('common.loadFailed')))
        setLoading(false)
        schedule()
      } finally {
        inFlight = false
      }
    }

    const handleVisibilityChange = () => {
      clearTimer()
      if (document.visibilityState === 'visible') void load()
    }

    document.addEventListener('visibilitychange', handleVisibilityChange)
    if (document.visibilityState === 'visible') void load()

    return () => {
      active = false
      clearTimer()
      document.removeEventListener('visibilitychange', handleVisibilityChange)
    }
  }, [reloadVersion, t])

  const retry = () => {
    setError('')
    setLoading(true)
    setReloadVersion(version => version + 1)
  }

  const toggleQueue = () => {
    setQueueOpen(open => {
      const next = !open
      localStorage.setItem(BUILD_QUEUE_COLLAPSED_KEY, String(!next))
      return next
    })
  }

  const toggleRecentBuilds = () => {
    setRecentBuildsOpen(open => {
      const next = !open
      localStorage.setItem(RECENT_BUILDS_COLLAPSED_KEY, String(!next))
      return next
    })
  }

  const recentBuildHistory = useMemo(() => {
    return [...(history || recentBuilds)]
      .sort((left, right) => {
        const leftStarted = left.started_at ? Date.parse(left.started_at) : 0
        const rightStarted = right.started_at ? Date.parse(right.started_at) : 0
        return (rightStarted || right.id) - (leftStarted || left.id)
      })
      .slice(0, 8)
  }, [history, recentBuilds])

  const rebuild = async (build: JenkinsRecentBuild) => {
    if (ACTIVE_BUILD_STATUSES.has(build.status) || rebuilding !== null) return
    const confirmed = await dialogs.confirm(
      t('dashboard.rebuildConfirm')
        .replace('{project}', build.project_name)
        .replace('{number}', String(build.number)),
      { title: t('dashboard.rebuildTitle'), action: t('dashboard.rebuild') },
    )
    if (!confirmed) return
    setRebuilding(build.id)
    try {
      const replayed = await api.retryBuild(build.id)
      dialogs.notify(
        t('builds.replayQueued').replace('{build}', `${build.project_name} #${build.number}`),
        'success',
      )
      navigate(`/builds/${replayed.id}`)
    } catch (reason) {
      dialogs.notify(errorMessage(reason, t('dashboard.rebuildFailed')))
    } finally {
      setRebuilding(null)
    }
  }

  const quickLinks = [
    { to: '/projects/new', label: t('projects.newProject'), Icon: IoAdd, editOnly: true },
    { to: '/builds', label: t('nav.builds'), Icon: IoTimeOutline, editOnly: false },
    { to: '/templates', label: t('nav.templates'), Icon: IoAlbumsOutline, editOnly: false },
    { to: '/projects', label: t('nav.projects'), Icon: IoFolderOutline, editOnly: false },
    { to: '/vcs-roots', label: t('nav.vcsRoots'), Icon: IoGitBranchOutline, editOnly: false },
    { to: '/api-tokens', label: t('nav.apiTokens'), Icon: IoKeyOutline, editOnly: false },
    { to: '/settings', label: t('nav.settings'), Icon: IoSettingsOutline, editOnly: true },
    { to: '/users', label: t('nav.users'), Icon: IoPeopleOutline, editOnly: true },
  ]
  const queueList = (queue || []).filter(item => item.build_id !== undefined && item.build_id !== null)
  const runningBuilds = queueList.filter(item => item.status === 'running').length
  const builtinLimit = Math.max(count(builtinCapacity.max_concurrent_builds), 1)
  const builtinCapacityLabel = t('buildQueue.builtinCapacity')
    .replace('{active}', String(runningBuilds))
    .replace('{limit}', String(builtinLimit))
  const activeCapacity = (agents || []).reduce((total, agent) => total + count(agent.active_builds), 0)
  const totalCapacity = (agents || []).reduce((total, agent) => total + count(agent.max_concurrent_builds), 0)
  const agentList = (agents || []).filter(agent => count(agent.active_builds) > 0).slice(0, 3)

  return (
    <aside className="jenkins-rail-root" aria-label={t('shell.quickAccess')}>
      <nav className="jenkins-rail-links" aria-label={t('shell.quickAccess')}>
        {quickLinks.filter(item => editable || !item.editOnly).map(({ to, label, Icon }) => (
          <Link className="jenkins-rail-link" to={to} key={to}>
            <Icon size={20} aria-hidden="true" />
            <span className="jenkins-rail-link-label">{label}</span>
          </Link>
        ))}
      </nav>

      {loading && queue === null && (
        <div className="jenkins-rail-loading" role="status" aria-live="polite">
          <IoReload size={16} aria-hidden="true" />
          <span className="jenkins-rail-loading-label">{t('common.loading')}</span>
        </div>
      )}

      {error && (
        <div className="jenkins-rail-error" role="alert">
          <p className="jenkins-rail-error-message">{error}</p>
          <button className="jenkins-rail-retry" type="button" onClick={retry}>
            <IoReload size={14} aria-hidden="true" />
            <span className="jenkins-rail-retry-label">{t('common.retry')}</span>
          </button>
        </div>
      )}

      {(queue !== null || recentBuildHistory.length > 0 || (DISTRIBUTED_WORKERS_ENABLED && agents !== null)) && (
        <div className="jenkins-rail-panels">
          <section className={`jenkins-rail-panel ${queueOpen ? 'expanded' : 'collapsed'}`} id="buildQueue">
            <header className="jenkins-rail-panel-header">
              <Link className="jenkins-rail-panel-link" to="/build-queue">
                <span className="jenkins-rail-panel-title">{t('nav.buildQueue')} ({queueList.length})</span>
                <span className="jenkins-rail-panel-count" title={builtinCapacityLabel}>{builtinCapacityLabel}</span>
              </Link>
              <button
                className="jenkins-rail-panel-toggle"
                type="button"
                aria-label={t('nav.buildQueue')}
                aria-controls="buildQueue-content"
                aria-expanded={queueOpen}
                onClick={toggleQueue}
              >
                <IoChevronDown size={15} aria-hidden="true" />
              </button>
            </header>
            {queueOpen && (
              <div className="jenkins-rail-panel-body" id="buildQueue-content">
                {queueList.length ? (
                  <ul className="jenkins-rail-queue-list">
                    {queueList.map((item, index) => (
                      <li className="jenkins-rail-queue-item" key={item.id ?? item.build_id ?? index}>
                        <span
                          className={`jenkins-rail-status-dot ${buildStatusTone(item.status === 'queued' ? 'pending' : item.status)}`}
                          role="img"
                          aria-label={buildStatusLabel(t, item.status === 'queued' ? 'pending' : item.status)}
                        />
                        <Link className="jenkins-rail-queue-link" to={`/builds/${item.build_id}`}>
                          <span className="jenkins-rail-queue-name">{item.project_name || `#${item.project_id ?? item.build_id}`}</span>
                          <small className="jenkins-rail-queue-meta">
                            <span className="jenkins-rail-queue-number">#{item.build_number ?? item.build_id}</span>
                            <span className="jenkins-rail-queue-status">{buildStatusLabel(t, item.status === 'queued' ? 'pending' : item.status)}</span>
                          </small>
                          {item.status === 'running' && (() => {
                            const progress = runningProgress[Number(item.build_id)]
                            return <span className="jenkins-rail-queue-progress">
                              <progress aria-label={`${item.project_name || item.build_id} ${t('builds.progress')}`} value={progress?.percent || undefined} max={100} />
                              <small>{progress?.stage || (progress ? `${progress.percent}%` : t('buildQueue.reasonRunning'))}</small>
                            </span>
                          })()}
                        </Link>
                      </li>
                    ))}
                  </ul>
                ) : (
                  <p className="jenkins-rail-empty">{t('buildQueue.emptyTitle')}</p>
                )}
              </div>
            )}
          </section>

          <section className={`jenkins-rail-panel jenkins-rail-history ${recentBuildsOpen ? 'expanded' : 'collapsed'}`}>
            <header className="jenkins-rail-panel-header">
              <Link className="jenkins-rail-panel-link" to="/builds">
                <span className="jenkins-rail-panel-title">{t('dashboard.recentProjects')} ({recentBuildHistory.length})</span>
              </Link>
              <button
                className="jenkins-rail-panel-toggle"
                type="button"
                aria-label={t('dashboard.recentProjects')}
                aria-controls={recentBuildsPanelId}
                aria-expanded={recentBuildsOpen}
                onClick={toggleRecentBuilds}
              >
                <IoChevronDown size={15} aria-hidden="true" />
              </button>
            </header>
            {recentBuildsOpen && (
              <div className="jenkins-rail-panel-body" id={recentBuildsPanelId}>
                {recentBuildHistory.length ? (
                  <ul className="jenkins-rail-queue-list jenkins-rail-history-list">
                    {recentBuildHistory.map(build => {
                      const active = ACTIVE_BUILD_STATUSES.has(build.status)
                      const retrying = rebuilding === build.id
                      const statusLabel = buildStatusLabel(t, build.status)
                      const startedAtLabel = formatDateTimeWithWeekday(build.started_at)
                      const branchLabel = build.branch?.trim() && build.branch.trim().toLowerCase() !== 'main' ? ` · ${build.branch.trim()}` : ''
                      return (
                        <li className="jenkins-rail-queue-item jenkins-rail-history-item" key={build.id}>
                          <span
                            className={`jenkins-rail-history-status ${buildStatusTone(build.status)}`}
                            role="img"
                            aria-label={statusLabel}
                            title={statusLabel}
                          >
                            {active && <IoReload aria-hidden="true" />}
                          </span>
                          <Link className="jenkins-rail-queue-link jenkins-rail-history-link" to={`/builds/${build.id}`}>
                            <span className="jenkins-rail-queue-name jenkins-rail-history-name">{build.project_name}</span>
                            <small className="jenkins-rail-queue-meta jenkins-rail-history-meta">
                              <span>#{build.number}{branchLabel}</span>
                              {build.started_at && startedAtLabel !== '-' && (
                                <time dateTime={build.started_at}>{startedAtLabel}</time>
                              )}
                            </small>
                          </Link>
                          {editable && (
                            <button
                              className="jenkins-rail-history-rebuild"
                              type="button"
                              data-build-id={build.id}
                              disabled={active || rebuilding !== null}
                              aria-busy={retrying}
                              aria-label={`${t('dashboard.rebuild')} ${build.project_name}`}
                              title={active ? t('dashboard.rebuildUnavailable') : t('dashboard.rebuild')}
                              onClick={() => rebuild(build)}
                            >
                              {retrying
                                ? <IoReload className="timeline-spinner" size={14} aria-hidden="true" />
                                : <IoReload size={14} aria-hidden="true" />}
                            </button>
                          )}
                        </li>
                      )
                    })}
                  </ul>
                ) : (
                  <p className="jenkins-rail-empty">{t('dashboard.noRecentProjects')}</p>
                )}
              </div>
            )}
          </section>

          {DISTRIBUTED_WORKERS_ENABLED && <section className="jenkins-rail-panel">
            <header className="jenkins-rail-panel-header">
              <Link className="jenkins-rail-panel-link" to="/agents">
                <span className="jenkins-rail-panel-title">{t('agents.title')}</span>
                <span className="jenkins-rail-panel-count">{activeCapacity} / {totalCapacity}</span>
              </Link>
              <button
                className="jenkins-rail-panel-toggle"
                type="button"
                aria-label={t('agents.title')}
                aria-controls={agentsPanelId}
                aria-expanded={agentsOpen}
                onClick={() => setAgentsOpen(open => !open)}
              >
                <IoChevronDown size={15} aria-hidden="true" />
              </button>
            </header>
            {agentsOpen && (
              <div className="jenkins-rail-panel-body" id={agentsPanelId}>
                <div className="jenkins-rail-capacity">
                  <span className="jenkins-rail-capacity-label">{t('agents.capacity')}</span>
                  <span className="jenkins-rail-capacity-value">{activeCapacity} / {totalCapacity}</span>
                  <progress
                    className="jenkins-rail-capacity-progress"
                    aria-label={t('agents.capacity')}
                    value={Math.min(activeCapacity, Math.max(totalCapacity, 1))}
                    max={Math.max(totalCapacity, 1)}
                  />
                </div>
                {agentList.length ? (
                  <ul className="jenkins-rail-agent-list">
                    {agentList.map((agent, index) => {
                      const agentCapacity = count(agent.max_concurrent_builds)
                      const agentActive = count(agent.active_builds)
                      const statusLabel = agent.status === 'online'
                        ? t('agents.online')
                        : agent.status === 'offline' ? t('agents.offline') : agent.status || t('agents.offline')
                      return (
                        <li className="jenkins-rail-agent-item" key={agent.id ?? agent.name ?? index}>
                          <span className="jenkins-rail-agent-name">{agent.name || agent.id || '-'}</span>
                          <span className="jenkins-rail-agent-status" data-status={agent.status || 'offline'}>{statusLabel}</span>
                          <span className="jenkins-rail-agent-capacity">{agentActive} / {agentCapacity}</span>
                        </li>
                      )
                    })}
                  </ul>
                ) : (
                  <p className="jenkins-rail-empty">{t('common.noData')}</p>
                )}
              </div>
            )}
          </section>}
        </div>
      )}
    </aside>
  )
}
