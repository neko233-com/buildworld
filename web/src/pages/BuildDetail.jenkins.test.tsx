// @vitest-environment jsdom

import { act } from 'react'
import { createRoot, type Root } from 'react-dom/client'
import { MemoryRouter, Route, Routes } from 'react-router-dom'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { api } from '../api'
import BuildDetail from './BuildDetail'

vi.mock('../api', () => ({
  api: {
    getBuild: vi.fn(),
    getProject: vi.fn(),
    getBuildLogs: vi.fn(),
    getBuildTimeline: vi.fn(),
    getBuildProblems: vi.fn(),
    getBuildChain: vi.fn(),
    listArtifacts: vi.fn(),
    retryBuild: vi.fn(),
    pinBuild: vi.fn(),
    uploadArtifact: vi.fn(),
    stopBuild: vi.fn(),
    downloadBuildLogs: vi.fn(),
    downloadArtifact: vi.fn(),
  },
}))

vi.mock('../authz', () => ({ canEdit: () => true }))
vi.mock('../components/BuildApprovalPanel', () => ({ default: () => <div data-testid="approval" /> }))
vi.mock('../components/BuildProblemsPanel', () => ({ default: () => <div data-testid="problems" /> }))
vi.mock('../components/BuildChainPanel', () => ({ default: () => <div data-testid="chain" /> }))

const build = {
  id: 57,
  project_id: 4,
  number: 4,
  status: 'failed',
  branch: 'main',
  commit_sha: 'abc123456789',
  trigger: 'manual',
  duration_ms: 332,
  started_at: '2026-07-22T07:00:36Z',
  finished_at: '2026-07-22T07:00:37Z',
  parameters: '{"ENV":"test","TOKEN":"do-not-render"}',
}

