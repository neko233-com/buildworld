export type PortabilityCapability = {
  key: string
  version: number
  sensitive: boolean
  description?: string
  depends_on?: string[]
}

export type SettingsSection = 'overview' | 'runtime' | 'builds' | 'agents' | 'proxies' | 'validation' | 'security' | 'portability'

export const MAX_PORTABILITY_BUNDLE_BYTES = 32 * 1024 * 1024

const settingsSections = new Set<SettingsSection>([
  'overview',
  'runtime',
  'builds',
  'agents',
  'proxies',
  'validation',
  'security',
  'portability',
])

export function resolveSettingsSection(value: string | null): SettingsSection {
  return value && settingsSections.has(value as SettingsSection) ? value as SettingsSection : 'overview'
}

export function safePortabilitySections(capabilities: PortabilityCapability[]): string[] {
  return capabilities.filter(capability => !capability.sensitive).map(capability => capability.key)
}

export function completePortabilitySections(capabilities: PortabilityCapability[]): string[] {
  return capabilities.map(capability => capability.key)
}

export function sanitizePortabilitySections(
  capabilities: PortabilityCapability[],
  selected: string[],
  includeSecrets: boolean,
): string[] {
  const allowed = new Set(
    capabilities
      .filter(capability => includeSecrets || !capability.sensitive)
      .map(capability => capability.key),
  )
  return selected.filter((key, index) => allowed.has(key) && selected.indexOf(key) === index)
}

export function togglePortabilitySection(
  selected: string[],
  capability: PortabilityCapability,
  includeSecrets: boolean,
): { sections: string[]; blockedBySecrets: boolean } {
  if (capability.sensitive && !includeSecrets && !selected.includes(capability.key)) {
    return { sections: selected, blockedBySecrets: true }
  }
  return {
    sections: selected.includes(capability.key)
      ? selected.filter(key => key !== capability.key)
      : [...selected, capability.key],
    blockedBySecrets: false,
  }
}

export function hasSensitiveSelection(capabilities: PortabilityCapability[], selected: string[]): boolean {
  const sensitive = new Set(capabilities.filter(capability => capability.sensitive).map(capability => capability.key))
  return selected.some(key => sensitive.has(key))
}

export function missingPortabilityDependencies(
  capabilities: PortabilityCapability[],
  selected: string[],
): Array<{ section: string; dependency: string }> {
  const selectedKeys = new Set(selected)
  const result: Array<{ section: string; dependency: string }> = []
  for (const capability of capabilities) {
    if (!selectedKeys.has(capability.key)) continue
    for (const dependency of capability.depends_on || []) {
      if (!selectedKeys.has(dependency)) result.push({ section: capability.key, dependency })
    }
  }
  return result
}

export function humanizePortabilityKey(key: string): string {
  return key
    .split('_')
    .filter(Boolean)
    .map(word => word[0]?.toUpperCase() + word.slice(1))
    .join(' ')
}

export function isPortabilityBundleSizeAllowed(size: number): boolean {
  return Number.isFinite(size) && size >= 0 && size <= MAX_PORTABILITY_BUNDLE_BYTES
}
