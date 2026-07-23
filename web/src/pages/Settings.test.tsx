// @vitest-environment jsdom

import { act, useEffect, type ReactNode } from 'react'
import { createRoot, type Root } from 'react-dom/client'
import { MemoryRouter, useLocation } from 'react-router-dom'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { api } from '../api'
import { useI18n } from '../i18n'
import Settings from './Settings'

vi.mock('../api', () => ({
  api: {
    getGlobalSettings: vi.fn(),
    getServerMetrics: vi.fn(),
    listAgents: vi.fn(),
    listPackageProxies: vi.fn(),
    listPortabilityCapabilities: vi.fn(),
    updateGlobalSettings: vi.fn(),
    generateAgentToken: vi.fn(),
    applyPackageProxies: vi.fn(),
  },
}))

vi.mock('../components/AppDialogs', () => ({ dialogs: { confirm: vi.fn(), notify: vi.fn() } }))

function ChineseLocale({ children }: { children: ReactNode }) {
  const { locale, changeLocale } = useI18n()
  useEffect(() => {
    if (locale !== 'zh-CN') changeLocale('zh-CN')
  }, [changeLocale, locale])
  return locale === 'zh-CN' ? children : null
}

function LocationProbe() {
  const location = useLocation()
  return <output aria-label="location">{location.pathname}{location.search}</output>
}

