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

function markdownApproval(source: string): ApprovalPolicySettings {
  const section = source.match(/^## Approval\s*$([\s\S]*?)(?=^##\s|(?![\s\S]))/m)?.[1] || ''
  if (!section) return { ...defaults }
  const values = new Map<string, string>()
  for (const match of section.matchAll(/^\s*-\s*([A-Za-z_][\w-]*)\s*:\s*(.+?)\s*$/gm)) {
    values.set(match[1].toLowerCase(), match[2].replace(/^['"]|['"]$/g, '').trim())
  }
  const roles = (values.get('required_roles') || values.get('roles') || 'admin')
    .split(',')
    .map(role => role.trim())
    .filter((role): role is 'admin' | 'developer' => role === 'admin' || role === 'developer')
  return {
    enabled: (values.get('strategy') || 'none').toLowerCase() !== 'none',
    requiredRoles: roles.length ? roles : ['admin'],
    allowRequester: (values.get('allow_requester') || 'true').toLowerCase() !== 'false',
    prompt: values.get('prompt') || '',
  }
}

export function readApprovalPolicy(source: string): ApprovalPolicySettings {
  const trimmed = source.trim()
  if (!trimmed) return { ...defaults }
  if (trimmed.startsWith('#')) return markdownApproval(source)
  const parsed = trimmed.startsWith('{') ? JSON.parse(source) : parseYAML(source)
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

function writeMarkdownApproval(source: string, settings: ApprovalPolicySettings): string {
  const sectionPattern = /^## Approval\s*$[\s\S]*?(?=^##\s|(?![\s\S]))/m
  if (!settings.enabled) {
    return source.replace(sectionPattern, '').replace(/\n{3,}/g, '\n\n').trimEnd() + '\n'
  }
  const roles = settings.requiredRoles.length ? settings.requiredRoles : ['admin']
  const section = [
    '## Approval',
    '- version: 1',
    '- strategy: single',
    `- required_roles: ${roles.join(', ')}`,
    `- allow_requester: ${settings.allowRequester}`,
    ...(settings.prompt.trim() ? [`- prompt: ${settings.prompt.trim().replaceAll('\n', ' ')}`] : []),
    '',
  ].join('\n')
  if (sectionPattern.test(source)) return source.replace(sectionPattern, section).replace(/\n{3,}/g, '\n\n').trimEnd() + '\n'
  const pipeline = source.search(/^## Pipeline\s*$/m)
  if (pipeline >= 0) return `${source.slice(0, pipeline).trimEnd()}\n\n${section}\n${source.slice(pipeline).trimStart()}`.trimEnd() + '\n'
  return `${source.trimEnd()}\n\n${section}`.trimEnd() + '\n'
}

export function writeApprovalPolicy(source: string, settings: ApprovalPolicySettings): string {
  const trimmed = source.trim()
  if (trimmed.startsWith('#')) return writeMarkdownApproval(source, settings)
  const parsed = trimmed ? (trimmed.startsWith('{') ? JSON.parse(source) : parseYAML(source)) : {}
  if (!parsed || typeof parsed !== 'object' || Array.isArray(parsed)) throw new Error('Build configuration must be an object')
  const policy = configPolicy(settings)
  if (policy) parsed.approval = policy
  else delete parsed.approval
  return trimmed.startsWith('{') ? `${JSON.stringify(parsed, null, 2)}\n` : stringifyYAML(parsed, { indent: 2 })
}
