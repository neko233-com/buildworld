// @vitest-environment jsdom

import { act } from 'react'
import { createRoot } from 'react-dom/client'
import { expect, it, vi } from 'vitest'
import { api } from '../api'
import AuditLog from './AuditLog'

vi.mock('../api', () => ({ api: { listAuditLogs: vi.fn() } }))

it('retains audit rows and prevents duplicate refreshes while waiting for the server', async () => {
  const testEnvironment = globalThis as typeof globalThis & { IS_REACT_ACT_ENVIRONMENT: boolean }
  testEnvironment.IS_REACT_ACT_ENVIRONMENT = true
  const container = document.createElement('div')
  const root = createRoot(container)
  const listAuditLogs = vi.mocked(api.listAuditLogs)
  const previous = [{ id: 20, action: 'previous-event' }]
  const updated = [{ id: 21, action: 'new-event' }]
  let completeRefresh!: (rows: typeof updated) => void
  listAuditLogs.mockResolvedValueOnce(previous).mockReturnValueOnce(new Promise(resolve => {
    completeRefresh = resolve
  }))

  try {
    await act(async () => root.render(<AuditLog />))
    const refresh = container.querySelector<HTMLButtonElement>('.operations-heading button')!
    act(() => refresh.click())
    expect(refresh.disabled).toBe(true)
    expect(refresh.getAttribute('aria-busy')).toBe('true')
    expect(container.querySelector('tbody')?.textContent).toContain('previous-event')
    act(() => refresh.click())
    expect(listAuditLogs).toHaveBeenCalledTimes(2)

    await act(async () => completeRefresh(updated))
    expect(refresh.disabled).toBe(false)
    expect(refresh.getAttribute('aria-busy')).toBe('false')
    expect(container.querySelector('tbody')?.textContent).toContain('new-event')
  } finally {
    act(() => root.unmount())
    testEnvironment.IS_REACT_ACT_ENVIRONMENT = false
  }
})
