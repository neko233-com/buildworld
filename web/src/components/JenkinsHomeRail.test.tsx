// @vitest-environment jsdom

import { act } from 'react'
import { createRoot, type Root } from 'react-dom/client'
import { MemoryRouter } from 'react-router-dom'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { api } from '../api'
import { canEdit } from '../authz'
import JenkinsHomeRail from './JenkinsHomeRail'

vi.mock('../api', () => ({
  api: {
    listBuildQueue: vi.fn(),
    listAgents: vi.fn(),
  },
}))

vi.mock('../authz', () => ({ canEdit: vi.fn() }))

const listBuildQueue = vi.mocked(api.listBuildQueue)
const listAgents = vi.mocked(api.listAgents)
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

describe('JenkinsHomeRail', () => {
  let container: HTMLDivElement
  let root: Root

  beforeEach(() => {
    actEnvironment.IS_REACT_ACT_ENVIRONMENT = true
    Object.defineProperty(document, 'visibilityState', { configurable: true, value: 'visible' })
    mockedCanEdit.mockReset().mockReturnValue(false)
    listBuildQueue.mockReset().mockResolvedValue(queue)
    listAgents.mockReset().mockResolvedValue(agents)
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

  async function render(editable?: boolean) {
    await act(async () => {
      root.render(<MemoryRouter><JenkinsHomeRail editable={editable} /></MemoryRouter>)
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
    expect(Array.from(container.querySelectorAll<HTMLAnchorElement>('.jenkins-rail-links a')).map(link => link.getAttribute('href'))).toEqual([
      '/builds',
      '/templates',
      '/projects',
      '/vcs-roots',
    ])
    expect(container.querySelectorAll('.jenkins-rail-queue-item')).toHaveLength(4)
    expect(container.querySelector('a[href="/builds/101"]')).not.toBeNull()
    expect(container.textContent).toContain('Delta')
    expect(container.querySelector('#buildQueue .jenkins-rail-panel-title')?.textContent).toMatch(/\(4\)$/)
    expect(container.querySelectorAll('.jenkins-rail-agent-item')).toHaveLength(0)
    expect(container.querySelector('.jenkins-rail-panel-count')).toBeNull()
    expect(container.querySelector('progress')).toBeNull()

    const queueToggle = container.querySelector<HTMLButtonElement>('.jenkins-rail-panel-toggle')!
    act(() => queueToggle.click())
    expect(queueToggle.getAttribute('aria-expanded')).toBe('false')
    expect(container.querySelector('.jenkins-rail-queue-list')).toBeNull()
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
})
