import { useCallback, useEffect, useMemo, useState } from 'react'
import { motion } from 'motion/react'
import { Activity, Boxes, CheckCircle2, Clock3, Cpu, Gauge, RefreshCw, ServerCog, XCircle } from 'lucide-react'
import { Link } from 'react-router-dom'
import { api } from '../api'
import { useI18n } from '../i18n'
import { buildStatusLabel, buildStatusTone } from '../lib/buildPresentation'
import { PageState } from '../components/PageState'
import { formatDuration } from '../lib/durationPresentation'

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
  agent_status: Array<{ name: string; status: string; pool: string; active_builds: number; max_builds: number }>
  project_stats: Array<{ name: string; total_builds: number; success_rate: number; last_status: string }>
  trend_data: Array<{ date: string; success: number; failed: number; running: number }>
  system_metrics: { goroutines: number; uptime: string; go_version: string; os: string; arch: string; cpus: number }
  notifications: Array<{ channel: string; event: string; status: string; time: string }>
  current_time: string
}

const emptyData: DashboardData = {
  summary: { total_builds: 0, running_builds: 0, queued_builds: 0, success_today: 0, failed_today: 0, success_rate: 0, active_agents: 0, total_agents: 0, total_projects: 0 },
  recent_builds: [],
  agent_status: [],
  project_stats: [],
  trend_data: [],
  system_metrics: { goroutines: 0, uptime: '-', go_version: '-', os: '-', arch: '-', cpus: 0 },
  notifications: [],
  current_time: '',
}

function Metric({ icon: Icon, label, value, detail, tone = '' }: { icon: typeof Activity; label: string; value: string | number; detail: string; tone?: string }) {
  return <article><span className={`data-metric-icon ${tone}`}><Icon size={17} /></span><div><p>{label}</p><strong>{value}</strong><small>{detail}</small></div></article>
}

