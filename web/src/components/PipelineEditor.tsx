import { useEffect, useMemo, useState } from 'react'
import { motion } from 'motion/react'
import { parse as parseYAML } from 'yaml'
import { Link } from 'react-router-dom'
import {
  Braces, Check, CirclePlay, Code2, Copy, FileCode2, GitBranch,
  PanelRight, Plus, RotateCcw, Search, Settings2, TerminalSquare, Trash2, X,
} from 'lucide-react'
import { dialogs } from './AppDialogs'
import { useI18n } from '../i18n'

type Platform = 'default' | 'macos' | 'windows'

type PipelineNode = {
  id: string
  title: string
  stageTitle: string
  command: string
  platform: Platform
  stepType: string
  runtime: string
  dependencies: string[]
  explicitDependencies: string[]
  x: number
  y: number
}

type PipelineProject = { id: number; name: string; config?: string }
type GlobalVariable = { id: number; name: string; value: string; is_secret?: boolean }

type PipelineEditorProps = {
  project: PipelineProject
  projects: PipelineProject[]
  globalVariables: GlobalVariable[]
  onProjectChange?: (id: number) => void
  onSave: (document: string) => Promise<void>
  onRun: () => Promise<unknown>
  embedded?: boolean
}

const initialDocument = `# Release pipeline

> A Markdown-first production pipeline. Variables are shared with every build.

## Variables

- APP_NAME: buildworld
- NODE_ENV: production
- REGISTRY: registry.internal/buildworld

## Pipeline

### Checkout

\`\`\`default shell
git clone $REPOSITORY_URL .
git checkout $GIT_REF
\`\`\`

### Install dependencies

\`\`\`default shell
npm ci
\`\`\`

### Test

\`\`\`default shell
npm run test:ci
\`\`\`

### Build package

\`\`\`default shell
npm run build
\`\`\`

\`\`\`macos shell
./scripts/sign-macos.sh
\`\`\`

### Publish notification

\`\`\`default notify
channel: release-room
event: build.completed
\`\`\``

function quoteMarkdown(value: unknown): string {
  return String(value).replaceAll('\\', '\\\\').replaceAll('\n', ' ')
}

