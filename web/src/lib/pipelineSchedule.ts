import { parseDocument } from 'yaml'
import { isTypeScriptPipelineSource } from './configFormat'

const typescriptSchedulePattern = /trigger\s*\(\s*(['"])schedule\1\s*,\s*\{\s*cron\s*:\s*(['"])((?:\\.|(?!\2)[^\\])*)\2\s*\}\s*\)/

function quoteTypeScript(value: string) {
  return JSON.stringify(value)
}

function writeTypeScriptSchedule(source: string, cron: string): string {
  let next = source
  const importPattern = /import\s*\{([^}]*)\}\s*from\s*(['"])@buildworld\/pipeline\2/
  const importMatch = next.match(importPattern)
  if (!importMatch) throw new Error('TypeScript pipeline must import helpers from @buildworld/pipeline')
  const imports = importMatch[1].split(',').map(value => value.trim()).filter(Boolean)
  if (!imports.includes('trigger')) {
    next = next.replace(importPattern, `import { ${[...imports, 'trigger'].join(', ')} } from ${importMatch[2]}@buildworld/pipeline${importMatch[2]}`)
  }
  const schedule = `trigger("schedule", { cron: ${quoteTypeScript(cron)} })`
  if (typescriptSchedulePattern.test(next)) return next.replace(typescriptSchedulePattern, schedule)
  const triggersPattern = /\btriggers\s*:\s*\[/
  if (triggersPattern.test(next)) return next.replace(triggersPattern, match => `${match}${schedule}, `)
  const pipelinePattern = /definePipeline\s*\(\s*\{/
  if (!pipelinePattern.test(next)) throw new Error('TypeScript pipeline must export definePipeline({ ... })')
  return next.replace(pipelinePattern, match => `${match}\n  triggers: [${schedule}],`)
}

/** Returns the first supported scheduler cron expression, if one exists. */
export function readScheduleFromPipeline(source: string): string {
  if (isTypeScriptPipelineSource(source)) {
    const match = source.match(typescriptSchedulePattern)
    if (!match) return ''
    try { return JSON.parse(`${match[2]}${match[3]}${match[2]}`) }
    catch { return match[3] }
  }

  const document = parseDocument(source)
  if (document.errors.length > 0) return ''
  let schedule: unknown
  try { schedule = document.getIn(['on', 'schedule']) }
  catch { return '' }
  const value = schedule && typeof schedule === 'object' && 'toJSON' in schedule
    ? (schedule as { toJSON: () => unknown }).toJSON()
    : schedule
  if (Array.isArray(value)) {
    const first = value.find(item => item && typeof item === 'object' && typeof (item as { cron?: unknown }).cron === 'string')
    return first ? String((first as { cron: string }).cron) : ''
  }
  if (value && typeof value === 'object' && typeof (value as { cron?: unknown }).cron === 'string') {
    return String((value as { cron: string }).cron)
  }
  return ''
}

/** Removes the supported scheduler trigger while retaining the rest of the source. */
export function removeScheduleFromPipeline(source: string): string {
  if (isTypeScriptPipelineSource(source)) {
    if (!typescriptSchedulePattern.test(source)) return source
    return source
      .replace(typescriptSchedulePattern, '')
      .replace(/\[\s*,/, '[')
      .replace(/,\s*,/g, ',')
      .replace(/,\s*\]/g, ']')
  }

  const document = parseDocument(source)
  if (document.errors.length > 0) throw new Error(document.errors[0].message)
  let schedule: unknown
  try { schedule = document.getIn(['on', 'schedule']) }
  catch { return source }
  if (schedule === undefined || schedule === null) return source
  document.deleteIn(['on', 'schedule'])
  const on = document.get('on')
  const value = on && typeof on === 'object' && 'toJSON' in on
    ? (on as { toJSON: () => unknown }).toJSON()
    : on
  if (value && typeof value === 'object' && !Array.isArray(value) && Object.keys(value).length === 0) document.delete('on')
  return document.toString()
}

/**
 * Writes one scheduler trigger while retaining TypeScript or jobs-based YAML.
 */
export function writeScheduleToPipeline(source: string, cron: string): string {
  const trimmed = source.trim()
  if (isTypeScriptPipelineSource(source)) return writeTypeScriptSchedule(source, cron)
  if (trimmed.startsWith('{') || trimmed.startsWith('[') || trimmed.startsWith('#')) throw new Error('Only TypeScript and jobs-based YAML pipelines are supported')

  const document = parseDocument(source)
  if (document.errors.length > 0) throw new Error(document.errors[0].message)
  document.setIn(['on', 'schedule'], [{ cron }])
  return document.toString()
}
