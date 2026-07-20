import { describe, expect, it } from 'vitest'
import { readApprovalPolicy, writeApprovalPolicy } from './approvalPolicy'

const enabled = {
  enabled: true,
  requiredRoles: ['admin', 'developer'] as Array<'admin' | 'developer'>,
  allowRequester: false,
  prompt: 'Release?',
}

describe('approval policy configuration', () => {
  it.each([
    ['json', '{"stages":[]}'],
    ['yaml', 'stages: []\n'],
    ['markdown', '# Demo\n\n## Pipeline\n\n### Build\n```default shell\necho ok\n```\n'],
  ])('round trips %s project configuration', (_format, source) => {
    const written = writeApprovalPolicy(source, enabled)
    expect(readApprovalPolicy(written)).toEqual(enabled)
    expect(readApprovalPolicy(writeApprovalPolicy(written, { ...enabled, enabled: false })).enabled).toBe(false)
  })

  it('replaces an existing Markdown section without duplication', () => {
    const source = '# Demo\n\n## Approval\n- strategy: single\n\n## Pipeline\n'
    const written = writeApprovalPolicy(source, enabled)
    expect(written.match(/^## Approval$/gm)).toHaveLength(1)
  })
})
