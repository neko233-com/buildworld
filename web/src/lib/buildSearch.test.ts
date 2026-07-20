import { describe, expect, it } from 'vitest'
import {
  BUILD_HISTORY_PAGE_SIZE,
  activeBuildFilterCount,
  buildSearchQuery,
  readBuildSearchParams,
} from './buildSearch'

describe('build history search parameters', () => {
  it('normalizes invalid pagination and reads URL-backed filters', () => {
    const filters = readBuildSearchParams(new URLSearchParams(
      'q=release&project=7&status=failed&trigger=webhook&branch=main&pinned=1&page=-3',
    ))
    expect(filters).toEqual({
      q: 'release',
      projectId: 7,
      status: 'failed',
      trigger: 'webhook',
      branch: 'main',
      pinned: true,
      page: 1,
      limit: BUILD_HISTORY_PAGE_SIZE,
    })
    expect(activeBuildFilterCount(filters)).toBe(6)
  })

  it('builds a bounded server-side query with the expected offset', () => {
    const filters = readBuildSearchParams(new URLSearchParams('project=2&page=3'))
    const query = new URLSearchParams(buildSearchQuery(filters))
    expect(query.get('project_id')).toBe('2')
    expect(query.get('limit')).toBe(String(BUILD_HISTORY_PAGE_SIZE))
    expect(query.get('offset')).toBe(String(BUILD_HISTORY_PAGE_SIZE * 2))
    expect(query.has('status')).toBe(false)
  })
})
