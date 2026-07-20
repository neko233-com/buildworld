import { describe, expect, it } from 'vitest'
import { canMoveQueueItem, moveQueueItem, queueWaitReasonKey } from './queuePresentation'

describe('queue wait reason presentation', () => {
  it.each([
    ['running', 'buildQueue.reasonRunning'],
    ['approval', 'buildQueue.reasonApproval'],
    ['dependency', 'buildQueue.reasonDependency'],
    ['future-strategy', 'buildQueue.reasonDispatch'],
  ])('maps %s to %s', (reason, expected) => {
    expect(queueWaitReasonKey(reason)).toBe(expected)
  })

  it('moves only queued builds while preserving running and approval states', () => {
    const items = [
      { id: 10, status: 'running' },
      { id: 1, status: 'queued', queue_position: 1 },
      { id: 20, status: 'pending_approval' },
      { id: 2, status: 'queued', queue_position: 2 },
      { id: 3, status: 'queued', queue_position: 3 },
    ]
    const moved = moveQueueItem(items, 3, 'move_top')
    expect(moved.map(item => item.id)).toEqual([10, 3, 20, 1, 2])
    expect(moved.filter(item => item.status === 'queued').map(item => item.queue_position)).toEqual([1, 2, 3])
  })

  it('reports valid move boundaries', () => {
    const items = [
      { id: 1, status: 'queued' },
      { id: 2, status: 'queued' },
    ]
    expect(canMoveQueueItem(items, 1, 'move_up')).toBe(false)
    expect(canMoveQueueItem(items, 1, 'move_down')).toBe(true)
    expect(canMoveQueueItem(items, 2, 'move_bottom')).toBe(false)
    expect(canMoveQueueItem(items, 2, 'move_top')).toBe(true)
  })
})
