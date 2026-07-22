import { readFileSync } from 'node:fs'
import { resolve } from 'node:path'
import { describe, expect, it } from 'vitest'

const administrationPages = ['VCSRoots', 'Credentials', 'APITokens']
describe('remaining Jenkins page shells', () => {
  it.each(administrationPages)('%s keeps its breadcrumb during loading and error states', pageName => {
    const source = readFileSync(resolve(process.cwd(), `src/pages/${pageName}.tsx`), 'utf8')

    expect(source).toContain('JenkinsHeaderBreadcrumb')
    expect(source).toContain('jenkins-management-page')
    expect(source).toContain('<>{breadcrumb}<section className="jenkins-management-page"><PageState')
    expect(source).not.toMatch(/<main(?:\s|>)/)
  })

  it('keeps the build breadcrumb and Jenkins Run shell for TestReports states', () => {
    const source = readFileSync(resolve(process.cwd(), 'src/pages/TestReports.tsx'), 'utf8')

    expect(source).toContain('JenkinsHeaderBreadcrumb')
    expect(source).toContain('<>{breadcrumb}<section className="jenkins-run-page test-report-page"><PageState')
    expect(source).toContain('jenkins-run-layout')
    expect(source).toContain('jenkins-run-side-panel')
    expect(source).not.toMatch(/<main(?:\s|>)/)
  })

  it.each(administrationPages)('%s names destructive entities and exposes a deletion busy state', pageName => {
    const source = readFileSync(resolve(process.cwd(), `src/pages/${pageName}.tsx`), 'utf8')

    expect(source).toMatch(/ConfirmNamed'\)\.replace\('\{name\}'/)
    expect(source).toContain('deletingID')
    expect(source).toContain('aria-busy={deleting || undefined}')
    expect(source).toContain('className="timeline-spinner"')
  })

  it.each(administrationPages)('%s locks close and cancel while saving', pageName => {
    const source = readFileSync(resolve(process.cwd(), `src/pages/${pageName}.tsx`), 'utf8')
    const busyName = pageName === 'APITokens' ? 'creating' : 'saving'

    expect(source).toContain(`busy={${busyName}}`)
    expect(source).toMatch(new RegExp(`<header>[\\s\\S]*?<button type="button" disabled=\\{${busyName}\\}[\\s\\S]*?title=\\{t\\('common.close'\\)\\}`))
    expect(source).toMatch(new RegExp(`<footer><button type="button" disabled=\\{${busyName}\\}`))
  })

  it('loads build and project context for Jenkins test-result breadcrumbs', () => {
    const source = readFileSync(resolve(process.cwd(), 'src/pages/TestReports.tsx'), 'utf8')

    expect(source).toContain('api.getBuild(buildId)')
    expect(source).toContain('api.getProject(build.project_id)')
    expect(source).toContain('{ label: `#${buildNumber}`, to: `/builds/${buildId}` }')
  })

  it('uses Jenkins 49px test-result rows', () => {
    const styles = readFileSync(resolve(process.cwd(), 'src/pages/TestReports.jenkins.css'), 'utf8')

    expect(styles).toMatch(/\.test-report-page \.operations-table td\s*\{[^}]*height:\s*49px[^}]*font-size:\s*14px/s)
  })
})
