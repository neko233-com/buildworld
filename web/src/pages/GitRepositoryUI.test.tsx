// @vitest-environment jsdom

import { act } from 'react'
import { createRoot, type Root } from 'react-dom/client'
import { MemoryRouter, Route, Routes } from 'react-router-dom'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { api } from '../api'
import CreateProject from './CreateProject'
import Credentials from './Credentials'
import ProjectDetail from './ProjectDetail'
import VCSRoots from './VCSRoots'

vi.mock('../authz', () => ({
  canEdit: () => true,
  isAdmin: () => true,
}))

vi.mock('../api', () => ({
  API_FEEDBACK_EVENT: 'buildworld:api-feedback',
  api: {
    listVCSRoots: vi.fn(),
    listTemplates: vi.fn(),
    listProjects: vi.fn(),
    listProjectGroups: vi.fn(),
    listCredentials: vi.fn(),
    getProject: vi.fn(),
    listProjectBuilds: vi.fn(),
    listEnvVars: vi.fn(),
    updateProject: vi.fn(),
    validatePipeline: vi.fn(),
  },
}))

const actEnvironment = globalThis as typeof globalThis & { IS_REACT_ACT_ENVIRONMENT: boolean }
const pipeline = `jobs:
  build:
    steps:
      - name: Compile
        run: echo ok
`

async function flushRequests() {
  await act(async () => {
    await Promise.resolve()
    await Promise.resolve()
  })
}

describe('Git-only repository UI', () => {
  let container: HTMLDivElement
  let root: Root

  beforeEach(() => {
    actEnvironment.IS_REACT_ACT_ENVIRONMENT = true
    container = document.createElement('div')
    document.body.appendChild(container)
    root = createRoot(container)
    vi.mocked(api.listVCSRoots).mockResolvedValue([{ id: 1, name: 'Main Git', type: 'git', url: 'https://example.test/main.git', branch: 'main', poll_interval: 60, auto_checkout: true }])
    vi.mocked(api.listTemplates).mockResolvedValue([])
    vi.mocked(api.listProjects).mockResolvedValue([])
    vi.mocked(api.listProjectGroups).mockResolvedValue([])
    vi.mocked(api.listCredentials).mockResolvedValue([])
    vi.mocked(api.listProjectBuilds).mockResolvedValue([])
    vi.mocked(api.listEnvVars).mockResolvedValue([])
    vi.mocked(api.validatePipeline).mockResolvedValue({ valid: true, format: 'yaml', stages: 1, steps: 1, parameters: [], allow_long_running: false })
    vi.mocked(api.updateProject).mockResolvedValue({})
  })

  afterEach(() => {
    act(() => root.unmount())
    container.remove()
    localStorage.clear()
    vi.clearAllMocks()
    vi.useRealTimers()
    actEnvironment.IS_REACT_ACT_ENVIRONMENT = false
  })

  it('creates projects with a read-only Git repository type', async () => {
    await act(async () => {
      root.render(<MemoryRouter><CreateProject /></MemoryRouter>)
    })
    await flushRequests()

    const repositoryType = container.querySelector<HTMLInputElement>('input[aria-readonly="true"]')
    expect(repositoryType?.value).toContain('Git')
    expect(container.textContent).not.toMatch(/Subversion|Mercurial|\bSVN\b/)
    expect(container.querySelector('option[value="svn"], option[value="hg"]')).toBeNull()
  })

  it('edits repository templates without unsupported type choices', async () => {
    await act(async () => {
      root.render(<VCSRoots />)
    })
    await flushRequests()
    await act(async () => {
      container.querySelector<HTMLButtonElement>('.primary-command')?.click()
    })

    const repositoryType = document.querySelector<HTMLInputElement>('.vcs-editor input[aria-readonly="true"]')
    expect(repositoryType?.value).toContain('Git')
    expect(document.body.textContent).not.toMatch(/Subversion|Mercurial|\bSVN\b/)
    expect(document.querySelector('option[value="svn"], option[value="hg"]')).toBeNull()
  })

  it('offers only SSH key and Git credentials', async () => {
    await act(async () => {
      root.render(<Credentials />)
    })
    await flushRequests()
    await act(async () => {
      container.querySelector<HTMLButtonElement>('.primary-command')?.click()
    })

    const credentialOptions = [...document.querySelectorAll<HTMLSelectElement>('.credential-editor select')]
      .flatMap(select => [...select.options].map(option => option.value))
    expect(credentialOptions).toEqual(['ssh_key', 'git'])
    expect(document.body.textContent).not.toMatch(/Subversion|Mercurial/)
  })

  it('repairs an unsupported project type to Git when settings are saved', async () => {
    vi.useFakeTimers()
    vi.mocked(api.getProject).mockResolvedValue({
      id: 7,
      name: 'Imported project',
      description: '',
      repo_url: 'https://example.test/imported.git',
      repo_type: 'svn',
      default_branch: 'main',
      tags: [],
      config: pipeline,
    })
    await act(async () => {
      root.render(
        <MemoryRouter initialEntries={['/projects/7?view=settings']}>
          <Routes><Route path="/projects/:id" element={<ProjectDetail />} /></Routes>
        </MemoryRouter>,
      )
    })
    await flushRequests()
    await act(async () => {
      await vi.advanceTimersByTimeAsync(500)
    })

    const repositoryType = container.querySelector<HTMLInputElement>('#project-settings-repository-type')
    expect(repositoryType?.value).toContain('Git')
    await act(async () => {
      container.querySelector<HTMLFormElement>('.project-settings-form')?.dispatchEvent(new Event('submit', { bubbles: true, cancelable: true }))
      await Promise.resolve()
      await Promise.resolve()
    })

    expect(api.updateProject).toHaveBeenCalledWith(7, expect.objectContaining({ repo_type: 'git' }))
  })
})
