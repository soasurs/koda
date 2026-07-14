import {
  ChevronUp,
  CircleStop,
  ClipboardList,
  Hammer,
  Send,
} from 'lucide-react'
import { memo, useState, type RefObject } from 'react'

import { SessionModelPicker } from '@/components/sessions/session-model-picker'
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuLabel,
  DropdownMenuRadioGroup,
  DropdownMenuRadioItem,
  DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import type { Session } from '@/gen/koda/v1/service_pb'
import { AgentMode } from '@/gen/koda/v1/service_pb'

type SendShortcut = 'enter' | 'shift-enter' | 'command-enter'

const sendShortcutStorageKey = 'koda-studio-send-shortcut'
const sendShortcutOptions: {
  label: string
  shortcut: SendShortcut
}[] = [
  { label: 'Enter', shortcut: 'enter' },
  { label: 'Shift + Enter', shortcut: 'shift-enter' },
  { label: 'Command + Enter', shortcut: 'command-enter' },
]

function loadSendShortcut(): SendShortcut {
  const stored = window.localStorage.getItem(sendShortcutStorageKey)
  return stored === 'shift-enter' || stored === 'command-enter'
    ? stored
    : 'enter'
}

function matchesSendShortcut(
  event: React.KeyboardEvent<HTMLTextAreaElement>,
  shortcut: SendShortcut,
) {
  if (event.key !== 'Enter' || event.nativeEvent.isComposing) return false

  switch (shortcut) {
    case 'shift-enter':
      return event.shiftKey && !event.metaKey && !event.ctrlKey && !event.altKey
    case 'command-enter':
      return event.metaKey && !event.shiftKey && !event.ctrlKey && !event.altKey
    default:
      return (
        !event.shiftKey && !event.metaKey && !event.ctrlKey && !event.altKey
      )
  }
}

export const SessionComposer = memo(function SessionComposer({
  initialInput,
  inputRef,
  isRunning,
  mode,
  onModeChange,
  onRun,
  onStop,
  runError,
  session,
}: {
  initialInput: string
  inputRef: RefObject<HTMLTextAreaElement | null>
  isRunning: boolean
  mode: AgentMode
  onModeChange: (mode: AgentMode) => void
  onRun: (input: string) => void
  onStop: () => void
  runError: string
  session: Session
}) {
  const [input, setInput] = useState(initialInput)
  const [sendShortcut, setSendShortcut] =
    useState<SendShortcut>(loadSendShortcut)
  const sendShortcutLabel = sendShortcutOptions.find(
    (option) => option.shortcut === sendShortcut,
  )!.label

  function selectSendShortcut(shortcut: SendShortcut) {
    setSendShortcut(shortcut)
    window.localStorage.setItem(sendShortcutStorageKey, shortcut)
    requestAnimationFrame(() => inputRef.current?.focus())
  }

  function submit() {
    if (!input.trim() || isRunning) return
    setInput('')
    onRun(input)
  }

  return (
    <footer className="shrink-0 bg-linear-to-t from-background via-background to-transparent px-4 pb-4 pt-2 sm:px-6">
      <div className="mx-auto max-w-4xl">
        {runError && <p className="error-box mb-3">{runError}</p>}
        <div className="rounded-xl border border-border bg-card shadow-xl focus-within:border-ring">
          <textarea
            ref={inputRef}
            aria-label="Message"
            className="max-h-48 min-h-20 w-full resize-none bg-transparent px-4 py-3 text-sm leading-6 text-foreground outline-none placeholder:text-muted-foreground"
            disabled={isRunning}
            onChange={(event) => setInput(event.target.value)}
            onKeyDown={(event) => {
              if (matchesSendShortcut(event, sendShortcut)) {
                event.preventDefault()
                submit()
              }
            }}
            placeholder="Ask Koda to work in this directory…"
            value={input}
          />
          <div className="flex items-center justify-between px-2.5 pb-2.5">
            <div className="relative">
              <Select
                disabled={isRunning}
                onValueChange={(value) =>
                  onModeChange(Number(value) as AgentMode)
                }
                value={String(mode)}
              >
                <SelectTrigger className="inline-flex h-auto w-auto items-center gap-1 whitespace-nowrap rounded-md border border-border bg-background py-1.5 pl-3 pr-7 text-xs font-medium text-foreground hover:border-border/80 [&>svg]:hidden">
                  <SelectValue />
                </SelectTrigger>
                <ChevronUp className="pointer-events-none absolute right-2 top-1/2 size-3 -translate-y-1/2 text-muted-foreground" />
                <SelectContent side="top">
                  <SelectItem value={String(AgentMode.BUILD)}>
                    <span className="flex items-center gap-2">
                      <Hammer className="size-4 shrink-0" />
                      Build
                    </span>
                  </SelectItem>
                  <SelectItem value={String(AgentMode.PLAN)}>
                    <span className="flex items-center gap-2">
                      <ClipboardList className="size-4 shrink-0" />
                      Plan
                    </span>
                  </SelectItem>
                </SelectContent>
              </Select>
            </div>
            <div className="flex items-center gap-1.5">
              <SessionModelPicker
                disabled={isRunning}
                key={session.id}
                session={session}
              />
              {isRunning ? (
                <button
                  aria-label="Stop"
                  className="flex size-8 items-center justify-center rounded-md bg-primary text-primary-foreground hover:bg-primary/90"
                  onClick={onStop}
                  type="button"
                >
                  <CircleStop className="size-4" />
                </button>
              ) : (
                <div className="flex overflow-hidden rounded-md bg-primary text-primary-foreground">
                  <button
                    aria-label="Send"
                    className="flex h-8 w-8 items-center justify-center bg-transparent hover:bg-primary/90"
                    disabled={!input.trim()}
                    onClick={submit}
                    title={`Send (${sendShortcutLabel})`}
                    type="button"
                  >
                    <Send
                      className={`size-4 ${input.trim() ? '' : 'opacity-50'}`}
                    />
                  </button>
                  <DropdownMenu
                    onOpenChange={(open) => {
                      if (!open)
                        requestAnimationFrame(() => inputRef.current?.focus())
                    }}
                  >
                    <DropdownMenuTrigger asChild>
                      <button
                        aria-label="Choose send shortcut"
                        className="flex h-8 w-5 items-center justify-center border-l border-primary-foreground/30 bg-transparent text-primary-foreground/70 hover:bg-primary/90 hover:text-primary-foreground"
                        title={`Send shortcut: ${sendShortcutLabel}`}
                        type="button"
                      >
                        <ChevronUp className="size-3" />
                      </button>
                    </DropdownMenuTrigger>
                    <DropdownMenuContent align="end" side="top" sideOffset={8}>
                      <DropdownMenuLabel>Send message with</DropdownMenuLabel>
                      <DropdownMenuRadioGroup
                        onValueChange={(value) =>
                          selectSendShortcut(value as SendShortcut)
                        }
                        value={sendShortcut}
                      >
                        {sendShortcutOptions.map((option) => (
                          <DropdownMenuRadioItem
                            key={option.shortcut}
                            value={option.shortcut}
                          >
                            {option.label}
                          </DropdownMenuRadioItem>
                        ))}
                      </DropdownMenuRadioGroup>
                    </DropdownMenuContent>
                  </DropdownMenu>
                </div>
              )}
            </div>
          </div>
        </div>
        <p className="mt-2 text-center text-[11px] text-muted-foreground">
          Koda can make mistakes. Review commands and file changes.
        </p>
      </div>
    </footer>
  )
})
