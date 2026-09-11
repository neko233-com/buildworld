// @vitest-environment jsdom

import { act } from 'react'
import { createRoot, type Root } from 'react-dom/client'
import { MemoryRouter, useLocation } from 'react-router-dom'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { api } from '../api'
import Projects from './Projects'

vi.mock('../api', () => ({
  api: {
    listProjects: vi.fn(),
    getProject: vi.fn(),
    validateProject: vi.fn(),
    triggerBuild: vi.fn(),
    deleteProject: vi.fn(),
    setProjectFlags: vi.fn(),
  },
}))

const listProjects = vi.mocked(api.listProjects)
const getProject = vi.mocked(api.getProject)
const validateProject = vi.mocked(api.validateProject)
const triggerBuild = vi.mocked(api.triggerBuild)
const setProjectFlags = vi.mocked(api.setProjectFlags)
const actEnvironment = globalThis as typeof globalThis & { IS_REACT_ACT_ENVIRONMENT: boolean }

async function flushRequests() {
  await act(async () => {
    await Promise.resolve()
    await Promise.resolve()
  })
}

function LocationProbe() {
  return <output aria-label="location">{useLocation().pathname}</output>
}

describe('Projects', () => {
  let container: HTMLDivElement
  let root: Root

  beforeEach(() => {
    actEnvironment.IS_REACT_ACT_ENVIRONMENT = true
    const payload = btoa(JSON.stringify({ role: 'admin' }))
    localStorage.setItem('token', `test.${payload}.signature`)
    listProjects.mockReset()
    getProject.mockReset()
    validateProject.mockReset()
    triggerBuild.mockReset()
    setProjectFlags.mockReset().mockResolvedValue(undefined)
    listProjects.mockResolvedValue([{
      id: 3,
      name: 'Packaging matrix',
      description: 'Cross-language release validation',
      tags: ['qa'],
      repo_type: 'git',
      repo_url: 'https://example.invalid/matrix.git',
      default_branch: 'main',
      created_at: '2026-07-19T10:00:00Z',
    }])
    getProject.mockImplementation(async id => ({ ...(await listProjects())[0], id }))
    validateProject.mockResolvedValue({ valid: true, format: 'typescript', stages: 1, steps: 1, parameters: [], allow_long_running: false })
    container = document.createElement('div')
    document.body.appendChild(container)
    root = createRoot(container)
  })

  afterEach(() => {
    act(() => root.unmount())
    container.remove()
    localStorage.clear()
    actEnvironment.IS_REACT_ACT_ENVIRONMENT = false
  })

  it('uses real links for create, project details, and project settings', async () => {
    await act(async () => {
      root.render(<MemoryRouter><Projects /></MemoryRouter>)
    })
    expect(container.querySelector('a[href="/projects/new"]')).not.toBeNull()
    expect(container.querySelector('a.entity-link[href="/projects/3"]')?.textContent).toContain('Packaging matrix')
    expect(container.querySelector('a.row-settings[href="/projects/3/configure"]')).not.toBeNull()
    expect(container.querySelector('button.custom-build')).not.toBeNull()
    expect(container.querySelector('button.entity-link')).toBeNull()
    expect(container.querySelector('tr[tabindex]')).toBeNull()
    expect(container.querySelector('.jenkins-projects-page')).not.toBeNull()
    expect(container.querySelector('.jenkins-page-heading-count')?.textContent).toBe('1')
    expect(container.querySelector('caption')?.textContent).toMatch(/项目|Projects/)
    expect(container.querySelector('.project-folder')).toBeNull()
    expect(container.querySelector('thead')?.textContent).not.toContain('分组')
  })

  it('shows Jenkins-sized busy feedback while a project flag is saved', async () => {
    let resolveFlag!: () => void
    setProjectFlags.mockReturnValue(new Promise<void>(resolve => { resolveFlag = resolve }))
    await act(async () => {
      root.render(<MemoryRouter><Projects /></MemoryRouter>)
    })
    const favorite = container.querySelector<HTMLButtonElement>('button.favorite')!
    await act(async () => {
      favorite.click()
      await Promise.resolve()
    })
    expect(favorite.getAttribute('aria-busy')).toBe('true')
    expect(favorite.querySelector('svg.timeline-spinner')).not.toBeNull()

    await act(async () => {
      resolveFlag()
      await Promise.resolve()
    })
    expect(setProjectFlags).toHaveBeenCalledWith(3, true, undefined)
  })

  it('routes parameterized quick builds to the dedicated Jenkins form', async () => {
    const project = {
      id: 4,
      name: 'Signed release',
      repo_type: 'git',
      default_branch: 'main',
      config: `import { definePipeline, parameter } from '@buildworld/pipeline'
export default definePipeline({ parameters: [parameter('signing_token', 'password', { required: true })], stages: [] })
`,
    }
    listProjects.mockResolvedValue([{ ...project, config: undefined }])
    getProject.mockResolvedValue(project)
    validateProject.mockResolvedValue({ valid: true, format: 'typescript', stages: 0, steps: 0, parameters: [{ name: 'signing_token', type: 'password', required: true }], allow_long_running: false })
    await act(async () => {
      root.render(<MemoryRouter><Projects /><LocationProbe /></MemoryRouter>)
    })
    const quickBuild = container.querySelector<HTMLButtonElement>('button.row-run')
    expect(quickBuild).not.toBeNull()
    await act(async () => {
      quickBuild?.dispatchEvent(new MouseEvent('click', { bubbles: true }))
      await Promise.resolve()
    })
    await flushRequests()

    expect(quickBuild?.textContent).toMatch(/Build with Parameters|参数化构建/)
    expect(getProject).not.toHaveBeenCalled()
    expect(triggerBuild).not.toHaveBeenCalled()
    expect(container.querySelector('output[aria-label="location"]')?.textContent).toBe('/projects/4/build')
  })

  it('routes the custom build shortcut to the dedicated Jenkins form', async () => {
    const project = {
      id: 5,
      name: 'On-demand release',
      repo_type: 'git',
      default_branch: 'main',
      config: `import { definePipeline, parameter } from '@buildworld/pipeline'
export default definePipeline({ parameters: [parameter('release_channel', 'choice', { required: true, choices: ['staging', 'production'] })], stages: [] })
`,
    }
    listProjects.mockResolvedValue([{ ...project, config: undefined }])
    getProject.mockResolvedValue(project)
    validateProject.mockResolvedValue({ valid: true, format: 'typescript', stages: 0, steps: 0, parameters: [{ name: 'release_channel', type: 'choice', required: true, choices: ['staging', 'production'] }], allow_long_running: false })
    await act(async () => {
      root.render(<MemoryRouter><Projects /><LocationProbe /></MemoryRouter>)
    })
    const customBuild = container.querySelector<HTMLButtonElement>('button.custom-build')
    await act(async () => {
      customBuild?.dispatchEvent(new MouseEvent('click', { bubbles: true }))
      await Promise.resolve()
    })
    await flushRequests()

    expect(getProject).not.toHaveBeenCalled()
    expect(container.querySelector('output[aria-label="location"]')?.textContent).toBe('/projects/5/build')
  })

  it('keeps disabled jobs configurable while blocking quick and parameterized builds', async () => {
    listProjects.mockResolvedValue([{
      id: 8,
      enabled: false,
      name: 'Paused deployment',
      repo_type: 'git',
      default_branch: 'main',
    }])
    await act(async () => {
      root.render(<MemoryRouter><Projects /></MemoryRouter>)
    })
    expect(container.querySelector('a.row-settings[href="/projects/8/configure"]')).not.toBeNull()
    expect(container.querySelector<HTMLButtonElement>('button.row-run')?.disabled).toBe(true)
    expect(container.querySelector<HTMLButtonElement>('button.custom-build')?.disabled).toBe(true)
    expect(container.querySelector<HTMLButtonElement>('button.row-run')?.title).toBe('Disabled')
    expect(validateProject).not.toHaveBeenCalled()
  })

  it('renders a flat list and filters by fuzzy matches across project fields', async () => {
    listProjects.mockResolvedValue([
      {
        id: 6,
        name: 'World server',
        group_id: 8,
        description: 'Game server release lane',
        repo_url: 'https://example.invalid/world.git',
        tags: ['release'],
        repo_type: 'git',
        default_branch: 'main',
      },
      {
        id: 7,
        name: 'Unsorted tool',
        description: 'Desktop utility',
        repo_url: 'https://example.invalid/tools.git',
        tags: ['desktop'],
        repo_type: 'git',
        default_branch: 'develop',
      },
    ])
    await act(async () => {
      root.render(<MemoryRouter><Projects /></MemoryRouter>)
    })

    expect(container.querySelectorAll('table tbody tr')).toHaveLength(2)
    const search = container.querySelector<HTMLInputElement>('.project-list-search input')!
    await act(async () => {
      Object.getOwnPropertyDescriptor(HTMLInputElement.prototype, 'value')?.set?.call(search, 'release')
      search.dispatchEvent(new Event('input', { bubbles: true }))
    })
    expect(container.querySelectorAll('table tbody tr')).toHaveLength(1)
    expect(container.querySelector('a[href="/projects/6"]')?.textContent).toContain('World server')

    await act(async () => {
      Object.getOwnPropertyDescriptor(HTMLInputElement.prototype, 'value')?.set?.call(search, 'develop')
      search.dispatchEvent(new Event('input', { bubbles: true }))
    })
    expect(container.querySelectorAll('table tbody tr')).toHaveLength(1)
    expect(container.querySelector('a[href="/projects/7"]')?.textContent).toContain('Unsorted tool')
  })
})
