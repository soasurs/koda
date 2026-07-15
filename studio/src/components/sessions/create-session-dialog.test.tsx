import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { cleanup, render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

const mocks = vi.hoisted(() => ({
  createSession: vi.fn(),
  listDirectories: vi.fn(),
  listModels: vi.fn(),
  listProviders: vi.fn(),
  navigate: vi.fn(),
}))

vi.mock('@tanstack/react-router', () => ({
  useNavigate: () => mocks.navigate,
}))

vi.mock('@/lib/connect', () => ({
  kodaClient: {
    createSession: mocks.createSession,
    listDirectories: mocks.listDirectories,
    listModels: mocks.listModels,
    listProviders: mocks.listProviders,
  },
}))

import { CreateSessionDialog } from '@/components/sessions/create-session-dialog'

function renderDialog(onClose = vi.fn()) {
  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  })
  render(
    <QueryClientProvider client={queryClient}>
      <CreateSessionDialog onClose={onClose} />
    </QueryClientProvider>,
  )
  return onClose
}

describe('CreateSessionDialog', () => {
  afterEach(cleanup)

  beforeEach(() => {
    vi.clearAllMocks()
    mocks.listProviders.mockResolvedValue({ providers: [] })
    mocks.listDirectories.mockResolvedValue({
      path: '/workspace',
      parentPath: '/',
      directories: [],
    })
  })

  it('opens the directory picker without submitting the form', async () => {
    renderDialog()

    await userEvent.click(screen.getByRole('button', { name: 'Workspace' }))

    expect(
      await screen.findByRole('heading', { name: 'Choose workspace' }),
    ).toBeInTheDocument()
    await userEvent.click(
      screen.getByRole('button', { name: 'Select this directory' }),
    )

    expect(screen.getByText('/workspace')).toBeInTheDocument()
    expect(mocks.createSession).not.toHaveBeenCalled()
  })

  it('closes without submitting the form', async () => {
    const onClose = renderDialog()

    await userEvent.click(screen.getByRole('button', { name: 'Cancel' }))

    expect(onClose).toHaveBeenCalledOnce()
    expect(mocks.createSession).not.toHaveBeenCalled()
  })
})
