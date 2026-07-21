function ensureConfigurationRoot(value: unknown) {
  if (value === null || typeof value !== 'object' || Array.isArray(value)) {
    throw new Error('Configuration root must be an object')
  }
}

function ensureSupportedPipelineSource(source: string) {
  const trimmed = source.trim()
  if (!trimmed) throw new Error('Configuration cannot be empty')
  if (isTypeScriptPipelineSource(source)) return 'typescript' as const
  if (trimmed.startsWith('{') || trimmed.startsWith('[')) {
    throw new Error('JSON pipeline configs are not supported. Use TypeScript or jobs-based YAML.')
  }
  if (trimmed.startsWith('#')) {
    throw new Error('Markdown pipeline configs are not supported. Use TypeScript or jobs-based YAML.')
  }
  return 'yaml' as const
}

export function isTypeScriptPipelineSource(source: string) {
  const trimmed = source.trim()
  return trimmed.includes('@buildworld/pipeline') || trimmed.startsWith('// buildworld-pipeline: ts') || trimmed.startsWith('import ') || trimmed.includes('export default definePipeline')
}

export type PipelineSourceLanguage = 'typescript' | 'yaml'

export function resolvePipelineSourceLanguage(source: string, fallback: PipelineSourceLanguage = 'yaml'): PipelineSourceLanguage {
  if (isTypeScriptPipelineSource(source)) return 'typescript'
  if (/^jobs\s*:/m.test(source)) return 'yaml'
  return fallback
}

export function prettyConfigSourceSync(source: string): string {
  const trimmed = source.trim()
  if (!trimmed) return source
  if (isTypeScriptPipelineSource(source)) return `${trimmed}\n`
  if (!trimmed.startsWith('{')) return source
  const value = JSON.parse(trimmed)
  ensureConfigurationRoot(value)
  return `${JSON.stringify(value, null, 2)}\n`
}

export async function prettyConfigSource(source: string): Promise<string> {
  const trimmed = source.trim()
  if (!trimmed) throw new Error('Configuration cannot be empty')
  if (isTypeScriptPipelineSource(source)) return `${trimmed}\n`
  if (trimmed.startsWith('{')) return prettyConfigSourceSync(source)
  const { parseDocument } = await import('yaml')
  const document = parseDocument(trimmed)
  if (document.errors.length) throw document.errors[0]
  ensureConfigurationRoot(document.toJS())
  return `${document.toString({ indent: 2, lineWidth: 0 }).trimEnd()}\n`
}

export function prettyPipelineSourceSync(source: string): string {
  const trimmed = source.trim()
  if (!trimmed) return source
  const format = ensureSupportedPipelineSource(source)
  return format === 'typescript' ? `${trimmed}\n` : source
}

export async function prettyPipelineSource(source: string): Promise<string> {
  const trimmed = source.trim()
  const format = ensureSupportedPipelineSource(source)
  if (format === 'typescript') return `${trimmed}\n`

  const { parseDocument } = await import('yaml')
  const document = parseDocument(trimmed)
  if (document.errors.length) throw document.errors[0]
  const value = document.toJS()
  ensureConfigurationRoot(value)
  const jobs = (value as Record<string, unknown>).jobs
  if (!jobs || typeof jobs !== 'object' || Array.isArray(jobs) || Object.keys(jobs).length === 0) {
    throw new Error('YAML pipeline config must contain a non-empty jobs mapping')
  }
  return `${document.toString({ indent: 2, lineWidth: 0 }).trimEnd()}\n`
}
