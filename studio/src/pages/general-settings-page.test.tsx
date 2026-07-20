import { cleanup, fireEvent, render, screen } from '@testing-library/react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

import { GeneralSettingsPage } from '@/pages/general-settings-page'
import { AllProviders } from '@/test/providers'

vi.mock('@tanstack/react-router', () => ({
  Link: ({ children, to }: { children: React.ReactNode; to: string }) => (
    <a href={to}>{children}</a>
  ),
}))

vi.mock('@/components/layout/sidebar-expand-button', () => ({
  SidebarExpandButton: () => null,
}))

afterEach(cleanup)

describe('GeneralSettingsPage', () => {
  beforeEach(() => {
    window.localStorage.clear()
    window.localStorage.setItem('koda-studio-locale', 'en')
  })

  it('renders appearance and conversation sections', () => {
    render(
      <AllProviders>
        <GeneralSettingsPage />
      </AllProviders>,
    )
    expect(screen.getByText('Appearance')).toBeInTheDocument()
    expect(screen.getByText('Conversation')).toBeInTheDocument()
    expect(screen.getByText('Expand reasoning by default')).toBeInTheDocument()
    expect(screen.getByText('Expand tool calls by default')).toBeInTheDocument()
  })

  it('toggles the reasoning preference and persists it', () => {
    render(
      <AllProviders>
        <GeneralSettingsPage />
      </AllProviders>,
    )
    const reasoningSwitch = screen.getByRole('switch', {
      name: 'Expand reasoning by default',
    })
    expect(reasoningSwitch).not.toBeChecked()
    fireEvent.click(reasoningSwitch)
    expect(reasoningSwitch).toBeChecked()
    const stored = JSON.parse(
      window.localStorage.getItem('koda-studio-preferences') ?? '{}',
    ) as Record<string, unknown>
    expect(stored.expandReasoning).toBe(true)
  })
})
