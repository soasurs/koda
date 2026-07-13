import { Bot, ChevronRight, LoaderCircle } from 'lucide-react'
import { lazy, Suspense, useState } from 'react'

import type { Event } from '@/gen/koda/v1/service_pb'
import { Role } from '@/gen/koda/v1/service_pb'
import { eventText } from '@/lib/session-turns'

const MarkdownText = lazy(() => import('@/components/markdown-text'))

export function ReasoningView({
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
      className="group/reasoning ml-9 text-xs leading-5 text-neutral-500"
      open={open}
    >
      <summary
        className="flex w-fit cursor-pointer list-none items-center gap-1.5 font-medium text-neutral-600 hover:text-neutral-400"
        onClick={(event) => {
          event.preventDefault()
          setOpen((value) => !value)
        }}
      >
        <ChevronRight className="size-3 transition-transform group-open/reasoning:rotate-90" />
        Reasoning
        {streaming && (
          <LoaderCircle className="size-3 animate-spin text-neutral-500" />
        )}
      </summary>
      <div className="reasoning-markdown mt-1 min-w-0 border-l border-neutral-800 pl-3">
        <Suspense
          fallback={<span className="whitespace-pre-wrap">{reasoning}</span>}
        >
          <MarkdownText text={reasoning} />
        </Suspense>
        {streaming && (
          <span className="ml-1 inline-block h-3 w-1 animate-pulse bg-neutral-500 align-middle" />
        )}
      </div>
    </details>
  )
}

export function EventView({ event }: { event: Event }) {
  const message = event.message
  if (!message || message.role === Role.SYSTEM) return null

  const text = eventText(event)

  if (message.role === Role.USER) {
    return (
      <div className="flex justify-end">
        <div className="max-w-[85%] rounded-xl bg-neutral-100 px-4 py-2.5 text-sm leading-6 text-neutral-950">
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
}

export function AssistantText({
  text,
  streaming = false,
}: {
  text: string
  streaming?: boolean
}) {
  return (
    <div className="flex gap-3">
      <div className="mt-0.5 flex size-6 shrink-0 items-center justify-center rounded-md border border-neutral-800 bg-neutral-900">
        <Bot className="size-3.5 text-neutral-400" />
      </div>
      <div className="markdown min-w-0 text-sm leading-6 text-neutral-300">
        <Suspense
          fallback={<span className="whitespace-pre-wrap">{text}</span>}
        >
          <MarkdownText text={text} />
        </Suspense>
        {streaming && (
          <span className="ml-1 inline-block h-4 w-1 animate-pulse bg-neutral-500 align-middle" />
        )}
      </div>
    </div>
  )
}
