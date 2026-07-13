import { ArrowRight, MessageSquareText } from 'lucide-react'
import { Link } from '@tanstack/react-router'

export function HomePage() {
  return (
    <section className="flex h-full items-center justify-center px-6 py-12">
      <div className="max-w-sm text-center">
        <div className="mx-auto mb-4 flex size-11 items-center justify-center rounded-xl border border-neutral-800 bg-neutral-900">
          <MessageSquareText
            className="size-5 text-neutral-400"
            aria-hidden="true"
          />
        </div>
        <h1 className="text-base font-medium">Start with a session</h1>
        <p className="mt-2 text-sm leading-6 text-neutral-500">
          Create a session from the sidebar. Sessions are organized by their
          local project.
        </p>
        <Link
          className="mt-4 inline-flex items-center gap-1 text-sm text-neutral-300 hover:text-neutral-100"
          to="/settings/providers"
        >
          Configure providers
          <ArrowRight className="size-3.5" />
        </Link>
      </div>
    </section>
  )
}
