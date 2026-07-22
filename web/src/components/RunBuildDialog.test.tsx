// @vitest-environment jsdom

import { describe, expect, it } from 'vitest'
import { normalizeBuildParameterDefinitions, parseBuildParameterDefinitions } from './RunBuildDialog'

describe('custom build parameter definitions', () => {
  it('normalizes server-parsed parameters and removes duplicate names', () => {
    const definitions = normalizeBuildParameterDefinitions([
      { name: 'target', type: 'choice', choices: ['dev', 'prod'], default: 'prod', required: true },
      { name: 'token', type: 'password', is_secret: true },
      { name: 'target', type: 'string' },
    ])

    expect(definitions).toHaveLength(2)
    expect(definitions[0]).toMatchObject({ name: 'target', type: 'choice', choices: ['dev', 'prod'], defaultValue: 'prod', required: true })
    expect(definitions[1]).toMatchObject({ name: 'token', type: 'password', secret: true })
  })

  it('reads YAML parameters and safely ignores invalid configuration', () => {
    expect(parseBuildParameterDefinitions(`
parameters:
  - name: release
    type: boolean
    default: true
`)).toMatchObject([{ name: 'release', type: 'boolean', defaultValue: true }])
    expect(parseBuildParameterDefinitions('{ invalid')).toEqual([])
  })

  it('does not locally interpret TypeScript; authoritative metadata comes from validation API', () => {
    expect(parseBuildParameterDefinitions("import { definePipeline } from '@buildworld/pipeline'\nexport default definePipeline({ parameters: [] , stages: [] })")).toEqual([])
  })
})
