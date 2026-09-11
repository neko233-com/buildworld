import { useCallback, useEffect, useRef, useState } from 'react'
import { appendLiveLog, BROWSER_LIVE_LOG_MAX_CHARACTERS, BROWSER_LIVE_LOG_MAX_LINES, extractLiveLogDelta, mergeLiveLog } from './lib/logFollow'

export type BuildLogStreamState = 'connecting' | 'live' | 'reconnecting' | 'fallback'
const LIVE_LOG_BATCH_INTERVAL_MS = 75

type UseBuildLogStreamOptions = {
  buildID: number
  enabled: boolean
  snapshot?: string
  reloadSnapshot: () => void
  onBuildStatus: () => void
}

function formatStreamLog(payload: unknown): string {
  if (!payload || typeof payload !== 'object') return ''
  const entry = payload as Record<string, unknown>
  const timestamp = typeof entry.timestamp === 'string' ? entry.timestamp : ''
  const stage = typeof entry.stage === 'string' ? entry.stage : ''
  const line = typeof entry.line === 'string' ? entry.line : ''
  if (!line) return ''
  return `[${timestamp}] [${stage}] ${line}\n`
}

// One shared implementation powers both the embedded console and the
// standalone viewer. REST closes connection races; WebSocket messages append
// in short render batches and reconnect without replacing newer browser output.
export function useBuildLogStream({
  buildID,
  enabled,
  snapshot = '',
  reloadSnapshot,
  onBuildStatus,
}: UseBuildLogStreamOptions) {
  const [log, setLog] = useState('')
  const [state, setState] = useState<BuildLogStreamState>('fallback')
  const logRef = useRef('')
  const snapshotBuildID = useRef(buildID)
  const clearActiveRef = useRef(false)
  const snapshotCursorRef = useRef<string | null>(null)

  const clearLog = useCallback(() => {
    snapshotCursorRef.current = logRef.current
    clearActiveRef.current = true
    logRef.current = ''
    setLog('')
  }, [])

  useEffect(() => {
    // useApi intentionally keeps the previous response during route changes.
    // Ignore that one stale snapshot rather than flashing another build's log.
    if (snapshotBuildID.current !== buildID) {
      snapshotBuildID.current = buildID
      logRef.current = ''
      clearActiveRef.current = false
      snapshotCursorRef.current = null
      setLog('')
      return
    }
    if (clearActiveRef.current) {
      const cursor = snapshotCursorRef.current || ''
      const delta = extractLiveLogDelta(cursor, snapshot)
      if (delta === null) return
      snapshotCursorRef.current = mergeLiveLog(cursor, snapshot, BROWSER_LIVE_LOG_MAX_CHARACTERS, BROWSER_LIVE_LOG_MAX_LINES)
      if (!delta) return
      setLog(current => {
        const next = appendLiveLog(current, delta, BROWSER_LIVE_LOG_MAX_CHARACTERS, BROWSER_LIVE_LOG_MAX_LINES)
        logRef.current = next
        return next
      })
      return
    }
    setLog(current => {
      const next = mergeLiveLog(current, snapshot, BROWSER_LIVE_LOG_MAX_CHARACTERS, BROWSER_LIVE_LOG_MAX_LINES)
      logRef.current = next
      return next
    })
  }, [buildID, snapshot])

  useEffect(() => {
    if (!enabled || !Number.isSafeInteger(buildID) || buildID <= 0 || typeof WebSocket === 'undefined') {
      setState('fallback')
      return
    }

    let disposed = false
    let socket: WebSocket | undefined
    let retryTimer: number | undefined
    let logBatchTimer: number | undefined
    let logBatch: string[] = []
    let attempts = 0

    const flushLogBatch = () => {
      if (logBatchTimer !== undefined) {
        window.clearTimeout(logBatchTimer)
        logBatchTimer = undefined
      }
      if (disposed || logBatch.length === 0) {
        logBatch = []
        return
      }
      const entries = logBatch.join('')
      logBatch = []
      if (clearActiveRef.current && snapshotCursorRef.current !== null) {
        snapshotCursorRef.current = appendLiveLog(snapshotCursorRef.current, entries, BROWSER_LIVE_LOG_MAX_CHARACTERS, BROWSER_LIVE_LOG_MAX_LINES)
      }
      setLog(current => {
        const next = appendLiveLog(current, entries, BROWSER_LIVE_LOG_MAX_CHARACTERS, BROWSER_LIVE_LOG_MAX_LINES)
        logRef.current = next
        return next
      })
    }

    const queueLogEntry = (entry: string) => {
      logBatch.push(entry)
      if (logBatchTimer === undefined) {
        logBatchTimer = window.setTimeout(flushLogBatch, LIVE_LOG_BATCH_INTERVAL_MS)
      }
    }

    const connect = () => {
      if (disposed) return
      setState(attempts ? 'reconnecting' : 'connecting')
      const scheme = window.location.protocol === 'https:' ? 'wss:' : 'ws:'
      const connection = new WebSocket(`${scheme}//${window.location.host}/ws?room=build:${buildID}`)
      socket = connection

      connection.onopen = () => {
        attempts = 0
        setState('live')
        reloadSnapshot()
      }
      connection.onmessage = event => {
        try {
          const message = JSON.parse(String(event.data)) as { type?: string, payload?: unknown }
          if (message.type === 'build:log') {
            const entry = formatStreamLog(message.payload)
            if (entry) queueLogEntry(entry)
          } else if (message.type === 'build:status') {
            // Terminal status is published after the runner flushes its
            // durable log tail. Reconcile once more before this socket closes.
            flushLogBatch()
            reloadSnapshot()
            onBuildStatus()
          }
        } catch {
          // Polling remains the fallback for malformed or interrupted frames.
        }
      }
      connection.onerror = () => connection.close()
      connection.onclose = () => {
        if (disposed) return
        flushLogBatch()
        setState('fallback')
        const delay = Math.min(5000, 1000 * 2 ** attempts)
        attempts += 1
        retryTimer = window.setTimeout(connect, delay)
      }
    }

    connect()
    return () => {
      disposed = true
      if (retryTimer !== undefined) window.clearTimeout(retryTimer)
      if (logBatchTimer !== undefined) window.clearTimeout(logBatchTimer)
      logBatch = []
      socket?.close()
    }
  }, [buildID, enabled, onBuildStatus, reloadSnapshot])

  return { log, state, clearLog }
}
