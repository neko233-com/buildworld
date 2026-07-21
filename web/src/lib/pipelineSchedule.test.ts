import { describe, expect, it } from 'vitest'
import { writeScheduleToPipeline } from './pipelineSchedule'

describe('pipeline schedule authoring', () => {
  it('writes GitHub Actions-style YAML schedule configuration', () => {
    const source = 'jobs:\n  build:\n    steps:\n      - run: echo ok\n'
    expect(writeScheduleToPipeline(source, '15 2 * * *')).toContain('cron: 15 2 * * *')
  })

  it('adds the typed trigger helper and schedule to TypeScript', () => {
    const source = `import { definePipeline, shell, stage } from '@buildworld/pipeline'
export default definePipeline({ stages: [stage('Build', shell('Run', 'echo ok'))] })`
    const written = writeScheduleToPipeline(source, '0 * * * *')
    expect(written).toContain('trigger')
    expect(written).toContain('triggers: [trigger("schedule", { cron: "0 * * * *" })]')
  })

  it.each(['{"stages":[]}', '# Markdown pipeline'])('rejects removed format %s', source => {
    expect(() => writeScheduleToPipeline(source, '* * * * *')).toThrow('Only TypeScript')
  })
})