export default function BigScreen() {
  const { t, locale } = useI18n()
  const [data, setData] = useState<DashboardData>(emptyData)
  const [loading, setLoading] = useState(true)
  const [refreshing, setRefreshing] = useState(false)
  const [error, setError] = useState('')
  const [lastUpdate, setLastUpdate] = useState<Date | null>(null)

  const fetchData = useCallback(async (background = false) => {
    if (background) setRefreshing(true)
    else setLoading(true)
    try {
      setData(await api.getBigScreenData())
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

  const maxTrend = useMemo(() => Math.max(1, ...data.trend_data.map(point => point.success + point.failed + point.running)), [data.trend_data])
  const visibleRecentBuilds = data.recent_builds.slice(0, 8)
  const formatDate = (value: string) => value ? new Date(value.replace(' ', 'T')).toLocaleString(locale === 'zh-CN' ? 'zh-CN' : 'en-US') : '-'

  if (loading) return <PageState />
  if (error && !lastUpdate) return <PageState error={error} onRetry={() => fetchData()} />

  return <motion.section className="operations-page data-dashboard-page" initial={{ opacity: 0, y: 8 }} animate={{ opacity: 1, y: 0 }} transition={{ duration: 0.24, ease: 'easeOut' }}>
    <header className="operations-heading data-dashboard-heading">
      <div><p>{t('bigScreen.operationsOverview')}</p><h1>{t('bigScreen.title')}</h1><small>{t('bigScreen.description')}</small></div>
      <div><span>{t('bigScreen.lastUpdate')}: {lastUpdate?.toLocaleTimeString() || '-'}</span><button className="secondary-command" onClick={() => fetchData(true)} disabled={refreshing}><RefreshCw className={refreshing ? 'timeline-spinner' : ''} size={15} />{t('bigScreen.refresh')}</button></div>
    </header>

    {error && <div className="detail-notice data-dashboard-error" role="alert"><span>{error}</span><div><button className="secondary-command compact" type="button" onClick={() => fetchData(true)} disabled={refreshing}><RefreshCw size={14} />{t('common.retry')}</button><button type="button" onClick={() => setError('')} title={t('common.dismiss')} aria-label={t('common.dismiss')}>×</button></div></div>}

    <section className="data-metric-grid" aria-label={t('bigScreen.summary')}>
      <Metric icon={Activity} label={t('bigScreen.totalBuilds')} value={data.summary.total_builds} detail={`${data.summary.running_builds} ${t('bigScreen.running')} · ${data.summary.queued_builds} ${t('bigScreen.queued')}`} />
      <Metric icon={CheckCircle2} label={t('bigScreen.successToday')} value={data.summary.success_today} detail={`${data.summary.success_rate.toFixed(1)}% ${t('bigScreen.successRate')}`} tone="success" />
      <Metric icon={XCircle} label={t('bigScreen.failedToday')} value={data.summary.failed_today} detail={t('bigScreen.today')} tone={data.summary.failed_today ? 'failed' : ''} />
      <Metric icon={ServerCog} label={t('bigScreen.activeAgents')} value={`${data.summary.active_agents} / ${data.summary.total_agents}`} detail={t('bigScreen.distributedCapacity')} />
      <Metric icon={Boxes} label={t('bigScreen.projects')} value={data.summary.total_projects} detail={t('bigScreen.configuredProjects')} />
    </section>

    <div className="data-dashboard-grid">
      <section className="data-panel recent-build-panel">
        <header><div><Activity size={16} /><h2>{t('bigScreen.recentBuilds')}</h2></div><div className="data-panel-actions"><span>{visibleRecentBuilds.length}/{data.recent_builds.length}</span><Link to="/builds">{t('dashboard.viewAllBuilds')}</Link></div></header>
        <div className="operations-table-wrap"><table className="operations-table data-build-table"><thead><tr><th>{t('projects.name')}</th><th>{t('builds.status')}</th><th>{t('builds.branch')}</th><th>{t('builds.duration')}</th><th>{t('projectDetail.started')}</th></tr></thead><tbody>{!visibleRecentBuilds.length && <tr><td colSpan={5} className="operations-empty">{t('common.noData')}</td></tr>}{visibleRecentBuilds.map(build => <tr key={build.id}><td><Link className="data-build-link" to={`/builds/${build.id}`}><strong>{build.project}</strong><small>#{build.number}</small></Link></td><td><span className={`build-status ${buildStatusTone(build.status)}`}>{buildStatusLabel(t, build.status)}</span></td><td><code>{build.branch || '-'}</code></td><td className="muted-cell">{formatDuration(build.duration_ms)}</td><td className="muted-cell">{formatDate(build.started_at)}</td></tr>)}</tbody></table></div>
      </section>

      <section className="data-panel trend-panel">
        <header><div><Gauge size={16} /><h2>{t('bigScreen.trend7d')}</h2></div></header>
        {data.trend_data.length ? <div className="trend-chart" aria-label={t('bigScreen.trend7d')}>{data.trend_data.map(point => {
          const success = point.success / maxTrend * 100
          const failed = point.failed / maxTrend * 100
          const running = point.running / maxTrend * 100
          const pointLabel = `${point.date}: ${t('status.success')} ${point.success}, ${t('status.failed')} ${point.failed}, ${t('status.running')} ${point.running}`
          return <div className="trend-column" key={point.date} role="img" aria-label={pointLabel}><div className="trend-values" aria-hidden="true"><span>{point.success + point.failed + point.running}</span><i className="running" style={{ height: `${running}%` }} /><i className="failed" style={{ height: `${failed}%` }} /><i className="success" style={{ height: `${success}%` }} /></div><small aria-hidden="true">{point.date.slice(5)}</small></div>
        })}</div> : <p className="data-panel-empty">{t('common.noData')}</p>}
        <footer className="trend-legend"><span><i className="success" />{t('status.success')}</span><span><i className="failed" />{t('status.failed')}</span><span><i className="running" />{t('status.running')}</span></footer>
      </section>

      <section className="data-panel worker-panel">
        <header><div><ServerCog size={16} /><h2>{t('bigScreen.agentStatus')}</h2></div><span>{data.agent_status.length}</span></header>
        <div className="worker-overview-list">{!data.agent_status.length && <p className="data-panel-empty">{t('common.noData')}</p>}{data.agent_status.map(worker => {
          const capacity = worker.max_builds ? Math.min(100, worker.active_builds / worker.max_builds * 100) : 0
          return <article key={worker.name}><div><span className={`worker-state ${worker.status}`}><i />{worker.status === 'online' ? t('agents.online') : t('agents.offline')}</span><strong>{worker.name}</strong><small>{worker.pool || t('agents.defaultPool')}</small></div><div><span>{worker.active_builds} / {worker.max_builds || '-'}</span><i><b style={{ width: `${capacity}%` }} /></i></div></article>
        })}</div>
      </section>

      <section className="data-panel project-ranking-panel">
        <header><div><Boxes size={16} /><h2>{t('bigScreen.projectRanking')}</h2></div></header>
        <div className="project-ranking-list">{!data.project_stats.length && <p className="data-panel-empty">{t('common.noData')}</p>}{data.project_stats.map(project => <article key={project.name}><div><strong>{project.name}</strong><span className={`build-status ${buildStatusTone(project.last_status)}`}>{project.last_status ? buildStatusLabel(t, project.last_status) : '-'}</span></div><div><span>{project.total_builds} {t('bigScreen.buildsUnit')}</span><em>{project.success_rate.toFixed(1)}%</em></div><i><b style={{ width: `${project.success_rate}%` }} /></i></article>)}</div>
      </section>

      <section className="data-panel system-panel">
        <header><div><Cpu size={16} /><h2>{t('bigScreen.systemInfo')}</h2></div></header>
        <dl><div><dt>{t('bigScreen.runtime')}</dt><dd>{data.system_metrics.go_version}</dd></div><div><dt>{t('bigScreen.platform')}</dt><dd>{data.system_metrics.os}/{data.system_metrics.arch}</dd></div><div><dt>CPU</dt><dd>{data.system_metrics.cpus}</dd></div><div><dt>Goroutines</dt><dd>{data.system_metrics.goroutines}</dd></div><div><dt>{t('bigScreen.uptime')}</dt><dd>{data.system_metrics.uptime}</dd></div><div><dt><Clock3 size={13} />{t('bigScreen.serverTime')}</dt><dd>{data.current_time || '-'}</dd></div></dl>
      </section>
    </div>
  </motion.section>
}
