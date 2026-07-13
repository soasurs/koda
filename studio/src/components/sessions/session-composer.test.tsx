import { cleanup, fireEvent, render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { createRef } from 'react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

import { SessionComposer } from '@/components/sessions/session-composer'
import type { Session } from '@/gen/koda/v1/service_pb'
import { AgentMode } from '@/gen/koda/v1/service_pb'

vi.mock('@/components/sessions/session-model-picker', () => ({
  SessionModelPicker: () => null,
}))

function renderComposer(onRun = vi.fn()) {
  render(
    <SessionComposer
      initialInput="Hello"
      inputRef={createRef<HTMLTextAreaElement>()}
      isRunning={false}
      mode={AgentMode.BUILD}
      onModeChange={vi.fn()}
      onRun={onRun}
      onStop={vi.fn()}
      runError=""
      session={{ id: 'session-1' } as Session}
    />,
  )
  return { input: screen.getByRole('textbox', { name: 'Message' }), onRun }
}

describe('SessionComposer send shortcut', () => {
  beforeEach(() => window.localStorage.clear())
  afterEach(cleanup)

  it('sends with Enter by default but not while composing text', () => {
    const { input, onRun } = renderComposer()

    fireEvent.keyDown(input, { key: 'Enter' })
    fireEvent.keyDown(input, { key: 'Enter', isComposing: true })
    fireEvent.keyDown(input, { key: 'Enter', shiftKey: true })

    expect(onRun).toHaveBeenCalledTimes(1)
    expect(onRun).toHaveBeenCalledWith('Hello')
  })

  it('keeps draft updates local until sending', () => {
    const { input, onRun } = renderComposer()

    fireEvent.change(input, { target: { value: 'Updated draft' } })

    expect(input).toHaveValue('Updated draft')
    expect(onRun).not.toHaveBeenCalled()

    fireEvent.keyDown(input, { key: 'Enter' })

    expect(onRun).toHaveBeenCalledWith('Updated draft')
    expect(input).toHaveValue('')
  })

  it('can select and persist Shift + Enter', async () => {
    const user = userEvent.setup()
    const { input, onRun } = renderComposer()

    await user.click(
      screen.getByRole('button', { name: 'Choose send shortcut' }),
    )
    await user.click(
      screen.getByRole('menuitemradio', { name: 'Shift + Enter' }),
    )

    fireEvent.keyDown(input, { key: 'Enter' })
    fireEvent.keyDown(input, { key: 'Enter', shiftKey: true })

    expect(onRun).toHaveBeenCalledTimes(1)
    expect(window.localStorage.getItem('koda-studio-send-shortcut')).toBe(
      'shift-enter',
    )
  })

  it('loads Command + Enter from local storage', () => {
    window.localStorage.setItem('koda-studio-send-shortcut', 'command-enter')
    const { input, onRun } = renderComposer()

    fireEvent.keyDown(input, { key: 'Enter' })
    fireEvent.keyDown(input, { key: 'Enter', metaKey: true })

    expect(onRun).toHaveBeenCalledTimes(1)
  })
})
