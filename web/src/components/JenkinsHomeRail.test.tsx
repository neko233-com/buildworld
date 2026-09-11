// @vitest-environment jsdom

import { act } from 'react'
import { createRoot, type Root } from 'react-dom/client'
import { MemoryRouter, useLocation } from 'react-router-dom'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { api } from '../api'
import { canEdit } from '../authz'
import { dialogs } from './AppDialogs'
import JenkinsHomeRail from './JenkinsHomeRail'

vi.mock('../api', () => ({
  api: {
    listBuildQueue: vi.fn(),
    getBuildQueueCapacity: vi.fn(),
    listAgents: vi.fn(),
    listBuilds: vi.fn(),
    listProjects: vi.fn(),
    getBuildTimeline: vi.fn(),
    retryBuild: vi.fn(),
  },
}))

vi.mock('../authz', () => ({ canEdit: vi.fn() }))
vi.mock('./AppDialogs', () => ({ dialogs: { confirm: vi.fn(), notify: vi.fn() } }))

const listBuildQueue = vi.mocked(api.listBuildQueue)
const getBuildQueueCapacity = vi.mocked(api.getBuildQueueCapacity)
const listAgents = vi.mocked(api.listAgents)
const listBuilds = vi.mocked(api.listBuilds)
const listProjects = vi.mocked(api.listProjects)
const getBuildTimeline = vi.mocked(api.getBuildTimeline)
const retryBuild = vi.mocked(api.retryBuild)
const mockedCanEdit = vi.mocked(canEdit)
const actEnvironment = globalThis as typeof globalThis & { IS_REACT_ACT_ENVIRONMENT: boolean }

const queue = [
  { id: 1, build_id: 101, build_number: 11, project_name: 'Alpha', status: 'running' },
  { id: 2, build_id: 102, build_number: 12, project_name: 'Beta', status: 'queued' },
  { id: 3, build_id: 103, build_number: 13, project_name: 'Gamma', status: 'pending_approval' },
  { id: 4, build_id: 104, build_number: 14, project_name: 'Delta', status: 'queued' },
]

const agents = [
  { id: 'a', name: 'Worker A', status: 'online', active_builds: 2, max_concurrent_builds: 4 },
  { id: 'b', name: 'Worker B', status: 'online', active_builds: 1, max_concurrent_builds: 2 },
  { id: 'c', name: 'Worker C', status: 'offline', active_builds: 0, max_concurrent_builds: 3 },
  { id: 'd', name: 'Worker D', status: 'online', active_builds: 0, max_concurrent_builds: 1 },
]

const recentBuilds = [
  { id: 201, project_id: 1, project_name: 'Alpha', number: 12, status: 'success', branch: 'main', started_at: '2026-07-21T10:12:00Z' },
  { id: 199, project_id: 1, project_name: 'Alpha', number: 11, status: 'failed', branch: 'main', started_at: '2026-07-21T10:10:00Z' },
  { id: 202, project_id: 2, project_name: 'Beta', number: 8, status: 'running', branch: 'develop', started_at: '2026-07-21T10:11:00Z' },
  { id: 198, project_id: 3, project_name: 'Gamma', number: 4, status: 'failed', started_at: '2026-07-21T10:09:00Z' },
  { id: 197, project_id: 4, project_name: 'Delta', number: 3, status: 'test_failed', started_at: '2026-07-21T10:08:00Z' },
  { id: 196, project_id: 5, project_name: 'Epsilon', number: 2, status: 'cancelled', started_at: '2026-07-21T10:07:00Z' },
]

function LocationProbe() {
  return <output aria-label="current-location">{useLocation().pathname}</output>
}

