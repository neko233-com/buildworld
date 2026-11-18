import { useCallback } from 'react'
import type { NavigateFunction } from 'react-router-dom'
import { api, getToken, type PipelineValidation } from '../api'

let nextValidationID = 0
let prefetchedValidation: { id: number; projectID: number; token: ReturnType<typeof getToken>; value: PipelineValidation } | undefined
let expiry: number | undefined

export function getPrefetchedBuildValidation(projectID: number, validationID: unknown) {
  const entry = prefetchedValidation
  return entry && entry.id === validationID && entry.projectID === projectID && entry.token === getToken() ? entry.value : undefined
}

export function useJenkinsBuildFlow(navigate: NavigateFunction) {
  const start = useCallback(async (project: { id: number }) => {
    const token = getToken()
    const metadata = await api.validateProject(project.id)
    if (token !== getToken()) return null
    if (metadata.parameters?.some(parameter => String(parameter?.name || '').trim())) {
      // Keep validation in memory. Browser history receives only its lookup ID.
      const id = ++nextValidationID
      prefetchedValidation = { id, projectID: project.id, token, value: metadata }
      window.clearTimeout(expiry)
      expiry = window.setTimeout(() => { prefetchedValidation = undefined }, 10_000)
      navigate(`/projects/${project.id}/build`, { state: { validationID: id } })
      return null
    }
    const build = await api.triggerBuild(project.id)
    navigate(`/builds/${build.id}`)
    return build
  }, [navigate])

  return { start }
}
