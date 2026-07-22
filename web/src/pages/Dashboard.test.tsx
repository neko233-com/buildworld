// @vitest-environment jsdom

import { act, useEffect, type ReactNode } from 'react'
import { createRoot, type Root } from 'react-dom/client'
import { MemoryRouter, useLocation } from 'react-router-dom'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { api } from '../api'
import { dialogs } from '../components/AppDialogs'
import { useI18n } from '../i18n'
import Dashboard from './Dashboard'

vi.mock('../api', () => ({
  api: {
    listProjects: vi.fn(),
    listProjectBuildOverviews: vi.fn(),
    listProjectGroups: vi.fn(),
    listBuildQueue: vi.fn(),
    listAgents: vi.fn(),
    setProjectFlags: vi.fn(),
    getProject: vi.fn(),
    validateProject: vi.fn(),
    triggerBuild: vi.fn(),
  },
}))

vi.mock('../components/AppDialogs', () => ({
  dialogs: { confirm: vi.fn(), notify: vi.fn() },
}))

vi.mock('../authz', () => ({ canEdit: () => true, isAdmin: () => false }))

const listProjects = vi.mocked(api.listProjects)
const listProjectBuildOverviews = vi.mocked(api.listProjectBuildOverviews)
const listProjectGroups = vi.mocked(api.listProjectGroups)
const listBuildQueue = vi.mocked(api.listBuildQueue)
const listAgents = vi.mocked(api.listAgents)
const setProjectFlags = vi.mocked(api.setProjectFlags)
const getProject = vi.mocked(api.getProject)
const validateProject = vi.mocked(api.validateProject)
const triggerBuild = vi.mocked(api.triggerBuild)
const actEnvironment = globalThis as typeof globalThis & { IS_REACT_ACT_ENVIRONMENT: boolean }

const projects = [
  { id: 3, name: 'Zulu', default_branch: 'main', group_id: 20, favorite: false, quick_access: false },
  { id: 1, name: 'Alpha', default_branch: 'develop', group_id: 20, favorite: true, quick_access: false },
  { id: 2, name: 'Beta', default_branch: 'main', group_id: 10, favorite: false, quick_access: true, config: 'stages: []' },
]

const overviews = [
  {
    project_id: 1,
    latest: { id: 112, number: 12, status: 'running', duration_ms: 65_000, started_at: '2026-07-21T10:12:00Z' },
    last_success: { id: 111, number: 11, status: 'success', duration_ms: 60_000, started_at: '2026-07-21T10:11:00Z' },
    last_failure: { id: 110, number: 10, status: 'failed', duration_ms: 55_000, started_at: '2026-07-21T10:10:00Z' },
    recent_statuses: ['running', 'success', 'failed'],
  },
  {
    project_id: 2,
    latest: { id: 204, number: 4, status: 'success', duration_ms: 2_500, started_at: '2026-07-21T10:04:00Z' },
    last_success: { id: 204, number: 4, status: 'success', duration_ms: 2_500, started_at: '2026-07-21T10:04:00Z' },
    last_failure: null,
    recent_statuses: ['success'],
  },
  { project_id: 3, latest: null, last_success: null, last_failure: null, recent_statuses: [] },
]

const groups = [
  { id: 20, name: '服务端' },
  { id: 10, name: '客户端' },
]

function LocationProbe() {
  return <output aria-label="current-location">{useLocation().pathname}</output>
}

function ChineseLocale({ children }: { children: ReactNode }) {
  const { locale, changeLocale } = useI18n()
  useEffect(() => {
    if (locale !== 'zh-CN') changeLocale('zh-CN')
  }, [changeLocale, locale])
  return locale === 'zh-CN' ? children : null
}

