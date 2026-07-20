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
    triggerBuild: vi.fn(),
    deleteProject: vi.fn(),
  },
}))

const listProjects = vi.mocked(api.listProjects)
const getProject = vi.mocked(api.getProject)
const listProjectGroups = vi.mocked(api.listProjectGroups)
const triggerBuild = vi.mocked(api.triggerBuild)
const actEnvironment = globalThis as typeof globalThis & { IS_REACT_ACT_ENVIRONMENT: boolean }

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
    expect(container.querySelector('a.row-settings[href="/projects/3?view=settings"]')).not.toBeNull()
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
      config: JSON.stringify({
        parameters: [{ name: 'signing_token', type: 'password', required: true }],
      }),
    }
    listProjects.mockResolvedValue([{ ...project, config: undefined }])
    getProject.mockResolvedValue(project)
    await act(async () => {
      root.render(<MemoryRouter><Projects /></MemoryRouter>)
    })

    const quickBuild = container.querySelector<HTMLButtonElement>('button.row-run')
    expect(quickBuild).not.toBeNull()
    await act(async () => {
      quickBuild?.dispatchEvent(new MouseEvent('click', { bubbles: true }))
    })

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
      config: JSON.stringify({
        parameters: [{ name: 'release_channel', type: 'choice', required: true, choices: ['staging', 'production'] }],
      }),
    }
    listProjects.mockResolvedValue([{ ...project, config: undefined }])
    getProject.mockResolvedValue(project)
    await act(async () => {
      root.render(<MemoryRouter><Projects /></MemoryRouter>)
    })

    const customBuild = container.querySelector<HTMLButtonElement>('button.custom-build')
    await act(async () => {
      customBuild?.dispatchEvent(new MouseEvent('click', { bubbles: true }))
    })

    expect(getProject).toHaveBeenCalledWith(5)
    expect(document.body.querySelector('[role="dialog"]')?.textContent).toContain('release_channel')
  })
})
