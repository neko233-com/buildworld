import { describe, expect, it } from 'vitest'
import { buildStatusLabel, buildTriggerLabel } from './buildPresentation'

const copy: Record<string, string> = {
  'builds.success': '已成功',
  'builds.pending': '待执行',
  'builds.triggerManual': '手动',
  'builds.triggerRetry': '重试',
}
const t = (key: string) => copy[key] || key

describe('build presentation helpers', () => {
  it('localizes known build statuses and falls back safely', () => {
    expect(buildStatusLabel(t, 'success')).toBe('已成功')
    expect(buildStatusLabel(t)).toBe('待执行')
    expect(buildStatusLabel(t, 'paused')).toBe('paused')
  })

  it('normalizes known trigger sources without hiding unknown integrations', () => {
    expect(buildTriggerLabel(t, 'manual')).toBe('手动')
    expect(buildTriggerLabel(t, 'retry')).toBe('重试')
    expect(buildTriggerLabel(t, 'custom-hook')).toBe('custom-hook')
    expect(buildTriggerLabel(t)).toBe('-')
  })
})
