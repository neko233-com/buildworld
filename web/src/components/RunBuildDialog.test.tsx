// @vitest-environment jsdom

import { describe, expect, it } from 'vitest'
import { parseBuildParameterDefinitions, requiresBuildParameterInput } from './RunBuildDialog'

describe('custom build parameter definitions', () => {
  it('normalizes JSON parameters and removes duplicate names', () => {
    const definitions = parseBuildParameterDefinitions(JSON.stringify({
      parameters: [
        { name: 'target', type: 'choice', choices: ['dev', 'prod'], default: 'prod', required: true },
        { name: 'token', type: 'password', is_secret: true },
        { name: 'target', type: 'string' },
      ],
    }))

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

  it('only requires the dialog when a required value has no usable default', () => {
    expect(requiresBuildParameterInput(JSON.stringify({
      parameters: [
        { name: 'target', type: 'choice', choices: ['staging', 'production'], required: true },
        { name: 'release', type: 'boolean', required: true },
      ],
    }))).toBe(false)
    expect(requiresBuildParameterInput(JSON.stringify({
      parameters: [{ name: 'api_token', type: 'password', required: true }],
    }))).toBe(true)
    expect(requiresBuildParameterInput(JSON.stringify({
      parameters: [{ name: 'target', type: 'string', required: true, default: 'production' }],
    }))).toBe(false)
  })
})
