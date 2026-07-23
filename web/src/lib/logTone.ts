export type LogTone = 'error' | 'warning' | 'info'

export const LOG_TONE_STORAGE_KEY = 'buildworld.logs.colorize'

export function logTone(line: string): LogTone {
  if (/\b(?:error|erro|fatal|panic|failed|failure|exception|critical)\b|\bexit status\s+[1-9]\d*\b|\bnon[- ]zero\b|❌/i.test(line)) return 'error'
  if (/\bwarn(?:ing)?\b|\bcancel(?:led|ed)?\b|\btimed?\s*out\b|\btimeout\b|⚠(?:️)?/i.test(line)) return 'warning'
  return 'info'
}

function logMessage(line: string): string {
  return line.replace(/^(?:\[[^\]]+\]\s*){0,2}/, '')
}

function isStackTraceHeader(line: string): boolean {
  return /^\s*(?:=+\s*)?stack trace(?:\s*=+)?\s*$/i.test(logMessage(line))
}

function isStackFrame(line: string): boolean {
  return /^\s*(?:\[\d+\]\s+|at\s+|caused by:|suppressed:|\.{3}\s+\d+\s+more|=+\s*$)/i.test(logMessage(line))
}

export function logTones(lines: readonly string[]): LogTone[] {
  let errorStack = false

  return lines.map((line, index) => {
    const directTone = logTone(line)
    if (isStackTraceHeader(line) && index > 0 && logTone(lines[index - 1]) === 'error') {
      errorStack = true
      return 'error'
    }
    if (errorStack && isStackFrame(line)) return 'error'
    errorStack = false
    return directTone
  })
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
