import { create } from '@bufbuild/protobuf'
import { cleanup, render, screen } from '@testing-library/react'
import { afterEach, describe, expect, it, vi } from 'vitest'

import { SessionHeader } from '@/components/sessions/session-header'
import { ContextUsageSchema, SessionSchema } from '@/gen/koda/v1/service_pb'

vi.mock('@/components/layout/sidebar-expand-button', () => ({
  SidebarExpandButton: () => null,
}))

afterEach(cleanup)

describe('SessionHeader', () => {
  it('shows used, remaining, and percentage values', () => {
    render(
      <SessionHeader
        session={create(SessionSchema, {
          title: 'Context session',
          contextUsage: create(ContextUsageSchema, {
            measured: true,
            usedTokens: 32_000n,
            windowTokens: 256_000n,
          }),
        })}
      />,
    )

    expect(screen.getByRole('meter')).toHaveAccessibleName(
      '32,000 tokens used, 224,000 remaining, 13% of 256,000',
    )
    expect(
      screen.getByText('32K used · 224K left · 13% of 256K'),
    ).toBeInTheDocument()
  })

  it('reports unavailable usage before the first provider measurement', () => {
    render(
      <SessionHeader
        session={create(SessionSchema, {
          contextUsage: create(ContextUsageSchema, {
            windowTokens: 256_000n,
          }),
          workdir: '/workspace',
        })}
      />,
    )

    expect(screen.getByText('Context — / 256K')).toBeInTheDocument()
    expect(screen.queryByRole('meter')).not.toBeInTheDocument()
  })
})