describe('Dashboard Jenkins job view', () => {
  let container: HTMLDivElement
  let root: Root

  beforeEach(() => {
    actEnvironment.IS_REACT_ACT_ENVIRONMENT = true
    Object.defineProperty(document, 'visibilityState', { configurable: true, value: 'visible' })
    localStorage.setItem('locale', 'zh-CN')
    listProjects.mockReset().mockResolvedValue(projects)
    listProjectBuildOverviews.mockReset().mockResolvedValue(overviews)
    listProjectGroups.mockReset().mockResolvedValue(groups)
    listBuildQueue.mockReset().mockResolvedValue([])
    listAgents.mockReset().mockResolvedValue([])
    setProjectFlags.mockReset().mockResolvedValue({})
    getProject.mockReset().mockResolvedValue({ ...projects[1], config: 'stages:\n  - name: build' })
    validateProject.mockReset().mockResolvedValue({
      valid: true,
      format: 'yaml',
      stages: 1,
      steps: 1,
      parameters: [],
      allow_long_running: false,
    })
    triggerBuild.mockReset().mockResolvedValue({ id: 901 })
    vi.mocked(dialogs.confirm).mockReset()
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

  async function renderDashboard() {
    await act(async () => {
      root.render(
        <MemoryRouter initialEntries={['/']}>
          <ChineseLocale>
            <Dashboard />
            <LocationProbe />
          </ChineseLocale>
        </MemoryRouter>,
      )
    })
  }

  function projectRows(): HTMLTableRowElement[] {
    return Array.from(container.querySelectorAll<HTMLTableRowElement>('table tbody tr'))
  }

  function rowFor(projectID: number): HTMLTableRowElement {
    const link = container.querySelector<HTMLAnchorElement>(`a[href="/projects/${projectID}"]`)
    const row = link?.closest('tr')
    expect(row).not.toBeNull()
    return row as HTMLTableRowElement
  }

  function visibleProjectNames(): string[] {
    return projectRows().map(row => row.querySelector<HTMLAnchorElement>('a[href^="/projects/"] strong')?.textContent?.trim() || '')
  }

  function buttonNamed(name: string): HTMLButtonElement {
    const button = Array.from(container.querySelectorAll<HTMLButtonElement>('button'))
      .find(candidate => candidate.textContent?.trim() === name || candidate.getAttribute('aria-label') === name)
    expect(button).not.toBeUndefined()
    return button as HTMLButtonElement
  }

  it('shows every project in Jenkins columns with build history, duration, links, and name sorting', async () => {
    await renderDashboard()

    expect(listProjects).toHaveBeenCalledOnce()
    expect(listProjectBuildOverviews).toHaveBeenCalledOnce()
    expect(listProjectGroups).toHaveBeenCalledOnce()
    expect(listBuildQueue).toHaveBeenCalledOnce()
    expect(listAgents).toHaveBeenCalledOnce()

    const table = container.querySelector('table')
    expect(table).not.toBeNull()
    const headings = Array.from(table!.querySelectorAll('thead th')).map(cell => cell.textContent?.replace(/\s+/g, ' ').trim())
    expect(headings[0]).toBe('S')
    expect(headings[1]).toBe('W')
    expect(headings[2]).toContain('名称')
    expect(headings.slice(3)).toEqual(['最近成功构建', '最近失败构建', '耗时', ''])
    expect(table!.querySelector('[aria-label="状态"]')?.textContent).toBe('S')
    expect(table!.querySelector('[aria-label="成功率"]')?.textContent).toBe('W')
    const nameSort = table!.querySelector<HTMLButtonElement>('th[aria-sort="ascending"] button')
    expect(nameSort).not.toBeNull()

    expect(visibleProjectNames()).toEqual(['Alpha', 'Beta', 'Zulu'])

    const alpha = rowFor(1)
    expect(alpha.cells[3].textContent).toContain('#11')
    expect(alpha.cells[3].querySelector('a')?.getAttribute('href')).toBe('/builds/111')
    expect(alpha.cells[4].textContent).toContain('#10')
    expect(alpha.cells[4].querySelector('a')?.getAttribute('href')).toBe('/builds/110')
    expect(alpha.cells[5].textContent).toBe('1m 5s')

    const beta = rowFor(2)
    expect(beta.cells[3].textContent).toContain('#4')
    expect(beta.cells[4].textContent).toBe('无')
    expect(beta.cells[5].textContent).toBe('2.5s')

    const zulu = rowFor(3)
    expect(zulu.cells[3].textContent).toBe('无')
    expect(zulu.cells[4].textContent).toBe('无')
    expect(zulu.cells[5].textContent).toBe('-')
    expect(zulu.querySelector('[role="img"][aria-label="暂无构建活动。"]')).not.toBeNull()
    expect(zulu.querySelector('[role="img"][aria-label="Zulu 成功率"]')).not.toBeNull()

    act(() => nameSort!.click())
    expect(visibleProjectNames()).toEqual(['Zulu', 'Beta', 'Alpha'])
    expect(container.querySelector('th[aria-sort="descending"]')).not.toBeNull()
  })

  it('filters favorite, quick-access, and project-group views and persists both project flags', async () => {
    await renderDashboard()

    act(() => buttonNamed('收藏项目').click())
    expect(visibleProjectNames()).toEqual(['Alpha'])

    act(() => buttonNamed('快速访问').click())
    expect(visibleProjectNames()).toEqual(['Beta'])

    act(() => buttonNamed('服务端').click())
    expect(visibleProjectNames()).toEqual(['Alpha', 'Zulu'])

    await act(async () => buttonNamed('收藏 Zulu').click())
    expect(setProjectFlags).toHaveBeenCalledWith(3, true, false)

    await act(async () => buttonNamed('加入快速访问 Alpha').click())
    expect(setProjectFlags).toHaveBeenCalledWith(1, true, true)
  })

  it('keeps build action busy while validating and triggering, then opens the queued build', async () => {
    let resolveBuild!: (build: { id: number }) => void
    const pendingBuild = new Promise<{ id: number }>(resolve => { resolveBuild = resolve })
    triggerBuild.mockReturnValue(pendingBuild)
    await renderDashboard()

    const alphaBuild = buttonNamed('构建 Alpha')
    await act(async () => {
      alphaBuild.click()
      await Promise.resolve()
      await Promise.resolve()
      await Promise.resolve()
    })

    expect(getProject).not.toHaveBeenCalled()
    expect(validateProject).toHaveBeenCalledWith(1)
    expect(triggerBuild).toHaveBeenCalledWith(1)
    expect(alphaBuild.disabled).toBe(true)
    expect(alphaBuild.getAttribute('aria-busy')).toBe('true')
    for (const button of container.querySelectorAll<HTMLButtonElement>('button[aria-label^="构建 "]')) {
      expect(button.disabled).toBe(true)
    }

    await act(async () => {
      resolveBuild({ id: 901 })
      await pendingBuild
      await Promise.resolve()
    })

    expect(container.querySelector('output[aria-label="current-location"]')?.textContent).toBe('/builds/901')
    expect(alphaBuild.disabled).toBe(false)
    expect(alphaBuild.getAttribute('aria-busy')).toBe('false')
  })

  it('labels parameterized jobs and routes them without queueing', async () => {
    validateProject.mockImplementation(async id => ({
      valid: true,
      format: 'yaml',
      stages: 1,
      steps: 1,
      parameters: id === 1 ? [{ name: 'ENV', type: 'choice', choices: ['dev'] }] : [],
      allow_long_running: false,
    }))
    await renderDashboard()

    await act(async () => buttonNamed('参数化构建 Alpha').click())

    expect(triggerBuild).not.toHaveBeenCalled()
    expect(container.querySelector('output[aria-label="current-location"]')?.textContent).toBe('/projects/1/build')
  })

  it('keeps disabled Jenkins jobs visible but prevents builds', async () => {
    listProjects.mockResolvedValue(projects.map(project => project.id === 2 ? { ...project, enabled: false } : project))
    await renderDashboard()

    const betaBuild = buttonNamed('构建 Beta')
    expect(betaBuild.disabled).toBe(true)
    expect(betaBuild.title).toBe('已禁用')
    betaBuild.click()
    expect(validateProject).not.toHaveBeenCalledWith(2)
    expect(triggerBuild).not.toHaveBeenCalledWith(2)
  })

  it('refreshes active jobs every two seconds and stops at a terminal state', async () => {
    vi.useFakeTimers()
    const terminalOverviews = overviews.map(overview => overview.project_id === 1
      ? { ...overview, latest: { ...overview.latest!, status: 'success' }, recent_statuses: ['success'] }
      : overview)
    listProjectBuildOverviews.mockReset()
      .mockResolvedValueOnce(overviews)
      .mockResolvedValue(terminalOverviews)

    await renderDashboard()
    expect(listProjectBuildOverviews).toHaveBeenCalledTimes(1)

    await act(async () => {
      await vi.advanceTimersByTimeAsync(2000)
    })
    expect(listProjectBuildOverviews).toHaveBeenCalledTimes(2)

    await act(async () => {
      await vi.advanceTimersByTimeAsync(4000)
    })
    expect(listProjectBuildOverviews).toHaveBeenCalledTimes(2)
  })

  it('clears active refresh when the dashboard unmounts', async () => {
    vi.useFakeTimers()
    await renderDashboard()
    expect(listProjectBuildOverviews).toHaveBeenCalledTimes(1)

    await act(async () => {
      root.render(<MemoryRouter><div>unmounted</div></MemoryRouter>)
    })
    await act(async () => {
      await vi.advanceTimersByTimeAsync(4000)
    })
    expect(listProjectBuildOverviews).toHaveBeenCalledTimes(1)
  })
})
