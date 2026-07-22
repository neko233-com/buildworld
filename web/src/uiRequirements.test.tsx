// @vitest-environment jsdom

import { readFileSync } from 'node:fs'
import { resolve } from 'node:path'
import { act } from 'react'
import { createRoot } from 'react-dom/client'
import { MemoryRouter } from 'react-router-dom'
import { afterEach, describe, expect, it } from 'vitest'
import { BuildWorldMark } from './components/BuildWorldMark'
import JenkinsPageShell from './components/JenkinsPageShell'
import { PROJECT_GROUP_COLORS } from './lib/projectGroups'

const indexStyles = readFileSync(resolve(process.cwd(), 'src/index.css'), 'utf8')
const jenkinsPageStyles = readFileSync(resolve(process.cwd(), 'src/jenkins-pages.css'), 'utf8')
const jenkinsShellStyles = readFileSync(resolve(process.cwd(), 'src/jenkins-shell.css'), 'utf8')
const settingsStyles = readFileSync(resolve(process.cwd(), 'src/settings.css'), 'utf8')
const styles = `${indexStyles}\n${jenkinsPageStyles}\n${jenkinsShellStyles}`
const documentTemplate = readFileSync(resolve(process.cwd(), 'index.html'), 'utf8')
const entrypoint = readFileSync(resolve(process.cwd(), 'src/main.tsx'), 'utf8')

function installStyles(value: string) {
  const style = document.createElement('style')
  style.textContent = value.replace(/^@import[^;]+;\r?\n?/gm, '')
  document.body.appendChild(style)
}

function luminance(color: string) {
  const channels = color.match(/[a-f\d]{2}/gi)?.map(value => {
    const channel = Number.parseInt(value, 16) / 255
    return channel <= 0.04045 ? channel / 12.92 : ((channel + 0.055) / 1.055) ** 2.4
  })
  if (!channels || channels.length !== 3) throw new Error(`invalid color ${color}`)
  return 0.2126 * channels[0] + 0.7152 * channels[1] + 0.0722 * channels[2]
}

function contrast(left: string, right: string) {
  const [bright, dark] = [luminance(left), luminance(right)].sort((a, b) => b - a)
  return (bright + 0.05) / (dark + 0.05)
}

