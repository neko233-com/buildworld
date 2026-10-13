import { useEffect, useId, useRef, useState, type FormEvent } from 'react'
import { CirclePlay, GitBranch, KeyRound, LoaderCircle, RefreshCw, SlidersHorizontal, X } from 'lucide-react'
import { parse as parseYAML } from 'yaml'
import { api } from '../api'
import { useI18n } from '../i18n'
import { isTypeScriptPipelineSource } from '../lib/configFormat'
import { releaseActionLabel, validReleaseEnvironment } from '../lib/releasePresentation'
import { ModalDialog } from './ModalDialog'
import { dialogs } from './AppDialogs'
import './RunBuildDialog.css'

export type BuildParameterDefinition = {
  name: string
  type: 'string' | 'text' | 'password' | 'boolean' | 'choice' | 'number'
  description: string
  defaultValue: unknown
  required: boolean
  choices: string[]
  secret: boolean
}

export type BuildProject = {
  id: number
  name: string
  default_branch?: string
  config?: string
}

type RunBuildDialogProps = {
  project: BuildProject
  onClose: () => void
  onQueued: (build: any) => void
}

type BuildParametersFormProps = {
  project: BuildProject
  onCancel: () => void
  onQueued: (build: any) => void
  onBusyChange?: (busy: boolean) => void
  variant?: 'dialog' | 'page'
  initialParameters?: Record<string, string>
}

const supportedTypes = new Set<BuildParameterDefinition['type']>([
  'string',
  'text',
  'password',
  'boolean',
  'choice',
  'number',
])

export function normalizeBuildParameterDefinitions(parameters: unknown): BuildParameterDefinition[] {
  if (!Array.isArray(parameters)) return []
  const seen = new Set<string>()
  return parameters.flatMap((parameter: any) => {
    const name = String(parameter?.name || '').trim()
    if (!name || seen.has(name)) return []
    seen.add(name)
    const rawType = String(parameter?.type || 'string').toLowerCase() as BuildParameterDefinition['type']
    const type = supportedTypes.has(rawType) ? rawType : 'string'
    const choices = Array.isArray(parameter?.choices) ? parameter.choices.map(String) : []
    return [{
      name,
      type,
      description: String(parameter?.description || ''),
      defaultValue: parameter?.default,
      required: Boolean(parameter?.required),
      choices,
      secret: Boolean(parameter?.is_secret) || type === 'password',
    }]
  })
}

export function parseBuildParameterDefinitions(source = ''): BuildParameterDefinition[] {
  const trimmed = source.trim()
  if (!trimmed || isTypeScriptPipelineSource(source) || trimmed.startsWith('{') || trimmed.startsWith('[') || trimmed.startsWith('#')) return []
  try {
    return normalizeBuildParameterDefinitions(parseYAML(source)?.parameters)
  } catch {
    return []
  }
}

function initialParameterValues(definitions: BuildParameterDefinition[]): Record<string, string | boolean> {
  return Object.fromEntries(definitions.map(parameter => {
    if (parameter.defaultValue !== undefined && parameter.defaultValue !== null) {
      if (parameter.type === 'boolean') {
        const normalized = String(parameter.defaultValue).trim().toLowerCase()
        return [parameter.name, parameter.defaultValue === true || normalized === 'true']
      }
      return [parameter.name, String(parameter.defaultValue)]
    }
    if (parameter.type === 'boolean') return [parameter.name, false]
    if (parameter.type === 'choice' && parameter.choices.length) return [parameter.name, parameter.choices[0]]
    return [parameter.name, '']
  }))
}

function fieldDescriptionID(base: string, index: number): string {
  return `${base}-parameter-${index}-description`
}

function fieldErrorID(base: string, index: number): string {
  return `${base}-parameter-${index}-error`
}