describe('JenkinsHomeRail', () => {
  let container: HTMLDivElement
  let root: Root

  beforeEach(() => {
    actEnvironment.IS_REACT_ACT_ENVIRONMENT = true
    Object.defineProperty(document, 'visibilityState', { configurable: true, value: 'visible' })
    mockedCanEdit.mockReset().mockReturnValue(false)
    listBuildQueue.mockReset().mockResolvedValue(queue)
    getBuildQueueCapacity.mockReset().mockResolvedValue({ executor: 'builtin', max_concurrent_builds: 4 })
    listAgents.mockReset().mockResolvedValue(agents)
    listBuilds.mockReset().mockResolvedValue(recentBuilds)
    listProjects.mockReset().mockResolvedValue([{ id: 1, name: 'Alpha' }, { id: 2, name: 'Beta' }, { id: 3, name: 'Gamma' }])
    getBuildTimeline.mockReset().mockResolvedValue({ build_status: 'running', current_step: 1, total_steps: 4, completed_steps: 1, steps: [{ index: 1, stage: 'Compile', name: 'Compile', status: 'running' }] })
    retryBuild.mockReset().mockResolvedValue({ id: 301 })
    vi.mocked(dialogs.confirm).mockReset().mockResolvedValue(false)
    vi.mocked(dialogs.notify).mockReset()
    container = document.createElement('div')
    document.body.appendChild(container)
    root = createRoot(container)
  })

  afterEach(() => {
    act(() => root.unmount())
    container.remove()
    localStorage.clear()
    vi.useRealTimers()
    actEnvironment.IS_REACT_ACT_ENVIRONMENT = false
  })

  async function render(editable?: boolean, builds = recentBuilds) {
    await act(async () => {
      root.render(<MemoryRouter><JenkinsHomeRail editable={editable} recentBuilds={builds} /><LocationProbe /></MemoryRouter>)
    })
  }

  it('matches the Jenkins queue pane, keeps Worker dormant, and persists collapse state', async () => {
    await render()

    expect(mockedCanEdit).toHaveBeenCalledOnce()
    expect(container.querySelector('a[href="/projects/new"]')).toBeNull()
    expect(container.querySelector('a[href="/templates"]')).not.toBeNull()
    expect(container.querySelector('a[href="/settings"]')).toBeNull()
    for (const href of ['/builds', '/templates', '/projects', '/vcs-roots', '/build-queue']) {
      expect(container.querySelector(`a[href="${href}"]`)).not.toBeNull()
    }
    expect(container.querySelector('a[href="/agents"]')).toBeNull()
    expect(listAgents).not.toHaveBeenCalled()
    expect(listBuilds).toHaveBeenCalledWith(30)
    expect(listProjects).toHaveBeenCalledOnce()
    expect(Array.from(container.querySelectorAll<HTMLAnchorElement>('.jenkins-rail-links a')).map(link => link.getAttribute('href'))).toEqual([
      '/builds',
      '/templates',
      '/projects',
      '/vcs-roots',
      '/api-tokens',
    ])
    expect(container.querySelectorAll('#buildQueue .jenkins-rail-queue-item')).toHaveLength(4)
    expect(container.querySelectorAll('#buildQueue .jenkins-rail-status-dot')).toHaveLength(4)
    expect(container.querySelector('#buildQueue .jenkins-rail-status-dot.running')).not.toBeNull()
    expect(container.querySelectorAll('#buildQueue .jenkins-rail-status-dot.pending')).toHaveLength(3)
    expect(container.querySelector('a[href="/builds/101"]')).not.toBeNull()
    expect(container.textContent).toContain('Delta')
    expect(container.querySelector('#buildQueue .jenkins-rail-panel-title')?.textContent).toMatch(/\(4\)$/)
    expect(container.querySelector('#buildQueue .jenkins-rail-panel-count')?.textContent).toBe('builtin 1 / 4')
    expect(container.querySelectorAll('.jenkins-rail-agent-item')).toHaveLength(0)
    expect(container.querySelector('.jenkins-rail-capacity-progress')).toBeNull()
    expect(container.querySelectorAll('.jenkins-rail-history-item')).toHaveLength(6)
    expect(container.querySelector('.jenkins-rail-history-link')?.getAttribute('href')).toBe('/builds/201')
    expect(container.querySelector('.jenkins-rail-history-item')?.textContent).not.toContain('· main')
    expect(container.querySelector('.jenkins-rail-history-item')?.textContent).toContain('2026-07-21 18:12:00 ')
    for (const tone of ['success', 'unstable', 'failed', 'running', 'cancelled']) {
      expect(container.querySelector(`.jenkins-rail-history-status.${tone}`)).not.toBeNull()
    }
    expect(container.querySelector('.jenkins-rail-queue-progress progress')).not.toBeNull()
    expect(container.querySelectorAll('.jenkins-rail-history-rebuild')).toHaveLength(0)

    const queueToggle = container.querySelector<HTMLButtonElement>('.jenkins-rail-panel-toggle')!
    act(() => queueToggle.click())
    expect(queueToggle.getAttribute('aria-expanded')).toBe('false')
    expect(container.querySelector('#buildQueue .jenkins-rail-queue-list')).toBeNull()
    expect(localStorage.getItem('buildworld.jenkins.pane.buildQueue.collapsed')).toBe('true')

    await act(async () => {
      root.render(<MemoryRouter><JenkinsHomeRail editable /></MemoryRouter>)
    })
    expect(container.querySelector('a[href="/projects/new"]')).not.toBeNull()
  })

  it('keeps settings in the Jenkins masthead instead of duplicating it in the rail', async () => {
    await render()

    expect(container.querySelector('a[href="/settings"]')).toBeNull()
    expect(container.querySelector('a[href="/templates"]')).not.toBeNull()
  })

  it('refreshes every five seconds like Jenkins and pauses while hidden', async () => {
    vi.useFakeTimers()
    listBuildQueue.mockReset().mockResolvedValueOnce(queue).mockResolvedValue([])
    await render(true)
    expect(listBuildQueue).toHaveBeenCalledTimes(1)
    expect(listAgents).not.toHaveBeenCalled()

    await act(async () => {
      await vi.advanceTimersByTimeAsync(4999)
    })
    expect(listBuildQueue).toHaveBeenCalledTimes(1)

    await act(async () => {
      await vi.advanceTimersByTimeAsync(1)
    })
    expect(listBuildQueue).toHaveBeenCalledTimes(2)
    await act(async () => {
      await vi.advanceTimersByTimeAsync(5000)
    })
    expect(listBuildQueue).toHaveBeenCalledTimes(3)

    Object.defineProperty(document, 'visibilityState', { configurable: true, value: 'hidden' })
    act(() => document.dispatchEvent(new Event('visibilitychange')))
    await act(async () => {
      await vi.advanceTimersByTimeAsync(9000)
    })
    expect(listBuildQueue).toHaveBeenCalledTimes(3)

    Object.defineProperty(document, 'visibilityState', { configurable: true, value: 'visible' })
    await act(async () => {
      document.dispatchEvent(new Event('visibilitychange'))
      await Promise.resolve()
    })
    expect(listBuildQueue).toHaveBeenCalledTimes(4)
    expect(listAgents).not.toHaveBeenCalled()
  })

  it('shows an actionable error and retries the queue request', async () => {
    listBuildQueue.mockRejectedValueOnce(new Error('control plane offline')).mockResolvedValueOnce([])
    await render(true)

    expect(container.querySelector('[role="alert"]')?.textContent).toContain('control plane offline')
    const retry = container.querySelector<HTMLButtonElement>('.jenkins-rail-retry')!
    await act(async () => retry.click())

    expect(listBuildQueue).toHaveBeenCalledTimes(2)
    expect(listAgents).not.toHaveBeenCalled()
    expect(container.querySelector('[role="alert"]')).toBeNull()
    expect(container.querySelector('.jenkins-rail-empty')).not.toBeNull()
  })

  it('rebuilds a recent project only after confirmation and opens the queued build', async () => {
    let resolveRetry!: (build: { id: number }) => void
    const retryRequest = new Promise<{ id: number }>(resolve => { resolveRetry = resolve })
    retryBuild.mockReturnValue(retryRequest)
    await render(true)

    const alphaRebuild = container.querySelector<HTMLButtonElement>('[data-build-id="201"]')!
    const betaRebuild = container.querySelector<HTMLButtonElement>('[data-build-id="202"]')!
    expect(alphaRebuild.disabled).toBe(false)
    expect(betaRebuild.disabled).toBe(true)

    await act(async () => alphaRebuild.click())
    expect(dialogs.confirm).toHaveBeenCalledOnce()
    expect(retryBuild).not.toHaveBeenCalled()

    vi.mocked(dialogs.confirm).mockResolvedValueOnce(true)
    await act(async () => {
      alphaRebuild.click()
      await Promise.resolve()
    })
    expect(retryBuild).toHaveBeenCalledWith(201)
    expect(alphaRebuild.getAttribute('aria-busy')).toBe('true')
    expect(container.querySelector('output[aria-label="current-location"]')?.textContent).toBe('/')

    await act(async () => {
      resolveRetry({ id: 301 })
      await retryRequest
    })
    expect(dialogs.notify).toHaveBeenCalledWith(expect.stringContaining('Alpha #12'), 'success')
    expect(container.querySelector('output[aria-label="current-location"]')?.textContent).toBe('/builds/301')
  })

  it('reports a quick rebuild failure without leaving the dashboard', async () => {
    vi.mocked(dialogs.confirm).mockResolvedValueOnce(true)
    retryBuild.mockRejectedValueOnce(new Error('queue offline'))
    await render(true)

    await act(async () => container.querySelector<HTMLButtonElement>('[data-build-id="201"]')!.click())

    expect(dialogs.notify).toHaveBeenCalledWith('queue offline')
    expect(container.querySelector('output[aria-label="current-location"]')?.textContent).toBe('/')
  })
})
