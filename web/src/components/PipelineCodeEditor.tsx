import { useEffect, useId, useRef, useState } from 'react'
import Editor, { loader, type Monaco, type OnMount } from '@monaco-editor/react'
import * as monacoRuntime from 'monaco-editor/esm/vs/editor/editor.api.js'
import 'monaco-editor/esm/vs/editor/editor.main.js'
import EditorWorker from 'monaco-editor/esm/vs/editor/editor.worker?worker'
import TypeScriptWorker from 'monaco-editor/esm/vs/language/typescript/ts.worker?worker'
import pipelineDeclarations from '../../../sdk/pipeline/index.d.ts?raw'
import { api } from '../api'
import { useI18n } from '../i18n'
import {
  pendingPipelineValidation,
  type PipelineValidationState,
} from '../lib/pipelineValidation'

type EditorModel = NonNullable<ReturnType<Parameters<OnMount>[0]['getModel']>>

type PipelineCodeEditorProps = {
  value: string
  onChange: (value: string) => void
  readOnly?: boolean
  className?: string
  ariaLabel: string
  height?: number | string
  statusId?: string
  language?: 'typescript' | 'yaml' | 'jenkinsfile'
  onValidationChange?: (state: PipelineValidationState) => void
}

const serverMarkerOwner = 'buildworld-pipeline'
let monacoConfigured = false

function modelDiagnosticErrors(monaco: Monaco, model: EditorModel) {
  return monaco.editor
    .getModelMarkers({ resource: model.uri })
    .filter(marker => marker.owner !== serverMarkerOwner && marker.severity === monaco.MarkerSeverity.Error)
}

// registerGroovy adds a lightweight Monarch grammar for Jenkinsfile sources.
// Monaco does not ship Groovy, so we register it once so the editor can
// highlight Jenkinsfile pipelines alongside TypeScript and YAML.
function registerGroovy(monaco: Monaco) {
  if (typeof monaco.languages.register !== 'function' || typeof monaco.languages.setMonarchTokensProvider !== 'function') return
  if (monaco.languages.getLanguages?.().some(language => language.id === 'groovy')) return
  monaco.languages.register({ id: 'groovy', extensions: ['.groovy', 'Jenkinsfile'], aliases: ['Groovy', 'jenkinsfile'] })
  monaco.languages.setMonarchTokensProvider('groovy', {
    defaultToken: '',
    tokenPostfix: '.groovy',
    keywords: [
      'agent', 'any', 'none', 'stage', 'stages', 'steps', 'script', 'node', 'pipeline',
      'environment', 'options', 'parameters', 'triggers', 'post', 'always', 'success',
      'failure', 'unstable', 'changed', 'fixed', 'aborted', 'when', 'input', 'parallel',
      'matrix', 'axes', 'axis', 'tools', 'def', 'if', 'else', 'for', 'while', 'return',
      'try', 'catch', 'finally', 'throw', 'new', 'this', 'super', 'class', 'void', 'boolean',
      'int', 'String', 'def', 'true', 'false', 'null', 'import', 'static', 'public',
      'private', 'protected', 'final', 'abstract',
    ],
    operators: [
      '=', '>', '<', '!', '~', '?', ':', '==', '<=', '>=', '!=', '&&', '||', '++', '--',
      '+', '-', '*', '/', '&', '|', '^', '%', '<<', '>>', '>>>', '+=', '-=', '*=', '/=',
    ],
    symbols: /[=><!~?:&|+\-*/^%]+/,
    tokenizer: {
      root: [
        [/#.*$/, 'comment'],
        [/\/\/.*$/, 'comment'],
        [/\/\*/, 'comment', '@comment'],
        [/"([^"\\]|\\.)*$/, 'string.invalid'],
        [/'([^'\\]|\\.)*$/, 'string.invalid'],
        [/"""/, 'string', '@string2'],
        [/"/, 'string', '@string'],
        [/'/, 'string', '@string'],
        [/\b\d+(\.\d+)?\b/, 'number'],
        [/\$\{/, 'delimiter', '@interp'],
        [
          /[a-zA-Z_$][\w$]*/,
          {
            cases: {
              '@keywords': 'keyword',
              '@default': 'identifier',
            },
          },
        ],
        [/@symbols/, { cases: { '@operators': 'operator', '@default': '' } }],
        [/[{}()[\]]/, '@brackets'],
        [/@/, 'delimiter'],
      ],
      comment: [
        [/[^/*]+/, 'comment'],
        [/\*\//, 'comment', '@pop'],
        [/[/*]/, 'comment'],
      ],
      string: [
        [/[^'"\\$]+/, 'string'],
        [/\\./, 'string.escape'],
        [/'/, 'string', '@pop'],
        [/"$/, 'string', '@pop'],
        [/\$\{/, 'delimiter', '@interp'],
      ],
      string2: [
        [/[^"\\$]+/, 'string'],
        [/\\./, 'string.escape'],
        [/"""/, 'string', '@pop'],
      ],
      interp: [
        [/[}]/, 'delimiter', '@pop'],
        [/[a-zA-Z_][\w.]*/, 'variable'],
        [/./, 'delimiter'],
      ],
    },
  })
}

