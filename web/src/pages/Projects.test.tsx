// @vitest-environment jsdom

import { act } from 'react'
import { createRoot, type Root } from 'react-dom/client'
import { MemoryRouter } from 'react-router-dom'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { api } from '../api'
import Projects from './Projects'

vi.mock('../api', () => ({
  api: {
    listProjects: vi.fn(),
    getProject: vi.fn(),
    listProjectGroups: vi.fn(),
    validateProject: vi.fn(),
    triggerBuild: vi.fn(),
    deleteProject: vi.fn(),
  },
}))

const listProjects = vi.mocked(api.listProjects)
const getProject = vi.mocked(api.getProject)
const listProjectGroups = vi.mocked(api.listProjectGroups)
const validateProject = vi.mocked(api.validateProject)
const triggerBuild = vi.mocked(api.triggerBuild)
const actEnvironment = globalThis as typeof globalThis & { IS_REACT_ACT_ENVIRONMENT: boolean }

async function expandFolder(container: HTMLElement, contentID = 'project-folder-ungrouped') {
  const toggle = container.querySelector<HTMLButtonElement>(`button[aria-controls="${contentID}"]`)
  await act(async () => {
    toggle?.click()
  })
}

async function flushRequests() {
  await act(async () => {
    await Promise.resolve()
    await Promise.resolve()
  })
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
    listProjectGroups.mockReset()
    validateProject.mockReset()
    triggerBuild.mockReset()
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
    listProjectGroups.mockResolvedValue([])
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
    await expandFolder(container)

    expect(container.querySelector('a[href="/projects/new"]')).not.toBeNull()
    expect(container.querySelector('a.entity-link[href="/projects/3"]')?.textContent).toContain('Packaging matrix')
    expect(container.querySelector('a.row-settings[href="/projects/3/configure"]')).not.toBeNull()
    expect(container.querySelector('button.custom-build')).not.toBeNull()
    expect(container.querySelector('button.entity-link')).toBeNull()
    expect(container.querySelector('tr[tabindex]')).toBeNull()
  })

  it('opens parameter input instead of failing a quick build with unresolved required values', async () => {
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
      root.render(<MemoryRouter><Projects /></MemoryRouter>)
    })
    await expandFolder(container)

    const quickBuild = container.querySelector<HTMLButtonElement>('button.row-run')
    expect(quickBuild).not.toBeNull()
    await act(async () => {
      quickBuild?.dispatchEvent(new MouseEvent('click', { bubbles: true }))
      await Promise.resolve()
    })
    await flushRequests()

    expect(getProject).toHaveBeenCalledWith(4)
    expect(triggerBuild).not.toHaveBeenCalled()
    expect(document.body.querySelector('[role="dialog"]')?.textContent).toContain('signing_token')
  })

  it('loads the full project only when custom build is requested', async () => {
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
      root.render(<MemoryRouter><Projects /></MemoryRouter>)
    })
    await expandFolder(container)

    const customBuild = container.querySelector<HTMLButtonElement>('button.custom-build')
    await act(async () => {
      customBuild?.dispatchEvent(new MouseEvent('click', { bubbles: true }))
      await Promise.resolve()
    })
    await flushRequests()

    expect(getProject).toHaveBeenCalledWith(5)
    expect(document.body.querySelector('[role="dialog"]')?.textContent).toContain('release_channel')
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
    await expandFolder(container)

    expect(container.querySelector('a.row-settings[href="/projects/8/configure"]')).not.toBeNull()
    expect(container.querySelector<HTMLButtonElement>('button.row-run')?.disabled).toBe(true)
    expect(container.querySelector<HTMLButtonElement>('button.custom-build')?.disabled).toBe(true)
    expect(container.querySelector<HTMLButtonElement>('button.row-run')?.title).toBe('Disabled')
    expect(validateProject).not.toHaveBeenCalled()
  })

  it('starts folders collapsed, mounts tables on demand, and restores v2 expanded state', async () => {
    listProjects.mockResolvedValue([
      {
        id: 6,
        name: 'World server',
        group_id: 8,
        repo_type: 'git',
        default_branch: 'main',
      },
      {
        id: 7,
        name: 'Unsorted tool',
        repo_type: 'git',
        default_branch: 'main',
      },
    ])
    listProjectGroups.mockResolvedValue([{ id: 8, name: 'Game servers', color: 'mint' }])
    localStorage.setItem('buildworld.projects.folders', JSON.stringify({ version: 1, collapsed: [] }))
    await act(async () => {
      root.render(<MemoryRouter><Projects /></MemoryRouter>)
    })

    const folder = container.querySelector<HTMLElement>('.project-folder[data-group-color="mint"]')
    const toggle = folder?.querySelector<HTMLButtonElement>('button[aria-controls="project-folder-8"]')
    const content = container.querySelector<HTMLElement>('#project-folder-8')
    expect(folder).not.toBeNull()
    expect(toggle?.getAttribute('aria-expanded')).toBe('false')
    expect(content?.hidden).toBe(true)
    expect(content?.querySelector('table')).toBeNull()
    expect(container.querySelector('#project-folder-ungrouped table')).toBeNull()

    await act(async () => {
      toggle?.click()
    })
    expect(toggle?.getAttribute('aria-expanded')).toBe('true')
    expect(content?.hidden).toBe(false)
    expect(folder?.querySelector('a[href="/projects/6"]')?.textContent).toContain('World server')
    expect(JSON.parse(localStorage.getItem('buildworld.projects.folders') || '{}')).toEqual({
      version: 2,
      expanded: ['group:8'],
    })

    act(() => root.unmount())
    root = createRoot(container)
    await act(async () => {
      root.render(<MemoryRouter><Projects /></MemoryRouter>)
    })
    expect(container.querySelector('button[aria-controls="project-folder-8"]')?.getAttribute('aria-expanded')).toBe('true')
    expect(container.querySelector('#project-folder-8 a[href="/projects/6"]')?.textContent).toContain('World server')
    expect(container.querySelector('button[aria-controls="project-folder-ungrouped"]')?.getAttribute('aria-expanded')).toBe('false')
    expect(container.querySelector('#project-folder-ungrouped table')).toBeNull()
  })
})
