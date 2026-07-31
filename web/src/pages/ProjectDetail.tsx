import { useEffect, useMemo, useState } from 'react'
import {
  CircleCheckBig,
  CircleDashed,
  CircleX,
  Copy,
  LoaderCircle,
  Pencil,
  Search,
  Square,
} from 'lucide-react'
import { Link, Navigate, useNavigate, useParams } from 'react-router-dom'
import { api } from '../api'
import { canEdit } from '../authz'
import { dialogs } from '../components/AppDialogs'
import { JenkinsHeaderBreadcrumb } from '../components/JenkinsPageShell'
import { PageState } from '../components/PageState'
import { useApi } from '../hooks'
import { useI18n } from '../i18n'
import { buildStatusLabel, buildStatusTone } from '../lib/buildPresentation'
import { formatDate, formatDateTime } from '../lib/dateTime'
import { formatDuration } from '../lib/durationPresentation'
import { projectGroupPath } from '../lib/projectGroups'
import ProjectJobActions from './ProjectJobActions'
import { useJenkinsBuildFlow } from './projectBuildFlow'

const activeStatuses = new Set(['running', 'pending', 'queued', 'pending_approval'])
const buildsPerPage = 30

function JobStatusIcon({ status, size = 24 }: { status?: string; size?: number }) {
  const tone = buildStatusTone(status)
  if (tone === 'success') return <CircleCheckBig size={size} aria-hidden="true" />
  if (tone === 'failed') return <CircleX size={size} aria-hidden="true" />
  if (tone === 'running') return <LoaderCircle className="timeline-spinner" size={size} aria-hidden="true" />
  return <CircleDashed size={size} aria-hidden="true" />
}

function buildDayGroup(value: string | undefined): { key: string; label: string } {
  const label = formatDate(value)
  return { key: label === '-' ? 'unknown' : label, label }
}

