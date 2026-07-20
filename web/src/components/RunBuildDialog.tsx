import { useMemo, useState } from 'react'
import { CirclePlay, GitBranch, KeyRound, SlidersHorizontal, X } from 'lucide-react'
import { parse as parseYAML } from 'yaml'
import { api } from '../api'
import { useI18n } from '../i18n'
import { dialogs } from './AppDialogs'
import { ModalDialog } from './ModalDialog'

export type BuildParameterDefinition = {
  name: string
  type: 'string' | 'text' | 'password' | 'boolean' | 'choice' | 'number'
  description: string
  defaultValue: unknown
  required: boolean
  choices: string[]
  secret: boolean
}

type BuildProject = {
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

const supportedTypes = new Set<BuildParameterDefinition['type']>([
  'string',
  'text',
  'password',
  'boolean',
  'choice',
  'number',
])

export function parseBuildParameterDefinitions(source = ''): BuildParameterDefinition[] {
  if (!source.trim()) return []
  let config: any
  try {
    config = source.trimStart().startsWith('{') ? JSON.parse(source) : parseYAML(source)
  } catch {
    return []
  }
  if (!Array.isArray(config?.parameters)) return []

  const seen = new Set<string>()
  return config.parameters.flatMap((parameter: any) => {
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

export function requiresBuildParameterInput(source = ''): boolean {
  return parseBuildParameterDefinitions(source).some(parameter => {
    if (!parameter.required) return false
    if (parameter.type === 'boolean') return false
    if (parameter.type === 'choice' && parameter.choices.length > 0) return false
    if (parameter.defaultValue === undefined || parameter.defaultValue === null) return true
    return typeof parameter.defaultValue === 'string' && !parameter.defaultValue.trim()
  })
}

function initialParameterValues(definitions: BuildParameterDefinition[]): Record<string, string | boolean> {
  return Object.fromEntries(definitions.map(parameter => {
    if (parameter.defaultValue !== undefined && parameter.defaultValue !== null) {
      return [parameter.name, parameter.type === 'boolean' ? Boolean(parameter.defaultValue) : String(parameter.defaultValue)]
    }
    if (parameter.type === 'boolean') return [parameter.name, false]
    if (parameter.type === 'choice' && parameter.choices.length) return [parameter.name, parameter.choices[0]]
    return [parameter.name, '']
  }))
}

export default function RunBuildDialog({ project, onClose, onQueued }: RunBuildDialogProps) {
  const { t } = useI18n()
  const definitions = useMemo(() => parseBuildParameterDefinitions(project.config), [project.config])
  const [branch, setBranch] = useState(project.default_branch || 'main')
  const [values, setValues] = useState<Record<string, string | boolean>>(() => initialParameterValues(definitions))
  const [saving, setSaving] = useState(false)
  const [error, setError] = useState('')

  const updateValue = (name: string, value: string | boolean) => {
    setValues(current => ({ ...current, [name]: value }))
  }

  const handleSubmit = async (event: React.FormEvent) => {
    event.preventDefault()
    setError('')
    const missing = definitions.find(parameter => parameter.required && parameter.type !== 'boolean' && !String(values[parameter.name] ?? '').trim())
    if (missing) {
      const message = t('builds.requiredParameterMissing').replace('{name}', missing.name)
      setError(message)
      dialogs.notify(message)
      return
    }

    const parameters: Record<string, unknown> = {}
    definitions.forEach(parameter => {
      const value = values[parameter.name]
      if (parameter.type === 'number' && value !== '') parameters[parameter.name] = Number(value)
      else if (parameter.type === 'boolean') parameters[parameter.name] = Boolean(value)
      else if (String(value ?? '').length || parameter.required || parameter.defaultValue !== undefined) parameters[parameter.name] = value
    })

    setSaving(true)
    try {
      const build = await api.triggerBuild(project.id, {
        branch: branch.trim() || project.default_branch || 'main',
        parameters,
      })
      onQueued(build)
    } catch (reason: any) {
      const message = reason.message || t('builds.customBuildFailed')
      setError(message)
      if (reason?.name !== 'ApiError') dialogs.notify(message)
    } finally {
      setSaving(false)
    }
  }

  return (
    <ModalDialog className="run-build-dialog" ariaLabel={t('builds.customBuildTitle')} busy={saving} onClose={onClose}>
      <header>
        <div><CirclePlay size={18} /><div><h2>{t('builds.customBuildTitle')}</h2><p>{t('builds.customBuildHelp').replace('{project}', project.name)}</p></div></div>
        <button type="button" onClick={onClose} title={t('common.close')} aria-label={t('common.close')}><X size={18} /></button>
      </header>
      <form className="run-build-form" onSubmit={handleSubmit}>
        <div className="run-build-body">
          <label className="run-build-branch">
            <span><GitBranch size={14} />{t('builds.branch')}</span>
            <input required data-dialog-initial-focus value={branch} onChange={event => setBranch(event.target.value)} />
            <small>{t('builds.branchHelp')}</small>
          </label>

          <section className="run-parameter-section" aria-label={t('builds.buildParameters')}>
            <header>
              <div><SlidersHorizontal size={15} /><strong>{t('builds.buildParameters')}</strong></div>
              <span>{t('builds.parameterCount').replace('{count}', String(definitions.length))}</span>
            </header>
            {!definitions.length ? <p className="run-parameter-empty">{t('builds.noCustomParameters')}</p> : (
              <div className="run-parameter-grid">
                {definitions.map(parameter => {
                  const value = values[parameter.name]
                  return <label key={parameter.name} className={parameter.type === 'text' ? 'wide' : ''}>
                    <span>{parameter.secret && <KeyRound size={12} />}{parameter.name}{parameter.required && <em>*</em>}</span>
                    {parameter.type === 'choice' ? (
                      <select value={String(value ?? '')} onChange={event => updateValue(parameter.name, event.target.value)}>
                        {parameter.choices.map(choice => <option key={choice} value={choice}>{choice}</option>)}
                      </select>
                    ) : parameter.type === 'boolean' ? (
                      <span className="run-boolean-input"><input type="checkbox" checked={value === true} onChange={event => updateValue(parameter.name, event.target.checked)} />{value === true ? t('common.enabled') : t('common.disabled')}</span>
                    ) : parameter.type === 'text' ? (
                      <textarea rows={3} value={String(value ?? '')} onChange={event => updateValue(parameter.name, event.target.value)} />
                    ) : (
                      <input type={parameter.secret ? 'password' : parameter.type === 'number' ? 'number' : 'text'} value={String(value ?? '')} onChange={event => updateValue(parameter.name, event.target.value)} />
                    )}
                    {parameter.description && <small>{parameter.description}</small>}
                  </label>
                })}
              </div>
            )}
          </section>
        </div>
        {error && <p className="run-build-error" role="alert">{error}</p>}
        <footer>
          <button type="button" disabled={saving} onClick={onClose}>{t('common.cancel')}</button>
          <button type="submit" disabled={saving}><CirclePlay size={14} />{saving ? t('builds.queueing') : t('builds.queueBuild')}</button>
        </footer>
      </form>
    </ModalDialog>
  )
}
