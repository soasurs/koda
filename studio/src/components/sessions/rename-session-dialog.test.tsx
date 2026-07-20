import { create } from '@bufbuild/protobuf'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { describe, expect, it, vi } from 'vitest'

import { I18nProvider } from '@/app/i18n'
import { RenameSessionDialog } from '@/components/sessions/rename-session-dialog'
import { SessionSchema } from '@/gen/koda/v1/service_pb'

const mocks = vi.hoisted(() => ({
  updateSession: vi.fn(),
}))

vi.mock('@/lib/connect', () => ({
  kodaClient: { updateSession: mocks.updateSession },
}))

describe('RenameSessionDialog', () => {
  it('renames a session and trims its title', async () => {
    const session = create(SessionSchema, {
      id: 'session-1',
      title: 'Old title',
    })
    const updatedSession = create(SessionSchema, {
      id: 'session-1',
      title: 'New title',
    })
    mocks.updateSession.mockResolvedValue({ session: updatedSession })
    const onClose = vi.fn()
    const queryClient = new QueryClient({
      defaultOptions: { queries: { retry: false } },
    })
    render(
      <QueryClientProvider client={queryClient}>
        <I18nProvider>
          <RenameSessionDialog onClose={onClose} session={session} />
        </I18nProvider>
      </QueryClientProvider>,
    )

    const input = screen.getByRole('textbox', { name: 'Name' })
    await userEvent.clear(input)
    await userEvent.type(input, '  New title  ')
    await userEvent.click(screen.getByRole('button', { name: 'Rename' }))

    expect(mocks.updateSession).toHaveBeenCalledWith({
      sessionId: 'session-1',
      title: 'New title',
    })
    expect(onClose).toHaveBeenCalledOnce()
  })
})
