// @vitest-environment jsdom

import { act } from 'react'
import { createRoot, type Root } from 'react-dom/client'
import { MemoryRouter, Route, Routes, useLocation } from 'react-router-dom'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { api } from '../api'
import ProjectBuildWithParameters from './ProjectBuildWithParameters'

vi.mock('../api', () => ({
  api: {
    getProject: vi.fn(),
    validateProject: vi.fn(),
    triggerBuild: vi.fn(),
  },
}))

const project = {
  id: 7,
  name: 'Weather',
  default_branch: 'main',
  config: 'parameters: []',
}

function LocationProbe() {
  return <output aria-label="location">{useLocation().pathname}</output>
}

function deferred<T>() {
  let resolve!: (value: T) => void
  let reject!: (reason: unknown) => void
  const promise = new Promise<T>((resolvePromise, rejectPromise) => {
    resolve = resolvePromise
    reject = rejectPromise
  })
  return { promise, resolve, reject }
}

describe('ProjectBuildWithParameters', () => {
  let container: HTMLDivElement
  let breadcrumbHost: HTMLDivElement
  let root: Root

  beforeEach(() => {
    ;(globalThis as any).IS_REACT_ACT_ENVIRONMENT = true
    localStorage.setItem('locale', 'en')
    container = document.createElement('div')
    breadcrumbHost = document.createElement('div')
    breadcrumbHost.id = 'jenkins-header-breadcrumbs'
    document.body.append(breadcrumbHost, container)
    root = createRoot(container)
    vi.mocked(api.getProject).mockResolvedValue(project)
    vi.mocked(api.validateProject).mockResolvedValue({
      valid: true,
      format: 'typescript',
      stages: 1,
      steps: 1,
      parameters: [],
      allow_long_running: false,
    })
    vi.mocked(api.triggerBuild).mockResolvedValue({ id: 901 })
    vi.spyOn(window, 'requestAnimationFrame').mockImplementation(callback => {
      callback(0)
      return 1
    })
    vi.spyOn(window, 'cancelAnimationFrame').mockImplementation(() => undefined)
  })

  afterEach(() => {
    act(() => root.unmount())
    breadcrumbHost.remove()
    container.remove()
    localStorage.clear()
    vi.clearAllMocks()
    vi.restoreAllMocks()
    ;(globalThis as any).IS_REACT_ACT_ENVIRONMENT = false
  })

  async function renderPage() {
    await act(async () => {
      root.render(<MemoryRouter initialEntries={['/projects/7/build']}><Routes>
        <Route path="/projects/:id/build" element={<><ProjectBuildWithParameters /><LocationProbe /></>} />
        <Route path="/projects/:id" element={<LocationProbe />} />
        <Route path="/builds/:id" element={<LocationProbe />} />
      </Routes></MemoryRouter>)
      await Promise.resolve()
      await Promise.resolve()
      await Promise.resolve()
    })
  }

  it('loads the real project and authoritative parameters inside a Jenkins page shell', async () => {
    const projectRequest = deferred<typeof project>()
    const validationRequest = deferred<any>()
    vi.mocked(api.getProject).mockReturnValue(projectRequest.promise)
    vi.mocked(api.validateProject).mockReturnValue(validationRequest.promise)

    await act(async () => {
      root.render(<MemoryRouter initialEntries={['/projects/7/build']}><Routes><Route path="/projects/:id/build" element={<ProjectBuildWithParameters />} /></Routes></MemoryRouter>)
      await Promise.resolve()
    })

    expect(container.querySelector('[role="status"]')?.textContent).toContain('Loading')
    expect(container.querySelector('.jenkins-context-layout')).not.toBeNull()

    await act(async () => {
      projectRequest.resolve(project)
      await projectRequest.promise
      await Promise.resolve()
      await Promise.resolve()
    })

    expect(api.getProject).toHaveBeenCalledWith(7)
    expect(api.validateProject).toHaveBeenCalledWith(7)
    expect(container.querySelector('.run-parameter-loading[role="status"]')?.textContent).toContain('Loading')
    expect(container.querySelector<HTMLButtonElement>('button[type="submit"]')?.disabled).toBe(true)

    await act(async () => {
      validationRequest.resolve({ valid: true, format: 'typescript', stages: 1, steps: 1, parameters: [], allow_long_running: false })
      await validationRequest.promise
      await Promise.resolve()
    })

    expect(container.querySelector('h1')?.textContent).toBe('Build with Parameters')
    expect(container.querySelector('.jenkins-context-sidepanel')).not.toBeNull()
    expect(container.querySelector('a.active[href="/projects/7/build"]')).not.toBeNull()
    expect(breadcrumbHost.textContent).toContain('Weather')
    expect(breadcrumbHost.textContent).toContain('Build with Parameters')
  })

  it('blocks submission after metadata failure and supports an explicit retry', async () => {
    vi.mocked(api.validateProject).mockRejectedValueOnce(new Error('parameter metadata unavailable'))
      .mockResolvedValueOnce({ valid: true, format: 'typescript', stages: 1, steps: 1, parameters: [], allow_long_running: false })
    await renderPage()

    const alert = container.querySelector<HTMLElement>('.run-parameter-failure[role="alert"]')
    expect(alert?.textContent).toContain('parameter metadata unavailable')
    expect(container.querySelector<HTMLButtonElement>('button[type="submit"]')?.disabled).toBe(true)
    expect(api.triggerBuild).not.toHaveBeenCalled()

    await act(async () => {
      alert?.querySelector<HTMLButtonElement>('button')?.click()
      await Promise.resolve()
      await Promise.resolve()
    })

    expect(api.validateProject).toHaveBeenCalledTimes(2)
    expect(container.querySelector<HTMLButtonElement>('button[type="submit"]')?.disabled).toBe(false)
  })

  it('marks required parameters invalid, associates the error, and focuses the first error', async () => {
    vi.mocked(api.validateProject).mockResolvedValue({
      valid: true,
      format: 'typescript',
      stages: 1,
      steps: 1,
      parameters: [
        { name: 'API_TOKEN', type: 'password', required: true, is_secret: true, description: 'Deployment credential' },
        { name: 'REGION', type: 'string', required: true },
      ],
      allow_long_running: false,
    })
    await renderPage()

    await act(async () => {
      container.querySelector('form')?.dispatchEvent(new Event('submit', { bubbles: true, cancelable: true }))
      await Promise.resolve()
    })

    const token = container.querySelector<HTMLInputElement>('input[type="password"]')!
    expect(token.getAttribute('aria-invalid')).toBe('true')
    const describedBy = token.getAttribute('aria-describedby') || ''
    expect(describedBy).not.toBe('')
    expect(describedBy.split(' ').some(id => document.getElementById(id)?.textContent?.includes('API_TOKEN is required'))).toBe(true)
    expect(document.activeElement).toBe(token)
    expect(api.triggerBuild).not.toHaveBeenCalled()
  })

  it('locks controls while queueing and navigates only after a successful trigger', async () => {
    const buildRequest = deferred<{ id: number }>()
    vi.mocked(api.validateProject).mockResolvedValue({
      valid: true,
      format: 'typescript',
      stages: 1,
      steps: 1,
      parameters: [
        { name: 'ENV', type: 'choice', required: true, choices: ['staging', 'production'] },
        { name: 'TOKEN', type: 'password', required: true, is_secret: true },
      ],
      allow_long_running: false,
    })
    vi.mocked(api.triggerBuild).mockReturnValue(buildRequest.promise)
    await renderPage()

    const branch = container.querySelector<HTMLInputElement>('.run-build-branch input')!
    Object.getOwnPropertyDescriptor(HTMLInputElement.prototype, 'value')?.set?.call(branch, 'release')
    await act(async () => branch.dispatchEvent(new Event('input', { bubbles: true })))
    const token = container.querySelector<HTMLInputElement>('input[type="password"]')!
    Object.getOwnPropertyDescriptor(HTMLInputElement.prototype, 'value')?.set?.call(token, 'secret-value')
    await act(async () => token.dispatchEvent(new Event('input', { bubbles: true })))

    await act(async () => {
      container.querySelector('form')?.dispatchEvent(new Event('submit', { bubbles: true, cancelable: true }))
      await Promise.resolve()
    })

    const buttons = Array.from(container.querySelectorAll<HTMLButtonElement>('form footer button'))
    expect(api.triggerBuild).toHaveBeenCalledWith(7, {
      branch: 'release',
      parameters: { ENV: 'staging', TOKEN: 'secret-value' },
    })
    expect(buttons.every(button => button.disabled)).toBe(true)
    expect(container.querySelector('button[type="submit"] [role="status"]')?.textContent).toContain('Queueing')
    expect(container.querySelector('output[aria-label="location"]')?.textContent).toBe('/projects/7/build')

    await act(async () => {
      buildRequest.resolve({ id: 902 })
      await buildRequest.promise
      await Promise.resolve()
    })
    expect(container.querySelector('output[aria-label="location"]')?.textContent).toBe('/builds/902')
  })
})
