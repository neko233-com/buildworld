// @vitest-environment jsdom

import { act } from 'react'
import { createRoot, type Root } from 'react-dom/client'
import { MemoryRouter } from 'react-router-dom'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { api } from '../api'
import { dialogs } from '../components/AppDialogs'
import Dashboard from './Dashboard'

vi.mock('../api', () => ({
  api: {
    listProjects: vi.fn(),
    listBuilds: vi.fn(),
    listAgents: vi.fn(),
    retryBuild: vi.fn(),
  },
}))

vi.mock('../components/AppDialogs', () => ({
  dialogs: { confirm: vi.fn(), notify: vi.fn() },
}))

vi.mock('../authz', () => ({ canEdit: () => true }))

const listProjects = vi.mocked(api.listProjects)
const listBuilds = vi.mocked(api.listBuilds)
const listAgents = vi.mocked(api.listAgents)
const retryBuild = vi.mocked(api.retryBuild)
const confirm = vi.mocked(dialogs.confirm)
const actEnvironment = globalThis as typeof globalThis & { IS_REACT_ACT_ENVIRONMENT: boolean }

const projects = [
  { id: 1, name: 'Alpha' },
  { id: 2, name: 'Beta' },
  { id: 3, name: 'Gamma' },
]

const builds = [
  { id: 101, project_id: 1, number: 8, status: 'success', branch: 'main', started_at: '2026-07-21T10:08:00Z' },
  { id: 100, project_id: 1, number: 7, status: 'failed', branch: 'main', started_at: '2026-07-21T10:07:00Z' },
  { id: 99, project_id: 2, number: 4, status: 'failed', branch: 'release', started_at: '2026-07-21T10:06:00Z' },
  { id: 98, project_id: 3, number: 2, status: 'running', branch: 'main', started_at: '2026-07-21T10:05:00Z' },
  { id: 97, project_id: 404, number: 1, status: 'success', branch: 'main', started_at: '2026-07-21T10:04:00Z' },
]

describe('Dashboard recent projects', () => {
  let container: HTMLDivElement
  let root: Root

  beforeEach(() => {
    actEnvironment.IS_REACT_ACT_ENVIRONMENT = true
    localStorage.setItem('locale', 'zh-CN')
    listProjects.mockReset().mockResolvedValue(projects)
    listBuilds.mockReset().mockResolvedValue(builds)
    listAgents.mockReset().mockResolvedValue([])
    retryBuild.mockReset().mockResolvedValue({ id: 501 })
    confirm.mockReset().mockResolvedValue(false)
    vi.mocked(dialogs.notify).mockReset()
    container = document.createElement('div')
    document.body.appendChild(container)
    root = createRoot(container)
  })

  afterEach(() => {
    act(() => root.unmount())
    container.remove()
    localStorage.clear()
    actEnvironment.IS_REACT_ACT_ENVIRONMENT = false
  })

  it('deduplicates projects by their latest build and keeps rebuilds explicit', async () => {
    await act(async () => {
      root.render(<MemoryRouter><Dashboard /></MemoryRouter>)
    })

    expect(listBuilds).toHaveBeenCalledWith(60)
    const rows = container.querySelectorAll('.recent-project-row')
    expect(rows).toHaveLength(3)
    expect(rows[0].textContent).toContain('Alpha')
    expect(rows[0].textContent).toContain('#8')
    expect(rows[0].textContent).not.toContain('#7')
    expect(container.querySelector('.recent-projects-panel')?.textContent).not.toContain('#404')
    expect(container.querySelector('.recent-project-row[tabindex]')).toBeNull()

    const alphaRebuild = container.querySelector<HTMLButtonElement>('.recent-project-row button[aria-label$="Alpha"]')
    const gammaRebuild = container.querySelector<HTMLButtonElement>('.recent-project-row button[aria-label$="Gamma"]')
    expect(alphaRebuild).not.toBeNull()
    expect(gammaRebuild?.disabled).toBe(true)

    await act(async () => alphaRebuild?.click())
    expect(confirm).toHaveBeenCalledTimes(1)
    expect(confirm.mock.calls[0][0]).toContain('Alpha')
    expect(confirm.mock.calls[0][0]).toContain('#8')
    expect(retryBuild).not.toHaveBeenCalled()

    confirm.mockResolvedValueOnce(true)
    await act(async () => alphaRebuild?.click())
    expect(retryBuild).toHaveBeenCalledWith(101)
  })
})
