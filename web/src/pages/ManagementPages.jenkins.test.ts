import { readFileSync } from 'node:fs'
import { resolve } from 'node:path'
import { describe, expect, it } from 'vitest'

const pageNames = [
  'Agents',
  'Notifications',
  'Plugins',
  'Users',
  'AuditLog',
  'Statistics',
  'BigScreen',
  'Settings',
]

describe('Jenkins administration page shell', () => {
  it.each(pageNames)('%s owns a header breadcrumb and never nests a main landmark', pageName => {
    const source = readFileSync(resolve(process.cwd(), `src/pages/${pageName}.tsx`), 'utf8')

    expect(source).toContain('JenkinsHeaderBreadcrumb')
    expect(source).toContain('jenkins-management-page')
    expect(source).not.toMatch(/<main(?:\s|>)/)
  })

  it('uses full-width Jenkins geometry and readable table density', () => {
    const styles = readFileSync(resolve(process.cwd(), 'src/pages/ManagementPages.jenkins.css'), 'utf8')

    expect(styles).toMatch(/\.app-main:has\(\.jenkins-management-page\)[^{]*\{[^}]*max-width:\s*none[^}]*padding:\s*26px 26px 64px/s)
    expect(styles).toMatch(/\.jenkins-management-page\s*\{[^}]*font-size:\s*14px/s)
    expect(styles).toMatch(/\.operations-table td,[\s\S]*\.plugin-table td\s*\{[^}]*height:\s*49px[^}]*font-size:\s*14px/s)
  })
})
