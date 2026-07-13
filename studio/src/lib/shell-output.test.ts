import { describe, expect, it } from 'vitest'

import { parseShellOutput } from '@/lib/shell-output'

describe('parseShellOutput', () => {
  it('parses the structured shell result', () => {
    expect(
      parseShellOutput(
        JSON.stringify({
          exit_code: 1,
          stderr: 'failed',
          stdout: 'started',
          truncated: true,
        }),
        '',
      ),
    ).toEqual({
      exitCode: 1,
      stderr: 'failed',
      stdout: 'started',
      truncated: true,
    })
  })

  it('falls back when the structured result is incompatible', () => {
    expect(
      parseShellOutput(
        'not json',
        JSON.stringify({ exit_code: 0, stderr: '', stdout: 'ok' }),
      ),
    ).toEqual({
      exitCode: 0,
      stderr: '',
      stdout: 'ok',
      truncated: false,
    })
  })

  it('returns null when neither result is compatible', () => {
    expect(parseShellOutput('{}', '')).toBeNull()
  })
})
