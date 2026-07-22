// @vitest-environment jsdom

import { act } from 'react'
import { createRoot, type Root } from 'react-dom/client'
import { MemoryRouter, Route, Routes, useLocation } from 'react-router-dom'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { api } from '../api'
import CreateProject from './CreateProject'

vi.mock('../api', () => ({
  api: {
    listProjects: vi.fn(),
    listProjectGroups: vi.fn(),
    listTemplates: vi.fn(),
    createProject: vi.fn(),
    createProjectGroup: vi.fn(),
    validatePipeline: vi.fn(),
  },
}))

const actEnvironment = globalThis as typeof globalThis & { IS_REACT_ACT_ENVIRONMENT: boolean }

function LocationProbe() {
  const location = useLocation()
  return <output data-location>{location.pathname}</output>
}

async function flushRequests() {
  await act(async () => {
    await Promise.resolve()
    await Promise.resolve()
  })
}

describe('Jenkins New Item flow', () => {
  let container: HTMLDivElement
  let breadcrumbHost: HTMLDivElement
  let root: Root

  beforeEach(() => {
    actEnvironment.IS_REACT_ACT_ENVIRONMENT = true
    breadcrumbHost = document.createElement('div')
    breadcrumbHost.id = 'jenkins-header-breadcrumbs'
    document.body.appendChild(breadcrumbHost)
    container = document.createElement('div')
    document.body.appendChild(container)
    root = createRoot(container)
    vi.mocked(api.listProjects).mockResolvedValue([{ id: 1, name: 'Existing pipeline' }])
    vi.mocked(api.listProjectGroups).mockResolvedValue([{ id: 2, name: 'Existing folder' }])
    vi.mocked(api.listTemplates).mockResolvedValue([])
    vi.mocked(api.validatePipeline).mockResolvedValue({ valid: true })
    vi.mocked(api.createProject).mockResolvedValue({ id: 42 })
    vi.mocked(api.createProjectGroup).mockResolvedValue({ id: 9 })
  })

  afterEach(() => {
    act(() => root.unmount())
    container.remove()
    breadcrumbHost.remove()
    localStorage.clear()
    vi.clearAllMocks()
    actEnvironment.IS_REACT_ACT_ENVIRONMENT = false
  })

  async function renderPage(entry = '/projects/new') {
    await act(async () => {
      root.render(
        <MemoryRouter initialEntries={[entry]}>
          <Routes>
            <Route path="*" element={<><CreateProject /><LocationProbe /></>} />
          </Routes>
        </MemoryRouter>,
      )
    })
    await flushRequests()
  }

  async function enterName(value: string) {
    const input = container.querySelector<HTMLInputElement>('#jenkins-new-item-name')
    await act(async () => {
      if (!input) return
      Object.getOwnPropertyDescriptor(HTMLInputElement.prototype, 'value')?.set?.call(input, value)
      input.dispatchEvent(new Event('input', { bubbles: true }))
    })
  }

  it('matches Jenkins item-name and item-type selection semantics', async () => {
    await renderPage()

    expect(breadcrumbHost.textContent).toMatch(/New Item|新建任务/)
    expect(container.querySelector('.jenkins-new-item-panel')).not.toBeNull()
    expect(container.querySelectorAll('input[name="mode"]')).toHaveLength(2)
    expect(container.querySelector<HTMLInputElement>('input[value="pipeline"]')).not.toBeNull()
    expect(container.querySelector<HTMLInputElement>('input[value="folder"]')).not.toBeNull()
    expect(container.querySelector<HTMLButtonElement>('button[type="submit"]')?.disabled).toBe(true)

    await enterName('Existing pipeline')
    await act(async () => {
      container.querySelector<HTMLInputElement>('input[value="pipeline"]')?.click()
    })
    expect(container.querySelector<HTMLInputElement>('#jenkins-new-item-name')?.getAttribute('aria-invalid')).toBe('true')
    expect(container.querySelector('[role="alert"]')?.textContent).toMatch(/exists|同名/)
    expect(container.querySelector<HTMLButtonElement>('button[type="submit"]')?.disabled).toBe(true)

    await enterName('release-pipeline')
    expect(container.querySelector<HTMLButtonElement>('button[type="submit"]')?.disabled).toBe(false)
  })

  it('creates a TypeScript Pipeline then opens Jenkins Configure', async () => {
    let finishCreate: ((value: { id: number }) => void) | undefined
    vi.mocked(api.createProject).mockImplementation(() => new Promise(resolve => { finishCreate = resolve }))
    await renderPage()
    await enterName('release-pipeline')
    await act(async () => {
      container.querySelector<HTMLInputElement>('input[value="pipeline"]')?.click()
    })
    await act(async () => {
      container.querySelector<HTMLButtonElement>('button[type="submit"]')?.click()
      await Promise.resolve()
    })

    expect(container.querySelector('form')?.getAttribute('aria-busy')).toBe('true')
    expect(container.querySelector('.jenkins-new-item-spinner')).not.toBeNull()
    expect(api.validatePipeline).toHaveBeenCalledWith(expect.stringContaining('definePipeline'))
    expect(api.createProject).toHaveBeenCalledWith(expect.objectContaining({
      name: 'release-pipeline',
      repo_type: 'git',
      pipeline_format: 'typescript',
      pipeline_source_mode: 'inline',
    }))

    await act(async () => {
      finishCreate?.({ id: 42 })
      await Promise.resolve()
    })
    expect(container.querySelector('[data-location]')?.textContent).toBe('/projects/42/configure')
  })

  it('creates a Pipeline from the requested build template and persists its source links', async () => {
    const templateConfig = 'jobs:\n  build:\n    steps:\n      - run: echo from-template\n'
    vi.mocked(api.listTemplates).mockResolvedValue([{
      id: 7,
      name: 'Go service',
      description: 'Reusable server pipeline',
      config: templateConfig,
      vcs_root_id: 19,
      repo_url: 'https://example.test/game.git',
      default_branch: 'release',
    }])
    await renderPage('/projects/new?template=7')

    expect(api.listTemplates).toHaveBeenCalledOnce()
    expect(container.querySelector<HTMLInputElement>('input[value="pipeline"]')?.checked).toBe(true)
    expect(container.querySelector('.jenkins-new-item-template')?.textContent).toContain('Go service')
    await enterName('templated-service')
    await act(async () => {
      container.querySelector<HTMLButtonElement>('button[type="submit"]')?.click()
      await Promise.resolve()
      await Promise.resolve()
    })

    expect(api.validatePipeline).toHaveBeenCalledWith(templateConfig)
    expect(api.createProject).toHaveBeenCalledWith(expect.objectContaining({
      name: 'templated-service',
      config: templateConfig,
      template_id: 7,
      vcs_root_id: 19,
      repo_url: 'https://example.test/game.git',
      default_branch: 'release',
      pipeline_format: 'yaml',
    }))
  })

  it('does not silently create an untemplated Pipeline when the requested template is missing', async () => {
    vi.mocked(api.listTemplates).mockResolvedValue([])
    await renderPage('/projects/new?template=999')
    await enterName('missing-template-job')
    await act(async () => {
      container.querySelector<HTMLInputElement>('input[value="pipeline"]')?.click()
    })

    expect(container.querySelector('[role="alert"]')?.textContent).toMatch(/does not exist|不存在/)
    expect(container.querySelector<HTMLButtonElement>('button[type="submit"]')?.disabled).toBe(true)
    expect(api.createProject).not.toHaveBeenCalled()
  })

  it('creates a real one-level folder without creating or running a project', async () => {
    vi.mocked(api.listTemplates).mockResolvedValue([{ id: 8, name: 'Ignored template', config: 'jobs:\n  build:\n    steps: []\n' }])
    await renderPage('/projects/new?template=8')
    await enterName('game-servers')
    await act(async () => {
      container.querySelector<HTMLInputElement>('input[value="folder"]')?.click()
    })
    await act(async () => {
      container.querySelector<HTMLButtonElement>('button[type="submit"]')?.click()
      await Promise.resolve()
      await Promise.resolve()
    })

    expect(api.createProjectGroup).toHaveBeenCalledWith({ name: 'game-servers', description: '', color: 'neutral' })
    expect(api.createProject).not.toHaveBeenCalled()
    expect(api.validatePipeline).not.toHaveBeenCalled()
    expect(container.querySelector('[data-location]')?.textContent).toBe('/projects')
  })

  it('keeps the form available and exposes backend failures', async () => {
    vi.mocked(api.createProject).mockRejectedValue(new Error('Name rejected by server'))
    await renderPage()
    await enterName('bad-name')
    await act(async () => {
      container.querySelector<HTMLInputElement>('input[value="pipeline"]')?.click()
    })
    await act(async () => {
      container.querySelector<HTMLButtonElement>('button[type="submit"]')?.click()
      await Promise.resolve()
      await Promise.resolve()
    })

    expect(container.querySelector('[role="alert"]')?.textContent).toContain('Name rejected by server')
    expect(container.querySelector('form')?.getAttribute('aria-busy')).toBe('false')
    expect(container.querySelector<HTMLButtonElement>('button[type="submit"]')?.disabled).toBe(false)
  })
})
