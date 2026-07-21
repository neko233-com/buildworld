// @vitest-environment jsdom

import { act } from 'react'
import { createRoot, type Root } from 'react-dom/client'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import PipelineCodeEditor from './PipelineCodeEditor'

type TestMarker = {
  owner: string
  severity: number
  message: string
  startLineNumber?: number
  startColumn?: number
  endLineNumber?: number
  endColumn?: number
}

const apiMocks = vi.hoisted(() => ({
  validatePipeline: vi.fn(),
}))

const i18nMocks = vi.hoisted(() => ({
  t: (key: string) => ({
    'config.invalid': 'Pipeline invalid.',
    'config.validated': 'Pipeline valid.',
    'config.validating': 'Checking pipeline...',
    'config.problems': 'problems',
  })[key] || key,
}))

const monacoHarness = vi.hoisted(() => {
  const state: {
    markers: TestMarker[]
    decorationListener?: () => void
  } = { markers: [] }
  const model = {
    uri: { toString: () => 'file:///buildworld/pipeline-test.ts' },
    getLineMaxColumn: () => 80,
  }
  const editor = {
    getModel: () => model,
    getPosition: () => ({ lineNumber: 1, column: 1 }),
    onDidChangeCursorPosition: vi.fn(),
    onDidChangeModelDecorations: vi.fn((listener: () => void) => {
      state.decorationListener = listener
      return { dispose: vi.fn() }
    }),
  }
  const monaco = {
    MarkerSeverity: { Warning: 4, Error: 8 },
    ModuleKind: undefined,
    editor: {
      getModelMarkers: vi.fn(() => state.markers),
      setModelMarkers: vi.fn((_model: unknown, owner: string, markers: TestMarker[]) => {
        state.markers = [
          ...state.markers.filter(marker => marker.owner !== owner),
          ...markers.map(marker => ({ ...marker, owner })),
        ]
        state.decorationListener?.()
      }),
    },
    languages: {
      typescript: {
        ModuleKind: { ESNext: 99 },
        ModuleResolutionKind: { NodeJs: 2 },
        ScriptTarget: { ES2020: 7 },
        typescriptDefaults: {
          setCompilerOptions: vi.fn(),
          setDiagnosticsOptions: vi.fn(),
          addExtraLib: vi.fn(),
        },
      },
    },
  }
  return { editor, model, monaco, state }
})

vi.mock('../api', () => ({
  api: {
    validatePipeline: apiMocks.validatePipeline,
  },
}))

vi.mock('../i18n', () => ({
  useI18n: () => ({ t: i18nMocks.t }),
}))

vi.mock('../../../sdk/pipeline/index.d.ts?raw', () => ({
  default: 'declare module "@buildworld/pipeline" {}',
}))

vi.mock('monaco-editor/esm/vs/editor/editor.main.js', () => ({}))

vi.mock('monaco-editor/esm/vs/editor/editor.api.js', () => ({}))

vi.mock('monaco-editor/esm/vs/editor/editor.worker?worker', () => ({
  default: class EditorWorker {},
}))

vi.mock('monaco-editor/esm/vs/language/typescript/ts.worker?worker', () => ({
  default: class TypeScriptWorker {},
}))

vi.mock('@monaco-editor/react', async () => {
  const React = await import('react')
  type MockEditorProps = {
    beforeMount?: (monaco: typeof monacoHarness.monaco) => void
    onMount?: (editor: typeof monacoHarness.editor, monaco: typeof monacoHarness.monaco) => void
  }
  const MockEditor = ({ beforeMount, onMount }: MockEditorProps) => {
    const mounted = React.useRef(false)
    React.useEffect(() => {
      if (mounted.current) return
      mounted.current = true
      beforeMount?.(monacoHarness.monaco)
      onMount?.(monacoHarness.editor, monacoHarness.monaco)
    }, [beforeMount, onMount])
    return React.createElement('div', { 'data-testid': 'monaco-editor' })
  }
  return {
    default: MockEditor,
    loader: { config: vi.fn() },
  }
})

const actEnvironment = globalThis as typeof globalThis & { IS_REACT_ACT_ENVIRONMENT: boolean }

