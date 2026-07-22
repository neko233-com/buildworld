import { useMemo, useState } from 'react'
import {
  CircleCheckBig,
  CircleDashed,
  CircleX,
  GitBranch,
  GitCommitHorizontal,
  LoaderCircle,
  Search,
  Square,
} from 'lucide-react'
import { Link, Navigate, useNavigate, useParams, useSearchParams } from 'react-router-dom'
import { api, type ProjectChange } from '../api'
import { canEdit } from '../authz'
import { dialogs } from '../components/AppDialogs'
import { JenkinsHeaderBreadcrumb } from '../components/JenkinsPageShell'
import { PageState } from '../components/PageState'
import { useApi } from '../hooks'
import { useI18n } from '../i18n'
import { buildStatusLabel, buildStatusTone } from '../lib/buildPresentation'
import { formatDuration } from '../lib/durationPresentation'
import { projectGroupPath } from '../lib/projectGroups'
import './ProjectChanges.css'
import ProjectJobActions from './ProjectJobActions'
import { useJenkinsBuildFlow } from './projectBuildFlow'

const activeStatuses = new Set(['running', 'pending', 'queued', 'pending_approval'])

function JobStatusIcon({ status, size = 16 }: { status?: string; size?: number }) {
  const tone = buildStatusTone(status)
  if (tone === 'success') return <CircleCheckBig size={size} aria-hidden="true" />
  if (tone === 'failed') return <CircleX size={size} aria-hidden="true" />
  if (tone === 'running') return <LoaderCircle className="timeline-spinner" size={size} aria-hidden="true" />
  return <CircleDashed size={size} aria-hidden="true" />
}

function buildDateLabel(value: string | undefined, locale: string): string {
  if (!value) return '-'
  const date = new Date(value)
  if (Number.isNaN(date.getTime())) return '-'
  return date.toLocaleString(locale, { month: 'short', day: 'numeric', hour: '2-digit', minute: '2-digit' })
}

function changeDateLabel(value: string | null, locale: string): string {
  if (!value) return '-'
  const date = new Date(value)
  if (Number.isNaN(date.getTime())) return '-'
  return date.toLocaleString(locale, {
    year: 'numeric',
    month: 'long',
    day: 'numeric',
    hour: '2-digit',
    minute: '2-digit',
    second: '2-digit',
  })
}

