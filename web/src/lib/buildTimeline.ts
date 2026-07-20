export type TimelineStepStatus = 'pending' | 'running' | 'success' | 'failed' | 'skipped' | 'cancelled'

export interface BuildTimelineStep {
  index: number
  stage: string
  name: string
  status: TimelineStepStatus
}

export interface BuildTimeline {
  build_status: string
  current_step: number
  total_steps: number
  completed_steps: number
  steps: BuildTimelineStep[]
}

export function visibleBuildLog(log = ''): string {
  return log
    .split('\n')
    .filter(line => !line.includes('::buildworld:plan '))
    .join('\n')
    .trimEnd()
}

export function timelineProgress(timeline?: BuildTimeline | null): number {
  if (!timeline?.total_steps) return 0
  if (timeline.build_status === 'success') return 100
  return Math.round((timeline.completed_steps / timeline.total_steps) * 100)
}
