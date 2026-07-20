export type ProjectGroup = {
  id: number
  name: string
  description?: string
  parent_id?: number | null
  created_at?: string
  updated_at?: string
}

export type FlatProjectGroup = ProjectGroup & {
  depth: number
  path: string
}

export function flattenProjectGroups(groups: ProjectGroup[]): FlatProjectGroup[] {
  const byParent = new Map<number | null, ProjectGroup[]>()
  const ids = new Set(groups.map(group => group.id))
  for (const group of groups) {
    const parent = group.parent_id != null && ids.has(group.parent_id) ? group.parent_id : null
    byParent.set(parent, [...(byParent.get(parent) || []), group])
  }
  for (const children of byParent.values()) {
    children.sort((left, right) => left.name.localeCompare(right.name))
  }

  const result: FlatProjectGroup[] = []
  const visited = new Set<number>()
  const walk = (parentID: number | null, depth: number, parents: string[]) => {
    for (const group of byParent.get(parentID) || []) {
      if (visited.has(group.id)) continue
      visited.add(group.id)
      const path = [...parents, group.name]
      result.push({ ...group, depth, path: path.join(' / ') })
      walk(group.id, depth + 1, path)
    }
  }
  walk(null, 0, [])

  // Corrupt legacy data should remain manageable instead of disappearing.
  for (const group of groups) {
    if (!visited.has(group.id)) result.push({ ...group, depth: 0, path: group.name })
  }
  return result
}

export function descendantProjectGroupIDs(groups: ProjectGroup[], selectedID: number): Set<number> {
  const result = new Set<number>([selectedID])
  let changed = true
  while (changed) {
    changed = false
    for (const group of groups) {
      if (group.parent_id != null && result.has(group.parent_id) && !result.has(group.id)) {
        result.add(group.id)
        changed = true
      }
    }
  }
  return result
}

export function projectGroupPath(groups: ProjectGroup[], groupID?: number | null): string {
  if (groupID == null) return ''
  return flattenProjectGroups(groups).find(group => group.id === groupID)?.path || ''
}
