import { motion } from 'motion/react'
import { Activity, ArrowUpRight, CircleDotDashed, Clock3, Cpu, FolderKanban, XCircle } from 'lucide-react'
import { Link } from 'react-router-dom'
import { useI18n } from '../i18n'
import { api } from '../api'
import { useApi } from '../hooks'
import { buildStatusLabel, buildStatusTone } from '../lib/buildPresentation'
import { PageState } from '../components/PageState'

function StatusBadge({ status, label }: { status: string; label: string }) {
  return (
    <span className={`build-status ${buildStatusTone(status)}`}>
      {label}
    </span>
  )
}

export default function Dashboard() {
  const { t } = useI18n()
  const { data: projects, loading: lp, error: ep, reload: reloadProjects } = useApi(() => api.listProjects())
  const { data: builds, loading: lb, error: eb, reload: reloadBuilds } = useApi(() => api.listBuilds(10))
  const { data: agents, loading: la, error: ea, reload: reloadAgents } = useApi(() => api.listAgents())

  const loading = lp || lb || la
  const error = ep || eb || ea

  if (loading) return <PageState />
  if (error) return <PageState error={error} onRetry={() => { reloadProjects(); reloadBuilds(); reloadAgents() }} />

  const projectMap = new Map((projects || []).map(p => [p.id, p]))
  const activeBuilds = (builds || []).filter(b => b.status === 'running' || b.status === 'pending').length
  const onlineAgents = (agents || []).filter(a => a.status === 'online').length
  const failedBuilds = (builds || []).filter(b => b.status === 'failed').length
  const recentBuilds = (builds || []).slice(0, 6)
  const latestBuild = recentBuilds[0]
  const capacity = (agents || []).reduce((total, agent) => total + (agent.max_concurrent_builds || 0), 0)
  const usedCapacity = (agents || []).reduce((total, agent) => total + (agent.active_builds || 0), 0)

  return (
    <motion.section className="workbench-dashboard" initial={{ opacity: 0, y: 10 }} animate={{ opacity: 1, y: 0 }} transition={{ duration: 0.32, ease: 'easeOut' }}>
      <header className="dashboard-heading">
        <div>
          <p>{t('dashboard.execution')}</p>
          <h1>{t('dashboard.title')}</h1>
        </div>
        <Link className="dashboard-link" to="/builds">{t('dashboard.viewAllBuilds')}<ArrowUpRight size={15} /></Link>
      </header>

      <section className="dashboard-metrics" aria-label={t('dashboard.title')}>
        <article><span className="metric-icon projects"><FolderKanban size={18} /></span><div><p>{t('dashboard.projects')}</p><strong>{projects?.length || 0}</strong></div></article>
        <article><span className="metric-icon active"><CircleDotDashed size={18} /></span><div><p>{t('dashboard.activeBuilds')}</p><strong>{activeBuilds}</strong></div></article>
        <article><span className="metric-icon agents"><Cpu size={18} /></span><div><p>{t('dashboard.workers')}</p><strong>{onlineAgents}</strong><small>{usedCapacity} / {capacity || '-'}</small></div></article>
        <article><span className="metric-icon failed"><XCircle size={18} /></span><div><p>{t('dashboard.failedBuilds')}</p><strong>{failedBuilds}</strong></div></article>
      </section>

      <div className="dashboard-grid">
        <section className="dashboard-panel recent-builds-panel">
          <header><div><Activity size={17} /><h2>{t('dashboard.recentBuilds')}</h2></div><span>{recentBuilds.length}</span></header>
          {recentBuilds.length === 0 ? <p className="dashboard-empty">{t('dashboard.noBuilds')}</p> : <div className="build-stream">
            {recentBuilds.map((build) => {
              const project = projectMap.get(build.project_id)
              return <Link key={build.id} to={`/builds/${build.id}`} className="build-stream-row">
                <span className={`stream-marker ${buildStatusTone(build.status)}`} />
                <div className="stream-name"><strong>{project?.name || `#${build.project_id}`}</strong><small>#{build.number}{build.branch ? ` · ${build.branch}` : ''}</small></div>
                <time>{build.started_at ? new Date(build.started_at).toLocaleString() : '-'}</time>
                <StatusBadge status={build.status} label={buildStatusLabel(t, build.status)} />
              </Link>
            })}
          </div>}
        </section>

        <aside className="dashboard-panel execution-panel">
          <header><div><Cpu size={17} /><h2>{t('dashboard.execution')}</h2></div></header>
          <div className="capacity-summary"><span className={onlineAgents ? 'online-dot' : 'offline-dot'} /> <strong>{onlineAgents}</strong><small>{t('dashboard.online')}</small></div>
          <div className="capacity-bar" aria-label={`${usedCapacity} / ${capacity || 0}`}><i style={{ width: `${capacity ? Math.min(100, Math.round(usedCapacity / capacity * 100)) : 0}%` }} /></div>
          <dl><div><dt>{t('dashboard.activeBuilds')}</dt><dd>{activeBuilds}</dd></div><div><dt>{t('dashboard.pending')}</dt><dd>{(builds || []).filter(build => build.status === 'pending').length}</dd></div><div><dt>{t('dashboard.latestBuild')}</dt><dd>{latestBuild ? `#${latestBuild.number}` : '-'}</dd></div></dl>
          <Link className="dashboard-panel-link" to="/agents"><Clock3 size={14} />{t('dashboard.viewAgents')}</Link>
        </aside>
      </div>
    </motion.section>
  )
}
