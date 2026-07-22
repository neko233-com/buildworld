// @vitest-environment jsdom

import { act } from 'react'
import { createRoot, type Root } from 'react-dom/client'
import { MemoryRouter } from 'react-router-dom'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { api } from '../api'
import MyDashboard from './MyDashboard'

vi.mock('../api', () => ({
  api: {
    getBigScreenData: vi.fn(),
    listQuickAccess: vi.fn(),
    listBuildQueue: vi.fn(),
    listAgents: vi.fn(),
  },
}))

const actEnvironment = globalThis as typeof globalThis & { IS_REACT_ACT_ENVIRONMENT: boolean }

describe('MyDashboard Jenkins view', () => {
  let container: HTMLDivElement
  let root: Root

  beforeEach(() => {
    actEnvironment.IS_REACT_ACT_ENVIRONMENT = true
    const payload = btoa(JSON.stringify({ role: 'admin' }))
    localStorage.setItem('token', `test.${payload}.signature`)
    localStorage.setItem('locale', 'zh-CN')
    vi.mocked(api.getBigScreenData).mockResolvedValue({
      summary: {
        total_builds: 9,
        running_builds: 1,
        queued_builds: 2,
        success_today: 4,
        failed_today: 1,
        success_rate: 80,
        active_agents: 2,
        total_agents: 3,
        total_projects: 5,
      },
      recent_builds: [
        { id: 7, number: 18, project: 'Alpha', status: 'running', branch: 'main', duration_ms: 2500, started_at: '2026-07-22 10:00:00' },
      ],
      trend_data: [{ date: '2026-07-22', success: 4, failed: 1, running: 1 }],
    } as any)
    vi.mocked(api.listQuickAccess).mockResolvedValue([{ id: 1, name: 'Alpha', favorite: true }] as any)
    vi.mocked(api.listBuildQueue).mockResolvedValue([])
    vi.mocked(api.listAgents).mockResolvedValue([])
    container = document.createElement('div')
    document.body.appendChild(container)
    root = createRoot(container)
  })

  afterEach(() => {
    act(() => root.unmount())
    container.remove()
    localStorage.clear()
    vi.restoreAllMocks()
    actEnvironment.IS_REACT_ACT_ENVIRONMENT = false
  })

  it('uses the Jenkins home rail and table language instead of a standalone card dashboard', async () => {
    await act(async () => {
      root.render(<MemoryRouter initialEntries={['/my-dashboard']}><MyDashboard /></MemoryRouter>)
    })

    expect(container.querySelector('.jenkins-home.jenkins-user-dashboard')).not.toBeNull()
    expect(container.querySelector('.jenkins-rail-root')).not.toBeNull()
    expect(container.querySelectorAll('.data-metric-grid article')).toHaveLength(4)
    expect(vi.mocked(api.listAgents)).not.toHaveBeenCalled()
    expect(container.querySelector('a[href="/agents"]')).toBeNull()
    expect(container.textContent).not.toContain('分布式容量')
    expect(container.querySelector('caption')?.textContent).toMatch(/最近构建|Recent Builds/)
    expect(container.querySelector('.jenkins-build-state.running svg.timeline-spinner')).not.toBeNull()
    expect(container.querySelector('.data-build-table .build-status')).toBeNull()
    expect(container.querySelector('.my-project-card strong')?.textContent).toBe('Alpha')
    expect(container.querySelector('a[href="/projects/1"]')).not.toBeNull()
  })
})
