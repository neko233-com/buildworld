// @vitest-environment jsdom

import { afterEach, describe, expect, it } from 'vitest'
import { canEdit, isAdmin, roleFromToken } from './authz'

function tokenFor(role: string) {
  const payload = btoa(JSON.stringify({ sub: 7, role })).replace(/\+/g, '-').replace(/\//g, '_').replace(/=+$/, '')
  return `header.${payload}.signature`
}

describe('role authorization helpers', () => {
  afterEach(() => localStorage.clear())

  it('reads supported roles from JWT payloads', () => {
    expect(roleFromToken(tokenFor('admin'))).toBe('admin')
    expect(roleFromToken(tokenFor('developer'))).toBe('developer')
    expect(roleFromToken(tokenFor('viewer'))).toBe('viewer')
  })

  it('fails closed for malformed or unknown claims', () => {
    expect(roleFromToken(null)).toBe('viewer')
    expect(roleFromToken('broken')).toBe('viewer')
    expect(roleFromToken(tokenFor('owner'))).toBe('viewer')
  })

  it('separates administration from build editing', () => {
    expect(isAdmin('admin')).toBe(true)
    expect(isAdmin('developer')).toBe(false)
    expect(canEdit('developer')).toBe(true)
    expect(canEdit('viewer')).toBe(false)
  })
})
