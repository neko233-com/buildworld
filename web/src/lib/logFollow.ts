export const LOG_FOLLOW_THRESHOLD = 48
export const LIVE_LOG_MAX_CHARACTERS = 1_000_000
export const LIVE_LOG_MAX_LINES = 20_000
export const LIVE_LOG_RETENTION_MARKER = '[buildworld] Earlier output was removed from this browser view; newest live output is still following.\n'
export const PERSISTED_LOG_RETENTION_MARKER = '[buildworld] Earlier persisted log output was truncated; only the newest output is retained. Live WebSocket output was not truncated.\n'

function splitRetentionMarkers(log: string) {
  let offset = 0
  let browserTruncated = false
  let persistedTruncated = false
  while (offset < log.length) {
    if (log.startsWith(LIVE_LOG_RETENTION_MARKER, offset)) {
      browserTruncated = true
      offset += LIVE_LOG_RETENTION_MARKER.length
      continue
    }
    if (log.startsWith(PERSISTED_LOG_RETENTION_MARKER, offset)) {
      persistedTruncated = true
      offset += PERSISTED_LOG_RETENTION_MARKER.length
      continue
    }
    break
  }
  return { body: log.slice(offset), browserTruncated, persistedTruncated }
}

function joinLogSegments(older: string, newer: string) {
  if (!older) return newer
  if (!newer) return older
  const separator = older.endsWith('\n') || newer.startsWith('\n') ? '' : '\n'
  return older + separator + newer
}

function restoreRetentionMarkers(
  body: string,
  ...windows: ReturnType<typeof splitRetentionMarkers>[]
) {
  const browserMarker = windows.some(window => window.browserTruncated) ? LIVE_LOG_RETENTION_MARKER : ''
  const persistedMarker = windows.some(window => window.persistedTruncated) ? PERSISTED_LOG_RETENTION_MARKER : ''
  return retainLiveLog(browserMarker + persistedMarker + body)
}

function containsLinear(haystack: string, needle: string) {
  if (!needle) return true
  if (needle.length > haystack.length) return false
  const prefix = new Int32Array(needle.length)
  for (let index = 1, matched = 0; index < needle.length; index += 1) {
    const code = needle.charCodeAt(index)
    while (matched > 0 && code !== needle.charCodeAt(matched)) matched = prefix[matched - 1]
    if (code === needle.charCodeAt(matched)) matched += 1
    prefix[index] = matched
  }
  for (let index = 0, matched = 0; index < haystack.length; index += 1) {
    const code = haystack.charCodeAt(index)
    while (matched > 0 && code !== needle.charCodeAt(matched)) matched = prefix[matched - 1]
    if (code === needle.charCodeAt(matched)) matched += 1
    if (matched === needle.length) return true
  }
  return false
}

export function isNearLogBottom(viewport: Pick<HTMLElement, 'scrollHeight' | 'scrollTop' | 'clientHeight'>, threshold = LOG_FOLLOW_THRESHOLD) {
  return viewport.scrollHeight - viewport.scrollTop - viewport.clientHeight <= threshold
}

// REST snapshots and WebSocket output can arrive out of order. Never let an
// older snapshot erase lines that have already reached the terminal view.
export function mergeLiveLog(current = '', snapshot = '') {
  if (!current) return retainLiveLog(snapshot)
  if (!snapshot || current === snapshot) return retainLiveLog(current)

  // SQLite snapshots are append-only but can lag behind the WebSocket because
  // the runner broadcasts each line before asynchronously persisting it. Work
  // on the real output windows so a retention marker cannot hide an overlap.
  const currentWindow = splitRetentionMarkers(current)
  const snapshotWindow = splitRetentionMarkers(snapshot)
  const currentBody = currentWindow.body
  const snapshotBody = snapshotWindow.body

  if (currentBody === snapshotBody || currentBody.startsWith(snapshotBody)) return retainLiveLog(current)
  if (snapshotBody.startsWith(currentBody)) {
    return restoreRetentionMarkers(snapshotBody, currentWindow, snapshotWindow)
  }
  // A retained REST window may sit wholly inside the longer browser window.
  // Treat that snapshot as idempotent instead of prepending a duplicate copy.
  if (containsLinear(currentBody, snapshotBody)) return retainLiveLog(current)

  // Check both directions. A fresh snapshot can continue after the current
  // view, while a stale snapshot can end where the newer WebSocket tail begins.
  const currentToSnapshot = suffixPrefixOverlap(currentBody, snapshotBody)
  if (currentToSnapshot > 0) {
    return restoreRetentionMarkers(currentBody + snapshotBody.slice(currentToSnapshot), currentWindow, snapshotWindow)
  }
  const snapshotToCurrent = suffixPrefixOverlap(snapshotBody, currentBody)
  if (snapshotToCurrent > 0) {
    return restoreRetentionMarkers(snapshotBody + currentBody.slice(snapshotToCurrent), currentWindow, snapshotWindow)
  }

  // With no reliable relationship, replacement or concatenation is unsafe:
  // a stale response must neither erase nor duplicate the newest live tail.
  return retainLiveLog(current)
}

