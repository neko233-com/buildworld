// @vitest-environment jsdom

import { act } from 'react'
import { createRoot, type Root } from 'react-dom/client'
import { MemoryRouter } from 'react-router-dom'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { api } from '../api'
import BigScreen from './BigScreen'

vi.mock('../api', () => ({
  api: { getBigScreenData: vi.fn() },
}))

const getBigScreenData = vi.mocked(api.getBigScreenData)
const actEnvironment = globalThis as typeof globalThis & { IS_REACT_ACT_ENVIRONMENT: boolean }

function dashboardFixture() {
  return {
    summary: {
      total_builds: 10,
      running_builds: 1,
      queued_builds: 0,
      success_today: 8,
      failed_today: 1,
      success_rate: 80,
      active_agents: 1,
      total_agents: 2,
      total_projects: 2,
    },
    recent_builds: Array.from({ length: 10 }, (_, index) => ({
      id: index + 1,
      number: 10 - index,
      project: `Project ${index + 1}`,
      status: index === 1 ? 'failed' : 'success',
      branch: 'main',
      duration_ms: 1234,
      started_at: '2026-07-19 10:00:00',
    })),
    agent_status: [],
    project_stats: [],
    trend_data: [{ date: '2026-07-19', success: 8, failed: 1, running: 1 }],
    system_metrics: { goroutines: 4, uptime: '1h', go_version: 'go1.26', os: 'windows', arch: 'amd64', cpus: 8 },
    notifications: [],
    current_time: '2026-07-19 10:00:00',
  }
}

describe('BigScreen', () => {
  let container: HTMLDivElement
  let root: Root

  beforeEach(() => {
    actEnvironment.IS_REACT_ACT_ENVIRONMENT = true
    localStorage.setItem('locale', 'zh-CN')
    getBigScreenData.mockReset()
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

  it('keeps the dashboard concise and exposes real links for keyboard navigation', async () => {
    getBigScreenData.mockResolvedValue(dashboardFixture())
    await act(async () => {
      root.render(<MemoryRouter><BigScreen /></MemoryRouter>)
    })

    expect(container.querySelectorAll('a[href^="/builds/"]')).toHaveLength(8)
    expect(container.querySelector('a[href="/builds"]')).not.toBeNull()
    expect(container.querySelector('tr[tabindex]')).toBeNull()
    expect(container.querySelector('[role="img"]')?.getAttribute('aria-label')).toContain('2026-07-19')
  })

  it('shows a retryable full-page error when the initial request fails', async () => {
    getBigScreenData.mockRejectedValueOnce(new Error('dashboard unavailable'))
    await act(async () => {
      root.render(<MemoryRouter><BigScreen /></MemoryRouter>)
    })

    expect(container.textContent).toContain('dashboard unavailable')
    const retry = container.querySelector<HTMLButtonElement>('.page-state.error button')
    expect(retry).not.toBeNull()

    getBigScreenData.mockResolvedValueOnce(dashboardFixture())
    await act(async () => retry?.click())
    expect(container.querySelector('h1')).not.toBeNull()
  })
})
