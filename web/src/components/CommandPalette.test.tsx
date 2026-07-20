// @vitest-environment jsdom

import { act } from 'react'
import { createRoot, type Root } from 'react-dom/client'
import { LayoutDashboard } from 'lucide-react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { CommandPalette, type CommandPaletteGroup } from './CommandPalette'

const actEnvironment = globalThis as typeof globalThis & { IS_REACT_ACT_ENVIRONMENT: boolean }
const groups: CommandPaletteGroup[] = [{
  id: 'commands',
  label: '命令',
  items: [
    { id: 'dashboard', label: '仪表盘', href: '/', icon: LayoutDashboard },
    { id: 'projects', label: '项目', href: '/projects', icon: LayoutDashboard },
  ],
}]

describe('global command palette', () => {
  let container: HTMLDivElement
  let root: Root
  let origin: HTMLButtonElement
  let navigate = vi.fn<(href: string) => void>()
  let close = vi.fn<() => void>()

  beforeEach(() => {
    actEnvironment.IS_REACT_ACT_ENVIRONMENT = true
    container = document.createElement('div')
    origin = document.createElement('button')
    origin.textContent = '打开搜索'
    document.body.append(origin, container)
    origin.focus()
    root = createRoot(container)
    navigate = vi.fn<(href: string) => void>()
    close = vi.fn<() => void>()
    vi.spyOn(window, 'requestAnimationFrame').mockImplementation(callback => {
      callback(0)
      return 1
    })
    act(() => root.render(
      <CommandPalette
        ariaLabel="全局搜索"
        placeholder="搜索项目、构建和命令"
        query=""
        groups={groups}
        loadingLabel="加载中"
        emptyLabel="无结果"
        keyboardHint="方向键选择"
        onQueryChange={() => undefined}
        onNavigate={navigate}
        onClose={close}
      />,
    ))
  })

  afterEach(() => {
    act(() => root.unmount())
    container.remove()
    origin.remove()
    actEnvironment.IS_REACT_ACT_ENVIRONMENT = false
    vi.restoreAllMocks()
  })

  it('supports arrow-key selection and Enter navigation', () => {
    const input = document.querySelector<HTMLInputElement>('[role="combobox"]')!
    expect(document.activeElement).toBe(input)
    expect(input.getAttribute('aria-activedescendant')).toContain('option-0')

    act(() => input.dispatchEvent(new KeyboardEvent('keydown', { key: 'ArrowDown', bubbles: true })))
    expect(input.getAttribute('aria-activedescendant')).toContain('option-1')

    act(() => input.dispatchEvent(new KeyboardEvent('keydown', { key: 'Enter', bubbles: true })))
    expect(navigate).toHaveBeenCalledWith('/projects')
  })

  it('requests a close when Escape is pressed', () => {
    const input = document.querySelector<HTMLInputElement>('[role="combobox"]')!
    act(() => input.dispatchEvent(new KeyboardEvent('keydown', { key: 'Escape', bubbles: true })))
    expect(close).toHaveBeenCalledOnce()
  })
})
