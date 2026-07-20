import { describe, expect, it } from 'vitest'
import { isNearLogBottom, mergeLiveLog } from './logFollow'

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
})
