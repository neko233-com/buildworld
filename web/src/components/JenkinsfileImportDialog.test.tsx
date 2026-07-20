// @vitest-environment jsdom

import { act } from 'react'
import { createRoot, type Root } from 'react-dom/client'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { api, type PipelineMigrationResult } from '../api'
import JenkinsfileImportDialog from './JenkinsfileImportDialog'

vi.mock('../api', () => ({
  api: {
    migratePipeline: vi.fn(),
  },
}))

const migratePipeline = vi.mocked(api.migratePipeline)
const actEnvironment = globalThis as typeof globalThis & { IS_REACT_ACT_ENVIRONMENT: boolean }

describe('JenkinsfileImportDialog', () => {
  let container: HTMLDivElement
  let root: Root

  beforeEach(() => {
    actEnvironment.IS_REACT_ACT_ENVIRONMENT = true
    localStorage.setItem('locale', 'zh-CN')
    container = document.createElement('div')
    container.id = 'root'
    document.body.appendChild(container)
    root = createRoot(container)
    vi.spyOn(window, 'requestAnimationFrame').mockImplementation(callback => {
      callback(0)
      return 1
    })
    vi.spyOn(window, 'cancelAnimationFrame').mockImplementation(() => undefined)
    migratePipeline.mockReset()
  })

  afterEach(() => {
    act(() => root.unmount())
    container.remove()
    localStorage.clear()
    actEnvironment.IS_REACT_ACT_ENVIRONMENT = false
    vi.restoreAllMocks()
  })

  it('converts, previews, warns, and applies a Jenkinsfile explicitly', async () => {
    const result: PipelineMigrationResult = {
      version: 'pipeline-migration/v1',
      source_format: 'jenkinsfile',
      target_format: 'buildworld-json',
      config: '{\n  "stages": []\n}\n',
      warnings: [{ code: 'post_review_required', message: 'Review Jenkins post conditions.' }],
      summary: { stage_count: 5, environment_count: 12 },
      hints: { repository_url: 'https://example.invalid/game.git', default_branch: 'main' },
    }
    migratePipeline.mockResolvedValue(result)
    const onApply = vi.fn()
    const onClose = vi.fn()
    await act(async () => {
      root.render(<JenkinsfileImportDialog projectName="game-server" onApply={onApply} onClose={onClose} />)
    })

    const source = document.querySelector<HTMLTextAreaElement>('.jenkins-source-panel textarea')
    expect(source).not.toBeNull()
    await act(async () => {
      if (source) {
        const setter = Object.getOwnPropertyDescriptor(HTMLTextAreaElement.prototype, 'value')?.set
        setter?.call(source, "pipeline { stages { stage('Build') { steps { sh 'go build' } } } }")
        source.dispatchEvent(new Event('input', { bubbles: true }))
      }
    })
    const convert = document.querySelectorAll<HTMLButtonElement>('.jenkins-import-dialog footer button')[1]
    await act(async () => {
      convert?.click()
    })

    expect(migratePipeline).toHaveBeenCalledWith('jenkinsfile', expect.stringContaining('pipeline'), 'game-server')
    expect(document.querySelector('.jenkins-preview-panel .jenkins-panel-heading')?.textContent).toMatch(/5.*12/)
    expect(document.body.textContent).toContain('Jenkins post conditions')
    expect(document.querySelector<HTMLTextAreaElement>('.jenkins-preview-panel textarea')?.value).toBe(result.config)

    const apply = document.querySelector<HTMLButtonElement>('.jenkins-import-dialog footer button:last-child')
    await act(async () => {
      apply?.click()
    })
    expect(onApply).toHaveBeenCalledWith(result)
    expect(onClose).toHaveBeenCalledOnce()
  })
})
