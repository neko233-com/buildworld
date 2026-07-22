import { describe, expect, it } from 'vitest'
import {
  appendLiveLog,
  isNearLogBottom,
  LIVE_LOG_MAX_CHARACTERS,
  LIVE_LOG_MAX_LINES,
  LIVE_LOG_RETENTION_MARKER,
  mergeLiveLog,
  PERSISTED_LOG_RETENTION_MARKER,
  retainLiveLog,
} from './logFollow'

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

  it('joins an older snapshot to the start of a newer live suffix', () => {
    expect(mergeLiveLog('two\nthree\n', 'one\ntwo\n')).toBe('one\ntwo\nthree\n')
  })

  it('does not let a longer non-overlapping stale REST snapshot replace the live tail', () => {
    const current = '[12:00:03] [watch] live newest\n'
    const snapshot = '[12:00:00] [watch] persisted old one\n[12:00:01] [watch] persisted old two\n'
    const merged = mergeLiveLog(current, snapshot)
    expect(merged).toBe(current)
    expect(merged.endsWith(current)).toBe(true)
  })

  it('treats a stale snapshot in the middle of the current window as idempotent', () => {
    const current = 'oldest\nretained one\nretained two\nlive newest\n'
    const snapshot = 'retained one\nretained two\n'
    expect(mergeLiveLog(current, snapshot)).toBe(current)
    expect(mergeLiveLog(mergeLiveLog(current, snapshot), snapshot)).toBe(current)
  })

  it('starts a WebSocket log record on a new line after a snapshot without a trailing newline', () => {
    expect(appendLiveLog('snapshot line', 'live line\n')).toBe('snapshot line\nlive line\n')
  })

  it('keeps a newer live tail when the persisted snapshot carries a retention marker', () => {
    const current = 'shared\nlive newest\n'
    const snapshot = `${PERSISTED_LOG_RETENTION_MARKER}older\nshared\n`
    const merged = mergeLiveLog(current, snapshot)
    expect(merged).toBe(`${PERSISTED_LOG_RETENTION_MARKER}older\nshared\nlive newest\n`)
  })

  it('bounds an indefinitely open live viewer by characters while retaining newest output', () => {
    const history = 'old\n'.repeat(300_000)
    const retained = appendLiveLog(history, 'newest\n')
    expect(retained.length).toBeLessThanOrEqual(LIVE_LOG_MAX_CHARACTERS)
    expect(retained.startsWith(LIVE_LOG_RETENTION_MARKER)).toBe(true)
    expect(retained.endsWith('newest\n')).toBe(true)
  })

  it('also bounds many short lines and emits the browser marker once', () => {
    const retained = appendLiveLog('x\n'.repeat(LIVE_LOG_MAX_LINES + 5_000), 'newest\n')
    expect(retained.split('\n').length).toBeLessThanOrEqual(LIVE_LOG_MAX_LINES)
    expect(retained.split(LIVE_LOG_RETENTION_MARKER)).toHaveLength(2)
    expect(retained.endsWith('newest\n')).toBe(true)
    expect(appendLiveLog(retained, 'again\n').split(LIVE_LOG_RETENTION_MARKER)).toHaveLength(2)
  })

  it('merges one-million-character non-overlapping snapshots in bounded time', () => {
    const started = performance.now()
    const current = 'a'.repeat(1_000_000)
    const snapshot = 'b'.repeat(1_000_000)
    const merged = mergeLiveLog(current, snapshot)
    expect(merged).toBe(current)
    expect(merged.endsWith(current)).toBe(true)
    expect(performance.now() - started).toBeLessThan(3_000)
  })

  it('finds a near-million-character overlap without rescanning suffixes', () => {
    const overlap = '月'.repeat(900_000)
    const merged = mergeLiveLog(`prefix\n${overlap}`, `${overlap}\nnewest\n`)
    expect(merged.startsWith('prefix\n')).toBe(true)
    expect(merged.endsWith('\nnewest\n')).toBe(true)
  })

  it('honors custom character and line limits together', () => {
    const retained = retainLiveLog('old\n'.repeat(100) + 'newest\n', 200, 12)
    expect(retained.length).toBeLessThanOrEqual(200)
    expect(retained.split('\n').length).toBeLessThanOrEqual(12)
    expect(retained.startsWith(LIVE_LOG_RETENTION_MARKER)).toBe(true)
    expect(retained.endsWith('newest\n')).toBe(true)
  })
})
