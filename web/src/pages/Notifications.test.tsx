// @vitest-environment jsdom

import { act } from 'react'
import { createRoot, type Root } from 'react-dom/client'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { api } from '../api'
import Notifications from './Notifications'

vi.mock('../api', () => ({
  api: {
    listNotificationChannels: vi.fn(),
    updateNotificationChannel: vi.fn(),
    createNotificationChannel: vi.fn(),
    deleteNotificationChannel: vi.fn(),
    listNotificationEvents: vi.fn(),
  },
}))

const listNotificationChannels = vi.mocked(api.listNotificationChannels)
const updateNotificationChannel = vi.mocked(api.updateNotificationChannel)
const actEnvironment = globalThis as typeof globalThis & { IS_REACT_ACT_ENVIRONMENT: boolean }

describe('Notifications', () => {
  let container: HTMLDivElement
  let root: Root

  beforeEach(() => {
    actEnvironment.IS_REACT_ACT_ENVIRONMENT = true
    localStorage.setItem('locale', 'zh-CN')
    listNotificationChannels.mockReset()
    updateNotificationChannel.mockReset()
    listNotificationChannels.mockResolvedValue([{
      id: 1,
      name: '页面内通知',
      type: 'web',
      config: '{}',
      conditions: '{}',
      description: '系统标配',
      enabled: true,
      created_at: '2026-07-20T10:00:00Z',
      updated_at: '2026-07-20T10:00:00Z',
    }])
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

  it('renders the built-in Web channel as required instead of an interactive toggle', async () => {
    await act(async () => {
      root.render(<Notifications />)
    })

    expect(container.querySelector('.channel-required')).not.toBeNull()
    expect(container.querySelector('button[aria-label="停用渠道"]')).toBeNull()

    const edit = container.querySelector<HTMLButtonElement>('.row-actions button:nth-child(2)')
    await act(async () => {
      edit?.dispatchEvent(new MouseEvent('click', { bubbles: true }))
    })

    const enabled = document.body.querySelector<HTMLInputElement>('.channel-enabled input')
    expect(enabled?.checked).toBe(true)
    expect(enabled?.disabled).toBe(true)
    expect(document.body.querySelector('.channel-enabled.required small')?.textContent).toBeTruthy()
    expect(updateNotificationChannel).not.toHaveBeenCalled()
  })
})