function configureMonaco(monaco: Monaco) {
  if (monacoConfigured) return
  monacoConfigured = true
  registerGroovy(monaco)
  monaco.languages.typescript.typescriptDefaults.setCompilerOptions({
    allowNonTsExtensions: true,
    module: monaco.languages.typescript.ModuleKind.ESNext,
    moduleResolution: monaco.languages.typescript.ModuleResolutionKind.NodeJs,
    noEmit: true,
    strict: true,
    target: monaco.languages.typescript.ScriptTarget.ES2020,
  })
  monaco.languages.typescript.typescriptDefaults.setDiagnosticsOptions({
    noSemanticValidation: false,
    noSyntaxValidation: false,
    noSuggestionDiagnostics: false,
  })
  monaco.languages.typescript.typescriptDefaults.addExtraLib(
    pipelineDeclarations,
    'file:///node_modules/@buildworld/pipeline/index.d.ts',
  )
}

if (typeof self !== 'undefined') {
  ;(self as typeof self & { MonacoEnvironment?: unknown }).MonacoEnvironment = {
    getWorker: (_moduleId: string, label: string) => (
      label === 'typescript' || label === 'javascript'
        ? new TypeScriptWorker()
        : new EditorWorker()
    ),
  }
}
loader.config({ monaco: monacoRuntime })

