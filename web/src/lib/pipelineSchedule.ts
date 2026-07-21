import { parseDocument } from 'yaml'
import { isTypeScriptPipelineSource } from './configFormat'

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
  const schedulePattern = /trigger\s*\(\s*(['"])schedule\1\s*,\s*\{\s*cron\s*:\s*(['"])(?:\\.|(?!\2)[^\\])*\2\s*\}\s*\)/
  const schedule = `trigger("schedule", { cron: ${quoteTypeScript(cron)} })`
  if (schedulePattern.test(next)) return next.replace(schedulePattern, schedule)
  const triggersPattern = /\btriggers\s*:\s*\[/
  if (triggersPattern.test(next)) return next.replace(triggersPattern, match => `${match}${schedule}, `)
  const pipelinePattern = /definePipeline\s*\(\s*\{/
  if (!pipelinePattern.test(next)) throw new Error('TypeScript pipeline must export definePipeline({ ... })')
  return next.replace(pipelinePattern, match => `${match}\n  triggers: [${schedule}],`)
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
