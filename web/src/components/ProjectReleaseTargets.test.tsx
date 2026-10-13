// @vitest-environment jsdom
import { act } from 'react'
import { createRoot, type Root } from 'react-dom/client'
import { MemoryRouter } from 'react-router-dom'
import { afterEach, beforeEach, expect, it, vi } from 'vitest'
import { api } from '../api'
import { formatDateTime } from '../lib/dateTime'
import ProjectReleaseTargets from './ProjectReleaseTargets'

vi.mock('../api', () => ({ api: { listReleaseTargets: vi.fn() } }))
let container: HTMLDivElement
let root: Root
beforeEach(() => {
  ;(globalThis as any).IS_REACT_ACT_ENVIRONMENT = true
  localStorage.setItem('locale', 'en')
  container = document.createElement('div')
  document.body.append(container)
  root = createRoot(container)
})
afterEach(() => {
  act(() => root.unmount())
  container.remove()
  localStorage.clear()
  vi.clearAllMocks()
})

it('shows completed publication time and independent target history links', async () => {
  const finished = '2026-10-13T01:23:45Z'
  vi.mocked(api.listReleaseTargets).mockResolvedValue([
    { channel: 'wechat', environment: 'test', latest: { id: 125, number: 9, status: 'failed' }, last_success: { id: 123, number: 7, status: 'success', started_at: '2026-10-12T01:00:00Z', finished_at: finished } },
    { channel: 'taptap', environment: 'release', latest: { id: 124, number: 8, status: 'failed' }, last_success: null },
  ])
  await act(async () => root.render(<MemoryRouter><ProjectReleaseTargets projectId={33} /></MemoryRouter>))
  expect(container.textContent).toContain(formatDateTime(finished))
  expect(container.textContent).toContain('No successful publication')
  expect(container.querySelector('a[href="/builds/123"]')?.textContent).toContain('#7')
  const historyLinks = [...container.querySelectorAll('a')].map(link => link.getAttribute('href'))
  expect(historyLinks).toContain('/builds?project=33&release_channel=wechat&release_environment=test')
  expect(historyLinks).toContain('/builds?project=33&release_channel=taptap&release_environment=release')
})

it('keeps legacy projects unclassified and hides an empty target panel', async () => {
  vi.mocked(api.listReleaseTargets).mockResolvedValue([])
  await act(async () => root.render(<MemoryRouter><ProjectReleaseTargets projectId={33} /></MemoryRouter>))
  expect(container.textContent).toBe('')
})

it('allows retry after a history failure', async () => {
  vi.mocked(api.listReleaseTargets).mockRejectedValueOnce(new Error('history unavailable')).mockResolvedValue([])
  await act(async () => root.render(<MemoryRouter><ProjectReleaseTargets projectId={33} /></MemoryRouter>))
  expect(container.querySelector('[role="alert"]')?.textContent).toContain('history unavailable')
  await act(async () => container.querySelector('button')!.click())
  expect(api.listReleaseTargets).toHaveBeenCalledTimes(2)
  expect(container.textContent).toBe('')
})
