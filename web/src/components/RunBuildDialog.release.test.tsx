// @vitest-environment jsdom
import { act } from 'react'
import { createRoot, type Root } from 'react-dom/client'
import { afterEach, beforeEach, expect, it, vi } from 'vitest'
import { api } from '../api'
import { dialogs } from './AppDialogs'
import { BuildParametersForm } from './RunBuildDialog'

vi.mock('../api', () => ({ api: { validateProject: vi.fn(), triggerBuild: vi.fn(), listReleaseTargets: vi.fn(), listReleaseRecords: vi.fn() } }))
vi.mock('./AppDialogs', () => ({ dialogs: { confirm: vi.fn() } }))
let container: HTMLDivElement
let root: Root
beforeEach(() => {
  ;(globalThis as any).IS_REACT_ACT_ENVIRONMENT = true
  localStorage.setItem('locale', 'en')
  container = document.createElement('div')
  document.body.append(container)
  root = createRoot(container)
  vi.mocked(api.listReleaseRecords).mockResolvedValue([])
  vi.mocked(api.validateProject).mockResolvedValue({ valid: true, format: 'jenkinsfile', stages: 1, steps: 1, allow_long_running: false, parameters: [
    { name: 'RELEASE_CHANNEL', type: 'choice', choices: ['wechat', 'taptap'], default: 'wechat' },
    { name: 'RELEASE_ENVIRONMENT', type: 'choice', choices: ['test', 'release', 'audit'], default: 'test' },
    { name: 'RELEASE_ACTION', type: 'choice', choices: ['restart-version', 'publish-latest'], default: 'restart-version' },
    { name: 'RELEASE_VERSION', type: 'string' },
    { name: 'CONFIRM_PUBLISH_LATEST', type: 'boolean', default: false },
  ], release_targets: { schemaVersion: 1, targets: [
    { channel: 'wechat', environment: 'test', channelLabel: '微信' },
    { channel: 'wechat', environment: 'release', channelLabel: '微信' },
    { channel: 'wechat', environment: 'audit', channelLabel: '微信' },
    { channel: 'wechat', environment: 'internal-2', channelLabel: '微信' },
    { channel: 'taptap', environment: 'test', channelLabel: 'TapTap' },
    { channel: 'taptap', environment: 'release', channelLabel: 'TapTap' },
    { channel: 'taptap', environment: 'audit', channelLabel: 'TapTap' },
    { channel: 'third-store', environment: 'preview-17', channelLabel: '第三渠道' },
  ] } })
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

it('selects a published branch with its frozen config and runs tooling from main', async () => {
  const metadata = await api.validateProject(33)
  metadata.parameters!.push({ name: 'RELEASE_BRANCH', type: 'string' })
  vi.mocked(api.validateProject).mockResolvedValue(metadata)
  const branch = 'release/wechat/release/game-server/0.1.234'
  vi.mocked(api.listReleaseRecords).mockResolvedValue([{ channel: 'wechat', environment: 'release', action: 'publish-latest', component: 'game-server', branch, version: '0.1.234', configVersion: '0.0.731', configCommit: 'a'.repeat(40) }])
  await render({ RELEASE_CHANNEL: 'wechat', RELEASE_ENVIRONMENT: 'release' })
  const selector = container.querySelectorAll<HTMLInputElement>('input[role="combobox"]')[2]
  await act(async () => selector.focus())
  expect(container.textContent).toContain('0.0.731')
  await act(async () => (container.querySelector('[role="option"]') as HTMLButtonElement).click())
  await submit()
  expect(api.triggerBuild).toHaveBeenCalledWith(33, expect.objectContaining({ branch: 'main', parameters: expect.objectContaining({ RELEASE_BRANCH: branch, RELEASE_VERSION: '0.1.234', RELEASE_CHANNEL: 'wechat', RELEASE_ENVIRONMENT: 'release' }) }))
  expect(dialogs.confirm).not.toHaveBeenCalled()
})

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
  metadata.release_targets = null
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

it('offers only configured linked environments for string declarations, without using history as authority', async () => {
  const metadata = await api.validateProject(33)
  metadata.parameters!.find(parameter => parameter.name === 'RELEASE_ENVIRONMENT')!.type = 'string'
  vi.mocked(api.validateProject).mockResolvedValue(metadata)
  await render({ RELEASE_VERSION: '0.1.234', RELEASE_ENVIRONMENT: 'internal-2' })
  const environment = container.querySelectorAll<HTMLInputElement>('input[role="combobox"]')[1]
  await act(async () => environment.focus())
  expect([...container.querySelectorAll('[role="option"]')].map(option => option.textContent)).toEqual(['test', 'release', 'audit', 'internal-2'])
  expect(api.listReleaseTargets).not.toHaveBeenCalled()
  await submit()
  expect(api.triggerBuild).toHaveBeenCalledWith(33, expect.objectContaining({ parameters: expect.objectContaining({ RELEASE_ENVIRONMENT: 'internal-2' }) }))
})

it('rejects invalid environments before showing a latest confirmation', async () => {
  const metadata = await api.validateProject(33)
  metadata.parameters!.find(parameter => parameter.name === 'RELEASE_ENVIRONMENT')!.type = 'string'
  vi.mocked(api.validateProject).mockResolvedValue(metadata)
  await render({ RELEASE_ACTION: 'publish-latest', RELEASE_ENVIRONMENT: '../release' })
  await submit()
  expect(container.textContent).toContain('is not configured for deployment or restart')
  expect(dialogs.confirm).not.toHaveBeenCalled()
  expect(api.triggerBuild).not.toHaveBeenCalled()
})

it('searches an arbitrary third channel and updates only its configured environment using the keyboard', async () => {
  await render({ RELEASE_ACTION: 'publish-latest' })
  const channel = container.querySelector<HTMLInputElement>('input[role="combobox"]')!
  await act(async () => channel.focus())
  await act(async () => {
    Object.getOwnPropertyDescriptor(HTMLInputElement.prototype, 'value')!.set!.call(channel, '第三')
    channel.dispatchEvent(new Event('input', { bubbles: true }))
  })
  expect(container.querySelectorAll('[role="option"]')).toHaveLength(1)
  const enter = new KeyboardEvent('keydown', { key: 'Enter', bubbles: true, cancelable: true })
  await act(async () => channel.dispatchEvent(enter))
  expect(enter.defaultPrevented).toBe(true)
  expect(channel.value).toBe('第三渠道')
  const environment = container.querySelectorAll<HTMLInputElement>('input[role="combobox"]')[1]
  expect(environment.value).toBe('preview-17')
  await act(async () => environment.focus())
  expect([...container.querySelectorAll('[role="option"]')].map(option => option.textContent)).toEqual(['preview-17'])
  expect(api.triggerBuild).not.toHaveBeenCalled()
  vi.mocked(dialogs.confirm).mockResolvedValue(true)
  await submit()
  expect(api.triggerBuild).toHaveBeenCalledWith(33, expect.objectContaining({ parameters: expect.objectContaining({ RELEASE_CHANNEL: 'third-store', RELEASE_ENVIRONMENT: 'preview-17', CONFIRM_PUBLISH_LATEST: true }) }))
})

it('preserves the exact historical target and version, then clears the version when the operator changes its target', async () => {
  await render({ RELEASE_CHANNEL: 'taptap', RELEASE_ENVIRONMENT: 'audit', RELEASE_VERSION: '0.1.234' })
  const controls = container.querySelectorAll<HTMLInputElement>('input[role="combobox"]')
  expect(controls[0].value).toBe('TapTap')
  expect(controls[1].value).toBe('audit')
  const version = [...container.querySelectorAll<HTMLInputElement>('input')].find(input => input.value === '0.1.234')!
  expect(version).toBeTruthy()
  await act(async () => controls[0].focus())
  await act(async () => (container.querySelectorAll('[role="option"]')[0] as HTMLButtonElement).click())
  expect(controls[1].value).toBe('test')
  expect(version.value).toBe('')
  expect(container.textContent).toContain('Enter the historical version for this target again')
  await submit()
  expect(api.triggerBuild).not.toHaveBeenCalled()
})

it('does not replace or submit a removed historical target and fails closed without configured metadata', async () => {
  await render({ RELEASE_CHANNEL: 'taptap', RELEASE_ENVIRONMENT: 'removed-env', RELEASE_VERSION: '0.1.234' })
  expect(container.querySelector<HTMLButtonElement>('button[type="submit"]')!.disabled).toBe(true)
  expect(container.textContent).toContain('taptap / removed-env is not configured')
  await submit()
  expect(api.triggerBuild).not.toHaveBeenCalled()
  const metadata = await api.validateProject(33)
  metadata.release_targets = null
  vi.mocked(api.validateProject).mockResolvedValue(metadata)
  await act(async () => root.render(<BuildParametersForm project={{ id: 34, name: 'Other', default_branch: 'main' }} initialParameters={{ RELEASE_VERSION: '0.1.234' }} onCancel={() => {}} onQueued={() => {}} />))
  expect(container.textContent).toContain('RELEASE_TARGETS_JSON')
  expect(container.querySelector<HTMLButtonElement>('button[type="submit"]')!.disabled).toBe(true)
})

it('loads a new historical shortcut on the same project without replacing it with the previous selection', async () => {
  await render({ RELEASE_CHANNEL: 'wechat', RELEASE_ENVIRONMENT: 'test', RELEASE_VERSION: '0.1.132' })
  await render({ RELEASE_CHANNEL: 'taptap', RELEASE_ENVIRONMENT: 'audit', RELEASE_VERSION: '0.1.234' })
  const controls = container.querySelectorAll<HTMLInputElement>('input[role="combobox"]')
  expect(controls[0].value).toBe('TapTap')
  expect(controls[1].value).toBe('audit')
  await submit()
  expect(api.triggerBuild).toHaveBeenCalledWith(33, expect.objectContaining({ parameters: expect.objectContaining({ RELEASE_CHANNEL: 'taptap', RELEASE_ENVIRONMENT: 'audit', RELEASE_VERSION: '0.1.234' }) }))
})
