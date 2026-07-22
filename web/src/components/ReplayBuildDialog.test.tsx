// @vitest-environment jsdom

import { act } from 'react'
import { createRoot, type Root } from 'react-dom/client'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { api } from '../api'
import { dialogs } from './AppDialogs'
import ReplayBuildDialog, { replayParameterEntries } from './ReplayBuildDialog'

vi.mock('../api', () => ({ api: { getBuild: vi.fn(), retryBuild: vi.fn() } }))
vi.mock('./AppDialogs', () => ({ dialogs: { notify: vi.fn() } }))

function deferred<T>() {
  let resolve!: (value: T) => void
  let reject!: (reason: unknown) => void
  const promise = new Promise<T>((resolvePromise, rejectPromise) => {
    resolve = resolvePromise
    reject = rejectPromise
  })
  return { promise, resolve, reject }
}

const target = {
  id: 57,
  number: 4,
  projectId: 9,
  projectName: 'Weather',
  branch: 'release/2026',
  parameters: '{"ENV":"production","API_TOKEN":"never-show"}',
}

describe('ReplayBuildDialog', () => {
  let container: HTMLDivElement
  let root: Root

  beforeEach(() => {
    ;(globalThis as any).IS_REACT_ACT_ENVIRONMENT = true
    localStorage.setItem('locale', 'en')
    container = document.createElement('div')
    container.id = 'root'
    document.body.appendChild(container)
    root = createRoot(container)
  })

  afterEach(() => {
    act(() => root.unmount())
    container.remove()
    localStorage.clear()
    vi.clearAllMocks()
    ;(globalThis as any).IS_REACT_ACT_ENVIRONMENT = false
  })

  it('summarizes the source and only calls retryBuild after explicit Replay', async () => {
    const request = deferred<{ id: number }>()
    vi.mocked(api.retryBuild).mockReturnValue(request.promise)
    const onReplayed = vi.fn()
    const onBusyChange = vi.fn()

    await act(async () => {
      root.render(<ReplayBuildDialog target={target} onClose={vi.fn()} onReplayed={onReplayed} onBusyChange={onBusyChange} />)
    })

    const dialog = document.querySelector<HTMLElement>('.jenkins-replay-dialog')!
    expect(dialog.textContent).toContain('Weather #4')
    expect(dialog.textContent).toContain('release/2026')
    expect(dialog.textContent).toContain('production')
    expect(dialog.textContent).toContain('••••••••')
    expect(dialog.textContent).not.toContain('never-show')
    expect(api.retryBuild).not.toHaveBeenCalled()

    const submit = dialog.querySelector<HTMLButtonElement>('.jenkins-replay-submit')!
    await act(async () => submit.click())
    expect(api.retryBuild).toHaveBeenCalledWith(57)
    expect(submit.disabled).toBe(true)
    expect(submit.getAttribute('aria-busy')).toBe('true')
    await act(async () => submit.click())
    expect(api.retryBuild).toHaveBeenCalledTimes(1)

    await act(async () => request.resolve({ id: 58 }))
    expect(onBusyChange).toHaveBeenNthCalledWith(1, true)
    expect(onBusyChange).toHaveBeenLastCalledWith(false)
    expect(dialogs.notify).toHaveBeenCalledWith(expect.stringContaining('Weather #4'), 'success')
    expect(onReplayed).toHaveBeenCalledWith(58)
  })

  it('loads missing history details and keeps API failures visible and retryable', async () => {
    vi.mocked(api.getBuild).mockResolvedValue({ branch: 'main', parameters: '{"region":"cn"}' })
    vi.mocked(api.retryBuild).mockRejectedValue(new Error('worker unavailable'))

    await act(async () => {
      root.render(<ReplayBuildDialog target={{ ...target, branch: undefined, parameters: undefined }} loadDetails onClose={vi.fn()} onReplayed={vi.fn()} />)
      await Promise.resolve()
      await Promise.resolve()
    })

    const dialog = document.querySelector<HTMLElement>('.jenkins-replay-dialog')!
    expect(api.getBuild).toHaveBeenCalledWith(57)
    expect(dialog.textContent).toContain('main')
    expect(dialog.textContent).toContain('cn')

    await act(async () => dialog.querySelector<HTMLButtonElement>('.jenkins-replay-submit')!.click())
    expect(dialog.querySelector('[role="alert"]')?.textContent).toContain('worker unavailable')
    expect(dialog.querySelector<HTMLButtonElement>('.jenkins-replay-submit')?.disabled).toBe(false)
  })
})

describe('replayParameterEntries', () => {
  it('accepts object payloads and redacts secret-shaped names', () => {
    expect(replayParameterEntries({ target: 'prod', signing_key: 'secret' })).toEqual([
      ['target', 'prod'],
      ['signing_key', '••••••••'],
    ])
    expect(replayParameterEntries('not-json')).toEqual([])
  })
})
