import { parse as parseYAML, stringify as stringifyYAML } from 'yaml'

export type ApprovalPolicySettings = {
  enabled: boolean
  requiredRoles: Array<'admin' | 'developer'>
  allowRequester: boolean
  prompt: string
}

const defaults: ApprovalPolicySettings = {
  enabled: false,
  requiredRoles: ['admin'],
  allowRequester: true,
  prompt: '',
}

function normalizePolicy(policy: any): ApprovalPolicySettings {
  if (!policy || String(policy.strategy || 'none').toLowerCase() === 'none') return { ...defaults }
  const roles = Array.isArray(policy.required_roles)
    ? policy.required_roles.filter((role: unknown): role is 'admin' | 'developer' => role === 'admin' || role === 'developer')
    : []
  return {
    enabled: true,
    requiredRoles: roles.length ? roles : ['admin'],
    allowRequester: policy.allow_requester !== false,
    prompt: String(policy.prompt || ''),
  }
}

export function readApprovalPolicy(source: string): ApprovalPolicySettings {
  const trimmed = source.trim()
  if (!trimmed) return { ...defaults }
  if (trimmed.startsWith('{') || trimmed.startsWith('[') || trimmed.startsWith('#')) {
    throw new Error('Approval settings require jobs-based YAML')
  }
  const parsed = parseYAML(source)
  return normalizePolicy(parsed?.approval)
}

function configPolicy(settings: ApprovalPolicySettings) {
  if (!settings.enabled) return undefined
  return {
    version: 1,
    strategy: 'single',
    required_roles: settings.requiredRoles.length ? settings.requiredRoles : ['admin'],
    allow_requester: settings.allowRequester,
    ...(settings.prompt.trim() ? { prompt: settings.prompt.trim() } : {}),
  }
}

export function writeApprovalPolicy(source: string, settings: ApprovalPolicySettings): string {
  const trimmed = source.trim()
  if (trimmed.startsWith('{') || trimmed.startsWith('[') || trimmed.startsWith('#')) {
    throw new Error('Approval settings require jobs-based YAML')
  }
  const parsed = trimmed ? parseYAML(source) : {}
  if (!parsed || typeof parsed !== 'object' || Array.isArray(parsed)) throw new Error('Build configuration must be an object')
  const policy = configPolicy(settings)
  if (policy) parsed.approval = policy
  else delete parsed.approval
  return stringifyYAML(parsed, { indent: 2 })
}
