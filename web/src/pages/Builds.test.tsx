// @vitest-environment jsdom

import { act } from 'react'
import { createRoot, type Root } from 'react-dom/client'
import { MemoryRouter } from 'react-router-dom'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { api } from '../api'
import Builds from './Builds'

vi.mock('../api', () => ({
  api: {
    searchBuilds: vi.fn(),
    listProjects: vi.fn(),
    getBuild: vi.fn(),
    retryBuild: vi.fn(),
    pinBuild: vi.fn(),
  },
}))

vi.mock('../authz', () => ({ canEdit: () => true }))
vi.mock('../components/AppDialogs', () => ({ dialogs: { notify: vi.fn() } }))

const searchBuilds = vi.mocked(api.searchBuilds)
const listProjects = vi.mocked(api.listProjects)
const actEnvironment = globalThis as typeof globalThis & { IS_REACT_ACT_ENVIRONMENT: boolean }

const activeResult = {
  version: 1,
  items: [
    { id: 11, project_id: 1, number: 11, status: 'running', trigger: 'manual', branch: 'main' },
    { id: 12, project_id: 1, number: 12, status: 'pending_approval', trigger: 'manual', branch: 'main' },
  ],
  total: 2,
  limit: 25,
  offset: 0,
}

const terminalResult = {
  ...activeResult,
  version: 2,
  items: activeResult.items.map(build => ({ ...build, status: 'success' })),
}

describe('Builds active refresh', () => {
  let container: HTMLDivElement
  let root: Root

  beforeEach(() => {
    actEnvironment.IS_REACT_ACT_ENVIRONMENT = true
    Object.defineProperty(document, 'visibilityState', { configurable: true, value: 'visible' })
    localStorage.setItem('locale', 'zh-CN')
    searchBuilds.mockReset().mockResolvedValue(activeResult)
    listProjects.mockReset().mockResolvedValue([{ id: 1, name: 'Alpha' }])
    vi.mocked(api.getBuild).mockReset().mockResolvedValue({ id: 11, project_id: 1, number: 11, branch: 'main', parameters: '{"target":"prod"}' })
    vi.mocked(api.retryBuild).mockReset().mockResolvedValue({ id: 21 })
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

  async function renderBuilds() {
    await act(async () => {
      root.render(<MemoryRouter initialEntries={['/builds']}><Builds /></MemoryRouter>)
    })
  }

  it('reloads while active rows exist and stops after all visible rows become terminal', async () => {
    vi.useFakeTimers()
    searchBuilds.mockReset().mockResolvedValueOnce(activeResult).mockResolvedValue(terminalResult)
    await renderBuilds()
    expect(searchBuilds).toHaveBeenCalledTimes(1)

    await act(async () => {
      await vi.advanceTimersByTimeAsync(2000)
    })
    expect(searchBuilds).toHaveBeenCalledTimes(2)

    await act(async () => {
      await vi.advanceTimersByTimeAsync(4000)
    })
    expect(searchBuilds).toHaveBeenCalledTimes(2)
  })

  it('renders Jenkins status icons, table caption, and row actions', async () => {
    await renderBuilds()

    expect(container.querySelector('.jenkins-build-history-page')).not.toBeNull()
    expect(container.querySelector('caption')?.textContent).toMatch(/构建历史|Build History/)
    expect(container.querySelector('.jenkins-page-heading-count')?.textContent).toBe('2')
    expect(container.querySelector('.jenkins-build-state.running svg.timeline-spinner')).not.toBeNull()
    expect(container.querySelector('.jenkins-build-state.pending > i')).not.toBeNull()
    expect(container.querySelector('.builds-table .build-status')).toBeNull()
    expect(container.querySelectorAll('.builds-table tbody .row-actions > *')).toHaveLength(6)
  })

  it('clears active polling when the page unmounts', async () => {
    vi.useFakeTimers()
    await renderBuilds()
    expect(searchBuilds).toHaveBeenCalledTimes(1)

    await act(async () => {
      root.render(<MemoryRouter><div>unmounted</div></MemoryRouter>)
    })
    await act(async () => {
      await vi.advanceTimersByTimeAsync(4000)
    })
    expect(searchBuilds).toHaveBeenCalledTimes(1)
  })

  it('loads a terminal build summary and requires Replay confirmation', async () => {
    searchBuilds.mockReset().mockResolvedValue(terminalResult)
    await renderBuilds()

    const replay = container.querySelector<HTMLButtonElement>('.builds-table tbody tr .row-actions button')
    expect(replay).not.toBeNull()
    expect(replay?.getAttribute('aria-label')).toContain('Alpha #11')
    await act(async () => replay?.click())
    expect(api.retryBuild).not.toHaveBeenCalled()

    await act(async () => {
      await Promise.resolve()
      await Promise.resolve()
    })
    const dialog = document.querySelector<HTMLElement>('.jenkins-replay-dialog')
    expect(api.getBuild).toHaveBeenCalledWith(11)
    expect(dialog?.textContent).toContain('Alpha #11')
    expect(dialog?.textContent).toContain('prod')

    await act(async () => dialog?.querySelector<HTMLButtonElement>('.jenkins-replay-submit')?.click())
    expect(api.retryBuild).toHaveBeenCalledWith(11)
  })
})
