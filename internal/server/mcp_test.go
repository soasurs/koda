package server

import (
	"context"
	"testing"

	"connectrpc.com/connect"
	"github.com/soasurs/adk/tool"

	v1 "github.com/soasurs/koda/gen/koda/v1"
	kodamcp "github.com/soasurs/koda/internal/mcp"
	kodatools "github.com/soasurs/koda/internal/tools"
)

func TestListAndGetMCPServers(t *testing.T) {
	handler := &Handler{mcp: fakeMCPCatalog{servers: []kodamcp.Server{{
		ID: "exa", Name: "Exa", Transport: kodamcp.TransportHTTP, Target: "https://mcp.exa.ai/mcp", ReadOnly: true,
		Tools: []kodamcp.Tool{{Name: "mcp__exa__web_search", OriginalName: "web_search", Description: "Search the web"}},
	}}}}
	listed, err := handler.ListMCPServers(t.Context(), v1.ListMCPServersRequest_builder{}.Build())
	if err != nil {
		t.Fatalf("ListMCPServers() error = %v", err)
	}
	if len(listed.GetServers()) != 1 || listed.GetServers()[0].GetId() != "exa" || listed.GetServers()[0].GetToolCount() != 1 ||
		listed.GetServers()[0].GetTransport() != v1.MCPTransport_MCP_TRANSPORT_HTTP || !listed.GetServers()[0].GetReadOnly() {
		t.Fatalf("ListMCPServers() = %+v", listed)
	}
	got, err := handler.GetMCPServer(t.Context(), v1.GetMCPServerRequest_builder{Id: new("exa")}.Build())
	if err != nil {
		t.Fatalf("GetMCPServer() error = %v", err)
	}
	if got.GetServer().GetTarget() != "https://mcp.exa.ai/mcp" || got.GetServer().GetTools()[0].GetOriginalName() != "web_search" {
		t.Fatalf("GetMCPServer() = %+v", got)
	}
}

func TestGetMCPServerValidatesRequest(t *testing.T) {
	handler := &Handler{}
	if _, err := handler.GetMCPServer(t.Context(), nil); connect.CodeOf(err) != connect.CodeInvalidArgument {
		t.Fatalf("GetMCPServer(nil) code = %v", connect.CodeOf(err))
	}
	if _, err := handler.GetMCPServer(t.Context(), v1.GetMCPServerRequest_builder{}.Build()); connect.CodeOf(err) != connect.CodeInvalidArgument {
		t.Fatalf("GetMCPServer(empty) code = %v", connect.CodeOf(err))
	}
	if _, err := handler.GetMCPServer(t.Context(), v1.GetMCPServerRequest_builder{Id: new("missing")}.Build()); connect.CodeOf(err) != connect.CodeNotFound {
		t.Fatalf("GetMCPServer(missing) code = %v", connect.CodeOf(err))
	}
}

func TestListMCPServersMapsCanceledContext(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if _, err := (&Handler{}).ListMCPServers(ctx, v1.ListMCPServersRequest_builder{}.Build()); connect.CodeOf(err) != connect.CodeCanceled {
		t.Fatalf("ListMCPServers() code = %v", connect.CodeOf(err))
	}
}

type fakeMCPCatalog struct {
	servers []kodamcp.Server
}

func (c fakeMCPCatalog) BuildTools(kodatools.Authorizer) []tool.Tool {
	return nil
}

func (c fakeMCPCatalog) PlanTools() []tool.Tool {
	return nil
}

func (c fakeMCPCatalog) Servers() []kodamcp.Server {
	return append([]kodamcp.Server(nil), c.servers...)
}
