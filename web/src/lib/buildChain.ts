export type BuildChainRelationType = 'retry' | 'dependency' | string

export interface BuildChainNode {
  id: number
  project_id: number
  project_name: string
  number: number
  status: string
  trigger: string
  branch?: string
  duration_ms?: number
  pinned: boolean
  focus: boolean
}

export interface BuildChainEdge {
  from_build_id: number
  to_build_id: number
  type: BuildChainRelationType
}

export interface BuildChain {
  version: number
  focus_build_id: number
  root_build_ids: number[]
  nodes: BuildChainNode[]
  edges: BuildChainEdge[]
  truncated: boolean
}

export function buildChainRelationKey(type: BuildChainRelationType): string {
  if (type === 'retry') return 'builds.chainRetry'
  if (type === 'dependency') return 'builds.chainDependency'
  return 'builds.chainRelated'
}
