// @vitest-environment jsdom

import { act } from 'react'
import { createRoot, type Root } from 'react-dom/client'
import { MemoryRouter, Route, Routes, useLocation } from 'react-router-dom'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { api } from '../api'
import ProjectChanges from './ProjectChanges'

vi.mock('../api', () => ({
  api: {
    getProject: vi.fn(),
    listProjectChanges: vi.fn(),
    searchBuilds: vi.fn(),
    listProjectGroups: vi.fn(),
    validateProject: vi.fn(),
    triggerBuild: vi.fn(),
    stopBuild: vi.fn(),
    deleteProject: vi.fn(),
    updateProject: vi.fn(),
  },
}))

vi.mock('../authz', () => ({ canEdit: () => true }))
vi.mock('../components/AppDialogs', () => ({ dialogs: { confirm: vi.fn(), notify: vi.fn() } }))
vi.mock('../components/RunBuildDialog', () => ({
  default: ({ project }: any) => <div role="dialog" aria-label="custom-build">{project.name}</div>,
  normalizeBuildParameterDefinitions: (definitions: any[]) => definitions || [],
}))

const project = {
  id: 7,
  name: 'Weather',
  enabled: true,
  group_id: 3,
  repo_type: 'git',
  config: 'jobs:\n  build:\n    steps: []',
}

const builds = [
  { id: 519, project_id: 7, number: 519, status: 'running', branch: 'main', commit_sha: 'abcdef519', started_at: '2026-07-22T04:04:50Z' },
  { id: 518, project_id: 7, number: 518, status: 'success', branch: 'release', commit_sha: 'abcdef518', started_at: '2026-07-22T02:25:00Z', duration_ms: 45_000 },
]

const changes = [
  { build_id: 519, build_number: 519, status: 'running', commit_sha: 'abcdef5190123456789', branch: 'main', timestamp: '2026-07-22T04:04:50Z' },
  { build_id: 518, build_number: 518, status: 'success', commit_sha: 'abcdef5180123456789', branch: 'release', timestamp: '2026-07-22T02:25:00Z' },
]

function LocationProbe() {
  return <output aria-label="location">{useLocation().pathname}</output>
}

