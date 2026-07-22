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
    { id: 11, project_id: 1, number: 11, status: 'pending', trigger: 'manual', branch: 'main' },
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
})
