export const BUILD_HISTORY_PAGE_SIZE = 25

export interface BuildSummary {
  id: number
  project_id: number
  number: number
  status: string
  trigger: string
  branch?: string
  commit_sha?: string
  pinned?: boolean
  started_at?: string
  finished_at?: string
  duration_ms?: number
}

export interface BuildSearchFilters {
  q: string
  projectId: number
  status: string
  trigger: string
  branch: string
  pinned: boolean
  page: number
  limit: number
}

export interface BuildSearchResponse {
  version: number
  items: BuildSummary[]
  total: number
  limit: number
  offset: number
}

function positiveInteger(value: string | null, fallback: number): number {
  const parsed = Number(value)
  return Number.isSafeInteger(parsed) && parsed > 0 ? parsed : fallback
}

export function readBuildSearchParams(params: URLSearchParams): BuildSearchFilters {
  return {
    q: (params.get('q') || '').trim(),
    projectId: positiveInteger(params.get('project'), 0),
    status: params.get('status') || '',
    trigger: params.get('trigger') || '',
    branch: (params.get('branch') || '').trim(),
    pinned: params.get('pinned') === '1',
    page: positiveInteger(params.get('page'), 1),
    limit: BUILD_HISTORY_PAGE_SIZE,
  }
}

export function buildSearchQuery(filters: BuildSearchFilters): string {
  const params = new URLSearchParams()
  if (filters.q) params.set('q', filters.q)
  if (filters.projectId) params.set('project_id', String(filters.projectId))
  if (filters.status) params.set('status', filters.status)
  if (filters.trigger) params.set('trigger', filters.trigger)
  if (filters.branch) params.set('branch', filters.branch)
  if (filters.pinned) params.set('pinned', 'true')
  params.set('limit', String(filters.limit))
  params.set('offset', String((filters.page - 1) * filters.limit))
  return params.toString()
}

export function activeBuildFilterCount(filters: BuildSearchFilters): number {
  return [
    filters.q,
    filters.projectId,
    filters.status,
    filters.trigger,
    filters.branch,
    filters.pinned,
  ].filter(Boolean).length
}
