// @vitest-environment jsdom

import { act } from 'react'
import { createRoot, type Root } from 'react-dom/client'
import { MemoryRouter } from 'react-router-dom'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import type { BuildProblemReport } from '../lib/buildProblems'
import BuildProblemsPanel, { type BuildProblemsPanelProps } from './BuildProblemsPanel'

const actEnvironment = globalThis as typeof globalThis & { IS_REACT_ACT_ENVIRONMENT: boolean }

const report: BuildProblemReport = {
  version: 1,
  build_status: 'failed',
  summary: 'Compiler failed',
  failed_step_count: 1,
  problems: [{
    id: 'problem-1',
    code: 'compile_failed',
    severity: 'error',
    stage: 'Build',
    step: 'Compile',
    message: '<img src=x onerror="alert(1)">',
    excerpt: '<script>alert(1)</script>',
    line: 9,
    suggested_action: 'project_settings',
  }],
}

describe('BuildProblemsPanel', () => {
  let container: HTMLDivElement
  let root: Root

  beforeEach(() => {
    actEnvironment.IS_REACT_ACT_ENVIRONMENT = true
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

  function render(props: Partial<BuildProblemsPanelProps> = {}) {
    act(() => root.render(<MemoryRouter><BuildProblemsPanel
      buildId={57}
      projectId={4}
      {...props}
    /></MemoryRouter>))
  }

  it('shows distinct loading, failure, and empty states', () => {
    render({ loading: true })
    expect(container.querySelector('[role="status"]')).not.toBeNull()
    expect(container.querySelector('section')?.getAttribute('aria-busy')).toBe('true')

    const reload = vi.fn()
    render({ error: '<b>network down</b>', onReload: reload })
    expect(container.querySelector('[role="alert"]')?.textContent).toContain('<b>network down</b>')
    expect(container.querySelector('b')).toBeNull()
    act(() => container.querySelector<HTMLButtonElement>('button')!.click())
    expect(reload).toHaveBeenCalledOnce()

    render({ report: { ...report, problems: [], failed_step_count: 0 } })
    expect(container.querySelector('[role="status"]')).not.toBeNull()
    expect(container.querySelector('ol')).toBeNull()
  })

  it('renders diagnostics only as text and exposes explicit actions', () => {
    const retry = vi.fn()
    render({ report, editable: true, onRetry: retry })

    expect(container.textContent).toContain('<img src=x onerror="alert(1)">')
    expect(container.textContent).toContain('<script>alert(1)</script>')
    expect(container.querySelector('img')).toBeNull()
    expect(container.querySelector('script')).toBeNull()
    expect(container.querySelector<HTMLAnchorElement>('a[href="/builds/57/logs"]')).not.toBeNull()
    expect(container.querySelector<HTMLAnchorElement>('a[href="/projects/4/configure"]')).not.toBeNull()

    act(() => container.querySelector<HTMLButtonElement>('.build-problem-actions button')!.click())
    expect(retry).toHaveBeenCalledOnce()
  })
})
