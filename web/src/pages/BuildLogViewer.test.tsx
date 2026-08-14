// @vitest-environment jsdom

import { act } from 'react'
import { createRoot, type Root } from 'react-dom/client'
import { MemoryRouter, Route, Routes } from 'react-router-dom'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { api } from '../api'
import { useBuildLogStream } from '../useBuildLogStream'
import BuildLogViewer from './BuildLogViewer'

vi.mock('../api', () => ({
  api: {
    getBuild: vi.fn(),
    getBuildLogs: vi.fn(),
    downloadBuildLogs: vi.fn(),
  },
}))

const getBuild = vi.mocked(api.getBuild)
const getBuildLogs = vi.mocked(api.getBuildLogs)
const downloadBuildLogs = vi.mocked(api.downloadBuildLogs)
const actEnvironment = globalThis as typeof globalThis & { IS_REACT_ACT_ENVIRONMENT: boolean }
const noop = () => {}

function BuildLogStreamProbe() {
  const { log } = useBuildLogStream({
    buildID: 42,
    enabled: true,
    reloadSnapshot: noop,
    onBuildStatus: noop,
  })
  return <output data-stream-log>{log}</output>
}

class MockWebSocket {
  static instances: MockWebSocket[] = []
  onopen: (() => void) | null = null
  onclose: (() => void) | null = null
  onerror: (() => void) | null = null
  onmessage: ((event: MessageEvent) => void) | null = null
  readonly url: string
  constructor(url: string) {
    this.url = url
    MockWebSocket.instances.push(this)
  }
  close() { this.onclose?.() }
  emitOpen() { this.onopen?.() }
  emitMessage(data: unknown) { this.onmessage?.(new MessageEvent('message', { data: JSON.stringify(data) })) }
}

