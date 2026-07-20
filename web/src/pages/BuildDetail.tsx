import { useEffect, useRef, useState } from 'react'
import { motion } from 'motion/react'
import { Link, Navigate, useNavigate, useParams } from 'react-router-dom'
import { Ban, Check, Circle, CircleDot, Download, ExternalLink, FileText, FlaskConical, Gauge, GitBranch, GitCommitHorizontal, ListTree, LoaderCircle, Pin, PinOff, Radio, RotateCcw, Settings2, SlidersHorizontal, Square, Upload, UserRound, Workflow, X } from 'lucide-react'
import { useI18n } from '../i18n'
import { api } from '../api'
import { useApi } from '../hooks'
import { dialogs } from '../components/AppDialogs'
import { PageState } from '../components/PageState'
import { timelineProgress, visibleBuildLog, type BuildTimelineStep } from '../lib/buildTimeline'
import { buildStatusTone, buildTriggerLabel } from '../lib/buildPresentation'
import { canEdit } from '../authz'
import { formatDuration } from '../lib/durationPresentation'
import BuildApprovalPanel from '../components/BuildApprovalPanel'
import BuildProblemsPanel from '../components/BuildProblemsPanel'
import BuildChainPanel from '../components/BuildChainPanel'

function formatTime(value?: string): string {
  return value ? new Date(value).toLocaleString() : '-'
}

function formatSize(bytes?: number): string {
  if (!bytes) return '0 B'
  if (bytes < 1024) return `${bytes} B`
  if (bytes < 1024 * 1024) return `${(bytes / 1024).toFixed(1)} KB`
  return `${(bytes / (1024 * 1024)).toFixed(1)} MB`
}

function parseParameters(value?: string): Array<[string, unknown]> {
  if (!value) return []
  try {
    const parsed = JSON.parse(value)
    return parsed && typeof parsed === 'object' && !Array.isArray(parsed) ? Object.entries(parsed) : []
  } catch {
    return []
  }
}

function parameterValue(name: string, value: unknown): string {
  if (/(password|passwd|secret|token|key)/i.test(name)) return '••••••••'
  if (typeof value === 'string') return value
  return JSON.stringify(value)
}

function TimelineStatusIcon({ status }: { status: BuildTimelineStep['status'] }) {
  if (status === 'success') return <Check size={14} />
  if (status === 'failed') return <X size={14} />
  if (status === 'running') return <LoaderCircle size={14} className="timeline-spinner" />
  if (status === 'skipped' || status === 'cancelled') return <Ban size={13} />
  return <Circle size={12} />
}

