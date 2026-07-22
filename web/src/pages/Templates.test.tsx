// @vitest-environment jsdom

import { act, type ReactNode } from 'react'
import { createRoot, type Root } from 'react-dom/client'
import { MemoryRouter, useLocation } from 'react-router-dom'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { api } from '../api'
import { dialogs } from '../components/AppDialogs'
import Templates from './Templates'

vi.mock('motion/react', () => ({
  motion: { section: ({ children, initial: _initial, animate: _animate, transition: _transition, ...props }: { children: ReactNode } & Record<string, unknown>) => <section {...props}>{children}</section> },
}))

vi.mock('../authz', () => ({ canEdit: () => true }))
vi.mock('../components/AppDialogs', () => ({ dialogs: { confirm: vi.fn(), notify: vi.fn() } }))
vi.mock('../components/PipelineSourceEditor', async () => {
  const React = await import('react')
  return {
    default: ({ value, onChange, onValidationChange, ariaLabel, readOnly }: any) => {
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
      return <textarea aria-label={ariaLabel} readOnly={readOnly} value={value} onChange={event => onChange(event.target.value)} />
    },
  }
})
vi.mock('../api', () => ({
  api: {
    listTemplates: vi.fn(),
    listProjects: vi.fn(),
    createTemplate: vi.fn(),
    updateTemplate: vi.fn(),
    deleteTemplate: vi.fn(),
    validatePipeline: vi.fn(),
  },
}))

const actEnvironment = globalThis as typeof globalThis & { IS_REACT_ACT_ENVIRONMENT: boolean }
const pipeline = `import { definePipeline, shell, stage } from '@buildworld/pipeline'

export default definePipeline({ stages: [stage('Build', [shell('Compile', 'echo ok')])] })
`

function LocationProbe() {
  const location = useLocation()
  return <output data-location>{location.pathname}{location.search}</output>
}

async function flushRequests() {
  await act(async () => {
    await Promise.resolve()
    await Promise.resolve()
    await Promise.resolve()
  })
}

function changeInput(input: HTMLInputElement, value: string) {
  Object.getOwnPropertyDescriptor(HTMLInputElement.prototype, 'value')?.set?.call(input, value)
  input.dispatchEvent(new Event('input', { bubbles: true }))
}

