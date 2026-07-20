import { useMutation } from '@tanstack/react-query'
import { Check, Folder, ShieldAlert, User, Wrench, X } from 'lucide-react'
import { memo, useState } from 'react'

import { useI18n } from '@/app/i18n'
import { Button } from '@/components/ui/button'
import { FileChangesView } from '@/components/sessions/tool-activity'
import type {
  QuestionAnswer,
  QuestionPrompt,
  ToolApproval,
} from '@/gen/koda/v1/service_pb'
import { kodaClient } from '@/lib/connect'
import { errorMessage } from '@/lib/koda'
import { toolPresentation } from '@/lib/session-turns'

export const ApprovalCard = memo(function ApprovalCard({
  approval,
  onResolved,
}: {
  approval: ToolApproval
  onResolved: () => void
}) {
  const { t } = useI18n()
  const tool = toolPresentation(approval.toolName, approval.argumentsJson)
  const location =
    approval.toolName === 'run_shell' ? approval.targetPaths[0] : ''
  const mutation = useMutation({
    mutationFn: (approved: boolean) =>
      kodaClient.resolveToolApproval({ approvalId: approval.id, approved }),
    onSuccess: onResolved,
  })

  return (
    <div className="mt-6 rounded-xl border border-amber-300 bg-amber-50 p-4 dark:border-amber-900/70 dark:bg-amber-950/20">
      <div className="flex gap-3">
        <ShieldAlert className="mt-0.5 size-5 shrink-0 text-amber-600 dark:text-amber-500" />
        <div className="min-w-0 flex-1">
          <h3 className="text-sm font-medium text-amber-800 dark:text-amber-200">
            {t('runPrompt.permissionRequired')}
          </h3>
          <p className="mt-1 text-sm leading-6 text-muted-foreground">
            {t('runPrompt.permissionBody')}
          </p>
          <div className="mt-3 overflow-hidden rounded-lg border border-amber-200/80 bg-white/60 dark:border-amber-900/60 dark:bg-background/50">
            <div className="flex items-center gap-2 border-b border-amber-200/70 px-3 py-2.5 text-sm dark:border-amber-900/50">
              <Wrench className="size-4 shrink-0 text-amber-600 dark:text-amber-500" />
              <span className="font-medium text-foreground">{tool.label}</span>
            </div>
            <div className="space-y-2 px-3 py-2.5">
              {tool.detail ? (
                <pre className="overflow-x-auto whitespace-pre-wrap wrap-break-word font-mono text-xs leading-5 text-muted-foreground">
                  {tool.detail}
                </pre>
              ) : (
                <p className="text-xs leading-5 text-muted-foreground">
                  {approval.summary}
                </p>
              )}
              {location && (
                <p className="flex min-w-0 items-center gap-1.5 text-[11px] text-muted-foreground">
                  <Folder className="size-3 shrink-0" />
                  <span className="truncate" title={location}>
                    {location}
                  </span>
                </p>
              )}
            </div>
          </div>
          {approval.fileChanges.length > 0 && (
            <details className="mt-3 text-xs">
              <summary className="cursor-pointer text-muted-foreground">
                {t('runPrompt.reviewChanges')}
              </summary>
              <div className="mt-2">
                <FileChangesView changes={approval.fileChanges} />
              </div>
            </details>
          )}
          {mutation.isError && (
            <p className="mt-2 text-xs text-red-400">
              {errorMessage(mutation.error)}
            </p>
          )}
          <div className="mt-4 flex gap-2">
            <Button
              disabled={mutation.isPending}
              onClick={() => mutation.mutate(true)}
            >
              <Check className="size-4" /> {t('runPrompt.approve')}
            </Button>
            <Button
              disabled={mutation.isPending}
              onClick={() => mutation.mutate(false)}
              variant="outline"
            >
              <X className="size-4" /> {t('runPrompt.reject')}
            </Button>
          </div>
        </div>
      </div>
    </div>
  )
})

