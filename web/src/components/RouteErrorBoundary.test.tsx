// @vitest-environment jsdom

import { act } from 'react'
import { createRoot, type Root } from 'react-dom/client'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { chunkRecoveryKey, isChunkLoadError, RouteErrorBoundary } from './RouteErrorBoundary'

const actEnvironment = globalThis as typeof globalThis & { IS_REACT_ACT_ENVIRONMENT: boolean }
const chunkError = new TypeError('Failed to fetch dynamically imported module: /assets/Builds-old.js')

function BrokenRoute(): never {
  throw chunkError
}

describe('RouteErrorBoundary deployment recovery', () => {
  let container: HTMLDivElement
  let root: Root

  beforeEach(() => {
    actEnvironment.IS_REACT_ACT_ENVIRONMENT = true
    sessionStorage.clear()
    container = document.createElement('div')
    document.body.appendChild(container)
    root = createRoot(container)
    vi.spyOn(console, 'error').mockImplementation(() => undefined)
  })

  afterEach(() => {
    act(() => root.unmount())
    container.remove()
    sessionStorage.clear()
    actEnvironment.IS_REACT_ACT_ENVIRONMENT = false
    vi.restoreAllMocks()
  })

  it('recognizes stale lazy-route module failures without classifying ordinary render errors', () => {
    expect(isChunkLoadError(chunkError)).toBe(true)
    expect(isChunkLoadError(new Error('Cannot read properties of undefined'))).toBe(false)
  })

  it('reloads once for a stale deployment chunk', async () => {
    const reloadPage = vi.fn()
    await act(async () => {
      root.render(<RouteErrorBoundary resetKey="/builds" reloadPage={reloadPage}><BrokenRoute /></RouteErrorBoundary>)
    })

    expect(reloadPage).toHaveBeenCalledOnce()
    expect(sessionStorage.getItem(chunkRecoveryKey(chunkError))).toBeTruthy()
  })

  it('stops reload loops and offers an explicit latest-version action', async () => {
    const reloadPage = vi.fn()
    sessionStorage.setItem(chunkRecoveryKey(chunkError), 'already-attempted')
    await act(async () => {
      root.render(<RouteErrorBoundary resetKey="/builds" reloadPage={reloadPage}><BrokenRoute /></RouteErrorBoundary>)
    })

    expect(reloadPage).not.toHaveBeenCalled()
    expect(container.textContent).toContain('应用已更新')
    const recovery = Array.from(container.querySelectorAll('button')).find(button => button.textContent?.includes('加载最新版本'))
    expect(recovery).toBeTruthy()

    await act(async () => recovery?.click())
    expect(reloadPage).toHaveBeenCalledOnce()
  })
})
