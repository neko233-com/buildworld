// @vitest-environment jsdom

import { act } from 'react'
import { createRoot, type Root } from 'react-dom/client'
import { MemoryRouter, Route, Routes } from 'react-router-dom'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { api } from '../api'
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

  it('renders a standalone searchable log surface with download controls', async () => {
    await renderViewer()

    expect(container.querySelector('h1')?.textContent).toBe('Standalone log viewer')
    expect(container.querySelector('a[href="/builds/42"]')).not.toBeNull()
    expect(container.querySelector('a[href="/"]')).toBeNull()
    expect(container.querySelectorAll('.plain-log-lines > div')).toHaveLength(3)
    expect(container.querySelectorAll('.plain-log-lines > .error')).toHaveLength(2)
    expect(document.title).toBe('Logs · #7 · buildworld')

    const search = container.querySelector<HTMLInputElement>('input[aria-label="Search logs"]')!
    await act(async () => {
      Object.getOwnPropertyDescriptor(HTMLInputElement.prototype, 'value')?.set?.call(search, 'error')
      search.dispatchEvent(new Event('input', { bubbles: true }))
    })
    expect(container.textContent).toContain('1 of 1 matches')
    expect(container.querySelector('mark')?.textContent).toBe('ERROR')

    const wrap = Array.from(container.querySelectorAll<HTMLButtonElement>('button'))
      .find(button => button.textContent?.includes('Wrap lines'))!
    act(() => wrap.click())
    expect(wrap.getAttribute('aria-pressed')).toBe('true')
    expect(container.querySelector('.plain-log-page')?.classList.contains('wrap-lines')).toBe(true)

    const txtDownload = Array.from(container.querySelectorAll<HTMLButtonElement>('button'))
      .find(button => button.textContent?.includes('.txt'))!
    await act(async () => txtDownload.click())
    expect(downloadBuildLogs).toHaveBeenCalledWith(42, 'txt')
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

    await act(async () => socket?.emitMessage({
      type: 'build:log',
      payload: { timestamp: '10:00:02', stage: 'Build', line: 'compile completed' },
    }))
    expect(container.textContent).toContain('[10:00:02] [Build] compile completed')
  })

  it('follows new live output until the user scrolls away from the bottom', async () => {
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
    expect(viewport.scrollTop).toBe(500)
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
