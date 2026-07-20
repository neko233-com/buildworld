// @vitest-environment jsdom

import { StrictMode, useState } from 'react'
import { act } from 'react'
import { createRoot, type Root } from 'react-dom/client'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { ModalDialog } from './ModalDialog'

const actEnvironment = globalThis as typeof globalThis & { IS_REACT_ACT_ENVIRONMENT: boolean }

function Harness({ busy = false }: { busy?: boolean }) {
  const [open, setOpen] = useState(false)
  return <>
    <button type="button" onClick={() => setOpen(true)}>打开编辑器</button>
    {open && <ModalDialog ariaLabel="编辑配置" busy={busy} onClose={() => setOpen(false)}>
      <input aria-label="名称" />
      <button type="button" data-dialog-initial-focus>取消</button>
      <button type="button">保存</button>
    </ModalDialog>}
  </>
}

describe('ModalDialog', () => {
  let container: HTMLDivElement
  let root: Root

  beforeEach(() => {
    actEnvironment.IS_REACT_ACT_ENVIRONMENT = true
    vi.useFakeTimers()
    container = document.createElement('div')
    container.id = 'root'
    document.body.appendChild(container)
    root = createRoot(container)
    vi.spyOn(window, 'requestAnimationFrame').mockImplementation(callback => {
      callback(0)
      return 1
    })
    vi.spyOn(window, 'cancelAnimationFrame').mockImplementation(() => undefined)
    act(() => root.render(<StrictMode><Harness /></StrictMode>))
  })

  afterEach(() => {
    act(() => root.unmount())
    vi.runAllTimers()
    container.remove()
    document.body.style.overflow = ''
    actEnvironment.IS_REACT_ACT_ENVIRONMENT = false
    vi.useRealTimers()
    vi.restoreAllMocks()
  })

  it('focuses the preferred control, traps Tab, closes with Escape, and restores focus', () => {
    const trigger = container.querySelector<HTMLButtonElement>('button')!
    trigger.focus()
    act(() => trigger.click())

    const dialog = document.querySelector<HTMLElement>('[role="dialog"]')!
    const cancel = document.querySelector<HTMLButtonElement>('[data-dialog-initial-focus]')!
    const save = Array.from(dialog.querySelectorAll('button')).find(button => button.textContent === '保存')!
    expect(document.activeElement).toBe(cancel)
    expect(document.body.style.overflow).toBe('hidden')
    expect(container.hasAttribute('inert')).toBe(true)
    expect(container.getAttribute('aria-hidden')).toBe('true')

    const focusables = Array.from(dialog.querySelectorAll<HTMLElement>('*')).filter(element => element.matches('[data-dialog-initial-focus],button:not(:disabled),input:not(:disabled):not([type="hidden"]),select:not(:disabled),textarea:not(:disabled),[href],[tabindex]:not([tabindex="-1"])'))
    expect(focusables.map(element => element.textContent || element.getAttribute('aria-label'))).toEqual(['名称', '取消', '保存'])
    save.focus()
    act(() => window.dispatchEvent(new window.KeyboardEvent('keydown', { key: 'Tab', code: 'Tab', bubbles: true, cancelable: true })))
    expect(document.activeElement).toBe(dialog.querySelector('input'))

    act(() => window.dispatchEvent(new window.KeyboardEvent('keydown', { key: 'Escape', code: 'Escape', bubbles: true, cancelable: true })))
    act(() => vi.runAllTimers())
    expect(document.querySelector('[role="dialog"]')).toBeNull()
    expect(document.activeElement).toBe(trigger)
    expect(document.body.style.overflow).toBe('')
    expect(container.hasAttribute('inert')).toBe(false)
    expect(container.hasAttribute('aria-hidden')).toBe(false)
  })

  it('does not close a busy dialog with Escape', () => {
    act(() => root.render(<StrictMode><Harness busy /></StrictMode>))
    const trigger = container.querySelector<HTMLButtonElement>('button')!
    act(() => trigger.click())

    act(() => window.dispatchEvent(new window.KeyboardEvent('keydown', { key: 'Escape', code: 'Escape', bubbles: true, cancelable: true })))
    expect(document.querySelector('[role="dialog"]')).not.toBeNull()
  })
})
