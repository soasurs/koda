import { render, screen } from '@testing-library/react'
import type { ReactNode } from 'react'
import { describe, expect, it, vi } from 'vitest'

vi.mock('@tanstack/react-router', () => ({
  Link: ({ children }: { children: ReactNode }) => <a>{children}</a>,
}))

import { HomePage } from '@/pages/home-page'

describe('HomePage', () => {
  it('renders the empty session state', () => {
    render(<HomePage />)

    expect(
      screen.getByRole('heading', { name: 'Start with a session' }),
    ).toBeInTheDocument()
  })
})
