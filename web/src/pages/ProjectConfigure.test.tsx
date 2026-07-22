// @vitest-environment jsdom

import { readFileSync } from 'node:fs'
import { resolve } from 'node:path'
import { act } from 'react'
import { createRoot, type Root } from 'react-dom/client'
import { MemoryRouter, Route, Routes } from 'react-router-dom'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { api } from '../api'
import { dialogs } from '../components/AppDialogs'
import ProjectConfigure from './ProjectConfigure'

vi.mock('../components/AppDialogs', () => ({ dialogs: { confirm: vi.fn(), notify: vi.fn() } }))

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
  api: {
    getProject: vi.fn(),
    listProjectGroups: vi.fn(),
    listVCSRoots: vi.fn(),
    listTemplates: vi.fn(),
    updateProject: vi.fn(),
    validatePipeline: vi.fn(),
    migratePipeline: vi.fn(),
  },
}))

const actEnvironment = globalThis as typeof globalThis & { IS_REACT_ACT_ENVIRONMENT: boolean }

async function flushRequests() {
  await act(async () => {
    await Promise.resolve()
    await Promise.resolve()
    await Promise.resolve()
  })
}

function changeControl(control: HTMLInputElement | HTMLTextAreaElement, value: string) {
  const prototype = control instanceof HTMLTextAreaElement ? HTMLTextAreaElement.prototype : HTMLInputElement.prototype
  Object.getOwnPropertyDescriptor(prototype, 'value')?.set?.call(control, value)
  control.dispatchEvent(new Event('input', { bubbles: true }))
}

