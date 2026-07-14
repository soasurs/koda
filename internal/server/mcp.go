package server

import (
	"context"
	"errors"
	"strings"

	"connectrpc.com/connect"

	v1 "github.com/soasurs/koda/gen/koda/v1"
	kodamcp "github.com/soasurs/koda/internal/mcp"
)

// ListMCPServers returns process-wide MCP servers connected when Koda started.
func (h *Handler) ListMCPServers(ctx context.Context, _ *v1.ListMCPServersRequest) (*v1.ListMCPServersResponse, error) {
	if err := ctx.Err(); err != nil {
		return nil, mcpContextError(err)
	}
	servers := mcpServers(h.mcp)
	result := make([]*v1.MCPServerSummary, len(servers))
	for index, server := range servers {
		result[index] = v1.MCPServerSummary_builder{
			Id:        new(server.ID),
			Name:      new(server.Name),
			Transport: mcpTransportToProto(server.Transport).Enum(),
			Target:    new(server.Target),
			ToolCount: new(int32(len(server.Tools))),
			ReadOnly:  new(server.ReadOnly),
		}.Build()
	}
	return v1.ListMCPServersResponse_builder{Servers: result}.Build(), nil
}

// GetMCPServer returns one connected MCP server and its discovered tools.
func (h *Handler) GetMCPServer(ctx context.Context, request *v1.GetMCPServerRequest) (*v1.GetMCPServerResponse, error) {
	if request == nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("get MCP server request must not be nil"))
	}
	if err := ctx.Err(); err != nil {
		return nil, mcpContextError(err)
	}
	id := strings.TrimSpace(request.GetId())
	if id == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("MCP server ID must not be empty"))
	}
	for _, server := range mcpServers(h.mcp) {
		if server.ID == id {
			return v1.GetMCPServerResponse_builder{Server: mcpServerToProto(server)}.Build(), nil
		}
	}
	return nil, connect.NewError(connect.CodeNotFound, errors.New("MCP server not found"))
}

func mcpServers(catalog MCPCatalog) []kodamcp.Server {
	if catalog == nil {
		return nil
	}
	return catalog.Servers()
}

func mcpServerToProto(server kodamcp.Server) *v1.MCPServer {
	tools := make([]*v1.MCPTool, len(server.Tools))
	for index, value := range server.Tools {
		tools[index] = v1.MCPTool_builder{
			Name:         new(value.Name),
			OriginalName: new(value.OriginalName),
			Description:  new(value.Description),
		}.Build()
	}
	return v1.MCPServer_builder{
		Id:        new(server.ID),
		Name:      new(server.Name),
		Transport: mcpTransportToProto(server.Transport).Enum(),
		Target:    new(server.Target),
		Tools:     tools,
		ReadOnly:  new(server.ReadOnly),
	}.Build()
}

func mcpTransportToProto(value kodamcp.Transport) v1.MCPTransport {
	switch value {
	case kodamcp.TransportHTTP:
		return v1.MCPTransport_MCP_TRANSPORT_HTTP
	case kodamcp.TransportStdio:
		return v1.MCPTransport_MCP_TRANSPORT_STDIO
	default:
		return v1.MCPTransport_MCP_TRANSPORT_UNSPECIFIED
	}
}

func mcpContextError(err error) error {
	if errors.Is(err, context.DeadlineExceeded) {
		return connect.NewError(connect.CodeDeadlineExceeded, err)
	}
	return connect.NewError(connect.CodeCanceled, err)
}
