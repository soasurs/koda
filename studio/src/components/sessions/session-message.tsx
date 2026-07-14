import { Bot, ChevronRight, LoaderCircle } from 'lucide-react'
import { lazy, memo, Suspense, useState } from 'react'

import type { Event } from '@/gen/koda/v1/service_pb'
import { Role } from '@/gen/koda/v1/service_pb'
import { eventText } from '@/lib/session-turns'

const MarkdownText = lazy(() => import('@/components/markdown-text'))

export const ReasoningView = memo(function ReasoningView({
  reasoning,
  streaming = false,
}: {
  reasoning?: string
  streaming?: boolean
}) {
  const [open, setOpen] = useState(streaming)

  if (!reasoning) return null
  return (
    <details
      className="group/reasoning ml-9 text-xs leading-5 text-muted-foreground"
      open={open}
    >
      <summary
        className="flex w-fit cursor-pointer list-none items-center gap-1.5 font-medium text-muted-foreground hover:text-foreground"
        onClick={(event) => {
          event.preventDefault()
          setOpen((value) => !value)
        }}
      >
        <ChevronRight className="size-3 transition-transform group-open/reasoning:rotate-90" />
        {streaming ? 'Thinking' : 'Thought'}
        {streaming && (
          <LoaderCircle className="size-3 animate-spin text-muted-foreground" />
        )}
      </summary>
      <div className="reasoning-markdown mt-1 min-w-0 border-l border-border pl-3">
        <Suspense
          fallback={<span className="whitespace-pre-wrap">{reasoning}</span>}
        >
          <MarkdownText text={reasoning} />
        </Suspense>
        {streaming && (
          <span className="ml-1 inline-block h-3 w-1 animate-pulse bg-muted-foreground align-middle" />
        )}
      </div>
    </details>
  )
})

export const EventView = memo(function EventView({ event }: { event: Event }) {
  const message = event.message
  if (!message || message.role === Role.SYSTEM) return null

  const text = eventText(event)

  if (message.role === Role.USER) {
    return (
      <div className="flex justify-end">
        <div className="max-w-[85%] rounded-xl bg-primary px-4 py-2.5 text-sm leading-6 text-primary-foreground">
          <Suspense
            fallback={<span className="whitespace-pre-wrap">{text}</span>}
          >
            <MarkdownText text={text} />
          </Suspense>
        </div>
      </div>
    )
  }

  if (message.role === Role.TOOL) return null

  return text && <AssistantText text={text} />
})

export const AssistantText = memo(function AssistantText({
  text,
  streaming = false,
}: {
  text: string
  streaming?: boolean
}) {
  return (
    <div className="flex gap-3">
      <div className="mt-0.5 flex size-6 shrink-0 items-center justify-center rounded-md border border-border bg-muted">
        <Bot className="size-3.5 text-muted-foreground" />
      </div>
      <div className="markdown min-w-0 text-sm leading-6 text-foreground">
        <Suspense
          fallback={<span className="whitespace-pre-wrap">{text}</span>}
        >
          <MarkdownText text={text} />
        </Suspense>
        {streaming && (
          <span className="ml-1 inline-block h-4 w-1 animate-pulse bg-muted-foreground align-middle" />
        )}
      </div>
    </div>
  )
})