export default function PipelineCodeEditor({
  value,
  onChange,
  readOnly = false,
  className = '',
  ariaLabel,
  height = 420,
  statusId,
  language = 'typescript',
  onValidationChange,
}: PipelineCodeEditorProps) {
  const { t } = useI18n()
  const modelID = useId().replaceAll(':', '-')
  const editorRef = useRef<Parameters<OnMount>[0] | null>(null)
  const monacoRef = useRef<Monaco | null>(null)
  const validationSequence = useRef(0)
  const serverErrorRef = useRef('')
  const serverValidRef = useRef(false)
  const checkingRef = useRef(false)
  const sourceRef = useRef(value)
  const onValidationChangeRef = useRef(onValidationChange)
  const [cursor, setCursor] = useState({ line: 1, column: 1 })
  const validationRef = useRef<PipelineValidationState>(pendingPipelineValidation(value))
  const [validation, setValidation] = useState<PipelineValidationState>(validationRef.current)
  sourceRef.current = value
  onValidationChangeRef.current = onValidationChange

  const publishValidation = (next: PipelineValidationState) => {
    validationRef.current = next
    setValidation(next)
    onValidationChangeRef.current?.(next)
  }

  const refreshProblems = (diagnosticsReady = true) => {
    const editor = editorRef.current
    const monaco = monacoRef.current
    const model = editor?.getModel()
    if (!monaco || !model) return
    const errors = modelDiagnosticErrors(monaco, model)
    const checking = checkingRef.current
    const serverError = serverErrorRef.current
    const valid = diagnosticsReady && serverValidRef.current && !checking && errors.length === 0
    publishValidation({
      source: sourceRef.current,
      checking,
      diagnosticsReady,
      diagnosticErrors: errors.length,
      serverValid: serverValidRef.current,
      valid,
      message: serverError || errors[0]?.message || (checking ? t('config.validating') : valid ? t('config.validated') : t('config.invalid')),
      problems: errors.length + (serverError ? 1 : 0),
    })
  }

  const handleMount: OnMount = (editor, monaco) => {
    configureMonaco(monaco)
    editorRef.current = editor
    monacoRef.current = monaco
    setCursor({
      line: editor.getPosition()?.lineNumber || 1,
      column: editor.getPosition()?.column || 1,
    })
    editor.onDidChangeCursorPosition(event => {
      setCursor({ line: event.position.lineNumber, column: event.position.column })
    })
    editor.onDidChangeModelDecorations(() => refreshProblems())
    refreshProblems()
  }

  useEffect(() => {
    const sequence = ++validationSequence.current
    serverErrorRef.current = ''
    serverValidRef.current = false
    checkingRef.current = false
    const currentModel = editorRef.current?.getModel()
    if (currentModel && monacoRef.current) {
      monacoRef.current.editor.setModelMarkers(currentModel, serverMarkerOwner, [])
    }
    if (!value.trim()) {
      publishValidation({
        ...pendingPipelineValidation(value),
        diagnosticsReady: Boolean(currentModel && monacoRef.current),
        message: t('config.invalid'),
        problems: 1,
      })
      return
    }
    const timer = window.setTimeout(async () => {
      checkingRef.current = true
      publishValidation({
        source: value,
        checking: true,
        diagnosticsReady: validationRef.current.source === value && validationRef.current.diagnosticsReady,
        diagnosticErrors: validationRef.current.source === value ? validationRef.current.diagnosticErrors : 0,
        serverValid: false,
        valid: false,
        message: t('config.validating'),
        problems: validationRef.current.source === value ? validationRef.current.diagnosticErrors : 0,
      })
      try {
        await api.validatePipeline(value)
        if (sequence !== validationSequence.current) return
        checkingRef.current = false
        serverValidRef.current = true
        const model = editorRef.current?.getModel()
        if (model && monacoRef.current) monacoRef.current.editor.setModelMarkers(model, serverMarkerOwner, [])
        const errors = model && monacoRef.current ? modelDiagnosticErrors(monacoRef.current, model) : []
        const diagnosticsReady = Boolean(model && monacoRef.current)
        publishValidation({
          source: value,
          checking: false,
          diagnosticsReady,
          diagnosticErrors: errors.length,
          serverValid: true,
          valid: diagnosticsReady && errors.length === 0,
          message: errors[0]?.message || (diagnosticsReady ? t('config.validated') : t('config.validating')),
          problems: errors.length,
        })
      } catch (error: any) {
        if (sequence !== validationSequence.current) return
        checkingRef.current = false
        const message = error?.message || t('config.invalid')
        serverErrorRef.current = message
        serverValidRef.current = false
        const model = editorRef.current?.getModel()
        const monaco = monacoRef.current
        if (model && monaco) {
          monaco.editor.setModelMarkers(model, serverMarkerOwner, [{
            severity: monaco.MarkerSeverity.Error,
            message,
            startLineNumber: 1,
            startColumn: 1,
            endLineNumber: 1,
            endColumn: Math.max(2, model.getLineMaxColumn(1)),
          }])
        }
        const diagnosticErrors = model && monaco ? modelDiagnosticErrors(monaco, model).length : 0
        publishValidation({
          source: value,
          checking: false,
          diagnosticsReady: Boolean(model && monaco),
          diagnosticErrors,
          serverValid: false,
          valid: false,
          message,
          problems: diagnosticErrors + 1,
        })
      }
    }, 450)
    return () => window.clearTimeout(timer)
    // `validation` intentionally stays out: source changes, not status updates,
    // own the debounced request and its sequence guard.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [value, t])

  useEffect(() => () => {
    validationSequence.current += 1
    editorRef.current = null
    monacoRef.current = null
  }, [])

  const monacoLang = language === 'jenkinsfile' ? 'groovy' : language
  const fileExt = language === 'jenkinsfile' ? 'groovy' : language === 'yaml' ? 'yaml' : 'ts'
  const formatLabel = language === 'jenkinsfile' ? 'Jenkinsfile' : language === 'yaml' ? 'YAML' : 'TypeScript'
  return (
    <div
      className={`pipeline-monaco-editor ${className}`.trim()}
      aria-label={ariaLabel}
      aria-describedby={statusId}
      aria-invalid={validation.diagnosticsReady && !validation.checking && !validation.valid}
      aria-busy={validation.checking}
    >
      <Editor
        beforeMount={configureMonaco}
        height={height}
        language={monacoLang}
        onChange={next => onChange(next ?? '')}
        onMount={handleMount}
        path={`file:///buildworld/pipeline-${modelID}.${fileExt}`}
        theme="vs-dark"
        value={value}
        options={{
          ariaLabel,
          automaticLayout: true,
          bracketPairColorization: { enabled: true },
          contextmenu: true,
          fontFamily: 'ui-monospace, SFMono-Regular, Menlo, Consolas, monospace',
          fontLigatures: true,
          fontSize: 13,
          formatOnPaste: true,
          formatOnType: true,
          glyphMargin: true,
          lineNumbersMinChars: 3,
          minimap: { enabled: false },
          padding: { top: 12, bottom: 12 },
          readOnly,
          renderValidationDecorations: 'on',
          scrollBeyondLastLine: false,
          smoothScrolling: true,
          tabSize: 2,
          wordWrap: 'on',
        }}
      />
      <div className="pipeline-monaco-status" id={statusId} role="status" aria-live="polite">
        <span className={validation.valid ? 'valid' : validation.checking ? 'checking' : validation.problems ? 'invalid' : ''}>
          {validation.message || t('config.validating')}
        </span>
        <span>{formatLabel}</span>
        <span>UTF-8</span>
        <span>Ln {cursor.line}, Col {cursor.column}</span>
        <span>{validation.problems} {t('config.problems')}</span>
      </div>
    </div>
  )
}