export function sourceToMarkdown(source: string, fallbackName: string): string | null {
  let config: any
  try {
    config = source.trimStart().startsWith('{') ? JSON.parse(source) : parseYAML(source)
  } catch {
    return null
  }
  if (!config || typeof config !== 'object') return null
  // Markdown is only offered when the conversion is lossless. Parameter
  // schemas do not yet have a Markdown representation, so keep their source
  // configuration intact instead of silently dropping them.
  if (Array.isArray(config.parameters) && config.parameters.length) return null
  const environment = config.environment ?? config.env ?? {}
  const stages = Array.isArray(config.stages) ? config.stages : []
  if (!stages.length && Array.isArray(config.steps)) stages.push({ name: 'Build', steps: config.steps })
  if (!stages.length && config.jobs && typeof config.jobs === 'object') {
    for (const [name, job] of Object.entries(config.jobs as Record<string, any>)) {
      stages.push({ name: job?.name || name, steps: job?.steps || [] })
    }
  }
  if (!stages.length) return null
  const lines = [`# ${config.name || fallbackName}`]
  if (Object.keys(environment).length) {
    lines.push('', '## Variables', '')
    for (const [key, value] of Object.entries(environment)) lines.push(`- ${key}: ${quoteMarkdown(value)}`)
  }
  if (Array.isArray(config.artifacts) && config.artifacts.length) {
    lines.push('', '## Artifacts', '')
    for (const pattern of config.artifacts) lines.push(`- ${quoteMarkdown(pattern)}`)
  }
  if (Array.isArray(config.agent_requirements) && config.agent_requirements.length) {
    lines.push('', '## Agents', '')
    for (const requirement of config.agent_requirements) lines.push(String(requirement).startsWith('pool=') ? `- pool: ${String(requirement).slice(5)}` : `- ${quoteMarkdown(requirement)}`)
  }
  if (config.retention_completed) {
    lines.push('', '## Retention', '', `- keep: ${Number(config.retention_completed)}`)
  }
  if (config.toolchains && typeof config.toolchains === 'object' && Object.keys(config.toolchains).length) {
    lines.push('', '## Toolchains', '')
    for (const [name, versions] of Object.entries(config.toolchains)) {
      const values = Array.isArray(versions) ? versions : [versions]
      lines.push(`- ${name}: ${values.map(quoteMarkdown).join(', ')}`)
    }
  }
  const schedule = Array.isArray(config.triggers)
    ? config.triggers.find((trigger: any) => trigger?.type === 'schedule' && trigger?.config?.cron)
    : null
  if (schedule) lines.push('', '## Schedule', '', `- cron: "${quoteMarkdown(schedule.config.cron)}"`)
  const approval = config.approval
  if (approval && String(approval.strategy || 'none').toLowerCase() !== 'none') {
    const roles = Array.isArray(approval.required_roles) && approval.required_roles.length ? approval.required_roles : ['admin']
    lines.push(
      '',
      '## Approval',
      '',
      `- version: ${Number(approval.version) || 1}`,
      `- strategy: ${quoteMarkdown(approval.strategy || 'single')}`,
      `- required_roles: ${roles.map(quoteMarkdown).join(', ')}`,
      `- allow_requester: ${approval.allow_requester !== false}`,
    )
    if (approval.prompt) lines.push(`- prompt: ${quoteMarkdown(approval.prompt)}`)
  }
  lines.push('', '## Pipeline')
  for (const stage of stages) {
    const steps = Array.isArray(stage?.steps) ? stage.steps : []
    for (const step of steps) {
      const title = step?.name || stage?.name || 'Build step'
      const command = step?.command ?? step?.run
      if (typeof command !== 'string' || !command.trim()) continue
      const type = step?.type || 'shell'
      const runtimeValue = step?.runtime || (step?.shell && step.shell !== 'shell' ? step.shell : '')
      const runtime = runtimeValue ? ` ${runtimeValue}` : ''
      lines.push('', `### ${title}`, '', `\`\`\`default ${type}${runtime}`, command.trim(), '```')
    }
  }
  return lines.some((line) => line.startsWith('### ')) ? `${lines.join('\n')}\n` : null
}

function stageSections(source: string) {
  const heading = /^###\s+(.+)$/gm
  const blocks = [...source.matchAll(heading)]
  return blocks.map((match, index) => ({
    title: match[1].trim(),
    start: (match.index ?? 0) + match[0].length,
    end: blocks[index + 1]?.index ?? source.length,
  }))
}