describe('Build Templates workflow', () => {
  let container: HTMLDivElement
  let breadcrumbHost: HTMLDivElement
  let root: Root

  beforeEach(() => {
    actEnvironment.IS_REACT_ACT_ENVIRONMENT = true
    document.documentElement.dataset.skin = 'jenkins'
    breadcrumbHost = document.createElement('div')
    breadcrumbHost.id = 'jenkins-header-breadcrumbs'
    document.body.appendChild(breadcrumbHost)
    container = document.createElement('div')
    document.body.appendChild(container)
    root = createRoot(container)
    vi.spyOn(window, 'requestAnimationFrame').mockImplementation(callback => { callback(0); return 1 })
    vi.spyOn(window, 'cancelAnimationFrame').mockImplementation(() => undefined)
    vi.mocked(api.listTemplates).mockResolvedValue([{ id: 3, name: 'Go server', description: 'Build a Go service', config: pipeline, created_at: '2026-07-22T00:00:00Z' }])
    vi.mocked(api.listProjects).mockResolvedValue([{ id: 9, template_id: 3 }])
    vi.mocked(api.createTemplate).mockResolvedValue({ id: 4 })
    vi.mocked(api.updateTemplate).mockResolvedValue({ id: 3 })
    vi.mocked(api.deleteTemplate).mockResolvedValue({ status: 'ok' })
    vi.mocked(api.validatePipeline).mockResolvedValue({ valid: true })
    vi.mocked(dialogs.confirm).mockResolvedValue(true)
  })

  afterEach(() => {
    act(() => root.unmount())
    container.remove()
    breadcrumbHost.remove()
    document.documentElement.removeAttribute('data-skin')
    document.querySelectorAll('.schedule-modal-backdrop').forEach(node => node.remove())
    vi.clearAllMocks()
    vi.restoreAllMocks()
    actEnvironment.IS_REACT_ACT_ENVIRONMENT = false
  })

  async function renderPage() {
    await act(async () => {
      root.render(<MemoryRouter initialEntries={['/templates']}><Templates /><LocationProbe /></MemoryRouter>)
    })
    await flushRequests()
  }

  it('renders real API data and sends Use to Jenkins New Item with template id', async () => {
    await renderPage()

    expect(breadcrumbHost.textContent).toMatch(/Build Templates|构建模板/)
    expect(container.querySelector('.jenkins-management-page')).not.toBeNull()
    expect(container.textContent).toContain('Go server')
    expect(container.querySelector('.operations-heading p')?.textContent).toMatch(/1.*1/)
    await act(async () => container.querySelector<HTMLButtonElement>('.row-run')?.click())
    expect(container.querySelector('[data-location]')?.textContent).toBe('/projects/new?template=3')
  })

  it('creates a validated template, keeps modal focus contained, and restores trigger focus', async () => {
    await renderPage()
    const trigger = container.querySelector<HTMLButtonElement>('.primary-command')!
    trigger.focus()
    await act(async () => trigger.click())

    const dialog = document.querySelector<HTMLElement>('.template-editor')!
    const name = dialog.querySelector<HTMLInputElement>('input[data-dialog-initial-focus]')!
    expect(document.activeElement).toBe(name)
    await act(async () => changeInput(name, 'Release pipeline'))
    await act(async () => {
      dialog.querySelector<HTMLButtonElement>('button[type="submit"]')?.click()
      await Promise.resolve()
      await Promise.resolve()
    })

    expect(api.validatePipeline).toHaveBeenCalled()
    expect(api.createTemplate).toHaveBeenCalledWith(expect.objectContaining({ name: 'Release pipeline' }))
    expect(dialog.isConnected).toBe(false)
    await act(async () => { await new Promise(resolve => setTimeout(resolve, 0)) })
    expect(document.activeElement).toBe(trigger)
    expect(dialogs.notify).toHaveBeenCalledWith(expect.any(String), 'success')
  })

  it('edits through PUT and reports backend save failures without closing the editor', async () => {
    vi.mocked(api.updateTemplate).mockRejectedValue(new Error('template name already exists'))
    await renderPage()
    await act(async () => container.querySelector<HTMLButtonElement>('.entity-link')?.click())
    await flushRequests()
    const dialog = document.querySelector<HTMLElement>('.template-editor')!
    const name = dialog.querySelector<HTMLInputElement>('input[data-dialog-initial-focus]')!
    await act(async () => changeInput(name, 'Duplicate'))
    expect(dialog.querySelector<HTMLButtonElement>('button[type="submit"]')?.disabled).toBe(false)
    await act(async () => {
      dialog.querySelector<HTMLButtonElement>('button[type="submit"]')?.click()
      await Promise.resolve()
      await Promise.resolve()
    })

    expect(api.validatePipeline).toHaveBeenCalled()
    expect(api.updateTemplate).toHaveBeenCalledWith(3, expect.objectContaining({ name: 'Duplicate' }))
    expect(dialog.querySelector('[role="alert"]')?.textContent).toContain('template name already exists')
    expect(dialog.isConnected).toBe(true)
  })

  it('names destructive confirmation, shows busy state, and surfaces delete errors', async () => {
    let rejectDelete: ((reason: Error) => void) | undefined
    vi.mocked(api.deleteTemplate).mockImplementation(() => new Promise((_resolve, reject) => { rejectDelete = reject }))
    await renderPage()
    const remove = container.querySelector<HTMLButtonElement>('.row-icon.danger')!
    await act(async () => {
      remove.click()
      await Promise.resolve()
    })

    expect(dialogs.confirm).toHaveBeenCalledWith(expect.stringContaining('Go server'), expect.any(Object))
    expect(container.querySelector('tr[aria-busy="true"]')).not.toBeNull()
    expect(remove.disabled).toBe(true)
    await act(async () => {
      rejectDelete?.(new Error('template is used by project'))
      await Promise.resolve()
      await Promise.resolve()
    })
    expect(container.querySelector('[role="alert"]')?.textContent).toContain('template is used by project')
  })

  it('renders empty and retryable load states', async () => {
    vi.mocked(api.listTemplates).mockRejectedValueOnce(new Error('template API offline')).mockResolvedValueOnce([])
    vi.mocked(api.listProjects).mockResolvedValue([])
    await renderPage()

    expect(container.querySelector('[role="alert"]')?.textContent).toContain('template API offline')
    await act(async () => {
      container.querySelector<HTMLButtonElement>('.secondary-command')?.click()
      await Promise.resolve()
      await Promise.resolve()
    })
    expect(container.querySelector('[role="alert"]')).toBeNull()
    expect(container.querySelector('.operations-empty')?.textContent).toMatch(/No templates|暂无模板/)
  })
})
