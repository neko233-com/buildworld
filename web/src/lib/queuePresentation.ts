export function queueWaitReasonKey(reason?: string): string {
  switch (reason) {
    case 'running':
      return 'buildQueue.reasonRunning'
    case 'approval':
      return 'buildQueue.reasonApproval'
    case 'dependency':
      return 'buildQueue.reasonDependency'
    default:
      return 'buildQueue.reasonDispatch'
  }
}

export type QueueMoveOperation = 'move_up' | 'move_down' | 'move_top' | 'move_bottom'

type QueueItem = {
  id: number
  status: string
  priority?: number
  queue_position?: number
}

export function movableQueueItems<T extends QueueItem>(items: T[]): T[] {
  return items.filter(item => item.status === 'queued')
}

export function canMoveQueueItem<T extends QueueItem>(items: T[], id: number, operation: QueueMoveOperation): boolean {
  const movable = movableQueueItems(items)
  const index = movable.findIndex(item => item.id === id)
  if (index < 0) return false
  if (operation === 'move_up' || operation === 'move_top') return index > 0
  return index < movable.length - 1
}

export function moveQueueItem<T extends QueueItem>(items: T[], id: number, operation: QueueMoveOperation): T[] {
  const movable = movableQueueItems(items)
  const current = movable.findIndex(item => item.id === id)
  if (current < 0) return items
  const target = operation === 'move_top'
    ? 0
    : operation === 'move_bottom'
      ? movable.length - 1
      : current + (operation === 'move_up' ? -1 : 1)
  if (target < 0 || target >= movable.length || target === current) return items

  const reordered = [...movable]
  const [selected] = reordered.splice(current, 1)
  reordered.splice(target, 0, selected)
  let cursor = 0
  return items.map(item => {
    if (item.status !== 'queued') return item
    const moved = reordered[cursor]
    const position = cursor + 1
    cursor++
    return {
      ...moved,
      priority: reordered.length - position + 1,
      queue_position: position,
    }
  })
}
