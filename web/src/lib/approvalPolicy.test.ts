import { describe, expect, it } from 'vitest'
import { readApprovalPolicy, writeApprovalPolicy } from './approvalPolicy'

const enabled = {
  enabled: true,
  requiredRoles: ['admin', 'developer'] as Array<'admin' | 'developer'>,
  allowRequester: false,
  prompt: 'Release?',
}

describe('approval policy configuration', () => {
  it('round trips jobs-based YAML project configuration', () => {
    const source = 'jobs:\n  build:\n    steps:\n      - run: echo ok\n'
    const written = writeApprovalPolicy(source, enabled)
    expect(readApprovalPolicy(written)).toEqual(enabled)
    expect(readApprovalPolicy(writeApprovalPolicy(written, { ...enabled, enabled: false })).enabled).toBe(false)
  })

  it.each(['{"stages":[]}', '# Markdown pipeline'])('rejects removed authoring format %s', source => {
    expect(() => writeApprovalPolicy(source, enabled)).toThrow('jobs-based YAML')
  })
})
