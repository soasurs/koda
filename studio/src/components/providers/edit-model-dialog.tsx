import { useMutation, useQueryClient } from '@tanstack/react-query'
import { LoaderCircle } from 'lucide-react'
import { useMemo, useState } from 'react'

import { useI18n } from '@/app/i18n'
import { Button } from '@/components/ui/button'
import { Modal } from '@/components/ui/modal'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import type { Model, Provider } from '@/gen/koda/v1/service_pb'
import { kodaClient } from '@/lib/connect'
import { errorMessage, kodaKeys } from '@/lib/koda'
import { parseContextWindowTokens } from '@/components/providers/provider-types'

function parseEfforts(input: string): string[] {
  return input
    .split(',')
    .map((s) => s.trim())
    .filter(Boolean)
}

export function EditModelDialog({
  model,
  models,
  onClose,
  provider,
}: {
  model: Model
  models: Model[]
  onClose: () => void
  provider: Provider
}) {
  const { t } = useI18n()
  const queryClient = useQueryClient()
  const [name, setName] = useState(model.name || '')
  const [reasoningEffortsInput, setReasoningEffortsInput] = useState(
    (model.reasoningEfforts ?? []).join(', '),
  )
  const [defaultEffort, setDefaultEffort] = useState(
    model.defaultReasoningEffort || '__default',
  )
  const [contextWindowInput, setContextWindowInput] = useState(
    model.contextWindowTokens > 0n ? model.contextWindowTokens.toString() : '',
  )

  const efforts = useMemo(
    () => parseEfforts(reasoningEffortsInput),
    [reasoningEffortsInput],
  )
  const contextWindowTokens = parseContextWindowTokens(contextWindowInput)

  const saveMutation = useMutation({
    mutationFn: () => {
      if (contextWindowTokens === null) {
        throw new Error(t('editModel.contextInvalid'))
      }
      const otherModels = models.filter((m) => m.id !== model.id)
      return kodaClient.saveProvider({
        id: provider.id,
        name: provider.name,
        type: provider.type,
        baseUrl: provider.baseUrl,
        modelOverrides: [
          ...otherModels,
          {
            id: model.id,
            name: name.trim(),
            reasoningEfforts: efforts,
            defaultReasoningEffort:
              defaultEffort === '__default' ? '' : defaultEffort,
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
      description={t('editModel.description')}
      onClose={onClose}
      title={t('editModel.title', { id: model.id })}
    >
      <form
        className="space-y-4 p-5"
        onSubmit={(event) => {
          event.preventDefault()
          saveMutation.mutate()
        }}
      >
        <label className="field-label">
          {t('editModel.modelId')}
          <input className="input" disabled value={model.id} />
        </label>

        <label className="field-label">
          {t('editModel.displayName')}
          <input
            className="input"
            onChange={(event) => setName(event.target.value)}
            placeholder={t('editModel.displayNamePlaceholder')}
            value={name}
          />
        </label>

        <label className="field-label">
          {t('editModel.reasoningEfforts')}
          <input
            className="input"
            onChange={(event) => setReasoningEffortsInput(event.target.value)}
            placeholder={t('editModel.reasoningPlaceholder')}
            value={reasoningEffortsInput}
          />
          <span className="mt-1 text-[11px] text-neutral-600">
            {t('editModel.reasoningHelp')}
          </span>
        </label>

        <label className="field-label">
          {t('editModel.contextWindow')}
          <input
            className="input"
            inputMode="numeric"
            onChange={(event) => setContextWindowInput(event.target.value)}
            placeholder={t('editModel.contextPlaceholder')}
            value={contextWindowInput}
          />
          <span className="mt-1 text-[11px] text-neutral-600">
            {t('editModel.contextHelp')}
          </span>
        </label>

        {efforts.length > 0 && (
          <label className="field-label">
            {t('editModel.defaultReasoningEffort')}
            <Select
              onValueChange={setDefaultEffort}
              value={defaultEffort || '__default'}
            >
              <SelectTrigger>
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                <SelectItem value="__default">
                  {t('editModel.providerDefault')}
                </SelectItem>
                {efforts.map((effort) => (
                  <SelectItem key={effort} value={effort}>
                    {effort}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
          </label>
        )}

        {saveMutation.isError && (
          <p className="error-box">{errorMessage(saveMutation.error)}</p>
        )}
        {contextWindowTokens === null && (
          <p className="error-box">{t('editModel.contextInvalid')}</p>
        )}

        <footer className="flex justify-end gap-2 pt-1">
          <Button variant="outline" onClick={onClose} type="button">
            {t('editModel.cancel')}
          </Button>
          <Button
            disabled={contextWindowTokens === null || saveMutation.isPending}
            type="submit"
          >
            {saveMutation.isPending && (
              <LoaderCircle className="size-4 animate-spin" />
            )}
            {t('editModel.save')}
          </Button>
        </footer>
      </form>
    </Modal>
  )
}
