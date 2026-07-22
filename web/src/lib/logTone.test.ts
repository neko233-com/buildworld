import { describe, expect, it, vi } from 'vitest'
import { LOG_TONE_STORAGE_KEY, logTone, readLogTonePreference, writeLogTonePreference } from './logTone'

describe('logTone', () => {
  it('detects common error and warning levels and defaults every other line to info', () => {
    expect(logTone('\u001b[31mlevel=ERROR request failed\u001b[0m')).toBe('error')
    expect(logTone('process exited with exit status 2')).toBe('error')
    expect(logTone('[WARN] retry timed out')).toBe('warning')
    expect(logTone('⚠️ using fallback')).toBe('warning')
    expect(logTone('[INFO] server ready')).toBe('info')
    expect(logTone('plain application output')).toBe('info')
    expect(logTone('')).toBe('info')
  })

  it('defaults the shared preference on and persists explicit changes', () => {
    const values = new Map<string, string>()
    const storage = {
      getItem: vi.fn((key: string) => values.get(key) ?? null),
      setItem: vi.fn((key: string, value: string) => { values.set(key, value) }),
    }

    expect(readLogTonePreference(storage)).toBe(true)
    writeLogTonePreference(false, storage)
    expect(storage.setItem).toHaveBeenCalledWith(LOG_TONE_STORAGE_KEY, 'false')
    expect(readLogTonePreference(storage)).toBe(false)
  })
})
