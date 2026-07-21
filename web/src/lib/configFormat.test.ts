import { describe, expect, it } from 'vitest'
import { isTypeScriptPipelineSource, prettyConfigSource, prettyConfigSourceSync, prettyPipelineSource, prettyPipelineSourceSync, resolvePipelineSourceLanguage } from './configFormat'

describe('configuration formatting', () => {
  it('rejects removed JSON authoring', () => {
    const source = '{"stages":[{"name":"Build","steps":[{"type":"shell","command":"echo ok"}]}]}'
    expect(() => prettyPipelineSourceSync(source)).toThrow('JSON pipeline configs are not supported')
  })

  it('validates and normalizes YAML configuration', async () => {
    const formatted = await prettyPipelineSource('name: test\njobs: {build: {runs-on: local, steps: [{run: echo ok}]}}\n')

    expect(formatted).toContain('jobs:')
    expect(formatted).toContain('runs-on: local')
  })

  it('rejects scalar values and invalid source', async () => {
    await expect(prettyPipelineSource('just-a-string')).rejects.toThrow('Configuration root')
    await expect(prettyPipelineSource('{broken')).rejects.toThrow('JSON pipeline configs are not supported')
    await expect(prettyPipelineSource('stages: []')).rejects.toThrow('non-empty jobs mapping')
    await expect(prettyPipelineSource('# Markdown pipeline')).rejects.toThrow('Markdown pipeline configs are not supported')
  })

  it('keeps typed declarative pipelines as source code', async () => {
    const source = 'import { definePipeline } from "@buildworld/pipeline"\nexport default definePipeline({ stages: [] })'
    expect(isTypeScriptPipelineSource(source)).toBe(true)
    await expect(prettyPipelineSource(source)).resolves.toBe(`${source}\n`)
  })

  it('keeps generic JSON/YAML formatting available for non-pipeline forms', async () => {
    expect(prettyConfigSourceSync('{"token":"value"}')).toContain('\n  "token"')
    await expect(prettyConfigSource('kind: deployment\n')).resolves.toContain('kind: deployment')
  })

  it('keeps active editor language while source is temporarily incomplete', () => {
    expect(resolvePipelineSourceLanguage('', 'typescript')).toBe('typescript')
    expect(resolvePipelineSourceLanguage('export default definePipeline({', 'yaml')).toBe('typescript')
    expect(resolvePipelineSourceLanguage('name: verify\njobs:\n  build:\n', 'typescript')).toBe('yaml')
  })
})
