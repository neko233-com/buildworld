import { useEffect, useMemo, useRef, useState } from 'react'
import { motion } from 'motion/react'
import { Link, useSearchParams } from 'react-router-dom'
import {
  Activity, AlertTriangle, Bot, Check, CheckCircle2, ChevronRight, CircleGauge, Clipboard,
  Clock3, Code2, Copy, Database, Download, FileCode2, FileJson, HardDrive, Info, KeyRound,
  LockKeyhole, Network, PackageCheck, RefreshCw, RotateCw, Save, Search, Server, Settings2,
  ShieldCheck, TerminalSquare, TestTube2, Upload, Workflow,
} from 'lucide-react'
import { api } from '../api'
import { dialogs } from '../components/AppDialogs'
import { PageState } from '../components/PageState'
import { useApi } from '../hooks'
import { useI18n } from '../i18n'
import {
  completePortabilitySections,
  hasSensitiveSelection,
  humanizePortabilityKey,
  isPortabilityBundleSizeAllowed,
  missingPortabilityDependencies,
  resolveSettingsSection,
  safePortabilitySections,
  sanitizePortabilitySections,
  togglePortabilitySection,
  type PortabilityCapability,
  type SettingsSection,
} from '../lib/portabilitySelection'

type SettingsMap = Record<string, string>
type AgentPlatform = 'linux' | 'windows'
type SettingsView = SettingsSection | 'status'

const SETTINGS_VIEWS = new Set<SettingsView>([
  'overview', 'runtime', 'builds', 'agents', 'proxies', 'validation', 'security', 'portability', 'status',
])

const HIDDEN_PORTABILITY_CAPABILITIES = new Set(['build_templates', 'templates'])

function resolveSettingsView(value: string | null): SettingsView {
  if (value && SETTINGS_VIEWS.has(value as SettingsView)) return value as SettingsView
  return resolveSettingsSection(value)
}

const editableKeys = [
  'host', 'build_timeout', 'build_concurrency', 'local_agent_concurrency',
  'cpu_limit_percent', 'background_mode',
  'retry_policy', 'artifacts_path', 'build_temp_path',
  'go_validation_enabled', 'go_version', 'go_checks', 'node_validation_enabled',
  'node_version', 'node_package_manager', 'node_checks', 'validation_fail_fast',
]

const defaults: SettingsMap = {
  host: '0.0.0.0', port: '8700', build_timeout: '1800',
  build_concurrency: '2', local_agent_concurrency: '1', cpu_limit_percent: '25',
  background_mode: 'true', retry_policy: 'failed_once',
  artifacts_path: './artifacts', build_temp_path: './build_temp',
  go_validation_enabled: 'true', go_version: '1.26', go_checks: 'fmt,vet,test,build',
  node_validation_enabled: 'true', node_version: '24', node_package_manager: 'npm',
  node_checks: 'install,lint,typecheck,test,build', validation_fail_fast: 'true',
}

function normalizeSettings(value: SettingsMap | null | undefined): SettingsMap {
  return value ? { ...defaults, ...value, port: '8700' } : { ...defaults }
}

function parseChecks(value: string): string[] {
  return value.split(',').map(item => item.trim()).filter(Boolean)
}

function formatUptime(seconds: number | undefined): string {
  if (typeof seconds !== 'number') return '-'
  const days = Math.floor(seconds / 86400)
  const hours = Math.floor((seconds % 86400) / 3600)
  const minutes = Math.floor((seconds % 3600) / 60)
  return days > 0 ? `${days}d ${hours}h` : `${hours}h ${minutes}m`
}

function shellQuote(value: string): string {
  return `'${value.replaceAll("'", "'\\''")}'`
}

function powerShellQuote(value: string): string {
  return `'${value.replaceAll("'", "''")}'`
}

function SettingField({ label, hint, value, onChange, type = 'text', min, max, suffix, readOnly = false }: {
  label: string
  hint?: string
  value: string
  onChange: (value: string) => void
  type?: 'text' | 'number'
  min?: number
  max?: number
  suffix?: string
  readOnly?: boolean
}) {
  return <label className="settings-field">
    <span>{label}</span>
    <div className="settings-input-wrap">
      <input type={type} min={min} max={max} value={value} readOnly={readOnly} aria-readonly={readOnly} onChange={event => onChange(event.target.value)} />
      {suffix && <small>{suffix}</small>}
    </div>
    {hint && <em>{hint}</em>}
  </label>
}

function Toggle({ checked, onChange, label, description, id }: {
  checked: boolean
  onChange: (checked: boolean) => void
  label: string
  description?: string
  id?: string
}) {
  return <label className="settings-toggle-row">
    <span><strong>{label}</strong>{description && <small>{description}</small>}</span>
    <button id={id} type="button" role="switch" aria-checked={checked} className={`settings-switch ${checked ? 'on' : ''}`} onClick={() => onChange(!checked)}><i /></button>
  </label>
}

function SectionHeading({ icon: Icon, title, description, action }: {
  icon: typeof Settings2
  title: string
  description: string
  action?: React.ReactNode
}) {
  return <header className="settings-section-heading">
    <div className="settings-section-icon"><Icon size={17} /></div>
    <div><h2>{title}</h2><p>{description}</p></div>
    {action && <aside>{action}</aside>}
  </header>
}

function CodePanel({ title, value, copyLabel, onCopy, copied }: {
  title: string
  value: string
  copyLabel: string
  onCopy: () => void
  copied: boolean
}) {
  return <section className="settings-code-panel">
    <header><span><TerminalSquare size={14} />{title}</span><button type="button" onClick={onCopy} title={copyLabel}>{copied ? <Check size={14} /> : <Copy size={14} />}{copied ? copyLabel : null}</button></header>
    <pre>{value}</pre>
  </section>
}

function CheckSelector({ options, selected, onChange }: {
  options: Array<{ id: string; label: string }>
  selected: string[]
  onChange: (next: string[]) => void
}) {
  const toggle = (id: string) => onChange(selected.includes(id) ? selected.filter(item => item !== id) : [...selected, id])
  return <div className="validation-checks">{options.map(option => <button type="button" key={option.id} className={selected.includes(option.id) ? 'selected' : ''} onClick={() => toggle(option.id)}><span>{selected.includes(option.id) && <Check size={11} />}</span>{option.label}</button>)}</div>
}

