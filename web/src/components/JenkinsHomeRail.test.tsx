// @vitest-environment jsdom

import { act } from 'react'
import { createRoot, type Root } from 'react-dom/client'
import { MemoryRouter } from 'react-router-dom'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { api } from '../api'
import { canEdit, isAdmin } from '../authz'
import JenkinsHomeRail from './JenkinsHomeRail'

vi.mock('../api', () => ({
  api: {
    listBuildQueue: vi.fn(),
    listAgents: vi.fn(),
  },
}))

vi.mock('../authz', () => ({ canEdit: vi.fn(), isAdmin: vi.fn() }))

const listBuildQueue = vi.mocked(api.listBuildQueue)
const listAgents = vi.mocked(api.listAgents)
const mockedCanEdit = vi.mocked(canEdit)
const mockedIsAdmin = vi.mocked(isAdmin)
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
    mockedIsAdmin.mockReset().mockReturnValue(false)
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

  it('matches Jenkins shortcuts, limits live rows, and supports collapsing panels', async () => {
    await render()

    expect(mockedCanEdit).toHaveBeenCalledOnce()
    expect(mockedIsAdmin).toHaveBeenCalledOnce()
    expect(container.querySelector('a[href="/projects/new"]')).toBeNull()
    expect(container.querySelector('a[href="/templates"]')).toBeNull()
    expect(container.querySelector('a[href="/settings"]')).toBeNull()
    for (const href of ['/builds', '/projects', '/vcs-roots', '/build-queue', '/agents']) {
      expect(container.querySelector(`a[href="${href}"]`)).not.toBeNull()
    }
    expect(container.querySelectorAll('.jenkins-rail-queue-item')).toHaveLength(3)
    expect(container.querySelector('a[href="/builds/101"]')).not.toBeNull()
    expect(container.textContent).not.toContain('Delta')
    expect(container.querySelectorAll('.jenkins-rail-agent-item')).toHaveLength(3)
    expect(container.textContent).not.toContain('Worker D')
    expect(container.querySelector('.jenkins-rail-panel-count')?.textContent).toBe('4')
    expect(container.querySelector('.jenkins-rail-capacity-value')?.textContent).toBe('3 / 10')
    expect(container.querySelector('progress')?.getAttribute('value')).toBe('3')

    const queueToggle = container.querySelector<HTMLButtonElement>('.jenkins-rail-panel-toggle')!
    act(() => queueToggle.click())
    expect(queueToggle.getAttribute('aria-expanded')).toBe('false')
    expect(container.querySelector('.jenkins-rail-queue-list')).toBeNull()

    await act(async () => {
      root.render(<MemoryRouter><JenkinsHomeRail editable /></MemoryRouter>)
    })
    expect(container.querySelector('a[href="/projects/new"]')).not.toBeNull()
  })

  it('gives administrators a visible settings shortcut', async () => {
    mockedIsAdmin.mockReturnValue(true)
    await render()

    const settings = container.querySelector<HTMLAnchorElement>('a[href="/settings"]')
    expect(settings).not.toBeNull()
    expect(settings?.querySelector('.jenkins-rail-link-label')?.textContent?.trim()).toBeTruthy()
    expect(container.querySelector('a[href="/templates"]')).toBeNull()
  })

  it('adapts between active and idle polling and pauses while hidden', async () => {
    vi.useFakeTimers()
    listBuildQueue.mockReset().mockResolvedValueOnce(queue).mockResolvedValue([])
    listAgents.mockReset().mockResolvedValueOnce(agents).mockResolvedValue([])
    await render(true)
    expect(listBuildQueue).toHaveBeenCalledTimes(1)
    expect(listAgents).toHaveBeenCalledTimes(1)

    await act(async () => {
      await vi.advanceTimersByTimeAsync(3000)
    })
    expect(listBuildQueue).toHaveBeenCalledTimes(2)
    expect(listAgents).toHaveBeenCalledTimes(2)

    await act(async () => {
      await vi.advanceTimersByTimeAsync(4999)
    })
    expect(listBuildQueue).toHaveBeenCalledTimes(2)
    await act(async () => {
      await vi.advanceTimersByTimeAsync(1)
    })
    expect(listBuildQueue).toHaveBeenCalledTimes(3)
    expect(listAgents).toHaveBeenCalledTimes(3)

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
    expect(listAgents).toHaveBeenCalledTimes(4)
  })

  it('shows an actionable error and retries both requests', async () => {
    listBuildQueue.mockRejectedValueOnce(new Error('control plane offline')).mockResolvedValueOnce([])
    listAgents.mockResolvedValueOnce([]).mockResolvedValueOnce([])
    await render(true)

    expect(container.querySelector('[role="alert"]')?.textContent).toContain('control plane offline')
    const retry = container.querySelector<HTMLButtonElement>('.jenkins-rail-retry')!
    await act(async () => retry.click())

    expect(listBuildQueue).toHaveBeenCalledTimes(2)
    expect(listAgents).toHaveBeenCalledTimes(2)
    expect(container.querySelector('[role="alert"]')).toBeNull()
    expect(container.querySelector('.jenkins-rail-empty')).not.toBeNull()
  })
})