describe('BuildDetail Jenkins Run layout', () => {
  let container: HTMLDivElement
  let breadcrumbHost: HTMLDivElement
  let root: Root

  beforeEach(() => {
    ;(globalThis as any).IS_REACT_ACT_ENVIRONMENT = true
    localStorage.setItem('locale', 'en')
    vi.mocked(api.getBuild).mockResolvedValue(build)
    vi.mocked(api.getProject).mockResolvedValue({ id: 4, name: 'Weather' })
    vi.mocked(api.getBuildLogs).mockResolvedValue({ log: '[07:00:36] [Build] hello\n' })
    vi.mocked(api.getBuildTimeline).mockResolvedValue({ total_steps: 1, completed_steps: 0, steps: [{ index: 0, stage: 'Build', name: 'Compile', status: 'failed' }] })
    vi.mocked(api.getBuildProblems).mockResolvedValue(null)
    vi.mocked(api.getBuildChain).mockResolvedValue(null)
    vi.mocked(api.listArtifacts).mockResolvedValue([])
    vi.mocked(api.retryBuild).mockResolvedValue({ id: 58 })
    vi.mocked(api.pinBuild).mockResolvedValue({})
    vi.mocked(api.stopBuild).mockResolvedValue({})
    vi.mocked(api.downloadBuildLogs).mockResolvedValue(undefined)
    vi.mocked(api.downloadArtifact).mockResolvedValue(undefined)
    container = document.createElement('div')
    breadcrumbHost = document.createElement('div')
    breadcrumbHost.id = 'jenkins-header-breadcrumbs'
    document.body.append(breadcrumbHost, container)
    root = createRoot(container)
  })

  afterEach(() => {
    act(() => root.unmount())
    container.remove()
    breadcrumbHost.remove()
    localStorage.clear()
    vi.clearAllMocks()
    ;(globalThis as any).IS_REACT_ACT_ENVIRONMENT = false
  })

  async function renderPage(currentBuild = build) {
    vi.mocked(api.getBuild).mockResolvedValue(currentBuild)
    await act(async () => {
      root.render(<MemoryRouter initialEntries={['/builds/57']}><Routes>
        <Route path="/builds/:id" element={<BuildDetail />} />
        <Route path="*" element={<div />} />
      </Routes></MemoryRouter>)
      await Promise.resolve()
      await Promise.resolve()
    })
  }

  it('renders Jenkins header breadcrumbs, Run tasks, summary, console, and artifacts', async () => {
    await renderPage()

    expect(breadcrumbHost.textContent).toContain('Weather')
    expect(breadcrumbHost.textContent).toContain('#4')
    expect(container.querySelector('.jenkins-run-layout')).not.toBeNull()
    expect(container.querySelector('.jenkins-run-side-panel')).not.toBeNull()
    expect(container.querySelector('a.active[href="#overview"]')?.textContent).toContain('Status')
    expect(container.querySelector('a[href="/projects/4/changes?build=57"]')?.textContent).toContain('Changes')
    expect(container.querySelector('a[href="/builds/57/logs"]')?.textContent).toContain('Logs')
    expect(container.querySelector('a[href="/builds/57/tests"]')?.textContent).toContain('Test Reports')
    expect(container.querySelector('.jenkins-run-caption')?.textContent).toContain('Build #4')
    expect(container.querySelector('#out')?.textContent).toContain('hello')
    expect(container.querySelector('#artifacts')).not.toBeNull()
    expect(container.querySelector('.build-parameters-panel')?.textContent).toContain('••••••••')
    expect(container.querySelector('.build-parameters-panel')?.textContent).not.toContain('do-not-render')
  })

  it('keeps Jenkins-visible loading feedback while a build is running', async () => {
    await renderPage({ ...build, status: 'running', finished_at: null })

    expect(container.querySelector('.jenkins-run-caption .build-status-spinner')).not.toBeNull()
    expect(container.querySelector('.jenkins-console-progress[role="status"]')).not.toBeNull()
    expect(container.querySelector('.jenkins-run-tasks button.danger')?.getAttribute('aria-busy')).toBe('false')

    const consoleOutput = container.querySelector<HTMLPreElement>('.jenkins-console-output')!
    Object.defineProperties(consoleOutput, {
      scrollHeight: { configurable: true, value: 1000 },
      clientHeight: { configurable: true, value: 200 },
      scrollTop: { configurable: true, value: 100, writable: true },
    })
    await act(async () => consoleOutput.dispatchEvent(new Event('scroll', { bubbles: true })))
    const follow = Array.from(container.querySelectorAll<HTMLButtonElement>('.jenkins-console-controls button'))
      .find(button => button.getAttribute('aria-pressed') !== null)!
    expect(follow.getAttribute('aria-pressed')).toBe('false')
    expect(follow.textContent).toContain('Resume follow')

    const animationFrame = vi.spyOn(window, 'requestAnimationFrame').mockImplementation(callback => {
      callback(0)
      return 1
    })
    await act(async () => follow.click())
    expect(consoleOutput.scrollTop).toBe(1000)
    expect(follow.getAttribute('aria-pressed')).toBe('true')
    animationFrame.mockRestore()
  })

  it('opens Jenkins Replay summary before creating and navigating to the replacement build', async () => {
    await renderPage()
    const retry = Array.from(container.querySelectorAll<HTMLButtonElement>('.jenkins-run-tasks button')).find(button => button.textContent?.includes('Replay'))
    expect(retry).toBeDefined()

    await act(async () => retry?.click())
    expect(api.retryBuild).not.toHaveBeenCalled()
    const replayDialog = document.querySelector<HTMLElement>('.jenkins-replay-dialog')
    expect(replayDialog?.textContent).toContain('Weather #4')
    expect(replayDialog?.textContent).toContain('main')
    expect(replayDialog?.textContent).not.toContain('do-not-render')

    await act(async () => replayDialog?.querySelector<HTMLButtonElement>('.jenkins-replay-submit')?.click())
    expect(api.retryBuild).toHaveBeenCalledWith(57)
  })
})