describe('BuildLogViewer', () => {
  let container: HTMLDivElement
  let root: Root

  beforeEach(() => {
    actEnvironment.IS_REACT_ACT_ENVIRONMENT = true
    localStorage.setItem('token', 'test-token')
    localStorage.setItem('locale', 'zh-CN')
    getBuild.mockReset()
    getBuildLogs.mockReset()
    downloadBuildLogs.mockReset()
    MockWebSocket.instances = []
    vi.stubGlobal('WebSocket', MockWebSocket)
    getBuild.mockResolvedValue({ id: 42, number: 7, project_id: 3, status: 'failed' })
    getBuildLogs.mockResolvedValue({
      log: [
        '[10:00:00] [Prepare] === Stage: Prepare ===',
        '[10:00:01] [Verify] ERROR: exit status 7',
        '[10:00:01] [] BUILD FAILED: verification failed',
      ].join('\n'),
    })
    downloadBuildLogs.mockResolvedValue('build-42-logs.txt')
    container = document.createElement('div')
    document.body.appendChild(container)
    root = createRoot(container)
  })

  afterEach(() => {
    act(() => root.unmount())
    vi.useRealTimers()
    container.remove()
    localStorage.clear()
    vi.unstubAllGlobals()
    document.title = ''
    actEnvironment.IS_REACT_ACT_ENVIRONMENT = false
  })

  async function renderViewer(path = '/builds/42/logs') {
    await act(async () => {
      root.render(
        <MemoryRouter initialEntries={[path]}>
          <Routes>
            <Route path="/builds/:id/logs" element={<BuildLogViewer />} />
            <Route path="/builds" element={<div data-route="builds" />} />
            <Route path="/login" element={<div data-route="login" />} />
          </Routes>
        </MemoryRouter>,
      )
    })
  }

  async function waitForLogBatch() {
    await act(async () => {
      await new Promise(resolve => window.setTimeout(resolve, 90))
    })
  }

  it('renders a standalone searchable log surface with download controls', async () => {
    await renderViewer()

    expect(MockWebSocket.instances).toHaveLength(0)
    expect(container.querySelector('h1')?.textContent).toBe('Standalone log viewer')
    expect(container.querySelector('a[href="/builds/42"]')).not.toBeNull()
    expect(container.querySelector('a[href="/"]')).toBeNull()
    expect(container.querySelectorAll('.plain-log-lines > div')).toHaveLength(3)
    expect(container.querySelectorAll('.plain-log-lines > .error')).toHaveLength(2)
    expect(container.querySelectorAll('.plain-log-lines > .info')).toHaveLength(1)
    expect(document.title).toBe('Logs · #7 · buildworld')
    const viewport = container.querySelector<HTMLElement>('.plain-log-viewport')!
    expect(viewport.getAttribute('role')).toBe('region')
    expect(viewport.getAttribute('aria-label')).toBe('Logs')
    expect(viewport.tabIndex).toBe(0)
    expect(viewport.hasAttribute('aria-live')).toBe(false)
    expect(container.querySelector('.plain-log-lines')?.hasAttribute('role')).toBe(false)
    expect(container.querySelector('.plain-log-lines')?.hasAttribute('aria-live')).toBe(false)

    const search = container.querySelector<HTMLInputElement>('input[aria-label="Search logs"]')!
    await act(async () => {
      Object.getOwnPropertyDescriptor(HTMLInputElement.prototype, 'value')?.set?.call(search, 'error')
      search.dispatchEvent(new Event('input', { bubbles: true }))
    })
    expect(container.textContent).toContain('1 of 1 matches')
    expect(container.querySelector('mark')?.textContent).toBe('ERROR')
    expect(container.textContent).toContain('Live follow paused')

    const resumeFollow = Array.from(container.querySelectorAll<HTMLButtonElement>('button'))
      .find(button => button.textContent?.includes('Resume follow'))!
    await act(async () => resumeFollow.click())
    expect(search.value).toBe('')
    expect(resumeFollow.getAttribute('aria-pressed')).toBe('true')
    expect(resumeFollow.textContent).toContain('Pause follow')

    const wrap = Array.from(container.querySelectorAll<HTMLButtonElement>('button'))
      .find(button => button.textContent?.includes('Wrap lines'))!
    act(() => wrap.click())
    expect(wrap.getAttribute('aria-pressed')).toBe('true')
    expect(container.querySelector('.plain-log-page')?.classList.contains('wrap-lines')).toBe(true)

    const themeToggle = container.querySelector<HTMLButtonElement>('button[aria-label="Switch to dark theme"]')!
    expect(container.querySelector('.plain-log-page')?.classList.contains('theme-light')).toBe(true)
    await act(async () => themeToggle.click())
    expect(container.querySelector('.plain-log-page')?.classList.contains('theme-dark')).toBe(true)
    expect(localStorage.getItem('buildworld.logs.theme')).toBe('dark')

    const txtDownload = Array.from(container.querySelectorAll<HTMLButtonElement>('button'))
      .find(button => button.textContent?.includes('.txt'))!
    await act(async () => txtDownload.click())
    expect(downloadBuildLogs).toHaveBeenCalledWith(42, 'txt')
  })

  it('clears only the current browser screen without refetching logs', async () => {
    await renderViewer()

    const snapshotsBeforeClear = getBuildLogs.mock.calls.length
    const clear = container.querySelector<HTMLButtonElement>('button[aria-label="Clear current screen logs"]')!
    expect(clear.disabled).toBe(false)

    await act(async () => clear.click())

    expect(container.querySelector('.plain-log-empty')?.textContent).toContain('No logs available')
    expect(getBuildLogs.mock.calls.length).toBe(snapshotsBeforeClear)
  })

  it('keeps only incremental WebSocket and REST output after clearing', async () => {
    getBuild.mockResolvedValue({ id: 42, number: 7, project_id: 3, status: 'running' })
    await renderViewer()

    const socket = MockWebSocket.instances[0]
    const clear = container.querySelector<HTMLButtonElement>('button[aria-label="Clear current screen logs"]')!
    await act(async () => clear.click())
    expect(container.textContent).not.toContain('exit status 7')

    await act(async () => socket?.emitMessage({
      type: 'build:log',
      payload: { timestamp: '10:00:02', stage: 'Build', line: 'incremental socket line' },
    }))
    await waitForLogBatch()
    expect(container.textContent).toContain('incremental socket line')
    expect(container.textContent).not.toContain('exit status 7')

    getBuildLogs.mockResolvedValueOnce({ log: [
      '[10:00:00] [Prepare] === Stage: Prepare ===',
      '[10:00:01] [Verify] ERROR: exit status 7',
      '[10:00:01] [] BUILD FAILED: verification failed',
      '[10:00:02] [Build] incremental socket line',
      '[10:00:03] [Build] incremental snapshot line',
    ].join('\n') })
    await act(async () => socket?.emitOpen())
    await act(async () => { await new Promise(resolve => window.setTimeout(resolve, 0)) })
    expect(container.textContent).toContain('incremental snapshot line')
    expect(container.textContent).not.toContain('exit status 7')
  })

  it('filters the log viewport to matching lines when enabled', async () => {
    await renderViewer()

    const search = container.querySelector<HTMLInputElement>('input[aria-label="Search logs"]')!
    await act(async () => {
      Object.getOwnPropertyDescriptor(HTMLInputElement.prototype, 'value')?.set?.call(search, 'error')
      search.dispatchEvent(new Event('input', { bubbles: true }))
    })
    const filter = container.querySelector<HTMLButtonElement>('.plain-log-toolbar .filter-action')!
    expect(filter.disabled).toBe(false)
    await act(async () => filter.click())

    expect(filter.getAttribute('aria-pressed')).toBe('true')
    expect(container.querySelectorAll('.plain-log-lines > div')).toHaveLength(1)
    expect(container.textContent).toContain('exit status 7')
    expect(container.textContent).not.toContain('Stage: Prepare')
  })

  it('colors error, warning, and default info lines without filtering and persists the accessible toggle', async () => {
    getBuildLogs.mockResolvedValueOnce({
      log: [
        'plain application output',
        '[INFO] ready',
        '[WARN] retrying',
        'ERROR request failed',
        '[15:46:49] [Live Log Monitor] === Stack Trace ===',
        '[15:46:49] [Live Log Monitor]   [0] logger.Error at logger233.go:985',
      ].join('\n'),
    })
    await renderViewer()

    const toneToggle = container.querySelector<HTMLButtonElement>('button[aria-label="Log level colors"]')!
    expect(toneToggle.getAttribute('aria-pressed')).toBe('true')
    expect(container.querySelectorAll('.plain-log-lines > div')).toHaveLength(6)
    expect(container.querySelectorAll('.plain-log-lines > .info')).toHaveLength(2)
    expect(container.querySelectorAll('.plain-log-lines > .warning')).toHaveLength(1)
    expect(container.querySelectorAll('.plain-log-lines > .error')).toHaveLength(3)

    await act(async () => toneToggle.click())
    expect(toneToggle.getAttribute('aria-pressed')).toBe('false')
    expect(container.querySelectorAll('.plain-log-lines > div')).toHaveLength(6)
    expect(container.querySelectorAll('.plain-log-lines > .info, .plain-log-lines > .warning, .plain-log-lines > .error')).toHaveLength(0)
    expect(container.textContent).toContain('plain application output')
    expect(container.textContent).toContain('[INFO] ready')
    expect(container.textContent).toContain('[WARN] retrying')
    expect(container.textContent).toContain('ERROR request failed')
    expect(localStorage.getItem('buildworld.logs.colorize')).toBe('false')
  })

  it('shows a retryable error when either log request fails', async () => {
    getBuildLogs.mockRejectedValueOnce(new Error('log endpoint unavailable'))
    await renderViewer()

    expect(container.textContent).toContain('log endpoint unavailable')
    const retry = container.querySelector<HTMLButtonElement>('.page-state.error button')
    expect(retry).not.toBeNull()

    getBuildLogs.mockResolvedValueOnce({ log: 'recovered' })
    await act(async () => retry?.click())
    expect(container.querySelector('code')?.textContent).toBe('recovered')
  })

  it('keeps download failures inside the standalone page', async () => {
    downloadBuildLogs.mockRejectedValueOnce(new Error('download unavailable'))
    await renderViewer()

    const jsonDownload = Array.from(container.querySelectorAll<HTMLButtonElement>('button'))
      .find(button => button.textContent?.includes('JSON'))!
    await act(async () => jsonDownload.click())

    expect(container.querySelector('[role="alert"]')?.textContent).toBe('download unavailable')
    expect(jsonDownload.disabled).toBe(false)
  })

  it('clearly reports when durable history was truncated', async () => {
    getBuildLogs.mockResolvedValueOnce({
      log: '[buildworld] Earlier persisted log output was truncated\nnewest',
      truncated: true,
      retention_characters: 1_000_000,
    })
    await renderViewer()

    const warning = container.querySelector('[role="status"].retention-warning')
    expect(warning?.textContent).toContain('1000000-character limit')
    expect(warning?.textContent).toContain('live WebSocket stream was unaffected')
  })

  it('appends build-log socket events and reports the live connection state', async () => {
    getBuild.mockResolvedValue({ id: 42, number: 7, project_id: 3, status: 'running' })
    await renderViewer()

    const socket = MockWebSocket.instances[0]
    expect(socket?.url).toContain('/ws?room=build:42')
    await act(async () => socket?.emitOpen())
    expect(container.textContent).toContain('Live socket connected')
    const streamStatus = container.querySelector<HTMLElement>('.plain-log-toolbar [role="status"]')!
    expect(streamStatus.getAttribute('aria-live')).toBe('polite')
    expect(streamStatus.getAttribute('aria-atomic')).toBe('true')

    await act(async () => socket?.emitMessage({
      type: 'build:log',
      payload: { timestamp: '10:00:02', stage: 'Build', line: 'compile completed' },
    }))
    await waitForLogBatch()
    expect(container.textContent).toContain('[10:00:02] [Build] compile completed')

    const snapshotsBeforeStatus = getBuildLogs.mock.calls.length
    await act(async () => socket?.emitMessage({ type: 'build:status', payload: { status: 'success' } }))
    expect(getBuildLogs.mock.calls.length).toBeGreaterThan(snapshotsBeforeStatus)
  })

  it('polls build status without repeatedly downloading the full log window while the socket is live', async () => {
    vi.useFakeTimers()
    getBuild.mockResolvedValue({ id: 42, number: 7, project_id: 3, status: 'running' })
    await renderViewer()

    const socket = MockWebSocket.instances[0]
    await act(async () => socket?.emitOpen())
    const snapshotsAfterOpen = getBuildLogs.mock.calls.length
    const buildsAfterOpen = getBuild.mock.calls.length

    await act(async () => vi.advanceTimersByTimeAsync(5_000))
    expect(getBuild.mock.calls.length).toBeGreaterThan(buildsAfterOpen)
    expect(getBuildLogs.mock.calls.length).toBe(snapshotsAfterOpen)
  })

  it('fetches one final log snapshot when polling observes the build finish', async () => {
    vi.useFakeTimers()
    getBuild
      .mockResolvedValueOnce({ id: 42, number: 7, project_id: 3, status: 'running' })
      .mockResolvedValue({ id: 42, number: 7, project_id: 3, status: 'failed' })
    await renderViewer()

    const socket = MockWebSocket.instances[0]
    await act(async () => socket?.emitOpen())
    const snapshotsBeforeFinish = getBuildLogs.mock.calls.length

    await act(async () => {
      await vi.advanceTimersByTimeAsync(5_000)
      await Promise.resolve()
    })

    expect(container.querySelector('.build-status')?.textContent).toBe('Failed')
    expect(getBuildLogs.mock.calls.length).toBeGreaterThan(snapshotsBeforeFinish)
  })

  it('starts at the latest output and pauses follow on any user wheel or history scroll', async () => {
    getBuild.mockResolvedValue({ id: 42, number: 7, project_id: 3, status: 'running' })
    await renderViewer()

    const viewport = container.querySelector<HTMLDivElement>('.plain-log-viewport')!
    Object.defineProperty(viewport, 'scrollHeight', { configurable: true, get: () => 1200 })
    Object.defineProperty(viewport, 'clientHeight', { configurable: true, get: () => 300 })
    const socket = MockWebSocket.instances[0]

    await act(async () => socket?.emitMessage({
      type: 'build:log',
      payload: { timestamp: '10:00:03', stage: 'Observe', line: 'latest server line' },
    }))
    await waitForLogBatch()
    expect(viewport.scrollTop).toBe(1200)

    await act(async () => viewport.dispatchEvent(new WheelEvent('wheel', { bubbles: true, deltaY: -1 })))
    expect(container.textContent).toContain('Live follow paused')
    const follow = Array.from(container.querySelectorAll<HTMLButtonElement>('button'))
      .find(button => button.getAttribute('aria-pressed') !== null && /follow/i.test(button.textContent || ''))!
    await act(async () => follow.click())
    expect(viewport.scrollTop).toBe(1200)

    await act(async () => {
      viewport.scrollTop = 500
      viewport.dispatchEvent(new Event('scroll', { bubbles: true }))
    })
    expect(container.textContent).toContain('Live follow paused')

    await act(async () => socket?.emitMessage({
      type: 'build:log',
      payload: { timestamp: '10:00:04', stage: 'Observe', line: 'line while reviewing history' },
    }))
    await waitForLogBatch()
    expect(viewport.scrollTop).toBe(500)
  })

  it('batches burst WebSocket log records into one short render interval', async () => {
    vi.useFakeTimers()
    await act(async () => root.render(<BuildLogStreamProbe />))
    const socket = MockWebSocket.instances[0]

    act(() => {
      socket?.emitMessage({ type: 'build:log', payload: { timestamp: '10:00:01', stage: 'Build', line: 'one' } })
      socket?.emitMessage({ type: 'build:log', payload: { timestamp: '10:00:02', stage: 'Build', line: 'two' } })
    })
    expect(container.querySelector('[data-stream-log]')?.textContent).toBe('')

    act(() => vi.advanceTimersByTime(74))
    expect(container.querySelector('[data-stream-log]')?.textContent).toBe('')
    act(() => vi.advanceTimersByTime(1))
    expect(container.querySelector('[data-stream-log]')?.textContent).toContain('[10:00:01] [Build] one\n[10:00:02] [Build] two')
  })

  it('clears a pending WebSocket log batch when the consumer unmounts', async () => {
    vi.useFakeTimers()
    await act(async () => root.render(<BuildLogStreamProbe />))
    const socket = MockWebSocket.instances[0]
    act(() => socket?.emitMessage({
      type: 'build:log',
      payload: { timestamp: '10:00:01', stage: 'Build', line: 'must be discarded' },
    }))
    expect(vi.getTimerCount()).toBe(1)

    act(() => root.unmount())
    expect(vi.getTimerCount()).toBe(0)
    act(() => vi.advanceTimersByTime(100))
    expect(container.textContent).toBe('')
    root = createRoot(container)
  })

  it('redirects invalid and unauthenticated routes without issuing API calls', async () => {
    await renderViewer('/builds/not-a-number/logs')
    expect(container.querySelector('[data-route="builds"]')).not.toBeNull()
    expect(getBuild).not.toHaveBeenCalled()
    expect(getBuildLogs).not.toHaveBeenCalled()

    act(() => root.unmount())
    root = createRoot(container)
    localStorage.removeItem('token')
    await renderViewer('/builds/42/logs')
    expect(container.querySelector('[data-route="login"]')).not.toBeNull()
    expect(getBuild).not.toHaveBeenCalled()
    expect(getBuildLogs).not.toHaveBeenCalled()
  })
})
