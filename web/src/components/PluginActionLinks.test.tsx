// @vitest-environment jsdom

import { act } from 'react'
import { createRoot, type Root } from 'react-dom/client'
import { MemoryRouter } from 'react-router-dom'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { api } from '../api'
import PluginActionLinks from './PluginActionLinks'

vi.mock('../api', () => ({ api: { listPlugins: vi.fn() } }))

const listPlugins = vi.mocked(api.listPlugins)
const actEnvironment = globalThis as typeof globalThis & { IS_REACT_ACT_ENVIRONMENT: boolean }

describe('PluginActionLinks', () => {
  let container: HTMLDivElement
  let root: Root

  beforeEach(() => {
    actEnvironment.IS_REACT_ACT_ENVIRONMENT = true
    container = document.createElement('div')
    document.body.appendChild(container)
    root = createRoot(container)
    listPlugins.mockReset().mockResolvedValue([
      {
        name: 'reports',
        enabled: true,
        loaded: true,
        ui_extensions: [
          { location: 'project.action', label: 'Project report', url: '/projects/{projectId}/report' },
          { location: 'build.action', label: 'Deployment', url: 'https://deployments.example.test/builds/{buildId}?number={buildNumber}', open_in_new_tab: true },
        ],
      },
      {
        name: 'disabled',
        enabled: false,
        loaded: true,
        ui_extensions: [{ location: 'build.action', label: 'Hidden', url: '/hidden' }],
      },
    ])
  })

  afterEach(() => {
    act(() => root.unmount())
    container.remove()
  })

  it('renders only enabled host-declared actions and resolves build placeholders', async () => {
    await act(async () => {
      root.render(<MemoryRouter><PluginActionLinks location="build.action" projectId={7} buildId={42} buildNumber={9} /></MemoryRouter>)
      await Promise.resolve()
    })

    const link = container.querySelector<HTMLAnchorElement>('a')
    expect(link?.textContent).toContain('Deployment')
    expect(link?.getAttribute('href')).toBe('https://deployments.example.test/builds/42?number=9')
    expect(link?.getAttribute('target')).toBe('_blank')
    expect(link?.getAttribute('rel')).toBe('noopener noreferrer')
    expect(container.textContent).not.toContain('Project report')
    expect(container.textContent).not.toContain('Hidden')
  })
})
