// @vitest-environment jsdom
import { afterEach, describe, expect, it, vi } from 'vitest'
import { api } from '../api'
import { beginProjectDetailNavigation, loadProjectDetail } from './projectDetailLoader'

vi.mock('../api', () => ({ api: { getProject: vi.fn() } }))

afterEach(() => {
  vi.useRealTimers()
  vi.clearAllMocks()
  localStorage.clear()
})

describe('project detail loading', () => {
  it('shares an in-flight navigation request but fetches again after it completes', async () => {
    let resolve!: (value: unknown) => void
    vi.mocked(api.getProject).mockReturnValueOnce(new Promise(done => { resolve = done }))
      .mockResolvedValueOnce({ id: 7, name: 'updated' })
    const first = loadProjectDetail(7)
    expect(loadProjectDetail(7)).toBe(first)
    expect(api.getProject).toHaveBeenCalledTimes(1)
    resolve({ id: 7, name: 'initial' })
    await expect(first).resolves.toMatchObject({ name: 'initial' })
    await expect(loadProjectDetail(7)).resolves.toMatchObject({ name: 'updated' })
    expect(api.getProject).toHaveBeenCalledTimes(2)
  })

  it('times out, aborts the request and permits a retry', async () => {
    vi.useFakeTimers()
    vi.mocked(api.getProject).mockReturnValueOnce(new Promise(() => {})).mockResolvedValueOnce({ id: 8 })
    const request = loadProjectDetail(8)
    const failure = expect(request).rejects.toThrow(/timed out|超时/)
    const signal = vi.mocked(api.getProject).mock.calls[0][1]?.signal
    await vi.advanceTimersByTimeAsync(12_000)
    await failure
    expect(signal?.aborted).toBe(true)
    await expect(loadProjectDetail(8)).resolves.toEqual({ id: 8 })
  })

  it('uses a completed navigation request once when the route arrives later', async () => {
    vi.mocked(api.getProject).mockResolvedValueOnce({ id: 9 }).mockResolvedValueOnce({ id: 9, updated: true })
    beginProjectDetailNavigation(9)
    await Promise.resolve()
    await Promise.resolve()
    await expect(loadProjectDetail(9)).resolves.toEqual({ id: 9 })
    expect(api.getProject).toHaveBeenCalledTimes(1)
    await expect(loadProjectDetail(9)).resolves.toEqual({ id: 9, updated: true })
    expect(api.getProject).toHaveBeenCalledTimes(2)
  })

  it('does not reuse navigation data after the authentication token changes', async () => {
    vi.useFakeTimers()
    vi.mocked(api.getProject).mockResolvedValueOnce({ id: 10, owner: 'first' }).mockResolvedValueOnce({ id: 10, owner: 'second' })
    localStorage.setItem('token', 'first')
    beginProjectDetailNavigation(10)
    await Promise.resolve()
    await Promise.resolve()
    localStorage.setItem('token', 'second')
    await expect(loadProjectDetail(10)).resolves.toEqual({ id: 10, owner: 'second' })
    expect(api.getProject).toHaveBeenCalledTimes(2)
    await vi.advanceTimersByTimeAsync(10_000)
  })
})
