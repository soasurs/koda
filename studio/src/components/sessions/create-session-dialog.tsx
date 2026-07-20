import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { useNavigate } from '@tanstack/react-router'
import { FolderSearch, LoaderCircle } from 'lucide-react'
import { useMemo, useState } from 'react'

import { useI18n } from '@/app/i18n'
import { Button } from '@/components/ui/button'
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
  const { t } = useI18n()
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
        description={t('createSession.description')}
        onClose={onClose}
        title={t('createSession.title')}
      >
        <form
          className="space-y-5 p-5"
          onSubmit={(event) => {
            event.preventDefault()
            createMutation.mutate()
          }}
        >
          <label className="field-label">
            {t('createSession.workspace')}
            <Button
              className="justify-start"
              onClick={() => setShowDirectoryPicker(true)}
              type="button"
              variant="outline"
            >
              <FolderSearch className="size-4 shrink-0 text-muted-foreground" />
              <span className={workdir ? 'truncate' : 'text-muted-foreground'}>
                {workdir || t('createSession.chooseDirectory')}
              </span>
            </Button>
          </label>

          <div className="grid gap-4 sm:grid-cols-2">
            <label className="field-label">
              {t('createSession.provider')}
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
                      {t('createSession.noProviders')}
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
              {t('createSession.model')}
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
                      {t('createSession.noModels')}
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
              {t('createSession.reasoningEffort')}
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
                  <SelectItem value="__default">
                    {t('createSession.providerDefault')}
                  </SelectItem>
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
              {t('createSession.fileAccess')}
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
                    {t('createSession.fileAccess.workspaceRead')}
                  </SelectItem>
                  <SelectItem value={String(FileAccess.WORKSPACE_WRITE)}>
                    {t('createSession.fileAccess.workspaceWrite')}
                  </SelectItem>
                  <SelectItem value={String(FileAccess.UNRESTRICTED)}>
                    {t('createSession.fileAccess.unrestricted')}
                  </SelectItem>
                </SelectContent>
              </Select>
            </label>
            <label className="field-label">
              {t('createSession.shellAccess')}
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
                    {t('createSession.shellAccess.askEveryTime')}
                  </SelectItem>
                  <SelectItem value={String(ShellAccess.UNRESTRICTED)}>
                    {t('createSession.shellAccess.unrestricted')}
                  </SelectItem>
                </SelectContent>
              </Select>
            </label>
          </div>

          {createMutation.isError && (
            <p className="error-box">{errorMessage(createMutation.error)}</p>
          )}

          <footer className="flex justify-end gap-2 pt-1">
            <Button onClick={onClose} type="button" variant="outline">
              {t('createSession.cancel')}
            </Button>
            <Button
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
              {t('createSession.submit')}
            </Button>
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
