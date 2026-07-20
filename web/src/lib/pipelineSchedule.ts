import { parseDocument } from 'yaml'

function quoteMarkdown(value: string): string {
  return value.replaceAll('\\', '\\\\').replaceAll('"', '\\"')
}

function upsertMarkdownSchedule(source: string, cron: string): string {
  const heading = /^##[ \t]+Schedule[ \t]*\r?$/gim
  const current = heading.exec(source)
  const section = `## Schedule\n\n- cron: "${quoteMarkdown(cron)}"\n`
  if (!current) return `${source.trimEnd()}\n\n${section}`

  const nextHeading = /^##[ \t]+.+$/gm
  nextHeading.lastIndex = heading.lastIndex
  const next = nextHeading.exec(source)
  const end = next?.index ?? source.length
  return `${source.slice(0, current.index)}${section}\n${source.slice(end).trimStart()}`
}

/**
 * Writes one scheduler trigger while retaining the project's native pipeline format.
 * JSON uses Buildworld's `triggers` model, YAML uses its GitHub-compatible `on`
 * model, and Markdown remains the single editable source for Markdown pipelines.
 */
export function writeScheduleToPipeline(source: string, cron: string): string {
  const trimmed = source.trim()
  if (trimmed.startsWith('#')) return upsertMarkdownSchedule(source, cron)

  if (trimmed.startsWith('{')) {
    const parsed = JSON.parse(source) as Record<string, unknown>
    const existing = Array.isArray(parsed.triggers) ? parsed.triggers : []
    parsed.triggers = [
      ...existing.filter((trigger: any) => trigger?.type !== 'schedule'),
      { type: 'schedule', config: { cron } },
    ]
    return JSON.stringify(parsed, null, 2)
  }

  const document = parseDocument(source)
  if (document.errors.length > 0) throw new Error(document.errors[0].message)
  document.setIn(['on', 'schedule'], [{ cron }])
  return document.toString()
}
