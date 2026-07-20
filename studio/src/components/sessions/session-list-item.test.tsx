import { create } from '@bufbuild/protobuf'
import { cleanup, fireEvent, render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import type { ComponentProps, ReactNode } from 'react'
import { afterEach, describe, expect, it, vi } from 'vitest'

import { I18nProvider } from '@/app/i18n'
import { SessionListItem } from '@/components/sessions/session-list-item'
import { SessionSchema } from '@/gen/koda/v1/service_pb'

vi.mock('@tanstack/react-router', () => ({
  Link: ({
    activeProps: _activeProps,
    children,
    params: _params,
    to: _to,
    ...props
  }: ComponentProps<'a'> & {
    activeProps?: unknown
    children: ReactNode
    params?: unknown
    to?: unknown
  }) => {
    void _activeProps
    void _params
    void _to
    return (
      <a href="/" {...props}>
        {children}
      </a>
    )
  },
}))

afterEach(cleanup)

describe('SessionListItem', () => {
  it('archives a session from its context menu', async () => {
    const onArchive = vi.fn()
    render(
      <I18nProvider>
        <SessionListItem
          archiving={false}
          onArchive={onArchive}
          onRename={vi.fn()}
          session={create(SessionSchema, {
            id: 'session-1',
            title: 'Context menu session',
          })}
        />
      </I18nProvider>,
    )

    fireEvent.contextMenu(screen.getByRole('link'))
    await userEvent.click(
      await screen.findByRole('menuitem', { name: 'Archive' }),
    )

    expect(onArchive).toHaveBeenCalledOnce()
  })

  it('opens rename from its context menu', async () => {
    const onRename = vi.fn()
    render(
      <I18nProvider>
        <SessionListItem
          archiving={false}
          onArchive={vi.fn()}
          onRename={onRename}
          session={create(SessionSchema, {
            id: 'session-1',
            title: 'Rename me',
          })}
        />
      </I18nProvider>,
    )

    fireEvent.contextMenu(screen.getByRole('link'))
    await userEvent.click(
      await screen.findByRole('menuitem', { name: 'Rename' }),
    )

    expect(onRename).toHaveBeenCalledOnce()
  })
})
