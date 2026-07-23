// @vitest-environment jsdom

import { act } from 'react'
import { createRoot, type Root } from 'react-dom/client'
import { MemoryRouter, Route, Routes } from 'react-router-dom'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { api } from '../api'
import CreateProject from './CreateProject'
import Credentials from './Credentials'
import ProjectConfigure from './ProjectConfigure'
import VCSRoots from './VCSRoots'

vi.mock('../authz', () => ({
  canEdit: () => true,
  isAdmin: () => true,
}))

vi.mock('../components/PipelineSourceEditor', async () => {
  const React = await import('react')
  return {
    default: ({ value, onChange, onValidationChange, ariaLabel }: any) => {
      React.useEffect(() => {
        onValidationChange?.({
          source: value,
          checking: false,
          diagnosticsReady: true,
          diagnosticErrors: 0,
          serverValid: true,
          valid: true,
          message: '',
          problems: 0,
        })
      }, [value, onValidationChange])
      return <textarea aria-label={ariaLabel} value={value} onChange={event => onChange(event.target.value)} />
    },
  }
})

vi.mock('../api', () => ({
  API_FEEDBACK_EVENT: 'buildworld:api-feedback',
  api: {
    listVCSRoots: vi.fn(),
    listTemplates: vi.fn(),
    listProjects: vi.fn(),
    listProjectGroups: vi.fn(),
    createProject: vi.fn(),
    createProjectGroup: vi.fn(),
    listCredentials: vi.fn(),
    getProject: vi.fn(),
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
    vi.mocked(api.createProject).mockResolvedValue({ id: 9 })
    vi.mocked(api.listCredentials).mockResolvedValue([])
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

  it('creates Jenkinsfile Pipeline items from Git SCM by default', async () => {
    await act(async () => {
      root.render(<MemoryRouter><CreateProject /></MemoryRouter>)
    })
    await flushRequests()

    expect(container.textContent).not.toMatch(/Subversion|Mercurial|\bSVN\b/)
    expect(container.querySelector('option[value="svn"], option[value="hg"]')).toBeNull()
    expect(api.listTemplates).not.toHaveBeenCalled()
    expect(Array.from(container.querySelectorAll('label > span')).map(node => node.textContent)).not.toContain('Template')

    const name = container.querySelector<HTMLInputElement>('#jenkins-new-item-name')
    await act(async () => {
      if (name) {
        Object.getOwnPropertyDescriptor(HTMLInputElement.prototype, 'value')?.set?.call(name, 'standalone-project')
        name.dispatchEvent(new Event('input', { bubbles: true }))
      }
      container.querySelector<HTMLInputElement>('input[value="pipeline"]')?.click()
    })
    await act(async () => {
      container.querySelector<HTMLButtonElement>('button[type="submit"]')?.click()
      await Promise.resolve()
      await Promise.resolve()
    })
    await flushRequests()

    expect(api.createProject).toHaveBeenCalledOnce()
    expect(api.createProject).toHaveBeenCalledWith(expect.objectContaining({
      name: 'standalone-project',
      repo_type: 'git',
      config: '',
      pipeline_format: 'jenkinsfile',
      pipeline_source_mode: 'scm',
      pipeline_scm_branch: 'main',
      pipeline_scm_path: 'Jenkinsfile',
    }))
    expect(vi.mocked(api.createProject).mock.calls[0]?.[0]).not.toHaveProperty('template_id')
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
        <MemoryRouter initialEntries={['/projects/7/configure']}>
          <Routes><Route path="/projects/:id/configure" element={<ProjectConfigure />} /></Routes>
        </MemoryRouter>,
      )
    })
    await flushRequests()

    const repositoryType = container.querySelector<HTMLInputElement>('#jenkins-configure-repository-type')
    expect(repositoryType?.value).toContain('Git')
    const save = container.querySelector<HTMLButtonElement>('button[value="save"]')
    expect(save?.disabled).toBe(false)
    await act(async () => {
      save?.click()
      await Promise.resolve()
      await Promise.resolve()
    })
    await flushRequests()

    expect(container.querySelector('[role="alert"]')?.textContent).toBeUndefined()
    expect(api.updateProject).toHaveBeenCalledWith(7, expect.objectContaining({ repo_type: 'git' }))
  })
})
