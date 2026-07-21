export const PROJECT_GROUP_COLORS = ['neutral', 'blue', 'cyan', 'mint', 'green', 'yellow', 'orange', 'pink', 'purple'] as const

export type ProjectGroupColor = typeof PROJECT_GROUP_COLORS[number]

export type ProjectGroup = {
  id: number
  name: string
  description?: string
  color?: ProjectGroupColor | string
  created_at?: string
  updated_at?: string
}

const projectGroupColorSet = new Set<string>(PROJECT_GROUP_COLORS)

export function normalizeProjectGroupColor(color?: string | null): ProjectGroupColor {
  return color && projectGroupColorSet.has(color) ? color as ProjectGroupColor : 'neutral'
}

export function sortProjectGroups(groups: ProjectGroup[]): ProjectGroup[] {
  return [...groups].sort((left, right) => left.name.localeCompare(right.name) || left.id - right.id)
}

export function projectGroupPath(groups: ProjectGroup[], groupID?: number | null): string {
  if (groupID == null) return ''
  return groups.find(group => group.id === groupID)?.name || ''
}
