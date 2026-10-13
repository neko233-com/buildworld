import { LoaderCircle } from 'lucide-react'
import { Link } from 'react-router-dom'
import { api } from '../api'
import { useApi } from '../hooks'
import { useI18n } from '../i18n'
import { buildStatusLabel } from '../lib/buildPresentation'
import { formatDateTime } from '../lib/dateTime'
import { releaseActionLabel } from '../lib/releasePresentation'
import './ProjectReleaseTargets.css'

export default function ProjectReleaseTargets({ projectId, refreshKey }: { projectId: number; refreshKey?: string }) {
  const { t } = useI18n()
  const { data: targets, loading, error, reload } = useApi(() => api.listReleaseTargets(projectId), [projectId, refreshKey])
  if (!loading && !error && !targets?.length) return null

  return <section className="project-release-targets" aria-label={t('projectDetail.releaseTargets')}>
    <h2>{t('projectDetail.releaseTargets')}</h2>
    {error ? <div role="alert"><span>{error}</span><button type="button" className="secondary-command" onClick={reload}>{t('common.retry')}</button></div>
      : loading && !targets ? <p role="status"><LoaderCircle className="timeline-spinner" size={15} />{t('common.loading')}</p>
        : <>
          <p>{t('projectDetail.releaseTargetsHelp')}</p>
          <div className="project-release-target-table"><table>
            <thead><tr><th>{t('projectDetail.releaseChannel')}</th><th>{t('projectDetail.releaseEnvironment')}</th><th>{t('projectDetail.lastReleaseSuccess')}</th><th>{t('projectDetail.releaseVersion')}</th><th>{t('projectDetail.latestBuild')}</th><th>{t('builds.title')}</th></tr></thead>
            <tbody>{targets?.map(target => {
              const query = new URLSearchParams({ project: String(projectId), release_channel: target.channel, release_environment: target.environment })
              const restartQuery = target.last_success?.version ? new URLSearchParams({ release_action: 'restart-version', release_channel: target.channel, release_environment: target.environment, release_version: target.last_success.version }) : null
              return <tr key={`${target.channel}:${target.environment}`}>
                <td>{target.channel}</td><td><code>{target.environment}</code></td>
                <td>{target.last_success ? <Link to={`/builds/${target.last_success.id}`}><time dateTime={target.last_success.finished_at || undefined}>{formatDateTime(target.last_success.finished_at)}</time> <span>#{target.last_success.number}</span></Link> : t('projectDetail.noReleaseSuccess')}</td>
                <td>{target.last_success?.version || '-'}{target.last_success?.branch && <small> · {target.last_success.branch}</small>}{restartQuery && <div><Link to={`/projects/${projectId}/build?${restartQuery}`}>{t('projectDetail.restartVersion')}</Link></div>}</td>
                <td>{target.latest ? <Link to={`/builds/${target.latest.id}`}>#{target.latest.number} · {buildStatusLabel(t, target.latest.status)}{target.latest.action && <small> · {releaseActionLabel(t, target.latest.action)}</small>}</Link> : '-'}</td>
                <td><Link to={`/builds?${query}`}>{t('projectDetail.viewTargetHistory')}</Link></td>
              </tr>
            })}</tbody>
          </table></div>
        </>}
  </section>
}