export function BuildParametersForm({
  project,
  onCancel,
  onQueued,
  onBusyChange,
  variant = 'dialog',
  initialParameters,
}: BuildParametersFormProps) {
  const { t } = useI18n()
  const formID = useId().replace(/:/g, '')
  const branchRef = useRef<HTMLInputElement>(null)
  const parameterRefs = useRef<Record<string, HTMLInputElement | HTMLSelectElement | HTMLTextAreaElement | null>>({})
  const [definitions, setDefinitions] = useState(() => parseBuildParameterDefinitions(project.config))
  const [loadingDefinitions, setLoadingDefinitions] = useState(true)
  const [definitionsFailed, setDefinitionsFailed] = useState(false)
  const [definitionRequest, setDefinitionRequest] = useState(0)
  const [branch, setBranch] = useState(project.default_branch || 'main')
  const [values, setValues] = useState<Record<string, string | boolean>>(() => ({ ...initialParameterValues(definitions), ...initialParameters }))
  const [saving, setSaving] = useState(false)
  const [confirming, setConfirming] = useState(false)
  const [error, setError] = useState('')
  const [branchError, setBranchError] = useState('')
  const [fieldErrors, setFieldErrors] = useState<Record<string, string>>({})
  const [historicalEnvironments, setHistoricalEnvironments] = useState<{ channel: string; environment: string }[]>([])
  const environmentDefinition = definitions.find(parameter => parameter.name === 'RELEASE_ENVIRONMENT')
  const stringEnvironment = environmentDefinition?.type === 'string' && !environmentDefinition.secret

  useEffect(() => {
    let active = true
    if (!stringEnvironment || loadingDefinitions) return
    api.listReleaseTargets(project.id).then(targets => {
      if (active) setHistoricalEnvironments(targets.filter(target => validReleaseEnvironment(target.environment)))
    }).catch(() => {
      // History suggestions are optional; the pipeline still validates the target.
      if (active) setHistoricalEnvironments([])
    })
    return () => { active = false }
  }, [loadingDefinitions, project.id, stringEnvironment])

  useEffect(() => {
    let active = true
    setLoadingDefinitions(true)
    setDefinitionsFailed(false)
    setError('')
    api.validateProject(project.id).then(metadata => {
      if (!active) return
      if (metadata.valid === false) throw new Error(t('config.invalid'))
      const next = normalizeBuildParameterDefinitions(metadata.parameters)
      setDefinitions(next)
      setValues(current => ({ ...initialParameterValues(next), ...current }))
      setLoadingDefinitions(false)
    }).catch(reason => {
      if (!active) return
      setError(reason instanceof Error ? reason.message : t('config.invalid'))
      setDefinitionsFailed(true)
      setLoadingDefinitions(false)
    })
    return () => {
      active = false
    }
  }, [definitionRequest, project.id, t])

  useEffect(() => {
    onBusyChange?.(saving)
    return () => onBusyChange?.(false)
  }, [onBusyChange, saving])

  const updateValue = (name: string, value: string | boolean) => {
    setValues(current => ({ ...current, [name]: value }))
    setFieldErrors(current => {
      if (!current[name]) return current
      const next = { ...current }
      delete next[name]
      return next
    })
  }

  const focusFirstInvalid = (missingBranch: boolean, missingParameter?: string) => {
    window.requestAnimationFrame(() => {
      if (missingBranch) branchRef.current?.focus()
      else if (missingParameter) parameterRefs.current[missingParameter]?.focus()
    })
  }

  const handleSubmit = async (event: FormEvent) => {
    event.preventDefault()
    if (saving || loadingDefinitions || definitionsFailed) return

    setError('')
    const nextBranchError = branch.trim() ? '' : t('builds.requiredParameterMissing').replace('{name}', t('builds.branch'))
    const nextFieldErrors: Record<string, string> = {}
    for (const parameter of definitions) {
      if (parameter.required && parameter.type !== 'boolean' && !String(values[parameter.name] ?? '').trim()) {
        nextFieldErrors[parameter.name] = t('builds.requiredParameterMissing').replace('{name}', parameter.name)
      }
    }
    if (values.RELEASE_ACTION === 'restart-version' && !String(values.RELEASE_VERSION || '').trim()) {
      nextFieldErrors.RELEASE_VERSION = t('builds.requiredParameterMissing').replace('{name}', 'RELEASE_VERSION')
    }
    if (environmentDefinition && !validReleaseEnvironment(String(values.RELEASE_ENVIRONMENT ?? ''))) {
      nextFieldErrors.RELEASE_ENVIRONMENT = t('projectDetail.invalidReleaseEnvironment')
    }
    setBranchError(nextBranchError)
    setFieldErrors(nextFieldErrors)

    const firstMissingParameter = definitions.find(parameter => nextFieldErrors[parameter.name])?.name
    if (nextBranchError || firstMissingParameter) {
      const message = nextBranchError || nextFieldErrors[firstMissingParameter as string]
      setError(message)
      focusFirstInvalid(Boolean(nextBranchError), firstMissingParameter)
      return
    }

    const parameters: Record<string, unknown> = {}
    definitions.forEach(parameter => {
      if (parameter.name === 'RELEASE_VERSION' && values.RELEASE_ACTION === 'publish-latest') return
      const value = values[parameter.name]
      if (parameter.type === 'number' && value !== '') parameters[parameter.name] = Number(value)
      else if (parameter.type === 'boolean') parameters[parameter.name] = Boolean(value)
      else if (String(value ?? '').length || parameter.required || parameter.defaultValue !== undefined) parameters[parameter.name] = value
    })

    setSaving(true)
    try {
      if (parameters.RELEASE_ACTION === 'publish-latest') {
        setConfirming(true)
        const sharedService = String(parameters.RELEASE_SHARED_SERVICE || '').trim()
        const confirmation = t(sharedService ? 'projectDetail.confirmPublishSharedLatestHelp' : 'projectDetail.confirmPublishLatestHelp')
          .replace('{service}', sharedService)
          .replace('{channel}', String(parameters.RELEASE_CHANNEL || '-'))
          .replace('{environment}', String(parameters.RELEASE_ENVIRONMENT || '-'))
          .replace('{branch}', branch.trim())
        const accepted = await dialogs.confirm(confirmation, {
          title: t('projectDetail.confirmPublishLatest'), action: t('projectDetail.confirmPublishLatest'),
        })
        setConfirming(false)
        if (!accepted) return
        parameters.CONFIRM_PUBLISH_LATEST = true
      } else if (parameters.RELEASE_ACTION) {
        parameters.CONFIRM_PUBLISH_LATEST = false
      }
      const build = await api.triggerBuild(project.id, {
        branch: branch.trim(),
        parameters,
      })
      onQueued(build)
    } catch (reason: any) {
      setError(reason?.message || t('builds.customBuildFailed'))
    } finally {
      setConfirming(false)
      setSaving(false)
    }
  }

  const branchDescription = `${formID}-branch-description`
  const branchErrorID = `${formID}-branch-error`
  const releaseParameterLabels: Record<string, string> = {
    RELEASE_CHANNEL: t('projectDetail.releaseChannel'),
    RELEASE_ENVIRONMENT: t('projectDetail.releaseEnvironment'),
    RELEASE_ACTION: t('projectDetail.releaseAction'),
    RELEASE_VERSION: t('projectDetail.historicalVersion'),
  }
  const visibleDefinitions = definitions.filter(parameter => !(parameter.name === 'CONFIRM_PUBLISH_LATEST' && definitions.some(definition => definition.name === 'RELEASE_ACTION')) && !(parameter.name === 'RELEASE_VERSION' && values.RELEASE_ACTION === 'publish-latest'))
  const environmentOptions = [...new Set([
    String(environmentDefinition?.defaultValue ?? ''),
    ...historicalEnvironments.filter(target => target.channel === values.RELEASE_CHANNEL).map(target => target.environment),
  ])].filter(validReleaseEnvironment)

  return <form className={`run-build-form ${variant === 'page' ? 'jenkins-parameter-form' : ''}`} onSubmit={handleSubmit} noValidate aria-busy={saving || loadingDefinitions || undefined}>
    <div className="run-build-body">
      <label className="run-build-branch" htmlFor={`${formID}-branch`}>
        <span><GitBranch size={14} aria-hidden="true" />{t('builds.branch')}<em>*</em></span>
        <input
          ref={branchRef}
          id={`${formID}-branch`}
          required
          data-dialog-initial-focus={variant === 'dialog' ? true : undefined}
          value={branch}
          disabled={saving}
          aria-invalid={branchError ? true : undefined}
          aria-describedby={`${branchDescription}${branchError ? ` ${branchErrorID}` : ''}`}
          onChange={event => {
            setBranch(event.target.value)
            setBranchError('')
          }}
        />
        <small id={branchDescription}>{t('builds.branchHelp')}</small>
        {branchError && <span className="run-field-error" id={branchErrorID}>{branchError}</span>}
      </label>

      <section className="run-parameter-section" aria-label={t('builds.buildParameters')}>
        <header>
          <div><SlidersHorizontal size={15} aria-hidden="true" /><strong>{t('builds.buildParameters')}</strong></div>
          <span>{t('builds.parameterCount').replace('{count}', String(visibleDefinitions.length))}</span>
        </header>
        {loadingDefinitions ? <div className="run-parameter-empty run-parameter-loading" role="status" aria-live="polite"><LoaderCircle className="timeline-spinner" size={16} aria-hidden="true" /><span>{t('common.loading')}</span></div> : definitionsFailed ? <div className="run-parameter-empty run-parameter-failure" role="alert"><span>{error}</span><button type="button" className="secondary-command" onClick={() => setDefinitionRequest(value => value + 1)}><RefreshCw size={14} aria-hidden="true" />{t('common.retry')}</button></div> : !definitions.length ? <p className="run-parameter-empty">{t('builds.noCustomParameters')}</p> : <div className="run-parameter-grid">
          {definitions.map((parameter, index) => {
            if (parameter.name === 'CONFIRM_PUBLISH_LATEST' && definitions.some(definition => definition.name === 'RELEASE_ACTION')) return null
            if (parameter.name === 'RELEASE_VERSION' && values.RELEASE_ACTION === 'publish-latest') return null
            const value = values[parameter.name]
            const controlID = `${formID}-parameter-${index}`
            const descriptionID = fieldDescriptionID(formID, index)
            const errorID = fieldErrorID(formID, index)
            const invalid = Boolean(fieldErrors[parameter.name])
            const environmentInput = parameter.name === 'RELEASE_ENVIRONMENT' && stringEnvironment
            const environmentHelpID = `${controlID}-environment-help`
            const describedBy = [parameter.description ? descriptionID : '', environmentInput ? environmentHelpID : '', invalid ? errorID : ''].filter(Boolean).join(' ') || undefined
            const sharedProps = {
              id: controlID,
              disabled: saving,
              'aria-invalid': invalid || undefined,
              'aria-describedby': describedBy,
            }
            return <label key={parameter.name} className={parameter.type === 'text' ? 'wide' : ''} htmlFor={controlID}>
              <span>{parameter.secret && <KeyRound size={12} aria-hidden="true" />}{releaseParameterLabels[parameter.name] || parameter.name}{(parameter.required || parameter.name === 'RELEASE_VERSION' && values.RELEASE_ACTION === 'restart-version') && <em>*</em>}</span>
              {parameter.type === 'choice' ? <select {...sharedProps} ref={element => { parameterRefs.current[parameter.name] = element }} value={String(value ?? '')} onChange={event => updateValue(parameter.name, event.target.value)}>
                {parameter.choices.map(choice => <option key={choice} value={choice}>{parameter.name === 'RELEASE_ACTION' ? releaseActionLabel(t, choice) : choice}</option>)}
              </select> : parameter.type === 'boolean' ? <span className="run-boolean-input"><input {...sharedProps} ref={element => { parameterRefs.current[parameter.name] = element }} type="checkbox" checked={value === true} onChange={event => updateValue(parameter.name, event.target.checked)} />{value === true ? t('common.enabled') : t('common.disabled')}</span> : parameter.type === 'text' ? <textarea {...sharedProps} ref={element => { parameterRefs.current[parameter.name] = element }} rows={3} value={String(value ?? '')} onChange={event => updateValue(parameter.name, event.target.value)} /> : <input {...sharedProps} ref={element => { parameterRefs.current[parameter.name] = element }} type={parameter.secret ? 'password' : parameter.type === 'number' ? 'number' : 'text'} list={environmentInput ? `${controlID}-environments` : undefined} value={String(value ?? '')} onChange={event => updateValue(parameter.name, event.target.value)} />}
              {environmentInput && <><datalist id={`${controlID}-environments`}>{environmentOptions.map(environment => <option key={environment} value={environment} />)}</datalist><small id={environmentHelpID}>{t('projectDetail.releaseEnvironmentHelp')}</small></>}
              {parameter.description && <small id={descriptionID}>{parameter.description}</small>}
              {invalid && <span className="run-field-error" id={errorID}>{fieldErrors[parameter.name]}</span>}
            </label>
          })}
        </div>}
      </section>
    </div>
    {error && !definitionsFailed && <p className="run-build-error" role="alert">{error}</p>}
    <footer>
      <button type="button" disabled={saving} onClick={() => { if (!saving) onCancel() }}>{t('common.cancel')}</button>
      <button type="submit" disabled={saving || loadingDefinitions || definitionsFailed}>
        {saving ? <LoaderCircle className="timeline-spinner" size={14} aria-hidden="true" /> : <CirclePlay size={14} aria-hidden="true" />}
        <span role={saving ? 'status' : undefined}>{confirming ? t('projectDetail.waitingPublishConfirmation') : saving ? t('builds.queueing') : t('builds.queueBuild')}</span>
      </button>
    </footer>
  </form>
}

export default function RunBuildDialog({ project, onClose, onQueued }: RunBuildDialogProps) {
  const { t } = useI18n()
  const [saving, setSaving] = useState(false)
  const requestClose = () => {
    if (!saving) onClose()
  }

  return <ModalDialog className="run-build-dialog" ariaLabel={t('builds.customBuildTitle')} busy={saving} onClose={requestClose}>
    <header>
      <div><CirclePlay size={18} aria-hidden="true" /><div><h2>{t('builds.customBuildTitle')}</h2><p>{t('builds.customBuildHelp').replace('{project}', project.name)}</p></div></div>
      <button type="button" disabled={saving} onClick={requestClose} title={t('common.close')} aria-label={t('common.close')}><X size={18} aria-hidden="true" /></button>
    </header>
    <BuildParametersForm project={project} variant="dialog" onCancel={requestClose} onQueued={onQueued} onBusyChange={setSaving} />
  </ModalDialog>
}
