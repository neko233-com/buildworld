import { describe, expect, it } from 'vitest'
import { normalizeProjectGroupColor, projectGroupPath, sortProjectGroups } from './projectGroups'

const groups = [
  { id: 2, name: 'Desktop' },
  { id: 1, name: 'Products' },
  { id: 3, name: 'Windows' },
]

describe('one-level project groups', () => {
  it('sorts independent top-level folders by name', () => {
    expect(sortProjectGroups(groups).map(group => [group.id, group.name])).toEqual([
      [2, 'Desktop'],
      [1, 'Products'],
      [3, 'Windows'],
    ])
    expect(projectGroupPath(groups, 3)).toBe('Windows')
  })

  it('accepts only supported quiet color tokens', () => {
    expect(normalizeProjectGroupColor('mint')).toBe('mint')
    expect(normalizeProjectGroupColor('neon-red')).toBe('neutral')
    expect(normalizeProjectGroupColor()).toBe('neutral')
  })
})
