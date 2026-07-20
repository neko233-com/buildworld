export const LOG_FOLLOW_THRESHOLD = 48

export function isNearLogBottom(viewport: Pick<HTMLElement, 'scrollHeight' | 'scrollTop' | 'clientHeight'>, threshold = LOG_FOLLOW_THRESHOLD) {
  return viewport.scrollHeight - viewport.scrollTop - viewport.clientHeight <= threshold
}

// REST snapshots and WebSocket output can arrive out of order. Never let an
// older snapshot erase lines that have already reached the terminal view.
export function mergeLiveLog(current = '', snapshot = '') {
  if (!current) return snapshot
  if (!snapshot || current === snapshot || current.includes(snapshot)) return current
  if (snapshot.includes(current)) return snapshot

  const maximum = Math.min(current.length, snapshot.length)
  for (let size = maximum; size > 0; size -= 1) {
    if (current.endsWith(snapshot.slice(0, size))) return current + snapshot.slice(size)
  }
  return snapshot.length >= current.length ? snapshot : current
}
