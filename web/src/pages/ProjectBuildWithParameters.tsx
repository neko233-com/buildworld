import { useEffect, useState } from 'react'
import { ArrowLeft, CirclePlay, LoaderCircle, RefreshCw, Settings2 } from 'lucide-react'
import { Link, Navigate, useNavigate, useParams } from 'react-router-dom'
import { api } from '../api'
import { BuildParametersForm, type BuildProject } from '../components/RunBuildDialog'
import JenkinsPageShell from '../components/JenkinsPageShell'
import { useI18n } from '../i18n'
import './ProjectBuildWithParameters.css'

function BuildTasks({ project }: { project?: BuildProject | null }) {
  const { t } = useI18n()
  const projectPath = project ? `/projects/${project.id}` : '/projects'
  return <nav className="jenkins-context-task-list jenkins-parameter-tasks" aria-label={t('projectDetail.jobActions')}>
    <Link to={projectPath}><ArrowLeft aria-hidden="true" />{t('projectDetail.status')}</Link>
    {project && <Link className="active" to={`/projects/${project.id}/build`} aria-current="page"><CirclePlay aria-hidden="true" />{t('builds.buildWithParameters')}</Link>}
    {project && <Link to={`/projects/${project.id}/configure`}><Settings2 aria-hidden="true" />{t('projectDetail.configure')}</Link>}
  </nav>
}

export default function ProjectBuildWithParameters() {
  const { t } = useI18n()
  const navigate = useNavigate()
  const { id } = useParams<{ id: string }>()
  const projectID = Number(id)
  const validProjectID = Number.isSafeInteger(projectID) && projectID > 0
  const [project, setProject] = useState<BuildProject | null>(null)
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState('')
  const [request, setRequest] = useState(0)

  useEffect(() => {
    if (!validProjectID) return
    let active = true
    setLoading(true)
    setError('')
    api.getProject(projectID).then(nextProject => {
      if (!active) return
      setProject(nextProject)
      setLoading(false)
    }).catch(reason => {
      if (!active) return
      setError(reason instanceof Error ? reason.message : t('common.error'))
      setLoading(false)
    })
    return () => {
      active = false
    }
  }, [projectID, request, t, validProjectID])

  if (!validProjectID) return <Navigate to="/projects" replace />

  const breadcrumbs = project
    ? [{ label: project.name, to: `/projects/${project.id}` }, { label: t('builds.buildWithParameters') }]
    : [{ label: t('builds.buildWithParameters') }]

  return <JenkinsPageShell
    breadcrumbs={breadcrumbs}
    sidepanel={<BuildTasks project={project} />}
    sidepanelLabel={t('projectDetail.jobActions')}
    className="jenkins-parameter-build-page"
  >
    <div className="jenkins-parameter-build-main">
      <h1>{t('builds.buildWithParameters')}</h1>
      {loading ? <div className="jenkins-parameter-page-state" role="status" aria-live="polite"><LoaderCircle className="timeline-spinner" aria-hidden="true" /><span>{t('common.loading')}</span></div> : error ? <div className="jenkins-parameter-page-state error" role="alert"><span>{error}</span><button type="button" className="secondary-command" onClick={() => setRequest(value => value + 1)}><RefreshCw size={15} aria-hidden="true" />{t('common.retry')}</button></div> : project ? <>
        <p className="jenkins-parameter-intro">{t('builds.customBuildHelp').replace('{project}', project.name)}</p>
        <BuildParametersForm
          project={project}
          variant="page"
          onCancel={() => navigate(`/projects/${project.id}`)}
          onQueued={build => navigate(`/builds/${build.id}`)}
        />
      </> : null}
    </div>
  </JenkinsPageShell>
}
