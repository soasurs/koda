import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { CheckCircle2, KeyRound, LoaderCircle } from 'lucide-react'
import { useState } from 'react'

import { useI18n } from '@/app/i18n'
import { Button } from '@/components/ui/button'
import {
  editableProviderTypes,
  providerTypeLabels,
} from '@/components/providers/provider-types'
import { Modal } from '@/components/ui/modal'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import type { Provider } from '@/gen/koda/v1/service_pb'
import { ProviderType } from '@/gen/koda/v1/service_pb'
import { kodaClient } from '@/lib/connect'
import { errorMessage, kodaKeys } from '@/lib/koda'

export function ProviderDialog({
  onClose,
  provider,
}: {
  onClose: () => void
  provider: Provider | null
}) {
  const { t } = useI18n()
  const queryClient = useQueryClient()
  const [id, setId] = useState(provider?.id ?? '')
  const [name, setName] = useState(provider?.name ?? '')
  const [type, setType] = useState(provider?.type ?? ProviderType.UNSPECIFIED)
  const [baseUrl, setBaseUrl] = useState(provider?.baseUrl ?? '')
  const [apiKey, setApiKey] = useState('')
  const [apiKeyDirty, setApiKeyDirty] = useState(false)
  const [enabled, setEnabled] = useState(provider?.enabled !== false)
  const modelsQuery = useQuery({
    queryKey: kodaKeys.models(provider?.id ?? ''),
    queryFn: () => kodaClient.listModels({ providerId: provider?.id ?? '' }),
    enabled: Boolean(provider),
  })

  const saveMutation = useMutation({
    mutationFn: () =>
      kodaClient.saveProvider({
        id,
        name,
        type,
        baseUrl,
        modelOverrides: provider ? (modelsQuery.data?.models ?? []) : [],
        ...(apiKeyDirty || !provider ? { apiKey } : {}),
        enabled,
      }),
    onSuccess: async () => {
      await queryClient.invalidateQueries({ queryKey: kodaKeys.providers })
      await queryClient.invalidateQueries({ queryKey: kodaKeys.models(id) })
      onClose()
    },
  })

  return (
    <Modal
      description={
        provider
          ? t('provider.dialog.editDescription')
          : t('provider.dialog.addDescription')
      }
      onClose={onClose}
      title={
        provider
          ? t('provider.dialog.editTitle', { name: provider.name })
          : t('provider.dialog.addTitle')
      }
    >
      <form
        className="space-y-4 p-5"
        onSubmit={(event) => {
          event.preventDefault()
          saveMutation.mutate()
        }}
      >
        <div className="grid gap-4 sm:grid-cols-2">
          <label className="field-label">
            {t('provider.dialog.id')}
            <input
              className="input"
              disabled={Boolean(provider)}
              onChange={(event) => setId(event.target.value)}
              placeholder="my-provider"
              required
              value={id}
            />
          </label>
          <label className="field-label">
            {t('provider.dialog.displayName')}
            <input
              className="input"
              onChange={(event) => setName(event.target.value)}
              placeholder="My Provider"
              required
              value={name}
            />
          </label>
        </div>
        <label className="field-label">
          {t('provider.dialog.apiType')}
          <Select
            disabled={provider?.builtin}
            onValueChange={(value) => setType(Number(value) as ProviderType)}
            required
            value={String(type)}
          >
            <SelectTrigger>
              <SelectValue placeholder={t('provider.dialog.selectApi')} />
            </SelectTrigger>
            <SelectContent>
              {editableProviderTypes.map((providerType) => (
                <SelectItem key={providerType} value={String(providerType)}>
                  {providerTypeLabels[providerType]}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
        </label>
        <label className="field-label">
          {t('provider.dialog.baseUrl')}
          <input
            className="input"
            onChange={(event) => setBaseUrl(event.target.value)}
            placeholder={t('provider.dialog.baseUrlPlaceholder')}
            type="url"
            value={baseUrl}
          />
        </label>
        <label className="field-label">
          {t('provider.dialog.apiKey')}
          <div className="relative">
            <KeyRound className="absolute left-3 top-1/2 size-4 -translate-y-1/2 text-muted-foreground" />
            <input
              autoComplete="off"
              className="input pl-9"
              onChange={(event) => {
                setApiKey(event.target.value)
                setApiKeyDirty(true)
              }}
              placeholder={
                provider?.configured
                  ? t('provider.dialog.apiKeyKeep')
                  : t('provider.dialog.apiKeyRequired')
              }
              type="password"
              value={apiKey}
            />
          </div>
        </label>

        <label className="flex items-center gap-2.5 text-sm">
          <input
            checked={enabled}
            className="peer sr-only"
            onChange={(event) => setEnabled(event.target.checked)}
            type="checkbox"
          />
          <span className="relative inline-flex h-5 w-9 shrink-0 cursor-pointer items-center rounded-full border border-border bg-muted transition-colors after:absolute after:start-[3px] after:size-4 after:rounded-full after:bg-muted-foreground after:transition-transform peer-checked:border-emerald-700 peer-checked:bg-emerald-900/60 peer-checked:after:translate-x-[15px] peer-checked:after:bg-emerald-400" />
          {t('provider.card.enableGeneration')}
        </label>

        {saveMutation.isError && (
          <p className="error-box">{errorMessage(saveMutation.error)}</p>
        )}
        {modelsQuery.isError && (
          <p className="error-box">{errorMessage(modelsQuery.error)}</p>
        )}

        <footer className="flex justify-end gap-2 pt-1">
          <Button variant="outline" onClick={onClose} type="button">
            {t('provider.dialog.cancel')}
          </Button>
          <Button
            disabled={
              !id ||
              !name ||
              type === ProviderType.UNSPECIFIED ||
              (Boolean(provider) && !modelsQuery.data) ||
              saveMutation.isPending
            }
            type="submit"
          >
            {saveMutation.isPending ? (
              <LoaderCircle className="size-4 animate-spin" />
            ) : (
              <CheckCircle2 className="size-4" />
            )}
            {t('provider.dialog.save')}
          </Button>
        </footer>
      </form>
    </Modal>
  )
}
