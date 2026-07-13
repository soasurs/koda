import { cleanup, fireEvent, render, screen } from '@testing-library/react'
import type { ReactNode } from 'react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

import { SidebarContext } from '@/components/layout/sidebar-context'

vi.mock('@tanstack/react-router', () => ({
  Link: ({ children }: { children: ReactNode }) => <a href="/">{children}</a>,
}))

import { SettingsLayout } from '@/components/settings/settings-layout'

describe('SettingsLayout', () => {
  beforeEach(() => window.localStorage.clear())
  afterEach(cleanup)

  it('expands a collapsed app sidebar', () => {
    const setCollapsed = vi.fn()

    render(
      <SidebarContext.Provider value={{ collapsed: true, setCollapsed }}>
        <SettingsLayout active="providers">Settings content</SettingsLayout>
      </SidebarContext.Provider>,
    )

    fireEvent.click(screen.getByRole('button', { name: 'Expand sidebar' }))

    expect(setCollapsed).toHaveBeenCalledWith(false)
    expect(window.localStorage.getItem('koda-studio-sidebar-collapsed')).toBe(
      'false',
    )
  })

  it('hides the expand control while the app sidebar is open', () => {
    render(
      <SidebarContext.Provider
        value={{ collapsed: false, setCollapsed: vi.fn() }}
      >
        <SettingsLayout active="skills">Settings content</SettingsLayout>
      </SidebarContext.Provider>,
    )

    expect(
      screen.queryByRole('button', { name: 'Expand sidebar' }),
    ).not.toBeInTheDocument()
  })
})
