export const LOG_FOLLOW_THRESHOLD = 48
export const LIVE_LOG_MAX_CHARACTERS = 1_000_000
export const LIVE_LOG_RETENTION_MARKER = '[buildworld] Earlier output was removed from this browser view; newest live output is still following.\n'

export function isNearLogBottom(viewport: Pick<HTMLElement, 'scrollHeight' | 'scrollTop' | 'clientHeight'>, threshold = LOG_FOLLOW_THRESHOLD) {
  return viewport.scrollHeight - viewport.scrollTop - viewport.clientHeight <= threshold
}

// REST snapshots and WebSocket output can arrive out of order. Never let an
// older snapshot erase lines that have already reached the terminal view.
export function mergeLiveLog(current = '', snapshot = '') {
  if (!current) return retainLiveLog(snapshot)
  if (!snapshot || current === snapshot || current.startsWith(snapshot)) return retainLiveLog(current)
  if (snapshot.startsWith(current)) return retainLiveLog(snapshot)

  const overlap = suffixPrefixOverlap(current, snapshot)
  if (overlap > 0) return retainLiveLog(current + snapshot.slice(overlap))
  return retainLiveLog(snapshot.length >= current.length ? snapshot : current)
}

// Returns the longest suffix of current that is also a prefix of snapshot.
// KMP keeps reconnect merges linear even at the one-million-character viewer
// retention boundary; the previous descending slice/endsWith loop was O(n²).
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
  return retainLiveLog(current + entry)
}

export function retainLiveLog(log = '', limit = LIVE_LOG_MAX_CHARACTERS) {
  if (limit <= 0 || log.length <= limit) return log
  const markerBudget = Math.max(0, limit - LIVE_LOG_RETENTION_MARKER.length)
  let start = log.length - markerBudget
  const boundary = log.charCodeAt(start)
  if (boundary >= 0xDC00 && boundary <= 0xDFFF) start += 1
  const nextLine = log.indexOf('\n', start)
  if (nextLine >= 0 && nextLine+1 < log.length) start = nextLine + 1
  return LIVE_LOG_RETENTION_MARKER + log.slice(start)
}