describe('ProjectChanges', () => {
  let container: HTMLDivElement
  let root: Root

  beforeEach(() => {
    ;(globalThis as any).IS_REACT_ACT_ENVIRONMENT = true
    localStorage.setItem('locale', 'en')
    vi.mocked(api.getProject).mockResolvedValue(project)
    vi.mocked(api.listProjectChanges).mockResolvedValue(changes)
    vi.mocked(api.searchBuilds).mockResolvedValue({ version: 1, items: builds, total: builds.length, limit: 100, offset: 0 })
    vi.mocked(api.listProjectGroups).mockResolvedValue([{ id: 3, name: 'Servers' }])
    vi.mocked(api.validateProject).mockResolvedValue({ valid: true, format: 'yaml', stages: 1, steps: 1, parameters: [], allow_long_running: false })
    vi.mocked(api.triggerBuild).mockResolvedValue({ id: 520 })
    vi.mocked(api.stopBuild).mockResolvedValue({})
    vi.mocked(api.deleteProject).mockResolvedValue({})
    vi.mocked(api.updateProject).mockImplementation(async (_id, payload) => ({ ...project, ...payload }))
    container = document.createElement('div')
    document.body.appendChild(container)
    root = createRoot(container)
  })

  afterEach(() => {
    act(() => root.unmount())
    container.remove()
    localStorage.clear()
    vi.clearAllMocks()
    ;(globalThis as any).IS_REACT_ACT_ENVIRONMENT = false
  })

  async function flushRequests() {
    await act(async () => {
      await Promise.resolve()
      await Promise.resolve()
      await Promise.resolve()
    })
  }

  async function renderPage() {
    await act(async () => {
      root.render(<MemoryRouter initialEntries={['/projects/7/changes']}><Routes>
        <Route path="/projects/:id/changes" element={<><ProjectChanges /><LocationProbe /></>} />
        <Route path="/projects" element={<LocationProbe />} />
        <Route path="/projects/:id/build" element={<LocationProbe />} />
        <Route path="/builds/:id" element={<LocationProbe />} />
      </Routes></MemoryRouter>)
    })
    await flushRequests()
  }

  function button(name: string): HTMLButtonElement {
    const match = Array.from(container.querySelectorAll<HTMLButtonElement>('button')).find(item => item.textContent?.trim() === name || item.getAttribute('aria-label') === name)
    expect(match).toBeDefined()
    return match as HTMLButtonElement
  }

  it('matches Jenkins Changes navigation and renders recorded revisions by build', async () => {
    await renderPage()

    expect(container.querySelector('.jenkins-job-layout')).not.toBeNull()
    expect(container.querySelector('a.active[href="/projects/7/changes"]')?.textContent).toContain('Changes')
    expect(container.querySelector('.jenkins-job-actions a[href="/projects/7"]')?.textContent).toContain('Status')
    expect(container.querySelector('.jenkins-job-actions a[href="/projects/7/configure"]')?.textContent).toContain('Configure')
    expect(container.querySelector('.jenkins-job-actions a[href*="jenkins-configure-general"]')).toBeNull()
    expect(button('Rename').tagName).toBe('BUTTON')
    expect(container.querySelector('a[href="/builds?project=7"]')?.textContent).toContain('Build History')
    expect(container.querySelector('.jenkins-changes-main > h1')?.textContent).toBe('Changes')

    const headings = Array.from(container.querySelectorAll('.jenkins-change-build h2')).map(item => item.textContent || '')
    expect(headings).toHaveLength(2)
    expect(headings[0]).toContain('#519')
    expect(headings[1]).toContain('#518')
    expect(container.querySelector('.jenkins-change-build a[href="/builds/519"]')).not.toBeNull()
    expect(container.querySelector('.jenkins-change-build time[datetime="2026-07-22T04:04:50Z"]')).not.toBeNull()
    expect(container.textContent).toContain('abcdef5190123456789')
    expect(container.textContent).toContain('release')
    expect(container.textContent).toContain('Succeeded')
    expect(container.textContent).not.toContain('Author')
    expect(api.listProjectChanges).toHaveBeenCalledWith(7)
  })

  it('keeps Build Now functional from the Changes action rail', async () => {
    await renderPage()

    await act(async () => button('Build Now').click())
    await flushRequests()

    expect(api.validateProject).toHaveBeenCalledWith(7)
    expect(api.triggerBuild).toHaveBeenCalledWith(7)
    expect(container.querySelector('output[aria-label="location"]')?.textContent).toBe('/builds/520')
  })

  it('labels and routes parameterized builds from the Changes action rail', async () => {
    vi.mocked(api.validateProject).mockResolvedValue({ valid: true, format: 'yaml', stages: 1, steps: 1, parameters: [{ name: 'ENV', type: 'choice', choices: ['dev'] }], allow_long_running: false })
    await renderPage()

    await act(async () => button('Build with Parameters').click())

    expect(api.triggerBuild).not.toHaveBeenCalled()
    expect(container.querySelector('output[aria-label="location"]')?.textContent).toBe('/projects/7/build')
  })

  it('keeps real Jenkins job-management actions available on Changes', async () => {
    await renderPage()

    await act(async () => button('Move').click())
    expect(document.querySelector('[role="dialog"][aria-label="Move project"]')).not.toBeNull()
    await act(async () => document.querySelector<HTMLButtonElement>('[role="dialog"][aria-label="Move project"] button[aria-label="Close"]')?.click())

    await act(async () => button('Pipeline Syntax').click())
    expect(document.querySelector('[role="dialog"][aria-label="Pipeline Syntax"] pre')?.textContent).toContain('jobs:')
  })

  it('shows an explicit empty state when no revisions were recorded', async () => {
    vi.mocked(api.listProjectChanges).mockResolvedValue([])
    await renderPage()

    expect(container.querySelector('.jenkins-changes-empty')?.textContent).toBe('No data')
    expect(container.querySelectorAll('.jenkins-change-build')).toHaveLength(0)
  })

  it('exposes a retry action when loading changes fails', async () => {
    vi.mocked(api.listProjectChanges).mockRejectedValueOnce(new Error('changes unavailable')).mockResolvedValueOnce(changes)
    await renderPage()

    expect(container.querySelector('.jenkins-changes-state.error')?.textContent).toContain('changes unavailable')
    await act(async () => button('Retry').click())
    await flushRequests()

    expect(api.listProjectChanges).toHaveBeenCalledTimes(2)
    expect(container.querySelectorAll('.jenkins-change-build')).toHaveLength(2)
  })
})
