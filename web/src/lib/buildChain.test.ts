import { describe, expect, it } from 'vitest'
import { buildChainRelationKey } from './buildChain'

describe('build chain presentation', () => {
  it.each([
    ['retry', 'builds.chainRetry'],
    ['dependency', 'builds.chainDependency'],
    ['promotion', 'builds.chainRelated'],
  ])('maps %s relationship to an extensible presentation key', (type, key) => {
    expect(buildChainRelationKey(type)).toBe(key)
  })
})
