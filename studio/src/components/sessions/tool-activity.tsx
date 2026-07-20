import { Check, ChevronRight, LoaderCircle, Wrench, X } from 'lucide-react'
import { memo, useState } from 'react'

import { useI18n } from '@/app/i18n'
import { usePreferences } from '@/app/preferences-context-value'
import type { Event, FileChange } from '@/gen/koda/v1/service_pb'
import { DiffLineKind } from '@/gen/koda/v1/service_pb'
import { parseShellOutput, type ShellOutput } from '@/lib/shell-output'
import { toolCallPresentation } from '@/lib/session-turns'

export const ToolGroup = memo(function ToolGroup({
  assistant,
  toolEvents,
}: {
  assistant: Event
  toolEvents: Event[]
}) {
  const { t } = useI18n()
  const { expandToolCalls } = usePreferences()
  const message = assistant.message
  const responses = new Map(
    toolEvents.flatMap((event) => {
      const response = event.message?.toolResponse
      return response ? [[response.toolCallId, response] as const] : []
    }),
  )
  const hasPending =
    message?.toolCalls.some((toolCall) => !responses.has(toolCall.id)) ?? false
  const [open, setOpen] = useState(hasPending || expandToolCalls)
  const [userToggled, setUserToggled] = useState(false)

  if (!message) return null
  const toolCount = message.toolCalls.length || toolEvents.length
  if (toolCount === 0) return null

  const effectiveOpen = userToggled ? open : hasPending || expandToolCalls
  return (
    <details
      className="group ml-9 rounded-lg border border-border bg-muted/30"
      open={effectiveOpen}
    >
      <summary
        className="flex cursor-pointer list-none items-center gap-2 px-3 py-2 text-xs text-muted-foreground hover:text-foreground"
        onClick={(event) => {
          event.preventDefault()
          setUserToggled(true)
          setOpen((value) => !value)
        }}
      >
        <ChevronRight className="size-3.5 transition-transform group-open:rotate-90" />
        <Wrench className="size-3.5" />
        {toolCount > 0
          ? toolCount === 1
            ? t('session.tool.steps.one', { count: toolCount })
            : t('session.tool.steps.other', { count: toolCount })
          : t('session.tool.tools')}
      </summary>
      <div className="divide-y divide-border border-t border-border px-3">
        {message.toolCalls.map((toolCall) => (
          <ToolCallRow
            key={toolCall.id}
            response={responses.get(toolCall.id)}
            toolCall={toolCall}
          />
        ))}
      </div>
    </details>
  )
})

const ToolCallRow = memo(function ToolCallRow({
  response,
  toolCall,
}: {
  response?: NonNullable<Event['message']>['toolResponse']
  toolCall: NonNullable<Event['message']>['toolCalls'][number]
}) {
  const { t } = useI18n()
  const finished = Boolean(response)
  const { label, detail } = toolCallPresentation(toolCall, finished)
  const failed = response?.outcome.case === 'error'
  const status = failed
    ? t('session.tool.status.failed')
    : finished
      ? t('session.tool.status.completed')
      : t('session.tool.status.running')
  const fileChanges =
    response?.outcome.case === 'result'
      ? response.outcome.value.fileChanges
      : []
  const shellOutput =
    toolCall.name === 'run_shell' && response?.outcome.case === 'result'
      ? parseShellOutput(
          response.outcome.value.structuredContentJson,
          response.outcome.value.content,
        )
      : null
  const expandable = fileChanges.length > 0 || shellOutput !== null

  const content = (
    <>
      <Wrench className="size-3.5 shrink-0 text-muted-foreground" />
      <span className="shrink-0 font-medium text-foreground">{label}</span>
      {detail && (
        <span
          className="truncate font-mono text-[11px] text-muted-foreground"
          title={detail}
        >
          {detail}
        </span>
      )}
      <span
        className={`ml-auto flex shrink-0 items-center gap-1.5 ${
          failed
            ? 'text-red-500'
            : response
              ? 'text-emerald-600'
              : 'text-muted-foreground'
        }`}
      >
        {response ? (
          failed ? (
            <X className="size-3.5" />
          ) : (
            <Check className="size-3.5" />
          )
        ) : (
          <LoaderCircle className="size-3.5 animate-spin" />
        )}
        {status}
      </span>
      {expandable && (
        <ChevronRight className="size-3.5 shrink-0 text-muted-foreground transition-transform group-open/tool:rotate-90" />
      )}
    </>
  )

  if (!expandable) {
    return (
      <div className="flex min-w-0 items-center gap-2 py-2.5 text-xs">
        {content}
      </div>
    )
  }

  return (
    <details className="group/tool text-xs">
      <summary className="flex min-w-0 cursor-pointer list-none items-center gap-2 py-2.5">
        {content}
      </summary>
      {fileChanges.length > 0 && <FileChangesView changes={fileChanges} />}
      {shellOutput && <ShellOutputView output={shellOutput} />}
    </details>
  )
})

