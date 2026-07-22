import { readdirSync, readFileSync } from 'node:fs'
import { extname, join } from 'node:path'
import { fileURLToPath } from 'node:url'
import { describe, expect, it } from 'vitest'

const sourceRoot = fileURLToPath(new URL('../', import.meta.url))
const forbiddenDatePresentation = /\b(?:toLocaleString|toLocaleDateString|toLocaleTimeString)\s*\(|\bIntl\.(?:DateTimeFormat|RelativeTimeFormat)\s*\(/

function sourceFiles(directory: string): string[] {
  return readdirSync(directory, { withFileTypes: true }).flatMap(entry => {
    const path = join(directory, entry.name)
    if (entry.isDirectory()) return sourceFiles(path)
    if (!['.ts', '.tsx'].includes(extname(entry.name)) || entry.name.includes('.test.')) return []
    return [path]
  })
}

describe('date and time presentation policy', () => {
  it('keeps locale-dependent date formatting out of production Web source', () => {
    const violations = sourceFiles(sourceRoot).flatMap(path => {
      const source = readFileSync(path, 'utf8')
      return forbiddenDatePresentation.test(source) ? [path.slice(sourceRoot.length + 1)] : []
    })

    expect(violations).toEqual([])
  })
})
