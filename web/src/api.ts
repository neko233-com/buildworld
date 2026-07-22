import type { BuildTimeline } from './lib/buildTimeline'
import { normalizeBuildProblemReport, type BuildProblemReport } from './lib/buildProblems'
import { buildSearchQuery, type BuildSearchFilters, type BuildSearchResponse } from './lib/buildSearch'
import type { BuildChain } from './lib/buildChain'
import type { QueueMoveOperation } from './lib/queuePresentation'

// API client with JWT token management
const API_BASE = '/api'
const RETRYABLE_STATUS = new Set([502, 503, 504])
const RETRY_DELAYS = [200, 500]
export const API_STATUS_EVENT = 'buildworld:api-status'
export const API_FEEDBACK_EVENT = 'buildworld:api-feedback'

export type APIFeedbackDetail = {
  operationId: string
  phase: 'start' | 'success' | 'error'
  tone: 'info' | 'success' | 'error'
  message: string
}

let nextOperationID = 1

export type PipelineMigrationResult = {
  version: string
  source_format: string
  target_format: string
  config: string
  warnings: Array<{ code: string; message: string }>
  summary: { stage_count: number; environment_count: number }
  hints: { repository_url?: string; default_branch?: string }
}

export type ProjectBuildOverview = {
  project_id: number
  latest: BuildOverview | null
  last_success: BuildOverview | null
  last_failure: BuildOverview | null
  recent_statuses: string[]
}

export type BuildOverview = {
  id: number
  number: number
  status: string
  started_at?: string | null
  finished_at?: string | null
  duration_ms?: number | null
}

export type ProjectChange = {
  build_id: number
  build_number: number
  status: string
  commit_sha: string
  branch: string
  timestamp: string | null
}

export type TestReportSummary = {
  id: number
  build_id: number
  total: number
  passed: number
  failed: number
  skipped: number
  duration_ms: number
  created_at: string
}

export type TestCaseResult = {
  name: string
  classname?: string
  suite_name?: string
  status: 'passed' | 'failed' | 'skipped'
  duration_ms: number
  message?: string
  type?: string
  details?: string
}

export type BuildTestResultsResponse = {
  summary: TestReportSummary | null
  cases: TestCaseResult[]
  results: TestReportSummary[]
}

type RequestOptions = {
  contentType?: string
  rawBody?: boolean
}

export class ApiError extends Error {
  readonly status: number
  readonly code: string

  constructor(message: string, status = 0, code = 'api_error') {
    super(message)
    this.name = 'ApiError'
    this.status = status
    this.code = code
  }
}

function getToken(): string | null {
  return localStorage.getItem('token')
}

export function setToken(token: string) {
  localStorage.setItem('token', token)
}

export function clearToken() {
  localStorage.removeItem('token')
}

function authHeader(): Record<string, string> {
  const token = getToken()
  return token ? { Authorization: `Bearer ${token}` } : {}
}

function dispatchAPIStatus(status: 'available' | 'unavailable') {
  if (typeof window !== 'undefined') {
    window.dispatchEvent(new CustomEvent(API_STATUS_EVENT, { detail: { status } }))
  }
}

function prefersChinese(): boolean {
  return typeof navigator !== 'undefined' && (localStorage.getItem('locale') === 'zh-CN' || navigator.language.startsWith('zh'))
}

function operationMessage(phase: APIFeedbackDetail['phase']): string {
  if (prefersChinese()) {
    if (phase === 'start') return '正在处理操作…'
    if (phase === 'success') return '操作已完成'
    return '操作失败'
  }
  if (phase === 'start') return 'Processing…'
  if (phase === 'success') return 'Operation completed.'
  return 'Operation failed.'
}

function localizedAPIError(code: string, fallback: string): string {
  if (code === 'project_group_name_exists') {
    return prefersChinese() ? '项目分组名称已存在' : 'A project group with this name already exists.'
  }
  if (code === 'required_notification_channel') {
    return prefersChinese() ? 'Web 页面内通知是系统标配，必须保持启用' : 'In-app Web notifications are required and must remain enabled.'
  }
  return fallback
}

function dispatchAPIFeedback(operationId: string | undefined, phase: APIFeedbackDetail['phase'], message?: string) {
  if (!operationId || typeof window === 'undefined') return
  const tone = phase === 'start' ? 'info' : phase
  window.dispatchEvent(new CustomEvent<APIFeedbackDetail>(API_FEEDBACK_EVENT, {
    detail: { operationId, phase, tone, message: message || operationMessage(phase) },
  }))
}

