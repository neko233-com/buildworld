import { validReleaseEnvironment } from './releasePresentation'

export type ConfiguredReleaseTarget = {
  channel: string
  environment: string
  channelLabel?: string
  environmentLabel?: string
}

export type ReleaseTargetCatalog = { schemaVersion: 1; targets: ConfiguredReleaseTarget[] }

export function normalizeReleaseTargetCatalog(value: unknown): ReleaseTargetCatalog | null {
  if (!value || typeof value !== 'object' || Array.isArray(value)) return null
  const source = value as Record<string, unknown>
  if (Object.keys(source).some(key => key !== 'schemaVersion' && key !== 'targets') || source.schemaVersion !== 1 || !Array.isArray(source.targets) || !source.targets.length || source.targets.length > 512) return null
  const seen = new Set<string>()
  const channelLabels = new Map<string, string>()
  const targets: ConfiguredReleaseTarget[] = []
  for (const candidate of source.targets) {
    if (!candidate || typeof candidate !== 'object' || Array.isArray(candidate)) return null
    const target = candidate as Record<string, unknown>
    if (Object.keys(target).some(key => !['channel', 'environment', 'channelLabel', 'environmentLabel'].includes(key))) return null
    if (typeof target.channel !== 'string' || typeof target.environment !== 'string' || !validReleaseEnvironment(target.channel) || !validReleaseEnvironment(target.environment)) return null
    const key = `${target.channel}/${target.environment}`
    if (seen.has(key)) return null
    for (const label of [target.channelLabel, target.environmentLabel]) {
      if (label !== undefined && (typeof label !== 'string' || [...label].length > 80 || label.trim() !== label || [...label].some(character => { const code = character.codePointAt(0)!; return code < 32 || code >= 127 && code <= 159 }))) return null
    }
    const channelLabel = String(target.channelLabel || '')
    if (channelLabels.has(target.channel) && channelLabels.get(target.channel) !== channelLabel) return null
    channelLabels.set(target.channel, channelLabel)
    seen.add(key)
    targets.push({ channel: target.channel, environment: target.environment, channelLabel: target.channelLabel as string | undefined, environmentLabel: target.environmentLabel as string | undefined })
  }
  return { schemaVersion: 1, targets }
}

export function preferredReleaseEnvironment(targets: ConfiguredReleaseTarget[], channel: string, pipelineDefault = ''): string {
  const choices = targets.filter(target => target.channel === channel)
  return choices.find(target => target.environment === 'test')?.environment
    || choices.find(target => target.environment === pipelineDefault)?.environment
    || choices[0]?.environment || ''
}

export function configuredReleaseTarget(targets: ConfiguredReleaseTarget[], channel: unknown, environment: unknown): boolean {
  return targets.some(target => target.channel === channel && target.environment === environment)
}
