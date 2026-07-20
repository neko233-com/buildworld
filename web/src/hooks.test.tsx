// @vitest-environment jsdom

import { act } from 'react'
import { createRoot, type Root } from 'react-dom/client'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { useApi } from './hooks'

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
  const { data, loading, error } = useApi(fetcher, [version])
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
})
