import { describe, expect, it } from 'vitest'
import { timelineProgress, visibleBuildLog } from './buildTimeline'

describe('build timeline helpers', () => {
  it('keeps machine-readable plan markers out of the operator log', () => {
    const log = [
      '[10:00:00] [] ::buildworld:plan {"stages":[]}',
      '[10:00:01] [Build] === Stage: Build ===',
      '[10:00:02] [Build] compiler output',
    ].join('\n')

    expect(visibleBuildLog(log)).toBe([
      '[10:00:01] [Build] === Stage: Build ===',
      '[10:00:02] [Build] compiler output',
    ].join('\n'))
  })

  it('reports deterministic step completion progress', () => {
    expect(timelineProgress({
      build_status: 'running',
      current_step: 2,
      total_steps: 4,
      completed_steps: 2,
      steps: [],
    })).toBe(50)
    expect(timelineProgress({
      build_status: 'success',
      current_step: 3,
      total_steps: 4,
      completed_steps: 4,
      steps: [],
    })).toBe(100)
  })
})
