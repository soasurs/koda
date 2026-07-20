import { useMutation, useQueryClient } from '@tanstack/react-query'
import { LoaderCircle } from 'lucide-react'
import { useState } from 'react'

import { useI18n } from '@/app/i18n'
import { Button } from '@/components/ui/button'
import { Modal } from '@/components/ui/modal'
import type { Model, Provider } from '@/gen/koda/v1/service_pb'
import { kodaClient } from '@/lib/connect'
import { errorMessage, kodaKeys } from '@/lib/koda'
import { parseContextWindowTokens } from '@/components/providers/provider-types'

export function AddModelDialog({
  models,
  onClose,
  provider,
}: {
  models: Model[]
  onClose: () => void
  provider: Provider
}) {
  const { t } = useI18n()
  const queryClient = useQueryClient()
  const [id, setId] = useState('')
  const [name, setName] = useState('')
  const [contextWindowInput, setContextWindowInput] = useState('')
  const duplicate = models.some((model) => model.id === id.trim())
  const contextWindowTokens = parseContextWindowTokens(contextWindowInput)
  const saveMutation = useMutation({
    mutationFn: () => {
      if (contextWindowTokens === null) {
        throw new Error(t('addModel.contextInvalid'))
      }
      return kodaClient.saveProvider({
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
            contextWindowTokens,
          },
        ],
        enabled: provider.enabled !== false,
      })
    },
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
      description={t('addModel.description')}
      onClose={onClose}
      title={t('addModel.title', { name: provider.name })}
    >
      <form
        className="space-y-4 p-5"
        onSubmit={(event) => {
          event.preventDefault()
          saveMutation.mutate()
        }}
      >
        <label className="field-label">
          {t('addModel.modelId')}
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
          {t('addModel.displayName')}
          <input
            className="input"
            onChange={(event) => setName(event.target.value)}
            placeholder={t('addModel.displayNamePlaceholder')}
            value={name}
          />
        </label>

        <label className="field-label">
          {t('addModel.contextWindow')}
          <input
            className="input"
            inputMode="numeric"
            onChange={(event) => setContextWindowInput(event.target.value)}
            placeholder={t('addModel.contextPlaceholder')}
            value={contextWindowInput}
          />
          <span className="mt-1 text-[11px] text-neutral-600">
            {t('addModel.contextHelp')}
          </span>
        </label>

        {duplicate && <p className="error-box">{t('addModel.duplicate')}</p>}
        {contextWindowTokens === null && (
          <p className="error-box">{t('addModel.contextInvalid')}</p>
        )}
        {saveMutation.isError && (
          <p className="error-box">{errorMessage(saveMutation.error)}</p>
        )}

        <footer className="flex justify-end gap-2 pt-1">
          <Button variant="outline" onClick={onClose} type="button">
            {t('addModel.cancel')}
          </Button>
          <Button
            disabled={
              !id.trim() ||
              duplicate ||
              contextWindowTokens === null ||
              saveMutation.isPending
            }
            type="submit"
          >
            {saveMutation.isPending && (
              <LoaderCircle className="size-4 animate-spin" />
            )}
            {t('addModel.submit')}
          </Button>
        </footer>
      </form>
    </Modal>
  )
}
