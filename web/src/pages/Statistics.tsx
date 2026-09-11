import { useState } from 'react'
import { Activity, CheckCircle2, Clock3, Hash, XCircle } from 'lucide-react'
import { useI18n } from '../i18n'
import { api } from '../api'
import { PageState } from '../components/PageState'
import { JenkinsHeaderBreadcrumb } from '../components/JenkinsPageShell'
import { useApi } from '../hooks'
import { formatDuration } from '../lib/durationPresentation'
import { formatDateWithWeekday } from '../lib/dateTime'
import './ManagementPages.jenkins.css'

interface TrendPoint {
  date: string
  total_builds: number
  success_count: number
  failed_count: number
  avg_duration_ms: number
}

interface DashboardStats {
  total_builds: number
  success_total: number
  failed_total: number
  success_rate: number
  failure_rate: number
  avg_duration_ms: number
  trend: TrendPoint[]
}

function formatRate(value: number): string {
  return `${value.toFixed(value % 1 === 0 ? 0 : 1)}%`
}

export default function Statistics() {
  const { t } = useI18n()
  const breadcrumb = <JenkinsHeaderBreadcrumb breadcrumbs={[{ label: t('statistics.title') }]} />
  const [days, setDays] = useState(7)
  const { data: stats, loading, error, reload } = useApi<DashboardStats>(() => api.getDashboardStats(), [])

  if (loading) return <>{breadcrumb}<section className="jenkins-management-page"><PageState /></section></>
  if (error) return <>{breadcrumb}<section className="jenkins-management-page"><PageState error={error} onRetry={reload} /></section></>

  const view = (stats?.trend || []).slice(-days)
  const maxCount = Math.max(1, ...view.map(point => point.total_builds))

  return (
    <>{breadcrumb}<section className="operations-page statistics-page jenkins-management-page">
      <header className="operations-heading">
        <div>
          <p>{view.length} {t('statistics.daysWithData')}</p>
          <h1>{t('statistics.title')}</h1>
        </div>
        <div className="statistics-range" role="group" aria-label={t('statistics.range')}>
          <button type="button" className={days === 7 ? 'active' : ''} aria-pressed={days === 7} onClick={() => setDays(7)}>{t('statistics.last7Days')}</button>
          <button type="button" className={days === 30 ? 'active' : ''} aria-pressed={days === 30} onClick={() => setDays(30)}>{t('statistics.last30Days')}</button>
        </div>
      </header>

      <div className="statistics-metrics">
        <article>
          <span className="statistics-metric-icon success"><CheckCircle2 size={17} /></span>
          <div><p>{t('statistics.successRate')}</p><strong>{formatRate(stats?.success_rate || 0)}</strong><small>{stats?.success_total || 0} {t('statistics.successful')}</small></div>
        </article>
        <article>
          <span className="statistics-metric-icon failed"><XCircle size={17} /></span>
          <div><p>{t('statistics.failureRate')}</p><strong>{formatRate(stats?.failure_rate || 0)}</strong><small>{stats?.failed_total || 0} {t('statistics.failed')}</small></div>
        </article>
        <article>
          <span className="statistics-metric-icon"><Clock3 size={17} /></span>
          <div><p>{t('statistics.avgDuration')}</p><strong>{formatDuration(stats?.avg_duration_ms)}</strong><small>{t('statistics.completedBuilds')}</small></div>
        </article>
        <article>
          <span className="statistics-metric-icon total"><Hash size={17} /></span>
          <div><p>{t('statistics.totalBuilds')}</p><strong>{stats?.total_builds || 0}</strong><small>{t('statistics.allRecordedBuilds')}</small></div>
        </article>
      </div>

      <div className="statistics-grid">
        <section className="statistics-panel statistics-trend-panel">
          <header><div><Activity size={15} /><h2>{t('statistics.trend')}</h2></div><span>{days}d</span></header>
          {view.length === 0 ? <p className="dashboard-empty">{t('common.noData')}</p> : (
            <div className="statistics-chart-scroll">
              <div className="statistics-chart" style={{ minWidth: `${Math.max(420, view.length * 38)}px` }}>
                {view.map(point => {
                  const height = point.total_builds ? Math.max(4, point.total_builds / maxCount * 100) : 0
                  return <div className="statistics-column" key={point.date} title={`${formatDateWithWeekday(point.date)}: ${point.total_builds}`}>
                    <strong>{point.total_builds}</strong>
                    <div className="statistics-bar-track">
                      <div className="statistics-bar" style={{ height: `${height}%` }}>
                        <i className="failed" style={{ flex: point.failed_count }} />
                        <i className="success" style={{ flex: point.success_count }} />
                      </div>
                    </div>
                    <time dateTime={point.date}>{formatDateWithWeekday(point.date)}</time>
                  </div>
                })}
              </div>
            </div>
          )}
          <footer className="statistics-legend"><span><i className="success" />{t('statistics.successful')}</span><span><i className="failed" />{t('statistics.failed')}</span></footer>
        </section>

        <section className="statistics-panel">
          <header><div><Activity size={15} /><h2>{t('statistics.heatmap')}</h2></div><span>{t('statistics.intensity')}</span></header>
          {view.length === 0 ? <p className="dashboard-empty">{t('common.noData')}</p> : (
            <div className="statistics-heatmap">
              {view.map(point => {
                const intensity = point.total_builds / maxCount
                return <div key={point.date} title={`${formatDateWithWeekday(point.date)}: ${point.total_builds}`} style={{ '--heat': intensity } as React.CSSProperties}>
                  <span>{formatDateWithWeekday(point.date)}</span><strong>{point.total_builds}</strong>
                </div>
              })}
            </div>
          )}
          <footer className="statistics-scale"><span>{t('statistics.less')}</span><i /><i /><i /><i /><span>{t('statistics.more')}</span></footer>
        </section>
      </div>
    </section></>
  )
}
