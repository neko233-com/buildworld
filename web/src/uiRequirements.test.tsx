// @vitest-environment jsdom

import { readFileSync } from 'node:fs'
import { resolve } from 'node:path'
import { act } from 'react'
import { createRoot } from 'react-dom/client'
import { afterEach, describe, expect, it } from 'vitest'
import { BuildWorldMark } from './components/BuildWorldMark'
import { PROJECT_GROUP_COLORS } from './lib/projectGroups'

const styles = readFileSync(resolve(process.cwd(), 'src/index.css'), 'utf8')

function luminance(color: string) {
  const channels = color.match(/[a-f\d]{2}/gi)?.map(value => {
    const channel = Number.parseInt(value, 16) / 255
    return channel <= 0.04045 ? channel / 12.92 : ((channel + 0.055) / 1.055) ** 2.4
  })
  if (!channels || channels.length !== 3) throw new Error(`invalid color ${color}`)
  return 0.2126 * channels[0] + 0.7152 * channels[1] + 0.0722 * channels[2]
}

function contrast(left: string, right: string) {
  const [bright, dark] = [luminance(left), luminance(right)].sort((a, b) => b - a)
  return (bright + 0.05) / (dark + 0.05)
}

describe('requested UI contracts', () => {
  afterEach(() => { document.body.replaceChildren() })

  it('keeps sidebar type readable over the intended dark material', () => {
    expect(styles).toMatch(/--sidebar-text:\s*#d1d1d6/)
    expect(styles).toMatch(/\.sidebar-link\s*\{[^}]*font-size:\s*14px/)
    expect(styles).toContain('background: linear-gradient(180deg, #242426 0%, #1c1c1e 100%)')
    expect(contrast('#d1d1d6', '#242426')).toBeGreaterThan(7)
    expect(contrast('#d1d1d6', '#1c1c1e')).toBeGreaterThan(7)
  })

  it('keeps project workbench stretched through available viewport height', () => {
    expect(styles).toMatch(/\.project-detail-page\s*\{[^}]*height:\s*calc\(100dvh - 126px\)[^}]*flex-direction:\s*column/)
    expect(styles).toMatch(/\.project-workbench\s*\{[^}]*flex:\s*1[^}]*flex-direction:\s*column/)
    expect(styles).toMatch(/\.project-workbench \.detail-panel-body\s*\{[^}]*flex:\s*1[^}]*overflow:\s*auto/)
  })

  it('exposes nine restrained project-folder color presets', () => {
    expect(PROJECT_GROUP_COLORS).toEqual([
      'neutral', 'blue', 'cyan', 'mint', 'green', 'yellow', 'orange', 'pink', 'purple',
    ])
    for (const color of PROJECT_GROUP_COLORS.filter(value => value !== 'neutral')) {
      expect(styles).toContain(`[data-group-color="${color}"]`)
    }
  })

  it('renders the requested moon and build hammer mark', () => {
    const container = document.createElement('div')
    document.body.appendChild(container)
    const root = createRoot(container)
    act(() => root.render(<BuildWorldMark title="BuildWorld" />))

    const mark = container.querySelector('svg[role="img"][aria-label="BuildWorld"]')
    expect(mark?.querySelector('path[fill="url(#buildworld-moon)"]')).not.toBeNull()
    expect(mark?.querySelector('g[transform^="rotate("] path[fill="url(#buildworld-hammer)"]')).not.toBeNull()

    act(() => root.unmount())
  })
})
