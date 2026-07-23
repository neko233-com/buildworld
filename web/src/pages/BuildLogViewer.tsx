import { useCallback, useEffect, useLayoutEffect, useMemo, useRef, useState } from 'react'
import type { ReactNode } from 'react'
import { Link, Navigate, useParams } from 'react-router-dom'
import { ArrowDown, ArrowLeft, ArrowUp, Download, FileText, LoaderCircle, Moon, Palette, Pause, Play, Search, Sun, WrapText } from 'lucide-react'
import { api } from '../api'
import { useApi } from '../hooks'
import { useBuildLogStream } from '../useBuildLogStream'
import { useI18n } from '../i18n'
import { visibleBuildLog } from '../lib/buildTimeline'
import { isNearLogBottom } from '../lib/logFollow'
import { logTones, readLogTonePreference, writeLogTonePreference } from '../lib/logTone'
import { PageState } from '../components/PageState'

function highlightLine(line: string, query: string) {
  if (!query) return line
  const lowerLine = line.toLocaleLowerCase()
  const lowerQuery = query.toLocaleLowerCase()
  const fragments: ReactNode[] = []
  let cursor = 0
  let match = lowerLine.indexOf(lowerQuery)
  while (match >= 0) {
    if (match > cursor) fragments.push(line.slice(cursor, match))
    fragments.push(<mark key={`${match}-${cursor}`}>{line.slice(match, match + query.length)}</mark>)
    cursor = match + query.length
    match = lowerLine.indexOf(lowerQuery, cursor)
  }
  if (cursor < line.length) fragments.push(line.slice(cursor))
  return fragments
}

const logViewerThemeStorageKey = 'buildworld.logs.theme'
type LogViewerTheme = 'light' | 'dark'

function readLogViewerTheme(): LogViewerTheme {
  return typeof window !== 'undefined' && window.localStorage.getItem(logViewerThemeStorageKey) === 'dark' ? 'dark' : 'light'
}

