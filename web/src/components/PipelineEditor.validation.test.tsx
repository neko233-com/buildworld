// @vitest-environment jsdom

import { act } from 'react'
import { createRoot, type Root } from 'react-dom/client'
import { MemoryRouter } from 'react-router-dom'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import PipelineEditor from './PipelineEditor'

const validationHarness = vi.hoisted(() => ({
  diagnosticErrors: 1,
}))

vi.mock('./PipelineCodeEditor', async () => {
  const React = await import('react')
  type Props = {
    value: string
    onChange: (source: string) => void
    onValidationChange?: (state: {
      source: string
      checking: boolean
      diagnosticsReady: boolean
      diagnosticErrors: number
      serverValid: boolean
      valid: boolean
      message: string
      problems: number
    }) => void
    statusId?: string
  }
  return {
    default: ({ value, onChange, onValidationChange, statusId }: Props) => {
      React.useEffect(() => {
        onValidationChange?.({
          source: value,
          checking: false,
          diagnosticsReady: true,
          diagnosticErrors: validationHarness.diagnosticErrors,
          serverValid: true,
          // Deliberately true: consumers must still inspect the Error count.
          valid: true,
          message: validationHarness.diagnosticErrors ? 'TypeScript Error' : 'Pipeline valid',
          problems: validationHarness.diagnosticErrors,
        })
      }, [onValidationChange, value])
      return React.createElement(
        React.Fragment,
        null,
        React.createElement('button', {
          'data-testid': 'edit-source',
          onClick: () => onChange(`${value}\n// edited`),
          type: 'button',
        }, 'edit'),
        React.createElement('div', { id: statusId, role: 'status' }, 'validation status'),
      )
    },
  }
})

vi.mock('../i18n', () => ({
  useI18n: () => ({ t: (key: string) => key }),
}))

vi.mock('../api', () => ({
  api: {
    validatePipeline: vi.fn().mockResolvedValue({ valid: true, format: 'typescript' }),
  },
}))

vi.mock('./AppDialogs', () => ({
  dialogs: {
    confirm: vi.fn(),
    notify: vi.fn(),
  },
}))

const actEnvironment = globalThis as typeof globalThis & { IS_REACT_ACT_ENVIRONMENT: boolean }
const source = `import { definePipeline } from '@buildworld/pipeline'
export default definePipeline({ stages: [] })
`

describe('PipelineEditor TypeScript validation gate', () => {
  let container: HTMLDivElement
  let root: Root

  beforeEach(() => {
    actEnvironment.IS_REACT_ACT_ENVIRONMENT = true
    validationHarness.diagnosticErrors = 1
    container = document.createElement('div')
    document.body.appendChild(container)
    root = createRoot(container)
  })

  afterEach(() => {
    act(() => root.unmount())
    container.remove()
    actEnvironment.IS_REACT_ACT_ENVIRONMENT = false
  })

  async function renderEditor(onSave = vi.fn(), onRun = vi.fn()) {
    await act(async () => {
      root.render(
        <MemoryRouter>
          <PipelineEditor
            project={{ id: 1, name: 'project', config: source }}
            projects={[]}
            globalVariables={[]}
            onSave={onSave}
            onRun={onRun}
          />
        </MemoryRouter>,
      )
      await Promise.resolve()
    })
    await act(async () => {
      await Promise.resolve()
    })
    return { onSave, onRun }
  }

  function action(label: string) {
    const button = [...container.querySelectorAll('button')].find(candidate => candidate.textContent?.includes(label))
    if (!(button instanceof HTMLButtonElement)) throw new Error(`Missing ${label} button`)
    return button
  }

  it('blocks save and run when Monaco has an Error even if valid was set true', async () => {
    const { onSave, onRun } = await renderEditor()
    await act(async () => {
      ;(container.querySelector('[data-testid="edit-source"]') as HTMLButtonElement).click()
      await Promise.resolve()
    })

    expect(action('pipeline.save').disabled).toBe(true)
    expect(action('pipeline.run').disabled).toBe(true)
    expect(action('pipeline.run').getAttribute('aria-describedby')).toBe('pipeline-editor-validation-status')
    expect(onSave).not.toHaveBeenCalled()
    expect(onRun).not.toHaveBeenCalled()
  })

  it('allows warnings by enabling save and run when there are no Error diagnostics', async () => {
    validationHarness.diagnosticErrors = 0
    const { onSave } = await renderEditor()
    await act(async () => {
      ;(container.querySelector('[data-testid="edit-source"]') as HTMLButtonElement).click()
      await Promise.resolve()
    })

    expect(action('pipeline.save').disabled).toBe(false)
    expect(action('pipeline.run').disabled).toBe(false)
    await act(async () => {
      action('pipeline.save').click()
      await Promise.resolve()
    })
    expect(onSave).toHaveBeenCalledWith(expect.stringContaining('// edited'))
  })
})
