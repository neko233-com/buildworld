export type UserRole = 'admin' | 'developer' | 'viewer'

const knownRoles = new Set<UserRole>(['admin', 'developer', 'viewer'])

export function roleFromToken(token: string | null): UserRole {
  if (!token) return 'viewer'
  try {
    const payload = token.split('.')[1]
    if (!payload) return 'viewer'
    const normalized = payload.replace(/-/g, '+').replace(/_/g, '/')
    const padded = normalized.padEnd(Math.ceil(normalized.length / 4) * 4, '=')
    const role = JSON.parse(atob(padded))?.role
    return knownRoles.has(role) ? role : 'viewer'
  } catch {
    return 'viewer'
  }
}

export function currentRole(): UserRole {
  return roleFromToken(localStorage.getItem('token'))
}

export function canEdit(role = currentRole()): boolean {
  return role === 'admin' || role === 'developer'
}

export function isAdmin(role = currentRole()): boolean {
  return role === 'admin'
}
