// @vitest-environment jsdom
import { act } from 'react'
import { createRoot, type Root } from 'react-dom/client'
import { MemoryRouter } from 'react-router-dom'
import { afterEach, beforeEach, expect, it, vi } from 'vitest'
import { api, type WorkerRuntimeView } from '../api'
import { WorkerRuntimeDialog } from './WorkerRuntimeDialog'

vi.mock('../api', () => ({ api: { workerRuntime: vi.fn(), workerOperation: vi.fn(), uploadWorkerUpdate: vi.fn() } }))
vi.mock('../components/ModalDialog', () => ({ ModalDialog: ({ children }: { children: React.ReactNode }) => <section>{children}</section> }))
vi.mock('../i18n', () => ({ useI18n: () => ({ t: (key: string) => key }) }))
let root: Root
let container: HTMLDivElement
const fixture = (name: string):WorkerRuntimeView => ({ worker: { name, status: 'online' }, draining: false, observed_at: '2026-11-13T10:00:00Z', runtime: { version: 'v1.17.0',source_commit:'a'.repeat(40),os:'windows',arch:'amd64',pid:1,session:'1',goroutines:10,heap_bytes:1024,tasks: [],capacity:2 }, operations: [] })
beforeEach(() => {
  (globalThis as typeof globalThis & { IS_REACT_ACT_ENVIRONMENT: boolean }).IS_REACT_ACT_ENVIRONMENT = true
  vi.useFakeTimers(); vi.clearAllMocks()
  container = document.createElement('div'); document.body.appendChild(container); root = createRoot(container)
})
afterEach(() => { act(() => root.unmount()); container.remove(); vi.useRealTimers(); vi.unstubAllGlobals() })
const render = async (id: string) => { await act(async () => { root.render(<MemoryRouter><WorkerRuntimeDialog key={id} id={id} onClose={() => {}} /></MemoryRouter>); await Promise.resolve() }) }

it('ignores a late snapshot after switching worker context', async () => {
  let finish!: (value: ReturnType<typeof fixture>) => void
  vi.mocked(api.workerRuntime).mockImplementationOnce(() => new Promise(resolve => { finish = resolve })).mockResolvedValueOnce(fixture('second') as never)
  await render('first'); await render('second')
  await act(async () => finish(fixture('first')))
  expect(container.textContent).toContain('second'); expect(container.textContent).not.toContain('first')
})

it('keeps editable capacity during polling errors and waits for operation receipt', async () => {
  vi.mocked(api.workerRuntime).mockResolvedValueOnce(fixture('worker') as never).mockRejectedValueOnce(new Error('snapshot retry required'))
  await render('worker')
  const input = container.querySelector<HTMLInputElement>('input[type=number]')!
  await act(async () => { Object.getOwnPropertyDescriptor(HTMLInputElement.prototype, 'value')!.set!.call(input, '7'); input.dispatchEvent(new Event('input', { bubbles: true })) })
  await act(async () => { await vi.advanceTimersByTimeAsync(5000) })
  expect(input.value).toBe('7'); expect(container.textContent).toContain('snapshot retry required')
  vi.mocked(api.workerOperation).mockResolvedValue({ id: 'operation-1' } as never)
  await act(async () => { Array.from(container.querySelectorAll('button')).find(button => button.textContent === 'workerOps.apply')!.click() })
  expect(api.workerOperation).toHaveBeenCalledWith('worker', expect.objectContaining({ kind: 'concurrency', capacity: 7 }))
  expect(container.textContent).toContain('operation-1')
  expect(container.textContent).toContain('workerOps.queued')
})

it('supports HTTP origins without randomUUID and preserves action failures through polling', async () => {
  vi.stubGlobal('crypto', { getRandomValues: (bytes: Uint8Array) => bytes.fill(7) })
  vi.mocked(api.workerRuntime).mockResolvedValue(fixture('worker') as never)
  vi.mocked(api.workerOperation).mockRejectedValue(new Error('operation refused'))
  await render('worker')
  await act(async () => { Array.from(container.querySelectorAll('button')).find(button => button.textContent === 'workerOps.diagnose')!.click() })
  expect(api.workerOperation).toHaveBeenCalledWith('worker', expect.objectContaining({ request_id: '07070707-0707-4707-8707-070707070707', kind: 'diagnose' }))
  expect(container.textContent).toContain('operation refused')
  await act(async () => { await vi.advanceTimersByTimeAsync(5000) })
  expect(container.textContent).toContain('operation refused')
})
