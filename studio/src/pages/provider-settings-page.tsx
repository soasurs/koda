import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { LoaderCircle, Plus, Settings2 } from 'lucide-react'
import { useState } from 'react'

import { ProviderCard } from '@/components/providers/provider-card'
import { ProviderDialog } from '@/components/providers/provider-dialog'
import type { Provider } from '@/gen/koda/v1/service_pb'
import { kodaClient } from '@/lib/connect'
import { errorMessage, kodaKeys, listProviders } from '@/lib/koda'

export function ProviderSettingsPage() {
  const queryClient = useQueryClient()
  const [editingProvider, setEditingProvider] = useState<Provider | null>()
  const providersQuery = useQuery({
    queryKey: kodaKeys.providers,
    queryFn: listProviders,
  })

  const deleteMutation = useMutation({
    mutationFn: (providerId: string) =>
      kodaClient.deleteProvider({ providerId }),
    onSuccess: () =>
      queryClient.invalidateQueries({ queryKey: kodaKeys.providers }),
  })

  return (
    <section className="mx-auto flex h-full w-full max-w-6xl flex-col px-5 pt-8 sm:px-8 sm:pt-10">
      <div className="shrink-0 border-b border-neutral-800 pb-6">
        <p className="text-xs font-medium uppercase tracking-[0.18em] text-neutral-600">
          Koda Studio
        </p>
        <h1 className="mt-2 text-xl font-semibold tracking-tight">Settings</h1>
        <p className="mt-2 max-w-xl text-sm leading-6 text-neutral-500">
          Manage providers and other workspace-wide preferences.
        </p>
      </div>

      <div className="grid min-h-0 flex-1 grid-rows-[auto_minmax(0,1fr)] gap-8 pt-6 md:grid-cols-[12rem_minmax(0,1fr)] md:grid-rows-[minmax(0,1fr)]">
        <nav aria-label="Settings" className="self-start">
          <a
            aria-current="page"
            className="flex items-center gap-2 rounded-md bg-neutral-900 px-3 py-2 text-sm font-medium text-neutral-100"
            href="#providers"
          >
            <Settings2 className="size-4" aria-hidden="true" />
            Providers
          </a>
        </nav>

        <div
          id="providers"
          className="min-h-0 min-w-0 overflow-y-auto pb-8 pr-1 sm:pb-10"
        >
          <div className="flex items-start justify-between gap-5">
            <div>
              <h2 className="text-lg font-semibold tracking-tight">
                Providers
              </h2>
              <p className="mt-1 max-w-xl text-sm leading-6 text-neutral-500">
                Configure credentials and compatible endpoints stored by your
                local Koda service.
              </p>
            </div>
            <button
              className="button-primary shrink-0"
              onClick={() => setEditingProvider(null)}
              type="button"
            >
              <Plus className="size-4" />
              Add provider
            </button>
          </div>

          {providersQuery.isPending ? (
            <div className="flex h-56 items-center justify-center">
              <LoaderCircle className="size-5 animate-spin text-neutral-600" />
            </div>
          ) : providersQuery.isError ? (
            <p className="error-box mt-6">
              {errorMessage(providersQuery.error)}
            </p>
          ) : (
            <div className="mt-6 grid gap-3">
              {providersQuery.data.map((provider) => (
                <ProviderCard
                  key={provider.id}
                  onDelete={() => deleteMutation.mutate(provider.id)}
                  onEdit={() => setEditingProvider(provider)}
                  provider={provider}
                />
              ))}
            </div>
          )}
        </div>
      </div>

      {editingProvider !== undefined && (
        <ProviderDialog
          onClose={() => setEditingProvider(undefined)}
          provider={editingProvider}
        />
      )}
    </section>
  )
}
