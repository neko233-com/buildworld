import { expect, it } from 'vitest'
import { configuredReleaseTarget, normalizeReleaseTargetCatalog, preferredReleaseEnvironment } from './releaseTargets'

it('uses a configured non-cartesian matrix with arbitrary channels and N environments', () => {
  const catalog = normalizeReleaseTargetCatalog({ schemaVersion: 1, targets: [{ channel: 'alpha', environment: 'release' }, { channel: 'alpha', environment: 'test' }, { channel: 'third-store', environment: 'audit-2' }] })!
  expect(catalog).not.toBeNull()
  expect(preferredReleaseEnvironment(catalog.targets, 'alpha', 'release')).toBe('test')
  expect(preferredReleaseEnvironment(catalog.targets, 'third-store', 'test')).toBe('audit-2')
  expect(configuredReleaseTarget(catalog.targets, 'third-store', 'audit-2')).toBe(true)
  expect(configuredReleaseTarget(catalog.targets, 'third-store', 'test')).toBe(false)
})
it('fails closed on empty, invalid, duplicate, unsafe or malformed targets', () => {
  for (const targets of [[], [{ channel: 'Alpha', environment: 'test' }], [{ channel: 'alpha', environment: '../test' }], [{ channel: 'alpha', environment: 'test' }, { channel: 'alpha', environment: 'test' }], [{ channel: 'alpha', environment: 'test', channelLabel: 'label\ninvalid' }], [{ channel: 'alpha', environment: null }]]) {
    expect(normalizeReleaseTargetCatalog({ schemaVersion: 1, targets })).toBeNull()
  }
  expect(normalizeReleaseTargetCatalog({ schemaVersion: 2, targets: [{ channel: 'alpha', environment: 'test' }] })).toBeNull()
  expect(normalizeReleaseTargetCatalog({ schemaVersion: 1, targets: [{ channel: 'alpha', environment: 'test', password: 'invalid' }] })).toBeNull()
  expect(normalizeReleaseTargetCatalog({ schemaVersion: 1, targets: [{ channel: 'alpha', environment: 'test' }], extra: true })).toBeNull()
  expect(normalizeReleaseTargetCatalog({ schemaVersion: 1, targets: [{ channel: 'alpha', environment: 'test', channelLabel: 'Alpha' }, { channel: 'alpha', environment: 'audit', channelLabel: 'Different' }] })).toBeNull()
})
