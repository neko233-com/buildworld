// @vitest-environment jsdom

import { act } from 'react'
import { createRoot, type Root } from 'react-dom/client'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { PageState } from './PageState'

const actEnvironment = globalThis as typeof globalThis & { IS_REACT_ACT_ENVIRONMENT: boolean }

describe('PageState', () => {
  let container: HTMLDivElement
  let root: Root

  beforeEach(() => {
    actEnvironment.IS_REACT_ACT_ENVIRONMENT = true
    localStorage.setItem('locale', 'zh-CN')
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

  it('renders an accessible loading state', () => {
    act(() => root.render(<PageState />))
    expect(container.querySelector('[role="status"]')?.textContent).toMatch(/Loading|加载/)
  })

  it('renders the error detail and retries in place', () => {
    const retry = vi.fn()
    act(() => root.render(<PageState error="构建服务暂时不可用" onRetry={retry} />))
    expect(container.querySelector('[role="alert"]')?.textContent).toContain('构建服务暂时不可用')
    const button = container.querySelector<HTMLButtonElement>('button')!
    act(() => button.click())
    expect(retry).toHaveBeenCalledOnce()
  })
})
