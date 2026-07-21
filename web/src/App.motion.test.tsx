// @vitest-environment jsdom

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
      '/templates',
    ])
    expect(WORKSPACE_NAV_ITEMS.map(item => item.labelKey)).toEqual([
      'nav.dashboard',
      'nav.projects',
      'nav.buildQueue',
      'nav.builds',
      'nav.vcsRoots',
      'nav.templates',
    ])
    expect([zhCN.nav.buildQueue, zhCN.nav.builds, zhCN.nav.vcsRoots, zhCN.nav.templates]).toEqual([
      '构建进行中',
      '构建历史',
      'VCS 仓库模板',
      '构建模板',
    ])
    expect([en.nav.buildQueue, en.nav.builds, en.nav.vcsRoots, en.nav.templates]).toEqual([
      'Builds in Progress',
      'Build History',
      'VCS Repository Templates',
      'Build Templates',
    ])
    expect('deployments' in en.nav).toBe(false)
    expect('deployments' in en).toBe(false)
    expect('gitHooks' in en).toBe(false)
    expect('deployments' in zhCN.nav).toBe(false)
    expect('deployments' in zhCN).toBe(false)
    expect('gitHooks' in zhCN).toBe(false)
  })
})