function parseNeeds(section: string): string[] {
  const match = section.match(/^\s*-\s*needs\s*:\s*(.+?)\s*$/m)
  return match ? match[1].split(',').map((item) => item.trim().replace(/^['"]|['"]$/g, '')).filter(Boolean) : []
}

export function parseDocument(source: string): PipelineNode[] {
  const stages = stageSections(source)
  const rawNodes: Array<Omit<PipelineNode, 'x' | 'y'>> = []

  stages.forEach((stage, index) => {
    const section = source.slice(stage.start, stage.end)
    const code = /```(default|macos|windows)\s*([^\n]*)\n([\s\S]*?)```/g
    const codeBlocks = [...section.matchAll(code)]
    if (!codeBlocks.length) return
    const needs = parseNeeds(section)
    const primaryID = `stage-${index}`
    codeBlocks.forEach((block, blockIndex) => {
      const platform = block[1] as Platform
      const metadata = block[2].trim().split(/\s+/).filter(Boolean)
      rawNodes.push({
        id: blockIndex === 0 ? primaryID : `${primaryID}-${platform}`,
        title: blockIndex === 0 ? stage.title : `${stage.title} (${platformIcon[platform]})`,
        stageTitle: stage.title,
        platform,
        stepType: metadata[0] || 'shell',
        runtime: metadata.slice(1).join(' '),
        command: block[3].trim(),
        dependencies: blockIndex === 0 ? needs : [primaryID],
        explicitDependencies: blockIndex === 0 ? needs : [],
      })
    })
  })
  const previousPrimary: string[] = []
  const idByTitle = new Map(rawNodes.filter((node) => node.platform === 'default').map((node) => [node.stageTitle, node.id]))
  rawNodes.forEach((node) => {
    if (node.platform !== 'default') return
    node.explicitDependencies = node.explicitDependencies.map((dependency) => idByTitle.get(dependency) ?? dependency)
    node.dependencies = [...node.explicitDependencies]
    if (!node.dependencies.length && previousPrimary.length) node.dependencies = [previousPrimary[previousPrimary.length - 1]]
    previousPrimary.push(node.id)
  })
  const rankByID = new Map<string, number>()
  const resolving = new Set<string>()
  const rankFor = (node: Omit<PipelineNode, 'x' | 'y'>): number => {
    const known = rankByID.get(node.id)
    if (known !== undefined) return known
    if (resolving.has(node.id)) return 0
    resolving.add(node.id)
    const rank = node.dependencies.reduce((maximum, dependency) => {
      const parent = rawNodes.find((candidate) => candidate.id === dependency)
      return Math.max(maximum, parent ? rankFor(parent) + 1 : 0)
    }, 0)
    resolving.delete(node.id)
    rankByID.set(node.id, rank)
    return rank
  }
  const rowsByRank = new Map<number, number>()
  return rawNodes.map((node) => {
    const rank = rankFor(node)
    const row = rowsByRank.get(rank) ?? 0
    rowsByRank.set(rank, row + 1)
    return { ...node, x: 42 + rank * 214, y: 48 + row * 122 }
  })
}

function replaceStage(source: string, title: string, transform: (section: string) => string) {
  const stage = stageSections(source).find((item) => item.title === title)
  if (!stage) return source
  return `${source.slice(0, stage.start)}${transform(source.slice(stage.start, stage.end))}${source.slice(stage.end)}`
}

function writeNeeds(section: string, needs: string[]) {
  const withoutNeeds = section.replace(/^\s*-\s*needs\s*:\s*.+?\s*$(?:\r?\n)?/m, '')
  return needs.length ? `\n\n- needs: ${needs.join(', ')}${withoutNeeds}` : withoutNeeds
}

const platformStyle: Record<Platform, string> = {
  default: 'node-platform-default',
  macos: 'node-platform-macos',
  windows: 'node-platform-windows',
}

const platformIcon: Record<Platform, string> = {
  default: 'default', macos: 'macOS', windows: 'Windows',
}

export default function PipelineEditor({ project, projects, globalVariables, onProjectChange, onSave, onRun, embedded = false }: PipelineEditorProps) {
  const { t } = useI18n()
  const [document, setDocument] = useState(project.config || initialDocument)
  const [savedDocument, setSavedDocument] = useState(project.config || initialDocument)
  const [activeView, setActiveView] = useState<'split' | 'code' | 'graph'>('split')
  const [selectedNode, setSelectedNode] = useState<string>('')
  const [saving, setSaving] = useState(false)
  const [running, setRunning] = useState(false)
  const [message, setMessage] = useState('')
  const [inspectorOpen, setInspectorOpen] = useState(true)
  const [variableSearchOpen, setVariableSearchOpen] = useState(false)
  const [variableQuery, setVariableQuery] = useState('')
  const [cursor, setCursor] = useState({ line: 1, column: 1 })
  const nodes = useMemo(() => parseDocument(document), [document])
  const selected = nodes.find((node) => node.id === selectedNode) ?? nodes[0]
  const isDirty = document !== savedDocument
  const isMarkdown = document.trimStart().startsWith('#')
  const copy = (key: string, values: Record<string, string | number> = {}) => Object.entries(values).reduce((value, [name, replacement]) => value.replaceAll(`{${name}}`, String(replacement)), t(key))
  const filteredVariables = globalVariables.filter((variable) => !variableQuery.trim() || `${variable.name} ${variable.value}`.toLowerCase().includes(variableQuery.trim().toLowerCase()))

  useEffect(() => {
    const source = project.config || initialDocument
    setDocument(source)
    setSavedDocument(source)
    setActiveView(source.trimStart().startsWith('#') ? 'split' : 'code')
    setMessage('')
    setCursor({ line: 1, column: 1 })
  }, [project.id, project.config])

  useEffect(() => {
    if (nodes.length && !nodes.some((node) => node.id === selectedNode)) setSelectedNode(nodes[0].id)
  }, [nodes, selectedNode])

  useEffect(() => {
    const warnBeforeUnload = (event: BeforeUnloadEvent) => {
      if (!isDirty) return
      event.preventDefault()
    }
    window.addEventListener('beforeunload', warnBeforeUnload)
    return () => window.removeEventListener('beforeunload', warnBeforeUnload)
  }, [isDirty])

  const resetDocument = () => {
    setDocument(savedDocument)
    setActiveView(savedDocument.trimStart().startsWith('#') ? 'split' : 'code')
    setMessage('')
    setCursor({ line: 1, column: 1 })
  }
  const appendStep = () => {
    const existing = new Set(stageSections(document).map((stage) => stage.title))
    let suffix = 1
    let title = 'New step'
    while (existing.has(title)) title = `New step ${suffix++}`
    setDocument((value) => `${value}\n\n### ${title}\n\n\`\`\`default shell\necho "new build step"\n\`\`\``)
  }
  const migrateToMarkdown = () => {
    const migrated = sourceToMarkdown(document, project.name)
    if (!migrated) {
      setMessage(t('pipeline.migrateFailed'))
      dialogs.notify(t('pipeline.migrateFailed'))
      return
    }
    setDocument(migrated)
    setActiveView('split')
    setMessage(t('pipeline.migrated'))
    dialogs.notify(t('pipeline.migrated'), 'success')
  }
  const updateSelectedCommand = (command: string) => {
    if (!selected) return
    setDocument((value) => replaceStage(value, selected.stageTitle, (section) => {
      const fence = new RegExp('(```' + selected.platform + '\\s*[^\\n]*\\n)[\\s\\S]*?(?=\\n```)')
      return section.replace(fence, `$1${command}`)
    }))
  }
  const updateSelectedNeeds = (value: string) => {
    if (!selected || selected.platform !== 'default') return
    const needs = value.split(',').map((item) => item.trim()).filter(Boolean).filter((item) => item !== selected.stageTitle)
    setDocument((source) => replaceStage(source, selected.stageTitle, (section) => writeNeeds(section, needs)))
  }
  const updateSelectedMetadata = (nextPlatform: Platform, nextType: string) => {
    if (!selected) return
    setDocument((value) => replaceStage(value, selected.stageTitle, (section) => {
      const fence = new RegExp('```' + selected.platform + '\\s*[^\\n]*\\n')
      const runtime = selected.runtime ? ` ${selected.runtime}` : ''
      return section.replace(fence, `\`\`\`${nextPlatform} ${nextType}${runtime}\n`)
    }))
  }
  const renameSelectedStage = (title: string) => {
    if (!selected) return
    const nextTitle = title.trim()
    if (!nextTitle || nextTitle === selected.stageTitle || stageSections(document).some((stage) => stage.title === nextTitle)) return
    setDocument((source) => {
      const stage = stageSections(source).find((item) => item.title === selected.stageTitle)
      if (!stage) return source
      const renamed = `${source.slice(0, stage.start - (`### ${selected.stageTitle}`).length)}### ${nextTitle}${source.slice(stage.start)}`
      return stageSections(renamed).reduce((result, item) => {
        if (item.title === nextTitle) return result
        return replaceStage(result, item.title, (section) => writeNeeds(section, parseNeeds(section).map((need) => need === selected.stageTitle ? nextTitle : need)))
      }, renamed)
    })
  }
  const deleteSelectedStage = () => {
    if (!selected) return
    setDocument((source) => {
      const stage = stageSections(source).find((item) => item.title === selected.stageTitle)
      if (!stage) return source
      const removed = `${source.slice(0, stage.start - (`### ${selected.stageTitle}`).length)}${source.slice(stage.end)}`.replace(/\n{3,}/g, '\n\n')
      return stageSections(removed).reduce((result, item) => replaceStage(result, item.title, (section) => writeNeeds(section, parseNeeds(section).filter((need) => need !== selected.stageTitle))), removed)
    })
  }
  const saveDocument = async () => {
    if (!isDirty || saving) return
    setSaving(true)
    setMessage('')
    try {
      await onSave(document)
      setSavedDocument(document)
      setMessage(t('pipeline.savedMessage'))
    } catch (error: any) {
      setMessage(error.message || t('pipeline.saveFailed'))
    } finally {
      setSaving(false)
    }
  }
  const runPipeline = async () => {
    if (running) return
    setRunning(true)
    setMessage('')
    try {
      if (isDirty) {
        await onSave(document)
        setSavedDocument(document)
      }
      await onRun()
      setMessage(t('pipeline.queued'))
    } catch (error: any) {
      setMessage(error.message || t('pipeline.runFailed'))
    } finally {
      setRunning(false)
    }
  }
  const copySource = async () => {
    try {
      await navigator.clipboard.writeText(document)
      setMessage(t('pipeline.copied'))
      dialogs.notify(t('pipeline.copied'), 'success')
    } catch {
      dialogs.notify(t('common.copyFailed'))
    }
  }
  const changeProject = async (id: number) => {
    if (id === project.id) return
    if (isDirty && !await dialogs.confirm(t('pipeline.discardMessage'), { title: t('pipeline.discardTitle'), action: t('pipeline.discardAction') })) return
    onProjectChange?.(id)
  }
  const updateCursor = (target: HTMLTextAreaElement) => {
    const before = target.value.slice(0, target.selectionStart)
    const lines = before.split('\n')
    setCursor({ line: lines.length, column: (lines.at(-1)?.length || 0) + 1 })
  }

  return (
    <motion.section className={`pipeline-workbench ${embedded ? 'pipeline-workbench-embedded' : ''}`} initial={{ opacity: 0, y: 8 }} animate={{ opacity: 1, y: 0 }} transition={{ duration: 0.22, ease: 'easeOut' }}>
      <header className="pipeline-heading">
        <div>
          {!embedded && <div className="crumbs">{copy('pipeline.breadcrumb', { project: project.name })}</div>}
          <div className="title-row"><h1>{embedded ? t('pipeline.definition') : project.name}</h1><span className="saved-state"><Check size={14} /> {isDirty ? t('pipeline.unsaved') : t('pipeline.saved')}</span></div>
        </div>
        <div className="pipeline-actions">
          <button className="icon-button" title={t('pipeline.revert')} aria-label={t('pipeline.revert')} onClick={resetDocument} disabled={!isDirty}><RotateCcw size={16} /></button>
          {!embedded && <select className="select-control project-picker" value={project.id} onChange={(event) => changeProject(Number(event.target.value))} aria-label={t('pipeline.selectProject')}>
            {projects.map((item) => <option key={item.id} value={item.id}>{item.name}</option>)}
          </select>}
          {!isMarkdown && <button className="secondary-button" onClick={migrateToMarkdown}>{t('pipeline.migrate')}</button>}
          <button className="secondary-button" onClick={appendStep} disabled={!isMarkdown}><Plus size={16} /> {t('pipeline.addStep')}</button>
          <button className="secondary-button" onClick={saveDocument} disabled={!isDirty || saving}>{saving ? t('pipeline.saving') : t('pipeline.save')}</button>
          <button className="run-button" onClick={runPipeline} disabled={running}><CirclePlay size={16} /> {running ? t('pipeline.running') : t('pipeline.run')}</button>
          <button className="icon-button" title={t('pipeline.copySource')} aria-label={t('pipeline.copySource')} onClick={copySource}><Copy size={16} /></button>
        </div>
      </header>

      {!embedded && <div className="pipeline-subnav">
        <span className="pipeline-tab active"><GitBranch size={15} /> {t('pipeline.definition')}</span>
        <Link className="pipeline-tab" to={`/builds?project=${project.id}`}><TerminalSquare size={15} /> {t('pipeline.runs')}</Link>
        <Link className="pipeline-tab" to={`/projects/${project.id}`}><Settings2 size={15} /> {t('pipeline.settings')}</Link>
        <div className="view-toggle" role="group" aria-label="Editor layout">
          <button title={t('pipeline.codeView')} aria-label={t('pipeline.codeView')} onClick={() => setActiveView('code')} className={activeView === 'code' ? 'selected' : ''}><Code2 size={15} /></button>
          <button title={t('pipeline.splitView')} aria-label={t('pipeline.splitView')} onClick={() => setActiveView('split')} disabled={!isMarkdown} className={activeView === 'split' ? 'selected' : ''}><PanelRight size={15} /></button>
          <button title={t('pipeline.graphView')} aria-label={t('pipeline.graphView')} onClick={() => setActiveView('graph')} disabled={!isMarkdown} className={activeView === 'graph' ? 'selected' : ''}><GitBranch size={15} /></button>
          {!inspectorOpen && activeView !== 'code' && isMarkdown && <button title={t('pipeline.openInspector')} aria-label={t('pipeline.openInspector')} onClick={() => setInspectorOpen(true)}><Settings2 size={15} /></button>}
        </div>
      </div>}
      {embedded && <div className="pipeline-subnav pipeline-embedded-views"><div className="view-toggle" role="group" aria-label="Editor layout"><button title={t('pipeline.codeView')} aria-label={t('pipeline.codeView')} onClick={() => setActiveView('code')} className={activeView === 'code' ? 'selected' : ''}><Code2 size={15} /></button><button title={t('pipeline.splitView')} aria-label={t('pipeline.splitView')} onClick={() => setActiveView('split')} disabled={!isMarkdown} className={activeView === 'split' ? 'selected' : ''}><PanelRight size={15} /></button><button title={t('pipeline.graphView')} aria-label={t('pipeline.graphView')} onClick={() => setActiveView('graph')} disabled={!isMarkdown} className={activeView === 'graph' ? 'selected' : ''}><GitBranch size={15} /></button>{!inspectorOpen && activeView !== 'code' && isMarkdown && <button title={t('pipeline.openInspector')} aria-label={t('pipeline.openInspector')} onClick={() => setInspectorOpen(true)}><Settings2 size={15} /></button>}</div></div>}

      <div className={`editor-grid ${activeView} ${inspectorOpen ? '' : 'no-inspector'}`}>
        {activeView !== 'graph' && <section className="markdown-panel">
          <div className="panel-header"><div><FileCode2 size={16} /> {isMarkdown ? t('pipeline.markdownFile') : t('pipeline.importedConfig')}</div><span>{isMarkdown ? t('pipeline.markdownDsl') : t('pipeline.migrateHint')}</span></div>
          <div className="editor-body">
            <div className="line-numbers" aria-hidden="true">{document.split('\n').map((_, index) => <span key={index}>{index + 1}</span>)}</div>
            <textarea aria-label={t('pipeline.sourceLabel')} spellCheck="false" value={document} onChange={(event) => { setDocument(event.target.value); updateCursor(event.currentTarget) }} onClick={(event) => updateCursor(event.currentTarget)} onKeyUp={(event) => updateCursor(event.currentTarget)} />
          </div>
          <div className="editor-footer"><span><Braces size={14} /> {copy('pipeline.nodes', { value: nodes.length })}</span><span>UTF-8</span><span>Ln {cursor.line}, Col {cursor.column}</span></div>
        </section>}

        {activeView !== 'code' && isMarkdown && <section className="graph-panel">
          <div className="panel-header"><div><GitBranch size={16} /> {t('pipeline.executionGraph')}</div><span>{t('pipeline.autoLayout')}</span></div>
          <div className="graph-canvas">
            <svg className="graph-links" aria-hidden="true" viewBox="0 0 980 520" preserveAspectRatio="none">
              {nodes.flatMap((node) => node.dependencies.map((dependency) => {
                const parent = nodes.find((candidate) => candidate.id === dependency)
                if (!parent) return null
                return <path key={`${parent.id}-${node.id}`} d={`M ${parent.x + 165} ${parent.y + 47} C ${parent.x + 190} ${parent.y + 47}, ${node.x - 24} ${node.y + 47}, ${node.x} ${node.y + 47}`} />
              }))}
            </svg>
            {nodes.map((node, index) => <button key={node.id} className={`graph-node ${selectedNode === node.id ? 'selected' : ''}`} style={{ left: `${node.x}px`, top: `${node.y}px` }} onClick={() => setSelectedNode(node.id)}>
              <span className={`node-number ${platformStyle[node.platform]}`}>{index + 1}</span>
              <strong>{node.title}</strong>
              <small>{node.platform === 'default' ? t('pipeline.sharedScript') : copy('pipeline.platformOverride', { platform: platformIcon[node.platform] })}</small>
            </button>)}
            <div className="graph-legend"><span><i className="legend-default" /> default</span><span><i className="legend-macos" /> macOS override</span><span><i className="legend-windows" /> Windows override</span></div>
          </div>
        </section>}

        {activeView !== 'code' && isMarkdown && inspectorOpen && <aside className="inspector-panel">
          <div className="panel-header"><div><Settings2 size={16} /> {t('pipeline.inspector')}</div><button title={t('pipeline.closeInspector')} aria-label={t('pipeline.closeInspector')} onClick={() => setInspectorOpen(false)}><X size={15} /></button></div>
          {selected && <div className="inspector-body">
            <p className="inspector-label">{t('pipeline.selectedNode')}</p>
            <input className="inspector-title" value={selected.stageTitle} onChange={(event) => renameSelectedStage(event.target.value)} aria-label={t('pipeline.stepName')} />
            <div className="form-row"><label htmlFor="node-execution">{t('pipeline.execution')}</label><select id="node-execution" className="select-control" value={selected.stepType} onChange={(event) => updateSelectedMetadata(selected.platform, event.target.value)}>{!['shell', 'notify', 'tail'].includes(selected.stepType) && <option value={selected.stepType}>{selected.stepType}</option>}<option value="shell">Shell</option><option value="notify">Notify</option><option value="tail">Tail</option></select></div>
            <div className="form-row"><label htmlFor="node-platform">{t('pipeline.platformRule')}</label><select id="node-platform" className="select-control" value={selected.platform} onChange={(event) => updateSelectedMetadata(event.target.value as Platform, selected.stepType)}><option value="default">{platformIcon.default}</option><option value="macos">{platformIcon.macos}</option><option value="windows">{platformIcon.windows}</option></select></div>
            {selected.platform === 'default' && <div className="form-row"><label htmlFor="node-needs">{t('pipeline.dependsOn')}</label><input id="node-needs" className="inspector-input" value={selected.explicitDependencies.map((dependency) => nodes.find((node) => node.id === dependency)?.stageTitle ?? dependency).join(', ')} onChange={(event) => updateSelectedNeeds(event.target.value)} placeholder={t('pipeline.dependsPlaceholder')} /></div>}
            <label className="inspector-label" htmlFor="node-command">{t('pipeline.script')}</label>
            <textarea id="node-command" className="inspector-script" value={selected.command} onChange={(event) => updateSelectedCommand(event.target.value)} spellCheck="false" />
            <div className="variables-heading"><span>{t('pipeline.globalVariables')}</span><button title={t('pipeline.searchVariables')} aria-label={t('pipeline.searchVariables')} onClick={() => { setVariableSearchOpen((value) => !value); setVariableQuery('') }}><Search size={14} /></button></div>
            {variableSearchOpen && <input className="variable-search" autoFocus value={variableQuery} onChange={(event) => setVariableQuery(event.target.value)} placeholder={t('pipeline.variableSearchPlaceholder')} aria-label={t('pipeline.searchVariables')} />}
            <dl className="variable-list">{filteredVariables.length ? filteredVariables.map((variable) => <div key={variable.id}><dt>{variable.name}</dt><dd>{variable.is_secret ? '********' : variable.value}</dd></div>) : <div><dt>{t('pipeline.noVariables')}</dt><dd>-</dd></div>}</dl>
            <button className="danger-text-button" onClick={deleteSelectedStage}><Trash2 size={14} /> {t('pipeline.deleteStep')}</button>
          </div>}
        </aside>}
      </div>

      <div className="run-strip"><span className={`run-dot ${isDirty ? 'pending' : ''}`} /><strong>{isDirty ? t('pipeline.draft') : t('pipeline.ready')}</strong><span>{project.name}</span><span className="run-stage">{isMarkdown ? copy('pipeline.nodes', { value: nodes.length }) : t('pipeline.imported')}</span><div className="run-progress"><i /></div><span className="run-time">{message || (isMarkdown ? t('pipeline.sourceOfTruth') : t('pipeline.migrateHint'))}</span></div>

    </motion.section>
  )
}
