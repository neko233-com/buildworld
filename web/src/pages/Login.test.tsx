// @vitest-environment jsdom

import { act } from 'react'
import { createRoot, type Root } from 'react-dom/client'
import { readFileSync } from 'node:fs'
import { resolve } from 'node:path'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { api } from '../api'
import Login from './Login'

vi.mock('../api', () => ({
  api: { login: vi.fn() },
  setToken: vi.fn(),
}))

const actEnvironment = globalThis as typeof globalThis & { IS_REACT_ACT_ENVIRONMENT: boolean }
const login = vi.mocked(api.login)
const loginStyles = readFileSync(resolve(process.cwd(), 'src/pages/Login.jenkins.css'), 'utf8')

function inputValue(input: HTMLInputElement, value: string) {
  const setter = Object.getOwnPropertyDescriptor(HTMLInputElement.prototype, 'value')?.set
  act(() => {
    setter?.call(input, value)
    input.dispatchEvent(new Event('input', { bubbles: true }))
  })
}

async function submit(form: HTMLFormElement) {
  await act(async () => {
    form.dispatchEvent(new Event('submit', { bubbles: true, cancelable: true }))
    await Promise.resolve()
  })
}

describe('Login', () => {
  let container: HTMLDivElement
  let root: Root

  beforeEach(() => {
    actEnvironment.IS_REACT_ACT_ENVIRONMENT = true
    localStorage.clear()
    login.mockReset()
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

  it('renders one Jenkins-sized sign-in column without the marketing split', () => {
    expect(container.querySelector('.jenkins-login-page')).not.toBeNull()
    expect(container.querySelector('.jenkins-login-content')).not.toBeNull()
    expect(container.querySelector('.jenkins-login-brand')).not.toBeNull()
    expect(container.querySelectorAll('form')).toHaveLength(1)
    expect(container.querySelector('.login-context')).toBeNull()
    expect(container.querySelector('.login-capabilities')).toBeNull()
    expect(loginStyles).toMatch(/\.jenkins-login-content\s*\{[^}]*width:\s*min\(430px, 100%\)/s)
    expect(loginStyles).toMatch(/\.jenkins-login-heading h1\s*\{[^}]*font-family:\s*Georgia/s)
    expect(loginStyles).not.toContain('grid-template-columns')
  })

  it('associates labels and exposes password visibility safely', () => {
    expect(container.querySelector('label[for="login-username"]')).not.toBeNull()
    expect(container.querySelector('label[for="login-password"]')).not.toBeNull()
    const password = container.querySelector<HTMLInputElement>('#login-password')!
    const toggle = container.querySelector<HTMLButtonElement>('.jenkins-login-password button')!
    expect(password.type).toBe('password')
    act(() => toggle.click())
    expect(password.type).toBe('text')
  })

  it('locks every credential control and shows progress while authentication is pending', async () => {
    login.mockImplementation(() => new Promise(() => undefined))
    const username = container.querySelector<HTMLInputElement>('#login-username')!
    const password = container.querySelector<HTMLInputElement>('#login-password')!
    inputValue(username, 'admin')
    inputValue(password, 'secret')

    await submit(container.querySelector<HTMLFormElement>('form')!)

    expect(container.querySelector('form')?.getAttribute('aria-busy')).toBe('true')
    expect(username.disabled).toBe(true)
    expect(password.disabled).toBe(true)
    expect(container.querySelector<HTMLSelectElement>('#login-language')?.disabled).toBe(true)
    expect(container.querySelector<HTMLButtonElement>('.jenkins-login-password button')?.disabled).toBe(true)
    expect(container.querySelector('.jenkins-login-spinner')).not.toBeNull()
  })

  it('trims the username and exposes authentication failures to assistive technology', async () => {
    login.mockRejectedValue(new Error('Invalid username or password'))
    const username = container.querySelector<HTMLInputElement>('#login-username')!
    const password = container.querySelector<HTMLInputElement>('#login-password')!
    inputValue(username, ' admin ')
    inputValue(password, 'secret')

    await submit(container.querySelector<HTMLFormElement>('form')!)

    expect(login).toHaveBeenCalledWith('admin', 'secret')
    expect(container.querySelector('[role="alert"]')?.textContent).toBe('Invalid username or password')
    expect(username.getAttribute('aria-invalid')).toBe('true')
    expect(password.getAttribute('aria-describedby')).toBe('login-error')
  })
})
