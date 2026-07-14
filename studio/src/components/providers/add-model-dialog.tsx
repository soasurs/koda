import { useMutation, useQueryClient } from '@tanstack/react-query'
import { LoaderCircle } from 'lucide-react'
import { useState } from 'react'

import { Button } from '@/components/ui/button'
import { Modal } from '@/components/ui/modal'
import type { Model, Provider } from '@/gen/koda/v1/service_pb'
import { kodaClient } from '@/lib/connect'
import { errorMessage, kodaKeys } from '@/lib/koda'

export function AddModelDialog({
  models,
  onClose,
  provider,
}: {
  models: Model[]
  onClose: () => void
  provider: Provider
}) {
  const queryClient = useQueryClient()
  const [id, setId] = useState('')
  const [name, setName] = useState('')
  const duplicate = models.some((model) => model.id === id.trim())
  const saveMutation = useMutation({
    mutationFn: () =>
      kodaClient.saveProvider({
        id: provider.id,
        name: provider.name,
        type: provider.type,
        baseUrl: provider.baseUrl,
        modelOverrides: [
          ...models,
          {
            id: id.trim(),
            name: name.trim(),
            reasoningEfforts: [],
            defaultReasoningEffort: '',
          },
        ],
        enabled: provider.enabled !== false,
      }),
    onSuccess: async () => {
      await Promise.all([
        queryClient.invalidateQueries({ queryKey: kodaKeys.providers }),
        queryClient.invalidateQueries({
          queryKey: kodaKeys.models(provider.id),
        }),
      ])
      onClose()
    },
  })

  return (
    <Modal
      description="The model ID is sent to the provider API exactly as entered."
      onClose={onClose}
      title={`Add model to ${provider.name}`}
    >
      <form
        className="space-y-4 p-5"
        onSubmit={(event) => {
          event.preventDefault()
          saveMutation.mutate()
        }}
      >
        <label className="field-label">
          Model ID
          <input
            autoFocus
            className="input"
            onChange={(event) => setId(event.target.value)}
            placeholder="model-id"
            required
            value={id}
          />
        </label>
        <label className="field-label">
          Display name
          <input
            className="input"
            onChange={(event) => setName(event.target.value)}
            placeholder="Defaults to model ID"
            value={name}
          />
        </label>

        {duplicate && (
          <p className="error-box">A model with this ID already exists.</p>
        )}
        {saveMutation.isError && (
          <p className="error-box">{errorMessage(saveMutation.error)}</p>
        )}

        <footer className="flex justify-end gap-2 pt-1">
          <Button variant="outline" onClick={onClose} type="button">
            Cancel
          </Button>
          <Button
            disabled={!id.trim() || duplicate || saveMutation.isPending}
            type="submit"
          >
            {saveMutation.isPending && (
              <LoaderCircle className="size-4 animate-spin" />
            )}
            Add model
          </Button>
        </footer>
      </form>
    </Modal>
  )
}
