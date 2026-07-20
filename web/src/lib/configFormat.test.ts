import { describe, expect, it } from 'vitest'
import { isTypeScriptPipelineSource, prettyConfigSource, prettyConfigSourceSync } from './configFormat'

describe('configuration formatting', () => {
  it('pretty prints compact JSON without changing its value', () => {
    const source = '{"stages":[{"name":"Build","steps":[{"type":"shell","command":"echo ok"}]}]}'
    const formatted = prettyConfigSourceSync(source)

    expect(formatted).toContain('\n  "stages": [\n')
    expect(JSON.parse(formatted)).toEqual(JSON.parse(source))
  })

  it('validates and normalizes YAML configuration', async () => {
    const formatted = await prettyConfigSource('name: test\njobs: {build: {runs-on: local}}\n')

    expect(formatted).toContain('jobs:')
    expect(formatted).toContain('runs-on: local')
  })

  it('rejects scalar values and invalid source', async () => {
    await expect(prettyConfigSource('just-a-string')).rejects.toThrow('Configuration root')
    await expect(prettyConfigSource('{broken')).rejects.toThrow()
  })

  it('keeps typed declarative pipelines as source code', async () => {
    const source = 'import { definePipeline } from "@buildworld/pipeline"\nexport default definePipeline({ stages: [] })'
    expect(isTypeScriptPipelineSource(source)).toBe(true)
    await expect(prettyConfigSource(source)).resolves.toBe(`${source}\n`)
  })
})
