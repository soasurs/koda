import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { cleanup, render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

import { I18nProvider } from '@/app/i18n'
import { SessionModelPicker } from '@/components/sessions/session-model-picker'
import type { Session } from '@/gen/koda/v1/service_pb'

const mocks = vi.hoisted(() => ({
  listModels: vi.fn(async () => ({
    models: [
      {
        defaultReasoningEffort: 'medium',
        id: 'model-1',
        name: 'Model One',
        reasoningEfforts: ['low', 'medium', 'high'],
      },
    ],
  })),
  listProviders: vi.fn(async () => [
    { configured: true, enabled: true, id: 'provider-1', name: 'Provider One' },
    { configured: true, enabled: true, id: 'provider-2', name: 'Provider Two' },
  ]),
  updateSession: vi.fn(),
}))

vi.mock('@/lib/connect', () => ({
  kodaClient: {
    listModels: mocks.listModels,
    updateSession: mocks.updateSession,
  },
}))

vi.mock('@/lib/koda', async (importOriginal) => ({
  ...(await importOriginal<typeof import('@/lib/koda')>()),
  listProviders: mocks.listProviders,
}))

beforeEach(() => {
  HTMLElement.prototype.hasPointerCapture = vi.fn(() => false)
  HTMLElement.prototype.setPointerCapture = vi.fn()
  HTMLElement.prototype.releasePointerCapture = vi.fn()
  HTMLElement.prototype.scrollIntoView = vi.fn()
})

afterEach(() => {
  cleanup()
  vi.clearAllMocks()
})

function renderPicker() {
  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  })
  render(
    <QueryClientProvider client={queryClient}>
      <I18nProvider>
        <SessionModelPicker
          disabled={false}
          session={
            {
              id: 'session-1',
              modelId: 'model-1',
              providerId: 'provider-1',
              reasoningEffort: 'medium',
            } as Session
          }
        />
      </I18nProvider>
    </QueryClientProvider>,
  )
}

describe('SessionModelPicker', () => {
  it('keeps model settings open while selecting from a portaled menu', async () => {
    const user = userEvent.setup()
    renderPicker()

    await user.click(
      screen.getByRole('button', { name: 'Session model settings' }),
    )
    await user.click(
      await screen.findByRole('combobox', { name: 'Session provider' }),
    )

    await user.click(
      await screen.findByRole('option', { name: 'Provider Two' }),
    )

    expect(screen.getByRole('button', { name: 'Apply' })).toBeInTheDocument()
  })
})
