import { useEffect, useLayoutEffect, useMemo, useRef, useState } from 'react'
import { Link, Navigate, useNavigate, useParams, useSearchParams } from 'react-router-dom'
import { Activity, AlertTriangle, Ban, Check, Circle, CircleDot, Download, ExternalLink, FileText, FlaskConical, GitBranch, GitCommitHorizontal, LoaderCircle, Package, Palette, Pause, Pin, PinOff, Play, RotateCcw, Settings2, SlidersHorizontal, Square, Upload, UserRound, Workflow, X } from 'lucide-react'
import { useI18n } from '../i18n'
import { api } from '../api'
import { useApi } from '../hooks'
import { useBuildLogStream } from '../useBuildLogStream'
import { dialogs } from '../components/AppDialogs'
import { PageState } from '../components/PageState'
import { timelineProgress, visibleBuildLog, type BuildTimelineStep } from '../lib/buildTimeline'
import { isNearLogBottom } from '../lib/logFollow'
import { logTone, readLogTonePreference, writeLogTonePreference } from '../lib/logTone'
import { buildTriggerLabel } from '../lib/buildPresentation'
import { canEdit } from '../authz'
import { formatDuration } from '../lib/durationPresentation'
import BuildApprovalPanel from '../components/BuildApprovalPanel'
import BuildProblemsPanel from '../components/BuildProblemsPanel'
import { BuildStatusBadge } from '../components/BuildStatusBadge'
import { JenkinsHeaderBreadcrumb } from '../components/JenkinsPageShell'
import ReplayBuildDialog from '../components/ReplayBuildDialog'
import './BuildDetail.jenkins.css'

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

type BuildDetailTab = 'current' | 'problems' | 'artifacts'