function beginOperation(message?: string): string {
  const operationId = `api-operation-${nextOperationID++}`
  dispatchAPIFeedback(operationId, 'start', message)
  return operationId
}

function shouldReportMutation(method: string, path: string): boolean {
  if (method === 'GET' || method === 'HEAD') return false
  if (path === '/auth/login' || path === '/notifications/in-app/read') return false
  // These POST endpoints only inspect source or project metadata. They run while
  // editors and Jenkins-style build actions are loading, so mutation toasts
  // would falsely announce that user data changed on every validation pass.
  if (path === '/pipeline-validation' || /^\/projects\/\d+\/validate$/.test(path)) return false
  return true
}

function serviceUnavailableMessage(status?: number): string {
  if (prefersChinese()) return status ? `构建服务暂时不可用（HTTP ${status}），已自动重试。` : '无法连接到构建服务，已自动重试。'
  return status ? `The build service is temporarily unavailable (HTTP ${status}) after automatic retries.` : 'Unable to reach the build service after automatic retries.'
}

async function parseResponseBody(resp: Response): Promise<any> {
  const text = await resp.text()
  if (!text) return undefined
  try {
    return JSON.parse(text)
  } catch {
    return text
  }
}

function responseFilename(resp: Response, fallback: string): string {
  const disposition = resp.headers.get('Content-Disposition') || ''
  const encoded = disposition.match(/filename\*=UTF-8''([^;]+)/i)?.[1]
  if (encoded) {
    try {
      return decodeURIComponent(encoded.replace(/^"|"$/g, ''))
    } catch {
      // Fall through to the plain filename or caller-provided fallback.
    }
  }
  return disposition.match(/filename="([^"]+)"/i)?.[1] || fallback
}

export async function downloadAuthenticated(path: string, fallbackFilename: string): Promise<string> {
  const operationId = beginOperation(prefersChinese() ? '正在准备下载…' : 'Preparing download…')
  let response: Response
  try {
    response = await fetch(`${API_BASE}${path}`, { headers: authHeader() })
  } catch {
    const message = serviceUnavailableMessage()
    dispatchAPIFeedback(operationId, 'error', message)
    throw new ApiError(message, 0, 'network_unavailable')
  }
  if (response.status === 401) {
    clearToken()
    window.location.href = '/login'
    throw new ApiError('Unauthorized', 401, 'unauthorized')
  }
  if (!response.ok) {
    const payload = await parseResponseBody(response)
    const message = typeof payload === 'object' ? payload?.error : ''
    const errorMessage = message || response.statusText || `HTTP ${response.status}`
    dispatchAPIFeedback(operationId, 'error', errorMessage)
    throw new ApiError(errorMessage, response.status)
  }

  const filename = responseFilename(response, fallbackFilename)
  const objectURL = URL.createObjectURL(await response.blob())
  const anchor = document.createElement('a')
  anchor.href = objectURL
  anchor.download = filename
  anchor.hidden = true
  document.body.appendChild(anchor)
  anchor.click()
  anchor.remove()
  window.setTimeout(() => URL.revokeObjectURL(objectURL), 0)
  dispatchAPIFeedback(operationId, 'success', prefersChinese() ? `已开始下载 ${filename}` : `Download started: ${filename}`)
  return filename
}

async function retryDelay(attempt: number) {
  await new Promise(resolve => window.setTimeout(resolve, RETRY_DELAYS[attempt]))
}

