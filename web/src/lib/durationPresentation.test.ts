import { describe, expect, it } from 'vitest'
import { formatDuration } from './durationPresentation'

describe('formatDuration', () => {
  it('keeps fast builds precise and formats longer durations compactly', () => {
    expect(formatDuration(49.6)).toBe('50 ms')
    expect(formatDuration(1_250)).toBe('1.25s')
    expect(formatDuration(69_200)).toBe('1m 9s')
    expect(formatDuration(3_720_000)).toBe('1h 2m')
  })

  it('distinguishes a real zero duration from missing data', () => {
    expect(formatDuration(0)).toBe('0 ms')
    expect(formatDuration()).toBe('-')
  })
})
