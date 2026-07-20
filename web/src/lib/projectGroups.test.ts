import { describe, expect, it } from 'vitest'
import { descendantProjectGroupIDs, flattenProjectGroups, projectGroupPath } from './projectGroups'

const groups = [
  { id: 2, name: 'Desktop', parent_id: 1 },
  { id: 1, name: 'Products' },
  { id: 3, name: 'Windows', parent_id: 2 },
]

describe('project group hierarchy', () => {
  it('flattens parents before children and builds paths', () => {
    expect(flattenProjectGroups(groups).map(group => [group.id, group.depth, group.path])).toEqual([
      [1, 0, 'Products'],
      [2, 1, 'Products / Desktop'],
      [3, 2, 'Products / Desktop / Windows'],
    ])
    expect(projectGroupPath(groups, 3)).toBe('Products / Desktop / Windows')
  })

  it('includes descendant groups in a parent filter', () => {
    expect([...descendantProjectGroupIDs(groups, 1)]).toEqual([1, 2, 3])
  })
})
