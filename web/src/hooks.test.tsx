// @vitest-environment jsdom

import { act } from 'react'
import { createRoot, type Root } from 'react-dom/client'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { useApi, useVisiblePolling } from './hooks'

const actEnvironment = globalThis as typeof globalThis & { IS_REACT_ACT_ENVIRONMENT: boolean }

function deferred<T>() {
  let resolve!: (value: T) => void
  let reject!: (reason: Error) => void
  const promise = new Promise<T>((nextResolve, nextReject) => {
    resolve = nextResolve
    reject = nextReject
  })
  return { promise, resolve, reject }
}

function Harness({ version, fetcher }: { version: number; fetcher: () => Promise<string> }) {
  const { data, loading, error, poll } = useApi(fetcher, [version])
  useVisiblePolling(poll, 1000)
  return <div data-loading={loading}>{error || data || ''}</div>
}

describe('useApi', () => {
  let container: HTMLDivElement
  let root: Root

  beforeEach(() => {
    actEnvironment.IS_REACT_ACT_ENVIRONMENT = true
    container = document.createElement('div')
    document.body.appendChild(container)
    root = createRoot(container)
  })

  afterEach(() => {
    act(() => root.unmount())
    container.remove()
    actEnvironment.IS_REACT_ACT_ENVIRONMENT = false
    vi.useRealTimers()
    vi.restoreAllMocks()
  })

  it('ignores a stale response after dependencies change', async () => {
    const first = deferred<string>()
    const second = deferred<string>()
    const fetcher = vi.fn()
      .mockImplementationOnce(() => first.promise)
      .mockImplementationOnce(() => second.promise)

    act(() => root.render(<Harness version={1} fetcher={fetcher} />))
    act(() => root.render(<Harness version={2} fetcher={fetcher} />))

    await act(async () => second.resolve('current'))
    await act(async () => first.resolve('stale'))

    expect(container.textContent).toBe('current')
    expect(container.firstElementChild?.getAttribute('data-loading')).toBe('false')
  })

  it('lets slow background requests finish and resumes after the tab becomes visible', async () => {
    vi.useFakeTimers()
    const visibility = vi.spyOn(document, 'visibilityState', 'get').mockReturnValue('visible')
    const slow = deferred<string>()
    const fetcher = vi.fn()
      .mockResolvedValueOnce('initial')
      .mockReturnValueOnce(slow.promise)
      .mockResolvedValue('resumed')

    await act(async () => root.render(<Harness version={1} fetcher={fetcher} />))
    await act(async () => { await vi.advanceTimersByTimeAsync(3500) })
    expect(fetcher).toHaveBeenCalledTimes(2)
    expect(container.textContent).toBe('initial')
    await act(async () => slow.resolve('updated'))
    expect(container.textContent).toBe('updated')

    visibility.mockReturnValue('hidden')
    act(() => document.dispatchEvent(new Event('visibilitychange')))
    await act(async () => { await vi.advanceTimersByTimeAsync(5000) })
    expect(fetcher).toHaveBeenCalledTimes(2)
    visibility.mockReturnValue('visible')
    await act(async () => { document.dispatchEvent(new Event('visibilitychange')) })
    expect(container.textContent).toBe('resumed')
  })
})
