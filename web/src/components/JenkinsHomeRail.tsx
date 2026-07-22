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
import { Link } from 'react-router-dom'
import { api } from '../api'
import { canEdit } from '../authz'
import { useI18n } from '../i18n'
import { buildStatusLabel } from '../lib/buildPresentation'

type JenkinsHomeRailProps = {
  editable?: boolean
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

function count(value: unknown): number {
  const parsed = Number(value)
  return Number.isFinite(parsed) && parsed > 0 ? parsed : 0
}

function hasActivity(queue: QueueItem[], agents: Agent[]): boolean {
  return queue.some(item => ['pending_approval', 'queued', 'running'].includes(item.status || ''))
    || agents.some(agent => count(agent.active_builds) > 0)
}

function errorMessage(reason: unknown, fallback: string): string {
  return reason instanceof Error && reason.message ? reason.message : fallback
}

export default function JenkinsHomeRail({ editable: editableOverride }: JenkinsHomeRailProps) {
  const { t } = useI18n()
  const authorizedToEdit = useMemo(() => canEdit(), [])
  const editable = editableOverride ?? authorizedToEdit
  const queuePanelId = useId()
  const agentsPanelId = useId()
  const [queue, setQueue] = useState<QueueItem[] | null>(null)
  const [agents, setAgents] = useState<Agent[] | null>(null)
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState('')
  const [reloadVersion, setReloadVersion] = useState(0)
  const [queueOpen, setQueueOpen] = useState(true)
  const [agentsOpen, setAgentsOpen] = useState(true)

  useEffect(() => {
    let active = true
    let inFlight = false
    let timer: number | undefined

    const clearTimer = () => {
      if (timer !== undefined) window.clearTimeout(timer)
      timer = undefined
    }

    const schedule = (busy: boolean) => {
      clearTimer()
      if (!active || document.visibilityState !== 'visible') return
      timer = window.setTimeout(load, busy ? 3000 : 5000)
    }

    const load = async () => {
      if (!active || inFlight || document.visibilityState !== 'visible') return
      inFlight = true
      setError('')
      try {
        const [nextQueue, nextAgents] = await Promise.all([api.listBuildQueue(), api.listAgents()])
        if (!active) return
        const normalizedQueue = Array.isArray(nextQueue) ? nextQueue : []
        const normalizedAgents = Array.isArray(nextAgents) ? nextAgents : []
        setQueue(normalizedQueue)
        setAgents(normalizedAgents)
        setLoading(false)
        schedule(hasActivity(normalizedQueue, normalizedAgents))
      } catch (reason) {
        if (!active) return
        setError(errorMessage(reason, t('common.loadFailed')))
        setLoading(false)
        schedule(false)
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

  const quickLinks = [
    { to: '/projects/new', label: t('projects.newProject'), Icon: Plus, editOnly: true },
    { to: '/builds', label: t('nav.builds'), Icon: History, editOnly: false },
    { to: '/templates', label: t('nav.templates'), Icon: BookTemplate, editOnly: false },
    { to: '/projects', label: t('nav.projects'), Icon: FolderKanban, editOnly: false },
    { to: '/vcs-roots', label: t('nav.vcsRoots'), Icon: GitBranch, editOnly: false },
  ]
  const queueList = (queue || []).filter(item => item.build_id !== undefined && item.build_id !== null).slice(0, 3)
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

      {loading && queue === null && agents === null && (
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

      {(queue !== null || agents !== null) && (
        <div className="jenkins-rail-panels">
          <section className="jenkins-rail-panel">
            <header className="jenkins-rail-panel-header">
              <Link className="jenkins-rail-panel-link" to="/build-queue">
                <span className="jenkins-rail-panel-title">{t('nav.buildQueue')}</span>
              </Link>
              <button
                className="jenkins-rail-panel-toggle"
                type="button"
                aria-label={t('nav.buildQueue')}
                aria-controls={queuePanelId}
                aria-expanded={queueOpen}
                onClick={() => setQueueOpen(open => !open)}
              >
                <ChevronDown size={15} aria-hidden="true" />
              </button>
            </header>
            {queueOpen && (
              <div className="jenkins-rail-panel-body" id={queuePanelId}>
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

          <section className="jenkins-rail-panel">
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
          </section>
        </div>
      )}
    </aside>
  )
}
