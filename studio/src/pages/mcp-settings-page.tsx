import { useQuery } from '@tanstack/react-query'
import { ChevronRight, LoaderCircle, Network, PackageOpen } from 'lucide-react'
import { useState } from 'react'

import { SettingsLayout } from '@/components/settings/settings-layout'
import { Modal } from '@/components/ui/modal'
import { MCPTransport, type MCPServer } from '@/gen/koda/v1/service_pb'
import {
  errorMessage,
  getMCPServer,
  kodaKeys,
  listMCPServers,
} from '@/lib/koda'

export function MCPSettingsPage() {
  const [selectedID, setSelectedID] = useState<string>()
  const serversQuery = useQuery({
    queryKey: kodaKeys.mcpServers,
    queryFn: listMCPServers,
  })
  const serverQuery = useQuery({
    queryKey: kodaKeys.mcpServer(selectedID ?? ''),
    queryFn: () => getMCPServer(selectedID ?? ''),
    enabled: Boolean(selectedID),
  })
  const selectedSummary = serversQuery.data?.find(
    (server) => server.id === selectedID,
  )

  return (
    <SettingsLayout active="mcp">
      <div>
        <h2 className="text-lg font-semibold tracking-tight">MCP servers</h2>
        <p className="mt-1 max-w-2xl text-sm leading-6 text-muted-foreground">
          Inspect the process-wide MCP servers loaded from ~/.koda/koda.yaml.
          Restart Koda to apply configuration changes.
        </p>
      </div>

      {serversQuery.isPending ? (
        <div className="flex h-56 items-center justify-center">
          <LoaderCircle className="size-5 animate-spin text-muted-foreground" />
        </div>
      ) : serversQuery.isError ? (
        <p className="error-box mt-6">{errorMessage(serversQuery.error)}</p>
      ) : serversQuery.data.length === 0 ? (
        <div className="mt-6 rounded-lg border border-dashed border-border px-6 py-12 text-center">
          <PackageOpen className="mx-auto size-6 text-muted-foreground" />
          <p className="mt-3 text-sm font-medium text-foreground">
            No MCP servers configured
          </p>
          <p className="mt-1 text-sm text-muted-foreground">
            Add mcp.servers entries to ~/.koda/koda.yaml and restart Koda.
          </p>
        </div>
      ) : (
        <div className="mt-6 grid gap-3 sm:grid-cols-2">
          {serversQuery.data.map((server) => (
            <button
              aria-label={`Open ${server.name}`}
              className="group flex min-h-28 items-start gap-3 rounded-lg border border-border bg-background p-4 text-left transition hover:border-border/80 hover:bg-accent"
              key={server.id}
              onClick={() => setSelectedID(server.id)}
              type="button"
            >
              <span className="flex size-8 shrink-0 items-center justify-center rounded-md bg-muted text-muted-foreground group-hover:text-foreground">
                <Network className="size-4" aria-hidden="true" />
              </span>
              <span className="min-w-0 flex-1">
                <span className="block text-sm font-medium text-foreground">
                  {server.name}
                </span>
                <span className="mt-1 block truncate text-xs text-muted-foreground">
                  {transportLabel(server.transport)} · {server.toolCount}{' '}
                  {server.toolCount === 1 ? 'tool' : 'tools'} ·{' '}
                  {server.readOnly ? 'Plan + Build' : 'Build with approval'}
                </span>
                <span className="mt-1 block truncate text-xs text-muted-foreground">
                  {server.target}
                </span>
              </span>
              <ChevronRight
                className="mt-1 size-4 shrink-0 text-muted-foreground group-hover:text-foreground"
                aria-hidden="true"
              />
            </button>
          ))}
        </div>
      )}

      {selectedID && (
        <Modal
          description={selectedSummary?.target}
          onClose={() => setSelectedID(undefined)}
          title={selectedSummary?.name ?? selectedID}
          wide
        >
          <div className="min-h-40 p-5 sm:p-6">
            {serverQuery.isPending ? (
              <div className="flex min-h-40 items-center justify-center">
                <LoaderCircle className="size-5 animate-spin text-muted-foreground" />
              </div>
            ) : serverQuery.isError ? (
              <p className="error-box">{errorMessage(serverQuery.error)}</p>
            ) : serverQuery.data ? (
              <MCPServerDetails server={serverQuery.data} />
            ) : null}
          </div>
        </Modal>
      )}
    </SettingsLayout>
  )
}

function MCPServerDetails({ server }: { server: MCPServer }) {
  return (
    <article className="min-w-0">
      <dl className="grid gap-3 border-b border-border pb-5 text-sm sm:grid-cols-2">
        <Detail label="ID" value={server.id} />
        <Detail label="Transport" value={transportLabel(server.transport)} />
        <Detail
          label="Agent modes"
          value={server.readOnly ? 'Plan and Build' : 'Build with approval'}
        />
        <div className="sm:col-span-2">
          <Detail label="Target" value={server.target} />
        </div>
      </dl>

      <section className="mt-6">
        <h4 className="text-xs font-medium uppercase tracking-wider text-muted-foreground">
          Tools
        </h4>
        {server.tools.length === 0 ? (
          <p className="mt-3 text-sm text-muted-foreground">
            This server exposed no tools at startup.
          </p>
        ) : (
          <div className="mt-3 grid gap-3">
            {server.tools.map((tool) => (
              <div
                className="rounded-lg border border-border bg-muted p-4"
                key={tool.name}
              >
                <code className="break-all text-xs text-foreground">
                  {tool.name}
                </code>
                {tool.originalName !== tool.name && (
                  <p className="mt-1 text-xs text-muted-foreground">
                    MCP name: {tool.originalName}
                  </p>
                )}
                {tool.description && (
                  <p className="mt-2 text-sm leading-5 text-muted-foreground">
                    {tool.description}
                  </p>
                )}
              </div>
            ))}
          </div>
        )}
      </section>
    </article>
  )
}

function Detail({ label, value }: { label: string; value: string }) {
  return (
    <div>
      <dt className="text-xs font-medium uppercase tracking-wider text-muted-foreground">
        {label}
      </dt>
      <dd className="mt-1 wrap-break-word text-foreground">{value}</dd>
    </div>
  )
}

function transportLabel(transport: MCPTransport): string {
  switch (transport) {
    case MCPTransport.MCP_TRANSPORT_HTTP:
      return 'HTTP'
    case MCPTransport.MCP_TRANSPORT_STDIO:
      return 'stdio'
    default:
      return 'Unknown'
  }
}