export default function ProjectChanges() {
  const { t, locale } = useI18n()
  const navigate = useNavigate()
  const [searchParams] = useSearchParams()
  const { id } = useParams<{ id: string }>()
  const projectId = Number(id)
  const validProjectId = Number.isSafeInteger(projectId) && projectId > 0
  const editable = canEdit()
  const { data: project, loading, error, reload, setData: setProject } = useApi(
    () => validProjectId ? api.getProject(projectId) : Promise.resolve(null),
    [projectId, validProjectId],
  )
  const {
    data: changes,
    loading: changesLoading,
    error: changesError,
    reload: reloadChanges,
  } = useApi(
    () => validProjectId ? api.listProjectChanges(projectId) : Promise.resolve([] as ProjectChange[]),
    [projectId, validProjectId],
  )
  const {
    data: buildResult,
    loading: buildsLoading,
    error: buildsError,
    reload: reloadBuilds,
  } = useApi(
    () => validProjectId ? api.searchBuilds({
      q: '', projectId, status: '', trigger: '', branch: '', pinned: false, page: 1, limit: 100,
    }) : Promise.resolve({ version: 1, items: [], total: 0, limit: 100, offset: 0 }),
    [projectId, validProjectId],
  )
  const { data: projectGroups } = useApi(() => api.listProjectGroups())
  const [buildFilter, setBuildFilter] = useState('')
  const [building, setBuilding] = useState(false)
  const [deleting, setDeleting] = useState(false)
  const [stopping, setStopping] = useState<number | null>(null)
  const { isParameterized, start } = useJenkinsBuildFlow(project && editable ? [project] : [], navigate)
  const requestedBuildId = Number(searchParams.get('build'))
  const filteredChanges = Number.isSafeInteger(requestedBuildId) && requestedBuildId > 0
    ? (changes || []).filter(change => change.build_id === requestedBuildId)
    : changes

  const buildList = useMemo(
    () => [...(buildResult?.items || [])].sort((left, right) => (right.number - left.number) || (right.id - left.id)),
    [buildResult],
  )
  const visibleBuilds = useMemo(() => {
    const query = buildFilter.trim().toLowerCase().replace(/^#/, '')
    if (!query) return buildList
    return buildList.filter(build => String(build.number).includes(query)
      || build.status?.toLowerCase().includes(query)
      || build.branch?.toLowerCase().includes(query)
      || build.commit_sha?.toLowerCase().includes(query))
  }, [buildFilter, buildList])

  if (!validProjectId) return <Navigate to="/projects" replace />
  if (loading) return <PageState />
  if (error) return <PageState error={error} onRetry={reload} />
  if (!project) return null

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
    if (!await dialogs.confirm(t('projectDetail.deleteDescriptionNamed').replace('{name}', project.name), {
      title: t('projectDetail.deleteTitleNamed').replace('{name}', project.name),
      action: t('projectDetail.deleteTitle'),
    })) return
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
    if (!await dialogs.confirm(t('projectDetail.stopBuildConfirm').replace('{project}', project.name).replace('{build}', String(build.number)), {
      title: t('builds.stopTitle'),
      action: t('builds.stopBuild'),
    })) return
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

  return <section className="jenkins-job-page jenkins-changes-page">
    <JenkinsHeaderBreadcrumb ariaLabel={t('projectDetail.breadcrumb')} breadcrumbs={[
      { label: groupName, to: '/projects' },
      { label: project.name, to: `/projects/${projectId}` },
      { label: t('projectDetail.changes') },
    ]} />

    <div className="jenkins-job-layout">
      <aside className="jenkins-job-sidebar" aria-label={t('projectDetail.jobActions')}>
        <ProjectJobActions
          project={project}
          projectGroups={projectGroups || []}
          projectId={projectId}
          activeView="changes"
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
          <label className="jenkins-job-build-filter"><Search size={14} /><input value={buildFilter} onChange={event => setBuildFilter(event.target.value)} placeholder={t('projectDetail.filterBuilds')} aria-label={t('projectDetail.filterBuilds')} /></label>
          {buildsError && <div className="jenkins-job-build-error" role="alert"><span>{buildsError}</span><button type="button" onClick={reloadBuilds}>{t('common.retry')}</button></div>}
          <div className="jenkins-job-build-list" aria-busy={buildsLoading}>
            {buildsLoading && !buildResult && <div className="jenkins-job-build-empty"><LoaderCircle className="timeline-spinner" size={15} />{t('common.loading')}</div>}
            {!buildsLoading && !visibleBuilds.length && <div className="jenkins-job-build-empty">{buildFilter ? t('projectDetail.noMatchingBuilds') : t('builds.noBuilds')}</div>}
            {visibleBuilds.map(build => <article className={`jenkins-job-build ${buildStatusTone(build.status)}`} key={build.id}>
              <Link to={`/builds/${build.id}`} aria-label={`#${build.number} ${buildStatusLabel(t, build.status)}`}>
                <JobStatusIcon status={build.status} />
                <strong>#{build.number}</strong>
                <time dateTime={build.started_at}>{buildDateLabel(build.started_at, locale)}</time>
                <small>{formatDuration(build.duration_ms)}</small>
              </Link>
              {editable && activeStatuses.has(build.status) && <button type="button" className="jenkins-job-stop" disabled={stopping !== null} onClick={() => handleStop(build)} aria-label={`${t('builds.stopBuild')} #${build.number}`} title={t('builds.stopBuild')}>{stopping === build.id ? <LoaderCircle className="timeline-spinner" size={13} /> : <Square size={12} />}</button>}
            </article>)}
          </div>
        </section>
      </aside>

      <div className="jenkins-job-main jenkins-changes-main" aria-busy={changesLoading}>
        <h1>{t('projectDetail.changes')}</h1>
        {changesLoading && !changes && <div className="jenkins-changes-state" role="status"><LoaderCircle className="timeline-spinner" size={18} />{t('common.loading')}</div>}
        {changesError && <div className="jenkins-changes-state error" role="alert"><span>{changesError}</span><button type="button" className="secondary-command" onClick={reloadChanges}>{t('common.retry')}</button></div>}
        {!changesLoading && !changesError && filteredChanges?.length === 0 && <div className="jenkins-changes-empty" role="status">{t('common.noData')}</div>}
        {!changesError && filteredChanges && filteredChanges.length > 0 && <div className="jenkins-changes-list">
          {filteredChanges.map(change => <section className="jenkins-change-build" key={change.build_id} aria-labelledby={`jenkins-change-build-${change.build_id}`}>
            <h2 id={`jenkins-change-build-${change.build_id}`}>
              <Link to={`/builds/${change.build_id}`}>#{change.build_number} (<time dateTime={change.timestamp || undefined}>{changeDateLabel(change.timestamp, locale)}</time>)</Link>
            </h2>
            <div className="jenkins-change-record">
              <GitCommitHorizontal size={17} aria-hidden="true" />
              <div className="jenkins-change-revision">
                <span>{t('builds.commit')}</span>
                <code title={change.commit_sha}>{change.commit_sha}</code>
              </div>
              <div className="jenkins-change-branch">
                <span>{t('builds.branch')}</span>
                <strong><GitBranch size={13} aria-hidden="true" />{change.branch || '-'}</strong>
              </div>
              <div className="jenkins-change-status">
                <span>{t('builds.status')}</span>
                <strong className={buildStatusTone(change.status)}><JobStatusIcon status={change.status} size={13} />{buildStatusLabel(t, change.status)}</strong>
              </div>
            </div>
          </section>)}
        </div>}
      </div>
    </div>
  </section>
}
