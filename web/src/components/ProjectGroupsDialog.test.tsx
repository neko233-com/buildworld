// @vitest-environment jsdom

import { act } from 'react'
import { createRoot, type Root } from 'react-dom/client'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { api } from '../api'
import ProjectGroupsDialog from './ProjectGroupsDialog'

vi.mock('../api', () => ({
  API_FEEDBACK_EVENT: 'buildworld:api-feedback',
  api: {
    createProjectGroup: vi.fn(),
    updateProjectGroup: vi.fn(),
    deleteProjectGroup: vi.fn(),
  },
}))

const actEnvironment = globalThis as typeof globalThis & { IS_REACT_ACT_ENVIRONMENT: boolean }
const updateProjectGroup = vi.mocked(api.updateProjectGroup)

describe('ProjectGroupsDialog', () => {
  let container: HTMLDivElement
  let root: Root

  beforeEach(() => {
    actEnvironment.IS_REACT_ACT_ENVIRONMENT = true
    localStorage.setItem('locale', 'zh-CN')
    container = document.createElement('div')
    container.id = 'root'
    document.body.appendChild(container)
    root = createRoot(container)
    updateProjectGroup.mockReset()
    updateProjectGroup.mockResolvedValue({})
    vi.spyOn(window, 'requestAnimationFrame').mockImplementation(callback => {
      callback(0)
      return 1
    })
    vi.spyOn(window, 'cancelAnimationFrame').mockImplementation(() => undefined)
  })

  afterEach(() => {
    act(() => root.unmount())
    container.remove()
    localStorage.clear()
    actEnvironment.IS_REACT_ACT_ENVIRONMENT = false
    vi.restoreAllMocks()
  })

  it('closes the dialog from the footer cancel action', async () => {
    const onClose = vi.fn()
    await act(async () => {
      root.render(<ProjectGroupsDialog groups={[]} projects={[]} onReload={vi.fn()} onClose={onClose} />)
    })

    const cancel = document.querySelector<HTMLButtonElement>('.project-group-editor footer .secondary-command')
    expect(['取消', 'Cancel']).toContain(cancel?.textContent)
    await act(async () => {
      cancel?.click()
    })

    expect(onClose).toHaveBeenCalledOnce()
  })

  it('edits a group with a keyboard-accessible preset color', async () => {
    await act(async () => {
      root.render(<ProjectGroupsDialog groups={[{ id: 2, name: 'Game servers', color: 'pink' }]} projects={[]} onReload={vi.fn()} onClose={vi.fn()} />)
    })

    await act(async () => {
      document.querySelector<HTMLButtonElement>('.project-group-main')?.click()
    })
    const pink = document.querySelector<HTMLInputElement>('input[name="project-group-color"][value="pink"]')
    const purple = document.querySelector<HTMLInputElement>('input[name="project-group-color"][value="purple"]')
    expect(document.querySelectorAll('input[name="project-group-color"]')).toHaveLength(9)
    expect(pink?.checked).toBe(true)

    await act(async () => {
      purple?.click()
    })
    expect(purple?.checked).toBe(true)
    await act(async () => {
      document.querySelector<HTMLFormElement>('.project-group-editor')?.dispatchEvent(new Event('submit', { bubbles: true, cancelable: true }))
      await Promise.resolve()
    })

    expect(updateProjectGroup).toHaveBeenCalledWith(2, {
      name: 'Game servers',
      description: '',
      color: 'purple',
    })
  })
})
