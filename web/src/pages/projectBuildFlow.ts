import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import type { NavigateFunction } from 'react-router-dom'
import { api } from '../api'
import { normalizeBuildParameterDefinitions } from '../components/RunBuildDialog'

export type JenkinsBuildProject = {
  id: number
  enabled?: boolean
  updated_at?: string
}

type JenkinsBuildMode = 'immediate' | 'parameters'

function projectModeKey(project: JenkinsBuildProject): string {
  return `${project.id}:${project.updated_at || ''}`
}

export function useJenkinsBuildFlow(projects: JenkinsBuildProject[], navigate: NavigateFunction) {
  const [modes, setModes] = useState<Record<string, JenkinsBuildMode>>({})
  const requests = useRef(new Map<string, Promise<JenkinsBuildMode>>())
  const mounted = useRef(true)
  const projectsRef = useRef(projects)
  projectsRef.current = projects
  const projectKeys = useMemo(
    () => projects.filter(project => project.enabled !== false).map(projectModeKey).sort().join('|'),
    [projects],
  )

  const inspect = useCallback((project: JenkinsBuildProject): Promise<JenkinsBuildMode> => {
    const key = projectModeKey(project)
    const current = requests.current.get(key)
    if (current) return current

    const request = api.validateProject(project.id)
      .then(metadata => normalizeBuildParameterDefinitions(metadata.parameters).length > 0 ? 'parameters' as const : 'immediate' as const)
      .then(mode => {
        if (mounted.current) setModes(previous => previous[key] === mode ? previous : { ...previous, [key]: mode })
        return mode
      })
      .catch(reason => {
        requests.current.delete(key)
        throw reason
      })
    requests.current.set(key, request)
    return request
  }, [])

  useEffect(() => {
    const enabledProjects = projectsRef.current.filter(project => project.enabled !== false)
    void Promise.allSettled(enabledProjects.map(inspect))
  }, [inspect, projectKeys])

  useEffect(() => {
    mounted.current = true
    return () => {
      mounted.current = false
    }
  }, [])

  const isParameterized = useCallback(
    (project: JenkinsBuildProject) => modes[projectModeKey(project)] === 'parameters',
    [modes],
  )

  const start = useCallback(async (project: JenkinsBuildProject) => {
    const mode = modes[projectModeKey(project)] || await inspect(project)
    if (mode === 'parameters') {
      navigate(`/projects/${project.id}/build`)
      return null
    }
    const build = await api.triggerBuild(project.id)
    navigate(`/builds/${build.id}`)
    return build
  }, [inspect, modes, navigate])

  return { inspect, isParameterized, start }
}
