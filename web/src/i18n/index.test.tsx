// @vitest-environment jsdom

import { act } from 'react'
import { createRoot } from 'react-dom/client'
import { afterEach, describe, expect, it } from 'vitest'
import { useI18n } from './index'

function LocaleProbe() {
  const { changeLocale, t } = useI18n()
  return <div>
    <span data-testid="dashboard">{t('nav.dashboard')}</span>
    <span data-testid="my-dashboard">{t('nav.myDashboard')}</span>
    <button onClick={() => changeLocale('ja-JP')}>ja</button>
    <button onClick={() => changeLocale('zh-CN')}>zh</button>
  </div>
}

describe('locale switching', () => {
  afterEach(() => {
    document.body.replaceChildren()
    localStorage.clear()
  })

  it('keeps the current Japanese and Chinese copy while updating document language', () => {
    const container = document.createElement('div')
    document.body.appendChild(container)
    const root = createRoot(container)
    act(() => root.render(<LocaleProbe />))

    act(() => container.querySelector<HTMLButtonElement>('button:first-of-type')?.click())
    expect(document.documentElement.lang).toBe('ja-JP')
    expect(container.querySelector('[data-testid="dashboard"]')?.textContent).toBe('ダッシュボード')

    act(() => container.querySelector<HTMLButtonElement>('button:last-of-type')?.click())
    expect(document.documentElement.lang).toBe('zh-CN')
    expect(container.querySelector('[data-testid="my-dashboard"]')?.textContent).toBe('我的数据大盘')

    act(() => root.unmount())
  })
})
