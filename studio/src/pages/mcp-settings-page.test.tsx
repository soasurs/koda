import { create } from '@bufbuild/protobuf'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import type { ReactNode } from 'react'
import { beforeEach, describe, expect, it, vi } from 'vitest'

import {
  MCPServerSchema,
  MCPServerSummarySchema,
  MCPTransport,
  MCPToolSchema,
} from '@/gen/koda/v1/service_pb'

const { getMCPServerMock, listMCPServersMock } = vi.hoisted(() => ({
  getMCPServerMock: vi.fn(),
  listMCPServersMock: vi.fn(),
}))

vi.mock('@/components/settings/settings-layout', () => ({
  SettingsLayout: ({ children }: { children: ReactNode }) => (
    <div>{children}</div>
  ),
}))

vi.mock('@/lib/koda', () => ({
  errorMessage: (error: Error) => error.message,
  getMCPServer: getMCPServerMock,
  kodaKeys: {
    mcpServers: ['mcp-servers'],
    mcpServer: (id: string) => ['mcp-server', id],
  },
  listMCPServers: listMCPServersMock,
}))

import { MCPSettingsPage } from '@/pages/mcp-settings-page'

describe('MCPSettingsPage', () => {
  beforeEach(() => {
    listMCPServersMock.mockReset()
    getMCPServerMock.mockReset()
  })

  it('shows connected servers and discovered tools', async () => {
    listMCPServersMock.mockResolvedValue([
      create(MCPServerSummarySchema, {
        id: 'exa',
        name: 'Exa',
        transport: MCPTransport.MCP_TRANSPORT_HTTP,
        target: 'https://mcp.exa.ai/mcp',
        toolCount: 1,
        readOnly: true,
      }),
    ])
    getMCPServerMock.mockResolvedValue(
      create(MCPServerSchema, {
        id: 'exa',
        name: 'Exa',
        transport: MCPTransport.MCP_TRANSPORT_HTTP,
        target: 'https://mcp.exa.ai/mcp',
        readOnly: true,
        tools: [
          create(MCPToolSchema, {
            name: 'mcp__exa__web_search',
            originalName: 'web_search',
            description: 'Search the web',
          }),
        ],
      }),
    )
    const queryClient = new QueryClient({
      defaultOptions: { queries: { retry: false } },
    })

    render(
      <QueryClientProvider client={queryClient}>
        <MCPSettingsPage />
      </QueryClientProvider>,
    )

    await userEvent.click(
      await screen.findByRole('button', { name: 'Open Exa' }),
    )
    expect(
      await screen.findByRole('heading', { name: 'Exa' }),
    ).toBeInTheDocument()
    expect(screen.getByText('mcp__exa__web_search')).toBeInTheDocument()
    expect(screen.getByText('MCP name: web_search')).toBeInTheDocument()
    expect(screen.getByText('Search the web')).toBeInTheDocument()
    expect(getMCPServerMock).toHaveBeenCalledWith('exa')
  })

  it('shows an empty state when no servers are configured', async () => {
    listMCPServersMock.mockResolvedValue([])
    const queryClient = new QueryClient({
      defaultOptions: { queries: { retry: false } },
    })

    render(
      <QueryClientProvider client={queryClient}>
        <MCPSettingsPage />
      </QueryClientProvider>,
    )

    expect(
      await screen.findByText('No MCP servers configured'),
    ).toBeInTheDocument()
    expect(getMCPServerMock).not.toHaveBeenCalled()
  })
})