// Extract only output that appeared after a known browser-side cursor. A
// disconnected viewer may receive a complete REST snapshot after the screen
// was cleared, so returning null is safer than showing that snapshot again
// when no reliable overlap can be proven.
export function extractLiveLogDelta(previous = '', next = ''): string | null {
  if (!next || next === previous) return ''
  if (!previous) return next
  if (next.startsWith(previous)) return next.slice(previous.length)

  const previousIndex = next.indexOf(previous)
  if (previousIndex >= 0) return next.slice(previousIndex + previous.length)
  if (previous.startsWith(next) || containsLinear(previous, next)) return ''

  const overlap = suffixPrefixOverlap(previous, next)
  return overlap > 0 && next.charCodeAt(overlap - 1) === 10 ? next.slice(overlap) : null
}

// Returns the longest suffix of current that is also a prefix of snapshot.
// KMP keeps reconnect merges linear even for very large viewer windows; the
// previous descending slice/endsWith loop was O(n²).
export function suffixPrefixOverlap(current: string, snapshot: string) {
  const maximum = Math.min(current.length, snapshot.length)
  if (maximum === 0) return 0
  const prefix = new Int32Array(maximum)
  for (let index = 1, matched = 0; index < maximum; index += 1) {
    const code = snapshot.charCodeAt(index)
    while (matched > 0 && code !== snapshot.charCodeAt(matched)) matched = prefix[matched - 1]
    if (code === snapshot.charCodeAt(matched)) matched += 1
    prefix[index] = matched
  }

  let matched = 0
  const start = current.length - maximum
  const finalIndex = current.length - 1
  for (let index = start; index <= finalIndex; index += 1) {
    const code = current.charCodeAt(index)
    while (matched > 0 && code !== snapshot.charCodeAt(matched)) matched = prefix[matched - 1]
    if (code === snapshot.charCodeAt(matched)) matched += 1
    if (matched === maximum && index !== finalIndex) matched = prefix[matched - 1]
  }
  return matched
}

export function appendLiveLog(current = '', entry = '') {
  return retainLiveLog(joinLogSegments(current, entry))
}

function retainedLineStart(log: string, maximumLines: number) {
  if (!log) return 0
  let lines = 1
  for (let index = log.length - 1; index >= 0; index -= 1) {
    if (log.charCodeAt(index) !== 10) continue
    lines += 1
    if (lines > maximumLines) return index + 1
  }
  return 0
}

export function retainLiveLog(
  log = '',
  characterLimit = LIVE_LOG_MAX_CHARACTERS,
  lineLimit = LIVE_LOG_MAX_LINES,
) {
  if (!log || characterLimit <= 0 || lineLimit <= 0) return ''

  const window = splitRetentionMarkers(log)
  const body = (window.persistedTruncated ? PERSISTED_LOG_RETENTION_MARKER : '') + window.body
  const unmarkedLineStart = retainedLineStart(body, lineLimit)
  const needsMarker = window.browserTruncated || body.length > characterLimit || unmarkedLineStart > 0
  if (!needsMarker) return body

  if (characterLimit <= LIVE_LOG_RETENTION_MARKER.length || lineLimit === 1) {
    return LIVE_LOG_RETENTION_MARKER.slice(0, characterLimit)
  }

  const bodyCharacterLimit = characterLimit - LIVE_LOG_RETENTION_MARKER.length
  const bodyLineLimit = lineLimit - 1
  const characterStart = Math.max(0, body.length - bodyCharacterLimit)
  const lineStart = retainedLineStart(body, bodyLineLimit)
  let start = Math.max(characterStart, lineStart)
  const boundary = body.charCodeAt(start)
  if (boundary >= 0xDC00 && boundary <= 0xDFFF) start += 1
  if (start > 0 && body.charCodeAt(start - 1) !== 10) {
    const nextLine = body.indexOf('\n', start)
    if (nextLine >= 0 && nextLine + 1 < body.length) start = nextLine + 1
  }
  return LIVE_LOG_RETENTION_MARKER + body.slice(start)
}
