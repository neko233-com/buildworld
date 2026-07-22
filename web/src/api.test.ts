// @vitest-environment jsdom

import { afterEach, describe, expect, it, vi } from 'vitest'
import { API_FEEDBACK_EVENT, api, downloadAuthenticated, request, setToken, type APIFeedbackDetail } from './api'

describe('API request resilience', () => {
  afterEach(() => {
    vi.useRealTimers()
    vi.restoreAllMocks()
    localStorage.clear()
  })

  it('retries transient gateway failures for reads', async () => {
    vi.useFakeTimers()
    const fetchMock = vi.spyOn(globalThis, 'fetch')
      .mockResolvedValueOnce(new Response('Bad Gateway', { status: 502 }))
      .mockResolvedValueOnce(new Response('Unavailable', { status: 503 }))
      .mockResolvedValueOnce(new Response(JSON.stringify({ status: 'ok' }), { status: 200 }))

    const pending = request<{ status: string }>('GET', '/health')
    await vi.runAllTimersAsync()

    await expect(pending).resolves.toEqual({ status: 'ok' })
    expect(fetchMock).toHaveBeenCalledTimes(3)
  })

  it('does not retry mutations that could be applied twice', async () => {
    const fetchMock = vi.spyOn(globalThis, 'fetch')
      .mockResolvedValue(new Response('Bad Gateway', { status: 502 }))

    await expect(request('POST', '/projects/', { name: 'demo' })).rejects.toMatchObject({
      status: 502,
      code: 'service_unavailable',
    })
    expect(fetchMock).toHaveBeenCalledTimes(1)
  })

  it('reports mutation progress and replaces it with the final result', async () => {
    const feedback: APIFeedbackDetail[] = []
    const listener = (event: Event) => feedback.push((event as CustomEvent<APIFeedbackDetail>).detail)
    window.addEventListener(API_FEEDBACK_EVENT, listener)
    vi.spyOn(globalThis, 'fetch').mockResolvedValue(new Response(JSON.stringify({ id: 8 }), { status: 201 }))

    await expect(request('POST', '/project-groups/', { name: 'Games' })).resolves.toEqual({ id: 8 })

    expect(feedback).toHaveLength(2)
    expect(feedback[0]).toMatchObject({ phase: 'start', tone: 'info' })
    expect(feedback[1]).toMatchObject({ phase: 'success', tone: 'success', operationId: feedback[0].operationId })
    window.removeEventListener(API_FEEDBACK_EVENT, listener)
  })

  it.each([
    '/pipeline-validation',
    '/projects/7/validate',
  ])('does not report read-only POST validation as a mutation for %s', async path => {
    const feedback: APIFeedbackDetail[] = []
    const listener = (event: Event) => feedback.push((event as CustomEvent<APIFeedbackDetail>).detail)
    window.addEventListener(API_FEEDBACK_EVENT, listener)
    vi.spyOn(globalThis, 'fetch').mockResolvedValue(new Response(JSON.stringify({ valid: true }), { status: 200 }))

    await expect(request('POST', path, { source: 'pipeline' })).resolves.toEqual({ valid: true })

    expect(feedback).toEqual([])
    window.removeEventListener(API_FEEDBACK_EVENT, listener)
  })

  it('reports the server error at the top-level feedback channel', async () => {
    const feedback: APIFeedbackDetail[] = []
    const listener = (event: Event) => feedback.push((event as CustomEvent<APIFeedbackDetail>).detail)
    window.addEventListener(API_FEEDBACK_EVENT, listener)
    vi.spyOn(globalThis, 'fetch').mockResolvedValue(new Response(JSON.stringify({ error: '分组名称已存在' }), {
      status: 409,
      headers: { 'Content-Type': 'application/json' },
    }))

    await expect(request('POST', '/project-groups/', { name: 'Games' })).rejects.toThrow('分组名称已存在')

    expect(feedback.at(-1)).toMatchObject({ phase: 'error', tone: 'error', message: '分组名称已存在' })
    window.removeEventListener(API_FEEDBACK_EVENT, listener)
  })

  it('localizes stable API error codes without exposing database details', async () => {
    localStorage.setItem('locale', 'zh-CN')
    vi.spyOn(globalThis, 'fetch').mockResolvedValue(new Response(JSON.stringify({
      error: 'project group name already exists',
      code: 'project_group_name_exists',
    }), {
      status: 409,
      headers: { 'Content-Type': 'application/json' },
    }))

    await expect(request('POST', '/project-groups/', { name: 'Games' })).rejects.toMatchObject({
      status: 409,
      code: 'project_group_name_exists',
      message: '项目分组名称已存在',
    })
  })

  it('downloads protected files with an authorization header and no token in the URL', async () => {
    setToken('secret-token')
    const fetchMock = vi.spyOn(globalThis, 'fetch').mockResolvedValue(new Response('log output', {
      status: 200,
      headers: { 'Content-Disposition': 'attachment; filename="build-23.txt"' },
    }))
    const createObjectURL = vi.fn(() => 'blob:build-log')
    const revokeObjectURL = vi.fn()
    Object.defineProperty(URL, 'createObjectURL', { configurable: true, value: createObjectURL })
    Object.defineProperty(URL, 'revokeObjectURL', { configurable: true, value: revokeObjectURL })
    const click = vi.spyOn(HTMLAnchorElement.prototype, 'click').mockImplementation(() => undefined)

    await expect(downloadAuthenticated('/builds/23/logs/download?format=txt', 'fallback.txt')).resolves.toBe('build-23.txt')
    expect(fetchMock).toHaveBeenCalledWith('/api/builds/23/logs/download?format=txt', {
      headers: { Authorization: 'Bearer secret-token' },
    })
    expect(fetchMock.mock.calls[0][0]).not.toContain('token=')
    expect(createObjectURL).toHaveBeenCalledOnce()
    expect(click).toHaveBeenCalledOnce()
  })

  it('uploads JUnit reports as raw XML', async () => {
    setToken('secret-token')
    const xml = '<testsuite name="unit"><testcase name="works"/></testsuite>'
    const fetchMock = vi.spyOn(globalThis, 'fetch').mockResolvedValue(new Response(JSON.stringify({
      id: 4,
      build_id: 23,
      total: 1,
      passed: 1,
      failed: 0,
      skipped: 0,
      duration_ms: 0,
      created_at: '2026-07-21T10:00:00Z',
    }), { status: 201 }))

    await expect(api.uploadTestResults(23, xml)).resolves.toMatchObject({ id: 4, total: 1 })
    expect(fetchMock).toHaveBeenCalledWith('/api/builds/23/test-results', {
      method: 'POST',
      headers: { Authorization: 'Bearer secret-token', 'Content-Type': 'application/xml' },
      body: xml,
    })
  })

  it('falls back to a bounded build list when an older server treats job-overview as an id', async () => {
    const fetchMock = vi.spyOn(globalThis, 'fetch')
      .mockResolvedValueOnce(new Response(JSON.stringify({ error: 'invalid id' }), {
        status: 400,
        headers: { 'Content-Type': 'application/json' },
      }))
      .mockResolvedValueOnce(new Response(JSON.stringify([
        { id: 31, project_id: 7, number: 2, status: 'failed', started_at: '2026-07-22T10:02:00Z', duration_ms: 2_000, log: 'not copied' },
        { id: 30, project_id: 7, number: 1, status: 'success', started_at: '2026-07-22T10:01:00Z', duration_ms: 1_000 },
        { id: 92, project_id: 9, number: 2, status: 'success', started_at: '2026-07-22T10:04:00Z' },
        { id: 91, project_id: 9, number: 1, status: 'success', started_at: '2026-07-22T10:03:00Z' },
        { id: 90, project_id: 9, number: 0, status: 'failed', started_at: '2026-07-22T10:02:00Z' },
        { id: 89, project_id: 9, number: -1, status: 'cancelled', started_at: '2026-07-22T10:01:00Z' },
        { id: 88, project_id: 9, number: -2, status: 'running', started_at: '2026-07-22T10:00:00Z' },
        { id: 87, project_id: 9, number: -3, status: 'pending', started_at: '2026-07-22T09:59:00Z' },
        { id: 'invalid', project_id: 'invalid', number: 1, status: 'success' },
      ]), { status: 200 }))

    await expect(api.listProjectBuildOverviews()).resolves.toEqual([
      {
        project_id: 7,
        latest: { id: 31, number: 2, status: 'failed', started_at: '2026-07-22T10:02:00Z', duration_ms: 2_000 },
        last_success: { id: 30, number: 1, status: 'success', started_at: '2026-07-22T10:01:00Z', duration_ms: 1_000 },
        last_failure: { id: 31, number: 2, status: 'failed', started_at: '2026-07-22T10:02:00Z', duration_ms: 2_000 },
        recent_statuses: ['failed', 'success'],
      },
      {
        project_id: 9,
        latest: { id: 92, number: 2, status: 'success', started_at: '2026-07-22T10:04:00Z' },
        last_success: { id: 92, number: 2, status: 'success', started_at: '2026-07-22T10:04:00Z' },
        last_failure: { id: 90, number: 0, status: 'failed', started_at: '2026-07-22T10:02:00Z' },
        recent_statuses: ['success', 'success', 'failed', 'cancelled', 'running'],
      },
    ])
    expect(fetchMock.mock.calls.map(call => call[0])).toEqual([
      '/api/projects/job-overview',
      '/api/builds/?limit=500',
    ])
  })

  it.each([
    [403, { error: 'forbidden' }],
    [500, { error: 'database unavailable' }],
    [400, { error: 'invalid project' }],
    [400, { error: 'invalid id', code: 'invalid_project_id' }],
  ])('does not hide a real project overview HTTP %s error behind the legacy fallback', async (status, payload) => {
    const fetchMock = vi.spyOn(globalThis, 'fetch').mockResolvedValue(new Response(JSON.stringify(payload), {
      status,
      headers: { 'Content-Type': 'application/json' },
    }))

    await expect(api.listProjectBuildOverviews()).rejects.toMatchObject({ status, message: payload.error })
    expect(fetchMock).toHaveBeenCalledOnce()
  })

  it('surfaces a legacy fallback request failure', async () => {
    const fetchMock = vi.spyOn(globalThis, 'fetch')
      .mockResolvedValueOnce(new Response(JSON.stringify({ error: 'invalid id' }), { status: 400 }))
      .mockResolvedValueOnce(new Response(JSON.stringify({ error: 'build history unavailable' }), { status: 500 }))

    await expect(api.listProjectBuildOverviews()).rejects.toMatchObject({ status: 500, message: 'build history unavailable' })
    expect(fetchMock).toHaveBeenCalledTimes(2)
  })
})