export async function request<T = any>(method: string, path: string, body?: any, options: RequestOptions = {}): Promise<T> {
  const headers: Record<string, string> = {}
  const token = getToken()
  if (token) headers['Authorization'] = `Bearer ${token}`
  const hasBody = body !== undefined && body !== null
  if (hasBody) headers['Content-Type'] = options.contentType || 'application/json'

  const retryableMethod = method === 'GET' || method === 'HEAD'
  const operationId = shouldReportMutation(method, path) ? beginOperation() : undefined
  for (let attempt = 0; ; attempt++) {
    let resp: Response
    try {
      resp = await fetch(`${API_BASE}${path}`, {
        method,
        headers,
        body: hasBody ? (options.rawBody ? body : JSON.stringify(body)) : undefined,
      })
    } catch {
      if (retryableMethod && attempt < RETRY_DELAYS.length) {
        await retryDelay(attempt)
        continue
      }
      dispatchAPIStatus('unavailable')
      const message = serviceUnavailableMessage()
      dispatchAPIFeedback(operationId, 'error', message)
      throw new ApiError(message, 0, 'network_unavailable')
    }

    if (resp.status === 401) {
      clearToken()
      window.location.href = '/login'
      throw new ApiError('Unauthorized', 401, 'unauthorized')
    }
    if (!resp.ok) {
      if (retryableMethod && RETRYABLE_STATUS.has(resp.status) && attempt < RETRY_DELAYS.length) {
        await retryDelay(attempt)
        continue
      }
      const payload = await parseResponseBody(resp)
      if (RETRYABLE_STATUS.has(resp.status)) {
        dispatchAPIStatus('unavailable')
        const message = serviceUnavailableMessage(resp.status)
        dispatchAPIFeedback(operationId, 'error', message)
        throw new ApiError(message, resp.status, 'service_unavailable')
      }
      const message = typeof payload === 'object' ? payload?.error : ''
      const errorCode = typeof payload === 'object' ? payload?.code || 'api_error' : 'api_error'
      const errorMessage = localizedAPIError(errorCode, message || resp.statusText || `HTTP ${resp.status}`)
      dispatchAPIFeedback(operationId, 'error', errorMessage)
      throw new ApiError(errorMessage, resp.status, errorCode)
    }
    dispatchAPIStatus('available')
    const payload = await parseResponseBody(resp) as T
    dispatchAPIFeedback(operationId, 'success')
    return payload
  }
}

