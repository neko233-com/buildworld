import { useEffect, useMemo, useRef, useState } from 'react'
import type { ReactNode } from 'react'
import { motion } from 'motion/react'
import { Link, Navigate, useParams } from 'react-router-dom'
import { ArrowDown, ArrowLeft, ArrowUp, Download, FileText, LoaderCircle, Pause, Play, Search, WrapText } from 'lucide-react'
import { api } from '../api'
import { useApi } from '../hooks'
import { useI18n } from '../i18n'
import { visibleBuildLog } from '../lib/buildTimeline'
import { PageState } from '../components/PageState'

function lineTone(line: string) {
  if (/BUILD FAILED|ERROR:|FAILED/i.test(line)) return 'error'
  if (/WARN|CANCELLED|timed out/i.test(line)) return 'warning'
  if (/=== Stage:|--- Step:/i.test(line)) return 'boundary'
  if (/succeeded|complete/i.test(line)) return 'success'
  return ''
}

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

type StreamState = 'connecting' | 'live' | 'reconnecting' | 'fallback'

function formatStreamLog(payload: unknown): string {
  if (!payload || typeof payload !== 'object') return ''
  const entry = payload as Record<string, unknown>
  const timestamp = typeof entry.timestamp === 'string' ? entry.timestamp : ''
  const stage = typeof entry.stage === 'string' ? entry.stage : ''
  const line = typeof entry.line === 'string' ? entry.line : ''
  if (!line) return ''
  return `[${timestamp}] [${stage}] ${line}\n`
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
    () => canLoad ? api.getBuildLogs(buildID) : Promise.resolve({ log: '' }),
    [buildID, canLoad],
  )
  const [query, setQuery] = useState('')
  const [activeMatch, setActiveMatch] = useState(0)
  const [follow, setFollow] = useState(true)
  const [wrap, setWrap] = useState(false)
  const [streamState, setStreamState] = useState<StreamState>('fallback')
  const [streamLog, setStreamLog] = useState('')
  const [downloading, setDownloading] = useState<'' | 'txt' | 'json'>('')
  const [downloadError, setDownloadError] = useState('')
  const searchRef = useRef<HTMLInputElement>(null)
  const logViewportRef = useRef<HTMLDivElement>(null)
  const lineRefs = useRef(new Map<number, HTMLDivElement>())
  const running = build?.status === 'running' || build?.status === 'pending'
  const source = visibleBuildLog(streamLog)
  const lines = useMemo(() => source ? source.split('\n') : [], [source])
  const normalizedQuery = query.trim().toLocaleLowerCase()
  const matches = useMemo(() => normalizedQuery
    ? lines.flatMap((line, index) => line.toLocaleLowerCase().includes(normalizedQuery) ? [index] : [])
    : [], [lines, normalizedQuery])

  useEffect(() => {
    setStreamLog(logs?.log || '')
  }, [logs?.log])

  useEffect(() => {
    if (!running || !validBuildID || typeof WebSocket === 'undefined') {
      setStreamState('fallback')
      return
    }
    let disposed = false
    let socket: WebSocket | undefined
    let retryTimer: number | undefined
    let attempts = 0
    const connect = () => {
      if (disposed) return
      setStreamState(attempts ? 'reconnecting' : 'connecting')
      const scheme = window.location.protocol === 'https:' ? 'wss:' : 'ws:'
      const connection = new WebSocket(`${scheme}//${window.location.host}/ws?room=build:${buildID}`)
      socket = connection
      connection.onopen = () => {
        attempts = 0
        setStreamState('live')
        // The REST snapshot closes the short connection race before the socket opened.
        reloadLogs()
      }
      connection.onmessage = event => {
        try {
          const message = JSON.parse(String(event.data)) as { type?: string, payload?: unknown }
          if (message.type === 'build:log') {
            const entry = formatStreamLog(message.payload)
            if (entry) setStreamLog(current => current + entry)
          } else if (message.type === 'build:status') {
            reloadBuild()
          }
        } catch {
          // Ignore malformed socket messages; polling remains the safe fallback.
        }
      }
      connection.onerror = () => connection.close()
      connection.onclose = () => {
        if (disposed) return
        setStreamState('fallback')
        const delay = Math.min(5000, 1000 * 2 ** attempts)
        attempts += 1
        retryTimer = window.setTimeout(connect, delay)
      }
    }
    connect()
    return () => {
      disposed = true
      if (retryTimer) window.clearTimeout(retryTimer)
      socket?.close()
    }
  }, [buildID, reloadBuild, reloadLogs, running, validBuildID])

  useEffect(() => {
    if (!running || streamState === 'live') return
    const timer = window.setInterval(() => {
      reloadBuild()
      reloadLogs()
    }, 3000)
    return () => window.clearInterval(timer)
  }, [reloadBuild, reloadLogs, running, streamState])

  useEffect(() => {
    document.title = build ? `${t('builds.logs')} · #${build.number} · buildworld` : `buildworld · ${t('builds.logs')}`
    return () => { document.title = 'buildworld' }
  }, [build, t])

  useEffect(() => {
    setActiveMatch(0)
  }, [normalizedQuery])

  useEffect(() => {
    if (normalizedQuery && matches.length) {
      lineRefs.current.get(matches[Math.min(activeMatch, matches.length - 1)])?.scrollIntoView?.({ block: 'center' })
      return
    }
    if (follow && logViewportRef.current) {
      logViewportRef.current.scrollTop = logViewportRef.current.scrollHeight
    }
  }, [activeMatch, follow, lines.length, matches, normalizedQuery])

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
        setFollow(true)
      }
    }
    window.addEventListener('keydown', handleKeyboard)
    return () => window.removeEventListener('keydown', handleKeyboard)
  }, [query])

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

  return <motion.main className={`plain-log-page ${wrap ? 'wrap-lines' : ''}`} initial={{ opacity: 0 }} animate={{ opacity: 1 }}>
    <header className="plain-log-header">
      <div className="plain-log-identity">
        <Link to={`/builds/${buildID}`} aria-label={t('builds.backToBuild')}><ArrowLeft size={17} /></Link>
        <span className="plain-log-mark"><FileText size={17} /></span>
        <div><small>buildworld / {t('builds.build')} #{build.number}</small><h1>{t('builds.standaloneLogs')}</h1></div>
        <span className={`build-status ${build.status}`}>{t(`builds.${build.status}`)}</span>
      </div>
      <div className="plain-log-actions">
        <button type="button" className={follow ? 'selected' : ''} aria-pressed={follow} onClick={() => setFollow(value => !value)}>{follow ? <Pause size={14} /> : <Play size={14} />}{follow ? t('builds.pauseFollow') : t('builds.resumeFollow')}</button>
        <button type="button" className={wrap ? 'selected' : ''} aria-pressed={wrap} onClick={() => setWrap(value => !value)}><WrapText size={14} />{t('builds.wrapLines')}</button>
        <button type="button" onClick={() => download('txt')} disabled={!!downloading}>{downloading === 'txt' ? <LoaderCircle className="timeline-spinner" size={14} /> : <Download size={14} />}.txt</button>
        <button type="button" onClick={() => download('json')} disabled={!!downloading}>{downloading === 'json' ? <LoaderCircle className="timeline-spinner" size={14} /> : <Download size={14} />}JSON</button>
      </div>
    </header>
    <div className="plain-log-message" aria-live="polite">{downloadError && <span role="alert">{downloadError}</span>}</div>

    <section className="plain-log-toolbar" aria-label={t('builds.logTools')}>
      <label><Search size={15} /><input ref={searchRef} value={query} onChange={event => setQuery(event.target.value)} placeholder={t('builds.searchLogs')} aria-label={t('builds.searchLogs')} /><kbd>/</kbd></label>
      <span>{normalizedQuery ? t('builds.matchProgress').replace('{current}', matches.length ? String(activeMatch + 1) : '0').replace('{total}', String(matches.length)) : t('builds.lineCount').replace('{count}', String(lines.length))}</span>
      <button type="button" aria-label={t('builds.previousMatch')} title={t('builds.previousMatch')} disabled={!matches.length} onClick={() => moveMatch(-1)}><ArrowUp size={14} /></button>
      <button type="button" aria-label={t('builds.nextMatch')} title={t('builds.nextMatch')} disabled={!matches.length} onClick={() => moveMatch(1)}><ArrowDown size={14} /></button>
      {running && <strong><i />{t(`builds.${streamState}`)}</strong>}
    </section>

    <div className="plain-log-viewport" ref={logViewportRef} onScroll={() => {
      const viewport = logViewportRef.current
      if (viewport && viewport.scrollHeight - viewport.scrollTop - viewport.clientHeight > 48) setFollow(false)
    }}>
      {lines.length ? <div className="plain-log-lines" role="log" aria-live={running ? 'polite' : 'off'}>
        {lines.map((line, index) => <div key={index} ref={element => { if (element) lineRefs.current.set(index, element); else lineRefs.current.delete(index) }} className={`${lineTone(line)} ${matches[activeMatch] === index ? 'active-match' : ''}`}><span>{index + 1}</span><code>{highlightLine(line, normalizedQuery)}</code></div>)}
      </div> : <div className="plain-log-empty"><FileText size={22} /><span>{running ? t('builds.waitingForLogs') : t('builds.noLogs')}</span></div>}
    </div>
    <footer className="plain-log-footer"><span>{t('builds.outputLines')}: {lines.length}</span><span>{follow ? t('builds.followingOutput') : t('builds.followPaused')}</span><span>UTF-8</span></footer>
  </motion.main>
}
