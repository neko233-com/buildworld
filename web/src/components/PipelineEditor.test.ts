// @vitest-environment jsdom

import { describe, expect, it } from 'vitest'
import { parseDocument, sourceToMarkdown } from './PipelineEditor'

describe('sourceToMarkdown', () => {
  it('normalizes imported job steps and preserves the requested shell runtime', () => {
    const source = `
name: Release
jobs:
  build:
    steps:
      - name: Compile
        run: npm run build
        shell: cmd
`

    expect(sourceToMarkdown(source, 'fallback')).toContain(
      '### Compile\n\n```default shell cmd\nnpm run build\n```',
    )
  })

  it('supports legacy stages and environment variables', () => {
    const source = JSON.stringify({
      name: 'Notify',
      environment: { CHANNEL: 'release' },
      stages: [{
        name: 'Publish',
        steps: [{ name: 'Announce', type: 'notify', command: 'event: build.completed' }],
      }],
    })

    const result = sourceToMarkdown(source, 'fallback')
    expect(result).toContain('- CHANNEL: release')
    expect(result).toContain('```default notify\nevent: build.completed\n```')
  })

  it('preserves the approval gate while enabling the visual graph', () => {
    const source = JSON.stringify({
      approval: {
        version: 1,
        strategy: 'single',
        required_roles: ['admin', 'developer'],
        allow_requester: false,
        prompt: 'Release?',
      },
      stages: [{ name: 'Build', steps: [{ name: 'Package', type: 'shell', command: 'npm pack' }] }],
    })

    const result = sourceToMarkdown(source, 'fallback')
    expect(result).toContain('## Approval')
    expect(result).toContain('- required_roles: admin, developer')
    expect(result).toContain('- allow_requester: false')
    expect(result).toContain('- prompt: Release?')
  })

  it('refuses a lossy migration when build parameters are present', () => {
    const source = JSON.stringify({
      parameters: [{ name: 'release_tag', required: true }],
      stages: [{ name: 'Build', steps: [{ name: 'Package', command: 'npm pack' }] }],
    })
    expect(sourceToMarkdown(source, 'fallback')).toBeNull()
  })

  it('rejects invalid or non-executable configurations', () => {
    expect(sourceToMarkdown('not: [valid', 'fallback')).toBeNull()
    expect(sourceToMarkdown('name: empty', 'fallback')).toBeNull()
  })
})

describe('parseDocument', () => {
  it('links sequential steps and platform overrides', () => {
    const nodes = parseDocument(`# Build

## Pipeline

### Compile

\`\`\`default shell
npm run build
\`\`\`

\`\`\`windows shell powershell
npm run build:windows
\`\`\`

### Test

\`\`\`default shell
npm test
\`\`\`
`)

    expect(nodes.map((node) => node.title)).toEqual(['Compile', 'Compile (Windows)', 'Test'])
    expect(nodes[1].dependencies).toEqual([nodes[0].id])
    expect(nodes[2].dependencies).toEqual([nodes[0].id])
    expect(nodes[1].runtime).toBe('powershell')
  })
})
