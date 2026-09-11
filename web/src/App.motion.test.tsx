// @vitest-environment jsdom

import { readFileSync } from 'node:fs'
import { resolve } from 'node:path'
import { act, type ReactNode } from 'react'
import { createRoot, type Root } from 'react-dom/client'
import { MemoryRouter, Route, Routes, useNavigate } from 'react-router-dom'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import en from './i18n/en.json'
import zhCN from './i18n/zh-CN.json'
import { api } from './api'

vi.mock('motion/react', () => ({
  MotionConfig: ({ children, reducedMotion }: { children: ReactNode, reducedMotion?: string }) => (
    <div data-motion-policy={reducedMotion}>{children}</div>
  ),
}))

vi.mock('./components/InAppNotifications', () => ({
  default: () => <button type="button" className="notification-bell">Notifications</button>,
}))

import { AppMotionBoundary, focusRouteContent, Layout } from './App'

const actEnvironment = globalThis as typeof globalThis & { IS_REACT_ACT_ENVIRONMENT: boolean }
const appSource = readFileSync(resolve(process.cwd(), 'src/App.tsx'), 'utf8')
const shellStyles = readFileSync(resolve(process.cwd(), 'src/jenkins-shell.css'), 'utf8')

function ShellFixture() {
  const navigate = useNavigate()
  return <>
    <button type="button" className="route-switch" onClick={() => navigate('/second')}>Next route</button>
    <Routes>
      <Route element={<Layout />}>
        <Route path="/first" element={<section><h1>First page</h1></section>} />
        <Route path="/second" element={<section><h1>Second page</h1></section>} />
        <Route path="/empty" element={<section><p>Loading data</p></section>} />
      </Route>
    </Routes>
  </>
}

