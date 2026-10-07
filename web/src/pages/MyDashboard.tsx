import { useCallback, useEffect, useState } from 'react'
import { Activity, Boxes, CheckCircle2, Gauge, LoaderCircle, Maximize2, RefreshCw, ServerCog, UserRound, XCircle } from 'lucide-react'
import { motion } from 'motion/react'
import { Link } from 'react-router-dom'
import { api } from '../api'
import { useI18n } from '../i18n'
import { buildStatusLabel, buildStatusTone } from '../lib/buildPresentation'
import { PageState } from '../components/PageState'
import { formatDateTime } from '../lib/dateTime'
import { formatDuration } from '../lib/durationPresentation'
import { currentRole, isAdmin } from '../authz'
import JenkinsHomeRail from '../components/JenkinsHomeRail'
import { DISTRIBUTED_WORKERS_ENABLED } from '../featureFlags'
import { BuildTrendEChart } from '../components/BuildTrendEChart'

interface DashboardData {
  summary: {
    total_builds: number
    running_builds: number
    queued_builds: number
    success_today: number
    failed_today: number
    success_rate: number
    active_agents: number
    total_agents: number
    total_projects: number
  }
  recent_builds: Array<{ id: number; number: number; project: string; status: string; branch: string; duration_ms: number; started_at: string }>
  trend_data: Array<{ date: string; success: number; failed: number; running: number }>
}

const emptyData: DashboardData = {
  summary: { total_builds: 0, running_builds: 0, queued_builds: 0, success_today: 0, failed_today: 0, success_rate: 0, active_agents: 0, total_agents: 0, total_projects: 0 },
  recent_builds: [],
  trend_data: [],
}

function Metric({ icon: Icon, label, value, detail, tone = '' }: { icon: typeof Activity; label: string; value: string | number; detail: string; tone?: string }) {
  return <article><span className={`data-metric-icon ${tone}`}><Icon size={17} /></span><div><p>{label}</p><strong>{value}</strong><small>{detail}</small></div></article>
}

