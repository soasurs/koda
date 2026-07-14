// Package mcp loads process-wide MCP servers and exposes their tools to Koda agents.
package mcp

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"slices"
	"strings"
	"sync"

	sdkmcp "github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/soasurs/adk/tool"
	adkmcp "github.com/soasurs/adk/tool/mcp"

	"github.com/soasurs/koda/internal/config"
	"github.com/soasurs/koda/internal/logging"
	kodatools "github.com/soasurs/koda/internal/tools"
)

const defaultMaxResultBytes = 32 * 1024

// Transport identifies an MCP transport exposed through the public API.
type Transport string

const (
	// TransportHTTP uses MCP streamable HTTP.
	TransportHTTP Transport = "http"
	// TransportStdio launches an MCP server subprocess and uses stdin/stdout.
	TransportStdio Transport = "stdio"
)

// Tool describes one MCP tool exposed to agents.
type Tool struct {
	Name         string
	OriginalName string
	Description  string
}

// Server describes one connected process-wide MCP server.
type Server struct {
	ID        string
	Name      string
	Transport Transport
	Target    string
	ReadOnly  bool
	Tools     []Tool
}

// Manager owns connected MCP sessions and their immutable startup tool catalog.
type Manager struct {
	mu          sync.Mutex
	closed      bool
	connections []toolConnection
	servers     []Server
	tools       []managedTool
	planTools   []tool.Tool
}

type managedTool struct {
	value      tool.Tool
	serverID   string
	serverName string
	readOnly   bool
}

type toolConnection interface {
	Tools(context.Context) ([]tool.Tool, error)
	Close() error
}

type connectFunc func(context.Context, sdkmcp.Transport) (toolConnection, error)

// Open connects every configured MCP server and discovers its tools. A
// configured server that cannot connect or expose a valid tool catalog makes
// startup fail so Koda never silently drops an expected capability.
func Open(ctx context.Context, values config.MCPConfig, logger *slog.Logger) (*Manager, error) {
	return open(ctx, values, logging.OrDiscard(logger), connectToolSet)
}

func open(ctx context.Context, values config.MCPConfig, logger *slog.Logger, connect connectFunc) (*Manager, error) {
	if ctx == nil {
		return nil, errors.New("mcp: context must not be nil")
	}
	if connect == nil {
		return nil, errors.New("mcp: connector must not be nil")
	}
	logger = logging.OrDiscard(logger)
	manager := new(Manager)
	seenServers := make(map[string]struct{}, len(values.Servers))
	seenTools := make(map[string]string)
	for index, value := range values.Servers {
		resolved, transport, err := newTransport(value)
		if err != nil {
			manager.closeConnected(logger)
			return nil, fmt.Errorf("mcp: server %d: %w", index, err)
		}
		if _, exists := seenServers[resolved.ID]; exists {
			manager.closeConnected(logger)
			return nil, fmt.Errorf("mcp: duplicate server ID %q", resolved.ID)
		}
		seenServers[resolved.ID] = struct{}{}

		connection, err := connect(ctx, transport)
		if err != nil {
			manager.closeConnected(logger)
			return nil, fmt.Errorf("mcp: connect server %q: %w", resolved.ID, err)
		}
		manager.connections = append(manager.connections, connection)
		values, err := connection.Tools(ctx)
		if err != nil {
			manager.closeConnected(logger)
			return nil, fmt.Errorf("mcp: list tools for server %q: %w", resolved.ID, err)
		}
		server := Server{ID: resolved.ID, Name: resolved.Name, Transport: resolved.Transport, Target: resolved.Target, ReadOnly: value.ReadOnly}
		for _, value := range values {
			if value == nil {
				manager.closeConnected(logger)
				return nil, fmt.Errorf("mcp: server %q returned a nil tool", resolved.ID)
			}
			definition := value.Definition()
			name, err := exposedToolName(resolved.ID, definition.Name)
			if err != nil {
				manager.closeConnected(logger)
				return nil, fmt.Errorf("mcp: server %q: %w", resolved.ID, err)
			}
			if owner, exists := seenTools[name]; exists {
				manager.closeConnected(logger)
				return nil, fmt.Errorf("mcp: tool name %q from server %q conflicts with server %q", name, resolved.ID, owner)
			}
			seenTools[name] = resolved.ID
			wrapped := newNamespacedTool(value, name, definition.Name, resolved.Name)
			manager.tools = append(manager.tools, managedTool{value: wrapped, serverID: server.ID, serverName: server.Name, readOnly: server.ReadOnly})
			if server.ReadOnly {
				manager.planTools = append(manager.planTools, wrapped)
			}
			server.Tools = append(server.Tools, Tool{Name: name, OriginalName: definition.Name, Description: definition.Description})
		}
		slices.SortFunc(server.Tools, func(a, b Tool) int { return strings.Compare(a.Name, b.Name) })
		manager.servers = append(manager.servers, server)
		logger.InfoContext(ctx, "MCP server connected", "server_id", server.ID, "transport", server.Transport, "tool_count", len(server.Tools))
	}
	slices.SortFunc(manager.servers, func(a, b Server) int { return strings.Compare(a.ID, b.ID) })
	return manager, nil
}

// PlanTools returns MCP tools whose server is explicitly configured read-only.
func (m *Manager) PlanTools() []tool.Tool {
	if m == nil {
		return nil
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	return append([]tool.Tool(nil), m.planTools...)
}

func connectToolSet(ctx context.Context, transport sdkmcp.Transport) (toolConnection, error) {
	toolSet := adkmcp.NewToolSet(transport)
	if err := toolSet.Connect(ctx); err != nil {
		return nil, err
	}
	return toolSet, nil
}

// BuildTools returns every MCP tool, wrapping tools not declared read-only in
// Koda's per-call approval flow.
func (m *Manager) BuildTools(authorizer kodatools.Authorizer) []tool.Tool {
	if m == nil {
		return nil
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	result := make([]tool.Tool, len(m.tools))
	for index, value := range m.tools {
		result[index] = value.value
		if !value.readOnly {
			result[index] = newApprovalTool(value.value, value.serverID, value.serverName, authorizer)
		}
	}
	return result
}

// Servers returns a deep copy of the connected MCP server catalog.
func (m *Manager) Servers() []Server {
	if m == nil {
		return nil
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	result := make([]Server, len(m.servers))
	for index, server := range m.servers {
		result[index] = server
		result[index].Tools = append([]Tool(nil), server.Tools...)
	}
	return result
}

// Close closes every MCP connection. It is safe to call more than once.
func (m *Manager) Close() error {
	if m == nil {
		return nil
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.closed {
		return nil
	}
	m.closed = true
	var result error
	for index := len(m.connections) - 1; index >= 0; index-- {
		result = errors.Join(result, m.connections[index].Close())
	}
	return result
}

func (m *Manager) closeConnected(logger *slog.Logger) {
	for index := len(m.connections) - 1; index >= 0; index-- {
		if err := m.connections[index].Close(); err != nil {
			logger.Warn("close MCP connection after startup failure", "error", err)
		}
	}
}