export const api = {
  // auth
  login: (username: string, password: string) =>
    request<{ token: string; user: any }>('POST', '/auth/login', { username, password }),
  me: () => request<any>('GET', '/auth/me'),

  // projects
  listProjects: () => request<any[]>('GET', '/projects/'),
  listProjectBuildOverviews: () => request<ProjectBuildOverview[]>('GET', '/projects/job-overview'),
  listQuickAccess: () => request<any[]>('GET', '/projects/?quick_access=1'),
  setProjectFlags: (id: number, favorite: boolean, quickAccess: boolean) =>
    request<any>('POST', `/projects/${id}/flags`, { favorite, quick_access: quickAccess }),
  getProject: (id: number) => request<any>('GET', `/projects/${id}`),
  createProject: (data: any) => request('POST', '/projects/', data),
  updateProject: (id: number, data: any) => request('PUT', `/projects/${id}`, data),
  deleteProject: (id: number) => request('DELETE', `/projects/${id}`),
  triggerBuild: (id: number, data?: any) => request('POST', `/projects/${id}/builds`, data),
  listProjectChanges: (id: number) => request<ProjectChange[]>('GET', `/projects/${id}/changes`),
  validatePipeline: (source: string) =>
    request<{ valid: boolean; format: 'typescript' | 'yaml' | 'jenkinsfile' | 'auto'; stages: number; steps: number; parameters: any[]; allow_long_running: boolean }>('POST', '/pipeline-validation', { source }),
  validateProject: (id: number) =>
    request<{ valid: boolean; format: 'typescript' | 'yaml' | 'jenkinsfile' | 'auto'; stages: number; steps: number; parameters: any[]; allow_long_running: boolean }>('POST', `/projects/${id}/validate`),
  migratePipeline: (format: string, source: string, name?: string) =>
    request<PipelineMigrationResult>('POST', `/pipeline-migrations/${encodeURIComponent(format)}`, { source, name }),

  // builds
  listBuilds: (limit = 100) => request<any[]>('GET', `/builds/?limit=${limit}`),
  searchBuilds: (filters: BuildSearchFilters) =>
    request<BuildSearchResponse>('GET', `/builds/search?${buildSearchQuery(filters)}`),
  getBuild: (id: number) => request<any>('GET', `/builds/${id}`),
  getBuildLogs: (id: number) => request<{ log: string; truncated?: boolean; retention_characters?: number }>('GET', `/builds/${id}/logs`),
  getBuildTimeline: (id: number) => request<BuildTimeline>('GET', `/builds/${id}/timeline`),
  getBuildProblems: async (id: number): Promise<BuildProblemReport> =>
    normalizeBuildProblemReport(await request<unknown>('GET', `/builds/${id}/problems`)),
  getBuildChain: (id: number) => request<BuildChain>('GET', `/builds/${id}/chain`),
  stopBuild: (id: number) => request('POST', `/builds/${id}/stop`),

  // agents
  listAgents: () => request<any[]>('GET', '/agents/'),
  registerAgent: (data: any) => request('POST', '/agents/register', data),
  deleteAgent: (id: string) => request('DELETE', `/agents/${id}`),
  generateAgentToken: () => request<{ token: string }>('POST', '/agents/generate-token'),

  // plugins
  listPlugins: () => request<any[]>('GET', '/plugins/'),
  installGitHubPlugin: (url: string) => request('POST', '/plugins/github', { url }),
  deletePlugin: (idOrName: number | string) => request('DELETE', `/plugins/${idOrName}`),
  togglePlugin: (idOrName: number | string, enabled: boolean) => request('PUT', `/plugins/${idOrName}/enable`, { enabled }),
  reloadPlugin: (name: string) => request('POST', `/plugins/${name}/reload`),

  // users
  listUsers: () => request<any[]>('GET', '/users/'),
  createUser: (data: any) => request('POST', '/users/', data),
  deleteUser: (id: number) => request('DELETE', `/users/${id}`),
  updateUserRole: (id: number, role: string) => request('PUT', `/users/${id}/role`, { role }),
  updateUserPassword: (id: number, password: string) => request('PUT', `/users/${id}/password`, { password }),

  // credentials
  listCredentials: (type?: string) =>
    request<any[]>('GET', `/credentials/${type ? `?type=${type}` : ''}`),
  getCredential: (id: number, reveal?: boolean) =>
    request<any>('GET', `/credentials/${id}${reveal ? '?reveal=true' : ''}`),
  createCredential: (data: any) => request('POST', '/credentials/', data),
  updateCredential: (id: number, data: any) => request('PUT', `/credentials/${id}`, data),
  deleteCredential: (id: number) => request('DELETE', `/credentials/${id}`),

  // vcs roots
  listVCSRoots: () => request<any[]>('GET', '/vcs-roots/'),
  createVCSRoot: (data: any) => request('POST', '/vcs-roots/', data),
  updateVCSRoot: (id: number, data: any) => request('PUT', `/vcs-roots/${id}`, data),
  deleteVCSRoot: (id: number) => request('DELETE', `/vcs-roots/${id}`),

  // build templates
  listTemplates: () => request<any[]>('GET', '/templates/'),
  createTemplate: (data: any) => request('POST', '/templates/', data),
  updateTemplate: (id: number, data: any) => request('PUT', `/templates/${id}`, data),
  deleteTemplate: (id: number) => request('DELETE', `/templates/${id}`),

  // build control
  retryBuild: (id: number) => request<any>('POST', `/builds/${id}/retry`),
  pinBuild: (id: number, pinned: boolean) => request('POST', `/builds/${id}/pin`, { pinned }),

  // artifacts
  listArtifacts: (buildId: number) => request<any[]>('GET', `/builds/${buildId}/artifacts`),
  uploadArtifact: async (buildId: number, file: File) => {
    const form = new FormData()
    form.append('file', file)
    const token = getToken()
    const operationId = beginOperation(prefersChinese() ? '正在上传构建产物…' : 'Uploading artifact…')
    try {
      const response = await fetch(`${API_BASE}/builds/${buildId}/artifacts`, {
        method: 'POST',
        headers: token ? { Authorization: `Bearer ${token}` } : {},
        body: form,
      })
      if (!response.ok) {
        const payload = await parseResponseBody(response)
        const message = typeof payload === 'object' ? payload?.error : ''
        throw new ApiError(message || response.statusText || `HTTP ${response.status}`, response.status)
      }
      const payload = await response.json()
      dispatchAPIFeedback(operationId, 'success', prefersChinese() ? '构建产物已上传' : 'Artifact uploaded.')
      return payload
    } catch (reason) {
      const message = reason instanceof Error ? reason.message : operationMessage('error')
      dispatchAPIFeedback(operationId, 'error', message)
      throw reason
    }
  },
  downloadArtifact: (id: number, name: string) =>
    downloadAuthenticated(`/artifacts/${id}/download`, name || `artifact-${id}`),

  // notifications
  listNotificationChannels: () => request<any[]>('GET', '/notifications/channels/'),
  createNotificationChannel: (data: any) => request('POST', '/notifications/channels/', data),
  updateNotificationChannel: (id: number, data: any) => request('PUT', `/notifications/channels/${id}`, data),
  deleteNotificationChannel: (id: number) => request('DELETE', `/notifications/channels/${id}`),
  listNotificationEvents: (id: number, limit = 100) => request<any[]>('GET', `/notifications/channels/${id}/events?limit=${limit}`),
  listInAppNotifications: (limit = 30) => request<any>('GET', `/notifications/in-app?limit=${limit}`),
  markInAppNotificationsRead: (lastEventId: number) => request('POST', '/notifications/in-app/read', { last_event_id: lastEventId }),

  // statistics
  getDashboardStats: () => request<any>('GET', '/stats/dashboard'),

  // audit logs
  listAuditLogs: (page = 1, limit = 50) =>
    request<any[]>('GET', `/audit-logs?offset=${Math.max(0, page - 1) * limit}&limit=${limit}`),

  // api tokens
  listAPITokens: () => request<any[]>('GET', '/api-tokens/'),
  createAPIToken: (data: any) => request<any>('POST', '/api-tokens/', data),
  deleteAPIToken: (id: number) => request('DELETE', `/api-tokens/${id}`),

  // build approvals
  listPendingApprovals: () => request<any[]>('GET', '/approvals/pending'),
  approveBuild: (id: number, comment?: string) => request('POST', `/builds/${id}/approve`, { comment }),
  rejectBuild: (id: number, comment?: string) => request('POST', `/builds/${id}/reject`, { comment }),

  // build logs
  downloadBuildLogs: (id: number, format: 'txt' | 'json' = 'txt') =>
    downloadAuthenticated(`/builds/${id}/logs/download?format=${format}`, `build-${id}-logs.${format}`),

  // test reports
  getBuildTestResults: (id: number) => request<BuildTestResultsResponse>('GET', `/builds/${id}/test-results`),
  uploadTestResults: (id: number, data: string) =>
    request<TestReportSummary>('POST', `/builds/${id}/test-results`, data, { contentType: 'application/xml', rawBody: true }),


  // project groups
  listProjectGroups: () => request<any[]>('GET', '/project-groups/'),
  createProjectGroup: (data: any) => request('POST', '/project-groups/', data),
  updateProjectGroup: (id: number, data: any) => request('PUT', `/project-groups/${id}`, data),
  deleteProjectGroup: (id: number) => request('DELETE', `/project-groups/${id}`),

  // build queue
  listBuildQueue: () => request<any[]>('GET', '/build-queue'),
  reorderBuildQueue: (id: number, operation: QueueMoveOperation) =>
    request<{ version: number; status: string; operation: string; position: number; items: any[] }>('PUT', `/build-queue/${id}`, { operation }),

  // global settings
  getGlobalSettings: () => request<any>('GET', '/settings'),
  updateGlobalSettings: (data: any) => request('PUT', '/settings', data),
  listPackageProxies: () => request<any[]>('GET', '/settings/package-proxies'),
  applyPackageProxies: (mode: 'accelerated' | 'official', packages?: string[]) =>
    request<any>('POST', '/settings/package-proxies/apply', { mode, packages }),

  // versioned configuration portability
  listPortabilityCapabilities: () => request<any[]>('GET', '/portability/capabilities'),
  inspectPortabilityBundle: (bundle: any) => request<any>('POST', '/portability/inspect', bundle),
  importPortabilityBundle: (bundle: any, mode: 'skip' | 'overwrite') => request<any>('POST', `/portability/import?mode=${mode}`, bundle),
  exportPortabilityBundle: async (options: { sections: string[]; include_secrets: boolean }) => {
    const operationId = beginOperation(prefersChinese() ? '正在导出配置…' : 'Exporting configuration…')
    try {
      const response = await fetch(`${API_BASE}/portability/export`, {
        method: 'POST',
        headers: { ...authHeader(), 'Content-Type': 'application/json' },
        body: JSON.stringify(options),
      })
      if (!response.ok) {
        const error = await response.json().catch(() => ({ error: response.statusText }))
        throw new ApiError(error.error || `HTTP ${response.status}`, response.status)
      }
      const disposition = response.headers.get('Content-Disposition') || ''
      const filename = disposition.match(/filename="([^"]+)"/)?.[1] || 'buildworld-config.json'
      const result = { blob: await response.blob(), filename }
      dispatchAPIFeedback(operationId, 'success', prefersChinese() ? '配置导出已准备完成' : 'Configuration export is ready.')
      return result
    } catch (reason) {
      const message = reason instanceof Error ? reason.message : operationMessage('error')
      dispatchAPIFeedback(operationId, 'error', message)
      throw reason
    }
  },

  // server metrics
  getServerMetrics: () => request<any>('GET', '/metrics'),

  // bigscreen
  getBigScreenData: () => request<any>('GET', '/bigscreen'),

}
