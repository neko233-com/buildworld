// Run against an authenticated, isolated BuildWorld fixture through the in-app
// Browser: await runDesktopSmoke(tab, 'http://127.0.0.2:8080', { projectID: 1, buildID: 1 }).
// This suite is read-only. It never triggers real builds, changes credentials,
// enables remote Workers, or creates/deletes production data.
export async function runDesktopSmoke(tab, baseURL, { projectID, buildID } = {}) {
  const startedAt = Date.now()
  const routes = [
    '/', '/projects', '/projects/new', '/builds', '/build-queue', '/templates',
    '/vcs-roots', '/credentials', '/notifications', '/plugins', '/api-tokens',
    '/users', '/settings', '/statistics', '/audit-log', '/my-dashboard', '/bigscreen',
  ]
  if (projectID) routes.push(`/projects/${projectID}`, `/projects/${projectID}/configure`, `/projects/${projectID}/changes`, `/projects/${projectID}/build`)
  if (buildID) routes.push(`/builds/${buildID}`, `/builds/${buildID}/tests`, `/builds/${buildID}/logs`)
  // Revisit after lazy page styles load: sidebar CSS must not alter table actions.
  routes.push('/')
  const results = []
  for (const route of routes) {
    await tab.goto(`${baseURL.replace(/\/$/, '')}${route}`)
    await tab.playwright.getByRole('heading', { level: 1 }).waitFor({ state: 'visible', timeoutMs: 15_000 })
    await tab.playwright.locator('.page-state.loading, .route-loading').first().waitFor({ state: 'hidden', timeoutMs: 15_000 })
    const state = await tab.playwright.evaluate(() => {
      const body = document.body
      const isVisible = element => element.getClientRects().length > 0
      const errors = Array.from(document.querySelectorAll('[role="alert"], .route-error')).filter(isVisible).map(element => element.textContent?.trim()).filter(Boolean)
      const table = document.querySelector('.jenkins-job-table')
      const action = table?.querySelector('.jenkins-row-actions button:not(:disabled)')
      const layoutErrors = []
      if (table) {
        if (table.querySelector('thead')?.getBoundingClientRect().height > 60) layoutErrors.push('dashboard headers wrap across multiple lines')
        if (table.querySelector('th:nth-child(3)')?.getBoundingClientRect().width < 240) layoutErrors.push('project name column is too narrow')
        if (action && (getComputedStyle(action).opacity === '0' || getComputedStyle(action.parentElement).display !== 'flex')) layoutErrors.push('dashboard actions hidden or affected by sidebar styles')
        const success = table.querySelector('.jenkins-status-orb.success')
        if (success && getComputedStyle(success).color !== 'rgb(47, 158, 91)') layoutErrors.push('success status lost its green meaning')
      }
      return {
        title: document.title,
        heading: document.querySelector('h1')?.textContent?.trim(),
        blank: body.innerText.trim().length < 10,
        overflow: document.documentElement.scrollWidth > window.innerWidth + 1,
        overlay: Boolean(document.querySelector('vite-error-overlay')),
        errors,
        layoutErrors,
        placeholders: Array.from(document.querySelectorAll('progress')).map(element => element.getAttribute('aria-label') || '').filter(label => /\{(?:completed|total|percent)\}/.test(label)),
      }
    })
    const failures = []
    if (!state.title || !state.heading || state.blank) failures.push('blank page or missing identity')
    if (state.overflow) failures.push('page-level horizontal overflow')
    if (state.overlay) failures.push('framework error overlay')
    if (state.errors.length) failures.push(...state.errors)
    if (state.layoutErrors.length) failures.push(...state.layoutErrors)
    if (state.placeholders.length) failures.push('unresolved progress labels')
    results.push({ route, ...state, failures })
  }
  const consoleErrors = (await tab.dev.logs({ levels: ['error'], limit: 100 }))
    .filter(entry => Date.parse(entry.timestamp) >= startedAt)
  const failed = results.filter(result => result.failures.length)
  if (failed.length || consoleErrors.length) {
    throw new Error(JSON.stringify({ failed, consoleErrors }))
  }
  return results.map(({ route, heading }) => ({ route, heading, passed: true }))
}
