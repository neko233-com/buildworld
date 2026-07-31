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
    listBuilds: vi.fn(),
    getStorageUsage: vi.fn(),
    listBuildQueue: vi.fn(),
    getBuildQueueCapacity: vi.fn(),
    listAgents: vi.fn(),
    setProjectFlags: vi.fn(),
    reorderProjects: vi.fn(),
    deleteProject: vi.fn(),
    deleteProjectGroup: vi.fn(),
    getProject: vi.fn(),
    validateProject: vi.fn(),
    triggerBuild: vi.fn(),
    retryBuild: vi.fn(),
  },
}))

vi.mock('../components/AppDialogs', () => ({
  dialogs: { confirm: vi.fn(), notify: vi.fn() },
}))

vi.mock('../authz', () => ({ canEdit: () => true, isAdmin: () => false }))

const listProjects = vi.mocked(api.listProjects)
const listProjectBuildOverviews = vi.mocked(api.listProjectBuildOverviews)
const listProjectGroups = vi.mocked(api.listProjectGroups)
const listBuilds = vi.mocked(api.listBuilds)
const getStorageUsage = vi.mocked(api.getStorageUsage)
const listBuildQueue = vi.mocked(api.listBuildQueue)
const getBuildQueueCapacity = vi.mocked(api.getBuildQueueCapacity)
const listAgents = vi.mocked(api.listAgents)
const setProjectFlags = vi.mocked(api.setProjectFlags)
const reorderProjects = vi.mocked(api.reorderProjects)
const deleteProject = vi.mocked(api.deleteProject)
const deleteProjectGroup = vi.mocked(api.deleteProjectGroup)
const getProject = vi.mocked(api.getProject)
const validateProject = vi.mocked(api.validateProject)
const triggerBuild = vi.mocked(api.triggerBuild)
const retryBuild = vi.mocked(api.retryBuild)
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
    listBuilds.mockReset().mockResolvedValue([
      { id: 112, project_id: 1, number: 12, status: 'running', started_at: '2026-07-21T10:12:00Z' },
      { id: 204, project_id: 2, number: 4, status: 'success', started_at: '2026-07-21T10:04:00Z' },
      { id: 110, project_id: 1, number: 10, status: 'failed', started_at: '2026-07-21T10:10:00Z' },
    ])
    getStorageUsage.mockReset().mockResolvedValue({
      executor: 'builtin',
      volume: 'D:',
      total_bytes: 1_000,
      used_bytes: 760,
      free_bytes: 240,
      used_percent: 76,
    })
    listBuildQueue.mockReset().mockResolvedValue([])
    getBuildQueueCapacity.mockReset().mockResolvedValue({ executor: 'builtin', max_concurrent_builds: 1 })
    listAgents.mockReset().mockResolvedValue([])
    setProjectFlags.mockReset().mockResolvedValue({})
    reorderProjects.mockReset().mockResolvedValue({})
    deleteProject.mockReset().mockResolvedValue({})
    deleteProjectGroup.mockReset().mockResolvedValue({})
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
    retryBuild.mockReset().mockResolvedValue({ id: 902 })
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

  it('shows every project in persisted Jenkins order with build history, duration, links, and reorder controls', async () => {
    await renderDashboard()

    expect(listProjects).toHaveBeenCalledTimes(2)
    expect(listProjectBuildOverviews).toHaveBeenCalledOnce()
    expect(listProjectGroups).toHaveBeenCalledOnce()
    expect(listBuilds).toHaveBeenCalledWith(30)
    expect(getStorageUsage).toHaveBeenCalledOnce()
    expect(listBuildQueue).toHaveBeenCalledOnce()
    expect(listAgents).not.toHaveBeenCalled()
    expect(container.querySelector('a[href="/agents"]')).toBeNull()
    expect(container.querySelectorAll('.jenkins-rail-history-item')).toHaveLength(3)
    expect(Array.from(container.querySelectorAll<HTMLAnchorElement>('.jenkins-rail-history-link')).map(link => link.getAttribute('href'))).toEqual([
      '/builds/112',
      '/builds/110',
      '/builds/204',
    ])
    expect(retryBuild).not.toHaveBeenCalled()

    const storage = container.querySelector('.jenkins-storage-monitor.warning')
    expect(storage?.textContent).toContain('本机缓存磁盘')
    expect(storage?.textContent).toContain('76%')
    expect(storage?.querySelector('progress')?.value).toBe(76)
    const toolbar = container.querySelector('.jenkins-home-toolbar')
    expect(toolbar?.querySelector('.jenkins-view-tabs')).not.toBeNull()
    expect(toolbar?.querySelector('.jenkins-icon-size')).toBeNull()

    const table = container.querySelector('table')
    expect(table).not.toBeNull()
    expect(table?.className).toBe('jenkins-job-table')
    const headings = Array.from(table!.querySelectorAll('thead th')).map(cell => cell.textContent?.replace(/\s+/g, ' ').trim())
    expect(headings[0]).toBe('ID')
    expect(headings[1]).toBe('S')
    expect(headings[2]).toContain('名称')
    expect(headings.slice(3)).toEqual(['最近成功构建', '最近失败构建', '耗时', ''])
    expect(table!.querySelector('[aria-label="项目 ID"]')?.textContent).toBe('ID')
    expect(table!.querySelector('[aria-label="状态"]')?.textContent).toBe('S')
    expect(table!.querySelector('[aria-label="成功率"]')).toBeNull()
    expect(table!.querySelector('th[aria-sort]')).toBeNull()

    expect(visibleProjectNames()).toEqual(['Zulu', 'Alpha', 'Beta'])

    const alpha = rowFor(1)
    expect(alpha.cells[0].textContent).toBe('1')
    expect(alpha.cells[3].textContent).toContain('#11')
    expect(alpha.cells[3].querySelector('a')?.getAttribute('href')).toBe('/builds/111')
    expect(alpha.cells[4].textContent).toContain('#10')
    expect(alpha.cells[4].querySelector('a')?.getAttribute('href')).toBe('/builds/110')
    expect(alpha.cells[5].textContent).toBe('1m 5s')

    const beta = rowFor(2)
    expect(beta.cells[0].textContent).toBe('2')
    expect(beta.cells[3].textContent).toContain('#4')
    expect(beta.cells[4].textContent).toBe('无')
    expect(beta.cells[5].textContent).toBe('2.5s')
    expect(beta.querySelector('.jenkins-status-orb.success')).not.toBeNull()

    const zulu = rowFor(3)
    expect(zulu.cells[0].textContent).toBe('3')
    expect(zulu.cells[3].textContent).toBe('无')
    expect(zulu.cells[4].textContent).toBe('无')
    expect(zulu.cells[5].textContent).toBe('-')
    expect(zulu.querySelector('[role="img"][aria-label="暂无构建活动。"]')).not.toBeNull()
    expect(zulu.querySelector('.jenkins-health-dot')).toBeNull()

    const projectFooter = container.querySelector('.jenkins-project-footer')
    expect(projectFooter?.textContent).toMatch(/\d{4}-\d{2}-\d{2} 星期[一二三四五六日]/)
    expect(projectFooter?.textContent).toContain('BuildWorld · 开源持续集成与构建项目')
    expect(projectFooter?.querySelector('a')?.getAttribute('href')).toBe('https://github.com/neko233-com/buildworld')

    await act(async () => buttonNamed('下移 Zulu').click())
    expect(reorderProjects).toHaveBeenCalledWith([1, 3, 2])
  })

  it('uses one stable table density without user controls or stored preferences', async () => {
    localStorage.setItem('buildworld.jenkins.icon-size', 'large')
    await renderDashboard()

    expect(container.querySelector('.jenkins-icon-size')).toBeNull()
    expect(container.querySelector('.jenkins-job-table')?.className).toBe('jenkins-job-table')
    expect(Array.from(container.querySelectorAll('button')).some(button => ['小尺寸', '中尺寸', '大尺寸'].includes(button.getAttribute('aria-label') || ''))).toBe(false)
  })

  it('keeps project data available when disk monitoring fails and offers retry', async () => {
    getStorageUsage.mockRejectedValueOnce(new Error('disk unavailable')).mockResolvedValue({
      executor: 'builtin',
      volume: 'D:',
      total_bytes: 1_000,
      used_bytes: 500,
      free_bytes: 500,
      used_percent: 50,
    })
    await renderDashboard()

    expect(container.querySelector('.jenkins-job-table')).not.toBeNull()
    expect(container.querySelector('.jenkins-storage-monitor.error')?.textContent).toContain('暂时无法读取本机缓存磁盘')

    await act(async () => buttonNamed('重试').click())
    expect(getStorageUsage).toHaveBeenCalledTimes(2)
    expect(container.querySelector('.jenkins-storage-monitor.normal progress')).not.toBeNull()
  })

  it('filters highlighted favorites and project groups without exposing quick access', async () => {
    await renderDashboard()

    const favorites = buttonNamed('收藏项目')
    expect(favorites.className).toContain('jenkins-view-favorites')
    expect(Array.from(container.querySelectorAll('button')).some(button => button.textContent?.trim() === '快速访问')).toBe(false)
    act(() => favorites.click())
    expect(visibleProjectNames()).toEqual(['Alpha'])

    expect(container.querySelector('button[aria-label="常用功能"]')).toBeNull()
    expect(container.querySelector('.jenkins-rail-links a[href="/api-tokens"]')?.textContent).toContain('API Token')
    expect(container.querySelector('.jenkins-rail-links a[href="/settings"]')).not.toBeNull()

    act(() => buttonNamed('服务端').click())
    expect(visibleProjectNames()).toEqual(['Zulu', 'Alpha'])

    await act(async () => buttonNamed('收藏 Zulu').click())
    expect(setProjectFlags).toHaveBeenCalledWith(3, true, false)

    expect(container.querySelector('button[aria-label="加入快速访问 Alpha"]')).toBeNull()
  })

  it('offers an enhanced group right-click delete flow that keeps projects by default and can delete them explicitly', async () => {
    await renderDashboard()

    const groupTab = buttonNamed('服务端')
    await act(async () => {
      groupTab.dispatchEvent(new MouseEvent('contextmenu', { bubbles: true, cancelable: true, button: 2, clientX: 420, clientY: 220 }))
    })
    const menuDelete = Array.from(container.querySelectorAll<HTMLButtonElement>('.jenkins-group-context-menu button'))[0]
    expect(menuDelete).not.toBeUndefined()
    expect(menuDelete?.textContent).toContain('删除此分组')

    await act(async () => menuDelete?.click())
    const deleteProjects = document.querySelector<HTMLInputElement>('input[name="delete-group-projects"]')
    expect(deleteProjects?.checked).toBe(false)
    expect(document.querySelector('.project-group-delete-dialog-body')?.textContent).toContain('全部')

    await act(async () => deleteProjects?.click())
    expect(deleteProjects?.checked).toBe(true)
    const confirmDelete = document.querySelector<HTMLButtonElement>('.project-group-delete-dialog .danger-command')
    await act(async () => confirmDelete?.click())

    expect(deleteProjectGroup).toHaveBeenCalledWith(20, true)
    expect(localStorage.getItem('buildworld.jenkins.active-view')).toBe('all')
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

  it('refreshes active jobs every two seconds and switches to idle frequency at a terminal state', async () => {
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

  it('polls an idle visible dashboard at low frequency and discovers an external build without an immediate duplicate request', async () => {
    vi.useFakeTimers()
    const idleOverviews = overviews.map(overview => overview.project_id === 1
      ? { ...overview, latest: { ...overview.last_success! }, recent_statuses: ['success'] }
      : overview)
    listProjectBuildOverviews.mockReset()
      .mockResolvedValueOnce(idleOverviews)
      .mockResolvedValue(overviews)
    listBuilds.mockReset()
      .mockResolvedValueOnce([
        { id: 204, project_id: 2, number: 4, status: 'success', started_at: '2026-07-21T10:04:00Z' },
        { id: 110, project_id: 1, number: 10, status: 'failed', started_at: '2026-07-21T10:10:00Z' },
      ])
      .mockResolvedValueOnce([
        { id: 204, project_id: 2, number: 4, status: 'success', started_at: '2026-07-21T10:04:00Z' },
        { id: 110, project_id: 1, number: 10, status: 'failed', started_at: '2026-07-21T10:10:00Z' },
      ])
      .mockResolvedValue([
        { id: 112, project_id: 1, number: 12, status: 'running', started_at: '2026-07-21T10:12:00Z' },
        { id: 204, project_id: 2, number: 4, status: 'success', started_at: '2026-07-21T10:04:00Z' },
        { id: 110, project_id: 1, number: 10, status: 'failed', started_at: '2026-07-21T10:10:00Z' },
      ])

    await renderDashboard()
    expect(listProjectBuildOverviews).toHaveBeenCalledTimes(1)
    expect(container.querySelector('a[href="/builds/112"]')).toBeNull()

    await act(async () => {
      await vi.advanceTimersByTimeAsync(14_999)
    })
    expect(listProjectBuildOverviews).toHaveBeenCalledTimes(1)

    await act(async () => {
      await vi.advanceTimersByTimeAsync(1)
    })
    expect(listProjectBuildOverviews).toHaveBeenCalledTimes(2)
    expect(container.querySelector('a[href="/builds/112"]')).not.toBeNull()

    await act(async () => {
      await vi.advanceTimersByTimeAsync(1_999)
    })
    expect(listProjectBuildOverviews).toHaveBeenCalledTimes(2)

    await act(async () => {
      await vi.advanceTimersByTimeAsync(1)
    })
    expect(listProjectBuildOverviews).toHaveBeenCalledTimes(3)
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
