import { describe, expect, it } from 'vitest'
import { appendLiveLog, isNearLogBottom, LIVE_LOG_RETENTION_MARKER, mergeLiveLog, retainLiveLog } from './logFollow'

describe('live log following', () => {
  it('recognizes the bottom with a small scroll tolerance', () => {
    expect(isNearLogBottom({ scrollHeight: 1000, scrollTop: 750, clientHeight: 220 })).toBe(true)
    expect(isNearLogBottom({ scrollHeight: 1000, scrollTop: 700, clientHeight: 220 })).toBe(false)
  })

  it('keeps WebSocket output when a stale REST snapshot arrives', () => {
    expect(mergeLiveLog('one\ntwo\nthree\n', 'one\ntwo\n')).toBe('one\ntwo\nthree\n')
    expect(mergeLiveLog('one\n', 'one\ntwo\n')).toBe('one\ntwo\n')
  })

  it('joins a snapshot that continues from the live suffix', () => {
    expect(mergeLiveLog('one\ntwo\n', 'two\nthree\n')).toBe('one\ntwo\nthree\n')
  })

  it('bounds an indefinitely open live viewer while retaining newest output', () => {
    const retained = appendLiveLog('old\n'.repeat(300_000), 'newest\n')
    expect(retained.length).toBeLessThanOrEqual(1_000_000)
    expect(retained.startsWith(LIVE_LOG_RETENTION_MARKER)).toBe(true)
    expect(retained).toContain('newest')

    const withSmallLimit = retainLiveLog('old\n'.repeat(100) + 'newest\n', 200)
    expect(withSmallLimit.startsWith(LIVE_LOG_RETENTION_MARKER)).toBe(true)
    expect(withSmallLimit).toContain('newest')
  })

  it('merges one-million-character non-overlapping snapshots in bounded time', () => {
    const started = performance.now()
    const merged = mergeLiveLog('a'.repeat(1_000_000), 'b'.repeat(1_000_000))
    expect(merged).toBe('b'.repeat(1_000_000))
    expect(performance.now() - started).toBeLessThan(3_000)
  })

  it('finds a near-million-character overlap without rescanning suffixes', () => {
    const overlap = '月'.repeat(900_000)
    const merged = mergeLiveLog(`prefix\n${overlap}`, `${overlap}\nnewest\n`)
    expect(merged.startsWith('prefix\n')).toBe(true)
    expect(merged.endsWith('\nnewest\n')).toBe(true)
    expect(merged.length).toBeLessThanOrEqual(1_000_000)
  })
})
