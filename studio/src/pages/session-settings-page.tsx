import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { Link } from '@tanstack/react-router'
import { ArchiveRestore, LoaderCircle, MessageSquareText } from 'lucide-react'

import { useI18n } from '@/app/i18n'
import { SettingsLayout } from '@/components/settings/settings-layout'
import { Button } from '@/components/ui/button'
import type { Session } from '@/gen/koda/v1/service_pb'
import { kodaClient } from '@/lib/connect'
import { errorMessage, kodaKeys, listSessions } from '@/lib/koda'

export function SessionSettingsPage() {
  const { t } = useI18n()
  const queryClient = useQueryClient()
  const sessionsQuery = useQuery({
    queryKey: kodaKeys.archivedSessions,
    queryFn: () => listSessions(true),
  })
  const restoreMutation = useMutation({
    mutationFn: (sessionId: string) =>
      kodaClient.updateSession({ sessionId, archived: false }),
    onSuccess: async ({ session }, sessionId) => {
      queryClient.setQueryData<Session[]>(
        kodaKeys.archivedSessions,
        (sessions) => sessions?.filter((current) => current.id !== sessionId),
      )
      if (session) {
        queryClient.setQueryData(kodaKeys.session(sessionId), session)
      }
      await queryClient.invalidateQueries({ queryKey: kodaKeys.sessions })
    },
  })

  return (
    <SettingsLayout active="sessions">
      <div>
        <h2 className="text-lg font-semibold tracking-tight">
          {t('settings.sessions.title')}
        </h2>
        <p className="mt-1 max-w-xl text-sm leading-6 text-muted-foreground">
          {t('settings.sessions.description')}
        </p>
      </div>

      {restoreMutation.isError && (
        <p className="error-box mt-6">{errorMessage(restoreMutation.error)}</p>
      )}

      {sessionsQuery.isPending ? (
        <div className="flex h-56 items-center justify-center">
          <LoaderCircle className="size-5 animate-spin text-muted-foreground" />
        </div>
      ) : sessionsQuery.isError ? (
        <p className="error-box mt-6">{errorMessage(sessionsQuery.error)}</p>
      ) : sessionsQuery.data.length === 0 ? (
        <div className="mt-6 rounded-lg border border-dashed border-border px-6 py-12 text-center">
          <MessageSquareText className="mx-auto size-6 text-muted-foreground" />
          <p className="mt-3 text-sm font-medium text-foreground">
            {t('settings.sessions.empty.title')}
          </p>
          <p className="mt-1 text-sm text-muted-foreground">
            {t('settings.sessions.empty.body')}
          </p>
        </div>
      ) : (
        <div className="mt-6 grid gap-3">
          {sessionsQuery.data.map((session) => (
            <div
              className="flex items-center gap-4 rounded-lg border border-border bg-background p-4"
              key={session.id}
            >
              <div className="min-w-0 flex-1">
                <Link
                  className="block truncate text-sm font-medium text-foreground hover:underline"
                  params={{ sessionId: session.id }}
                  to="/sessions/$sessionId"
                >
                  {session.title || t('session.untitled')}
                </Link>
                <p className="mt-1 truncate text-xs text-muted-foreground">
                  {session.workdir}
                </p>
              </div>
              <Button
                disabled={
                  restoreMutation.isPending &&
                  restoreMutation.variables === session.id
                }
                onClick={() => restoreMutation.mutate(session.id)}
                size="sm"
                variant="outline"
              >
                {restoreMutation.isPending &&
                restoreMutation.variables === session.id ? (
                  <LoaderCircle className="animate-spin" aria-hidden="true" />
                ) : (
                  <ArchiveRestore aria-hidden="true" />
                )}
                {t('settings.sessions.restore')}
              </Button>
            </div>
          ))}
        </div>
      )}
    </SettingsLayout>
  )
}
