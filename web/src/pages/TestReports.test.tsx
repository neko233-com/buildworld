// @vitest-environment jsdom

import { act } from 'react'
import { createRoot, type Root } from 'react-dom/client'
import { MemoryRouter, Route, Routes } from 'react-router-dom'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { api, type BuildTestResultsResponse } from '../api'
import TestReports from './TestReports'

vi.mock('../api', () => ({
  api: {
    getBuildTestResults: vi.fn(),
    getBuild: vi.fn(),
    getProject: vi.fn(),
    uploadTestResults: vi.fn(),
  },
}))

vi.mock('../components/AppDialogs', () => ({
  dialogs: { notify: vi.fn() },
}))

const getBuildTestResults = vi.mocked(api.getBuildTestResults)
const getBuild = vi.mocked(api.getBuild)
const getProject = vi.mocked(api.getProject)
const uploadTestResults = vi.mocked(api.uploadTestResults)
const actEnvironment = globalThis as typeof globalThis & { IS_REACT_ACT_ENVIRONMENT: boolean }

const reportFixture: BuildTestResultsResponse = {
  summary: {
    id: 8,
    build_id: 42,
    total: 3,
    passed: 1,
    failed: 1,
    skipped: 1,
    duration_ms: 500,
    created_at: '2026-07-21T10:00:00Z',
  },
  cases: [
    { name: 'works', classname: 'pkg.Example', suite_name: 'unit', status: 'passed', duration_ms: 125 },
    { name: 'fails', classname: 'pkg.Example', suite_name: 'unit', status: 'failed', duration_ms: 250, message: 'want true', type: 'AssertionError', details: 'stack' },
    { name: 'later', classname: 'pkg.Example', suite_name: 'unit', status: 'skipped', duration_ms: 0, message: 'not ready' },
  ],
  results: [],
}

describe('TestReports', () => {
  let container: HTMLDivElement
  let breadcrumbHost: HTMLDivElement
  let root: Root

  beforeEach(() => {
    actEnvironment.IS_REACT_ACT_ENVIRONMENT = true
    getBuildTestResults.mockReset()
    getBuild.mockReset()
    getProject.mockReset()
    uploadTestResults.mockReset()
    getBuildTestResults.mockResolvedValue(reportFixture)
    getBuild.mockResolvedValue({ id: 42, number: 7, project_id: 9, status: 'success' })
    getProject.mockResolvedValue({ id: 9, name: 'weather-service' })
    uploadTestResults.mockResolvedValue(reportFixture.summary!)
    breadcrumbHost = document.createElement('div')
    breadcrumbHost.id = 'jenkins-header-breadcrumbs'
    container = document.createElement('div')
    document.body.append(breadcrumbHost, container)
    root = createRoot(container)
  })

  afterEach(() => {
    act(() => root.unmount())
    breadcrumbHost.remove()
    container.remove()
    actEnvironment.IS_REACT_ACT_ENVIRONMENT = false
  })

  async function renderReport() {
    await act(async () => {
      root.render(
        <MemoryRouter initialEntries={['/builds/42/tests']}>
          <Routes>
            <Route path="/builds/:id/tests" element={<TestReports />} />
          </Routes>
        </MemoryRouter>,
      )
    })
  }

  it('renders the typed summary and testcase details', async () => {
    await renderReport()

    const metrics = Array.from(container.querySelectorAll('.test-report-metrics article strong')).map(node => node.textContent)
    expect(metrics).toEqual(['3', '1', '1', '1'])
    expect(container.querySelectorAll('.test-results-table tbody tr')).toHaveLength(3)
    expect(container.querySelectorAll('.test-results-table .build-status.success')).toHaveLength(1)
    expect(container.querySelectorAll('.test-results-table .build-status.failed')).toHaveLength(1)
    expect(container.querySelectorAll('.test-results-table .build-status.pending')).toHaveLength(1)
    expect(container.textContent).toContain('pkg.Example · AssertionError · want true')
    expect(container.textContent).toContain('125 ms')
    expect(breadcrumbHost.textContent).toContain('weather-service')
    expect(breadcrumbHost.textContent).toContain('#7')
    expect(getBuild).toHaveBeenCalledWith(42)
    expect(getProject).toHaveBeenCalledWith(9)
  })

  it('renders a stable empty state', async () => {
    getBuildTestResults.mockResolvedValueOnce({ summary: null, cases: [], results: [] })
    await renderReport()

    expect(Array.from(container.querySelectorAll('.test-report-metrics article strong')).map(node => node.textContent)).toEqual(['0', '0', '0', '0'])
    expect(container.querySelector('.test-results-empty')).not.toBeNull()
    expect(container.querySelector('.test-results-table')).toBeNull()
  })

  it('uploads the selected file as XML and reloads the report', async () => {
    await renderReport()
    const input = container.querySelector<HTMLInputElement>('input[type="file"]')!
    const xml = '<testsuite name="unit"><testcase name="works"/></testsuite>'
    const file = new File([xml], 'junit.xml', { type: 'application/xml' })
    Object.defineProperty(file, 'text', { configurable: true, value: async () => xml })
    Object.defineProperty(input, 'files', { configurable: true, value: [file] })

    await act(async () => input.dispatchEvent(new Event('change', { bubbles: true })))

    expect(uploadTestResults).toHaveBeenCalledWith(42, xml)
    expect(getBuildTestResults).toHaveBeenCalledTimes(2)
    expect(input.value).toBe('')
  })
})
