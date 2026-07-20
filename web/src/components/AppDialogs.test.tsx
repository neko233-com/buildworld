// @vitest-environment jsdom

import { act } from 'react'
import { createRoot, type Root } from 'react-dom/client'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { API_FEEDBACK_EVENT, type APIFeedbackDetail } from '../api'
import { AppDialogs, dialogs } from './AppDialogs'

const actEnvironment = globalThis as typeof globalThis & { IS_REACT_ACT_ENVIRONMENT: boolean }

describe('application confirmation dialog', () => {
  let container: HTMLDivElement
  let root: Root

  beforeEach(() => {
    actEnvironment.IS_REACT_ACT_ENVIRONMENT = true
    vi.useFakeTimers()
    localStorage.setItem('locale', 'zh-CN')
    container = document.createElement('div')
    document.body.appendChild(container)
    root = createRoot(container)
    vi.spyOn(window, 'requestAnimationFrame').mockImplementation(callback => {
      callback(0)
      return 1
    })
    vi.spyOn(window, 'cancelAnimationFrame').mockImplementation(() => undefined)
    act(() => root.render(<AppDialogs />))
  })

  afterEach(() => {
    act(() => root.unmount())
    vi.runAllTimers()
    container.remove()
    localStorage.clear()
    actEnvironment.IS_REACT_ACT_ENVIRONMENT = false
    vi.useRealTimers()
    vi.restoreAllMocks()
  })

  it('focuses the safe action, closes with Escape, and restores focus', async () => {
    const trigger = document.createElement('button')
    trigger.textContent = '删除'
    document.body.appendChild(trigger)
    trigger.focus()

    let result!: Promise<boolean>
    await act(async () => {
      result = dialogs.confirm('确定删除吗？', { title: '删除项目', action: '删除' })
    })

    expect(document.querySelector('[role="dialog"]')?.getAttribute('aria-label')).toBe('删除项目')
    expect(document.activeElement?.hasAttribute('data-dialog-cancel')).toBe(true)

    await act(async () => {
      window.dispatchEvent(new window.KeyboardEvent('keydown', { key: 'Escape', bubbles: true }))
    })
    act(() => vi.runAllTimers())

    await expect(result).resolves.toBe(false)
    expect(document.querySelector('[role="dialog"]')).toBeNull()
    expect(document.activeElement).toBe(trigger)
    trigger.remove()
  })

  it('replaces progress with success and keeps simultaneous errors visible', async () => {
    const dispatch = (detail: APIFeedbackDetail) => window.dispatchEvent(
      new CustomEvent<APIFeedbackDetail>(API_FEEDBACK_EVENT, { detail }),
    )
    await act(async () => {
      dispatch({ operationId: 'save-group', phase: 'start', tone: 'info', message: '正在处理操作…' })
      dispatch({ operationId: 'save-group', phase: 'success', tone: 'success', message: '操作已完成' })
      dispatch({ operationId: 'delete-group', phase: 'error', tone: 'error', message: '无法删除项目分组' })
    })

    expect(document.querySelectorAll('.app-notice')).toHaveLength(2)
    expect(document.querySelector('[role="status"]')?.textContent).toContain('操作已完成')
    expect(document.querySelector('[role="alert"]')?.textContent).toContain('无法删除项目分组')
    expect(['操作通知', 'Operation notifications']).toContain(
      document.querySelector('.app-notices')?.getAttribute('aria-label'),
    )
  })
})
