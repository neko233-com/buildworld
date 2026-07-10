// API client with JWT token management
const API_BASE = '/api'

function getToken(): string | null {
  return localStorage.getItem('token')
}

export function setToken(token: string) {
  localStorage.setItem('token', token)
}

export function clearToken() {
  localStorage.removeItem('token')
}

async function request<T = any>(method: string, path: string, body?: any): Promise<T> {
  const headers: Record<string, string> = {}
  const token = getToken()
  if (token) headers['Authorization'] = `Bearer ${token}`
  if (body) headers['Content-Type'] = 'application/json'

  const resp = await fetch(`${API_BASE}${path}`, {
    method,
    headers,
    body: body ? JSON.stringify(body) : undefined,
  })

  if (resp.status === 401) {
    clearToken()
    window.location.href = '/login'
    throw new Error('Unauthorized')
  }
  if (!resp.ok) {
    const err = await resp.json().catch(() => ({ error: resp.statusText }))
    throw new Error(err.error || `HTTP ${resp.status}`)
  }
  return resp.json()
}

export const api = {
  // auth
  login: (username: string, password: string) =>
    request<{ token: string; user: any }>('POST', '/auth/login', { username, password }),
  register: (username: string, email: string, password: string) =>
    request('POST', '/auth/register', { username, email, password }),
  me: () => request<any>('GET', '/auth/me'),

  // projects
  listProjects: () => request<any[]>('GET', '/projects/'),
  getProject: (id: number) => request<any>('GET', `/projects/${id}`),
  createProject: (data: any) => request('POST', '/projects/', data),
  updateProject: (id: number, data: any) => request('PUT', `/projects/${id}`, data),
  deleteProject: (id: number) => request('DELETE', `/projects/${id}`),
  triggerBuild: (id: number, data?: any) => request('POST', `/projects/${id}/builds`, data),
  listProjectBuilds: (id: number) => request<any[]>('GET', `/projects/${id}/builds`),

  // builds
  listBuilds: (limit = 100) => request<any[]>('GET', `/builds/?limit=${limit}`),
  getBuild: (id: number) => request<any>('GET', `/builds/${id}`),
  getBuildLogs: (id: number) => request<{ log: string }>('GET', `/builds/${id}/logs`),
  stopBuild: (id: number) => request('POST', `/builds/${id}/stop`),

  // agents
  listAgents: () => request<any[]>('GET', '/agents/'),
  registerAgent: (data: any) => request('POST', '/agents/register', data),
  deleteAgent: (id: string) => request('DELETE', `/agents/${id}`),
  generateAgentToken: () => request<{ token: string }>('POST', '/agents/generate-token'),

  // plugins
  listPlugins: () => request<any[]>('GET', '/plugins/'),
  installPlugin: (data: any) => request('POST', '/plugins/', data),
  deletePlugin: (id: number) => request('DELETE', `/plugins/${id}`),
  togglePlugin: (id: number, enabled: boolean) => request('PUT', `/plugins/${id}/enable`, { enabled }),

  // users
  listUsers: () => request<any[]>('GET', '/users/'),
  createUser: (data: any) => request('POST', '/users/', data),
  deleteUser: (id: number) => request('DELETE', `/users/${id}`),
  updateUserRole: (id: number, role: string) => request('PUT', `/users/${id}/role`, { role }),
  updateUserPassword: (id: number, password: string) => request('PUT', `/users/${id}/password`, { password }),

  // env vars
  listEnvVars: (scope = 'global', projectId?: number) =>
    request<any[]>('GET', `/env-vars/?scope=${scope}${projectId ? `&project_id=${projectId}` : ''}`),
  setEnvVar: (data: any) => request('POST', '/env-vars/', data),
  deleteEnvVar: (id: number) => request('DELETE', `/env-vars/${id}`),

  // credentials
  listCredentials: (type?: string) =>
    request<any[]>('GET', `/credentials/${type ? `?type=${type}` : ''}`),
  getCredential: (id: number, reveal?: boolean) =>
    request<any>('GET', `/credentials/${id}${reveal ? '?reveal=true' : ''}`),
  createCredential: (data: any) => request('POST', '/credentials/', data),
  updateCredential: (id: number, data: any) => request('PUT', `/credentials/${id}`, data),
  deleteCredential: (id: number) => request('DELETE', `/credentials/${id}`),
  lookupCredential: (host: string, type: string) =>
    request<any>('GET', `/credentials/lookup?host=${encodeURIComponent(host)}&type=${type}`),

  // vcs roots
  listVCSRoots: () => request<any[]>('GET', '/vcs-roots/'),
  getVCSRoot: (id: number) => request<any>('GET', `/vcs-roots/${id}`),
  createVCSRoot: (data: any) => request('POST', '/vcs-roots/', data),
  updateVCSRoot: (id: number, data: any) => request('PUT', `/vcs-roots/${id}`, data),
  deleteVCSRoot: (id: number) => request('DELETE', `/vcs-roots/${id}`),

  // build templates
  listTemplates: () => request<any[]>('GET', '/templates/'),
  getTemplate: (id: number) => request<any>('GET', `/templates/${id}`),
  createTemplate: (data: any) => request('POST', '/templates/', data),
  updateTemplate: (id: number, data: any) => request('PUT', `/templates/${id}`, data),
  deleteTemplate: (id: number) => request('DELETE', `/templates/${id}`),

  // build control
  retryBuild: (id: number) => request<any>('POST', `/builds/${id}/retry`),
  pinBuild: (id: number, pinned: boolean) => request('POST', `/builds/${id}/pin`, { pinned }),

  // artifacts
  listArtifacts: (buildId: number) => request<any[]>('GET', `/builds/${buildId}/artifacts`),
  uploadArtifact: (buildId: number, file: File) => {
    const form = new FormData()
    form.append('file', file)
    const token = getToken()
    return fetch(`${API_BASE}/builds/${buildId}/artifacts`, {
      method: 'POST',
      headers: token ? { Authorization: `Bearer ${token}` } : {},
      body: form,
    }).then(r => { if (!r.ok) throw new Error(`HTTP ${r.status}`); return r.json() })
  },
  artifactDownloadUrl: (id: number) => {
    const token = getToken()
    return `${API_BASE}/artifacts/${id}/download${token ? `?token=${encodeURIComponent(token)}` : ''}`
  },

  // notifications
  listNotificationChannels: () => request<any[]>('GET', '/notifications/channels/'),
  getNotificationChannel: (id: number) => request<any>('GET', `/notifications/channels/${id}`),
  createNotificationChannel: (data: any) => request('POST', '/notifications/channels/', data),
  updateNotificationChannel: (id: number, data: any) => request('PUT', `/notifications/channels/${id}`, data),
  deleteNotificationChannel: (id: number) => request('DELETE', `/notifications/channels/${id}`),
  listNotificationEvents: (id: number, limit = 100) => request<any[]>('GET', `/notifications/channels/${id}/events?limit=${limit}`),

  // statistics
  getDashboardStats: () => request<any>('GET', '/stats/dashboard'),
  getProjectStats: (id: number, days: number) => request<any>('GET', `/stats/projects/${id}?days=${days}`),

  // audit logs
  listAuditLogs: (page?: number, limit?: number) => request<any[]>('GET', `/audit-logs?page=${page || 1}&limit=${limit || 50}`),

  // api tokens
  listAPITokens: () => request<any[]>('GET', '/tokens/'),
  createAPIToken: (data: any) => request<any>('POST', '/tokens/', data),
  deleteAPIToken: (id: number) => request('DELETE', `/tokens/${id}`),

  // build approvals
  listPendingApprovals: () => request<any[]>('GET', '/approvals'),
  approveBuild: (id: number, comment?: string) => request('POST', `/builds/${id}/approve`, { comment }),
  rejectBuild: (id: number, comment?: string) => request('POST', `/builds/${id}/reject`, { comment }),

  // build logs
  downloadBuildLogs: (id: number, format?: string) => `${API_BASE}/builds/${id}/logs/download?format=${format || 'txt'}&token=${localStorage.getItem('token')}`,
  searchBuildLogs: (id: number, query: string) => request<any[]>('GET', `/builds/${id}/logs/search?q=${encodeURIComponent(query)}`),

  // test reports
  getBuildTestResults: (id: number) => request<any>('GET', `/builds/${id}/test-results`),
  uploadTestResults: (id: number, data: string) => request('POST', `/builds/${id}/test-results`, { xml: data }),

  // deployments
  listDeploymentEnvs: () => request<any[]>('GET', '/deployments/'),
  createDeploymentEnv: (data: any) => request('POST', '/deployments/', data),
  deleteDeploymentEnv: (id: number) => request('DELETE', `/deployments/${id}`),
  deployBuild: (envId: number, buildId: number) => request('POST', `/deployments/${envId}/deploy/${buildId}`),

  // project groups
  listProjectGroups: () => request<any[]>('GET', '/project-groups/'),
  createProjectGroup: (data: any) => request('POST', '/project-groups/', data),
  deleteProjectGroup: (id: number) => request('DELETE', `/project-groups/${id}`),

  // build queue
  listBuildQueue: () => request<any[]>('GET', '/build-queue'),
  reorderBuildQueue: (id: number, priority: number) => request('PUT', `/build-queue/${id}/priority`, { priority }),

  // global settings
  getGlobalSettings: () => request<any>('GET', '/settings'),
  updateGlobalSettings: (data: any) => request('PUT', '/settings', data),

  // server metrics
  getServerMetrics: () => request<any>('GET', '/metrics'),
}
