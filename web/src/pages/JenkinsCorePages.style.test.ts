import { readFileSync } from 'node:fs'
import { resolve } from 'node:path'
import { describe, expect, it } from 'vitest'

const styles = readFileSync(resolve(process.cwd(), 'src/jenkins-pages.css'), 'utf8')

describe('Jenkins 2.563 core page geometry', () => {
  it('uses Jenkins dashboard control, table, and row dimensions', () => {
    expect(styles).toMatch(/\.jenkins-view-tabs\s*\{[^}]*min-height:\s*38px[^}]*border-radius:\s*20px/s)
    expect(styles).toMatch(/\.jenkins-view-tabs button\s*\{[^}]*height:\s*32px[^}]*font-size:\s*14px/s)
    expect(styles).toMatch(/\.jenkins-job-table-wrap\s*\{[^}]*border-radius:\s*var\(--jenkins-table-radius\)/s)
    expect(styles).toMatch(/\.jenkins-job-table\s*\{[^}]*min-width:\s*760px/s)
    expect(styles).toMatch(/\.jenkins-job-table td\s*\{[^}]*height:\s*49px[^}]*padding:\s*0 1\.6rem/s)
    expect(styles).toMatch(/\.jenkins-status-orb\s*\{[^}]*width:\s*20px[^}]*height:\s*20px/s)
  })

  it('keeps Jenkins list rows dense, readable, and horizontally scrollable on mobile', () => {
    expect(styles).toMatch(/\.jenkins-context-main \.operations-table td\s*\{[^}]*height:\s*48px[^}]*font-size:\s*14px/s)
    expect(styles).toMatch(/@media \(max-width: 620px\)[\s\S]*\.projects-table\s*\{[^}]*display:\s*table[^}]*min-width:\s*760px/s)
    expect(styles).toMatch(/@media \(prefers-reduced-motion: reduce\)[\s\S]*\.timeline-spinner\s*\{\s*animation:\s*none/s)
  })

  it('compacts project directory rows without shrinking action targets', () => {
    expect(styles).toMatch(/\.jenkins-projects-page \.projects-table thead th\s*\{[^}]*height:\s*36px[^}]*padding-block:\s*3px[^}]*white-space:\s*nowrap/s)
    expect(styles).toMatch(/\.jenkins-projects-page \.projects-table tbody td\s*\{[^}]*height:\s*52px[^}]*padding-block:\s*6px/s)
    expect(styles).toMatch(/\.jenkins-projects-page \.projects-table td\.muted-cell\s*\{[^}]*white-space:\s*nowrap/s)
    expect(styles).toMatch(/\.jenkins-projects-page \.projects-table \.entity-link strong\s*\{[^}]*line-height:\s*16px[^}]*white-space:\s*nowrap/s)
    expect(styles).toMatch(/\.jenkins-projects-page \.projects-table \.entity-link small\s*\{[^}]*margin-top:\s*1px[^}]*font-size:\s*12px[^}]*line-height:\s*15px[^}]*white-space:\s*nowrap/s)
    expect(styles).toMatch(/\.jenkins-projects-page \.projects-table \.row-actions\s*\{[^}]*flex-wrap:\s*nowrap/s)
    expect(styles).toMatch(/\.jenkins-projects-page \.projects-table \.project-row-tags\s*\{[^}]*flex-wrap:\s*nowrap/s)
    expect(styles).toMatch(/\.jenkins-projects-page \.projects-table \.project-tag\s*\{[^}]*flex:\s*0 0 auto[^}]*white-space:\s*nowrap/s)
    expect(styles).toMatch(/\.jenkins-context-main \.row-icon\s*\{[^}]*width:\s*38px[^}]*height:\s*38px/s)
  })

  it('uses Jenkins-sized actions and visible keyboard focus', () => {
    expect(styles).toMatch(/\.jenkins-home-main \.jenkins-job-actions\s*\{[^}]*display:\s*flex[^}]*padding:\s*0/s)
    expect(styles).toMatch(/\.jenkins-home-main \.jenkins-job-actions button\s*\{[^}]*width:\s*38px[^}]*height:\s*38px/s)
    expect(styles).toMatch(/\.jenkins-context-main \.row-icon\s*\{[^}]*width:\s*38px[^}]*height:\s*38px[^}]*margin:\s*-8px 0/s)
    expect(styles).toMatch(/:focus-visible\s*\{[^}]*outline:\s*3px solid rgba\(23, 105, 194, \.28\)/s)
    expect(styles).toMatch(/\.schedule-modal :is\(button, a, input, select, textarea\):focus-visible\s*\{[^}]*outline:\s*3px solid rgba\(23, 105, 194, \.28\)/s)
  })
})
