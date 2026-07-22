import { describe, expect, it } from 'vitest'
import {
  completePortabilitySections,
  hasSensitiveSelection,
  humanizePortabilityKey,
  isPortabilityBundleSizeAllowed,
  MAX_PORTABILITY_BUNDLE_BYTES,
  missingPortabilityDependencies,
  resolveSettingsSection,
  safePortabilitySections,
  sanitizePortabilitySections,
  togglePortabilitySection,
  type PortabilityCapability,
} from './portabilitySelection'

const capabilities: PortabilityCapability[] = [
  { key: 'credentials', version: 1, sensitive: true },
  { key: 'projects', version: 2, sensitive: false, depends_on: ['templates', 'vcs_roots'] },
  { key: 'settings', version: 1, sensitive: false },
  { key: 'templates', version: 1, sensitive: false },
  { key: 'vcs_roots', version: 2, sensitive: false, depends_on: ['credentials'] },
]

describe('settings navigation', () => {
  it('accepts known deep links and canonicalizes unknown values', () => {
    expect(resolveSettingsSection('portability')).toBe('portability')
    expect(resolveSettingsSection('appearance')).toBe('overview')
    expect(resolveSettingsSection('unknown')).toBe('overview')
    expect(resolveSettingsSection(null)).toBe('overview')
  })
})

describe('portability selection', () => {
  it('builds safe and complete presets from server capabilities', () => {
    expect(safePortabilitySections(capabilities)).toEqual(['projects', 'settings', 'templates', 'vcs_roots'])
    expect(completePortabilitySections(capabilities)).toEqual(['credentials', 'projects', 'settings', 'templates', 'vcs_roots'])
  })

  it('blocks sensitive sections until secret export is explicitly enabled', () => {
    expect(togglePortabilitySection(['settings'], capabilities[0], false)).toEqual({
      sections: ['settings'],
      blockedBySecrets: true,
    })
    expect(togglePortabilitySection(['settings'], capabilities[0], true)).toEqual({
      sections: ['settings', 'credentials'],
      blockedBySecrets: false,
    })
  })

  it('removes sensitive and unknown selections whenever secrets are disabled', () => {
    expect(sanitizePortabilitySections(capabilities, ['settings', 'credentials', 'future', 'settings'], false)).toEqual(['settings'])
    expect(hasSensitiveSelection(capabilities, ['settings', 'credentials'])).toBe(true)
  })

  it('reports focused bundles that rely on configuration already present in the target', () => {
    expect(missingPortabilityDependencies(capabilities, ['projects', 'templates'])).toEqual([
      { section: 'projects', dependency: 'vcs_roots' },
    ])
  })

  it('provides a readable fallback for future strategy keys', () => {
    expect(humanizePortabilityKey('cloud_signing_profiles')).toBe('Cloud Signing Profiles')
  })

  it('enforces the documented 32 MB bundle boundary', () => {
    expect(isPortabilityBundleSizeAllowed(0)).toBe(true)
    expect(isPortabilityBundleSizeAllowed(MAX_PORTABILITY_BUNDLE_BYTES)).toBe(true)
    expect(isPortabilityBundleSizeAllowed(MAX_PORTABILITY_BUNDLE_BYTES + 1)).toBe(false)
    expect(isPortabilityBundleSizeAllowed(Number.NaN)).toBe(false)
  })
})