export default function ProjectDetail() {
  const { t } = useI18n()
  const navigate = useNavigate()
  const { id } = useParams<{ id: string }>()
  const projectId = Number(id)
  const validProjectId = Number.isSafeInteger(projectId) && projectId > 0
  const editable = canEdit()
  const { data: project, loading, error, reload, setData: setProject } = useApi(() => validProjectId ? api.getProject(projectId) : Promise.resolve(null), [projectId, validProjectId])
  const { data: buildResult, loading: buildsLoading, error: buildsError, reload: reloadBuilds } = useApi(() => validProjectId ? api.searchBuilds({
    q: '', projectId, status: '', trigger: '', branch: '', pinned: false, page: 1, limit: 100,
  }) : Promise.resolve({ version: 1, items: [], total: 0, limit: 100, offset: 0 }), [projectId, validProjectId])
  const { data: projectGroups } = useApi(() => api.listProjectGroups())
  const [buildFilter, setBuildFilter] = useState('')
  const [buildPage, setBuildPage] = useState(0)
  const [building, setBuilding] = useState(false)
  const [deleting, setDeleting] = useState(false)
  const [stopping, setStopping] = useState<number | null>(null)
  const { isParameterized, start } = useJenkinsBuildFlow(project && editable ? [project] : [], navigate)

  const buildList = useMemo(() => [...(buildResult?.items || [])].sort((left, right) => (right.number - left.number) || (right.id - left.id)), [buildResult])
  const visibleBuilds = useMemo(() => {
    const query = buildFilter.trim().toLowerCase().replace(/^#/, '')
    if (!query) return buildList
    return buildList.filter(build => String(build.number).includes(query)
      || build.status?.toLowerCase().includes(query)
      || build.branch?.toLowerCase().includes(query)
      || build.commit_sha?.toLowerCase().includes(query))
  }, [buildFilter, buildList])
  const lastBuildPage = Math.max(0, Math.ceil(visibleBuilds.length / buildsPerPage) - 1)
  const currentBuildPage = Math.min(buildPage, lastBuildPage)
  const pagedBuilds = useMemo(() => visibleBuilds.slice(currentBuildPage * buildsPerPage, (currentBuildPage + 1) * buildsPerPage), [currentBuildPage, visibleBuilds])
  const groupedBuilds = useMemo(() => {
    const groups: Array<{ key: string; label: string; builds: typeof pagedBuilds }> = []
    for (const build of pagedBuilds) {
      const day = buildDayGroup(build.started_at)
      const previous = groups[groups.length - 1]
      if (previous?.key === day.key) previous.builds.push(build)
      else groups.push({ ...day, builds: [build] })
    }
    return groups
  }, [pagedBuilds])
  const hasActiveBuild = buildList.some(build => activeStatuses.has(build.status))

  useEffect(() => {
    if (!hasActiveBuild) return
    const pollVisibleBuilds = () => {
      if (document.visibilityState === 'visible') reloadBuilds()
    }
    const interval = window.setInterval(pollVisibleBuilds, 2_000)
    document.addEventListener('visibilitychange', pollVisibleBuilds)
    return () => {
      window.clearInterval(interval)
      document.removeEventListener('visibilitychange', pollVisibleBuilds)
    }
  }, [hasActiveBuild, reloadBuilds])

  if (!validProjectId) return <Navigate to="/projects" replace />
  if (loading) return <PageState />
  if (error) return <PageState error={error} onRetry={reload} />
  if (!project) return null

  const latest = buildList[0]
  const lastSuccess = buildList.find(build => build.status === 'success')
  const lastFailure = buildList.find(build => build.status === 'failed')
  const lastUnsuccessful = buildList.find(build => !activeStatuses.has(build.status) && build.status !== 'success')
  const lastCompleted = buildList.find(build => !activeStatuses.has(build.status))
  const projectEnabled = project.enabled !== false
  const jobTone = projectEnabled ? buildStatusTone(latest?.status) : 'neutral'
  const statusLabel = projectEnabled
    ? latest ? buildStatusLabel(t, latest.status) : t('projectDetail.noBuildsYet')
    : t('common.disabled')
  const groupName = projectGroupPath(projectGroups || [], project.group_id) || t('projectGroups.ungrouped')

  const handleBuildNow = async () => {
    setBuilding(true)
    try {
      await start(project)
    } catch (reason: any) {
      dialogs.notify(reason.message || t('projectDetail.buildFailed'))
    } finally {
      setBuilding(false)
    }
  }

  const handleDelete = async () => {
    if (!await dialogs.confirm(t('projectDetail.deleteDescriptionNamed').replace('{name}', project.name), { title: t('projectDetail.deleteTitleNamed').replace('{name}', project.name), action: t('projectDetail.deleteTitle') })) return
    setDeleting(true)
    try {
      await api.deleteProject(projectId)
      navigate('/projects')
    } catch (reason: any) {
      dialogs.notify(reason.message || t('projectDetail.deleteFailed'))
      setDeleting(false)
    }
  }

  const handleStop = async (build: any) => {
    if (!await dialogs.confirm(t('projectDetail.stopBuildConfirm').replace('{project}', project.name).replace('{build}', String(build.number)), { title: t('builds.stopTitle'), action: t('builds.stopBuild') })) return
    setStopping(build.id)
    try {
      await api.stopBuild(build.id)
      reloadBuilds()
    } catch (reason: any) {
      dialogs.notify(reason.message || t('common.error'))
    } finally {
      setStopping(null)
    }
  }

  const handleBuildFilter = (value: string) => {
    setBuildFilter(value)
    setBuildPage(0)
  }

  const copyHTTPTriggerURL = async () => {
    if (!project.http_trigger_url) return
    try {
      await navigator.clipboard.writeText(project.http_trigger_url)
      dialogs.notify(t('projectDetail.httpTriggerCopied'))
    } catch {
      const input = document.getElementById('jenkins-job-http-trigger-url') as HTMLInputElement | null
      input?.focus()
      input?.select()
      if (document.execCommand('copy')) {
        dialogs.notify(t('projectDetail.httpTriggerCopied'))
        return
      }
      dialogs.notify(t('projectDetail.httpTriggerCopyFailed'))
    }
  }

  const relatedBuild = (label: string, build: any) => build
    ? <li><Link to={`/builds/${build.id}`}>{label} (#{build.number})</Link><small>{formatDateTime(build.started_at)}</small></li>
    : <li><span>{label}</span><small>{t('projectDetail.none')}</small></li>

  return <section className="jenkins-job-page">
    <JenkinsHeaderBreadcrumb ariaLabel={t('projectDetail.breadcrumb')} breadcrumbs={[{ label: groupName, to: '/projects' }, { label: project.name }]} />

    <div className="jenkins-job-layout">
      <aside className="jenkins-job-sidebar" aria-label={t('projectDetail.jobActions')}>
        <ProjectJobActions
          project={project}
          projectGroups={projectGroups || []}
          projectId={projectId}
          activeView="status"
          editable={editable}
          building={building}
          parameterized={isParameterized(project)}
          deleting={deleting}
          onBuildNow={handleBuildNow}
          onDelete={handleDelete}
          onProjectUpdated={setProject}
        />

        <section className="jenkins-job-builds" aria-label={t('builds.title')}>
          <header><Link to={`/builds?project=${projectId}`}>{t('builds.title')}</Link><span>{buildList.length}{(buildResult?.total || 0) > buildList.length ? ` / ${buildResult?.total}` : ''}</span></header>
          <label className="jenkins-job-build-filter"><Search size={14} /><input value={buildFilter} onChange={event => handleBuildFilter(event.target.value)} placeholder={t('projectDetail.filterBuilds')} aria-label={t('projectDetail.filterBuilds')} /></label>
          {buildsError && <div className="jenkins-job-build-error" role="alert"><span>{buildsError}</span><button type="button" onClick={reloadBuilds}>{t('common.retry')}</button></div>}
          <div className="jenkins-job-build-list" aria-busy={buildsLoading}>
            {buildsLoading && !buildResult && <div className="jenkins-job-build-empty"><LoaderCircle className="timeline-spinner" size={15} />{t('common.loading')}</div>}
            {!buildsLoading && !visibleBuilds.length && <div className="jenkins-job-build-empty">{buildFilter ? t('projectDetail.noMatchingBuilds') : t('builds.noBuilds')}</div>}
            {groupedBuilds.map(group => <section className="jenkins-job-build-group" aria-label={group.label} key={group.key}>
              <h3>{group.label}</h3>
              {group.builds.map(build => <article className={`jenkins-job-build ${buildStatusTone(build.status)}`} key={build.id}>
                <Link to={`/builds/${build.id}`} aria-label={`#${build.number} ${buildStatusLabel(t, build.status)}`}>
                  <JobStatusIcon status={build.status} size={15} />
                  <strong>#{build.number}</strong>
                  <time>{formatDateTime(build.started_at)}</time>
                  <small>{formatDuration(build.duration_ms)}</small>
                </Link>
                {editable && activeStatuses.has(build.status) && <button type="button" className="jenkins-job-stop" disabled={stopping !== null} onClick={() => handleStop(build)} aria-label={`${t('builds.stopBuild')} #${build.number}`} title={t('builds.stopBuild')}>{stopping === build.id ? <LoaderCircle className="timeline-spinner" size={13} /> : <Square size={12} />}</button>}
              </article>)}
            </section>)}
          </div>
          {visibleBuilds.length > buildsPerPage && <nav className="jenkins-job-build-pagination" aria-label={t('builds.title')}>
            <button type="button" disabled={currentBuildPage === 0} onClick={() => setBuildPage(Math.max(0, currentBuildPage - 1))}>Newer builds</button>
            <button type="button" disabled={currentBuildPage === lastBuildPage} onClick={() => setBuildPage(Math.min(lastBuildPage, currentBuildPage + 1))}>Older builds</button>
          </nav>}
        </section>
      </aside>

      <div className="jenkins-job-main">
        <header className={`jenkins-job-heading ${jobTone}`}>
          <JobStatusIcon status={projectEnabled ? latest?.status : undefined} size={25} />
          <div><h1>{project.name}</h1><p>{statusLabel}</p></div>
          {editable && <Link className="secondary-command" to={`/projects/${projectId}/configure#jenkins-configure-general`}><Pencil size={14} />{t('projectDetail.editDescription')}</Link>}
        </header>
        <p className="jenkins-job-full-name">{t('projectDetail.fullProjectName').replace('{group}', groupName).replace('{project}', project.name)}</p>
        <p className="jenkins-job-description">{project.description || t('projectDetail.noDescription')}</p>

        <section className="jenkins-job-related">
          <h2>{t('projectDetail.relatedLinks')}</h2>
          <ul>
            {relatedBuild(t('projectDetail.latestBuild'), latest)}
            {relatedBuild(t('projectDetail.lastStableBuild'), lastSuccess)}
            {relatedBuild(t('projectDetail.lastSuccessfulBuild'), lastSuccess)}
            {relatedBuild(t('projectDetail.lastFailedBuild'), lastFailure)}
            {relatedBuild(t('projectDetail.lastUnsuccessfulBuild'), lastUnsuccessful)}
            {relatedBuild(t('projectDetail.lastCompletedBuild'), lastCompleted)}
          </ul>
        </section>

        <section className="jenkins-job-source">
          <h2>{t('projectDetail.repository')}</h2>
          {project.repo_url ? <a href={project.repo_url} target="_blank" rel="noreferrer">{project.repo_url}</a> : <span>{t('projectDetail.none')}</span>}
          <dl><div><dt>{t('builds.branch')}</dt><dd>{project.default_branch || '-'}</dd></div><div><dt>{t('projectGroups.title')}</dt><dd>{groupName}</dd></div></dl>
          <div className="jenkins-job-http-trigger">
            <h3>{t('projectDetail.httpTriggerAPI')}</h3>
            {project.http_trigger_enabled && project.http_trigger_url ? <div className="jenkins-job-http-trigger-url"><code>POST</code><input id="jenkins-job-http-trigger-url" readOnly value={project.http_trigger_url} aria-label={t('projectDetail.httpTriggerURL')} /><button type="button" onClick={() => void copyHTTPTriggerURL()}><Copy size={14} aria-hidden="true" />{t('common.copy')}</button></div> : <span>{project.http_trigger_enabled ? t('common.enabled') : t('projectDetail.httpTriggerDisabled')}</span>}
            {editable && <Link to={`/projects/${projectId}/configure#jenkins-configure-triggers`}>{t('projectDetail.configureHTTPTrigger')}</Link>}
          </div>
        </section>
      </div>
    </div>
  </section>
}
