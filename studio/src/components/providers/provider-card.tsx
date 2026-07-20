import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import {
  Check,
  ChevronDown,
  LoaderCircle,
  Pencil,
  Plus,
  RefreshCw,
  Server,
  Trash2,
  X,
} from 'lucide-react'
import { useState } from 'react'

import { useI18n } from '@/app/i18n'
import { Button } from '@/components/ui/button'
import { AddModelDialog } from '@/components/providers/add-model-dialog'
import { EditModelDialog } from '@/components/providers/edit-model-dialog'
import {
  formatContextWindowTokens,
  providerTypeLabels,
} from '@/components/providers/provider-types'
import type { Model, Provider } from '@/gen/koda/v1/service_pb'
import { kodaClient } from '@/lib/connect'
import { errorMessage, kodaKeys } from '@/lib/koda'

export function ProviderCard({
  onDelete,
  onEdit,
  provider,
}: {
  onDelete: () => void
  onEdit: () => void
  provider: Provider
}) {
  const { t } = useI18n()
  const queryClient = useQueryClient()
  const [expanded, setExpanded] = useState(false)
  const [showAddModel, setShowAddModel] = useState(false)
  const [editingModel, setEditingModel] = useState<Model | null>(null)
  const [deletingModelId, setDeletingModelId] = useState<string | null>(null)
  const modelsQuery = useQuery({
    queryKey: kodaKeys.models(provider.id),
    queryFn: () => kodaClient.listModels({ providerId: provider.id }),
  })
  const refreshMutation = useMutation({
    mutationFn: () => kodaClient.refreshModels({ providerId: provider.id }),
    onSuccess: () =>
      queryClient.invalidateQueries({ queryKey: kodaKeys.models(provider.id) }),
  })
  const toggleEnabledMutation = useMutation({
    mutationFn: (enabled: boolean) =>
      kodaClient.saveProvider({
        id: provider.id,
        name: provider.name,
        type: provider.type,
        baseUrl: provider.baseUrl,
        modelOverrides: modelsQuery.data?.models ?? [],
        enabled,
      }),
    onSuccess: () =>
      queryClient.invalidateQueries({ queryKey: kodaKeys.providers }),
  })
  const deleteModelMutation = useMutation({
    mutationFn: () => {
      const remaining = (modelsQuery.data?.models ?? []).filter(
        (m) => m.id !== deletingModelId,
      )
      return kodaClient.saveProvider({
        id: provider.id,
        name: provider.name,
        type: provider.type,
        baseUrl: provider.baseUrl,
        modelOverrides: remaining,
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
      setDeletingModelId(null)
    },
  })

  return (
    <article
      className={`relative overflow-hidden rounded-lg border px-4 py-3.5 ${
        provider.enabled === false
          ? 'border-border bg-muted/20 opacity-60'
          : provider.configured
            ? 'border-emerald-200 bg-emerald-50/50 dark:border-emerald-950 dark:bg-emerald-950/10'
            : 'border-border bg-muted/20'
      }`}
    >
      <div
        className={`absolute inset-y-0 left-0 w-0.5 ${
          provider.enabled === false
            ? 'bg-muted-foreground'
            : provider.configured
              ? 'bg-emerald-500'
              : 'bg-muted-foreground'
        }`}
      />
      <div className="flex items-center gap-1">
        <Button
          aria-expanded={expanded}
          className="h-auto flex-1 justify-start rounded-md p-0 text-left"
          onClick={() => setExpanded((value) => !value)}
          variant="ghost"
        >
          <ChevronDown
            className={`size-3.5 shrink-0 text-muted-foreground transition-transform ${expanded ? '' : '-rotate-90'}`}
          />
          <div
            className={`flex size-8 shrink-0 items-center justify-center rounded-md border ${
              provider.configured
                ? 'border-emerald-200 bg-emerald-100 text-emerald-700 dark:border-emerald-900 dark:bg-emerald-950 dark:text-emerald-400'
                : 'border-border bg-muted text-muted-foreground'
            }`}
          >
            <Server className="size-3.5" />
          </div>
          <div className="min-w-0 flex-1">
            <div className="flex min-w-0 flex-wrap items-center gap-2">
              <h2 className="truncate text-sm font-medium">{provider.name}</h2>
              {provider.builtin && (
                <span className="badge">{t('provider.card.builtIn')}</span>
              )}
            </div>
            <p className="mt-0.5 truncate text-xs text-muted-foreground">
              {providerTypeLabels[provider.type]}
              {provider.baseUrl ? ` · ${provider.baseUrl}` : ''}
              {' · '}
              {modelsQuery.isPending
                ? t('provider.card.loadingModels')
                : t('provider.card.modelCount', {
                    count: modelsQuery.data?.models.length ?? 0,
                  })}
            </p>
          </div>
        </Button>
        <div className="flex shrink-0 items-center gap-1">
          <span
            className={`inline-flex items-center gap-1.5 text-xs font-medium ${
              provider.configured
                ? 'text-emerald-700 dark:text-emerald-400'
                : 'text-muted-foreground'
            }`}
          >
            <span
              className={`size-1.5 rounded-full ${
                provider.configured ? 'bg-emerald-500' : 'bg-muted-foreground'
              }`}
            />
            {provider.configured
              ? t('provider.card.ready')
              : t('provider.card.notConfigured')}
          </span>
          <label className="flex cursor-pointer items-center">
            <input
              checked={provider.enabled !== false}
              className="peer sr-only"
              disabled={
                modelsQuery.isPending || toggleEnabledMutation.isPending
              }
              onChange={(event) =>
                toggleEnabledMutation.mutate(event.target.checked)
              }
              type="checkbox"
            />
            <span className="relative inline-flex h-5 w-9 shrink-0 cursor-pointer items-center rounded-full border border-border bg-muted transition-colors after:absolute after:inset-s-0.75 after:size-4 after:rounded-full after:bg-muted-foreground after:transition-transform peer-checked:border-emerald-700 peer-checked:bg-emerald-900/60 peer-checked:after:translate-x-3.75 peer-checked:after:bg-emerald-400 peer-disabled:cursor-not-allowed peer-disabled:opacity-50" />
          </label>
          <Button
            aria-label={t('provider.card.refreshAria', { name: provider.name })}
            disabled={!provider.configured || refreshMutation.isPending}
            onClick={() => refreshMutation.mutate()}
            size="icon"
            title={t('provider.card.refreshTitle')}
            variant="ghost"
          >
            <RefreshCw
              className={`size-4 ${refreshMutation.isPending ? 'animate-spin' : ''}`}
            />
          </Button>
          <Button onClick={onEdit} variant="outline">
            {t('provider.card.configure')}
          </Button>
          {!provider.builtin && (
            <Button
              aria-label={t('provider.card.deleteAria', {
                name: provider.name,
              })}
              onClick={() => {
                if (
                  window.confirm(
                    t('provider.card.deleteConfirm', { name: provider.name }),
                  )
                )
                  onDelete()
              }}
              size="icon"
              variant="ghost"
            >
              <Trash2 className="size-4" />
            </Button>
          )}
        </div>
      </div>

      {expanded && (
        <div className="ml-6 mt-3 border-t border-border/80 pt-3">
          <div className="mb-2 flex items-center justify-between gap-3">
            <p className="text-xs font-medium text-muted-foreground">
              {t('provider.card.models')}
            </p>
            <Button
              disabled={modelsQuery.isPending || modelsQuery.isError}
              onClick={() => setShowAddModel(true)}
              size="xs"
              variant="outline"
            >
              <Plus className="size-3.5" />
              {t('provider.card.addModel')}
            </Button>
          </div>
          {modelsQuery.isPending ? (
            <div className="flex h-16 items-center justify-center">
              <LoaderCircle className="size-4 animate-spin text-muted-foreground" />
            </div>
          ) : modelsQuery.isError ? (
            <p className="error-box">{errorMessage(modelsQuery.error)}</p>
          ) : modelsQuery.data.models.length === 0 ? (
            <p className="py-4 text-center text-xs text-muted-foreground">
              {t('provider.card.noModels')}
            </p>
          ) : (
            <div className="divide-y divide-border/70 rounded-md border border-border/80">
              {modelsQuery.data.models.map((model) => (
                <div
                  className="flex items-center gap-3 px-3 py-2 text-xs"
                  key={model.id}
                >
                  <div className="min-w-0 flex-1">
                    <p className="truncate font-medium text-foreground">
                      {model.name || model.id}
                    </p>
                    {model.name && model.name !== model.id && (
                      <p className="mt-0.5 truncate font-mono text-[11px] text-muted-foreground">
                        {model.id}
                      </p>
                    )}
                  </div>
                  {deletingModelId === model.id ? (
                    <div className="flex shrink-0 items-center gap-1.5">
                      <span className="text-[11px] text-muted-foreground">
                        {t('provider.card.deleteModelPrompt')}
                      </span>
                      <Button
                        aria-label={t('provider.card.confirmDeleteAria')}
                        disabled={deleteModelMutation.isPending}
                        onClick={() => deleteModelMutation.mutate()}
                        size="icon-xs"
                        variant="ghost"
                      >
                        {deleteModelMutation.isPending ? (
                          <LoaderCircle className="size-3 animate-spin" />
                        ) : (
                          <Check className="size-3" />
                        )}
                      </Button>
                      <Button
                        aria-label={t('provider.card.cancelDeleteAria')}
                        onClick={() => setDeletingModelId(null)}
                        size="icon-xs"
                        variant="ghost"
                      >
                        <X className="size-3" />
                      </Button>
                    </div>
                  ) : (
                    <>
                      {model.reasoningEfforts.length > 0 && (
                        <span className="text-[11px] text-muted-foreground">
                          {t('provider.card.reasoning', {
                            efforts: model.reasoningEfforts.join(', '),
                          })}
                        </span>
                      )}
                      {model.contextWindowTokens > 0n && (
                        <span className="text-[11px] text-muted-foreground">
                          {t('provider.card.context', {
                            tokens: formatContextWindowTokens(
                              model.contextWindowTokens,
                            ),
                          })}
                        </span>
                      )}
                      <Button
                        aria-label={t('provider.card.editModelAria', {
                          id: model.id,
                        })}
                        onClick={() => setEditingModel(model)}
                        size="icon-xs"
                        variant="ghost"
                      >
                        <Pencil className="size-3" />
                      </Button>
                      <Button
                        aria-label={t('provider.card.deleteModelAria', {
                          id: model.id,
                        })}
                        onClick={() => setDeletingModelId(model.id)}
                        size="icon-xs"
                        variant="ghost"
                      >
                        <Trash2 className="size-3" />
                      </Button>
                    </>
                  )}
                </div>
              ))}
            </div>
          )}
          {refreshMutation.isError && (
            <p className="mt-2 text-xs text-red-400">
              {errorMessage(refreshMutation.error)}
            </p>
          )}
          {deleteModelMutation.isError && (
            <p className="mt-2 text-xs text-red-400">
              {errorMessage(deleteModelMutation.error)}
            </p>
          )}
        </div>
      )}

      {showAddModel && modelsQuery.data && (
        <AddModelDialog
          models={modelsQuery.data.models}
          onClose={() => setShowAddModel(false)}
          provider={provider}
        />
      )}

      {editingModel && modelsQuery.data && (
        <EditModelDialog
          model={editingModel}
          models={modelsQuery.data.models}
          onClose={() => setEditingModel(null)}
          provider={provider}
        />
      )}
    </article>
  )
}