describe('ProjectConfigure', () => {
  let container: HTMLDivElement
  let root: Root

  beforeEach(() => {
    actEnvironment.IS_REACT_ACT_ENVIRONMENT = true
    const payload = btoa(JSON.stringify({ role: 'admin' }))
    localStorage.setItem('token', `test.${payload}.signature`)
    container = document.createElement('div')
    document.body.appendChild(container)
    root = createRoot(container)
    vi.mocked(api.listProjectGroups).mockResolvedValue([{ id: 13, name: 'Servers' }])
    vi.mocked(api.listVCSRoots).mockResolvedValue([{ id: 11, name: 'Jenkins Git' }])
    vi.mocked(api.listTemplates).mockResolvedValue([{ id: 12, name: 'Shared pipeline', config: 'jobs:\n  build:\n    steps: []\n' }])
    vi.mocked(api.updateProject).mockResolvedValue({})
    vi.mocked(api.validatePipeline).mockResolvedValue({ valid: true })
    vi.mocked(dialogs.confirm).mockResolvedValue(true)
  })

  afterEach(() => {
    act(() => root.unmount())
    container.remove()
    localStorage.clear()
    vi.clearAllMocks()
    actEnvironment.IS_REACT_ACT_ENVIRONMENT = false
  })

  it('round-trips Jenkinsfile SCM metadata through Apply without requiring an inline config', async () => {
    vi.mocked(api.getProject).mockResolvedValue({
      id: 42,
      name: 'weather-service',
      description: 'GAME Server',
      repo_url: 'https://git.example.test/game.git',
      repo_type: 'git',
      default_branch: 'main',
      group_id: 13,
      tags: ['go', 'production'],
      config: '',
      vcs_root_id: 11,
      template_id: 12,
      pipeline_format: 'jenkinsfile',
      pipeline_source_mode: 'scm',
      pipeline_scm_repo: 'ssh://git.example.test/ci.git',
      pipeline_scm_branch: 'release',
      pipeline_scm_path: 'ci/Jenkinsfile',
    })

    await act(async () => {
      root.render(<MemoryRouter initialEntries={['/projects/42/configure']}><Routes><Route path="/projects/:id/configure" element={<ProjectConfigure />} /></Routes></MemoryRouter>)
    })
    await flushRequests()

    expect(container.querySelector('h1')?.textContent).toBe('Configure')
    expect(container.querySelector('.jenkins-configure-nav a[href="#jenkins-configure-pipeline"]')).not.toBeNull()
    expect(container.textContent).toContain('Pipeline script from SCM')
    expect(container.querySelector('[aria-label="Build flow source"]')).toBeNull()
    expect(container.querySelector('option[value="12"]')?.textContent).toBe('Shared pipeline')
    expect(container.querySelector<HTMLSelectElement>('#jenkins-configure-template')?.value).toBe('12')

    await act(async () => {
      container.querySelector<HTMLButtonElement>('button[value="apply"]')?.click()
      await Promise.resolve()
      await Promise.resolve()
    })

    expect(api.updateProject).toHaveBeenCalledWith(42, {
      enabled: true,
      name: 'weather-service',
      description: 'GAME Server',
      repo_url: 'https://git.example.test/game.git',
      repo_type: 'git',
      default_branch: 'main',
      vcs_root_id: 11,
      template_id: 12,
      group_id: 13,
      tags: ['go', 'production'],
      config: '',
      pipeline_format: 'jenkinsfile',
      pipeline_source_mode: 'scm',
      pipeline_scm_repo: 'ssh://git.example.test/ci.git',
      pipeline_scm_branch: 'release',
      pipeline_scm_path: 'ci/Jenkinsfile',
    })
  })

  it('lets editors change the linked build template and persists template_id', async () => {
    vi.mocked(api.listTemplates).mockResolvedValue([
      { id: 12, name: 'Shared pipeline', config: 'jobs:\n  build:\n    steps: []\n' },
      { id: 18, name: 'Release pipeline', config: 'jobs:\n  release:\n    steps: []\n' },
    ])
    vi.mocked(api.getProject).mockResolvedValue({
      id: 47,
      enabled: true,
      name: 'template-link-job',
      description: '',
      repo_url: '',
      repo_type: 'git',
      default_branch: 'main',
      tags: [],
      config: 'jobs:\n  build:\n    steps: []\n',
      template_id: 12,
      pipeline_format: 'yaml',
      pipeline_source_mode: 'inline',
      pipeline_scm_repo: '',
      pipeline_scm_branch: '',
      pipeline_scm_path: '',
    })

    await act(async () => {
      root.render(<MemoryRouter initialEntries={['/projects/47/configure']}><Routes><Route path="/projects/:id/configure" element={<ProjectConfigure />} /></Routes></MemoryRouter>)
    })
    await flushRequests()

    const template = container.querySelector<HTMLSelectElement>('#jenkins-configure-template')!
    await act(async () => {
      Object.getOwnPropertyDescriptor(HTMLSelectElement.prototype, 'value')?.set?.call(template, '18')
      template.dispatchEvent(new Event('change', { bubbles: true }))
    })
    await act(async () => {
      container.querySelector<HTMLButtonElement>('button[value="apply"]')?.click()
      await Promise.resolve()
      await Promise.resolve()
    })

    expect(api.updateProject).toHaveBeenCalledWith(47, expect.objectContaining({ template_id: 18 }))
  })

  it('persists the Jenkins Enabled switch with Apply', async () => {
    vi.mocked(api.getProject).mockResolvedValue({
      id: 44,
      enabled: true,
      name: 'toggle-job',
      description: '',
      repo_url: '',
      repo_type: 'git',
      default_branch: 'main',
      tags: [],
      config: 'jobs:\n  build:\n    steps: []\n',
      pipeline_format: 'yaml',
      pipeline_source_mode: 'inline',
      pipeline_scm_repo: '',
      pipeline_scm_branch: '',
      pipeline_scm_path: '',
    })

    await act(async () => {
      root.render(<MemoryRouter initialEntries={['/projects/44/configure']}><Routes><Route path="/projects/:id/configure" element={<ProjectConfigure />} /></Routes></MemoryRouter>)
    })
    await flushRequests()

    const enabled = container.querySelector<HTMLInputElement>('input[role="switch"]')
    expect(enabled?.checked).toBe(true)
    await act(async () => enabled?.click())
    await act(async () => {
      container.querySelector<HTMLButtonElement>('button[value="apply"]')?.click()
      await Promise.resolve()
      await Promise.resolve()
    })

    expect(vi.mocked(api.updateProject).mock.calls[0]?.[1]).toMatchObject({ enabled: false })
  })

  it('offers only the supported inline schedule trigger and removes it from YAML when disabled', async () => {
    vi.mocked(api.getProject).mockResolvedValue({
      id: 43,
      name: 'yaml-job',
      description: '',
      repo_url: 'https://git.example.test/yaml.git',
      repo_type: 'git',
      default_branch: 'main',
      tags: [],
      config: `on:\n  schedule:\n    - cron: 15 3 * * *\njobs:\n  build:\n    steps:\n      - run: echo ok\n`,
      pipeline_format: 'yaml',
      pipeline_source_mode: 'inline',
      pipeline_scm_repo: 'https://git.example.test/dormant.git',
      pipeline_scm_branch: 'legacy',
      pipeline_scm_path: 'ci/legacy.yaml',
    })

    await act(async () => {
      root.render(<MemoryRouter initialEntries={['/projects/43/configure']}><Routes><Route path="/projects/:id/configure" element={<ProjectConfigure />} /></Routes></MemoryRouter>)
    })
    await flushRequests()

    const schedule = container.querySelector<HTMLInputElement>('.jenkins-configure-trigger input[type="checkbox"]')
    expect(schedule?.checked).toBe(true)
    expect(container.querySelector<HTMLInputElement>('.jenkins-configure-trigger input[required]')?.value).toBe('15 3 * * *')
    await act(async () => {
      schedule?.click()
    })
    await flushRequests()
    await act(async () => {
      container.querySelector<HTMLButtonElement>('button[value="apply"]')?.click()
      await Promise.resolve()
      await Promise.resolve()
    })

    const payload = vi.mocked(api.updateProject).mock.calls[0]?.[1]
    expect(payload.config).not.toContain('schedule')
    expect(payload).toMatchObject({
      pipeline_format: 'yaml',
      pipeline_source_mode: 'inline',
      pipeline_scm_repo: 'https://git.example.test/dormant.git',
      pipeline_scm_branch: 'legacy',
      pipeline_scm_path: 'ci/legacy.yaml',
    })
  })

  it('warns before leaving dirty settings, confirms Cancel, and clears dirty state after Apply', async () => {
    vi.mocked(api.getProject).mockResolvedValue({
      id: 45,
      enabled: true,
      name: 'weather-service',
      description: 'Current description',
      repo_url: 'https://git.example.test/weather.git',
      repo_type: 'git',
      default_branch: 'main',
      tags: [],
      config: 'jobs:\n  build:\n    steps: []\n',
      pipeline_format: 'yaml',
      pipeline_source_mode: 'inline',
      pipeline_scm_repo: '',
      pipeline_scm_branch: '',
      pipeline_scm_path: '',
    })

    await act(async () => {
      root.render(<MemoryRouter initialEntries={['/projects/45/configure']}><Routes><Route path="/projects/:id/configure" element={<ProjectConfigure />} /><Route path="/projects/:id" element={<div data-project-page>Project page</div>} /></Routes></MemoryRouter>)
    })
    await flushRequests()

    expect(container.querySelector('.jenkins-configure-save-state')?.textContent).toBe('Saved')
    const description = container.querySelector<HTMLTextAreaElement>('#jenkins-configure-description')
    await act(async () => {
      if (!description) return
      changeControl(description, 'Updated description')
    })
    expect(container.querySelector('.jenkins-configure-save-state')?.textContent).toBe('Unsaved changes')

    const beforeUnload = new Event('beforeunload', { cancelable: true })
    window.dispatchEvent(beforeUnload)
    expect(beforeUnload.defaultPrevented).toBe(true)

    vi.mocked(dialogs.confirm).mockResolvedValueOnce(false)
    await act(async () => {
      container.querySelector<HTMLButtonElement>('.jenkins-configure-actions button[type="button"]')?.click()
      await Promise.resolve()
    })
    expect(dialogs.confirm).toHaveBeenCalledWith(expect.stringContaining('discard'), expect.objectContaining({ title: expect.stringContaining('Discard') }))
    expect(container.querySelector('[data-project-page]')).toBeNull()

    await act(async () => {
      container.querySelector<HTMLButtonElement>('button[value="apply"]')?.click()
      await Promise.resolve()
      await Promise.resolve()
    })
    await flushRequests()
    expect(container.querySelector('.jenkins-configure-save-state')?.textContent).toBe('Saved')
    const afterApplyUnload = new Event('beforeunload', { cancelable: true })
    window.dispatchEvent(afterApplyUnload)
    expect(afterApplyUnload.defaultPrevented).toBe(false)
  })

  it('focuses the first invalid field and exposes its error state', async () => {
    vi.mocked(api.getProject).mockResolvedValue({
      id: 46,
      enabled: true,
      name: 'weather-service',
      description: '',
      repo_url: '',
      repo_type: 'git',
      default_branch: 'main',
      tags: [],
      config: 'jobs:\n  build:\n    steps: []\n',
      pipeline_format: 'yaml',
      pipeline_source_mode: 'inline',
      pipeline_scm_repo: '',
      pipeline_scm_branch: '',
      pipeline_scm_path: '',
    })

    await act(async () => {
      root.render(<MemoryRouter initialEntries={['/projects/46/configure']}><Routes><Route path="/projects/:id/configure" element={<ProjectConfigure />} /></Routes></MemoryRouter>)
    })
    await flushRequests()

    const name = container.querySelector<HTMLInputElement>('#jenkins-configure-name')
    await act(async () => {
      if (!name) return
      changeControl(name, '')
      container.querySelector<HTMLButtonElement>('button[value="apply"]')?.click()
      await Promise.resolve()
    })
    await flushRequests()

    expect(api.updateProject).not.toHaveBeenCalled()
    expect(name?.getAttribute('aria-invalid')).toBe('true')
    expect(name?.getAttribute('aria-describedby')).toBe('jenkins-configure-form-error')
    expect(document.activeElement).toBe(name)
  })

  it('is lazy-loaded at the Jenkins configure URL in the application router', () => {
    const appSource = readFileSync(resolve(process.cwd(), 'src/App.tsx'), 'utf8')
    expect(appSource).toContain("const ProjectConfigure = lazy(() => import('./pages/ProjectConfigure'))")
    expect(appSource).toContain('<Route path="/projects/:id/configure" element={<RoleGate roles={[\'admin\', \'developer\']}><ProjectConfigure /></RoleGate>} />')
  })
})
