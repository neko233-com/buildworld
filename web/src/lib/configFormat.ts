function ensureConfigurationRoot(value: unknown) {
  if (value === null || typeof value !== 'object' || Array.isArray(value)) {
    throw new Error('Configuration root must be an object')
  }
}

export function prettyConfigSourceSync(source: string): string {
  const trimmed = source.trim()
  if (!trimmed) return source
  if (/^#\s+.+/m.test(trimmed) && /^##\s+Pipeline\s*$/m.test(trimmed)) return `${trimmed}\n`
  if (!trimmed.startsWith('{')) return source

  const value = JSON.parse(trimmed)
  ensureConfigurationRoot(value)
  return `${JSON.stringify(value, null, 2)}\n`
}

export async function prettyConfigSource(source: string): Promise<string> {
  const trimmed = source.trim()
  if (!trimmed) throw new Error('Configuration cannot be empty')
  if (/^#\s+.+/m.test(trimmed) && /^##\s+Pipeline\s*$/m.test(trimmed)) return `${trimmed}\n`
  if (trimmed.startsWith('{')) return prettyConfigSourceSync(source)

  const { parseDocument } = await import('yaml')
  const document = parseDocument(trimmed)
  if (document.errors.length) throw document.errors[0]
  ensureConfigurationRoot(document.toJS())
  return `${document.toString({ indent: 2, lineWidth: 0 }).trimEnd()}\n`
}