export default function MyDashboard() {
  const { t } = useI18n()
  const [data, setData] = useState<DashboardData>(emptyData)
  const [projects, setProjects] = useState<any[]>([])
  const [loading, setLoading] = useState(true)
  const [refreshing, setRefreshing] = useState(false)
  const [error, setError] = useState('')
  const [lastUpdate, setLastUpdate] = useState<Date | null>(null)

  const role = currentRole()
  const admin = isAdmin(role)

  const fetchData = useCallback(async (background = false) => {
    if (background) setRefreshing(true)
    else setLoading(true)
    try {
      const [dashboard, quick] = await Promise.all([
        api.getBigScreenData(),
        api.listQuickAccess(),
      ])
      setData({
        summary: dashboard.summary,
        recent_builds: dashboard.recent_builds,
        trend_data: dashboard.trend_data,
      })
      setProjects(quick || [])
      setError('')
      setLastUpdate(new Date())
    } catch (reason: any) {
      setError(reason.message || t('bigScreen.loadFailed'))
    } finally {
      setLoading(false)
      setRefreshing(false)
    }
  }, [t])

  useEffect(() => {
    fetchData()
    const timer = window.setInterval(() => fetchData(true), 15000)
    return () => window.clearInterval(timer)
  }, [fetchData])

  const visibleRecentBuilds = data.recent_builds.slice(0, 8)
  if (loading) return <PageState />

  return <section className="jenkins-home jenkins-user-dashboard">
    <JenkinsHomeRail />
    <div className="jenkins-home-main jenkins-user-dashboard-main">
    <div className="operations-page data-dashboard-page">
    <header className="jenkins-page-heading data-dashboard-heading">
      <div><p>{t('nav.myDashboard')}</p><h1>{t('myDashboard.title')}</h1><small>{t('myDashboard.subtitle')}</small></div>
      <div><span>{t('bigScreen.lastUpdate')}: {formatDateTime(lastUpdate)}</span><Link className="secondary-command" to="/bigscreen"><Maximize2 size={15} />{t('bigScreen.openWall')}</Link><button className="secondary-command" onClick={() => fetchData(true)} disabled={refreshing}><RefreshCw className={refreshing ? 'timeline-spinner' : ''} size={15} />{t('bigScreen.refresh')}</button></div>
    </header>

    {error && !lastUpdate && <PageState error={error} onRetry={() => fetchData()} />}
    {error && lastUpdate && <p className="data-dashboard-refresh-error" role="alert">{error}</p>}

    <section className="data-metric-grid" aria-label={t('bigScreen.summary')}>
      <Metric icon={Activity} label={t('bigScreen.totalBuilds')} value={data.summary.total_builds} detail={`${data.summary.running_builds} ${t('bigScreen.running')} · ${data.summary.queued_builds} ${t('bigScreen.queued')}`} />
      <Metric icon={CheckCircle2} label={t('bigScreen.successToday')} value={data.summary.success_today} detail={`${data.summary.success_rate.toFixed(1)}% ${t('bigScreen.successRate')}`} tone="success" />
      <Metric icon={XCircle} label={t('bigScreen.failedToday')} value={data.summary.failed_today} detail={t('bigScreen.today')} tone={data.summary.failed_today ? 'failed' : ''} />
      {DISTRIBUTED_WORKERS_ENABLED && <Metric icon={ServerCog} label={t('bigScreen.activeAgents')} value={`${data.summary.active_agents} / ${data.summary.total_agents}`} detail={t('bigScreen.distributedCapacity')} />}
      <Metric icon={Boxes} label={t('nav.projects')} value={data.summary.total_projects} detail={`${projects.length} ${t('myDashboard.quickAccess')}`} />
    </section>

    <div className="data-dashboard-grid">
      <section className="data-panel recent-build-panel">
        <header><div><Activity size={16} /><h2>{t('bigScreen.recentBuilds')}</h2></div><div className="data-panel-actions"><span>{visibleRecentBuilds.length}/{data.recent_builds.length}</span><Link to="/builds">{t('dashboard.viewAllBuilds')}</Link></div></header>
        <div className="operations-table-wrap"><table className="operations-table data-build-table"><caption className="sr-only">{t('bigScreen.recentBuilds')}</caption><thead><tr><th>{t('projects.name')}</th><th>{t('builds.status')}</th><th>{t('builds.branch')}</th><th>{t('builds.duration')}</th><th>{t('projectDetail.started')}</th></tr></thead><tbody>{!visibleRecentBuilds.length && <tr><td colSpan={5} className="operations-empty">{t('common.noData')}</td></tr>}{visibleRecentBuilds.map(build => { const tone = buildStatusTone(build.status); return <tr key={build.id}><td><Link className="data-build-link" to={`/builds/${build.id}`}><strong>{build.project}</strong><small>#{build.number}</small></Link></td><td><span className={`jenkins-build-state ${tone}`}>{tone === 'running' ? <LoaderCircle className="timeline-spinner" size={20} aria-hidden="true" /> : <i aria-hidden="true" />}<span>{buildStatusLabel(t, build.status)}</span></span></td><td><code>{build.branch || '-'}</code></td><td className="muted-cell">{formatDuration(build.duration_ms)}</td><td className="muted-cell">{formatDateTime(build.started_at)}</td></tr>})}</tbody></table></div>
      </section>

      <motion.section className="data-panel trend-panel" initial={{ opacity: 0, scale: .98 }} animate={{ opacity: 1, scale: 1 }} transition={{ duration: .28, ease: 'easeOut' }}>
        <header><div><Gauge size={16} /><h2>{t('bigScreen.trend7d')}</h2></div></header>
        {data.trend_data.length ? <BuildTrendEChart data={data.trend_data} /> : <p className="data-panel-empty">{t('common.noData')}</p>}
      </motion.section>

      <section className="data-panel my-projects-panel" style={{ gridColumn: '1 / -1' }}>
        <header><div><Boxes size={16} /><h2>{t('myDashboard.myProjects')}</h2></div><span>{projects.length}</span></header>
        {projects.length ? <div className="my-projects-grid">{projects.map(project => <article key={project.id} className="my-project-card"><div><strong>{project.name}</strong>{project.favorite && <span className="my-project-fav" title={t('dashboard.favorites')}>★</span>}</div><Link className="secondary-command compact" to={`/projects/${project.id}`}>{t('myDashboard.openProject')}</Link></article>)}</div> : <p className="data-panel-empty">{t('myDashboard.myProjectsEmpty')}</p>}
      </section>

      <section className="data-panel my-account-panel">
        <header><div><UserRound size={16} /><h2>{t('myDashboard.myAccount')}</h2></div></header>
        <dl className="my-account-list">
          <div><dt>{t('myDashboard.role')}</dt><dd>{t(`users.role_${role}`)}</dd></div>
          <div><dt>{t('myDashboard.quickAccess')}</dt><dd>{projects.length}</dd></div>
          <div><dt>{t('nav.projects')}</dt><dd>{data.summary.total_projects}</dd></div>
          {DISTRIBUTED_WORKERS_ENABLED && <div><dt>{t('bigScreen.activeAgents')}</dt><dd>{`${data.summary.active_agents} / ${data.summary.total_agents}`}</dd></div>}
        </dl>
        {admin && <Link className="dashboard-panel-link" to="/settings">{t('nav.settings')}</Link>}
      </section>
    </div>
    </div>
    </div>
  </section>
}
