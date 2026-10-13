// @vitest-environment jsdom
import { act } from 'react'
import { createRoot, type Root } from 'react-dom/client'
import { afterEach, beforeEach, expect, it, vi } from 'vitest'
import { api } from '../api'
import { dialogs } from './AppDialogs'
import { BuildParametersForm } from './RunBuildDialog'

vi.mock('../api', () => ({ api: { validateProject: vi.fn(), triggerBuild: vi.fn(), listReleaseTargets: vi.fn() } }))
vi.mock('./AppDialogs', () => ({ dialogs: { confirm: vi.fn() } }))
let container: HTMLDivElement
let root: Root
beforeEach(() => {
  ;(globalThis as any).IS_REACT_ACT_ENVIRONMENT = true
  localStorage.setItem('locale', 'en')
  container = document.createElement('div')
  document.body.append(container)
  root = createRoot(container)
  vi.mocked(api.validateProject).mockResolvedValue({ valid: true, format: 'jenkinsfile', stages: 1, steps: 1, allow_long_running: false, parameters: [
    { name: 'RELEASE_CHANNEL', type: 'choice', choices: ['wechat', 'taptap'], default: 'wechat' },
    { name: 'RELEASE_ENVIRONMENT', type: 'choice', choices: ['test', 'release', 'audit'], default: 'test' },
    { name: 'RELEASE_ACTION', type: 'choice', choices: ['restart-version', 'publish-latest'], default: 'restart-version' },
    { name: 'RELEASE_VERSION', type: 'string' },
    { name: 'CONFIRM_PUBLISH_LATEST', type: 'boolean', default: false },
  ] })
  vi.mocked(api.triggerBuild).mockResolvedValue({ id: 99 })
  vi.mocked(api.listReleaseTargets).mockResolvedValue([
    { channel: 'wechat', environment: 'qa-17', latest: null, last_success: null },
    { channel: 'wechat', environment: '../release', latest: null, last_success: null },
    { channel: 'taptap', environment: 'taptap-audit', latest: null, last_success: null },
  ])
})
afterEach(() => {
  act(() => root.unmount())
  container.remove()
  localStorage.clear()
  vi.clearAllMocks()
})
async function render(parameters: Record<string, string>) {
  await act(async () => root.render(<BuildParametersForm project={{ id: 33, name: 'Release', default_branch: 'release/game' }} variant="page" initialParameters={parameters} onCancel={() => {}} onQueued={() => {}} />))
}
async function submit() {
  await act(async () => container.querySelector('form')!.dispatchEvent(new Event('submit', { bubbles: true, cancelable: true })))
}

it('cancels latest without sending a build and never exposes a confirmation checkbox', async () => {
  vi.mocked(dialogs.confirm).mockResolvedValue(false)
  await render({ RELEASE_ACTION: 'publish-latest', RELEASE_ENVIRONMENT: 'audit' })
  expect(container.querySelector('input[type="checkbox"]')).toBeNull()
  await submit()
  expect(dialogs.confirm).toHaveBeenCalledWith(expect.stringContaining('release/game to wechat / audit'), expect.any(Object))
  expect(api.triggerBuild).not.toHaveBeenCalled()
})

it('sends confirmation only after the operator accepts target and branch impact', async () => {
  vi.mocked(dialogs.confirm).mockResolvedValue(true)
  await render({ RELEASE_ACTION: 'publish-latest', RELEASE_CHANNEL: 'taptap', RELEASE_ENVIRONMENT: 'release' })
  await submit()
  expect(api.triggerBuild).toHaveBeenCalledWith(33, { branch: 'release/game', parameters: { RELEASE_CHANNEL: 'taptap', RELEASE_ENVIRONMENT: 'release', RELEASE_ACTION: 'publish-latest', CONFIRM_PUBLISH_LATEST: true } })
})

it('uses the selected historical version without a latest publication confirmation', async () => {
  await render({ RELEASE_ACTION: 'restart-version', RELEASE_CHANNEL: 'taptap', RELEASE_ENVIRONMENT: 'audit', RELEASE_VERSION: '0.1.234' })
  await submit()
  expect(dialogs.confirm).not.toHaveBeenCalled()
  expect(api.triggerBuild).toHaveBeenCalledWith(33, expect.objectContaining({ parameters: { RELEASE_CHANNEL: 'taptap', RELEASE_ENVIRONMENT: 'audit', RELEASE_ACTION: 'restart-version', RELEASE_VERSION: '0.1.234', CONFIRM_PUBLISH_LATEST: false } }))
})

it('confirms the shared physical service and all configured targets without inventing a channel', async () => {
  const metadata = await api.validateProject(33)
  metadata.parameters = metadata.parameters!.filter(parameter => parameter.name !== 'RELEASE_CHANNEL' && parameter.name !== 'RELEASE_ENVIRONMENT')
  metadata.parameters.push({ name: 'RELEASE_SHARED_SERVICE', type: 'string', default: 'sf-minigame-shared' })
  vi.mocked(api.validateProject).mockResolvedValue(metadata)
  vi.mocked(dialogs.confirm).mockResolvedValue(true)
  await render({ RELEASE_ACTION: 'publish-latest' })
  await submit()
  const confirmation = vi.mocked(dialogs.confirm).mock.calls[0][0]
  expect(confirmation).toContain('shared service sf-minigame-shared from branch release/game')
  expect(confirmation).toContain('every channel and environment')
  expect(confirmation).not.toContain('- / -')
  expect(api.triggerBuild).toHaveBeenCalledWith(33, { branch: 'release/game', parameters: { RELEASE_SHARED_SERVICE: 'sf-minigame-shared', RELEASE_ACTION: 'publish-latest', CONFIRM_PUBLISH_LATEST: true } })
})

it('offers channel history and default for string environments while accepting a new safe name', async () => {
  const metadata = await api.validateProject(33)
  metadata.parameters!.find(parameter => parameter.name === 'RELEASE_ENVIRONMENT')!.type = 'string'
  vi.mocked(api.validateProject).mockResolvedValue(metadata)
  await render({ RELEASE_VERSION: '0.1.234', RELEASE_ENVIRONMENT: 'internal-2' })
  expect([...container.querySelectorAll('datalist option')].map(option => option.getAttribute('value'))).toEqual(['test', 'qa-17'])
  await submit()
  expect(api.triggerBuild).toHaveBeenCalledWith(33, expect.objectContaining({ parameters: expect.objectContaining({ RELEASE_ENVIRONMENT: 'internal-2' }) }))
})

it('rejects invalid environments before showing a latest confirmation', async () => {
  const metadata = await api.validateProject(33)
  metadata.parameters!.find(parameter => parameter.name === 'RELEASE_ENVIRONMENT')!.type = 'string'
  vi.mocked(api.validateProject).mockResolvedValue(metadata)
  await render({ RELEASE_ACTION: 'publish-latest', RELEASE_ENVIRONMENT: '../release' })
  await submit()
  expect(container.textContent).toContain('Environment must start with a lowercase letter')
  expect(dialogs.confirm).not.toHaveBeenCalled()
  expect(api.triggerBuild).not.toHaveBeenCalled()
})