describe('application motion accessibility', () => {
  let container: HTMLDivElement
  let root: Root

  beforeEach(() => {
    actEnvironment.IS_REACT_ACT_ENVIRONMENT = true
    const payload = btoa(JSON.stringify({ role: 'admin' }))
    localStorage.setItem('token', `test.${payload}.signature`)
    container = document.createElement('div')
    document.body.appendChild(container)
    root = createRoot(container)
    vi.spyOn(window, 'requestAnimationFrame').mockImplementation(callback => {
      callback(0)
      return 1
    })
    vi.spyOn(window, 'cancelAnimationFrame').mockImplementation(() => undefined)
  })

  afterEach(() => {
    act(() => root.unmount())
    container.remove()
    localStorage.clear()
    actEnvironment.IS_REACT_ACT_ENVIRONMENT = false
    vi.restoreAllMocks()
  })

  it('lets the operating-system reduced-motion preference disable transforms', () => {
    act(() => root.render(<AppMotionBoundary><span>content</span></AppMotionBoundary>))

    expect(container.querySelector('[data-motion-policy="user"]')?.textContent).toBe('content')
  })

  it('keeps current workspace navigation translations', () => {
    expect([zhCN.nav.buildQueue, zhCN.nav.builds, zhCN.nav.templates, zhCN.nav.vcsRoots]).toEqual([
      '构建进行中',
      '构建历史',
      '构建模板',
      'VCS 仓库模板',
    ])
    expect([en.nav.buildQueue, en.nav.builds, en.nav.templates, en.nav.vcsRoots]).toEqual([
      'Builds in Progress',
      'Build History',
      'Build Templates',
      'VCS Repository Templates',
    ])
    expect('deployments' in en.nav).toBe(false)
    expect('deployments' in en).toBe(false)
    expect('gitHooks' in en).toBe(false)
    expect('deployments' in zhCN.nav).toBe(false)
    expect('deployments' in zhCN).toBe(false)
    expect('gitHooks' in zhCN).toBe(false)
  })

  it('keeps the Jenkins-compatible masthead controls without a global sidebar', () => {
    const mastheadStart = appSource.indexOf('<header className="app-topbar">')
    const mastheadEnd = appSource.indexOf('</header>', mastheadStart)
    const mastheadSource = appSource.slice(mastheadStart, mastheadEnd)

    expect(mastheadStart).toBeGreaterThan(-1)
    expect(mastheadEnd).toBeGreaterThan(mastheadStart)
    expect(mastheadSource).toContain('className="app-brand"')
    expect(mastheadSource).toContain('<BuildWorldMark size={36} />')
    expect(mastheadSource).toContain('<span>BuildWorld</span>')
    expect(mastheadSource).toContain('id="jenkins-header-breadcrumbs"')
    expect(mastheadSource).toContain('className="topbar-actions"')
    expect(mastheadSource).toContain('to="/my-dashboard"')
    expect(mastheadSource).toContain('account-dashboard-link')
    expect(mastheadSource).not.toContain('topbar-dashboard-link')
    expect(mastheadSource).toContain('to="/settings"')
    expect(mastheadSource).toContain('topbar-settings-link')
    expect(mastheadSource).toContain("<span className=\"jenkins-visually-hidden\">{t('nav.settings')}</span>")
    expect(mastheadSource.indexOf('to="/settings"')).toBeLessThan(mastheadSource.indexOf('className="account-menu"'))
    expect(mastheadSource.indexOf('to="/my-dashboard"')).toBeGreaterThan(mastheadSource.indexOf('className="account-menu"'))
    expect(mastheadSource).toContain('className="account-menu"')
    expect(mastheadSource).toContain('className="account-menu-trigger"')
    expect(mastheadSource).toContain('id="account-menu-popover"')
    expect(mastheadSource).toContain('hidden={!accountOpen}')
    expect(mastheadSource).toContain('<InAppNotifications menu menuOpen={accountOpen} />')
    expect(mastheadSource.indexOf('<InAppNotifications')).toBeGreaterThan(mastheadSource.indexOf('id="account-menu-popover"'))
    expect(mastheadSource.indexOf('<InAppNotifications')).toBeGreaterThan(mastheadSource.indexOf('className="account-menu"'))
    expect(appSource).toContain('<Route path="/my-dashboard" element={<MyDashboard />} />')
    expect(appSource).toContain('<Route path="/templates" element={<Templates />} />')
    expect(appSource).toContain("t('nav.templates')")
    expect(appSource).toContain("const Templates = lazy(() => import('./pages/Templates'))")
    expect(appSource).toContain('...(DISTRIBUTED_WORKERS_ENABLED ?')
    expect(appSource).toContain('<Route path="/agents" element={<Agents />} />')
    expect(appSource).not.toContain('app-sidebar')
    expect(shellStyles).toMatch(/\.app-topbar \.topbar-actions \.topbar-settings-link\s*\{[^}]*display:\s*inline-grid[^}]*width:\s*38px[^}]*height:\s*38px/s)
    expect(shellStyles).not.toMatch(/\.topbar-settings-link[^}]*display:\s*none/s)
  })

  it('lets an administrator manually check for an official update from the masthead', async () => {
    const check = vi.spyOn(api, 'checkSystemUpdate').mockResolvedValue({
      current_version: '1.0.0', latest_version: '1.1.0', update_available: true,
      platform: 'darwin/arm64', asset_name: 'buildworld-darwin-arm64.tar.gz', asset_size: 12,
      manual_only: true, administrator_required: true,
    })
    act(() => root.render(<MemoryRouter initialEntries={['/first']}><ShellFixture /></MemoryRouter>))

    const trigger = container.querySelector<HTMLButtonElement>('.system-update-trigger')!
    expect(check).not.toHaveBeenCalled()
    await act(async () => trigger.click())

    expect(check).toHaveBeenCalledOnce()
    expect(container.querySelector('.system-update-popover')?.textContent).toContain('Update available: v1.1.0')
    expect(container.querySelector('.system-update-popover')?.textContent).toContain('Manual update · administrator only')
  })

  it('moves focus to the new page heading after SPA route navigation', () => {
    act(() => root.render(<MemoryRouter initialEntries={['/first']}><ShellFixture /></MemoryRouter>))

    expect(document.activeElement?.textContent).toBe('First page')
    const routeSwitch = container.querySelector<HTMLButtonElement>('.route-switch')!
    routeSwitch.focus()
    act(() => routeSwitch.click())

    const secondHeading = container.querySelector<HTMLHeadingElement>('h1')!
    expect(secondHeading.textContent).toBe('Second page')
    expect(secondHeading.tabIndex).toBe(-1)
    expect(secondHeading.style.outline).toBe('none')
    expect(document.activeElement).toBe(secondHeading)
  })

  it('does not steal focus from an open modal dialog', () => {
    const main = document.createElement('main')
    main.tabIndex = -1
    main.innerHTML = '<h1>Changed route</h1>'
    const modal = document.createElement('section')
    modal.setAttribute('role', 'dialog')
    modal.setAttribute('aria-modal', 'true')
    const modalButton = document.createElement('button')
    modal.appendChild(modalButton)
    document.body.append(main, modal)
    modalButton.focus()

    expect(focusRouteContent(main)).toBe('blocked')
    expect(document.activeElement).toBe(modalButton)

    main.remove()
    modal.remove()
  })

  it('uses an outline-free main fallback and promotes focus when an async heading appears', async () => {
    await act(async () => root.render(<MemoryRouter initialEntries={['/empty']}><ShellFixture /></MemoryRouter>))
    const main = container.querySelector<HTMLElement>('.app-main')!

    expect(document.activeElement).toBe(main)
    expect(main.style.outline).toBe('none')

    const heading = document.createElement('h1')
    heading.textContent = 'Loaded page'
    await act(async () => {
      main.querySelector('section')?.appendChild(heading)
      await Promise.resolve()
    })
    expect(document.activeElement).toBe(heading)
  })

  it('supports complete keyboard navigation and restores account-menu focus', () => {
    act(() => root.render(<MemoryRouter initialEntries={['/first']}><ShellFixture /></MemoryRouter>))
    const trigger = container.querySelector<HTMLButtonElement>('.account-menu-trigger')!
    const popover = container.querySelector<HTMLDivElement>('#account-menu-popover')!

    act(() => trigger.click())
    const language = popover.querySelector<HTMLSelectElement>('select')!
    const dashboard = popover.querySelector<HTMLAnchorElement>('.account-dashboard-link')!
    const logout = popover.querySelector<HTMLButtonElement>('.account-logout')!
    expect(popover.hidden).toBe(false)
    expect(document.activeElement).toBe(language)

    act(() => language.dispatchEvent(new KeyboardEvent('keydown', { key: 'ArrowDown', bubbles: true })))
    expect(document.activeElement).toBe(dashboard)
    act(() => dashboard.dispatchEvent(new KeyboardEvent('keydown', { key: 'End', bubbles: true })))
    expect(document.activeElement).toBe(logout)
    act(() => logout.dispatchEvent(new KeyboardEvent('keydown', { key: 'Home', bubbles: true })))
    expect(document.activeElement).toBe(language)
    act(() => language.dispatchEvent(new KeyboardEvent('keydown', { key: 'ArrowUp', bubbles: true })))
    expect(document.activeElement).toBe(logout)
    act(() => logout.dispatchEvent(new KeyboardEvent('keydown', { key: 'Escape', bubbles: true })))
    expect(popover.hidden).toBe(true)
    expect(document.activeElement).toBe(trigger)

    act(() => trigger.click())
    const outside = document.createElement('button')
    document.body.appendChild(outside)
    act(() => outside.dispatchEvent(new Event('pointerdown', { bubbles: true })))
    expect(popover.hidden).toBe(true)
    expect(document.activeElement).toBe(trigger)
    outside.remove()
  })

  it('keeps the standalone log viewer outside the focus-managed application layout', () => {
    const logRoute = appSource.indexOf('<Route path="/builds/:id/logs"')
    const layoutRoute = appSource.indexOf('<Route element={<Layout />}>')

    expect(logRoute).toBeGreaterThan(-1)
    expect(logRoute).toBeLessThan(layoutRoute)
  })
})