describe('Settings Jenkins directory', () => {
  let container: HTMLDivElement
  let breadcrumbHost: HTMLDivElement
  let root: Root

  beforeEach(() => {
    ;(globalThis as any).IS_REACT_ACT_ENVIRONMENT = true
    vi.mocked(api.getGlobalSettings).mockResolvedValue({})
    vi.mocked(api.getServerMetrics).mockResolvedValue({ system: { uptime_seconds: 7200, go_version: 'go1.26' } })
    vi.mocked(api.listAgents).mockResolvedValue([])
    vi.mocked(api.listPackageProxies).mockResolvedValue([])
    vi.mocked(api.listPortabilityCapabilities).mockResolvedValue([
      { key: 'settings', version: 1, sensitive: false },
      { key: 'build_templates', version: 1, sensitive: false },
      { key: 'templates', version: 1, sensitive: false },
    ])
    container = document.createElement('div')
    breadcrumbHost = document.createElement('div')
    breadcrumbHost.id = 'jenkins-header-breadcrumbs'
    document.body.append(breadcrumbHost, container)
    root = createRoot(container)
  })

  afterEach(() => {
    act(() => root.unmount())
    container.remove()
    breadcrumbHost.remove()
    vi.clearAllMocks()
    localStorage.clear()
    ;(globalThis as any).IS_REACT_ACT_ENVIRONMENT = false
  })

  async function renderSettings(path = '/settings') {
    await act(async () => {
      root.render(<MemoryRouter initialEntries={[path]}><ChineseLocale><Settings /><LocationProbe /></ChineseLocale></MemoryRouter>)
      await Promise.resolve()
      await Promise.resolve()
      await Promise.resolve()
    })
  }

  function settingButton(label: string) {
    const button = Array.from(container.querySelectorAll<HTMLButtonElement>('.settings-directory-link'))
      .find(item => item.textContent?.includes(label))
    expect(button).toBeDefined()
    return button as HTMLButtonElement
  }

  it('renders the full-width Manage Jenkins directory without a build-template main entry', async () => {
    await renderSettings()

    expect(container.querySelector('.settings-center')).not.toBeNull()
    expect(container.querySelector('.settings-nav')).toBeNull()
    expect(container.querySelector('.settings-directory')?.tagName).toBe('SECTION')
    expect(Array.from(container.querySelectorAll('.settings-directory-group > h2')).map(item => item.textContent)).toEqual([
      '系统配置', '安全', '状态信息', '工具和数据',
    ])
    expect(container.querySelectorAll('.settings-directory-link')).toHaveLength(7)
    expect(vi.mocked(api.listAgents)).not.toHaveBeenCalled()
    expect(container.querySelector('.settings-directory')?.textContent).not.toContain('Worker')
    expect(container.querySelector('.settings-directory')?.textContent).not.toContain('构建模板')
    expect(container.querySelector('.settings-directory')?.textContent).not.toContain('外观')
    expect(settingButton('备份与恢复').textContent).toContain('兼容数据')
    expect(breadcrumbHost.textContent).toContain('系统设置')
    expect(container.querySelector('.settings-save-area')).toBeNull()
  })

  it('filters settings and opens existing detail content through query-addressable links', async () => {
    await renderSettings()

    const search = container.querySelector<HTMLInputElement>('input[aria-label="搜索设置"]')
    expect(search).not.toBeNull()
    await act(async () => {
      const setter = Object.getOwnPropertyDescriptor(HTMLInputElement.prototype, 'value')?.set
      setter?.call(search, '包代理')
      search?.dispatchEvent(new Event('input', { bubbles: true }))
    })
    expect(container.querySelectorAll('.settings-directory-link')).toHaveLength(1)
    expect(settingButton('包代理')).not.toBeNull()

    await act(async () => settingButton('包代理').click())
    expect(container.querySelector('output[aria-label="location"]')?.textContent).toBe('/settings?section=proxies')
    expect(container.querySelector('.package-proxies-pane')).not.toBeNull()

    await act(async () => container.querySelector<HTMLButtonElement>('.settings-back-link')?.click())
    await act(async () => {
      const restoredSearch = container.querySelector<HTMLInputElement>('input[aria-label="搜索设置"]')
      const setter = Object.getOwnPropertyDescriptor(HTMLInputElement.prototype, 'value')?.set
      setter?.call(restoredSearch, '')
      restoredSearch?.dispatchEvent(new Event('input', { bubbles: true }))
    })
    await act(async () => settingButton('系统概览').click())
    expect(container.querySelector('output[aria-label="location"]')?.textContent).toBe('/settings?section=status')
    expect(container.querySelector('.settings-health-grid')?.textContent).toContain('运行正常')
  })

  it('redirects the removed appearance route and keeps compatibility-data tools reachable', async () => {
    await renderSettings('/settings?section=appearance')

    expect(container.querySelector('.skin-card-grid')).toBeNull()
    expect(container.querySelector('output[aria-label="location"]')?.textContent).toBe('/settings')
    expect(container.querySelector('.settings-directory')).not.toBeNull()

    await act(async () => settingButton('备份与恢复').click())
    expect(container.querySelector('.portability-pane')).not.toBeNull()
    expect(container.querySelector('.settings-compatibility-label')?.textContent).toBe('兼容数据')
    expect(container.querySelectorAll('.portability-section-grid > button')).toHaveLength(1)
    expect(container.querySelector('.portability-pane')?.textContent).not.toContain('构建模板')
    expect(container.querySelector('.portability-pane')?.textContent).not.toContain('Build Templates')
    expect(container.textContent).toContain('导出配置')
    expect(container.textContent).toContain('导入配置')
  })

  it('keeps distributed Worker settings dormant and generates a builtin pipeline', async () => {
    await renderSettings('/settings?section=agents')

    expect(container.querySelector('output[aria-label="location"]')?.textContent).toBe('/settings')
    expect(container.querySelector('.agent-enrollment-section')).toBeNull()
    expect(vi.mocked(api.listAgents)).not.toHaveBeenCalled()

    await act(async () => settingButton('Go / TypeScript 验证').click())
    const pipeline = Array.from(container.querySelectorAll('.settings-code-panel'))
      .find(panel => panel.textContent?.includes('pipeline.buildworld.ts'))
    expect(pipeline?.textContent).toContain('definePipeline')
    expect(pipeline?.textContent).not.toContain('agentRequirements')
  })

  it('keeps automatic system updates visibly disabled by default', async () => {
    await renderSettings('/settings?section=runtime')

    const updateSwitch = Array.from(container.querySelectorAll<HTMLButtonElement>('[role="switch"]'))
      .find(item => item.getAttribute('aria-labelledby') &&
        document.getElementById(item.getAttribute('aria-labelledby') || '')?.textContent === '允许自动更新请求')
    expect(updateSwitch).toBeDefined()
    expect(updateSwitch?.getAttribute('aria-checked')).toBe('false')
    expect(container.textContent).toContain('人工更新')
  })
})
