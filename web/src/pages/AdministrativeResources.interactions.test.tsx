// @vitest-environment jsdom

import { act } from 'react'
import { createRoot, type Root } from 'react-dom/client'
import { MemoryRouter } from 'react-router-dom'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { api } from '../api'
import { dialogs } from '../components/AppDialogs'
import APITokens from './APITokens'
import Credentials from './Credentials'
import VCSRoots from './VCSRoots'

vi.mock('../api', () => ({
  api: {
    listVCSRoots: vi.fn(),
    listCredentials: vi.fn(),
    deleteVCSRoot: vi.fn(),
    deleteCredential: vi.fn(),
    getCredential: vi.fn(),
    listAPITokens: vi.fn(),
    createAPIToken: vi.fn(),
    deleteAPIToken: vi.fn(),
  },
}))

vi.mock('../authz', () => ({ canEdit: () => true, isAdmin: () => true }))

vi.mock('../components/AppDialogs', () => ({
  dialogs: { confirm: vi.fn(), notify: vi.fn() },
}))

const mockedAPI = {
  listVCSRoots: vi.mocked(api.listVCSRoots),
  listCredentials: vi.mocked(api.listCredentials),
  deleteVCSRoot: vi.mocked(api.deleteVCSRoot),
  deleteCredential: vi.mocked(api.deleteCredential),
  listAPITokens: vi.mocked(api.listAPITokens),
  createAPIToken: vi.mocked(api.createAPIToken),
  deleteAPIToken: vi.mocked(api.deleteAPIToken),
}
const confirm = vi.mocked(dialogs.confirm)
const actEnvironment = globalThis as typeof globalThis & { IS_REACT_ACT_ENVIRONMENT: boolean }

function deferred<T>() {
  let resolve!: (value: T | PromiseLike<T>) => void
  const promise = new Promise<T>(next => { resolve = next })
  return { promise, resolve }
}

describe('Jenkins administration resource interactions', () => {
  let container: HTMLDivElement
  let breadcrumbHost: HTMLDivElement
  let root: Root

  beforeEach(() => {
    actEnvironment.IS_REACT_ACT_ENVIRONMENT = true
    vi.clearAllMocks()
    mockedAPI.listVCSRoots.mockResolvedValue([{
      id: 11,
      name: 'game-repository',
      url: 'https://example.invalid/game.git',
      branch: 'main',
      poll_interval: 60,
      auto_checkout: true,
    }])
    mockedAPI.listCredentials.mockResolvedValue([{
      id: 12,
      name: 'deployment-key',
      type: 'ssh_key',
      host: 'example.invalid',
      username: 'git',
      private_key: '********',
      is_secret: true,
    }])
    mockedAPI.listAPITokens.mockResolvedValue([{
      id: 13,
      name: 'release-bot',
      token_prefix: 'bw_fake',
      scopes: ['build:read'],
    }])
    confirm.mockResolvedValue(true)
    breadcrumbHost = document.createElement('div')
    breadcrumbHost.id = 'jenkins-header-breadcrumbs'
    container = document.createElement('div')
    document.body.append(breadcrumbHost, container)
    root = createRoot(container)
  })

  afterEach(() => {
    act(() => root.unmount())
    document.querySelectorAll('.schedule-modal-backdrop').forEach(node => node.remove())
    breadcrumbHost.remove()
    container.remove()
    actEnvironment.IS_REACT_ACT_ENVIRONMENT = false
  })

  async function renderPage(page: React.ReactNode) {
    await act(async () => {
      root.render(<MemoryRouter>{page}</MemoryRouter>)
    })
  }

  it.each([
    { page: <VCSRoots />, remove: mockedAPI.deleteVCSRoot, name: 'game-repository' },
    { page: <Credentials />, remove: mockedAPI.deleteCredential, name: 'deployment-key' },
    { page: <APITokens />, remove: mockedAPI.deleteAPIToken, name: 'release-bot' },
  ])('locks $name deletion until the request settles', async ({ page, remove, name }) => {
    const pending = deferred<unknown>()
    remove.mockReturnValueOnce(pending.promise)
    await renderPage(page)

    const removeButton = container.querySelector<HTMLButtonElement>('.row-icon.danger')!
    await act(async () => {
      removeButton.click()
      await Promise.resolve()
    })

    expect(confirm.mock.calls.at(-1)?.[0]).toContain(name)
    expect(removeButton.disabled).toBe(true)
    expect(removeButton.getAttribute('aria-busy')).toBe('true')
    expect(removeButton.querySelector('.timeline-spinner')).not.toBeNull()

    await act(async () => {
      pending.resolve(undefined)
      await pending.promise
    })
    expect(removeButton.disabled).toBe(false)
  })

  it('locks API token close and cancel actions while creation is pending', async () => {
    mockedAPI.listAPITokens.mockResolvedValueOnce([])
    const pending = deferred<any>()
    mockedAPI.createAPIToken.mockReturnValueOnce(pending.promise)
    await renderPage(<APITokens />)

    act(() => container.querySelector<HTMLButtonElement>('.primary-command')!.click())
    const dialog = document.querySelector<HTMLElement>('.api-token-editor')!
    const nameInput = dialog.querySelector<HTMLInputElement>('input[required]')!
    const setInputValue = Object.getOwnPropertyDescriptor(HTMLInputElement.prototype, 'value')!.set!
    act(() => {
      setInputValue.call(nameInput, 'automation-test')
      nameInput.dispatchEvent(new Event('input', { bubbles: true }))
    })

    await act(async () => {
      dialog.querySelector<HTMLFormElement>('form')!.dispatchEvent(new Event('submit', { bubbles: true, cancelable: true }))
      await Promise.resolve()
    })

    expect(dialog.getAttribute('aria-busy')).toBe('true')
    expect(dialog.querySelector<HTMLButtonElement>(':scope > header button')!.disabled).toBe(true)
    expect(dialog.querySelector<HTMLButtonElement>('footer button:first-child')!.disabled).toBe(true)

    await act(async () => {
      pending.resolve({ token: 'fake-token-for-test' })
      await pending.promise
    })
  })
})
