import { describe, expect, it } from 'vitest'

import { kodaClient } from '@/lib/connect'

describe('kodaClient', () => {
  it('exposes the generated directory browsing method', () => {
    expect(kodaClient.listDirectories).toBeTypeOf('function')
  })
})