describe('requested UI contracts', () => {
  afterEach(() => {
    document.body.replaceChildren()
    document.documentElement.removeAttribute('data-skin')
  })

  it('keeps the Jenkins-compatible masthead white and readable', () => {
    expect(styles).toMatch(/\.app-shell\s*\{[^}]*display:\s*block/)
    expect(styles).toMatch(/\.app-topbar,\s*\[data-skin="jenkins"\] \.app-topbar\s*\{[^}]*height:\s*66px[^}]*background:\s*#fefefe/)
    expect(styles).toMatch(/\.app-brand,\s*\[data-skin="jenkins"\] \.app-brand\s*\{[^}]*height:\s*38px[^}]*color:\s*#0d1117/)
    expect(styles).not.toContain('.topbar-dashboard-link,')
    expect(styles).toMatch(/\.account-menu-popover\s*\{[^}]*right:\s*0[^}]*background:\s*#ffffff/)
    expect(contrast('#0d1117', '#fefefe')).toBeGreaterThan(7)

    document.documentElement.dataset.skin = 'jenkins'
    installStyles(styles)
    const topbar = document.createElement('header')
    topbar.className = 'app-topbar'
    const brand = document.createElement('a')
    brand.className = 'app-brand'
    topbar.appendChild(brand)
    document.body.appendChild(topbar)

    expect(getComputedStyle(topbar).height).toBe('66px')
    expect(getComputedStyle(brand).height).toBe('38px')
  })

  it('uses Jenkins as the only, pre-rendered interface skin', () => {
    expect(documentTemplate).toMatch(/<html\s+lang="zh-CN"\s+data-skin="jenkins">/)
    expect(documentTemplate).toContain('<meta name="theme-color" content="#ffffff" />')
    expect(entrypoint).not.toContain('applySkin')
    expect(entrypoint).not.toContain('buildworld.skin')
    expect(styles).not.toContain('.skin-card')
    expect(styles).not.toContain('data-preview="buildworld"')
  })

  it('applies the final Jenkins page cascade to the desktop rail and viewport height', () => {
    document.documentElement.dataset.skin = 'jenkins'
    installStyles(styles)
    const home = document.createElement('div')
    home.className = 'jenkins-home'
    const rail = document.createElement('aside')
    rail.className = 'jenkins-rail-root'
    const contextPage = document.createElement('section')
    contextPage.className = 'jenkins-context-page'
    home.appendChild(rail)
    document.body.append(home, contextPage)

    expect(getComputedStyle(home).gridTemplateColumns).toBe('340px minmax(0, 1fr)')
    expect(getComputedStyle(home).minHeight).toBe('calc(100vh - 66px)')
    expect(getComputedStyle(rail).width).toBe('340px')
    expect(getComputedStyle(contextPage).minHeight).toBe('calc(100vh - 66px)')
    expect(styles).toMatch(/\.jenkins-rail-links\s*\{[^}]*display:\s*grid/)
    expect(styles).toMatch(/\.jenkins-job-table-wrap\s*\{[^}]*overflow-x:\s*auto[^}]*background:\s*#ffffff/)
    expect(styles).toMatch(/\.jenkins-job-table\s*\{[^}]*min-width:\s*920px[^}]*table-layout:\s*fixed/)
    expect(styles).toMatch(/\.jenkins-job-table td\s*\{[^}]*height:\s*43px/)
  })

  it('collapses the Jenkins home layout cleanly at tablet and phone widths', () => {
    const tabletStart = indexStyles.lastIndexOf('@media (max-width: 980px)')
    const mobileStart = indexStyles.lastIndexOf('@media (max-width: 760px)')
    const phoneStart = indexStyles.lastIndexOf('@media (max-width: 520px)')
    const tabletStyles = indexStyles.slice(tabletStart, mobileStart)
    const mobileStyles = indexStyles.slice(mobileStart, phoneStart)

    expect(tabletStart).toBeGreaterThan(-1)
    expect(mobileStart).toBeGreaterThan(tabletStart)
    expect(phoneStart).toBeGreaterThan(mobileStart)
    expect(tabletStyles).toMatch(/\.jenkins-home\s*\{[^}]*grid-template-columns:\s*220px minmax\(0,\s*1fr\)/)
    expect(tabletStyles).toMatch(/\.jenkins-rail-root\s*\{[^}]*width:\s*220px/)
    expect(mobileStyles).toMatch(/\.jenkins-home\s*\{[^}]*grid-template-columns:\s*minmax\(0,\s*1fr\)/)
    expect(mobileStyles).toMatch(/\.jenkins-rail-root\s*\{[^}]*width:\s*100%/)
    expect(mobileStyles).toMatch(/\.jenkins-rail-links\s*\{[^}]*display:\s*flex[^}]*overflow-x:\s*auto/)
    expect(mobileStyles).toMatch(/\.app-topbar,\s*\[data-skin="jenkins"\] \.app-topbar\s*\{[^}]*height:\s*50px/)
    expect(jenkinsPageStyles).toMatch(/@media \(max-width: 900px\)[\s\S]*\.jenkins-context-layout\s*\{[^}]*grid-template-columns:\s*minmax\(0, 1fr\)/)
    expect(jenkinsPageStyles).toMatch(/@media \(max-width: 760px\)[\s\S]*\.jenkins-context-page\s*\{[^}]*min-height:\s*calc\(100vh - 50px\)/)
  })

  it('keeps one document main landmark inside JenkinsPageShell', () => {
    const breadcrumbHost = document.createElement('div')
    breadcrumbHost.id = 'jenkins-header-breadcrumbs'
    const outerMain = document.createElement('main')
    document.body.append(breadcrumbHost, outerMain)
    const root = createRoot(outerMain)
    act(() => root.render(<MemoryRouter><JenkinsPageShell breadcrumbs={[{ label: 'Projects' }]} sidepanel={<span>Tasks</span>} sidepanelLabel="Tasks"><h1>Projects</h1></JenkinsPageShell></MemoryRouter>))

    expect(document.querySelectorAll('main')).toHaveLength(1)
    expect(outerMain.querySelector('.jenkins-context-main')?.tagName).toBe('DIV')
    expect(outerMain.querySelector('.jenkins-context-breadcrumb')).toBeNull()
    expect(breadcrumbHost.querySelector('.jenkins-context-breadcrumb')).not.toBeNull()
    expect(breadcrumbHost.textContent).toContain('Projects')

    act(() => root.unmount())
  })

  it('lets the Jenkins settings directory escape the legacy content max-width', () => {
    document.documentElement.dataset.skin = 'jenkins'
    installStyles(`${indexStyles}\n${settingsStyles}`)
    const appMain = document.createElement('main')
    appMain.className = 'app-main'
    const settings = document.createElement('section')
    settings.className = 'settings-center'
    appMain.appendChild(settings)
    document.body.appendChild(appMain)

    expect(getComputedStyle(appMain).maxWidth).toBe('none')
  })

  it('keeps project workbench stretched through available viewport height', () => {
    expect(styles).toMatch(/\.project-detail-page\s*\{[^}]*height:\s*calc\(100dvh - 126px\)[^}]*flex-direction:\s*column/)
    expect(styles).toMatch(/\.project-workbench\s*\{[^}]*flex:\s*1[^}]*flex-direction:\s*column/)
    expect(styles).toMatch(/\.project-workbench \.detail-panel-body\s*\{[^}]*flex:\s*1[^}]*overflow:\s*auto/)
  })

  it('exposes nine restrained project-folder color presets', () => {
    expect(PROJECT_GROUP_COLORS).toEqual([
      'neutral', 'blue', 'cyan', 'mint', 'green', 'yellow', 'orange', 'pink', 'purple',
    ])
    for (const color of PROJECT_GROUP_COLORS.filter(value => value !== 'neutral')) {
      expect(styles).toContain(`[data-group-color="${color}"]`)
    }
  })

  it('renders the requested moon and build hammer mark', () => {
    const container = document.createElement('div')
    document.body.appendChild(container)
    const root = createRoot(container)
    act(() => root.render(<BuildWorldMark title="BuildWorld" />))

    const mark = container.querySelector('svg[role="img"][aria-label="BuildWorld"]')
    expect(mark?.querySelector('path[fill="url(#buildworld-moon)"]')).not.toBeNull()
    expect(mark?.querySelector('g[transform^="rotate("] path[fill="url(#buildworld-hammer)"]')).not.toBeNull()

    act(() => root.unmount())
  })

  it('keeps active build feedback visible without forcing motion', () => {
    expect(styles).toMatch(/\[data-skin="jenkins"\] \.build-status-live::before\s*\{[^}]*display:\s*none/)
    expect(styles).toMatch(/\[data-skin="jenkins"\] \.build-status-spinner\s*\{[^}]*display:\s*inline-block/)
    expect(styles).toMatch(/\.timeline-step\.running \.timeline-status::after\s*\{[^}]*animation:\s*build-activity-pulse/)
    expect(styles).toMatch(/@media \(prefers-reduced-motion: reduce\)\s*\{[^}]*\.timeline-spinner[^}]*animation:\s*none/)
    expect(styles).toMatch(/@media \(prefers-reduced-motion: reduce\)\s*\{[^}]*\.timeline-step\.running \.timeline-status::after[^}]*animation:\s*none/)
    expect(styles).toMatch(/@media \(prefers-reduced-motion: reduce\)\s*\{\s*\.jenkins-status-orb\.running,\s*\.jenkins-rail-loading svg\s*\{\s*animation:\s*none/)
  })
})
