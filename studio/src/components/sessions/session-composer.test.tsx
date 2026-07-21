import {
  cleanup,
  fireEvent,
  render,
  screen,
  waitFor,
} from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { createRef } from 'react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

import { SessionComposer } from '@/components/sessions/session-composer'
import type { Session } from '@/gen/koda/v1/service_pb'
import { AgentMode } from '@/gen/koda/v1/service_pb'
import type { ComposerInput } from '@/lib/composer-attachments'
import { AllProviders } from '@/test/providers'

vi.mock('@/components/sessions/session-model-picker', () => ({
  SessionModelPicker: () => null,
}))

function renderComposer(onRun = vi.fn()) {
  render(
    <AllProviders>
      <SessionComposer
        initialInput={{ text: 'Hello', attachments: [] }}
        inputRef={createRef<HTMLTextAreaElement>()}
        isRunning={false}
        mode={AgentMode.BUILD}
        onModeChange={vi.fn()}
        onRun={onRun}
        onStop={vi.fn()}
        runError=""
        session={{ id: 'session-1' } as Session}
      />
    </AllProviders>,
  )
  return { input: screen.getByRole('textbox', { name: 'Message' }), onRun }
}

function readStoredShortcut(): string | null {
  const raw = window.localStorage.getItem('koda-studio-preferences')
  if (!raw) return null
  try {
    const parsed = JSON.parse(raw) as { sendShortcut?: string }
    return parsed.sendShortcut ?? null
  } catch {
    return null
  }
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
    expect(onRun).toHaveBeenCalledWith({
      text: 'Hello',
      attachments: [],
    })
  })

  it('keeps draft updates local until sending', () => {
    const { input, onRun } = renderComposer()

    fireEvent.change(input, { target: { value: 'Updated draft' } })

    expect(input).toHaveValue('Updated draft')
    expect(onRun).not.toHaveBeenCalled()

    fireEvent.keyDown(input, { key: 'Enter' })

    expect(onRun).toHaveBeenCalledWith({
      text: 'Updated draft',
      attachments: [],
    })
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
    expect(readStoredShortcut()).toBe('shift-enter')
  })

  it('migrates the legacy send-shortcut storage key on first load', () => {
    window.localStorage.setItem('koda-studio-send-shortcut', 'command-enter')
    const { input, onRun } = renderComposer()

    fireEvent.keyDown(input, { key: 'Enter' })
    fireEvent.keyDown(input, { key: 'Enter', metaKey: true })

    expect(onRun).toHaveBeenCalledTimes(1)
    expect(readStoredShortcut()).toBe('command-enter')
    expect(window.localStorage.getItem('koda-studio-send-shortcut')).toBeNull()
  })
})

describe('SessionComposer attachments', () => {
  beforeEach(() => window.localStorage.clear())
  afterEach(cleanup)

  function makeImageFile(name = 'photo.png', type = 'image/png'): File {
    return new File([new Uint8Array([1, 2, 3, 4])], name, { type })
  }

  function getFileInput(): HTMLInputElement {
    return document.querySelector('input[type="file"]') as HTMLInputElement
  }

  async function addFileViaInput(file: File) {
    const input = getFileInput()
    fireEvent.change(input, { target: { files: [file] } })
  }

  it('accepts image files added via the attach button and sends them', async () => {
    const onRun = vi.fn()
    const { input } = renderComposer(onRun)

    fireEvent.change(input, { target: { value: 'look at this' } })
    await addFileViaInput(makeImageFile())

    await waitFor(() => expect(screen.getAllByRole('img')).toHaveLength(1))

    fireEvent.keyDown(input, { key: 'Enter' })

    expect(onRun).toHaveBeenCalledTimes(1)
    const payload = onRun.mock.calls[0]?.[0] as ComposerInput
    expect(payload.text).toBe('look at this')
    expect(payload.attachments).toHaveLength(1)
    expect(payload.attachments[0]?.mimeType).toBe('image/png')
    expect(input).toHaveValue('')
  })

  it('opens an attachment preview when its image is clicked', async () => {
    const onRun = vi.fn()
    renderComposer(onRun)

    await addFileViaInput(makeImageFile())
    await waitFor(() => expect(screen.getAllByRole('img')).toHaveLength(1))

    fireEvent.click(
      screen.getByRole('button', { name: 'View photo.png enlarged' }),
    )

    expect(screen.getByRole('dialog')).toBeInTheDocument()
    expect(screen.getByRole('dialog').querySelector('img')).toBeInTheDocument()
  })

  it('removes an attachment when its remove button is clicked', async () => {
    const onRun = vi.fn()
    renderComposer(onRun)

    await addFileViaInput(makeImageFile())
    await waitFor(() => expect(screen.getAllByRole('img')).toHaveLength(1))

    fireEvent.click(screen.getByRole('button', { name: 'Remove attachment' }))
    await waitFor(() => expect(screen.queryAllByRole('img')).toHaveLength(0))
  })

  it('reports an error for non-image files', async () => {
    const onRun = vi.fn()
    renderComposer(onRun)

    await addFileViaInput(
      new File([new Uint8Array([1])], 'doc.pdf', {
        type: 'application/pdf',
      }),
    )

    await waitFor(() =>
      expect(screen.getByText(/Only images can be attached/)).toBeTruthy(),
    )
    expect(screen.queryAllByRole('img')).toHaveLength(0)
  })

  it('allows sending with only attachments and no text', async () => {
    const onRun = vi.fn()
    const { input } = renderComposer(onRun)

    fireEvent.change(input, { target: { value: '' } })
    await addFileViaInput(makeImageFile())
    await waitFor(() => expect(screen.getAllByRole('img')).toHaveLength(1))

    const sendButton = screen.getByRole('button', { name: 'Send' })
    expect(sendButton).not.toBeDisabled()
    fireEvent.click(sendButton)

    expect(onRun).toHaveBeenCalledTimes(1)
    const payload = onRun.mock.calls[0]?.[0] as ComposerInput
    expect(payload.text).toBe('')
    expect(payload.attachments).toHaveLength(1)
  })

  it('accepts pasted image files', async () => {
    const onRun = vi.fn()
    const { input } = renderComposer(onRun)

    const file = makeImageFile()
    const pasteEvent = new Event('paste', { bubbles: true })
    Object.defineProperty(pasteEvent, 'clipboardData', {
      value: {
        types: ['Files'],
        items: [{ kind: 'file', type: 'image/png', getAsFile: () => file }],
        files: [file],
        getData: () => '',
      },
    })
    fireEvent(input, pasteEvent)

    await waitFor(() => expect(screen.getAllByRole('img')).toHaveLength(1))
  })
})
