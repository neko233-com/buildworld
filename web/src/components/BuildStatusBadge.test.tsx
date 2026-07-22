// @vitest-environment jsdom

import { act } from 'react'
import { createRoot, type Root } from 'react-dom/client'
import { afterEach, beforeEach, describe, expect, it } from 'vitest'
import { BuildStatusBadge } from './BuildStatusBadge'

describe('BuildStatusBadge', () => {
  let container: HTMLDivElement
  let root: Root

  beforeEach(() => {
    container = document.createElement('div')
    document.body.appendChild(container)
    root = createRoot(container)
  })

  afterEach(() => {
    act(() => root.unmount())
    container.remove()
  })

  it('announces a running build and renders a visible progress glyph', () => {
    act(() => root.render(<BuildStatusBadge status="running" label="执行中" />))

    const status = container.querySelector('[role="status"]')
    expect(status?.getAttribute('aria-live')).toBe('polite')
    expect(status?.classList.contains('build-status-live')).toBe(true)
    expect(status?.textContent).toBe('执行中')
    expect(status?.querySelector('.build-status-spinner[aria-hidden="true"]')).not.toBeNull()
  })

  it('keeps completed builds static', () => {
    act(() => root.render(<BuildStatusBadge status="success" label="已成功" />))

    expect(container.querySelector('[role="status"]')).toBeNull()
    expect(container.querySelector('.build-status-spinner')).toBeNull()
    expect(container.querySelector('.build-status.success')?.textContent).toBe('已成功')
  })

  it.each(['pending', 'queued', 'pending_approval'])('keeps %s builds visibly active', statusName => {
    act(() => root.render(<BuildStatusBadge status={statusName} label="等待中" />))

    expect(container.querySelector('[role="status"] .build-status-spinner')).not.toBeNull()
  })
})
