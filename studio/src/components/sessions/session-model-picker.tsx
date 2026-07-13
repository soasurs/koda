import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { ChevronUp, LoaderCircle } from 'lucide-react'
import { useEffect, useRef, useState } from 'react'

import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import type { Session } from '@/gen/koda/v1/service_pb'
import { kodaClient } from '@/lib/connect'
import { errorMessage, kodaKeys, listProviders } from '@/lib/koda'

export function SessionModelPicker({
  disabled,
  session,
}: {
  disabled: boolean
  session: Session
}) {
  const queryClient = useQueryClient()
  const pickerRef = useRef<HTMLDivElement>(null)
  const [open, setOpen] = useState(false)
  const [providerId, setProviderId] = useState(session.providerId)
  const [modelId, setModelId] = useState(session.modelId)
  const [reasoningEffort, setReasoningEffort] = useState(
    session.reasoningEffort,
  )

  const providersQuery = useQuery({
    queryKey: kodaKeys.providers,
    queryFn: listProviders,
  })
  const currentModelsQuery = useQuery({
    queryKey: kodaKeys.models(session.providerId),
    queryFn: () => kodaClient.listModels({ providerId: session.providerId }),
  })
  const modelsQuery = useQuery({
    queryKey: kodaKeys.models(providerId),
    queryFn: () => kodaClient.listModels({ providerId }),
    enabled: open && Boolean(providerId),
  })
  const availableProviders =
    providersQuery.data?.filter(
      (provider) =>
        (provider.configured && provider.enabled !== false) ||
        provider.id === session.providerId,
    ) ?? []
  const selectedModelId =
    modelsQuery.data?.models.some((model) => model.id === modelId) && modelId
      ? modelId
      : (modelsQuery.data?.models[0]?.id ?? '')
  const selectedModel = modelsQuery.data?.models.find(
    (model) => model.id === selectedModelId,
  )
  const displayModel =
    currentModelsQuery.data?.models.find(
      (model) => model.id === session.modelId,
    )?.name || session.modelId
  const displayEffort =
    session.reasoningEffort ||
    currentModelsQuery.data?.models.find(
      (model) => model.id === session.modelId,
    )?.defaultReasoningEffort ||
    'default'

  useEffect(() => {
    if (!open) return

    function closeOnOutsideClick(event: PointerEvent) {
      const target = event.target
      const isPickerSelect =
        target instanceof Element &&
        target.closest('[data-session-model-picker-select]')
      const isInsidePicker =
        target instanceof Node && pickerRef.current?.contains(target)
      if (!isPickerSelect && !isInsidePicker) {
        setOpen(false)
      }
    }

    function closeOnEscape(event: KeyboardEvent) {
      if (event.key === 'Escape') setOpen(false)
    }

    document.addEventListener('pointerdown', closeOnOutsideClick)
    document.addEventListener('keydown', closeOnEscape)
    return () => {
      document.removeEventListener('pointerdown', closeOnOutsideClick)
      document.removeEventListener('keydown', closeOnEscape)
    }
  }, [open])

  const updateMutation = useMutation({
    mutationFn: () =>
      kodaClient.updateSession({
        sessionId: session.id,
        providerId,
        modelId: selectedModelId,
        reasoningEffort,
      }),
    onSuccess: async () => {
      await Promise.all([
        queryClient.invalidateQueries({
          queryKey: kodaKeys.session(session.id),
        }),
        queryClient.invalidateQueries({ queryKey: kodaKeys.sessions }),
      ])
      setOpen(false)
    },
  })

  return (
    <div className="relative" ref={pickerRef}>
      <button
        aria-expanded={open}
        aria-label="Session model settings"
        className="flex h-8 max-w-56 items-center gap-1.5 rounded-md border border-neutral-800 bg-neutral-950 px-2.5 text-xs text-neutral-400 transition-colors hover:border-neutral-700 hover:text-neutral-200 disabled:opacity-40"
        disabled={disabled}
        onClick={() => {
          if (!open) {
            setProviderId(session.providerId)
            setModelId(session.modelId)
            setReasoningEffort(session.reasoningEffort)
          }
          setOpen((value) => !value)
        }}
        title={`${session.providerId} · ${session.modelId} · ${displayEffort}`}
        type="button"
      >
        <span className="truncate">{displayModel}</span>
        <span className="shrink-0 text-neutral-600">· {displayEffort}</span>
        <ChevronUp className="size-3 shrink-0 text-neutral-600" />
      </button>

      {open && (
        <div className="absolute bottom-full right-0 z-20 mb-2 w-[min(20rem,calc(100vw-2rem))] rounded-lg border border-neutral-700 bg-neutral-950 p-4 shadow-2xl">
          <div className="space-y-3">
            <label className="field-label">
              Provider
              <Select
                disabled={providersQuery.isPending || updateMutation.isPending}
                onValueChange={(value) => {
                  setProviderId(value)
                  setModelId('')
                  setReasoningEffort('')
                }}
                value={providerId}
              >
                <SelectTrigger aria-label="Session provider">
                  <SelectValue />
                </SelectTrigger>
                <SelectContent data-session-model-picker-select side="top">
                  {availableProviders.map((provider) => (
                    <SelectItem key={provider.id} value={provider.id}>
                      {provider.name}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
            </label>
            <label className="field-label">
              Model
              <Select
                disabled={modelsQuery.isPending || updateMutation.isPending}
                onValueChange={(value) => {
                  setModelId(value)
                  setReasoningEffort('')
                }}
                value={selectedModelId}
              >
                <SelectTrigger aria-label="Session model">
                  <SelectValue />
                </SelectTrigger>
                <SelectContent data-session-model-picker-select side="top">
                  {modelsQuery.data?.models.map((model) => (
                    <SelectItem key={model.id} value={model.id}>
                      {model.name || model.id}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
            </label>
            <label className="field-label">
              Reasoning effort
              <Select
                disabled={!selectedModel || updateMutation.isPending}
                onValueChange={(value) =>
                  setReasoningEffort(value === '__default' ? '' : value)
                }
                value={reasoningEffort || '__default'}
              >
                <SelectTrigger aria-label="Session reasoning effort">
                  <SelectValue />
                </SelectTrigger>
                <SelectContent data-session-model-picker-select side="top">
                  <SelectItem value="__default">Provider default</SelectItem>
                  {selectedModel?.reasoningEfforts.map((effort) => (
                    <SelectItem key={effort} value={effort}>
                      {effort}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
            </label>
          </div>

          {updateMutation.isError && (
            <p className="error-box mt-3">
              {errorMessage(updateMutation.error)}
            </p>
          )}

          <div className="mt-4 flex justify-end gap-2">
            <button
              className="button-secondary px-2.5 py-1.5"
              onClick={() => setOpen(false)}
              type="button"
            >
              Cancel
            </button>
            <button
              className="button-primary px-2.5 py-1.5"
              disabled={
                !providerId || !selectedModelId || updateMutation.isPending
              }
              onClick={() => updateMutation.mutate()}
              type="button"
            >
              {updateMutation.isPending && (
                <LoaderCircle className="size-3.5 animate-spin" />
              )}
              Apply
            </button>
          </div>
        </div>
      )}
    </div>
  )
}
