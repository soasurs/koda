import { create } from '@bufbuild/protobuf'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import type { ReactNode } from 'react'
import { beforeEach, describe, expect, it, vi } from 'vitest'

import { I18nProvider } from '@/app/i18n'
import { SessionSchema } from '@/gen/koda/v1/service_pb'

const mocks = vi.hoisted(() => ({
  listSessions: vi.fn(),
  updateSession: vi.fn(),
}))

vi.mock('@tanstack/react-router', () => ({
  Link: ({ children }: { children: ReactNode }) => <a href="/">{children}</a>,
}))

vi.mock('@/components/settings/settings-layout', () => ({
  SettingsLayout: ({ children }: { children: ReactNode }) => (
    <div>{children}</div>
  ),
}))

vi.mock('@/lib/connect', () => ({
  kodaClient: { updateSession: mocks.updateSession },
}))

vi.mock('@/lib/koda', async (importOriginal) => ({
  ...(await importOriginal<typeof import('@/lib/koda')>()),
  listSessions: mocks.listSessions,
}))

import { SessionSettingsPage } from '@/pages/session-settings-page'

describe('SessionSettingsPage', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    mocks.listSessions.mockResolvedValue([
      create(SessionSchema, {
        id: 'session-1',
        title: 'Archived work',
        workdir: '/workspace/project',
        archivedAt: 1n,
      }),
    ])
    mocks.updateSession.mockResolvedValue({
      session: create(SessionSchema, {
        id: 'session-1',
        title: 'Archived work',
        workdir: '/workspace/project',
      }),
    })
  })

  it('lists archived sessions and restores them', async () => {
    const queryClient = new QueryClient({
      defaultOptions: { queries: { retry: false } },
    })
    render(
      <QueryClientProvider client={queryClient}>
        <I18nProvider>
          <SessionSettingsPage />
        </I18nProvider>
      </QueryClientProvider>,
    )

    expect(await screen.findByText('Archived work')).toBeInTheDocument()
    expect(mocks.listSessions).toHaveBeenCalledWith(true)

    await userEvent.click(screen.getByRole('button', { name: 'Restore' }))

    expect(mocks.updateSession).toHaveBeenCalledWith({
      sessionId: 'session-1',
      archived: false,
    })
    expect(await screen.findByText('No archived sessions')).toBeInTheDocument()
  })
})