export const QuestionCard = memo(function QuestionCard({
  onResolved,
  prompt,
}: {
  onResolved: () => void
  prompt: QuestionPrompt
}) {
  const { t } = useI18n()
  const [answers, setAnswers] = useState<Record<string, QuestionAnswer>>({})
  const mutation = useMutation({
    mutationFn: (canceled: boolean) =>
      kodaClient.submitQuestionAnswers(
        canceled
          ? {
              promptId: prompt.id,
              resolution: { case: 'canceled', value: true },
            }
          : {
              promptId: prompt.id,
              resolution: {
                case: 'answers',
                value: {
                  answers: prompt.questions.map(
                    (question) =>
                      answers[question.id] ?? {
                        questionId: question.id,
                        selectedOptionIds: [],
                        freeform: '',
                      },
                  ),
                },
              },
            },
      ),
    onSuccess: onResolved,
  })
  const canSubmit = prompt.questions.every((question) => {
    const answer = answers[question.id]
    return Boolean(answer?.selectedOptionIds.length || answer?.freeform.trim())
  })

  return (
    <div className="mt-6 rounded-xl border border-blue-200 bg-blue-50 p-4 dark:border-blue-900/60 dark:bg-blue-950/20">
      <div className="flex gap-3">
        <User className="mt-0.5 size-5 shrink-0 text-blue-600 dark:text-blue-400" />
        <div className="min-w-0 flex-1 space-y-5">
          {prompt.questions.map((question) => (
            <fieldset key={question.id}>
              <legend className="text-sm font-medium text-foreground">
                {question.header}
              </legend>
              <p className="mt-1 text-sm text-muted-foreground">
                {question.prompt}
              </p>
              <div className="mt-3 space-y-2">
                {question.options.map((option) => {
                  const selected = answers[
                    question.id
                  ]?.selectedOptionIds.includes(option.id)
                  return (
                    <label
                      className="flex cursor-pointer gap-3 rounded-lg border border-border p-3 hover:bg-accent"
                      key={option.id}
                    >
                      <input
                        checked={selected ?? false}
                        name={question.id}
                        onChange={() => {
                          const current = answers[question.id]
                          const selectedOptionIds = question.multiple
                            ? selected
                              ? (current?.selectedOptionIds ?? []).filter(
                                  (id) => id !== option.id,
                                )
                              : [
                                  ...(current?.selectedOptionIds ?? []),
                                  option.id,
                                ]
                            : [option.id]
                          setAnswers((values) => ({
                            ...values,
                            [question.id]: {
                              questionId: question.id,
                              selectedOptionIds,
                              freeform: current?.freeform ?? '',
                            } as QuestionAnswer,
                          }))
                        }}
                        type={question.multiple ? 'checkbox' : 'radio'}
                      />
                      <span>
                        <span className="block text-sm text-foreground">
                          {option.label}
                        </span>
                        <span className="mt-0.5 block text-xs text-muted-foreground">
                          {option.description}
                        </span>
                      </span>
                    </label>
                  )
                })}
                {question.allowFreeform && (
                  <input
                    className="input"
                    onChange={(event) =>
                      setAnswers((values) => ({
                        ...values,
                        [question.id]: {
                          questionId: question.id,
                          selectedOptionIds:
                            values[question.id]?.selectedOptionIds ?? [],
                          freeform: event.target.value,
                        } as QuestionAnswer,
                      }))
                    }
                    placeholder={t('runPrompt.freeformPlaceholder')}
                    value={answers[question.id]?.freeform ?? ''}
                  />
                )}
              </div>
            </fieldset>
          ))}
          {mutation.isError && (
            <p className="text-xs text-red-400">
              {errorMessage(mutation.error)}
            </p>
          )}
          <div className="flex gap-2">
            <Button
              disabled={mutation.isPending || !canSubmit}
              onClick={() => mutation.mutate(false)}
            >
              {t('runPrompt.submitAnswers')}
            </Button>
            <Button
              disabled={mutation.isPending}
              onClick={() => mutation.mutate(true)}
              variant="outline"
            >
              {t('runPrompt.cancel')}
            </Button>
          </div>
        </div>
      </div>
    </div>
  )
})
