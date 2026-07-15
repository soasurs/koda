import { useMutation, useQueryClient } from '@tanstack/react-query'
import { LoaderCircle } from 'lucide-react'
import { useState } from 'react'

import { Button } from '@/components/ui/button'
import {
  Dialog,
  DialogClose,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import type { Session } from '@/gen/koda/v1/service_pb'
import { kodaClient } from '@/lib/connect'
import { errorMessage, kodaKeys, replaceSession } from '@/lib/koda'

export function RenameSessionDialog({
  onClose,
  session,
}: {
  onClose: () => void
  session: Session
}) {
  const queryClient = useQueryClient()
  const [title, setTitle] = useState(session.title)
  const normalizedTitle = title.trim()
  const renameMutation = useMutation({
    mutationFn: () =>
      kodaClient.updateSession({
        sessionId: session.id,
        title: normalizedTitle,
      }),
    onSuccess: async ({ session: updatedSession }) => {
      if (updatedSession) {
        queryClient.setQueryData(kodaKeys.session(session.id), updatedSession)
        queryClient.setQueryData<Session[]>(kodaKeys.sessions, (sessions) =>
          replaceSession(sessions, updatedSession),
        )
      } else {
        await queryClient.invalidateQueries({ queryKey: kodaKeys.sessions })
        await queryClient.invalidateQueries({
          queryKey: kodaKeys.session(session.id),
        })
      }
      onClose()
    },
  })

  return (
    <Dialog
      open
      onOpenChange={(open) => {
        if (!open) onClose()
      }}
    >
      <DialogContent>
        <DialogHeader>
          <DialogTitle>Rename session</DialogTitle>
          <DialogDescription>
            Choose a name that makes this session easy to find.
          </DialogDescription>
        </DialogHeader>
        <form
          className="grid gap-4"
          onSubmit={(event) => {
            event.preventDefault()
            renameMutation.mutate()
          }}
        >
          <div className="grid gap-2">
            <Label htmlFor="session-title">Name</Label>
            <Input
              autoFocus
              id="session-title"
              onChange={(event) => setTitle(event.target.value)}
              placeholder="Session name"
              value={title}
            />
          </div>
          {renameMutation.isError && (
            <p className="text-sm text-destructive">
              {errorMessage(renameMutation.error)}
            </p>
          )}
          <DialogFooter>
            <DialogClose asChild>
              <Button type="button" variant="outline">
                Cancel
              </Button>
            </DialogClose>
            <Button
              disabled={
                renameMutation.isPending ||
                !normalizedTitle ||
                normalizedTitle === session.title.trim()
              }
              type="submit"
            >
              {renameMutation.isPending && (
                <LoaderCircle className="animate-spin" aria-hidden="true" />
              )}
              Rename
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  )
}