export default function Settings() {
  const { locale, t } = useI18n()
  const [searchParams, setSearchParams] = useSearchParams()
  const requestedSection = searchParams.get('section')
  const section = resolveSettingsView(requestedSection)
  const { data: settings, loading, error, reload } = useApi(() => api.getGlobalSettings(), [])
  const { data: metrics } = useApi(() => api.getServerMetrics(), [])
  const { data: agents } = useApi(() => api.listAgents(), [])
  const { data: packageProxies, error: packageProxiesError, reload: reloadPackageProxies } = useApi(() => api.listPackageProxies(), [])
  const { data: portabilityCapabilities, error: portabilityError } = useApi<PortabilityCapability[]>(() => api.listPortabilityCapabilities(), [])
  const [draft, setDraft] = useState<SettingsMap>(defaults)
  const [baseline, setBaseline] = useState<SettingsMap>(defaults)
  const [initialized, setInitialized] = useState(false)
  const [saving, setSaving] = useState(false)
  const [saved, setSaved] = useState(false)
  const [copied, setCopied] = useState('')
  const [settingsQuery, setSettingsQuery] = useState('')
  const [agentToken, setAgentToken] = useState('')
  const [generatingToken, setGeneratingToken] = useState(false)
  const [agentPlatform, setAgentPlatform] = useState<AgentPlatform>('linux')
  const [agentConfig, setAgentConfig] = useState({
    server: window.location.origin,
    name: 'validation-agent-01',
    pool: 'validation',
    labels: 'go,nodejs,typescript',
    maxBuilds: '4',
  })
  const importFileRef = useRef<HTMLInputElement>(null)
  const [portabilityInitialized, setPortabilityInitialized] = useState(false)
  const [exportSections, setExportSections] = useState<string[]>([])
  const [includeSecrets, setIncludeSecrets] = useState(false)
  const [exporting, setExporting] = useState(false)
  const [exportError, setExportError] = useState('')
  const [exportStatus, setExportStatus] = useState('')
  const [importBundle, setImportBundle] = useState<any>(null)
  const [importInspection, setImportInspection] = useState<any>(null)
  const [importMode, setImportMode] = useState<'skip' | 'overwrite'>('skip')
  const [importing, setImporting] = useState(false)
  const [inspecting, setInspecting] = useState(false)
  const [importDragging, setImportDragging] = useState(false)
  const [importError, setImportError] = useState('')
  const [importResult, setImportResult] = useState<any>(null)
  const [selectedProxies, setSelectedProxies] = useState<string[]>([])
  const [proxiesInitialized, setProxiesInitialized] = useState(false)
  const [applyingProxies, setApplyingProxies] = useState<'accelerated' | 'official' | ''>('')
  const [proxyMessage, setProxyMessage] = useState('')
  const [proxyError, setProxyError] = useState('')
  const capabilities = portabilityCapabilities || []
  const visibleCapabilities = capabilities.filter(capability => !HIDDEN_PORTABILITY_CAPABILITIES.has(capability.key))
  const proxyList = packageProxies || []

  const changeSection = (nextSection: SettingsView) => {
    const next = new URLSearchParams(searchParams)
    if (nextSection === 'overview') next.delete('section')
    else next.set('section', nextSection)
    setSearchParams(next)
  }

  useEffect(() => {
    if (!requestedSection || requestedSection === section) return
    const next = new URLSearchParams(searchParams)
    next.delete('section')
    setSearchParams(next, { replace: true })
  }, [requestedSection, searchParams, section, setSearchParams])

  useEffect(() => {
    if (!settings || initialized) return
    const normalized = normalizeSettings(settings)
    setDraft(normalized)
    setBaseline(normalized)
    setInitialized(true)
  }, [settings, initialized])

  useEffect(() => {
    if (portabilityInitialized || !portabilityCapabilities) return
    setExportSections(safePortabilitySections(portabilityCapabilities))
    setPortabilityInitialized(true)
  }, [portabilityCapabilities, portabilityInitialized])

  useEffect(() => {
    if (proxiesInitialized || !packageProxies) return
    setSelectedProxies(packageProxies.map(proxy => proxy.id))
    setProxiesInitialized(true)
  }, [packageProxies, proxiesInitialized])

  const set = (key: string, value: string) => {
    setDraft(current => ({ ...current, [key]: value }))
    setSaved(false)
  }
  const dirty = editableKeys.some(key => draft[key] !== baseline[key])
  const restartKeys = ['host', 'artifacts_path', 'build_temp_path']
  const restartRequired = restartKeys.some(key => draft[key] !== baseline[key])
  const sys = metrics?.system || metrics || {}
  const agentList = agents || []
  const onlineAgents = agentList.filter((agent: any) => agent.status === 'online').length

  const handleSave = async () => {
    const payload = Object.fromEntries(editableKeys.map(key => [key, draft[key]]))
    setSaving(true)
    try {
      await api.updateGlobalSettings(payload)
      setBaseline({ ...draft })
      setSaved(true)
      reload()
    } catch (reason: any) {
      dialogs.notify(reason.message || t('settings.saveFailed'))
    } finally {
      setSaving(false)
    }
  }

  const toggleProxy = (id: string) => {
    setSelectedProxies(current => current.includes(id) ? current.filter(item => item !== id) : [...current, id])
    setProxyMessage('')
    setProxyError('')
  }

  const applyProxies = async (mode: 'accelerated' | 'official', packages = selectedProxies) => {
    if (!packages.length) {
      setProxyError(t('settings.proxySelectRequired'))
      return
    }
    const all = packages.length === proxyList.length
    const confirmed = await dialogs.confirm(
      all ? t(mode === 'accelerated' ? 'settings.proxyAccelerateAllConfirm' : 'settings.proxyOfficialAllConfirm') : t(mode === 'accelerated' ? 'settings.proxyAccelerateSelectedConfirm' : 'settings.proxyOfficialSelectedConfirm'),
      { title: t('settings.packageProxies'), action: t(mode === 'accelerated' ? 'settings.proxyAccelerate' : 'settings.proxyOfficial') },
    )
    if (!confirmed) return
    setApplyingProxies(mode)
    setProxyMessage('')
    setProxyError('')
    try {
      const result = await api.applyPackageProxies(mode, packages)
      setProxyMessage(t('settings.proxyApplied').replace('{count}', String(result.updated?.length || packages.length)))
      await reloadPackageProxies()
    } catch (reason: any) {
      setProxyError(reason.message || t('settings.proxyApplyFailed'))
    } finally {
      setApplyingProxies('')
    }
  }

  const capabilityLabel = (key: string) => {
    if (HIDDEN_PORTABILITY_CAPABILITIES.has(key)) {
      return locale === 'zh-CN' ? '兼容数据' : 'Compatibility data'
    }
    const translationKey = `settings.portability_${key}`
    const translated = t(translationKey)
    return translated === translationKey ? humanizePortabilityKey(key) : translated
  }

  const capabilityHelp = (capability: PortabilityCapability) => {
    const translationKey = `settings.portabilityHelp_${capability.key}`
    const translated = t(translationKey)
    return translated === translationKey ? capability.description || capabilityLabel(capability.key) : translated
  }

  const missingDependencies = missingPortabilityDependencies(capabilities, exportSections)
  const invalidSensitiveSelection = hasSensitiveSelection(capabilities, exportSections) && !includeSecrets

  const toggleExportSection = (capability: PortabilityCapability) => {
    const result = togglePortabilitySection(exportSections, capability, includeSecrets)
    setExportStatus('')
    if (result.blockedBySecrets) {
      setExportError(t('settings.sensitiveSelectionBlocked'))
      document.getElementById('include-portability-secrets')?.focus()
      return
    }
    setExportError('')
    setExportSections(result.sections)
  }

  const chooseSafeExport = () => {
    setIncludeSecrets(false)
    setExportSections(safePortabilitySections(capabilities))
    setExportError('')
    setExportStatus('')
  }

  const enableSecretExport = async () => {
    const confirmed = await dialogs.confirm(t('settings.enableSecretsConfirm'), {
      title: t('settings.enableSecretsTitle'),
      action: t('settings.enableSecretsAction'),
    })
    if (confirmed) {
      setIncludeSecrets(true)
      setExportError('')
    }
    return confirmed
  }

  const changeSecretExport = async (checked: boolean) => {
    setExportStatus('')
    if (!checked) {
      setIncludeSecrets(false)
      setExportSections(current => sanitizePortabilitySections(capabilities, current, false))
      setExportError('')
      return
    }
    await enableSecretExport()
  }

  const chooseCompleteExport = async () => {
    if (!includeSecrets && !await enableSecretExport()) return
    setIncludeSecrets(true)
    setExportSections(completePortabilitySections(capabilities))
    setExportError('')
    setExportStatus('')
  }

  const handleExport = async () => {
    if (!exportSections.length) return
    if (invalidSensitiveSelection) {
      setExportError(t('settings.sensitiveSelectionBlocked'))
      return
    }
    setExporting(true)
    setExportError('')
    setExportStatus('')
    try {
      const { blob, filename } = await api.exportPortabilityBundle({ sections: exportSections, include_secrets: includeSecrets })
      const url = URL.createObjectURL(blob)
      const anchor = document.createElement('a')
      anchor.href = url
      anchor.download = filename
      anchor.hidden = true
      document.body.appendChild(anchor)
      anchor.click()
      anchor.remove()
      window.setTimeout(() => URL.revokeObjectURL(url), 0)
      setExportStatus(t('settings.exportDownloaded').replace('{filename}', filename))
    } catch (reason: any) {
      setExportError(reason.message || t('settings.exportFailed'))
    } finally {
      setExporting(false)
    }
  }

  const inspectImportFile = async (file?: File) => {
    if (!file) return
    setImportInspection(null)
    setImportResult(null)
    setImportError('')
    if (!isPortabilityBundleSizeAllowed(file.size)) {
      setImportBundle(null)
      setImportError(t('settings.bundleTooLarge'))
      return
    }
    setInspecting(true)
    try {
      const bundle = JSON.parse(await file.text())
      const inspection = await api.inspectPortabilityBundle(bundle)
      setImportBundle(bundle)
      setImportInspection({ ...inspection, filename: file.name, size: file.size })
    } catch (reason: any) {
      setImportBundle(null)
      setImportError(reason.message || t('settings.inspectFailed'))
    } finally {
      setInspecting(false)
      if (importFileRef.current) importFileRef.current.value = ''
    }
  }

  const handleImport = async () => {
    if (!importBundle || !importInspection) return
    const confirmed = await dialogs.confirm(t('settings.importConfirm'), {
      title: t('settings.importBundle'),
      action: t('settings.importAction'),
    })
    if (!confirmed) return
    setImporting(true)
    setImportError('')
    try {
      const result = await api.importPortabilityBundle(importBundle, importMode)
      setImportResult(result)
      reload()
    } catch (reason: any) {
      setImportError(reason.message || t('settings.importFailed'))
    } finally {
      setImporting(false)
    }
  }

  const copyText = async (id: string, value: string) => {
    await navigator.clipboard.writeText(value)
    setCopied(id)
    window.setTimeout(() => setCopied(current => current === id ? '' : current), 1600)
  }

  const rotateAgentToken = async () => {
    setGeneratingToken(true)
    try {
      const response = await api.generateAgentToken()
      setAgentToken(response.token)
      setDraft(current => ({ ...current, agent_enrollment_token_configured: 'true' }))
    } catch (reason: any) {
      dialogs.notify(reason.message || t('settings.tokenFailed'))
    } finally {
      setGeneratingToken(false)
    }
  }

  const agentCommand = useMemo(() => {
    if (!agentToken) return t('settings.generateTokenFirst')
    const quote = agentPlatform === 'windows' ? powerShellQuote : shellQuote
    const binary = agentPlatform === 'windows' ? '.\\buildworld-worker.exe' : './buildworld-worker'
    const continuation = agentPlatform === 'windows' ? ' `\n  ' : ' \\\n  '
    return [
      binary,
      `--server ${quote(agentConfig.server)}`,
      `--token ${quote(agentToken)}`,
      `--name ${quote(agentConfig.name)}`,
      `--pool ${quote(agentConfig.pool)}`,
      `--labels ${quote(agentConfig.labels)}`,
      `--max-builds ${agentConfig.maxBuilds}`,
      `--build-temp ${quote(draft.build_temp_path || './build_temp')}`,
    ].join(continuation)
  }, [agentConfig, agentPlatform, agentToken, draft.build_temp_path, t])

  const validationPipeline = useMemo(() => {
    const steps: string[] = []
    const goChecks = parseChecks(draft.go_checks)
    const nodeChecks = parseChecks(draft.node_checks)
    const step = (name: string, runtime: string, command: string) => steps.push(`    shell(${JSON.stringify(name)}, ${JSON.stringify(command)}, { runtime: ${JSON.stringify(runtime)} })`)
    if (draft.go_validation_enabled === 'true') {
      step('Go dependencies', `go@${draft.go_version}`, 'go mod download && go mod verify')
      if (goChecks.includes('fmt')) step('Go format', `go@${draft.go_version}`, 'gofmt -w . && git diff --exit-code -- .')
      if (goChecks.includes('vet')) step('Go vet', `go@${draft.go_version}`, 'go vet ./...')
      if (goChecks.includes('test')) step('Go test', `go@${draft.go_version}`, 'go test -count=1 -coverprofile=coverage.out ./...')
      if (goChecks.includes('build')) step('Go build', `go@${draft.go_version}`, 'go build ./...')
    }
    if (draft.node_validation_enabled === 'true') {
      const manager = draft.node_package_manager
      const install = manager === 'npm' ? 'npm ci --no-audit --fund=false' : manager === 'pnpm' ? 'pnpm install --frozen-lockfile' : 'yarn install --immutable'
      const run = (script: string) => manager === 'npm' ? `npm run ${script} --if-present` : `${manager} run ${script}`
      if (nodeChecks.includes('install')) step('Node install', `node@${draft.node_version}`, install)
      if (nodeChecks.includes('lint')) step('TypeScript lint', `node@${draft.node_version}`, run('lint'))
      if (nodeChecks.includes('typecheck')) step('TypeScript typecheck', `node@${draft.node_version}`, manager === 'npm' ? 'npm exec tsc -- --noEmit' : `${manager} exec tsc --noEmit`)
      if (nodeChecks.includes('test')) step('TypeScript test', `node@${draft.node_version}`, run('test'))
      if (nodeChecks.includes('build')) step('TypeScript build', `node@${draft.node_version}`, run('build'))
    }
    const toolchains = [
      draft.go_validation_enabled === 'true' ? `go: [${JSON.stringify(draft.go_version)}]` : '',
      draft.node_validation_enabled === 'true' ? `node: [${JSON.stringify(draft.node_version)}]` : '',
    ].filter(Boolean).join(', ')
    const labels = draft.go_validation_enabled === 'true' && draft.node_validation_enabled === 'true' ? ['go', 'nodejs', 'typescript'] : draft.go_validation_enabled === 'true' ? ['go'] : ['nodejs', 'typescript']
    return `import { definePipeline, shell, stage } from '@buildworld/pipeline'

export default definePipeline({
  name: 'Production validation',
  environment: { CI: 'true' },
  toolchains: { ${toolchains} },
  agentRequirements: [${[`pool=${agentConfig.pool}`, ...labels].map(value => JSON.stringify(value)).join(', ')}],
  stages: [stage('Validation', [
${steps.join(',\n')}
  ])],
  artifacts: ['coverage.out', 'coverage/*', 'dist/*'],
  retentionCompleted: 30,
})
`
  }, [agentConfig.pool, draft])

  const directoryCopy = locale === 'zh-CN' ? {
    systemConfiguration: '系统配置',
    security: '安全',
    statusInformation: '状态信息',
    toolsAndData: '工具和数据',
    searchSettings: '搜索设置',
    noResults: '未找到匹配的设置。',
    compatibilityData: '兼容数据',
  } : {
    systemConfiguration: 'System Configuration',
    security: 'Security',
    statusInformation: 'Status Information',
    toolsAndData: 'Tools and Data',
    searchSettings: 'Search settings',
    noResults: 'No matching settings found.',
    compatibilityData: 'Compatibility data',
  }
  const navigation: Array<{
    id: Exclude<SettingsView, 'overview'>
    label: string
    description: string
    icon: typeof Settings2
    badge?: string
  }> = [
    { id: 'runtime', label: t('settings.runtimeStorage'), description: t('settings.runtimeStorageDescription'), icon: Server },
    { id: 'builds', label: t('settings.buildPolicy'), description: t('settings.buildPolicyDescription'), icon: Workflow },
    { id: 'agents', label: t('settings.agentAutomation'), description: t('settings.agentAutomationDescription'), icon: Bot },
    { id: 'security', label: t('settings.security'), description: t('settings.securityDescription'), icon: ShieldCheck },
    { id: 'status', label: t('settings.systemOverview'), description: t('settings.systemOverviewDescription'), icon: CircleGauge },
    { id: 'proxies', label: t('settings.packageProxies'), description: t('settings.packageProxiesDescription'), icon: Network },
    { id: 'validation', label: t('settings.validation'), description: t('settings.validationDescription'), icon: TestTube2 },
    { id: 'portability', label: t('settings.backup'), description: t('settings.portabilityDescription'), icon: Database, badge: directoryCopy.compatibilityData },
  ]
  const query = settingsQuery.trim().toLocaleLowerCase()
  const visibleSettings = query
    ? navigation.filter(item => `${item.label} ${item.description} ${item.badge || ''}`.toLocaleLowerCase().includes(query))
    : navigation
  const groupDefinitions: Array<{ id: string; title: string; items: Array<Exclude<SettingsView, 'overview'>> }> = [
    { id: 'system', title: directoryCopy.systemConfiguration, items: ['runtime', 'builds', 'agents'] },
    { id: 'security', title: directoryCopy.security, items: ['security'] },
    { id: 'status', title: directoryCopy.statusInformation, items: ['status'] },
    { id: 'tools', title: directoryCopy.toolsAndData, items: ['proxies', 'validation', 'portability'] },
  ]
  const directoryGroups = groupDefinitions
    .map(group => ({ ...group, items: visibleSettings.filter(item => group.items.includes(item.id)) }))
    .filter(group => group.items.length > 0)
  const currentNavigation = section === 'overview' ? null : navigation.find(item => item.id === section) || null

  if (loading || !initialized) return <PageState />
  if (error) return <PageState error={error} onRetry={reload} />

  return <motion.section className="settings-center" initial={{ opacity: 0, y: 6 }} animate={{ opacity: 1, y: 0 }} transition={{ duration: 0.2 }}>
    <nav className="settings-breadcrumbs" aria-label="Breadcrumb">
      <Link to="/">{t('dashboard.title')}</Link>
      <ChevronRight size={13} aria-hidden="true" />
      {currentNavigation ? <>
        <button type="button" onClick={() => changeSection('overview')}>{t('settings.title')}</button>
        <ChevronRight size={13} aria-hidden="true" />
        <span aria-current="page">{currentNavigation.label}</span>
      </> : <span aria-current="page">{t('settings.title')}</span>}
    </nav>

    {section === 'overview' ? <header className="settings-page-heading">
      <div><h1>{t('settings.title')}</h1><p>{t('settings.description')}</p></div>
      <div className="settings-save-area">
        <span className={dirty ? 'dirty' : saved ? 'saved' : ''}>{dirty ? t('settings.unsaved') : saved ? t('settings.saved') : t('settings.upToDate')}</span>
        <button className="primary-command" onClick={handleSave} disabled={!dirty || saving}>{saving ? <RefreshCw className="spin" size={15} /> : <Save size={15} />}{saving ? t('settings.saving') : t('settings.saveChanges')}</button>
      </div>
    </header> : <div className="settings-detail-toolbar">
      <button type="button" className="settings-back-link" onClick={() => changeSection('overview')}>{t('settings.title')}</button>
      <div className="settings-save-area">
        <span className={dirty ? 'dirty' : saved ? 'saved' : ''}>{dirty ? t('settings.unsaved') : saved ? t('settings.saved') : t('settings.upToDate')}</span>
        <button className="primary-command" onClick={handleSave} disabled={!dirty || saving}>{saving ? <RefreshCw className="spin" size={15} /> : <Save size={15} />}{saving ? t('settings.saving') : t('settings.saveChanges')}</button>
      </div>
    </div>}

    {section === 'overview' && <label className="settings-directory-search">
      <Search size={17} aria-hidden="true" />
      <span className="sr-only">{directoryCopy.searchSettings}</span>
      <input type="search" value={settingsQuery} onChange={event => setSettingsQuery(event.target.value)} placeholder={directoryCopy.searchSettings} aria-label={directoryCopy.searchSettings} autoComplete="off" />
    </label>}

    {restartRequired && <div className="settings-restart-notice"><RotateCw size={15} /><span><strong>{t('settings.restartRequired')}</strong>{t('settings.restartDescription')}</span></div>}

    {section === 'overview' ? <section className="settings-directory" aria-label={t('settings.title')}>
      {directoryGroups.map(group => <section className="settings-directory-group" key={group.id}>
        <h2>{group.title}</h2>
        <div className="settings-directory-list">
          {group.items.map(item => { const Icon = item.icon; return <button key={item.id} type="button" className="settings-directory-link" onClick={() => changeSection(item.id)}>
            <span className={`settings-directory-icon ${item.id}`}><Icon size={31} strokeWidth={1.65} /></span>
            <span className="settings-directory-copy"><strong>{item.label}</strong><small>{item.description}</small>{item.badge && <em>{item.badge}</em>}</span>
            <ChevronRight size={16} aria-hidden="true" />
          </button> })}
        </div>
      </section>)}
      {directoryGroups.length === 0 && <p className="settings-directory-empty" role="status">{directoryCopy.noResults}</p>}
    </section> : <div className="settings-workspace settings-detail-workspace">
      <div className="settings-content">
        {section === 'status' && <div className="settings-pane">
          <SectionHeading icon={CircleGauge} title={t('settings.systemOverview')} description={t('settings.systemOverviewDescription')} />
          <div className="settings-health-grid">
            <article><span><Activity size={15} />{t('settings.serviceStatus')}</span><strong className="positive"><i />{t('settings.healthy')}</strong><small>{t('settings.serviceStatusHelp')}</small></article>
            <article><span><Network size={15} />{t('settings.onlineAgents')}</span><strong>{onlineAgents} <small>/ {agentList.length}</small></strong><small>{t('settings.agentCapacityHelp')}</small></article>
            <article><span><Clock3 size={15} />{t('settings.uptime')}</span><strong>{formatUptime(sys.uptime_seconds ?? sys.uptime)}</strong><small>{sys.go_version || t('settings.runtimeUnknown')}</small></article>
            <article><span><HardDrive size={15} />{t('settings.buildIsolation')}</span><strong className="mono-value">{draft.build_temp_path}</strong><small>{t('settings.buildIsolationHelp')}</small></article>
          </div>
          <section className="settings-overview-list">
            <button type="button" onClick={() => changeSection('agents')}><span className="overview-list-icon agent"><Bot size={17} /></span><span><strong>{t('settings.agentAutomation')}</strong><small>{draft.agent_enrollment_token_configured === 'true' ? t('settings.tokenReady') : t('settings.tokenNotConfigured')}</small></span><em className={draft.agent_enrollment_token_configured === 'true' ? 'ready' : 'warning'}>{draft.agent_enrollment_token_configured === 'true' ? t('settings.ready') : t('settings.actionRequired')}</em><ChevronRight size={15} /></button>
            <button type="button" onClick={() => changeSection('validation')}><span className="overview-list-icon go"><Code2 size={17} /></span><span><strong>Go {t('settings.validation')}</strong><small>{t('settings.detectGoMod')} · {parseChecks(draft.go_checks).length} {t('settings.checksEnabled')}</small></span><em className="ready">{draft.go_validation_enabled === 'true' ? t('settings.enabled') : t('settings.disabled')}</em><ChevronRight size={15} /></button>
            <button type="button" onClick={() => changeSection('validation')}><span className="overview-list-icon node"><PackageCheck size={17} /></span><span><strong>TypeScript / Node.js</strong><small>{t('settings.detectPackageJson')} · {draft.node_package_manager}</small></span><em className="ready">{draft.node_validation_enabled === 'true' ? t('settings.enabled') : t('settings.disabled')}</em><ChevronRight size={15} /></button>
          </section>
        </div>}

        {section === 'runtime' && <div className="settings-pane">
          <SectionHeading icon={Server} title={t('settings.runtimeStorage')} description={t('settings.runtimeStorageDescription')} />
          <section className="settings-form-section"><header><Network size={15} /><div><h3>{t('settings.serverConfig')}</h3><p>{t('settings.serverConfigHelp')}</p></div></header><div className="settings-form-grid"><SettingField label={t('settings.host')} value={draft.host} onChange={value => set('host', value)} /><SettingField label={t('settings.port')} hint={t('settings.portFixed')} type="number" value="8700" readOnly onChange={() => undefined} /></div></section>
          <section className="settings-form-section"><header><Database size={15} /><div><h3>{t('settings.storagePaths')}</h3><p>{t('settings.storageHelp')}</p></div></header><div className="settings-form-grid"><SettingField label={t('settings.artifactsPath')} value={draft.artifacts_path} onChange={value => set('artifacts_path', value)} /><SettingField label={t('settings.buildTempPath')} hint={t('settings.buildTempHint')} value={draft.build_temp_path} onChange={value => set('build_temp_path', value)} /></div><div className="settings-inline-note"><Info size={14} /><span>{t('settings.buildTempNote')}</span></div></section>
          <section className="settings-form-section"><header><Activity size={15} /><div><h3>{t('settings.resourceControl')}</h3><p>{t('settings.resourceControlHelp')}</p></div></header><div className="settings-form-grid three"><SettingField label={t('settings.cpuLimit')} hint={t('settings.cpuLimitHelp')} type="number" min={5} max={100} suffix="%" value={draft.cpu_limit_percent} onChange={value => set('cpu_limit_percent', value)} /><SettingField label={t('settings.localConcurrency')} type="number" min={1} max={256} value={draft.local_agent_concurrency} onChange={value => set('local_agent_concurrency', value)} /><Toggle label={t('settings.backgroundMode')} description={t('settings.backgroundModeHelp')} checked={draft.background_mode === 'true'} onChange={value => set('background_mode', String(value))} /></div><div className="settings-inline-note"><Info size={14} /><span>{t('settings.resourceHotReloadHelp')}</span></div></section>
        </div>}

        {section === 'builds' && <div className="settings-pane">
          <SectionHeading icon={Workflow} title={t('settings.buildPolicy')} description={t('settings.buildPolicyDescription')} />
          <section className="settings-form-section"><header><Clock3 size={15} /><div><h3>{t('settings.executionDefaults')}</h3><p>{t('settings.executionDefaultsHelp')}</p></div></header><div className="settings-form-grid three"><SettingField label={t('settings.timeout')} type="number" min={30} max={86400} suffix={t('settings.seconds')} value={draft.build_timeout} onChange={value => set('build_timeout', value)} /><SettingField label={t('settings.concurrency')} type="number" min={1} max={256} value={draft.build_concurrency} onChange={value => set('build_concurrency', value)} /><label className="settings-field"><span>{t('settings.retryPolicy')}</span><select value={draft.retry_policy} onChange={event => set('retry_policy', event.target.value)}><option value="never">{t('settings.retryNever')}</option><option value="failed_once">{t('settings.retryOnce')}</option><option value="failed_twice">{t('settings.retryTwice')}</option></select></label></div></section>
          <section className="settings-form-section"><header><ShieldCheck size={15} /><div><h3>{t('settings.failureBehavior')}</h3><p>{t('settings.failureBehaviorHelp')}</p></div></header><Toggle label={t('settings.failFast')} description={t('settings.failFastHelp')} checked={draft.validation_fail_fast === 'true'} onChange={value => set('validation_fail_fast', String(value))} /></section>
        </div>}

        {section === 'agents' && <div className="settings-pane">
          <SectionHeading icon={Bot} title={t('settings.agentAutomation')} description={t('settings.agentAutomationDescription')} action={<span className={`settings-status-label ${onlineAgents ? 'ready' : ''}`}><i />{onlineAgents} {t('settings.online')}</span>} />
          <section className="settings-form-section agent-enrollment-section"><header><KeyRound size={15} /><div><h3>{t('settings.enrollmentCredential')}</h3><p>{t('settings.enrollmentCredentialHelp')}</p></div><button type="button" className="secondary-command" onClick={rotateAgentToken} disabled={generatingToken}>{generatingToken ? <RefreshCw className="spin" size={14} /> : <RotateCw size={14} />}{generatingToken ? t('settings.generating') : draft.agent_enrollment_token_configured === 'true' ? t('settings.rotateToken') : t('settings.generateToken')}</button></header>{agentToken ? <div className="agent-token-result"><span><CheckCircle2 size={15} /><strong>{t('settings.tokenGenerated')}</strong>{t('settings.tokenGeneratedHelp')}</span><code>{agentToken}</code><button type="button" onClick={() => copyText('token', agentToken)}>{copied === 'token' ? <Check size={14} /> : <Copy size={14} />}</button></div> : <div className="settings-inline-note"><Info size={14} /><span>{draft.agent_enrollment_token_configured === 'true' ? t('settings.tokenConfiguredHelp') : t('settings.tokenMissingHelp')}</span></div>}</section>
          <section className="settings-form-section"><header><Settings2 size={15} /><div><h3>{t('settings.agentProfile')}</h3><p>{t('settings.agentProfileHelp')}</p></div></header><div className="settings-form-grid"><SettingField label={t('settings.serverUrl')} value={agentConfig.server} onChange={value => setAgentConfig(current => ({ ...current, server: value }))} /><SettingField label={t('settings.agentName')} value={agentConfig.name} onChange={value => setAgentConfig(current => ({ ...current, name: value }))} /><SettingField label={t('agents.pool')} value={agentConfig.pool} onChange={value => setAgentConfig(current => ({ ...current, pool: value }))} /><SettingField label={t('agents.labels')} hint={t('settings.labelsHelp')} value={agentConfig.labels} onChange={value => setAgentConfig(current => ({ ...current, labels: value }))} /><SettingField label={t('agents.maxBuilds')} type="number" min={1} max={256} value={agentConfig.maxBuilds} onChange={value => setAgentConfig(current => ({ ...current, maxBuilds: value }))} /></div></section>
          <section className="settings-form-section"><header><TerminalSquare size={15} /><div><h3>{t('settings.installCommand')}</h3><p>{t('settings.installCommandHelp')}</p></div><div className="settings-segmented"><button type="button" className={agentPlatform === 'linux' ? 'active' : ''} onClick={() => setAgentPlatform('linux')}>Linux</button><button type="button" className={agentPlatform === 'windows' ? 'active' : ''} onClick={() => setAgentPlatform('windows')}>Windows</button></div></header><CodePanel title={agentPlatform === 'windows' ? 'PowerShell' : 'Shell'} value={agentCommand} copyLabel={t('settings.copy')} onCopy={() => copyText('agent-command', agentCommand)} copied={copied === 'agent-command'} /></section>
        </div>}

        {section === 'proxies' && <div className="settings-pane package-proxies-pane">
          <SectionHeading icon={Network} title={t('settings.packageProxies')} description={t('settings.packageProxiesDescription')} action={<div className="package-proxy-actions"><button type="button" className="secondary-command" onClick={() => applyProxies('official')} disabled={!!applyingProxies || !selectedProxies.length}>{applyingProxies === 'official' && <RefreshCw className="spin" size={14} />}{t('settings.proxyOfficial')}</button><button type="button" className="primary-command" onClick={() => applyProxies('accelerated')} disabled={!!applyingProxies || !selectedProxies.length}>{applyingProxies === 'accelerated' && <RefreshCw className="spin" size={14} />}{t('settings.proxyAccelerate')}</button></div>} />
          <section className="settings-form-section package-proxy-summary"><header><Network size={15} /><div><h3>{t('settings.proxyBatch')}</h3><p>{t('settings.proxyBatchHelp')}</p></div><button type="button" className="secondary-command" onClick={() => setSelectedProxies(proxyList.map(proxy => proxy.id))}>{t('settings.proxySelectAll')}</button></header><div className="settings-inline-note"><Info size={14} /><span>{t('settings.proxyPersistence')}</span></div>{packageProxiesError && <div className="settings-inline-note error" role="alert"><AlertTriangle size={14} /><span>{packageProxiesError}</span></div>}{proxyError && <div className="settings-inline-note error" role="alert"><AlertTriangle size={14} /><span>{proxyError}</span></div>}{proxyMessage && <div className="portability-result" role="status"><CheckCircle2 size={16} /><div><strong>{t('settings.proxySuccess')}</strong><span>{proxyMessage}</span></div></div>}</section>
          <section className="package-proxy-grid">{proxyList.map(proxy => <article key={proxy.id} className={`package-proxy-card ${selectedProxies.includes(proxy.id) ? 'selected' : ''}`}><header><button type="button" className="package-proxy-selector" aria-pressed={selectedProxies.includes(proxy.id)} aria-label={`${selectedProxies.includes(proxy.id) ? t('settings.proxyDeselect') : t('settings.proxySelect')} ${proxy.label}`} onClick={() => toggleProxy(proxy.id)}>{selectedProxies.includes(proxy.id) && <Check size={13} />}</button><div><h3>{proxy.label}</h3><p>{proxy.description}</p></div><span className={`package-proxy-mode ${proxy.mode}`}><i />{proxy.mode === 'accelerated' ? t('settings.proxyAccelerated') : proxy.mode === 'official' ? t('settings.proxyOfficialMode') : t('settings.proxyCustom')}</span></header><code>{Object.entries(proxy.variables || {}).map(([key, value]) => `${key}=${value || 'unset'}`).join('\n')}</code><footer><button type="button" onClick={() => applyProxies('official', [proxy.id])} disabled={!!applyingProxies}>{t('settings.proxyOfficial')}</button><button type="button" onClick={() => applyProxies('accelerated', [proxy.id])} disabled={!!applyingProxies}>{t('settings.proxyAccelerate')}</button></footer></article>)}</section>
        </div>}

        {section === 'validation' && <div className="settings-pane validation-settings-pane">
          <SectionHeading icon={TestTube2} title={t('settings.validation')} description={t('settings.validationDescription')} action={<button type="button" className="secondary-command" onClick={() => copyText('pipeline', validationPipeline)}><Clipboard size={14} />{copied === 'pipeline' ? t('settings.copied') : t('settings.copyPipeline')}</button>} />
          <section className={`validation-policy ${draft.go_validation_enabled === 'true' ? 'enabled' : ''}`}><header><div className="validation-language-icon go">Go</div><div><h3>Go</h3><p>{t('settings.detectGoMod')}</p></div><Toggle label={t('settings.enabled')} checked={draft.go_validation_enabled === 'true'} onChange={value => set('go_validation_enabled', String(value))} /></header><div className="validation-policy-body"><label><span>{t('settings.toolchainVersion')}</span><select value={draft.go_version} onChange={event => set('go_version', event.target.value)}><option>1.21</option><option>1.22</option><option>1.26</option></select></label><div><span>{t('settings.validationSteps')}</span><CheckSelector options={[{ id: 'fmt', label: 'gofmt' }, { id: 'vet', label: 'go vet' }, { id: 'test', label: 'go test' }, { id: 'build', label: 'go build' }]} selected={parseChecks(draft.go_checks)} onChange={value => set('go_checks', value.join(','))} /></div><small><HardDrive size={13} />GOCACHE / GOMODCACHE → {draft.build_temp_path}/cache/go</small></div></section>
          <section className={`validation-policy ${draft.node_validation_enabled === 'true' ? 'enabled' : ''}`}><header><div className="validation-language-icon node">TS</div><div><h3>TypeScript / Node.js</h3><p>{t('settings.detectPackageJson')}</p></div><Toggle label={t('settings.enabled')} checked={draft.node_validation_enabled === 'true'} onChange={value => set('node_validation_enabled', String(value))} /></header><div className="validation-policy-body"><label><span>{t('settings.toolchainVersion')}</span><select value={draft.node_version} onChange={event => set('node_version', event.target.value)}><option>20</option><option>22</option><option>24</option></select></label><label><span>{t('settings.packageManager')}</span><select value={draft.node_package_manager} onChange={event => set('node_package_manager', event.target.value)}><option value="npm">npm</option><option value="pnpm">pnpm</option><option value="yarn">Yarn</option></select></label><div className="wide"><span>{t('settings.validationSteps')}</span><CheckSelector options={[{ id: 'install', label: 'lockfile install' }, { id: 'lint', label: 'lint' }, { id: 'typecheck', label: 'tsc --noEmit' }, { id: 'test', label: 'test' }, { id: 'build', label: 'build' }]} selected={parseChecks(draft.node_checks)} onChange={value => set('node_checks', value.join(','))} /></div><small><HardDrive size={13} />npm / Corepack / pnpm → {draft.build_temp_path}/cache/node</small></div></section>
          <section className="settings-form-section pipeline-generator"><header><FileCode2 size={15} /><div><h3>{t('settings.pipelinePreview')}</h3><p>{t('settings.pipelinePreviewHelp')}</p></div></header><CodePanel title="pipeline.buildworld.ts" value={validationPipeline} copyLabel={t('settings.copy')} onCopy={() => copyText('pipeline-code', validationPipeline)} copied={copied === 'pipeline-code'} /></section>
        </div>}

        {section === 'security' && <div className="settings-pane">
          <SectionHeading icon={ShieldCheck} title={t('settings.security')} description={t('settings.securityDescription')} />
          <section className="security-audit-list"><article><span><KeyRound size={16} /></span><div><h3>{t('settings.agentCredentialSecurity')}</h3><p>{t('settings.agentCredentialSecurityHelp')}</p></div><em className={draft.agent_enrollment_token_configured === 'true' ? 'ready' : 'warning'}>{draft.agent_enrollment_token_configured === 'true' ? t('settings.configured') : t('settings.notConfigured')}</em></article><article><span><HardDrive size={16} /></span><div><h3>{t('settings.workspaceBoundary')}</h3><p>{t('settings.workspaceBoundaryHelp')}</p></div><em className="ready">{t('settings.enforced')}</em></article><article><span><ShieldCheck size={16} /></span><div><h3>{t('settings.secretMasking')}</h3><p>{t('settings.secretMaskingHelp')}</p></div><em className="ready">{t('settings.enforced')}</em></article></section>
        </div>}

        {section === 'portability' && <div className="settings-pane portability-pane">
          <SectionHeading icon={Database} title={t('settings.backup')} description={t('settings.portabilityDescription')} action={<span className="settings-compatibility-label">{directoryCopy.compatibilityData}</span>} />
          {portabilityError && <div className="settings-inline-note error"><Info size={14} /><span>{portabilityError}</span></div>}

          <section className="settings-form-section portability-card">
            <header><Download size={15} /><div><h3>{t('settings.exportBundle')}</h3><p>{t('settings.exportBundleHelp')}</p></div><button type="button" className="primary-command" onClick={handleExport} disabled={exporting || !exportSections.length || invalidSensitiveSelection}>{exporting ? <RefreshCw className="spin" size={14} /> : <Download size={14} />}{exporting ? t('settings.exporting') : t('settings.downloadBundle')}</button></header>
            <div className="portability-presets">
              <span><strong>{visibleCapabilities.filter(capability => exportSections.includes(capability.key)).length}</strong> / {visibleCapabilities.length} {t('settings.sectionsSelected')}</span>
              <div>
                <button type="button" className={!includeSecrets && exportSections.length === safePortabilitySections(capabilities).length ? 'active' : ''} onClick={chooseSafeExport}>{t('settings.safeExport')}</button>
                <button type="button" className={includeSecrets && exportSections.length === capabilities.length ? 'active' : ''} onClick={chooseCompleteExport}><LockKeyhole size={12} />{t('settings.completeExport')}</button>
              </div>
            </div>
            <div className="portability-section-grid">{visibleCapabilities.map(capability => <button type="button" key={capability.key} className={`${exportSections.includes(capability.key) ? 'selected' : ''} ${capability.sensitive ? 'sensitive' : ''}`} aria-pressed={exportSections.includes(capability.key)} disabled={capability.sensitive && !includeSecrets} onClick={() => toggleExportSection(capability)}><span>{exportSections.includes(capability.key) ? <Check size={12} /> : capability.sensitive ? <LockKeyhole size={11} /> : null}</span><div><strong>{capabilityLabel(capability.key)}{capability.sensitive && <em>{t('settings.sensitiveSection')}</em>}</strong><small>v{capability.version} · {capabilityHelp(capability)}</small></div></button>)}</div>
            <Toggle id="include-portability-secrets" checked={includeSecrets} onChange={changeSecretExport} label={t('settings.includeSecrets')} description={t('settings.includeSecretsHelp')} />
            {missingDependencies.length > 0 && <div className="settings-inline-note warning"><AlertTriangle size={14} /><span>{t('settings.missingDependencies')}: {missingDependencies.map(item => `${capabilityLabel(item.section)} → ${capabilityLabel(item.dependency)}`).join(' · ')}</span></div>}
            {exportError && <div className="settings-inline-note error" role="alert"><AlertTriangle size={14} /><span>{exportError}</span></div>}
            {exportStatus && <div className="portability-result" role="status"><CheckCircle2 size={16} /><div><strong>{t('settings.exportComplete')}</strong><span>{exportStatus}</span></div></div>}
            <div className="settings-inline-note"><Info size={14} /><span>{t('settings.portabilityCompatibility')}</span></div>
          </section>

          <section className="settings-form-section portability-card">
            <header><Upload size={15} /><div><h3>{t('settings.importBundle')}</h3><p>{t('settings.importBundleHelp')}</p></div><button type="button" className="secondary-command" onClick={() => importFileRef.current?.click()} disabled={inspecting}><FileJson size={14} />{inspecting ? t('settings.inspecting') : t('settings.chooseBundle')}</button><input ref={importFileRef} hidden type="file" accept="application/json,.json" tabIndex={-1} onChange={event => inspectImportFile(event.target.files?.[0])} /></header>
            {!importInspection ? <button type="button" className={`portability-dropzone ${importDragging ? 'dragging' : ''}`} disabled={inspecting} onClick={() => importFileRef.current?.click()} onDragEnter={() => setImportDragging(true)} onDragLeave={() => setImportDragging(false)} onDragOver={event => event.preventDefault()} onDrop={event => { event.preventDefault(); setImportDragging(false); inspectImportFile(event.dataTransfer.files?.[0]) }}><Upload className={inspecting ? 'spin' : ''} size={22} /><strong>{inspecting ? t('settings.inspecting') : t('settings.dropBundle')}</strong><small>{t('settings.bundleLimit')}</small></button> : <div className="portability-inspection">
              <header><FileJson size={18} /><div><strong>{importInspection.filename}</strong><small>{t('settings.bundleSchema')} v{importInspection.schema_version} · {new Date(importInspection.exported_at).toLocaleString()}</small></div><button type="button" onClick={() => { setImportBundle(null); setImportInspection(null); setImportResult(null); setImportError('') }}>{t('common.close')}</button></header>
              <div className="portability-summary">{importInspection.sections.map((item: any) => <article key={item.key}><strong>{item.count}</strong><span>{capabilityLabel(item.key)}</span></article>)}</div>
              {importInspection.unknown_sections?.length > 0 && <div className="settings-inline-note"><Info size={14} /><span>{t('settings.unknownSections')}: {importInspection.unknown_sections.join(', ')}</span></div>}
              <div className="portability-import-footer"><label><span>{t('settings.conflictPolicy')}</span><select value={importMode} onChange={event => setImportMode(event.target.value as 'skip' | 'overwrite')}><option value="skip">{t('settings.skipExisting')}</option><option value="overwrite">{t('settings.overwriteExisting')}</option></select></label><button type="button" className="primary-command" onClick={handleImport} disabled={importing}>{importing ? <RefreshCw className="spin" size={14} /> : <Upload size={14} />}{importing ? t('settings.importing') : t('settings.importAction')}</button></div>
            </div>}
            {importError && <div className="settings-inline-note error" role="alert"><AlertTriangle size={14} /><span>{importError}</span></div>}
            {importResult && <div className="portability-result" role="status"><CheckCircle2 size={16} /><div><strong>{t('settings.importComplete')}</strong><span>{importResult.sections.map((item: any) => `${capabilityLabel(item.key)}: +${item.created || 0} / ${t('settings.updated')} ${item.updated || 0} / ${t('settings.skipped')} ${item.skipped || 0}`).join(' · ')}</span></div></div>}
          </section>
        </div>}
      </div>
    </div>}
  </motion.section>
}
