// @vitest-environment jsdom

import { act } from 'react'
import { createRoot, type Root } from 'react-dom/client'
import { MemoryRouter } from 'react-router-dom'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { api } from '../api'
import BuildQueue from './BuildQueue'

vi.mock('../api', () => ({
  api: {
    listBuildQueue: vi.fn(),
    listPendingApprovals: vi.fn(),
    reorderBuildQueue: vi.fn(),
    stopBuild: vi.fn(),
    approveBuild: vi.fn(),
    rejectBuild: vi.fn(),
  },
}))

vi.mock('../authz', () => ({ canEdit: () => true }))
vi.mock('../components/AppDialogs', () => ({ dialogs: { confirm: vi.fn(), notify: vi.fn() } }))

const listBuildQueue = vi.mocked(api.listBuildQueue)
const listPendingApprovals = vi.mocked(api.listPendingApprovals)
const actEnvironment = globalThis as typeof globalThis & { IS_REACT_ACT_ENVIRONMENT: boolean }

const activeQueue = [
  { id: 1, build_id: 101, build_number: 11, project_id: 1, project_name: 'Alpha', status: 'running', branch: 'main', trigger: 'manual' },
  { id: 2, build_id: 102, build_number: 12, project_id: 2, project_name: 'Beta', status: 'queued', branch: 'main', trigger: 'manual' },
  { id: 3, build_id: 103, build_number: 13, project_id: 3, project_name: 'Gamma', status: 'pending_approval', branch: 'main', trigger: 'manual' },
]

describe('BuildQueue active refresh', () => {
  let container: HTMLDivElement
  let root: Root

  beforeEach(() => {
    actEnvironment.IS_REACT_ACT_ENVIRONMENT = true
    Object.defineProperty(document, 'visibilityState', { configurable: true, value: 'visible' })
    localStorage.setItem('locale', 'zh-CN')
    listBuildQueue.mockReset().mockResolvedValue(activeQueue)
    listPendingApprovals.mockReset().mockResolvedValue([])
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

  async function renderQueue() {
    await act(async () => {
      root.render(<MemoryRouter initialEntries={['/build-queue']}><BuildQueue /></MemoryRouter>)
    })
  }

  it('shows an animated running indicator, polls every two seconds, and stops when the queue is empty', async () => {
    vi.useFakeTimers()
    listBuildQueue.mockReset().mockResolvedValueOnce(activeQueue).mockResolvedValue([])
    await renderQueue()

    const spinner = container.querySelector('.queue-running-section svg.timeline-spinner')
    expect(spinner).not.toBeNull()
    expect(spinner?.getAttribute('aria-hidden')).toBe('true')
    expect(container.querySelector('.queue-running-pulse')).toBeNull()
    expect(container.textContent).toContain('Alpha')
    expect(listBuildQueue).toHaveBeenCalledTimes(1)

    await act(async () => {
      await vi.advanceTimersByTimeAsync(2000)
    })
    expect(listBuildQueue).toHaveBeenCalledTimes(2)

    await act(async () => {
      await vi.advanceTimersByTimeAsync(4000)
    })
    expect(listBuildQueue).toHaveBeenCalledTimes(2)
  })

  it('renders the waiting queue as a Jenkins table with status icons', async () => {
    await renderQueue()

    expect(container.querySelector('.jenkins-build-queue-page')).not.toBeNull()
    expect(container.querySelector('caption')?.textContent).toMatch(/构建进行中|Builds in Progress/)
    expect(container.querySelector('.jenkins-page-heading-count')?.textContent).toBe('2')
    expect(container.querySelectorAll('.build-queue-table tbody tr')).toHaveLength(2)
    expect(container.querySelectorAll('.build-queue-table .jenkins-build-state.pending > i')).toHaveLength(2)
    expect(container.querySelector('.build-queue-table .build-status')).toBeNull()
    expect(container.querySelector('a[href="/agents"]')).toBeNull()
  })

  it('keeps refresh feedback visible until the replacement queue arrives', async () => {
    await renderQueue()
    let resolveQueue!: (queue: typeof activeQueue) => void
    listBuildQueue.mockReturnValueOnce(new Promise(resolve => { resolveQueue = resolve }))
    const refresh = container.querySelector<HTMLButtonElement>('.jenkins-page-heading .secondary-command')!

    await act(async () => {
      refresh.click()
      await Promise.resolve()
    })
    expect(refresh.disabled).toBe(true)
    expect(refresh.getAttribute('aria-busy')).toBe('true')
    expect(refresh.querySelector('svg.timeline-spinner')).not.toBeNull()

    await act(async () => {
      resolveQueue(activeQueue)
      await Promise.resolve()
    })
    expect(refresh.disabled).toBe(false)
    expect(refresh.getAttribute('aria-busy')).toBe('false')
  })

  it('clears active polling when the page unmounts', async () => {
    vi.useFakeTimers()
    await renderQueue()
    expect(listBuildQueue).toHaveBeenCalledTimes(1)

    await act(async () => {
      root.render(<MemoryRouter><div>unmounted</div></MemoryRouter>)
    })
    await act(async () => {
      await vi.advanceTimersByTimeAsync(4000)
    })
    expect(listBuildQueue).toHaveBeenCalledTimes(1)
  })
})
