export type LogTone = 'error' | 'warning' | 'info'

export const LOG_TONE_STORAGE_KEY = 'buildworld.logs.colorize'

export function logTone(line: string): LogTone {
  if (/\b(?:error|erro|fatal|panic|failed|failure|exception|critical)\b|\bexit status\s+[1-9]\d*\b|\bnon[- ]zero\b|❌/i.test(line)) return 'error'
  if (/\bwarn(?:ing)?\b|\bcancel(?:led|ed)?\b|\btimed?\s*out\b|\btimeout\b|⚠(?:️)?/i.test(line)) return 'warning'
  return 'info'
}

function browserStorage(): Pick<Storage, 'getItem' | 'setItem'> | undefined {
  try {
    return typeof window === 'undefined' ? undefined : window.localStorage
  } catch {
    return undefined
  }
}

export function readLogTonePreference(storage = browserStorage()): boolean {
  try {
    return storage?.getItem(LOG_TONE_STORAGE_KEY) !== 'false'
  } catch {
    return true
  }
}

export function writeLogTonePreference(enabled: boolean, storage = browserStorage()): void {
  try {
    storage?.setItem(LOG_TONE_STORAGE_KEY, String(enabled))
  } catch {
    // Rendering logs must keep working when storage is unavailable.
  }
}
