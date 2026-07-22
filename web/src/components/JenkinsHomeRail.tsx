import { useEffect, useId, useMemo, useState } from 'react'
import {
  BookTemplate,
  ChevronDown,
  FolderKanban,
  GitBranch,
  History,
  LoaderCircle,
  Plus,
  RotateCw,
} from 'lucide-react'
import { Link, useNavigate } from 'react-router-dom'
import { api } from '../api'
import { canEdit } from '../authz'
import { dialogs } from './AppDialogs'
import { DISTRIBUTED_WORKERS_ENABLED } from '../featureFlags'
import { useI18n } from '../i18n'
import { buildStatusLabel, buildStatusTone } from '../lib/buildPresentation'

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
  const { t, locale } = useI18n()
  const navigate = useNavigate()
  const authorizedToEdit = useMemo(() => canEdit(), [])
  const editable = editableOverride ?? authorizedToEdit
  const agentsPanelId = useId()
  const recentBuildsPanelId = useId()
  const [queue, setQueue] = useState<QueueItem[] | null>(null)
  const [agents, setAgents] = useState<Agent[] | null>(DISTRIBUTED_WORKERS_ENABLED ? null : [])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState('')
  const [reloadVersion, setReloadVersion] = useState(0)
  const [queueOpen, setQueueOpen] = useState(initialQueueOpen)
  const [recentBuildsOpen, setRecentBuildsOpen] = useState(initialRecentBuildsOpen)
  const [agentsOpen, setAgentsOpen] = useState(true)
  const [rebuilding, setRebuilding] = useState<number | null>(null)

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
        const [nextQueue, nextAgents] = await Promise.all([
          api.listBuildQueue(),
          DISTRIBUTED_WORKERS_ENABLED ? api.listAgents() : Promise.resolve([]),
        ])
        if (!active) return
        const normalizedQueue = Array.isArray(nextQueue) ? nextQueue : []
        const normalizedAgents = Array.isArray(nextAgents) ? nextAgents : []
        setQueue(normalizedQueue)
        setAgents(normalizedAgents)
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

  const recentProjectBuilds = useMemo(() => {
    const seenProjects = new Set<number>()
    return [...recentBuilds]
      .sort((left, right) => {
        const leftStarted = left.started_at ? Date.parse(left.started_at) : 0
        const rightStarted = right.started_at ? Date.parse(right.started_at) : 0
        return (rightStarted || right.id) - (leftStarted || left.id)
      })
      .filter(build => {
        if (seenProjects.has(build.project_id)) return false
        seenProjects.add(build.project_id)
        return true
      })
      .slice(0, 8)
  }, [recentBuilds])

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
    { to: '/projects/new', label: t('projects.newProject'), Icon: Plus, editOnly: true },
    { to: '/builds', label: t('nav.builds'), Icon: History, editOnly: false },
    { to: '/templates', label: t('nav.templates'), Icon: BookTemplate, editOnly: false },
    { to: '/projects', label: t('nav.projects'), Icon: FolderKanban, editOnly: false },
    { to: '/vcs-roots', label: t('nav.vcsRoots'), Icon: GitBranch, editOnly: false },
  ]
  const queueList = (queue || []).filter(item => item.build_id !== undefined && item.build_id !== null)
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
          <LoaderCircle size={16} aria-hidden="true" />
          <span className="jenkins-rail-loading-label">{t('common.loading')}</span>
        </div>
      )}

      {error && (
        <div className="jenkins-rail-error" role="alert">
          <p className="jenkins-rail-error-message">{error}</p>
          <button className="jenkins-rail-retry" type="button" onClick={retry}>
            <RotateCw size={14} aria-hidden="true" />
            <span className="jenkins-rail-retry-label">{t('common.retry')}</span>
          </button>
        </div>
      )}

      {(queue !== null || recentProjectBuilds.length > 0 || (DISTRIBUTED_WORKERS_ENABLED && agents !== null)) && (
        <div className="jenkins-rail-panels">
          <section className={`jenkins-rail-panel ${queueOpen ? 'expanded' : 'collapsed'}`} id="buildQueue">
            <header className="jenkins-rail-panel-header">
              <Link className="jenkins-rail-panel-link" to="/build-queue">
                <span className="jenkins-rail-panel-title">{t('nav.buildQueue')} ({queueList.length})</span>
              </Link>
              <button
                className="jenkins-rail-panel-toggle"
                type="button"
                aria-label={t('nav.buildQueue')}
                aria-controls="buildQueue-content"
                aria-expanded={queueOpen}
                onClick={toggleQueue}
              >
                <ChevronDown size={15} aria-hidden="true" />
              </button>
            </header>
            {queueOpen && (
              <div className="jenkins-rail-panel-body" id="buildQueue-content">
                {queueList.length ? (
                  <ul className="jenkins-rail-queue-list">
                    {queueList.map((item, index) => (
                      <li className="jenkins-rail-queue-item" key={item.id ?? item.build_id ?? index}>
                        <Link className="jenkins-rail-queue-link" to={`/builds/${item.build_id}`}>
                          <span className="jenkins-rail-queue-name">{item.project_name || `#${item.project_id ?? item.build_id}`}</span>
                          <small className="jenkins-rail-queue-meta">
                            <span className="jenkins-rail-queue-number">#{item.build_number ?? item.build_id}</span>
                            <span className="jenkins-rail-queue-status">{buildStatusLabel(t, item.status === 'queued' ? 'pending' : item.status)}</span>
                          </small>
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
                <span className="jenkins-rail-panel-title">{t('dashboard.recentProjects')} ({recentProjectBuilds.length})</span>
              </Link>
              <button
                className="jenkins-rail-panel-toggle"
                type="button"
                aria-label={t('dashboard.recentProjects')}
                aria-controls={recentBuildsPanelId}
                aria-expanded={recentBuildsOpen}
                onClick={toggleRecentBuilds}
              >
                <ChevronDown size={15} aria-hidden="true" />
              </button>
            </header>
            {recentBuildsOpen && (
              <div className="jenkins-rail-panel-body" id={recentBuildsPanelId}>
                {recentProjectBuilds.length ? (
                  <ul className="jenkins-rail-queue-list jenkins-rail-history-list">
                    {recentProjectBuilds.map(build => {
                      const active = ACTIVE_BUILD_STATUSES.has(build.status)
                      const retrying = rebuilding === build.id
                      const statusLabel = buildStatusLabel(t, build.status)
                      const startedAt = build.started_at ? new Date(build.started_at) : null
                      return (
                        <li className="jenkins-rail-queue-item jenkins-rail-history-item" key={build.id}>
                          <span
                            className={`jenkins-rail-history-status ${buildStatusTone(build.status)}`}
                            role="img"
                            aria-label={statusLabel}
                            title={statusLabel}
                          >
                            {active && <LoaderCircle aria-hidden="true" />}
                          </span>
                          <Link className="jenkins-rail-queue-link jenkins-rail-history-link" to={`/builds/${build.id}`}>
                            <span className="jenkins-rail-queue-name jenkins-rail-history-name">{build.project_name}</span>
                            <small className="jenkins-rail-queue-meta jenkins-rail-history-meta">
                              <span>#{build.number}{build.branch ? ` · ${build.branch}` : ''}</span>
                              {startedAt && !Number.isNaN(startedAt.valueOf()) && (
                                <time dateTime={build.started_at || undefined}>{startedAt.toLocaleString(locale, { month: 'short', day: 'numeric', hour: '2-digit', minute: '2-digit' })}</time>
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
                                ? <LoaderCircle className="timeline-spinner" size={14} aria-hidden="true" />
                                : <RotateCw size={14} aria-hidden="true" />}
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
                <ChevronDown size={15} aria-hidden="true" />
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
