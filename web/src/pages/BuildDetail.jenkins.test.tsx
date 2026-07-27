// @vitest-environment jsdom

import { act } from 'react'
import { createRoot, type Root } from 'react-dom/client'
import { MemoryRouter, Route, Routes } from 'react-router-dom'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { readFileSync } from 'node:fs'
import { resolve } from 'node:path'
import { api } from '../api'
import BuildDetail from './BuildDetail'

const buildDetailStyles = readFileSync(resolve(process.cwd(), 'src/pages/BuildDetail.jenkins.css'), 'utf8')

vi.mock('../api', () => ({
  api: {
    getBuild: vi.fn(),
    getProject: vi.fn(),
    getBuildLogs: vi.fn(),
    getBuildTimeline: vi.fn(),
    getBuildProblems: vi.fn(),
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
    vi.mocked(api.getBuildProblems).mockResolvedValue({ version: 1, build_status: 'failed', summary: 'Compile failed', failed_step_count: 1, problems: [{ id: 'compile', code: 'step_failed', severity: 'error', step: 'Compile', message: 'Compiler exited with code 1', suggested_action: 'inspect_logs' }] })
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

  it('keeps current build, problems, and artifacts separate while omitting the build chain', async () => {
    await renderPage()

    expect(breadcrumbHost.textContent).toContain('Weather')
    expect(breadcrumbHost.textContent).toContain('#4')
    expect(container.querySelector('.jenkins-run-layout')).not.toBeNull()
    expect(container.querySelector('.jenkins-run-side-panel')).not.toBeNull()
    expect(container.querySelector('.jenkins-run-tasks button.active')?.textContent).toContain('Current Build')
    expect(container.querySelector('a[href="/projects/4/changes?build=57"]')?.textContent).toContain('Changes')
    expect(container.querySelector('a[href="/builds/57/logs"]')?.textContent).toContain('Logs')
    expect(container.querySelector('a[href="/builds/57/tests"]')?.textContent).toContain('Test Reports')
    expect(container.querySelector('.jenkins-run-caption')?.textContent).toContain('Build #4')
    expect(container.querySelectorAll('.jenkins-run-tabs [role="tab"]')).toHaveLength(3)
    expect(container.querySelector('#build-tab-current')?.getAttribute('aria-selected')).toBe('true')
    const currentPanel = container.querySelector<HTMLElement>('#build-panel-current')!
    const problemsPanel = container.querySelector<HTMLElement>('#build-panel-problems')!
    const artifactsPanel = container.querySelector<HTMLElement>('#build-panel-artifacts')!
    const consoleOutput = container.querySelector<HTMLElement>('#out')!
    expect(currentPanel.hidden).toBe(false)
    expect(problemsPanel.hidden).toBe(true)
    expect(artifactsPanel.hidden).toBe(true)
    expect(container.querySelector('.build-chain-panel')).toBeNull()
    expect(consoleOutput.textContent).toContain('hello')
    expect(consoleOutput.getAttribute('role')).toBe('region')
    expect(consoleOutput.getAttribute('aria-label')).toBe('Logs')
    expect(consoleOutput.tabIndex).toBe(0)
    expect(consoleOutput.hasAttribute('aria-live')).toBe(false)
    expect(container.querySelector('#artifacts')).not.toBeNull()
    expect(container.querySelector('.build-parameters-panel')?.textContent).toContain('••••••••')
    expect(container.querySelector('.build-parameters-panel')?.textContent).not.toContain('do-not-render')

    const uploadInput = container.querySelector<HTMLInputElement>('#artifact-upload')!
    const uploadLabel = container.querySelector<HTMLLabelElement>('label[for="artifact-upload"]')!
    uploadInput.focus()
    expect(document.activeElement).toBe(uploadInput)
    expect(uploadLabel.htmlFor).toBe(uploadInput.id)

    await act(async () => {
      container.querySelector<HTMLButtonElement>('#build-tab-problems')?.click()
      await Promise.resolve()
      await Promise.resolve()
    })
    expect(container.querySelector('#build-tab-problems')?.getAttribute('aria-selected')).toBe('true')
    expect(currentPanel.hidden).toBe(true)
    expect(problemsPanel.hidden).toBe(false)
    expect(problemsPanel.textContent).toContain('Compiler exited with code 1')
    expect(api.getBuildProblems).toHaveBeenCalledWith(57)

    await act(async () => container.querySelector<HTMLButtonElement>('#build-tab-artifacts')?.click())
    expect(container.querySelector('#build-tab-artifacts')?.getAttribute('aria-selected')).toBe('true')
    expect(currentPanel.hidden).toBe(true)
    expect(problemsPanel.hidden).toBe(true)
    expect(artifactsPanel.hidden).toBe(false)
    expect(container.querySelector('#out')).toBe(consoleOutput)

    const artifactsTab = container.querySelector<HTMLButtonElement>('#build-tab-artifacts')!
    artifactsTab.focus()
    await act(async () => artifactsTab.dispatchEvent(new KeyboardEvent('keydown', { key: 'ArrowLeft', bubbles: true })))
    expect(container.querySelector('#build-tab-problems')?.getAttribute('aria-selected')).toBe('true')
    expect(problemsPanel.hidden).toBe(false)
    const problemsTab = container.querySelector<HTMLButtonElement>('#build-tab-problems')!
    await act(async () => problemsTab.dispatchEvent(new KeyboardEvent('keydown', { key: 'ArrowLeft', bubbles: true })))
    expect(container.querySelector('#build-tab-current')?.getAttribute('aria-selected')).toBe('true')
    expect(currentPanel.hidden).toBe(false)
    expect(problemsPanel.hidden).toBe(true)
    expect(artifactsPanel.hidden).toBe(true)
  })

  it('shows a fixed stage rail with one copyable stage name per stage', async () => {
    const scrollIntoView = vi.fn()
    Object.defineProperty(HTMLElement.prototype, 'scrollIntoView', { configurable: true, value: scrollIntoView })
    const writeText = vi.fn().mockResolvedValue(undefined)
    Object.defineProperty(navigator, 'clipboard', { configurable: true, value: { writeText } })
    const longStageName = 'Prepare release package with a deliberately long stage name for inspection'
    vi.mocked(api.getBuildLogs).mockResolvedValueOnce({ log: `[07:00:36] [Build] compiling\n[07:00:37] [${longStageName}] packaging\n` })
    vi.mocked(api.getBuildTimeline).mockResolvedValueOnce({
      total_steps: 3,
      completed_steps: 2,
      steps: [
        { index: 0, stage: 'Build', name: 'Compile', status: 'success' },
        { index: 1, stage: 'Build', name: 'Unit tests', status: 'success' },
        { index: 2, stage: longStageName, name: 'Package', status: 'running' },
      ],
    })
    await renderPage({ ...build, status: 'running', finished_at: null })

    expect(container.querySelector('.build-timeline')).toBeNull()
    const rail = container.querySelector<HTMLElement>('.build-stage-rail')!
    expect(rail).not.toBeNull()
    const stageItems = rail.querySelectorAll<HTMLButtonElement>('.build-stage-rail-item')
    expect(stageItems).toHaveLength(2)
    expect(stageItems[0].textContent).toBe('Build')
    expect(stageItems[1].textContent).toBe(longStageName)
    expect(stageItems[1].title).toBe(longStageName)

    await act(async () => stageItems[1].click())
    expect(scrollIntoView).toHaveBeenCalledWith({ behavior: 'smooth', block: 'center' })

    await act(async () => rail.querySelector<HTMLButtonElement>('.build-stage-rail-copy')?.click())
    expect(writeText).toHaveBeenCalledWith('Build')

    const toggle = rail.querySelector<HTMLButtonElement>('.build-stage-rail-toggle')!
    await act(async () => toggle.click())
    expect(toggle.getAttribute('aria-expanded')).toBe('false')
    expect(rail.querySelector('ol')).toBeNull()
    expect(container.querySelector('.jenkins-run-page')?.classList.contains('has-collapsed-stage-rail')).toBe(true)
  })

  it('uses an HTTP-safe copy fallback for stage names', async () => {
    Object.defineProperty(navigator, 'clipboard', { configurable: true, value: undefined })
    const execCommand = vi.fn(() => true)
    Object.defineProperty(document, 'execCommand', { configurable: true, value: execCommand })
    await renderPage()

    const copy = container.querySelector<HTMLButtonElement>('.build-stage-rail-copy')!
    await act(async () => copy.click())
    expect(execCommand).toHaveBeenCalledWith('copy')
    expect(document.querySelector('textarea[readonly]')).toBeNull()
  })

  it('reserves space for the fixed stage rail instead of covering log content', () => {
    expect(buildDetailStyles).toMatch(/\.jenkins-run-page\.has-expanded-stage-rail\s*\{[\s\S]*?padding-right:\s*calc\(var\(--run-stage-rail-width\)/)
    expect(buildDetailStyles).toMatch(/\.jenkins-run-page\.has-collapsed-stage-rail\s*\{[\s\S]*?padding-right:\s*calc\(38px/)
  })

  it('keeps Jenkins-visible loading feedback while a build is running', async () => {
    await renderPage({ ...build, status: 'running', finished_at: null })

    expect(container.querySelector('.jenkins-run-caption .build-status-spinner')).not.toBeNull()
    expect(container.querySelector('.jenkins-console-progress[role="status"]')).not.toBeNull()
    expect(container.querySelector('.jenkins-run-tasks button.danger')?.getAttribute('aria-busy')).toBe('false')

    let consoleOutput = container.querySelector<HTMLPreElement>('.jenkins-console-output')!
    Object.defineProperties(consoleOutput, {
      scrollHeight: { configurable: true, value: 1000 },
      clientHeight: { configurable: true, value: 200 },
      scrollTop: { configurable: true, value: 800, writable: true },
    })
    await act(async () => consoleOutput.dispatchEvent(new WheelEvent('wheel', { bubbles: true, deltaY: -1 })))
    let follow = Array.from(container.querySelectorAll<HTMLButtonElement>('.jenkins-console-controls button'))
      .find(button => button.getAttribute('aria-pressed') !== null)!
    expect(follow.getAttribute('aria-pressed')).toBe('false')
    expect(follow.textContent).toContain('Resume follow')

    await act(async () => container.querySelector<HTMLButtonElement>('#build-tab-artifacts')?.click())
    await act(async () => container.querySelector<HTMLButtonElement>('#build-tab-current')?.click())
    consoleOutput = container.querySelector<HTMLPreElement>('.jenkins-console-output')!
    Object.defineProperties(consoleOutput, {
      scrollHeight: { configurable: true, value: 1000 },
      clientHeight: { configurable: true, value: 200 },
      scrollTop: { configurable: true, value: 800, writable: true },
    })
    follow = Array.from(container.querySelectorAll<HTMLButtonElement>('.jenkins-console-controls button'))
      .find(button => button.getAttribute('aria-pressed') !== null)!
    expect(follow.getAttribute('aria-pressed')).toBe('false')

    const animationFrame = vi.spyOn(window, 'requestAnimationFrame').mockImplementation(callback => {
      callback(0)
      return 1
    })
    await act(async () => follow.click())
    expect(consoleOutput.scrollTop).toBe(1000)
    expect(follow.getAttribute('aria-pressed')).toBe('true')
    animationFrame.mockRestore()
  })

  it('renders every console line with default-on level colors and can disable them without filtering output', async () => {
    vi.mocked(api.getBuildLogs).mockResolvedValueOnce({
      log: [
        'plain server output',
        '[WARN] retrying request',
        'level=ERROR request failed',
        '[15:46:49] [Live Log Monitor] === Stack Trace ===',
        '[15:46:49] [Live Log Monitor]   [0] logger.Error at logger233.go:985',
        '[15:46:49] [Live Log Monitor]   [1] betfish_module.load.func1 at bet_fish_service.go:1076',
      ].join('\n'),
    })
    await renderPage()

    const consoleOutput = container.querySelector<HTMLElement>('.jenkins-console-output')!
    const toneToggle = container.querySelector<HTMLButtonElement>('button[aria-label="Log level colors"]')!
    expect(consoleOutput.querySelectorAll('.jenkins-console-line')).toHaveLength(6)
    expect(consoleOutput.querySelectorAll('.jenkins-console-line.info')).toHaveLength(1)
    expect(consoleOutput.querySelectorAll('.jenkins-console-line.warning')).toHaveLength(1)
    expect(consoleOutput.querySelectorAll('.jenkins-console-line.error')).toHaveLength(4)
    expect(toneToggle.getAttribute('aria-pressed')).toBe('true')

    await act(async () => toneToggle.click())
    expect(toneToggle.getAttribute('aria-pressed')).toBe('false')
    expect(consoleOutput.querySelectorAll('.jenkins-console-line')).toHaveLength(6)
    expect(consoleOutput.querySelectorAll('.jenkins-console-line.info, .jenkins-console-line.warning, .jenkins-console-line.error')).toHaveLength(0)
    expect(consoleOutput.textContent).toContain('plain server output')
    expect(consoleOutput.textContent).toContain('[WARN] retrying request')
    expect(consoleOutput.textContent).toContain('level=ERROR request failed')
    expect(localStorage.getItem('buildworld.logs.colorize')).toBe('false')
  })

  it('shows the durable log retention limit in the embedded console', async () => {
    vi.mocked(api.getBuildLogs).mockResolvedValueOnce({
      log: '[buildworld] Earlier persisted log output was truncated\nnewest',
      truncated: true,
      retention_characters: 1_000_000,
    })
    await renderPage()

    expect(container.querySelector('.jenkins-console-retention-warning')?.textContent).toContain('1000000-character limit')
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