describe('PipelineCodeEditor diagnostics', () => {
  let container: HTMLDivElement
  let root: Root

  beforeEach(() => {
    actEnvironment.IS_REACT_ACT_ENVIRONMENT = true
    vi.useFakeTimers()
    apiMocks.validatePipeline.mockReset()
    monacoHarness.state.markers = []
    monacoHarness.state.decorationListener = undefined
    container = document.createElement('div')
    document.body.appendChild(container)
    root = createRoot(container)
  })

  afterEach(() => {
    act(() => root.unmount())
    container.remove()
    vi.clearAllTimers()
    vi.useRealTimers()
    actEnvironment.IS_REACT_ACT_ENVIRONMENT = false
  })

  async function renderWithMarkers(markers: TestMarker[]) {
    monacoHarness.state.markers = markers
    apiMocks.validatePipeline.mockResolvedValue({ valid: true })
    const onValidationChange = vi.fn()
    await act(async () => {
      root.render(
        <PipelineCodeEditor
          ariaLabel="Pipeline source"
          value="export default definePipeline({ stages: [] })"
          onChange={vi.fn()}
          onValidationChange={onValidationChange}
        />,
      )
    })
    await act(async () => {
      await vi.advanceTimersByTimeAsync(500)
    })
    await act(async () => {
      await Promise.resolve()
    })
    return onValidationChange
  }

  it('does not block a server-valid pipeline for Monaco warnings', async () => {
    const onValidationChange = await renderWithMarkers([{
      owner: 'typescript',
      severity: monacoHarness.monaco.MarkerSeverity.Warning,
      message: 'This can be simplified.',
    }])

    expect(onValidationChange).toHaveBeenLastCalledWith(expect.objectContaining({
      valid: true,
      checking: false,
      diagnosticsReady: true,
      diagnosticErrors: 0,
      serverValid: true,
      problems: 0,
    }))
    expect(container.querySelector('.pipeline-monaco-status > span')?.classList.contains('valid')).toBe(true)
  })

  it('blocks a server-valid pipeline and displays the first Monaco error', async () => {
    const message = "Type 'null' is not assignable to type 'readonly string[]'."
    const onValidationChange = await renderWithMarkers([{
      owner: 'typescript',
      severity: monacoHarness.monaco.MarkerSeverity.Error,
      message,
    }])

    expect(onValidationChange).toHaveBeenLastCalledWith(expect.objectContaining({
      valid: false,
      checking: false,
      diagnosticsReady: true,
      diagnosticErrors: 1,
      serverValid: true,
      message,
      problems: 1,
    }))
    expect(container.querySelector('.pipeline-monaco-status')?.textContent).toContain(message)
    expect(container.querySelector('.pipeline-monaco-status > span')?.classList.contains('invalid')).toBe(true)

    await act(async () => {
      monacoHarness.state.markers = []
      monacoHarness.state.decorationListener?.()
    })
    expect(onValidationChange).toHaveBeenLastCalledWith(expect.objectContaining({
      valid: true,
      checking: false,
      message: 'Pipeline valid.',
      problems: 0,
    }))
  })

  it('keeps the backend error message ahead of Monaco errors', async () => {
    const backendMessage = 'Backend rejected an unsafe pipeline call.'
    monacoHarness.state.markers = [{
      owner: 'typescript',
      severity: monacoHarness.monaco.MarkerSeverity.Error,
      message: 'Frontend type error.',
    }]
    apiMocks.validatePipeline.mockRejectedValue(new Error(backendMessage))
    const onValidationChange = vi.fn()
    await act(async () => {
      root.render(
        <PipelineCodeEditor
          ariaLabel="Pipeline source"
          value="export default definePipeline({ stages: [] })"
          onChange={vi.fn()}
          onValidationChange={onValidationChange}
        />,
      )
    })
    await act(async () => {
      await vi.advanceTimersByTimeAsync(500)
    })
    await act(async () => {
      await Promise.resolve()
    })

    expect(onValidationChange).toHaveBeenLastCalledWith(expect.objectContaining({
      valid: false,
      message: backendMessage,
      problems: 2,
    }))
    expect(container.querySelector('.pipeline-monaco-status')?.textContent).toContain(backendMessage)
  })
})
