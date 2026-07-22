// @vitest-environment jsdom

import { act } from 'react'
import { createRoot, type Root } from 'react-dom/client'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { api } from '../api'
import RunBuildDialog from './RunBuildDialog'

vi.mock('../api', () => ({
  api: {
    validateProject: vi.fn(),
    triggerBuild: vi.fn(),
  },
}))

describe('RunBuildDialog validation metadata', () => {
  let container: HTMLDivElement
  let root: Root

  beforeEach(() => {
    ;(globalThis as any).IS_REACT_ACT_ENVIRONMENT = true
    container = document.createElement('div')
    document.body.appendChild(container)
    root = createRoot(container)
    vi.spyOn(window, 'requestAnimationFrame').mockImplementation(callback => {
      callback(0)
      return 1
    })
    vi.spyOn(window, 'cancelAnimationFrame').mockImplementation(() => undefined)
  })

  afterEach(() => {
    act(() => root.unmount())
    document.querySelector('.schedule-modal-backdrop')?.remove()
    container.remove()
    vi.clearAllMocks()
    vi.restoreAllMocks()
    ;(globalThis as any).IS_REACT_ACT_ENVIRONMENT = false
  })

  it('blocks queueing until authoritative parameter metadata can be loaded', async () => {
    vi.mocked(api.validateProject).mockRejectedValueOnce(new Error('validation unavailable'))
      .mockResolvedValueOnce({ parameters: [] } as any)

    await act(async () => {
      root.render(<RunBuildDialog project={{ id: 7, name: 'Weather', default_branch: 'main' }} onClose={() => undefined} onQueued={() => undefined} />)
      await Promise.resolve()
      await Promise.resolve()
    })

    const dialog = document.querySelector('[role="dialog"]')
    const submit = dialog?.querySelector<HTMLButtonElement>('button[type="submit"]')
    expect(dialog?.querySelector('[role="alert"]')?.textContent).toContain('validation unavailable')
    expect(submit?.disabled).toBe(true)
    expect(api.triggerBuild).not.toHaveBeenCalled()

    await act(async () => {
      dialog?.querySelector<HTMLButtonElement>('.run-parameter-empty button')?.click()
      await Promise.resolve()
      await Promise.resolve()
    })

    expect(api.validateProject).toHaveBeenCalledTimes(2)
    expect(document.querySelector<HTMLButtonElement>('[role="dialog"] button[type="submit"]')?.disabled).toBe(false)
  })

  it('focuses and describes the first missing required parameter', async () => {
    vi.mocked(api.validateProject).mockResolvedValue({
      valid: true,
      format: 'typescript',
      stages: 1,
      steps: 1,
      parameters: [
        { name: 'SIGNING_TOKEN', type: 'password', required: true, is_secret: true, description: 'Release credential' },
        { name: 'CHANNEL', type: 'string', required: true },
      ],
      allow_long_running: false,
    })

    await act(async () => {
      root.render(<RunBuildDialog project={{ id: 7, name: 'Weather', default_branch: 'main' }} onClose={() => undefined} onQueued={() => undefined} />)
      await Promise.resolve()
      await Promise.resolve()
    })

    const dialog = document.querySelector<HTMLElement>('[role="dialog"]')!
    await act(async () => {
      dialog.querySelector('form')?.dispatchEvent(new Event('submit', { bubbles: true, cancelable: true }))
      await Promise.resolve()
    })

    const token = dialog.querySelector<HTMLInputElement>('input[type="password"]')!
    expect(token.getAttribute('aria-invalid')).toBe('true')
    const describedBy = token.getAttribute('aria-describedby') || ''
    expect(describedBy.split(' ').some(id => document.getElementById(id)?.textContent?.includes('SIGNING_TOKEN'))).toBe(true)
    expect(document.activeElement).toBe(token)
    expect(api.triggerBuild).not.toHaveBeenCalled()
  })

  it('locks close, cancel, and submit controls while queueing', async () => {
    let resolveBuild!: (build: { id: number }) => void
    const buildRequest = new Promise<{ id: number }>(resolve => { resolveBuild = resolve })
    const onClose = vi.fn()
    const onQueued = vi.fn()
    vi.mocked(api.validateProject).mockResolvedValue({ valid: true, format: 'typescript', stages: 1, steps: 1, parameters: [], allow_long_running: false })
    vi.mocked(api.triggerBuild).mockReturnValue(buildRequest)

    await act(async () => {
      root.render(<RunBuildDialog project={{ id: 7, name: 'Weather', default_branch: 'main' }} onClose={onClose} onQueued={onQueued} />)
      await Promise.resolve()
      await Promise.resolve()
    })

    const dialog = document.querySelector<HTMLElement>('[role="dialog"]')!
    await act(async () => {
      dialog.querySelector('form')?.dispatchEvent(new Event('submit', { bubbles: true, cancelable: true }))
      await Promise.resolve()
    })

    const close = dialog.querySelector<HTMLButtonElement>('header > button')!
    const footerButtons = Array.from(dialog.querySelectorAll<HTMLButtonElement>('footer button'))
    expect(close.disabled).toBe(true)
    expect(footerButtons.every(button => button.disabled)).toBe(true)
    expect(dialog.getAttribute('aria-busy')).toBe('true')
    window.dispatchEvent(new KeyboardEvent('keydown', { key: 'Escape', bubbles: true }))
    close.click()
    footerButtons[0].click()
    expect(onClose).not.toHaveBeenCalled()

    await act(async () => {
      resolveBuild({ id: 901 })
      await buildRequest
    })
    expect(onQueued).toHaveBeenCalledWith({ id: 901 })
  })

  it('keeps trigger failures visible and unlocks the dialog', async () => {
    vi.mocked(api.validateProject).mockResolvedValue({ valid: true, format: 'typescript', stages: 1, steps: 1, parameters: [], allow_long_running: false })
    vi.mocked(api.triggerBuild).mockRejectedValue(new Error('queue service unavailable'))

    await act(async () => {
      root.render(<RunBuildDialog project={{ id: 7, name: 'Weather', default_branch: 'main' }} onClose={() => undefined} onQueued={() => undefined} />)
      await Promise.resolve()
      await Promise.resolve()
    })
    const dialog = document.querySelector<HTMLElement>('[role="dialog"]')!
    await act(async () => {
      dialog.querySelector('form')?.dispatchEvent(new Event('submit', { bubbles: true, cancelable: true }))
      await Promise.resolve()
      await Promise.resolve()
    })

    expect(dialog.querySelector('.run-build-error[role="alert"]')?.textContent).toContain('queue service unavailable')
    expect(dialog.querySelector<HTMLButtonElement>('header > button')?.disabled).toBe(false)
    expect(dialog.querySelector<HTMLButtonElement>('button[type="submit"]')?.disabled).toBe(false)
  })
})
