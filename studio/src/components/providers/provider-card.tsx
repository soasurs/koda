import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import {
  ChevronDown,
  LoaderCircle,
  Plus,
  RefreshCw,
  Server,
  Trash2,
} from 'lucide-react'
import { useState } from 'react'

import { AddModelDialog } from '@/components/providers/add-model-dialog'
import { providerTypeLabels } from '@/components/providers/provider-types'
import type { Provider } from '@/gen/koda/v1/service_pb'
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
  const queryClient = useQueryClient()
  const [expanded, setExpanded] = useState(false)
  const [showAddModel, setShowAddModel] = useState(false)
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

  return (
    <article
      className={`relative overflow-hidden rounded-lg border px-4 py-3.5 ${
        provider.enabled === false
          ? 'border-neutral-800 bg-neutral-900/20 opacity-60'
          : provider.configured
            ? 'border-emerald-200 bg-emerald-50/50 dark:border-emerald-950 dark:bg-emerald-950/10'
            : 'border-neutral-800 bg-neutral-900/20'
      }`}
    >
      <div
        className={`absolute inset-y-0 left-0 w-0.5 ${
          provider.enabled === false
            ? 'bg-neutral-700'
            : provider.configured
              ? 'bg-emerald-500'
              : 'bg-neutral-700'
        }`}
      />
      <div className="flex items-center gap-1">
        <button
          aria-expanded={expanded}
          className="flex min-w-0 flex-1 items-center gap-3 rounded-md text-left"
          onClick={() => setExpanded((value) => !value)}
          type="button"
        >
          <ChevronDown
            className={`size-3.5 shrink-0 text-neutral-600 transition-transform ${expanded ? '' : '-rotate-90'}`}
          />
          <div
            className={`flex size-8 shrink-0 items-center justify-center rounded-md border ${
              provider.configured
                ? 'border-emerald-200 bg-emerald-100 text-emerald-700 dark:border-emerald-900 dark:bg-emerald-950 dark:text-emerald-400'
                : 'border-neutral-800 bg-neutral-900 text-neutral-600'
            }`}
          >
            <Server className="size-3.5" />
          </div>
          <div className="min-w-0 flex-1">
            <div className="flex min-w-0 flex-wrap items-center gap-2">
              <h2 className="truncate text-sm font-medium">{provider.name}</h2>
              {provider.builtin && <span className="badge">Built in</span>}
            </div>
            <p className="mt-0.5 truncate text-xs text-neutral-500">
              {providerTypeLabels[provider.type]}
              {provider.baseUrl ? ` · ${provider.baseUrl}` : ''}
              {' · '}
              {modelsQuery.isPending
                ? 'Loading models…'
                : `${modelsQuery.data?.models.length ?? 0} models`}
            </p>
          </div>
        </button>
        <div className="flex shrink-0 items-center gap-1">
          <span
            className={`inline-flex items-center gap-1.5 text-xs font-medium ${
              provider.configured
                ? 'text-emerald-700 dark:text-emerald-400'
                : 'text-neutral-600'
            }`}
          >
            <span
              className={`size-1.5 rounded-full ${
                provider.configured ? 'bg-emerald-500' : 'bg-neutral-600'
              }`}
            />
            {provider.configured ? 'Ready' : 'Not configured'}
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
            <span className="relative inline-flex h-5 w-9 shrink-0 cursor-pointer items-center rounded-full border border-neutral-700 bg-neutral-800/60 transition-colors after:absolute after:start-[3px] after:size-4 after:rounded-full after:bg-neutral-500 after:transition-transform peer-checked:border-emerald-700 peer-checked:bg-emerald-900/60 peer-checked:after:translate-x-[15px] peer-checked:after:bg-emerald-400 peer-disabled:cursor-not-allowed peer-disabled:opacity-50" />
          </label>
          <button
            aria-label={`Refresh ${provider.name} models`}
            className="icon-button"
            disabled={!provider.configured || refreshMutation.isPending}
            onClick={() => refreshMutation.mutate()}
            title="Refresh models"
            type="button"
          >
            <RefreshCw
              className={`size-4 ${refreshMutation.isPending ? 'animate-spin' : ''}`}
            />
          </button>
          <button
            className="button-secondary px-2.5 py-1.5"
            onClick={onEdit}
            type="button"
          >
            Configure
          </button>
          {!provider.builtin && (
            <button
              aria-label={`Delete ${provider.name}`}
              className="icon-button hover:text-red-400"
              onClick={() => {
                if (window.confirm(`Delete ${provider.name}?`)) onDelete()
              }}
              type="button"
            >
              <Trash2 className="size-4" />
            </button>
          )}
        </div>
      </div>

      {expanded && (
        <div className="ml-6 mt-3 border-t border-neutral-800/80 pt-3">
          <div className="mb-2 flex items-center justify-between gap-3">
            <p className="text-xs font-medium text-neutral-400">Models</p>
            <button
              className="button-secondary px-2 py-1 text-xs"
              disabled={modelsQuery.isPending || modelsQuery.isError}
              onClick={() => setShowAddModel(true)}
              type="button"
            >
              <Plus className="size-3.5" />
              Add model
            </button>
          </div>
          {modelsQuery.isPending ? (
            <div className="flex h-16 items-center justify-center">
              <LoaderCircle className="size-4 animate-spin text-neutral-600" />
            </div>
          ) : modelsQuery.isError ? (
            <p className="error-box">{errorMessage(modelsQuery.error)}</p>
          ) : modelsQuery.data.models.length === 0 ? (
            <p className="py-4 text-center text-xs text-neutral-600">
              No models available
            </p>
          ) : (
            <div className="divide-y divide-neutral-800/70 rounded-md border border-neutral-800/80">
              {modelsQuery.data.models.map((model) => (
                <div
                  className="flex items-center gap-3 px-3 py-2 text-xs"
                  key={model.id}
                >
                  <div className="min-w-0 flex-1">
                    <p className="truncate font-medium text-neutral-300">
                      {model.name || model.id}
                    </p>
                    {model.name && model.name !== model.id && (
                      <p className="mt-0.5 truncate font-mono text-[11px] text-neutral-600">
                        {model.id}
                      </p>
                    )}
                  </div>
                  {model.reasoningEfforts.length > 0 && (
                    <span className="text-[11px] text-neutral-600">
                      Reasoning: {model.reasoningEfforts.join(', ')}
                    </span>
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
        </div>
      )}

      {showAddModel && modelsQuery.data && (
        <AddModelDialog
          models={modelsQuery.data.models}
          onClose={() => setShowAddModel(false)}
          provider={provider}
        />
      )}
    </article>
  )
}
