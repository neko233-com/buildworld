import { expect, it } from 'vitest'
import { validReleaseEnvironment } from './releasePresentation'

it('keeps environment names consistent with the server, including final newlines', () => {
  expect(validReleaseEnvironment('internal-2')).toBe(true)
  for (const value of ['test\n', '../release', 'Release', ' test', '1test', 'qa_test', 'abcdefghijklmnopqrstuvwxyz1234567']) {
    expect(validReleaseEnvironment(value), value).toBe(false)
  }
})
