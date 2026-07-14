import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { LoaderCircle, Plus } from 'lucide-react'
import { useState } from 'react'

import { Button } from '@/components/ui/button'
import { ProviderCard } from '@/components/providers/provider-card'
import { ProviderDialog } from '@/components/providers/provider-dialog'
import { SettingsLayout } from '@/components/settings/settings-layout'
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
    <SettingsLayout active="providers">
      <div id="providers">
        <div className="flex items-start justify-between gap-5">
          <div>
            <h2 className="text-lg font-semibold tracking-tight">Providers</h2>
            <p className="mt-1 max-w-xl text-sm leading-6 text-muted-foreground">
              Configure credentials and compatible endpoints stored by your
              local Koda service.
            </p>
          </div>
          <Button onClick={() => setEditingProvider(null)}>
            <Plus className="size-4" />
            Add provider
          </Button>
        </div>

        {providersQuery.isPending ? (
          <div className="flex h-56 items-center justify-center">
            <LoaderCircle className="size-5 animate-spin text-muted-foreground" />
          </div>
        ) : providersQuery.isError ? (
          <p className="error-box mt-6">{errorMessage(providersQuery.error)}</p>
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

      {editingProvider !== undefined && (
        <ProviderDialog
          onClose={() => setEditingProvider(undefined)}
          provider={editingProvider}
        />
      )}
    </SettingsLayout>
  )
}
