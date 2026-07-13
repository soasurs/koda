import { create } from '@bufbuild/protobuf'
import { describe, expect, it } from 'vitest'

import { SessionSchema } from '@/gen/koda/v1/service_pb'
import { replaceSession } from '@/lib/koda'

describe('replaceSession', () => {
  it('replaces the matching session with the completed snapshot', () => {
    const first = create(SessionSchema, { id: 'first', title: '' })
    const second = create(SessionSchema, { id: 'second', title: 'Existing' })
    const completed = create(SessionSchema, {
      id: 'first',
      title: 'Generated title',
    })

    expect(replaceSession([first, second], completed)).toEqual([
      completed,
      second,
    ])
  })
})
