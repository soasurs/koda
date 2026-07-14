import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { useNavigate } from '@tanstack/react-router'
import { FolderSearch, LoaderCircle } from 'lucide-react'
import { useMemo, useState } from 'react'

import { DirectoryPicker } from '@/components/sessions/directory-picker'
import { Modal } from '@/components/ui/modal'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { FileAccess, ShellAccess } from '@/gen/koda/v1/service_pb'
import { kodaClient } from '@/lib/connect'
import { errorMessage, kodaKeys, listProviders } from '@/lib/koda'

type CreateSessionDialogProps = {
  initialWorkdir?: string
  onClose: () => void
}

export function CreateSessionDialog({
  initialWorkdir = '',
  onClose,
}: CreateSessionDialogProps) {
  const navigate = useNavigate()
  const queryClient = useQueryClient()
  const [workdir, setWorkdir] = useState(initialWorkdir)
  const [providerId, setProviderId] = useState('')
  const [modelId, setModelId] = useState('')
  const [reasoningEffort, setReasoningEffort] = useState<string>()
  const [fileAccess, setFileAccess] = useState(FileAccess.WORKSPACE_WRITE)
  const [shellAccess, setShellAccess] = useState(ShellAccess.APPROVAL_REQUIRED)
  const [showDirectoryPicker, setShowDirectoryPicker] = useState(false)

  const providersQuery = useQuery({
    queryKey: kodaKeys.providers,
    queryFn: listProviders,
  })
  const configuredProviders = useMemo(
    () =>
      providersQuery.data?.filter(
        (provider) => provider.configured && provider.enabled !== false,
      ) ?? [],
    [providersQuery.data],
  )

  const selectedProviderId = providerId || configuredProviders[0]?.id || ''

  const modelsQuery = useQuery({
    queryKey: kodaKeys.models(selectedProviderId),
    queryFn: () => kodaClient.listModels({ providerId: selectedProviderId }),
    enabled: Boolean(selectedProviderId),
  })

  const selectedModelId =
    modelsQuery.data?.models.some((model) => model.id === modelId) && modelId
      ? modelId
      : (modelsQuery.data?.models[0]?.id ?? '')

  const selectedModel = modelsQuery.data?.models.find(
    (model) => model.id === selectedModelId,
  )
  const selectedReasoningEffort =
    reasoningEffort ?? selectedModel?.defaultReasoningEffort ?? ''

  const createMutation = useMutation({
    mutationFn: () =>
      kodaClient.createSession({
        workdir,
        providerId: selectedProviderId,
        modelId: selectedModelId,
        reasoningEffort: selectedReasoningEffort,
        fileAccess,
        shellAccess,
      }),
    onSuccess: async ({ session }) => {
      await queryClient.invalidateQueries({ queryKey: kodaKeys.sessions })
      if (session) {
        onClose()
        await navigate({
          to: '/sessions/$sessionId',
          params: { sessionId: session.id },
        })
      }
    },
  })

  return (
    <>
      <Modal
        description="Select a workspace and model for the new coding session."
        onClose={onClose}
        title="New session"
      >
        <form
          className="space-y-5 p-5"
          onSubmit={(event) => {
            event.preventDefault()
            createMutation.mutate()
          }}
        >
          <label className="field-label">
            Workspace
            <button
              className="input flex w-full items-center gap-2 text-left"
              onClick={() => setShowDirectoryPicker(true)}
              type="button"
            >
              <FolderSearch className="size-4 shrink-0 text-muted-foreground" />
              <span className={workdir ? 'truncate' : 'text-muted-foreground'}>
                {workdir || 'Choose a local directory'}
              </span>
            </button>
          </label>

          <div className="grid gap-4 sm:grid-cols-2">
            <label className="field-label">
              Provider
              <Select
                onValueChange={(value) => {
                  setProviderId(value)
                  setModelId('')
                  setReasoningEffort(undefined)
                }}
                value={selectedProviderId}
              >
                <SelectTrigger>
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  {configuredProviders.length === 0 && (
                    <SelectItem value="no-providers" disabled>
                      No configured providers
                    </SelectItem>
                  )}
                  {configuredProviders.map((provider) => (
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
                disabled={!selectedProviderId || modelsQuery.isPending}
                onValueChange={(value) => {
                  setModelId(value)
                  setReasoningEffort(undefined)
                }}
                value={selectedModelId}
              >
                <SelectTrigger>
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  {modelsQuery.data?.models.length === 0 && (
                    <SelectItem value="no-models" disabled>
                      No models available
                    </SelectItem>
                  )}
                  {modelsQuery.data?.models.map((model) => (
                    <SelectItem key={model.id} value={model.id}>
                      {model.name}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
            </label>
          </div>

          {selectedModel && selectedModel.reasoningEfforts.length > 0 && (
            <label className="field-label">
              Reasoning effort
              <Select
                onValueChange={(value) =>
                  setReasoningEffort(value === '__default' ? '' : value)
                }
                value={selectedReasoningEffort || '__default'}
              >
                <SelectTrigger>
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  <SelectItem value="__default">Provider default</SelectItem>
                  {selectedModel.reasoningEfforts.map((effort) => (
                    <SelectItem key={effort} value={effort}>
                      {effort}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
            </label>
          )}

          <div className="grid gap-4 sm:grid-cols-2">
            <label className="field-label">
              File access
              <Select
                onValueChange={(value) =>
                  setFileAccess(Number(value) as FileAccess)
                }
                value={String(fileAccess)}
              >
                <SelectTrigger>
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  <SelectItem value={String(FileAccess.WORKSPACE_READ)}>
                    Workspace read
                  </SelectItem>
                  <SelectItem value={String(FileAccess.WORKSPACE_WRITE)}>
                    Workspace write
                  </SelectItem>
                  <SelectItem value={String(FileAccess.UNRESTRICTED)}>
                    Unrestricted
                  </SelectItem>
                </SelectContent>
              </Select>
            </label>
            <label className="field-label">
              Shell access
              <Select
                onValueChange={(value) =>
                  setShellAccess(Number(value) as ShellAccess)
                }
                value={String(shellAccess)}
              >
                <SelectTrigger>
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  <SelectItem value={String(ShellAccess.APPROVAL_REQUIRED)}>
                    Ask every time
                  </SelectItem>
                  <SelectItem value={String(ShellAccess.UNRESTRICTED)}>
                    Unrestricted
                  </SelectItem>
                </SelectContent>
              </Select>
            </label>
          </div>

          {createMutation.isError && (
            <p className="error-box">{errorMessage(createMutation.error)}</p>
          )}

          <footer className="flex justify-end gap-2 pt-1">
            <button
              className="button-secondary"
              onClick={onClose}
              type="button"
            >
              Cancel
            </button>
            <button
              className="button-primary"
              disabled={
                !workdir ||
                !selectedProviderId ||
                !selectedModelId ||
                createMutation.isPending
              }
              type="submit"
            >
              {createMutation.isPending && (
                <LoaderCircle className="size-4 animate-spin" />
              )}
              Create session
            </button>
          </footer>
        </form>
      </Modal>

      {showDirectoryPicker && (
        <DirectoryPicker
          initialPath={workdir}
          onClose={() => setShowDirectoryPicker(false)}
          onSelect={(selectedPath) => {
            setWorkdir(selectedPath)
            setShowDirectoryPicker(false)
          }}
        />
      )}
    </>
  )
}