function ShellOutputView({ output }: { output: ShellOutput }) {
  const { t } = useI18n()
  return (
    <div className="mb-3 overflow-hidden rounded-md border border-border bg-background font-mono text-[11px]">
      <div className="flex items-center justify-between border-b border-border px-3 py-2 text-muted-foreground">
        <span>{t('session.tool.output.title')}</span>
        <span>
          {t('session.tool.output.exit', { code: output.exitCode })}
          {output.truncated ? t('session.tool.output.truncated') : ''}
        </span>
      </div>
      {output.stdout || output.stderr ? (
        <div className="max-h-72 overflow-auto p-3 leading-5">
          {output.stdout && (
            <pre className="whitespace-pre-wrap text-foreground">
              {output.stdout}
            </pre>
          )}
          {output.stderr && (
            <pre className="whitespace-pre-wrap text-red-400">
              {output.stderr}
            </pre>
          )}
        </div>
      ) : (
        <p className="px-3 py-2 text-muted-foreground">
          {t('session.tool.output.empty')}
        </p>
      )}
    </div>
  )
}

export function FileChangesView({ changes }: { changes: FileChange[] }) {
  const { t } = useI18n()
  return (
    <div className="mb-3 space-y-3 overflow-hidden rounded-md border border-border bg-background">
      {changes.map((change, changeIndex) => (
        <div key={`${change.path}-${changeIndex}`}>
          <div className="flex items-center justify-between border-b border-border px-3 py-2 font-mono text-[11px] text-muted-foreground">
            <span className="truncate">{change.path}</span>
            {change.truncated && (
              <span className="shrink-0 text-muted-foreground">
                {t('session.tool.diff.truncated')}
              </span>
            )}
          </div>
          <div className="overflow-x-auto py-1 font-mono text-[11px] leading-5">
            {change.hunks.map((hunk, hunkIndex) => (
              <div key={`${hunk.oldStart}-${hunk.newStart}-${hunkIndex}`}>
                <div className="diff-hunk">
                  @@ -{hunk.oldStart} +{hunk.newStart} @@
                </div>
                {hunk.lines.map((line, lineIndex) => {
                  const added = line.kind === DiffLineKind.ADDED
                  const removed = line.kind === DiffLineKind.REMOVED
                  return (
                    <div
                      className={`diff-line ${
                        added
                          ? 'diff-line-added'
                          : removed
                            ? 'diff-line-removed'
                            : ''
                      }`}
                      key={`${line.oldLine}-${line.newLine}-${lineIndex}`}
                    >
                      <span className="diff-line-number">
                        {line.oldLine || ''}
                      </span>
                      <span className="diff-line-number">
                        {line.newLine || ''}
                      </span>
                      <span className="diff-line-code">
                        <span className="diff-marker">
                          {added ? '+' : removed ? '-' : ' '}
                        </span>
                        {line.content}
                      </span>
                    </div>
                  )
                })}
              </div>
            ))}
          </div>
        </div>
      ))}
    </div>
  )
}
