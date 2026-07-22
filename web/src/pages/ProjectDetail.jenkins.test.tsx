// @vitest-environment jsdom

import { act } from 'react'
import { createRoot, type Root } from 'react-dom/client'
import { MemoryRouter, Route, Routes, useLocation } from 'react-router-dom'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { api } from '../api'
import { dialogs } from '../components/AppDialogs'
import ProjectDetail from './ProjectDetail'

vi.mock('../api', () => ({
  api: {
    getProject: vi.fn(),
    searchBuilds: vi.fn(),
    listProjectGroups: vi.fn(),
    validateProject: vi.fn(),
    triggerBuild: vi.fn(),
    stopBuild: vi.fn(),
    deleteProject: vi.fn(),
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
  name: 'server-game-go',
  description: 'GAME Server',
  repo_url: 'https://git.example.test/server-game-go.git',
  default_branch: 'main',
  group_id: 3,
  config: 'jobs:\n  build:\n    steps: []',
}

const builds = [
  { id: 519, project_id: 7, number: 519, status: 'running', branch: 'main', commit_sha: 'abc519', started_at: '2026-07-22T04:04:00Z' },
  { id: 518, project_id: 7, number: 518, status: 'success', branch: 'main', commit_sha: 'abc518', started_at: '2026-07-22T02:25:00Z', duration_ms: 45_000 },
  { id: 514, project_id: 7, number: 514, status: 'failed', branch: 'release', commit_sha: 'abc514', started_at: '2026-07-21T21:49:00Z', duration_ms: 22_000 },
]

function LocationProbe() {
  return <output aria-label="location">{useLocation().pathname}</output>
}

describe('ProjectDetail Jenkins Job status', () => {
  let container: HTMLDivElement
  let root: Root

  beforeEach(() => {
    ;(globalThis as any).IS_REACT_ACT_ENVIRONMENT = true
    localStorage.setItem('locale', 'en')
    vi.mocked(api.getProject).mockResolvedValue(project)
    vi.mocked(api.searchBuilds).mockResolvedValue({ version: 1, items: builds, total: builds.length, limit: 100, offset: 0 })
    vi.mocked(api.listProjectGroups).mockResolvedValue([{ id: 3, name: '服务器 Go' }])
    vi.mocked(api.validateProject).mockResolvedValue({ valid: true, format: 'yaml', stages: 1, steps: 1, parameters: [], allow_long_running: false })
    vi.mocked(api.triggerBuild).mockResolvedValue({ id: 520 })
    vi.mocked(api.stopBuild).mockResolvedValue({})
    vi.mocked(api.deleteProject).mockResolvedValue({})
    vi.mocked(dialogs.confirm).mockResolvedValue(true)
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

  async function renderPage() {
    await act(async () => {
      root.render(<MemoryRouter initialEntries={['/projects/7']}><Routes>
        <Route path="/projects/:id" element={<><ProjectDetail /><LocationProbe /></>} />
        <Route path="/projects" element={<LocationProbe />} />
        <Route path="/builds/:id" element={<LocationProbe />} />
      </Routes></MemoryRouter>)
      await Promise.resolve()
      await Promise.resolve()
    })
  }

  function button(name: string): HTMLButtonElement {
    const match = Array.from(container.querySelectorAll<HTMLButtonElement>('button')).find(item => item.textContent?.trim() === name || item.getAttribute('aria-label') === name)
    expect(match).toBeDefined()
    return match as HTMLButtonElement
  }

  it('matches Jenkins Job navigation, permalinks, and clickable build history', async () => {
    await renderPage()

    expect(container.querySelector('.jenkins-job-layout')).not.toBeNull()
    expect(container.querySelector('a.active[href="/projects/7"]')?.textContent).toContain('Status')
    expect(container.querySelector('a[href="/projects/7/configure"]')?.textContent).toContain('Configure')
    expect(container.querySelector('a[href="/projects/7/configure#jenkins-configure-general"]')?.textContent).toContain('Rename')
    expect(container.querySelector('a[href="/projects/7/configure#jenkins-configure-pipeline"]')?.textContent).toContain('Stages')
    expect(container.querySelector('a[href="/builds?project=7"]')?.textContent).toContain('Changes')
    expect(container.querySelector('a[href="/builds/519"]')).not.toBeNull()
    expect(container.querySelector('a[href="/builds/518"]')).not.toBeNull()
    expect(container.querySelector('.jenkins-job-related')?.textContent).toContain('Last successful build')
    expect(container.querySelector('.jenkins-job-related')?.textContent).toContain('#518')
    expect(container.querySelector('.jenkins-job-related')?.textContent).toContain('#514')

    const filter = container.querySelector<HTMLInputElement>('input[aria-label="Filter builds..."]')!
    Object.getOwnPropertyDescriptor(HTMLInputElement.prototype, 'value')?.set?.call(filter, 'release')
    await act(async () => filter.dispatchEvent(new Event('input', { bubbles: true })))
    const filteredBuildList = container.querySelector('.jenkins-job-build-list')
    expect(filteredBuildList?.querySelector('a[href="/builds/514"]')).not.toBeNull()
    expect(filteredBuildList?.querySelector('a[href="/builds/518"]')).toBeNull()
  })

  it('groups build history by day and pages 30 loaded builds at a time', async () => {
    const today = new Date()
    today.setHours(12, 0, 0, 0)
    const yesterday = new Date(today)
    yesterday.setDate(today.getDate() - 1)
    const twoDaysAgo = new Date(today)
    twoDaysAgo.setDate(today.getDate() - 2)
    const history = Array.from({ length: 35 }, (_, index) => {
      const day = new Date(index < 4 ? today : index < 32 ? yesterday : twoDaysAgo)
      day.setHours(23 - (index % 20), index % 60)
      return {
        id: 1000 - index,
        project_id: 7,
        number: 100 - index,
        status: index === 0 ? 'running' : 'success',
        branch: 'main',
        commit_sha: `commit-${index}`,
        started_at: day.toISOString(),
        duration_ms: 1_000,
      }
    })
    vi.mocked(api.searchBuilds).mockResolvedValue({ version: 1, items: history, total: history.length, limit: 100, offset: 0 })
    await renderPage()

    const buildList = container.querySelector('.jenkins-job-build-list')!
    expect(buildList.querySelectorAll('.jenkins-job-build')).toHaveLength(30)
    expect(Array.from(buildList.querySelectorAll('.jenkins-job-build-group > h3')).map(item => item.textContent)).toEqual([
      'Today',
      yesterday.toLocaleDateString('en', { year: 'numeric', month: 'long', day: 'numeric' }),
    ])
    expect(buildList.querySelector('a[href="/builds/1000"]')).not.toBeNull()
    expect(buildList.querySelector('a[href="/builds/970"]')).toBeNull()

    const newer = button('Newer builds')
    const older = button('Older builds')
    expect(newer.disabled).toBe(true)
    expect(older.disabled).toBe(false)
    await act(async () => older.click())

    expect(buildList.querySelectorAll('.jenkins-job-build')).toHaveLength(5)
    expect(buildList.querySelector('a[href="/builds/1000"]')).toBeNull()
    expect(buildList.querySelector('a[href="/builds/970"]')).not.toBeNull()
    expect(Array.from(buildList.querySelectorAll('.jenkins-job-build-group > h3')).map(item => item.textContent)).toEqual([
      yesterday.toLocaleDateString('en', { year: 'numeric', month: 'long', day: 'numeric' }),
      twoDaysAgo.toLocaleDateString('en', { year: 'numeric', month: 'long', day: 'numeric' }),
    ])
    expect(newer.disabled).toBe(false)
    expect(older.disabled).toBe(true)
    expect(api.searchBuilds).toHaveBeenCalledTimes(1)

    const filter = container.querySelector<HTMLInputElement>('input[aria-label="Filter builds..."]')!
    Object.getOwnPropertyDescriptor(HTMLInputElement.prototype, 'value')?.set?.call(filter, '#99')
    await act(async () => filter.dispatchEvent(new Event('input', { bubbles: true })))
    expect(buildList.querySelectorAll('.jenkins-job-build')).toHaveLength(1)
    expect(buildList.querySelector('a[href="/builds/999"]')).not.toBeNull()

    Object.getOwnPropertyDescriptor(HTMLInputElement.prototype, 'value')?.set?.call(filter, '')
    await act(async () => filter.dispatchEvent(new Event('input', { bubbles: true })))
    expect(buildList.querySelectorAll('.jenkins-job-build')).toHaveLength(30)
    expect(buildList.querySelector('a[href="/builds/1000"]')).not.toBeNull()
    expect(buildList.querySelector('a[href="/builds/970"]')).toBeNull()
  })

  it('opens Build with Parameters for every parameterized Jenkins job', async () => {
    vi.mocked(api.validateProject).mockResolvedValue({ valid: true, format: 'yaml', stages: 1, steps: 1, parameters: [{ name: 'ENV', type: 'choice', default: 'dev' }], allow_long_running: false })
    await renderPage()

    await act(async () => button('Build Now').click())

    expect(container.querySelector('[role="dialog"][aria-label="custom-build"]')?.textContent).toBe('server-game-go')
    expect(api.triggerBuild).not.toHaveBeenCalled()
  })

  it('shows disabled Jenkins jobs and blocks Build Now', async () => {
    vi.mocked(api.getProject).mockResolvedValue({ ...project, enabled: false })
    await renderPage()

    const buildNow = button('Build Now')
    expect(buildNow.disabled).toBe(true)
    expect(buildNow.title).toBe('Disabled')
    expect(container.querySelector('.jenkins-job-heading')?.textContent).toContain('Disabled')
    await act(async () => buildNow.click())
    expect(api.validateProject).not.toHaveBeenCalled()
    expect(api.triggerBuild).not.toHaveBeenCalled()
  })

  it('stops active builds and deletes the job with confirmation', async () => {
    await renderPage()

    await act(async () => button('Stop build #519').click())
    expect(dialogs.confirm).toHaveBeenCalled()
    expect(api.stopBuild).toHaveBeenCalledWith(519)

    await act(async () => button('Delete Pipeline').click())
    expect(api.deleteProject).toHaveBeenCalledWith(7)
    expect(container.querySelector('output[aria-label="location"]')?.textContent).toBe('/projects')
  })
})
