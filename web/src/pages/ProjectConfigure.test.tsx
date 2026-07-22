// @vitest-environment jsdom

import { readFileSync } from 'node:fs'
import { resolve } from 'node:path'
import { act } from 'react'
import { createRoot, type Root } from 'react-dom/client'
import { MemoryRouter, Route, Routes } from 'react-router-dom'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { api } from '../api'
import ProjectConfigure from './ProjectConfigure'

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
    vi.mocked(api.updateProject).mockResolvedValue({})
    vi.mocked(api.validatePipeline).mockResolvedValue({ valid: true })
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
      name: 'server-game-go',
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
    expect(container.querySelector('option[value="12"]')).toBeNull()

    await act(async () => {
      container.querySelector<HTMLButtonElement>('button[value="apply"]')?.click()
      await Promise.resolve()
      await Promise.resolve()
    })

    expect(api.updateProject).toHaveBeenCalledWith(42, {
      enabled: true,
      name: 'server-game-go',
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

  it('is lazy-loaded at the Jenkins configure URL in the application router', () => {
    const appSource = readFileSync(resolve(process.cwd(), 'src/App.tsx'), 'utf8')
    expect(appSource).toContain("const ProjectConfigure = lazy(() => import('./pages/ProjectConfigure'))")
    expect(appSource).toContain('<Route path="/projects/:id/configure" element={<RoleGate roles={[\'admin\', \'developer\']}><ProjectConfigure /></RoleGate>} />')
  })
})