export default function BuildLogViewer() {
  const { t } = useI18n()
  const { id } = useParams<{ id: string }>()
  const buildID = Number(id)
  const authenticated = !!localStorage.getItem('token')
  const validBuildID = Number.isSafeInteger(buildID) && buildID > 0
  const canLoad = authenticated && validBuildID
  const { data: build, loading: buildLoading, error: buildError, reload: reloadBuild } = useApi(
    () => canLoad ? api.getBuild(buildID) : Promise.resolve(null),
    [buildID, canLoad],
  )
  const { data: logs, loading: logsLoading, error: logsError, reload: reloadLogs } = useApi(
    () => canLoad ? api.getBuildLogs(buildID) : Promise.resolve<{ log: string; truncated?: boolean; retention_characters?: number }>({ log: '' }),
    [buildID, canLoad],
  )
  const [query, setQuery] = useState('')
  const [activeMatch, setActiveMatch] = useState(0)
  const [follow, setFollow] = useState(true)
  const [wrap, setWrap] = useState(false)
  const [colorizeLogs, setColorizeLogs] = useState(readLogTonePreference)
  const [theme, setTheme] = useState<LogViewerTheme>(readLogViewerTheme)
  const [downloading, setDownloading] = useState<'' | 'txt' | 'json'>('')
  const [downloadError, setDownloadError] = useState('')
  const searchRef = useRef<HTMLInputElement>(null)
  const logViewportRef = useRef<HTMLDivElement>(null)
  const lineRefs = useRef(new Map<number, HTMLDivElement>())
  const followRef = useRef(true)
  const wasRunningRef = useRef(false)
  const running = build?.status === 'running'
    || build?.status === 'pending'
    || build?.status === 'queued'
    || build?.status === 'pending_approval'
  const { log: streamLog, state: streamState } = useBuildLogStream({
    buildID,
    enabled: canLoad && running,
    snapshot: logs?.log,
    reloadSnapshot: reloadLogs,
    onBuildStatus: reloadBuild,
  })
  const source = visibleBuildLog(streamLog)
  const lines = useMemo(() => source ? source.split('\n') : [], [source])
  const lineTones = useMemo(() => logTones(lines), [lines])
  const normalizedQuery = query.trim().toLocaleLowerCase()
  const matches = useMemo(() => normalizedQuery
    ? lines.flatMap((line, index) => line.toLocaleLowerCase().includes(normalizedQuery) ? [index] : [])
    : [], [lines, normalizedQuery])

  const scrollToLatest = useCallback(() => {
    if (!followRef.current) return
    const viewport = logViewportRef.current
    if (!viewport) return
    viewport.scrollTop = viewport.scrollHeight
  }, [])

  const setFollowing = useCallback((next: boolean) => {
    const changed = followRef.current !== next
    followRef.current = next
    if (changed) setFollow(next)
    if (changed && next) {
      if (query.trim()) setQuery('')
      requestAnimationFrame(scrollToLatest)
    }
  }, [query, scrollToLatest])

  useEffect(() => {
    if (!running) return
    const timer = window.setInterval(() => {
      reloadBuild()
      if (streamState !== 'live') reloadLogs()
    }, streamState === 'live' ? 5000 : 3000)
    return () => window.clearInterval(timer)
  }, [reloadBuild, reloadLogs, running, streamState])

  useEffect(() => {
    const wasRunning = wasRunningRef.current
    wasRunningRef.current = running
    if (wasRunning && !running && build?.status) reloadLogs()
  }, [build?.status, reloadLogs, running])

  useEffect(() => {
    document.title = build ? `${t('builds.logs')} · #${build.number} · buildworld` : `buildworld · ${t('builds.logs')}`
    return () => { document.title = 'buildworld' }
  }, [build, t])

  useEffect(() => {
    setActiveMatch(0)
  }, [normalizedQuery])

  useLayoutEffect(() => {
    if (normalizedQuery && matches.length) {
      lineRefs.current.get(matches[Math.min(activeMatch, matches.length - 1)])?.scrollIntoView?.({ block: 'center' })
      return
    }
    if (!normalizedQuery && followRef.current) {
      scrollToLatest()
    }
  }, [activeMatch, matches, normalizedQuery, scrollToLatest, source])

  useEffect(() => {
    const viewport = logViewportRef.current
    if (!viewport) return
    const pauseFollowing = () => setFollowing(false)
    viewport.addEventListener('wheel', pauseFollowing, { passive: true })
    return () => viewport.removeEventListener('wheel', pauseFollowing)
  }, [build?.id, setFollowing])

  useEffect(() => {
    const handleKeyboard = (event: KeyboardEvent) => {
      if (event.key === '/' && document.activeElement !== searchRef.current) {
        event.preventDefault()
        searchRef.current?.focus()
      }
      if (event.key === 'Escape' && query) {
        setQuery('')
        searchRef.current?.focus()
      }
      if (event.key === 'End' && !event.ctrlKey && !event.metaKey) {
        event.preventDefault()
        setFollowing(true)
      }
    }
    window.addEventListener('keydown', handleKeyboard)
    return () => window.removeEventListener('keydown', handleKeyboard)
  }, [query, setFollowing])

  useEffect(() => {
    const syncTheme = (event: StorageEvent) => {
      if (event.key === logViewerThemeStorageKey) setTheme(readLogViewerTheme())
    }
    window.addEventListener('storage', syncTheme)
    return () => window.removeEventListener('storage', syncTheme)
  }, [])

  if (!authenticated) return <Navigate to="/login" replace />
  if (!validBuildID) return <Navigate to="/builds" replace />
  if ((buildLoading && !build) || (logsLoading && !logs)) return <div className="plain-log-state"><PageState /></div>
  if (buildError || logsError || !build) return <div className="plain-log-state"><PageState error={buildError || logsError || t('builds.logLoadFailed')} onRetry={() => { reloadBuild(); reloadLogs() }} /></div>

  const moveMatch = (direction: -1 | 1) => {
    if (!matches.length) return
    setActiveMatch(current => (current + direction + matches.length) % matches.length)
  }
  const download = async (format: 'txt' | 'json') => {
    setDownloading(format)
    setDownloadError('')
    try {
      await api.downloadBuildLogs(buildID, format)
    } catch (reason) {
      setDownloadError(reason instanceof Error && reason.message ? reason.message : t('builds.logDownloadFailed'))
    } finally {
      setDownloading('')
    }
  }

  const toggleTheme = () => setTheme(current => {
    const next = current === 'dark' ? 'light' : 'dark'
    window.localStorage.setItem(logViewerThemeStorageKey, next)
    return next
  })

  return <main className={`plain-log-page theme-${theme} ${wrap ? 'wrap-lines' : ''}`}>
    <header className="plain-log-header">
      <div className="plain-log-identity">
        <Link to={`/builds/${buildID}`} aria-label={t('builds.backToBuild')}><ArrowLeft size={17} /></Link>
        <span className="plain-log-mark"><FileText size={17} /></span>
        <div><small>buildworld / {t('builds.build')} #{build.number}</small><h1>{t('builds.standaloneLogs')}</h1></div>
        <span className={`build-status ${build.status}`}>{t(`builds.${build.status}`)}</span>
      </div>
      <div className="plain-log-actions">
        <button type="button" className={theme === 'dark' ? 'selected' : ''} aria-label={theme === 'dark' ? t('builds.useLightTheme') : t('builds.useDarkTheme')} title={theme === 'dark' ? t('builds.useLightTheme') : t('builds.useDarkTheme')} aria-pressed={theme === 'dark'} onClick={toggleTheme}>{theme === 'dark' ? <Sun size={14} /> : <Moon size={14} />}{theme === 'dark' ? t('builds.lightTheme') : t('builds.darkTheme')}</button>
        <button type="button" className={follow ? 'selected' : ''} aria-pressed={follow} onClick={() => setFollowing(!follow)}>{follow ? <Pause size={14} /> : <Play size={14} />}{follow ? t('builds.pauseFollow') : t('builds.resumeFollow')}</button>
        <button type="button" className={wrap ? 'selected' : ''} aria-pressed={wrap} onClick={() => setWrap(value => !value)}><WrapText size={14} />{t('builds.wrapLines')}</button>
        <button type="button" className={colorizeLogs ? 'selected' : ''} aria-label={t('builds.colorizeLogs')} title={t('builds.colorizeLogs')} aria-pressed={colorizeLogs} onClick={() => setColorizeLogs(current => { const next = !current; writeLogTonePreference(next); return next })}><Palette size={14} />{t('builds.colorizeLogs')}</button>
        <button type="button" onClick={() => download('txt')} disabled={!!downloading}>{downloading === 'txt' ? <LoaderCircle className="timeline-spinner" size={14} /> : <Download size={14} />}.txt</button>
        <button type="button" onClick={() => download('json')} disabled={!!downloading}>{downloading === 'json' ? <LoaderCircle className="timeline-spinner" size={14} /> : <Download size={14} />}JSON</button>
      </div>
    </header>
    <div className="plain-log-message">
      {logs?.truncated && <span className="retention-warning" role="status">{t('builds.logTruncated').replace('{count}', String(logs.retention_characters || 1_000_000))}</span>}
      {downloadError && <span role="alert">{downloadError}</span>}
    </div>

    <section className="plain-log-toolbar" aria-label={t('builds.logTools')}>
      <label><Search size={15} /><input ref={searchRef} value={query} onChange={event => { setQuery(event.target.value); if (event.target.value.trim()) setFollowing(false) }} placeholder={t('builds.searchLogs')} aria-label={t('builds.searchLogs')} /><kbd>/</kbd></label>
      <span>{normalizedQuery ? t('builds.matchProgress').replace('{current}', matches.length ? String(activeMatch + 1) : '0').replace('{total}', String(matches.length)) : t('builds.lineCount').replace('{count}', String(lines.length))}</span>
      <button type="button" aria-label={t('builds.previousMatch')} title={t('builds.previousMatch')} disabled={!matches.length} onClick={() => moveMatch(-1)}><ArrowUp size={14} /></button>
      <button type="button" aria-label={t('builds.nextMatch')} title={t('builds.nextMatch')} disabled={!matches.length} onClick={() => moveMatch(1)}><ArrowDown size={14} /></button>
      {running && <strong role="status" aria-live="polite" aria-atomic="true"><i />{t(`builds.${streamState}`)}</strong>}
    </section>

    <div className="plain-log-viewport" ref={logViewportRef} role="region" aria-label={t('builds.logs')} tabIndex={0} onScroll={() => {
      const viewport = logViewportRef.current
      if (viewport && followRef.current && !isNearLogBottom(viewport)) setFollowing(false)
    }}>
      {lines.length ? <div className="plain-log-lines">
        {lines.map((line, index) => <div key={index} ref={element => { if (element) lineRefs.current.set(index, element); else lineRefs.current.delete(index) }} className={`${colorizeLogs ? lineTones[index] : ''} ${matches[activeMatch] === index ? 'active-match' : ''}`}><span>{index + 1}</span><code>{highlightLine(line, normalizedQuery)}</code></div>)}
      </div> : <div className="plain-log-empty"><FileText size={22} /><span>{running ? t('builds.waitingForLogs') : t('builds.noLogs')}</span></div>}
    </div>
    <footer className="plain-log-footer"><span>{t('builds.outputLines')}: {lines.length}</span><span>{follow ? t('builds.followingOutput') : t('builds.followPaused')}</span><span>UTF-8</span></footer>
  </main>
}