export default function BuildDetail() {
  const { locale, t } = useI18n()
  const editable = canEdit()
  const { id } = useParams<{ id: string }>()
  const navigate = useNavigate()
  const [searchParams, setSearchParams] = useSearchParams()
  const buildId = Number(id)
  const validBuildId = Number.isSafeInteger(buildId) && buildId > 0
  const requestedTab = searchParams.get('tab')
  const activeTab: BuildDetailTab = requestedTab === 'problems' || requestedTab === 'artifacts' ? requestedTab : 'current'
  const { data: build, loading, error, reload: reloadBuild } = useApi(() => validBuildId ? api.getBuild(buildId) : Promise.resolve(null), [buildId, validBuildId])
  const projectId = Number(build?.project_id)
  const { data: buildProject } = useApi(() => Number.isSafeInteger(projectId) && projectId > 0 ? api.getProject(projectId) : Promise.resolve(null), [projectId])
  const { data: logsResp, error: logsError, reload: reloadLogs } = useApi<{ log: string, truncated?: boolean, retention_characters?: number }>(() => validBuildId ? api.getBuildLogs(buildId) : Promise.resolve({ log: '' }), [buildId, validBuildId])
  const { data: timeline, error: timelineError, reload: reloadTimeline } = useApi(() => validBuildId ? api.getBuildTimeline(buildId) : Promise.resolve(null), [buildId, validBuildId])
  const { data: problems, loading: problemsLoading, error: problemsError, reload: reloadProblems } = useApi(() => validBuildId && build ? api.getBuildProblems(buildId) : Promise.resolve(null), [build?.id, buildId, validBuildId])
  const { data: artifacts, error: artifactsError, reload: reloadArtifacts } = useApi(() => validBuildId ? api.listArtifacts(buildId) : Promise.resolve([]), [buildId, validBuildId])
  const [retrying, setRetrying] = useState(false)
  const [replayOpen, setReplayOpen] = useState(false)
  const [pinning, setPinning] = useState(false)
  const [stopping, setStopping] = useState(false)
  const [downloading, setDownloading] = useState<string | null>(null)
  const [uploading, setUploading] = useState(false)
  const fileInputRef = useRef<HTMLInputElement>(null)
  const consoleRef = useRef<HTMLPreElement>(null)
  const followConsoleRef = useRef(true)
  const activeBuildRef = useRef<number | null>(null)
  const [followConsole, setFollowConsole] = useState(true)
  const [colorizeLogs, setColorizeLogs] = useState(readLogTonePreference)

  const isExecuting = build?.status === 'running'
  const isActive = isExecuting || build?.status === 'pending' || build?.status === 'queued' || build?.status === 'pending_approval'
  const isFinished = !!build?.status && !isActive
  const { log: liveLog, state: liveLogState } = useBuildLogStream({
    buildID: buildId,
    enabled: validBuildId && isActive,
    snapshot: logsResp?.log,
    reloadSnapshot: reloadLogs,
    onBuildStatus: reloadBuild,
  })
  const displayedLog = useMemo(() => visibleBuildLog(liveLog), [liveLog])
  const consoleLines = useMemo(() => displayedLog ? displayedLog.split(/\r?\n/) : [], [displayedLog])
  const logLines = useMemo(() => consoleLines.filter(Boolean), [consoleLines])
  const liveLogStatus = t(`builds.${liveLogState}`)
  const problemsPending = problemsLoading || (!!build && !problems && !problemsError)

  useEffect(() => {
    if (!isActive) return
    const refreshActiveBuild = () => {
      if (document.visibilityState !== 'visible') return
      reloadBuild()
      if (liveLogState !== 'live') reloadLogs()
      reloadTimeline()
      if (activeTab === 'problems') reloadProblems()
      reloadArtifacts()
    }
    const timer = setInterval(refreshActiveBuild, 2000)
    document.addEventListener('visibilitychange', refreshActiveBuild)
    return () => {
      clearInterval(timer)
      document.removeEventListener('visibilitychange', refreshActiveBuild)
    }
  }, [activeTab, isActive, liveLogState, reloadArtifacts, reloadBuild, reloadLogs, reloadProblems, reloadTimeline])

  useEffect(() => {
    const wasActive = activeBuildRef.current === buildId
    if (isActive) activeBuildRef.current = buildId
    if (!wasActive || !isFinished) return
    activeBuildRef.current = null
    reloadLogs()
    reloadTimeline()
    reloadArtifacts()
    if (activeTab === 'problems') reloadProblems()
  }, [activeTab, buildId, isActive, isFinished, reloadArtifacts, reloadLogs, reloadProblems, reloadTimeline])

  useEffect(() => {
    followConsoleRef.current = true
    setFollowConsole(true)
  }, [buildId])

  useLayoutEffect(() => {
    if (activeTab !== 'current') return
    if (!followConsoleRef.current) return
    const scrollToLatest = () => {
      if (!followConsoleRef.current) return
      const consoleOutput = consoleRef.current
      if (consoleOutput) consoleOutput.scrollTop = consoleOutput.scrollHeight
    }
    scrollToLatest()
    const frame = window.requestAnimationFrame(scrollToLatest)
    return () => window.cancelAnimationFrame(frame)
  }, [activeTab, displayedLog])

  useEffect(() => {
    if (activeTab !== 'current') return
    const consoleOutput = consoleRef.current
    if (!consoleOutput) return
    const pauseFollowing = () => {
      if (!followConsoleRef.current) return
      followConsoleRef.current = false
      setFollowConsole(false)
    }
    consoleOutput.addEventListener('wheel', pauseFollowing, { passive: true })
    return () => consoleOutput.removeEventListener('wheel', pauseFollowing)
  }, [activeTab, build?.id])

  const setConsoleFollowing = (next: boolean) => {
    followConsoleRef.current = next
    setFollowConsole(next)
    if (!next) return
    const scrollToLatest = () => {
      if (!followConsoleRef.current) return
      const consoleOutput = consoleRef.current
      if (consoleOutput) consoleOutput.scrollTop = consoleOutput.scrollHeight
    }
    scrollToLatest()
    window.requestAnimationFrame(scrollToLatest)
  }

  const selectTab = (tab: BuildDetailTab) => {
    const next = new URLSearchParams(searchParams)
    if (tab === 'current') next.delete('tab')
    else next.set('tab', tab)
    setSearchParams(next, { replace: true })
  }

  const handleTabKeyDown = (event: React.KeyboardEvent<HTMLDivElement>) => {
    if (!['ArrowLeft', 'ArrowRight', 'Home', 'End'].includes(event.key)) return
    event.preventDefault()
    const tabs = Array.from(event.currentTarget.querySelectorAll<HTMLButtonElement>('[role="tab"]'))
    const current = Math.max(0, tabs.indexOf(document.activeElement as HTMLButtonElement))
    const next = event.key === 'Home' ? 0 : event.key === 'End' ? tabs.length - 1 : event.key === 'ArrowRight' ? (current + 1) % tabs.length : (current - 1 + tabs.length) % tabs.length
    const tabOrder: BuildDetailTab[] = ['current', 'problems', 'artifacts']
    const tab = tabOrder[next] || 'current'
    selectTab(tab)
    tabs[next]?.focus()
  }

  const handleStop = async () => {
    if (!await dialogs.confirm(t('builds.stopConfirm'), { title: t('builds.stopTitle'), action: t('builds.stopBuild') })) return
    setStopping(true)
    try {
      await api.stopBuild(buildId)
      reloadBuild()
      reloadTimeline()
      dialogs.notify(t('builds.stopRequested'), 'success')
    } catch (reason: any) {
      dialogs.notify(reason.message || t('common.error'))
    } finally {
      setStopping(false)
    }
  }
  const handleRetry = () => setReplayOpen(true)
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

  const timelineSteps = timeline?.steps || []
  const activeTimelineStep = timelineSteps.find(step => step.status === 'running' || step.status === 'failed' || step.status === 'cancelled')
  const lastStage = activeTimelineStep?.stage || [...logLines].reverse().map(line => line.match(/^\[[^\]]+\] \[([^\]]*)\]/)?.[1]).find(Boolean) || t('builds.waiting')
  const progress = timelineProgress(timeline)
  const progressCopy = t('builds.progress')
    .replace('{completed}', String(timeline?.completed_steps || 0))
    .replace('{total}', String(timeline?.total_steps || 0))
    .replace('{percent}', String(progress))
  const parameters = parseParameters(build.parameters)
  const projectName = buildProject?.name || build.project_name || `${t('builds.project')} #${build.project_id}`
  const currentBuildLabel = locale === 'zh-CN'
    ? `${t('builds.currentBuild')}${t('builds.build')}`
    : `${t('builds.currentBuild')} ${t('builds.build')}`
  const currentBuildErrors = [logsError, timelineError].filter(Boolean)
  const reloadCurrentBuildData = () => {
    reloadLogs()
    reloadTimeline()
  }

  return <>
    <JenkinsHeaderBreadcrumb breadcrumbs={[{ label: projectName, to: `/projects/${build.project_id}` }, { label: `#${build.number}` }]} />
    <section className="jenkins-run-page">
    <div className="jenkins-run-layout">
      <aside className="jenkins-run-side-panel" aria-label={t('builds.build')}>
        <nav className="jenkins-run-tasks">
          <button type="button" className={activeTab === 'current' ? 'active' : ''} onClick={() => selectTab('current')}><Activity />{currentBuildLabel}</button>
          <button type="button" className={activeTab === 'problems' ? 'active' : ''} onClick={() => selectTab('problems')}><AlertTriangle />{t('builds.problems')}</button>
          <button type="button" className={activeTab === 'artifacts' ? 'active' : ''} onClick={() => selectTab('artifacts')}><Package />{t('builds.artifacts')}</button>
          <Link to={`/projects/${build.project_id}/changes?build=${buildId}`}><GitCommitHorizontal />{t('projectDetail.changes')}</Link>
          <Link to={`/builds/${buildId}/logs`}><FileText />{t('builds.logs')}</Link>
          <Link to={`/builds/${buildId}/tests`}><FlaskConical />{t('builds.testReports')}</Link>
          {editable && isFinished && <button type="button" onClick={handleRetry} disabled={retrying || replayOpen} aria-busy={retrying}>{retrying ? <LoaderCircle className="timeline-spinner" /> : <RotateCcw />}{retrying ? t('builds.replaying') : t('builds.replay')}</button>}
          {editable && <button type="button" onClick={handlePin} disabled={pinning} aria-busy={pinning}>{pinning ? <LoaderCircle className="timeline-spinner" /> : build.pinned ? <PinOff /> : <Pin />}{build.pinned ? t('builds.unpin') : t('builds.pin')}</button>}
          {editable && build.status === 'failed' && <Link to={`/projects/${build.project_id}/configure`}><Settings2 />{t('builds.fixProjectSettings')}</Link>}
          {editable && isActive && <button type="button" className="danger" onClick={handleStop} disabled={stopping} aria-busy={stopping}>{stopping ? <LoaderCircle className="timeline-spinner" /> : <Square />}{stopping ? t('builds.stopping') : t('builds.stopBuild')}</button>}
        </nav>

        <section className="jenkins-run-side-summary">
          <h2>{t('builds.build')} #{build.number}</h2>
          <dl>
            <div><dt>{t('builds.currentStage')}</dt><dd>{lastStage}</dd></div>
            <div><dt>{t('builds.outputLines')}</dt><dd>{logLines.length}</dd></div>
            <div><dt>{t('builds.duration')}</dt><dd>{formatDuration(build.duration_ms)}</dd></div>
            <div><dt>{t('builds.trigger')}</dt><dd>{buildTriggerLabel(t, build.trigger)}</dd></div>
          </dl>
        </section>
      </aside>

      <div className="jenkins-run-main" id="overview">
        <header className="jenkins-run-caption">
          <div className="jenkins-run-caption-identity">
            <BuildStatusBadge status={build.status} label={t(`builds.${build.status}`)} />
            <h1>{build.pinned && <Pin size={17} aria-label={t('builds.pinned')} />}{t('builds.build')} #{build.number}<small>({formatTime(build.started_at)})</small></h1>
          </div>
          <div className="jenkins-run-controls">
            <Link to={`/builds/${buildId}/tests`}><FlaskConical size={15} />{t('builds.testReports')}</Link>
            {editable && isFinished && <button type="button" onClick={handleRetry} disabled={retrying || replayOpen} aria-busy={retrying}>{retrying ? <LoaderCircle className="timeline-spinner" size={15} /> : <RotateCcw size={15} />}{retrying ? t('builds.replaying') : t('builds.replay')}</button>}
            {editable && isActive && <button type="button" className="danger" onClick={handleStop} disabled={stopping} aria-busy={stopping}>{stopping ? <LoaderCircle className="timeline-spinner" size={15} /> : <Square size={14} />}{stopping ? t('builds.stopping') : t('builds.stopBuild')}</button>}
          </div>
        </header>

        <p className="jenkins-run-description"><strong>{t(`builds.${build.status}`)}</strong>{isExecuting ? ` · ${liveLogStatus}` : ` · ${t('builds.outputComplete')}`}</p>

        <div className="jenkins-run-tabs" role="tablist" aria-label={t('builds.build')} onKeyDown={handleTabKeyDown}>
          <button type="button" role="tab" id="build-tab-current" aria-controls="build-panel-current" aria-selected={activeTab === 'current'} tabIndex={activeTab === 'current' ? 0 : -1} className={activeTab === 'current' ? 'active' : ''} onClick={() => selectTab('current')}><Activity size={15} />{currentBuildLabel}</button>
          <button type="button" role="tab" id="build-tab-problems" aria-controls="build-panel-problems" aria-selected={activeTab === 'problems'} tabIndex={activeTab === 'problems' ? 0 : -1} className={activeTab === 'problems' ? 'active' : ''} onClick={() => selectTab('problems')}><AlertTriangle size={15} />{t('builds.problems')}{problems && <span>{problems.problems.length}</span>}</button>
          <button type="button" role="tab" id="build-tab-artifacts" aria-controls="build-panel-artifacts" aria-selected={activeTab === 'artifacts'} tabIndex={activeTab === 'artifacts' ? 0 : -1} className={activeTab === 'artifacts' ? 'active' : ''} onClick={() => selectTab('artifacts')}><Package size={15} />{t('builds.artifacts')}<span>{(artifacts || []).length}</span></button>
        </div>

        <div className="jenkins-run-tab-panel" id="build-panel-current" role="tabpanel" aria-labelledby="build-tab-current" hidden={activeTab !== 'current'}>
        {currentBuildErrors.length > 0 && <div className="jenkins-run-data-warning" role="alert">
          <span>{currentBuildErrors[0]}</span>
          <button type="button" onClick={reloadCurrentBuildData}>{t('common.retry')}</button>
        </div>}

        <section className="jenkins-run-facts" aria-label={t('builds.build')}>
          <article><span>{t('builds.duration')}</span><strong>{formatDuration(build.duration_ms)}</strong></article>
          <article><span>{t('builds.branch')}</span><strong><GitBranch size={14} />{build.branch || '-'}</strong></article>
          <article><span>{t('builds.commit')}</span><strong className="mono"><GitCommitHorizontal size={14} />{build.commit_sha?.slice(0, 8) || '-'}</strong></article>
          <article><span>{t('builds.trigger')}</span><strong>{buildTriggerLabel(t, build.trigger)}</strong></article>
          <article><span>{t('builds.started')}</span><strong>{formatTime(build.started_at)}</strong></article>
          <article><span>{t('builds.finished')}</span><strong>{formatTime(build.finished_at)}</strong></article>
          {(build.agent_id || build.agent_requirements) && <article><span>{t('builds.agent')}</span><strong><UserRound size={14} />{build.agent_id ? `#${build.agent_id}` : t('builds.ruleBased')}</strong></article>}
          {build.retried_from && <article><span>{t('builds.retriedFrom')}</span><strong><Link to={`/builds/${build.retried_from}`}>ID {build.retried_from}</Link></strong></article>}
          {build.wait_dependency_on && <article><span>{t('builds.waitingForBuild')}</span><strong><Link to={`/builds/${build.wait_dependency_on}`}>ID {build.wait_dependency_on}</Link></strong></article>}
        </section>

        {build.status === 'pending_approval' && build.approval && <BuildApprovalPanel buildId={buildId} approval={build.approval} onResolved={() => { reloadBuild(); reloadTimeline(); reloadLogs() }} />}

        {parameters.length > 0 && <section className="detail-panel build-parameters-panel jenkins-run-section">
          <header><div><SlidersHorizontal size={17} /><h2>{t('builds.parameters')}</h2><span>{parameters.length}</span></div></header>
          <dl>{parameters.map(([name, value]) => <div key={name}><dt>{name}</dt><dd><code>{parameterValue(name, value)}</code></dd></div>)}</dl>
        </section>}

        <section className="build-timeline jenkins-run-section" aria-label={t('builds.timeline')}>
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

        <section className="jenkins-run-section jenkins-console-section" id="console">
          <header className="jenkins-run-section-heading">
            <h2><FileText size={20} />{t('builds.logs')}</h2>
            <div className="jenkins-console-controls">
              {isActive && <button type="button" className={followConsole ? 'selected' : ''} aria-pressed={followConsole} onClick={() => setConsoleFollowing(!followConsole)}>{followConsole ? <Pause size={14} /> : <Play size={14} />}{followConsole ? t('builds.pauseFollow') : t('builds.resumeFollow')}</button>}
              <button type="button" className={colorizeLogs ? 'selected' : ''} aria-label={t('builds.colorizeLogs')} title={t('builds.colorizeLogs')} aria-pressed={colorizeLogs} onClick={() => setColorizeLogs(current => { const next = !current; writeLogTonePreference(next); return next })}><Palette size={14} />{t('builds.colorizeLogs')}</button>
              <button type="button" onClick={() => handleLogDownload('txt')} disabled={downloading !== null} aria-busy={downloading === 'logs-txt'}>{downloading === 'logs-txt' ? <LoaderCircle className="timeline-spinner" size={15} /> : <Download size={15} />}{downloading === 'logs-txt' ? t('builds.downloading') : t('builds.downloadText')}</button>
              <Link to={`/builds/${buildId}/logs`} target="_blank" rel="noopener noreferrer"><ExternalLink size={14} />{t('builds.openStandaloneLogs')}</Link>
            </div>
          </header>
          {logsResp?.truncated && <div className="jenkins-console-retention-warning" role="status">{t('builds.logTruncated').replace('{count}', String(logsResp.retention_characters || 1_000_000))}</div>}
          <pre ref={consoleRef} className={`jenkins-console-output ${colorizeLogs ? 'log-tones-enabled' : ''}`} id="out" role="region" aria-label={t('builds.logs')} tabIndex={0} onScroll={() => {
            const consoleOutput = consoleRef.current
            if (!consoleOutput) return
            if (followConsoleRef.current && !isNearLogBottom(consoleOutput)) setConsoleFollowing(false)
          }}>{consoleLines.length ? consoleLines.map((line, index) => <span className={`jenkins-console-line ${colorizeLogs ? logTone(line) : ''}`} key={index}>{line || '\u00a0'}</span>) : t('builds.noLogs')}</pre>
          {isExecuting && <div className="jenkins-console-progress" role="status" aria-live="polite">{followConsole ? <LoaderCircle className="timeline-spinner" size={16} /> : <Pause size={16} />}{followConsole ? liveLogStatus : t('builds.followPaused')}</div>}
        </section>
        </div>
        <div className="jenkins-run-tab-panel jenkins-problems-tab-panel" id="build-panel-problems" role="tabpanel" aria-labelledby="build-tab-problems" hidden={activeTab !== 'problems'}>
          <BuildProblemsPanel
            buildId={buildId}
            projectId={build.project_id}
            report={problems}
            loading={problemsPending}
            error={problemsError}
            editable={editable}
            retrying={retrying}
            onRetry={handleRetry}
            onReload={reloadProblems}
          />
        </div>
        <div className="jenkins-run-tab-panel jenkins-artifact-tab-panel" id="build-panel-artifacts" role="tabpanel" aria-labelledby="build-tab-artifacts" hidden={activeTab !== 'artifacts'}>
        {artifactsError && <div className="jenkins-run-data-warning" role="alert"><span>{artifactsError}</span><button type="button" onClick={reloadArtifacts}>{t('common.retry')}</button></div>}
        <section className="detail-panel artifacts-panel jenkins-run-section" id="artifacts"><header><div><Package size={17} /><h2>{t('builds.artifacts')}</h2><span>{(artifacts || []).length}</span></div>{editable && <><input ref={fileInputRef} type="file" onChange={handleUpload} disabled={uploading} className="visually-hidden" id="artifact-upload" /><label htmlFor="artifact-upload" className={`jenkins-artifact-upload ${uploading ? 'is-disabled' : ''}`}><Upload size={15} />{uploading ? t('builds.uploading') : t('builds.uploadArtifact')}</label></>}</header>
          {(artifacts || []).length === 0 ? <p className="detail-empty">{t('builds.noArtifacts')}</p> : <div className="operations-table-wrap"><table className="operations-table"><thead><tr><th>{t('builds.name')}</th><th>{t('builds.size')}</th><th>{t('builds.downloads')}</th><th aria-label={t('builds.download')} /></tr></thead><tbody>{(artifacts || []).map((artifact: any) => <tr key={artifact.id}><td><strong>{artifact.name}</strong></td><td className="muted-cell">{formatSize(artifact.size)}</td><td className="muted-cell">{artifact.download_count || artifact.downloads || 0}</td><td><button type="button" onClick={() => handleArtifactDownload(artifact)} disabled={downloading !== null} aria-busy={downloading === `artifact-${artifact.id}`} className="table-link">{downloading === `artifact-${artifact.id}` ? <LoaderCircle className="timeline-spinner" size={14} /> : <Download size={14} />}{downloading === `artifact-${artifact.id}` ? t('builds.downloading') : t('builds.download')}</button></td></tr>)}</tbody></table></div>}
        </section>
        </div>
      </div>
    </div>
    </section>
    {replayOpen && <ReplayBuildDialog
      target={{ id: buildId, number: build.number, projectId: build.project_id, projectName, branch: build.branch, parameters: build.parameters }}
      onBusyChange={setRetrying}
      onClose={() => setReplayOpen(false)}
      onReplayed={replayedID => { setReplayOpen(false); navigate(`/builds/${replayedID}`) }}
    />}
  </>
}
