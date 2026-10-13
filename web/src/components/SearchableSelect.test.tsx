// @vitest-environment jsdom
import { act, useState } from 'react'
import { createRoot, type Root } from 'react-dom/client'
import { afterEach, beforeEach, expect, it, vi } from 'vitest'
import { SearchableSelect } from './SearchableSelect'

let container: HTMLDivElement
let root: Root
const options = [{ value: 'alpha', label: '阿尔法' }, { value: 'beta', label: '贝塔' }, { value: 'third-store', label: '第三渠道' }]
beforeEach(() => {
  ;(globalThis as any).IS_REACT_ACT_ENVIRONMENT = true
  container = document.createElement('div')
  document.body.append(container)
  root = createRoot(container)
})
afterEach(() => { act(() => root.unmount()); container.remove() })
function Picker({ parentKey }: { parentKey: (event: unknown) => void }) {
  const [value, setValue] = useState('alpha')
  return <form onKeyDown={parentKey}><label htmlFor="picker">渠道</label><SearchableSelect id="picker" value={value} onChange={setValue} options={options} placeholder="搜索渠道" emptyLabel="没有匹配目标" keyboardHint="↑ ↓ Enter Esc" /></form>
}
async function key(input: HTMLInputElement, name: string) {
  const event = new KeyboardEvent('keydown', { key: name, bubbles: true, cancelable: true })
  await act(async () => input.dispatchEvent(event))
  return event
}
async function search(input: HTMLInputElement, value: string) {
  await act(async () => {
    Object.getOwnPropertyDescriptor(HTMLInputElement.prototype, 'value')!.set!.call(input, value)
    input.dispatchEvent(new Event('input', { bubbles: true }))
  })
}
it('supports keyboard selection, focus retention, escaped popup closure and next-field tabbing', async () => {
  const parentKey = vi.fn()
  await act(async () => root.render(<Picker parentKey={parentKey} />))
  const input = container.querySelector<HTMLInputElement>('[role="combobox"]')!
  expect(input.value).toBe('阿尔法')
  await act(async () => input.focus())
  expect(input.getAttribute('aria-expanded')).toBe('true')
  await key(input, 'End')
  expect(document.getElementById(input.getAttribute('aria-activedescendant')!)?.textContent).toContain('第三渠道')
  await key(input, 'ArrowDown')
  expect(document.getElementById(input.getAttribute('aria-activedescendant')!)?.textContent).toContain('阿尔法')
  await key(input, 'ArrowUp')
  expect((await key(input, 'Enter')).defaultPrevented).toBe(true)
  expect(input.value).toBe('第三渠道')
  expect(input.getAttribute('aria-expanded')).toBe('false')
  expect(document.activeElement).toBe(input)
  await key(input, 'ArrowDown')
  await search(input, 'beta')
  expect(container.querySelectorAll('[role="option"]')).toHaveLength(1)
  parentKey.mockClear()
  const escape = await key(input, 'Escape')
  expect(escape.defaultPrevented).toBe(true)
  expect(parentKey).not.toHaveBeenCalled()
  expect(input.value).toBe('第三渠道')
  expect(input.getAttribute('aria-activedescendant')).toBeNull()
  await key(input, 'ArrowDown')
  expect((await key(input, 'Tab')).defaultPrevented).toBe(false)
  expect(input.getAttribute('aria-expanded')).toBe('false')
})
it('searches labels and codes, announces no matches and never commits free text', async () => {
  await act(async () => root.render(<Picker parentKey={() => {}} />))
  const input = container.querySelector<HTMLInputElement>('[role="combobox"]')!
  await act(async () => input.focus())
  await search(input, '不存在')
  expect(container.querySelector('[role="status"]')?.textContent).toBe('没有匹配目标')
  expect((await key(input, 'Enter')).defaultPrevented).toBe(true)
  await key(input, 'Escape')
  expect(input.value).toBe('阿尔法')
  await key(input, 'ArrowDown')
  await search(input, 'BEta')
  expect(container.querySelectorAll('[role="option"]')).toHaveLength(1)
  await act(async () => (container.querySelector('[role="option"]') as HTMLButtonElement).click())
  expect(input.value).toBe('贝塔')
  expect(document.activeElement).toBe(input)
})
