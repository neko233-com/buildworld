import { describe, expect, it } from 'vitest'
import { buildLogMessage, timelineProgress, visibleBuildLog } from './buildTimeline'

describe('build timeline helpers', () => {
  it('removes the build envelope while preserving original timestamps, levels and indentation', () => {
    expect(buildLogMessage('[10:27:21] [Resume Existing Log Monitor] 2026-10-08 10:27:21,123 [INFO] ready')).toBe('2026-10-08 10:27:21,123 [INFO] ready')
    expect(buildLogMessage('[10:27:21] [Live Log Monitor]   [0] load at service.go:42')).toBe('  [0] load at service.go:42')
    expect(buildLogMessage('[10:27:21] [] BUILD FAILED')).toBe('BUILD FAILED')
    expect(buildLogMessage('[10:27:21] [WARN] [Build] retrying')).toBe('[WARN] retrying')
    expect(buildLogMessage('[INFO] ready')).toBe('[INFO] ready')
    expect(buildLogMessage('[2026-10-08 10:27:21,123] [INFO] ready')).toBe('[2026-10-08 10:27:21,123] [INFO] ready')
    expect(buildLogMessage('plain output')).toBe('plain output')
  })

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
