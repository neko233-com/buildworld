// @vitest-environment jsdom

import { readFileSync } from 'node:fs'
import { resolve } from 'node:path'
import { act, type ReactNode } from 'react'
import { createRoot, type Root } from 'react-dom/client'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import en from './i18n/en.json'
import zhCN from './i18n/zh-CN.json'

vi.mock('motion/react', () => ({
  MotionConfig: ({ children, reducedMotion }: { children: ReactNode, reducedMotion?: string }) => (
    <div data-motion-policy={reducedMotion}>{children}</div>
  ),
}))

import { AppMotionBoundary, WORKSPACE_NAV_ITEMS } from './App'

const actEnvironment = globalThis as typeof globalThis & { IS_REACT_ACT_ENVIRONMENT: boolean }
const appSource = readFileSync(resolve(process.cwd(), 'src/App.tsx'), 'utf8')
const shellStyles = readFileSync(resolve(process.cwd(), 'src/jenkins-shell.css'), 'utf8')

describe('application motion accessibility', () => {
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

  it('lets the operating-system reduced-motion preference disable transforms', () => {
    act(() => root.render(<AppMotionBoundary><span>content</span></AppMotionBoundary>))

    expect(container.querySelector('[data-motion-policy="user"]')?.textContent).toBe('content')
  })

  it('keeps the workspace navigation names and requested order', () => {
    expect(WORKSPACE_NAV_ITEMS.map(item => item.href)).toEqual([
      '/',
      '/projects',
      '/build-queue',
      '/builds',
      '/vcs-roots',
    ])
    expect(WORKSPACE_NAV_ITEMS.map(item => item.labelKey)).toEqual([
      'nav.dashboard',
      'nav.projects',
      'nav.buildQueue',
      'nav.builds',
      'nav.vcsRoots',
    ])
    expect([zhCN.nav.buildQueue, zhCN.nav.builds, zhCN.nav.vcsRoots]).toEqual([
      '构建进行中',
      '构建历史',
      'VCS 仓库模板',
    ])
    expect([en.nav.buildQueue, en.nav.builds, en.nav.vcsRoots]).toEqual([
      'Builds in Progress',
      'Build History',
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
    expect(mastheadSource).toContain('<BuildWorldMark size={28} />')
    expect(mastheadSource).toContain('<span>BuildWorld</span>')
    expect(mastheadSource).toContain('className="topbar-actions"')
    expect(mastheadSource).toContain('to="/my-dashboard"')
    expect(mastheadSource).toContain('topbar-dashboard-link')
    expect(mastheadSource).toContain('to="/settings"')
    expect(mastheadSource).toContain('topbar-settings-link')
    expect(mastheadSource).toContain("<span>{t('nav.settings')}</span>")
    expect(mastheadSource.indexOf('to="/settings"')).toBeLessThan(mastheadSource.indexOf('className="account-menu"'))
    expect(mastheadSource.indexOf('to="/my-dashboard"')).toBeLessThan(mastheadSource.indexOf('className="account-menu"'))
    expect(mastheadSource).toContain('className="account-menu"')
    expect(mastheadSource).toContain('className="account-menu-trigger"')
    expect(mastheadSource).toContain('id="account-menu-popover"')
    expect(appSource).toContain('<Route path="/my-dashboard" element={<MyDashboard />} />')
    expect(appSource).toContain('<Route path="/templates" element={<Navigate to="/projects" replace />} />')
    expect(appSource).not.toContain("t('nav.templates')")
    expect(appSource).not.toContain("import('./pages/Templates')")
    expect(appSource).not.toContain('app-sidebar')
    expect(shellStyles).toMatch(/\.app-topbar \.topbar-actions \.topbar-settings-link\s*\{[^}]*display:\s*inline-flex[^}]*height:\s*38px/s)
    expect(shellStyles).not.toMatch(/\.topbar-settings-link[^}]*display:\s*none/s)
  })
})