export default function BuildDetail() {
  const { t } = useI18n()
  const editable = canEdit()
  const { id } = useParams<{ id: string }>()
  const navigate = useNavigate()
  const buildId = Number(id)
  const validBuildId = Number.isSafeInteger(buildId) && buildId > 0
  const { data: build, loading, error, reload: reloadBuild } = useApi(() => validBuildId ? api.getBuild(buildId) : Promise.resolve(null), [buildId, validBuildId])
  const { data: logsResp, reload: reloadLogs } = useApi(() => validBuildId ? api.getBuildLogs(buildId) : Promise.resolve({ log: '' }), [buildId, validBuildId])
  const { data: timeline, reload: reloadTimeline } = useApi(() => validBuildId ? api.getBuildTimeline(buildId) : Promise.resolve(null), [buildId, validBuildId])
  const { data: problems, reload: reloadProblems } = useApi(() => validBuildId ? api.getBuildProblems(buildId) : Promise.resolve(null), [buildId, validBuildId])
  const { data: chain, reload: reloadChain } = useApi(() => validBuildId ? api.getBuildChain(buildId) : Promise.resolve(null), [buildId, validBuildId])
  const { data: artifacts, reload: reloadArtifacts } = useApi(() => validBuildId ? api.listArtifacts(buildId) : Promise.resolve([]), [buildId, validBuildId])
  const [retrying, setRetrying] = useState(false)
  const [pinning, setPinning] = useState(false)
  const [stopping, setStopping] = useState(false)
  const [downloading, setDownloading] = useState<string | null>(null)
  const [uploading, setUploading] = useState(false)
  const fileInputRef = useRef<HTMLInputElement>(null)

  const isExecuting = build?.status === 'running'
  const isActive = isExecuting || build?.status === 'pending' || build?.status === 'pending_approval'
  const isFinished = !!build?.status && !isActive

  useEffect(() => {
    if (!isActive) return
    const timer = setInterval(() => {
      reloadBuild()
      reloadLogs()
      reloadTimeline()
      reloadProblems()
      reloadChain()
      reloadArtifacts()
    }, 2000)
    return () => clearInterval(timer)
  }, [isActive, reloadArtifacts, reloadBuild, reloadChain, reloadLogs, reloadProblems, reloadTimeline])

  useEffect(() => {
    if (isFinished) reloadArtifacts()
  }, [isFinished, reloadArtifacts])

  const handleStop = async () => {
    if (!await dialogs.confirm(t('builds.stopConfirm'), { title: t('builds.stopTitle'), action: t('builds.stopBuild') })) return
    setStopping(true)
    try {
      await api.stopBuild(buildId)
      reloadBuild()
      reloadTimeline()
      reloadProblems()
      dialogs.notify(t('builds.stopRequested'), 'success')
    } catch (reason: any) {
      dialogs.notify(reason.message || t('common.error'))
    } finally {
      setStopping(false)
    }
  }
  const handleRetry = async () => {
    setRetrying(true)
    try {
      const retried = await api.retryBuild(buildId)
      navigate(`/builds/${retried.id}`)
    } catch (reason: any) {
      dialogs.notify(reason.message || t('common.error'))
    } finally {
      setRetrying(false)
    }
  }
  const handlePin = async () => {
    if (!build) return
    setPinning(true)
    try { await api.pinBuild(buildId, !build.pinned); reloadBuild() } catch (reason: any) { dialogs.notify(reason.message || t('common.error')) } finally { setPinning(false) }
  }
  const handleUpload = async (event: React.ChangeEvent<HTMLInputElement>) => {
    const file = event.target.files?.[0]
    if (!file) return
    setUploading(true)
    try { await api.uploadArtifact(buildId, file); reloadArtifacts() } catch (reason: any) { dialogs.notify(reason.message || t('common.error')) } finally { setUploading(false); if (fileInputRef.current) fileInputRef.current.value = '' }
  }
  const handleLogDownload = async (format: 'txt' | 'json') => {
    const key = `logs-${format}`
    setDownloading(key)
    try {
      await api.downloadBuildLogs(buildId, format)
    } catch (reason: any) {
      dialogs.notify(reason.message || t('builds.downloadFailed'))
    } finally {
      setDownloading(null)
    }
  }
  const handleArtifactDownload = async (artifact: any) => {
    const key = `artifact-${artifact.id}`
    setDownloading(key)
    try {
      await api.downloadArtifact(artifact.id, artifact.name)
      reloadArtifacts()
    } catch (reason: any) {
      dialogs.notify(reason.message || t('builds.downloadFailed'))
    } finally {
      setDownloading(null)
    }
  }

  if (!validBuildId) return <Navigate to="/builds" replace />
  if (loading) return <PageState />
  if (error) return <PageState error={error} onRetry={reloadBuild} />
  if (!build) return null

  const displayedLog = visibleBuildLog(logsResp?.log)
  const logLines = displayedLog.split('\n').filter(Boolean)
  const timelineSteps = timeline?.steps || []
  const activeTimelineStep = timelineSteps.find(step => step.status === 'running' || step.status === 'failed' || step.status === 'cancelled')
  const lastStage = activeTimelineStep?.stage || [...logLines].reverse().map(line => line.match(/^\[[^\]]+\] \[([^\]]*)\]/)?.[1]).find(Boolean) || t('builds.waiting')
  const progress = timelineProgress(timeline)
  const progressCopy = t('builds.progress')
    .replace('{completed}', String(timeline?.completed_steps || 0))
    .replace('{total}', String(timeline?.total_steps || 0))
    .replace('{percent}', String(progress))
  const parameters = parseParameters(build.parameters)
  const retriedFromBuild = build.retried_from ? chain?.nodes.find(node => node.id === build.retried_from) : null
  const dependencyBuild = build.wait_dependency_on ? chain?.nodes.find(node => node.id === build.wait_dependency_on) : null

  return <motion.section className="detail-page" initial={{ opacity: 0, y: 8 }} animate={{ opacity: 1, y: 0 }} transition={{ duration: 0.24, ease: 'easeOut' }}>
    <header className="detail-heading">
      <div>
        <div className="detail-kicker"><Link to={`/projects/${build.project_id}`}>{t('builds.project')} #{build.project_id}</Link><span>/</span>{t('builds.build')}</div>
        <div className="detail-title-row"><h1>{build.pinned && <Pin size={18} aria-label={t('builds.pinned')} />}{t('builds.build')} #{build.number}</h1><span className={`build-status ${buildStatusTone(build.status)}`}>{t(`builds.${build.status}`)}</span></div>
      </div>
      <div className="detail-actions">
        <Link className="secondary-command" to={`/builds/${buildId}/tests`}><FlaskConical size={15} />{t('builds.testReports')}</Link>
        {editable && build.status === 'failed' && <Link className="secondary-command" to={`/projects/${build.project_id}?view=settings`}><Settings2 size={15} />{t('builds.fixProjectSettings')}</Link>}
        {editable && isFinished && <><button className="secondary-command" onClick={handleRetry} disabled={retrying}><RotateCcw size={15} />{retrying ? t('builds.retrying') : t('builds.retryBuild')}</button><button className={`icon-command ${build.pinned ? 'selected' : ''}`} title={build.pinned ? t('builds.unpin') : t('builds.pin')} aria-label={build.pinned ? t('builds.unpin') : t('builds.pin')} onClick={handlePin} disabled={pinning}>{build.pinned ? <PinOff size={15} /> : <Pin size={15} />}</button></>}
        {editable && isActive && <button className="danger-command" onClick={handleStop} disabled={stopping} aria-busy={stopping}>{stopping ? <LoaderCircle className="timeline-spinner" size={14} /> : <Square size={14} />}{stopping ? t('builds.stopping') : t('builds.stopBuild')}</button>}
      </div>
    </header>

    <section className="detail-facts" aria-label={t('builds.build')}>
      <article><span>{t('builds.duration')}</span><strong>{formatDuration(build.duration_ms)}</strong></article>
      <article><span>{t('builds.branch')}</span><strong><GitBranch size={14} />{build.branch || '-'}</strong></article>
      <article><span>{t('builds.commit')}</span><strong className="mono"><GitCommitHorizontal size={14} />{build.commit_sha?.slice(0, 8) || '-'}</strong></article>
      <article><span>{t('builds.trigger')}</span><strong>{buildTriggerLabel(t, build.trigger)}</strong></article>
      <article><span>{t('builds.started')}</span><strong>{formatTime(build.started_at)}</strong></article>
      <article><span>{t('builds.finished')}</span><strong>{formatTime(build.finished_at)}</strong></article>
      {(build.agent_id || build.agent_requirements) && <article><span>{t('builds.agent')}</span><strong><UserRound size={14} />{build.agent_id ? `#${build.agent_id}` : t('builds.ruleBased')}</strong><small title={typeof build.agent_requirements === 'string' ? build.agent_requirements : JSON.stringify(build.agent_requirements || {})}>{build.agent_requirements ? `${typeof build.agent_requirements === 'string' ? build.agent_requirements : JSON.stringify(build.agent_requirements)}` : ''}</small></article>}
      {build.retried_from && <article><span>{t('builds.retriedFrom')}</span><strong><Link to={`/builds/${build.retried_from}`}>{retriedFromBuild ? `#${retriedFromBuild.number}` : `ID ${build.retried_from}`}</Link></strong></article>}
      {build.wait_dependency_on && <article><span>{t('builds.waitingForBuild')}</span><strong><Link to={`/builds/${build.wait_dependency_on}`}>{dependencyBuild ? `${dependencyBuild.project_name} #${dependencyBuild.number}` : `ID ${build.wait_dependency_on}`}</Link></strong></article>}
    </section>

    {build.status === 'pending_approval' && build.approval && <BuildApprovalPanel buildId={buildId} approval={build.approval} onResolved={() => { reloadBuild(); reloadTimeline(); reloadLogs() }} />}

    {problems && <BuildProblemsPanel buildId={buildId} projectId={build.project_id} report={problems} editable={editable} retrying={retrying} onRetry={handleRetry} />}

    {chain && <BuildChainPanel chain={chain} />}

    {parameters.length > 0 && <section className="detail-panel build-parameters-panel">
      <header><div><SlidersHorizontal size={17} /><h2>{t('builds.parameters')}</h2><span>{parameters.length}</span></div></header>
      <dl>{parameters.map(([name, value]) => <div key={name}><dt>{name}</dt><dd><code>{parameterValue(name, value)}</code></dd></div>)}</dl>
    </section>}

    <section className="build-timeline" aria-label={t('builds.timeline')}>
      <header>
        <div><Workflow size={17} /><div><h2>{t('builds.timeline')}</h2><p>{t('builds.linearFlow')}</p></div></div>
        <strong>{progressCopy}</strong>
      </header>
      <div className="timeline-progress" aria-hidden="true"><i style={{ width: `${progress}%` }} /></div>
      {timelineSteps.length
        ? <ol className="timeline-track">
            {timelineSteps.map(step => <li key={step.index} className={`timeline-step ${step.status}`} aria-current={step.status === 'running' ? 'step' : undefined}>
              <span className="timeline-status"><TimelineStatusIcon status={step.status} /></span>
              <div><small>{step.stage}</small><strong>{step.name}</strong><em>{t(`builds.${step.status}`)}</em></div>
              {step.index < timelineSteps.length - 1 ? <span className="timeline-connector" aria-hidden="true" /> : null}
            </li>)}
          </ol>
        : <div className="timeline-empty"><CircleDot size={15} />{t('builds.waitingForPlan')}</div>}
    </section>

    <section className="build-log-workbench">
      <div className="build-log-main"><header><div><FileText size={17} /><h2>{t('builds.logs')}</h2></div><span className={isExecuting ? 'live-status' : 'log-status'}><Radio size={12} />{isExecuting ? t('builds.liveStream') : t(`builds.${build.status}`)}</span></header><pre>{displayedLog || t('builds.noLogs')}</pre></div>
      <aside className="build-log-inspector"><header><ListTree size={16} />{t('builds.logInspector')}</header><dl><div><dt>{t('builds.build')}</dt><dd>#{build.number}</dd></div><div><dt>{t('builds.currentStage')}</dt><dd>{lastStage}</dd></div><div><dt>{t('builds.outputLines')}</dt><dd>{logLines.length}</dd></div><div><dt>{t('builds.duration')}</dt><dd>{formatDuration(build.duration_ms)}</dd></div></dl><div className="inspector-note"><Gauge size={15} /><span>{isExecuting ? t('builds.followingOutput') : t('builds.outputComplete')}</span></div><Link className="open-plain-log" to={`/builds/${buildId}/logs`} target="_blank" rel="noopener noreferrer"><ExternalLink size={14} />{t('builds.openStandaloneLogs')}</Link><button type="button" onClick={() => handleLogDownload('txt')} disabled={downloading !== null} aria-busy={downloading === 'logs-txt'} className="download-log-button">{downloading === 'logs-txt' ? <LoaderCircle className="timeline-spinner" size={15} /> : <Download size={15} />}{downloading === 'logs-txt' ? t('builds.downloading') : t('builds.downloadText')}</button><button type="button" onClick={() => handleLogDownload('json')} disabled={downloading !== null} aria-busy={downloading === 'logs-json'} className="download-log-link">{downloading === 'logs-json' ? t('builds.downloading') : t('builds.downloadJSON')}</button></aside>
    </section>

    <section className="detail-panel artifacts-panel"><header><div><FileText size={17} /><h2>{t('builds.artifacts')}</h2><span>{(artifacts || []).length}</span></div>{editable && <><input ref={fileInputRef} type="file" onChange={handleUpload} className="visually-hidden" id="artifact-upload" /><label htmlFor="artifact-upload" className={`secondary-command upload-control ${uploading ? 'is-disabled' : ''}`}><Upload size={15} />{uploading ? t('builds.uploading') : t('builds.uploadArtifact')}</label></>}</header>
      {(artifacts || []).length === 0 ? <p className="detail-empty">{t('builds.noArtifacts')}</p> : <div className="operations-table-wrap"><table className="operations-table"><thead><tr><th>{t('builds.name')}</th><th>{t('builds.size')}</th><th>{t('builds.downloads')}</th><th aria-label={t('builds.download')} /></tr></thead><tbody>{(artifacts || []).map((artifact: any) => <tr key={artifact.id}><td><strong>{artifact.name}</strong></td><td className="muted-cell">{formatSize(artifact.size)}</td><td className="muted-cell">{artifact.download_count || artifact.downloads || 0}</td><td><button type="button" onClick={() => handleArtifactDownload(artifact)} disabled={downloading !== null} aria-busy={downloading === `artifact-${artifact.id}`} className="table-link">{downloading === `artifact-${artifact.id}` ? <LoaderCircle className="timeline-spinner" size={14} /> : <Download size={14} />}{downloading === `artifact-${artifact.id}` ? t('builds.downloading') : t('builds.download')}</button></td></tr>)}</tbody></table></div>}
    </section>
  </motion.section>
}
