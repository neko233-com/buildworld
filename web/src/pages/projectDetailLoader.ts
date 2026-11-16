import { api } from '../api'

const PROJECT_TIMEOUT_MS = 12_000
const PREFETCH_RETENTION_MS = 10_000
type Entry = { promise: Promise<any>; awaitingNavigation: boolean; settled: boolean; expiry?: ReturnType<typeof setTimeout> }
const pending = new Map<string, Entry>()

// A navigation can finish fetching before its lazy route is ready. Retain that
// result briefly for one consumer, scoped to the current authentication token.
function projectRequest(id: number, navigation: boolean): Promise<any> {
  const key = `${localStorage.getItem('token') || ''}:${id}`
  const existing = pending.get(key)
  if (existing) {
    if (!navigation) {
      existing.awaitingNavigation = false
      if (existing.expiry) clearTimeout(existing.expiry)
      if (existing.settled) pending.delete(key)
    }
    return existing.promise
  }

  const controller = new AbortController()
  let timer: ReturnType<typeof setTimeout>
  const timeout = new Promise<never>((_, reject) => {
    timer = setTimeout(() => {
      controller.abort()
      reject(new Error(document.documentElement.lang.toLowerCase().startsWith('zh')
        ? '项目加载超时，请检查服务连接后重试。'
        : 'Project loading timed out. Check the server connection and retry.'))
    }, PROJECT_TIMEOUT_MS)
  })
  const result = Promise.race([api.getProject(id, { signal: controller.signal }), timeout])
  const entry: Entry = { promise: result, awaitingNavigation: navigation, settled: false }
  pending.set(key, entry)
  const cleanup = () => {
    clearTimeout(timer)
    entry.settled = true
    if (pending.get(key) !== entry) return
    if (!entry.awaitingNavigation) pending.delete(key)
    else entry.expiry = setTimeout(() => { if (pending.get(key) === entry) pending.delete(key) }, PREFETCH_RETENTION_MS)
  }
  void result.then(cleanup, cleanup)
  return result
}

export function loadProjectDetail(id: number): Promise<any> {
  return projectRequest(id, false)
}

// Load the route on intent, then start its data request as navigation begins.
export function preloadProjectDetail() {
  void import('./ProjectDetail').catch(() => {})
}

export function beginProjectDetailNavigation(id: number) {
  preloadProjectDetail()
  void projectRequest(id, true).catch(() => {})
}
