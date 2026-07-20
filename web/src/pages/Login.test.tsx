// @vitest-environment jsdom

import { act } from 'react'
import { createRoot, type Root } from 'react-dom/client'
import { afterEach, beforeEach, describe, expect, it } from 'vitest'
import Login from './Login'

const actEnvironment = globalThis as typeof globalThis & { IS_REACT_ACT_ENVIRONMENT: boolean }

describe('Login', () => {
  let container: HTMLDivElement
  let root: Root

  beforeEach(() => {
    actEnvironment.IS_REACT_ACT_ENVIRONMENT = true
    localStorage.clear()
    container = document.createElement('div')
    document.body.appendChild(container)
    root = createRoot(container)
    act(() => root.render(<Login />))
  })

  afterEach(() => {
    act(() => root.unmount())
    container.remove()
    localStorage.clear()
    actEnvironment.IS_REACT_ACT_ENVIRONMENT = false
  })

  it('does not expose or prefill default credentials', () => {
    const username = container.querySelector<HTMLInputElement>('#login-username')!
    const password = container.querySelector<HTMLInputElement>('#login-password')!
    expect(username.value).toBe('')
    expect(password.value).toBe('')
    expect(container.textContent).not.toContain('root / root')
  })

  it('associates labels and exposes password visibility safely', () => {
    expect(container.querySelector('label[for="login-username"]')).not.toBeNull()
    expect(container.querySelector('label[for="login-password"]')).not.toBeNull()
    const password = container.querySelector<HTMLInputElement>('#login-password')!
    const toggle = container.querySelector<HTMLButtonElement>('.login-password-field button')!
    expect(password.type).toBe('password')
    act(() => toggle.click())
    expect(password.type).toBe('text')
  })
})
