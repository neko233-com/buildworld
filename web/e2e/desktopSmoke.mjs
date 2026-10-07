// Run against an authenticated, isolated BuildWorld fixture through the in-app
// Browser: await runDesktopSmoke(tab, 'http://127.0.0.2:8080', { projectID: 1, buildID: 1 }).
// This suite is read-only. It never triggers real builds, changes credentials,
// enables remote Workers, or creates/deletes production data.
export async function runDesktopSmoke(tab, baseURL, { projectID, buildID } = {}) {
  const routes = [
    '/', '/projects', '/projects/new', '/builds', '/build-queue', '/templates',
    '/vcs-roots', '/credentials', '/notifications', '/plugins', '/api-tokens',
    '/users', '/settings', '/statistics', '/audit-log', '/my-dashboard', '/bigscreen',
  ]
  if (projectID) routes.push(`/projects/${projectID}`, `/projects/${projectID}/configure`, `/projects/${projectID}/changes`, `/projects/${projectID}/build`)
  if (buildID) routes.push(`/builds/${buildID}`, `/builds/${buildID}/tests`, `/builds/${buildID}/logs`)
  const results = []
  for (const route of routes) {
    await tab.goto(`${baseURL.replace(/\/$/, '')}${route}`)
    await tab.playwright.getByRole('heading', { level: 1 }).waitFor({ state: 'visible', timeoutMs: 15_000 })
    await tab.playwright.locator('.page-state.loading, .route-loading').first().waitFor({ state: 'hidden', timeoutMs: 15_000 })
    const state = await tab.playwright.evaluate(() => {
      const body = document.body
      const isVisible = element => element.getClientRects().length > 0
      const errors = Array.from(document.querySelectorAll('[role="alert"], .route-error')).filter(isVisible).map(element => element.textContent?.trim()).filter(Boolean)
      return {
        title: document.title,
        heading: document.querySelector('h1')?.textContent?.trim(),
        blank: body.innerText.trim().length < 10,
        overflow: document.documentElement.scrollWidth > window.innerWidth + 1,
        overlay: Boolean(document.querySelector('vite-error-overlay')),
        errors,
        placeholders: Array.from(document.querySelectorAll('progress')).map(element => element.getAttribute('aria-label') || '').filter(label => /\{(?:completed|total|percent)\}/.test(label)),
      }
    })
    const failures = []
    if (!state.title || !state.heading || state.blank) failures.push('blank page or missing identity')
    if (state.overflow) failures.push('page-level horizontal overflow')
    if (state.overlay) failures.push('framework error overlay')
    if (state.errors.length) failures.push(...state.errors)
    if (state.placeholders.length) failures.push('unresolved progress labels')
    results.push({ route, ...state, failures })
  }
  const consoleErrors = await tab.dev.logs({ levels: ['error'], limit: 100 })
  const failed = results.filter(result => result.failures.length)
  if (failed.length || consoleErrors.length) {
    throw new Error(JSON.stringify({ failed, consoleErrors }))
  }
  return results.map(({ route, heading }) => ({ route, heading, passed: true }))
}
