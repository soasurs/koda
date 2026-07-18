import { describe, expect, it } from 'vitest'

import {
  formatContextWindowTokens,
  parseContextWindowTokens,
} from '@/components/providers/provider-types'

describe('provider model context windows', () => {
  it('parses an optional positive integer', () => {
    expect(parseContextWindowTokens('')).toBe(0n)
    expect(parseContextWindowTokens(' 400000 ')).toBe(400000n)
    expect(parseContextWindowTokens('0')).toBeNull()
    expect(parseContextWindowTokens('-1')).toBeNull()
    expect(parseContextWindowTokens('1.5')).toBeNull()
    expect(parseContextWindowTokens('9223372036854775808')).toBeNull()
  })

  it('formats token counts for display', () => {
    expect(formatContextWindowTokens(1050000n)).toBe('1,050,000')
  })
})
